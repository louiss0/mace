package lsp

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/sourcegraph/jsonrpc2"
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/louiss0/mace/internal/formatter"
	"github.com/louiss0/mace/internal/lexer"
	"github.com/louiss0/mace/internal/parser"
	"github.com/louiss0/mace/internal/parser/ast"
)

const (
	serverName    = "mace"
	serverVersion = "dev"
)

var diagnosticPositionPattern = regexp.MustCompile(`at (\d+):(\d+)`)

var keywordDocs = map[string]string{
	"array":       "Declares an array type like `array<string>`.",
	"injectable":  "Marks a script variable as overrideable through injections.",
	"output":      "Configures whether the output block returns `data` or `schema`.",
	"schema":      "Declares a reusable record schema.",
	"schema_file": "References a schema from another Mace file.",
	"type":        "Declares a reusable type alias.",
}

var keywordCompletions = []completionDefinition{
	{Label: "array", Kind: protocol.CompletionItemKindKeyword, Detail: "type constructor"},
	{Label: "boolean", Kind: protocol.CompletionItemKindKeyword, Detail: "primitive type"},
	{Label: "data", Kind: protocol.CompletionItemKindKeyword, Detail: "output mode"},
	{Label: "float", Kind: protocol.CompletionItemKindKeyword, Detail: "primitive type"},
	{Label: "from", Kind: protocol.CompletionItemKindKeyword, Detail: "import declaration"},
	{Label: "import", Kind: protocol.CompletionItemKindKeyword, Detail: "import declaration"},
	{Label: "injectable", Kind: protocol.CompletionItemKindKeyword, Detail: "variable modifier"},
	{Label: "int", Kind: protocol.CompletionItemKindKeyword, Detail: "primitive type"},
	{Label: "output", Kind: protocol.CompletionItemKindKeyword, Detail: "output directive"},
	{Label: "schema", Kind: protocol.CompletionItemKindKeyword, Detail: "schema declaration"},
	{Label: "schema_file", Kind: protocol.CompletionItemKindKeyword, Detail: "output directive"},
	{Label: "string", Kind: protocol.CompletionItemKindKeyword, Detail: "primitive type"},
	{Label: "type", Kind: protocol.CompletionItemKindKeyword, Detail: "type declaration"},
}

type Server struct {
	documents map[protocol.DocumentUri]document
	handler   protocol.Handler
	lock      sync.RWMutex
}

type document struct {
	text     string
	version  protocol.UInteger
	analysis analysisSnapshot
}

type completionDefinition struct {
	Label  string
	Kind   protocol.CompletionItemKind
	Detail string
}

type declarationDefinition struct {
	Name   string
	Kind   protocol.CompletionItemKind
	Detail string
}

func New() *Server {
	server := &Server{
		documents: map[protocol.DocumentUri]document{},
	}

	server.handler = protocol.Handler{
		Initialize:                 server.initialize,
		Initialized:                server.initialized,
		Shutdown:                   server.shutdown,
		SetTrace:                   server.setTrace,
		TextDocumentDidOpen:        server.didOpen,
		TextDocumentDidChange:      server.didChange,
		TextDocumentDidClose:       server.didClose,
		TextDocumentCompletion:     server.complete,
		TextDocumentHover:          server.hover,
		TextDocumentDocumentSymbol: server.documentSymbols,
		TextDocumentFormatting:     server.formatDocument,
	}

	return server
}

func (server *Server) Handler() *protocol.Handler {
	return &server.handler
}

func (server *Server) RunStdio() error {
	connection := jsonrpc2.NewConn(
		context.Background(),
		jsonrpc2.NewBufferedStream(stdrwc{}, jsonrpc2.VSCodeObjectCodec{}),
		jsonrpc2.HandlerWithError(server.handle).SuppressErrClosed(),
	)

	<-connection.DisconnectNotify()
	return nil
}

func (server *Server) initialize(context *glsp.Context, params *protocol.InitializeParams) (any, error) {
	capabilities := server.handler.CreateServerCapabilities()
	if syncOptions, ok := capabilities.TextDocumentSync.(*protocol.TextDocumentSyncOptions); ok {
		syncMode := protocol.TextDocumentSyncKindFull
		syncOptions.Change = &syncMode
	}

	if capabilities.CompletionProvider != nil {
		capabilities.CompletionProvider.TriggerCharacters = []string{".", ":"}
	}

	return protocol.InitializeResult{
		Capabilities: capabilities,
		ServerInfo: &protocol.InitializeResultServerInfo{
			Name:    serverName,
			Version: Ptr(serverVersion),
		},
	}, nil
}

func (server *Server) initialized(context *glsp.Context, params *protocol.InitializedParams) error {
	return nil
}

func (server *Server) shutdown(context *glsp.Context) error {
	protocol.SetTraceValue(protocol.TraceValueOff)
	return nil
}

func (server *Server) setTrace(context *glsp.Context, params *protocol.SetTraceParams) error {
	protocol.SetTraceValue(params.Value)
	return nil
}

func (server *Server) didOpen(context *glsp.Context, params *protocol.DidOpenTextDocumentParams) error {
	analysis := analyzeDocument(params.TextDocument.Text)

	server.lock.Lock()
	server.documents[params.TextDocument.URI] = document{
		text:     params.TextDocument.Text,
		version:  protocol.UInteger(params.TextDocument.Version),
		analysis: analysis,
	}
	server.lock.Unlock()

	server.publishDiagnostics(context, params.TextDocument.URI)
	return nil
}

func (server *Server) didChange(context *glsp.Context, params *protocol.DidChangeTextDocumentParams) error {
	server.lock.Lock()
	current := server.documents[params.TextDocument.URI]
	text := current.text

	for _, changeValue := range params.ContentChanges {
		switch change := changeValue.(type) {
		case protocol.TextDocumentContentChangeEvent:
			if change.Range == nil {
				text = change.Text
				continue
			}

			start, end := change.Range.IndexesIn(text)
			if start < 0 || end < start || end > len(text) {
				server.lock.Unlock()
				return fmt.Errorf("lsp: invalid text change range")
			}

			text = text[:start] + change.Text + text[end:]
		case protocol.TextDocumentContentChangeEventWhole:
			text = change.Text
		default:
			server.lock.Unlock()
			return fmt.Errorf("lsp: unsupported text change payload")
		}
	}

	analysis := analyzeDocument(text)

	server.documents[params.TextDocument.URI] = document{
		text:     text,
		version:  protocol.UInteger(params.TextDocument.Version),
		analysis: analysis,
	}
	server.lock.Unlock()

	server.publishDiagnostics(context, params.TextDocument.URI)
	return nil
}

func (server *Server) didClose(context *glsp.Context, params *protocol.DidCloseTextDocumentParams) error {
	server.lock.Lock()
	delete(server.documents, params.TextDocument.URI)
	server.lock.Unlock()

	server.notifyDiagnostics(context, protocol.PublishDiagnosticsParams{
		URI:         params.TextDocument.URI,
		Diagnostics: []protocol.Diagnostic{},
	})
	return nil
}

func (server *Server) complete(context *glsp.Context, params *protocol.CompletionParams) (any, error) {
	document, ok := server.document(params.TextDocument.URI)
	if !ok {
		return []protocol.CompletionItem{}, nil
	}

	prefix := identifierPrefixAt(document.text, params.Position)
	items := []protocol.CompletionItem{}

	for _, completion := range keywordCompletions {
		if strings.HasPrefix(completion.Label, prefix) {
			item := protocol.CompletionItem{
				Label: completion.Label,
				Kind:  Ptr(completion.Kind),
			}
			if completion.Detail != "" {
				item.Detail = Ptr(completion.Detail)
			}
			items = append(items, item)
		}
	}

	for _, declaration := range document.analysis.declarations {
		if declaration.Name == "" || !strings.HasPrefix(declaration.Name, prefix) {
			continue
		}

		item := protocol.CompletionItem{
			Label: declaration.Name,
			Kind:  Ptr(declaration.Kind),
		}
		if declaration.Detail != "" {
			item.Detail = Ptr(declaration.Detail)
		}
		items = append(items, item)
	}

	slices.SortFunc(items, func(left protocol.CompletionItem, right protocol.CompletionItem) int {
		return strings.Compare(left.Label, right.Label)
	})

	return items, nil
}

func (server *Server) hover(context *glsp.Context, params *protocol.HoverParams) (*protocol.Hover, error) {
	document, ok := server.document(params.TextDocument.URI)
	if !ok {
		return nil, nil
	}

	identifier, found := identifierAt(document.text, params.Position)
	if !found {
		return nil, nil
	}

	if documentation, ok := keywordDocs[identifier]; ok {
		return &protocol.Hover{
			Contents: protocol.MarkupContent{
				Kind:  protocol.MarkupKindMarkdown,
				Value: documentation,
			},
		}, nil
	}

	for _, declaration := range document.analysis.declarations {
		if declaration.Name != identifier {
			continue
		}

		return &protocol.Hover{
			Contents: protocol.MarkupContent{
				Kind:  protocol.MarkupKindMarkdown,
				Value: "```mace\n" + declaration.Detail + "\n```",
			},
		}, nil
	}

	return nil, nil
}

func (server *Server) documentSymbols(context *glsp.Context, params *protocol.DocumentSymbolParams) (any, error) {
	document, ok := server.document(params.TextDocument.URI)
	if !ok {
		return []protocol.DocumentSymbol{}, nil
	}

	if document.analysis.file == nil {
		return []protocol.DocumentSymbol{}, nil
	}

	symbols := []protocol.DocumentSymbol{}
	file := *document.analysis.file
	if file.Script != nil {
		for _, item := range file.Script.Items {
			switch declaration := item.(type) {
			case ast.TypeDeclaration:
				symbols = append(symbols, newSymbol(document.text, declaration.Name, "type", protocol.SymbolKindClass, nil))
			case ast.SchemaDeclaration:
				children := []protocol.DocumentSymbol{}
				for _, field := range declaration.Type.Fields {
					children = append(children, newSymbol(document.text, field.Name, fieldTypeDetail(field.Type), protocol.SymbolKindField, nil))
				}
				symbols = append(symbols, newSymbol(document.text, declaration.Name, "schema", protocol.SymbolKindStruct, children))
			case ast.VariableDeclaration:
				symbols = append(symbols, newSymbol(document.text, declaration.Name, typeReferenceDetail(declaration.Type), protocol.SymbolKindVariable, nil))
			}
		}
	}

	if len(file.Output.DataFields) > 0 || len(file.Output.SchemaFields) > 0 {
		children := []protocol.DocumentSymbol{}
		for _, field := range file.Output.DataFields {
			detail := "output field"
			if document.analysis.result != nil {
				if value, ok := document.analysis.result.Output[field.Name]; ok {
					detail = "output field = " + summarizeValue(value)
				}
			}
			children = append(children, newSymbol(document.text, field.Name, detail, protocol.SymbolKindProperty, nil))
		}
		for _, field := range file.Output.SchemaFields {
			children = append(children, newSymbol(document.text, field.Name, fieldTypeDetail(field.Type), protocol.SymbolKindProperty, nil))
		}
		symbols = append(symbols, newSymbol(document.text, "output", "output block", protocol.SymbolKindObject, children))
	}

	return symbols, nil
}

func (server *Server) formatDocument(context *glsp.Context, params *protocol.DocumentFormattingParams) ([]protocol.TextEdit, error) {
	document, ok := server.document(params.TextDocument.URI)
	if !ok {
		return []protocol.TextEdit{}, nil
	}

	if document.analysis.file == nil {
		file, err := parseFile(document.text)
		if err != nil {
			return nil, err
		}
		document.analysis.file = &file
	}

	formatted, err := formatter.FormatFile(*document.analysis.file)
	if err != nil {
		return nil, err
	}

	return []protocol.TextEdit{
		{
			Range: protocol.Range{
				Start: protocol.Position{},
				End:   positionFromIndex(document.text, len(document.text)),
			},
			NewText: formatted,
		},
	}, nil
}

func (server *Server) publishDiagnostics(context *glsp.Context, uri protocol.DocumentUri) {
	document, ok := server.document(uri)
	if !ok {
		return
	}

	server.notifyDiagnostics(context, protocol.PublishDiagnosticsParams{
		URI:         uri,
		Version:     Ptr(document.version),
		Diagnostics: document.analysis.diagnostics,
	})
}

func (server *Server) notifyDiagnostics(context *glsp.Context, params protocol.PublishDiagnosticsParams) {
	if context.Notify == nil {
		return
	}

	context.Notify(protocol.ServerTextDocumentPublishDiagnostics, params)
}

func (server *Server) document(uri protocol.DocumentUri) (document, bool) {
	server.lock.RLock()
	defer server.lock.RUnlock()

	document, ok := server.documents[uri]
	return document, ok
}

func diagnosticFromError(err error) protocol.Diagnostic {
	position := protocol.Position{}
	matches := diagnosticPositionPattern.FindStringSubmatch(err.Error())
	if len(matches) == 3 {
		line := parseUint(matches[1])
		column := parseUint(matches[2])
		if line > 0 {
			position.Line = line - 1
		}
		if column > 0 {
			position.Character = column - 1
		}
	}

	end := position
	end.Character++

	return protocol.Diagnostic{
		Severity: Ptr(protocol.DiagnosticSeverityError),
		Source:   Ptr(serverName),
		Message:  err.Error(),
		Range: protocol.Range{
			Start: position,
			End:   end,
		},
	}
}

func parseUint(value string) protocol.UInteger {
	var parsed protocol.UInteger
	_, _ = fmt.Sscanf(value, "%d", &parsed)
	return parsed
}

func parseFile(text string) (ast.File, error) {
	tokens, err := lex(text)
	if err != nil {
		return ast.File{}, err
	}

	return parser.New(tokens).ParseFile()
}

func lex(text string) ([]lexer.Token, error) {
	lexerInstance := lexer.New(text)
	tokens := []lexer.Token{}

	for {
		token, err := lexerInstance.NextToken()
		if err != nil {
			return nil, err
		}

		tokens = append(tokens, token)
		if token.Type == lexer.TokenEOF {
			return tokens, nil
		}
	}
}

func typeReferenceDetail(typeReference ast.TypeReference) string {
	switch value := typeReference.(type) {
	case ast.PrimitiveType:
		return value.Name
	case ast.NamedType:
		return value.Name
	case ast.ArrayType:
		return fmt.Sprintf("array<%s>", typeReferenceDetail(value.Element))
	case ast.RecordType:
		return recordTypeDetail(value)
	default:
		return "unknown"
	}
}

func fieldTypeDetail(typeReference ast.TypeReference) string {
	return typeReferenceDetail(typeReference)
}

func recordTypeDetail(record ast.RecordType) string {
	fields := make([]string, 0, len(record.Fields))
	for _, field := range record.Fields {
		name := field.Name
		if field.Optional {
			name += "?"
		}
		fields = append(fields, fmt.Sprintf("%s: %s", name, typeReferenceDetail(field.Type)))
	}

	return "{ " + strings.Join(fields, "; ") + "; }"
}

func newSymbol(text string, name string, detail string, kind protocol.SymbolKind, children []protocol.DocumentSymbol) protocol.DocumentSymbol {
	start, end := nameRange(text, name)
	symbol := protocol.DocumentSymbol{
		Name: name,
		Kind: kind,
		Range: protocol.Range{
			Start: start,
			End:   end,
		},
		SelectionRange: protocol.Range{
			Start: start,
			End:   end,
		},
		Children: children,
	}

	if detail != "" {
		symbol.Detail = Ptr(detail)
	}

	return symbol
}

func nameRange(text string, name string) (protocol.Position, protocol.Position) {
	index := strings.Index(text, name)
	if index < 0 {
		return protocol.Position{}, protocol.Position{}
	}

	start := positionFromIndex(text, index)
	end := positionFromIndex(text, index+len(name))
	return start, end
}

func positionFromIndex(text string, index int) protocol.Position {
	line := protocol.UInteger(0)
	column := protocol.UInteger(0)

	for currentIndex, runeValue := range text {
		if currentIndex >= index {
			break
		}
		if runeValue == '\n' {
			line++
			column = 0
			continue
		}
		column++
	}

	return protocol.Position{
		Line:      line,
		Character: column,
	}
}

func identifierPrefixAt(text string, position protocol.Position) string {
	index := position.IndexIn(text)
	if index < 0 {
		return ""
	}

	start := index
	for start > 0 && isIdentifierCharacter(text[start-1]) {
		start--
	}

	return text[start:index]
}

func identifierAt(text string, position protocol.Position) (string, bool) {
	index := position.IndexIn(text)
	if index < 0 || index > len(text) {
		return "", false
	}

	start := index
	for start > 0 && isIdentifierCharacter(text[start-1]) {
		start--
	}

	end := index
	for end < len(text) && isIdentifierCharacter(text[end]) {
		end++
	}

	if start == end {
		return "", false
	}

	return text[start:end], true
}

func isIdentifierCharacter(value byte) bool {
	return value == '_' ||
		(value >= 'a' && value <= 'z') ||
		(value >= 'A' && value <= 'Z') ||
		(value >= '0' && value <= '9')
}

func Ptr[T any](value T) *T {
	return &value
}

func (server *Server) handle(
	context context.Context,
	connection *jsonrpc2.Conn,
	request *jsonrpc2.Request,
) (any, error) {
	glspContext := glsp.Context{
		Method: request.Method,
		Notify: func(method string, params any) {
			_ = connection.Notify(context, method, params)
		},
		Call: func(method string, params any, result any) {
			_ = connection.Call(context, method, params, result)
		},
	}

	if request.Params != nil {
		glspContext.Params = *request.Params
	}

	switch request.Method {
	case protocol.MethodExit:
		_, _, _, _ = server.handler.Handle(&glspContext)
		return nil, connection.Close()
	default:
		result, validMethod, validParams, err := server.handler.Handle(&glspContext)
		if !validMethod {
			return nil, &jsonrpc2.Error{
				Code:    jsonrpc2.CodeMethodNotFound,
				Message: fmt.Sprintf("method not supported: %s", request.Method),
			}
		}
		if !validParams {
			if err != nil {
				return nil, &jsonrpc2.Error{
					Code:    jsonrpc2.CodeInvalidParams,
					Message: err.Error(),
				}
			}

			return nil, &jsonrpc2.Error{Code: jsonrpc2.CodeInvalidParams}
		}
		if err != nil {
			return nil, &jsonrpc2.Error{
				Code:    jsonrpc2.CodeInvalidRequest,
				Message: err.Error(),
			}
		}

		return result, nil
	}
}

type stdrwc struct{}

func (stdrwc) Read(buffer []byte) (int, error) {
	return os.Stdin.Read(buffer)
}

func (stdrwc) Write(buffer []byte) (int, error) {
	return os.Stdout.Write(buffer)
}

func (stdrwc) Close() error {
	err := os.Stdin.Close()
	if err == nil {
		return os.Stdout.Close()
	}

	return err
}
