package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/samber/lo"
	"github.com/sourcegraph/jsonrpc2"
	"github.com/spf13/cobra"
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/louiss0/mace/internal/analyzer"
)

const (
	serverName    = "mace"
	serverVersion = "dev"
)

type Server struct {
	documents        map[protocol.DocumentUri]document
	workspaceRootDir string
	handler          protocol.Handler
	lock             sync.RWMutex
}

type document struct {
	text     string
	version  protocol.UInteger
	analysis analyzer.Snapshot
}

type stdrwc struct{}

type textChangeResult struct {
	text string
	err  error
}

func newLSPCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "lsp",
		Short: "Run the Mace language server over stdio",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, args []string) error {
			server := newLSPServer()
			return server.RunStdio()
		},
	}
}

func newLSPServer() *Server {
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
		TextDocumentDidSave:        server.didSave,
		TextDocumentDidClose:       server.didClose,
		TextDocumentCompletion:     server.complete,
		TextDocumentHover:          server.hover,
		TextDocumentDefinition:     server.definition,
		TextDocumentDocumentSymbol: server.documentSymbols,
		TextDocumentCodeAction:     server.codeActions,
		TextDocumentRename:         server.rename,
		TextDocumentPrepareRename:  server.prepareRename,
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
	server.workspaceRootDir = workspaceRootDir(params)
	capabilities := server.handler.CreateServerCapabilities()
	if syncOptions, ok := capabilities.TextDocumentSync.(*protocol.TextDocumentSyncOptions); ok {
		syncMode := protocol.TextDocumentSyncKindFull
		syncOptions.Change = &syncMode
		syncOptions.Save = &protocol.SaveOptions{
			IncludeText: analyzer.Ptr(true),
		}
	}

	if capabilities.CompletionProvider != nil {
		capabilities.CompletionProvider.TriggerCharacters = []string{".", ":", "=", "$"}
	}
	return protocol.InitializeResult{
		Capabilities: capabilities,
		ServerInfo: &protocol.InitializeResultServerInfo{
			Name:    serverName,
			Version: analyzer.Ptr(serverVersion),
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
	analysis := analyzer.AnalyzeDocumentAtInRoot(params.TextDocument.Text, documentPath(params.TextDocument.URI), server.importRootDir(documentPath(params.TextDocument.URI)))

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
	changeResult := lo.Reduce(params.ContentChanges, func(aggregate textChangeResult, changeValue any, _ int) textChangeResult {
		if aggregate.err != nil {
			return aggregate
		}

		switch change := changeValue.(type) {
		case protocol.TextDocumentContentChangeEvent:
			if change.Range == nil {
				aggregate.text = change.Text
				return aggregate
			}

			start, end := change.Range.IndexesIn(aggregate.text)
			if start < 0 || end < start || end > len(aggregate.text) {
				aggregate.err = fmt.Errorf("lsp: invalid text change range")
				return aggregate
			}

			aggregate.text = aggregate.text[:start] + change.Text + aggregate.text[end:]
			return aggregate
		case protocol.TextDocumentContentChangeEventWhole:
			aggregate.text = change.Text
			return aggregate
		default:
			aggregate.err = fmt.Errorf("lsp: unsupported text change payload")
			return aggregate
		}
	}, textChangeResult{text: current.text})
	if changeResult.err != nil {
		server.lock.Unlock()
		return changeResult.err
	}

	analysis := analyzer.AnalyzeDocumentAtInRoot(changeResult.text, documentPath(params.TextDocument.URI), server.importRootDir(documentPath(params.TextDocument.URI)))
	server.documents[params.TextDocument.URI] = document{
		text:     changeResult.text,
		version:  protocol.UInteger(params.TextDocument.Version),
		analysis: analysis,
	}
	server.lock.Unlock()

	server.publishDiagnostics(context, params.TextDocument.URI)
	return nil
}

func (server *Server) didSave(context *glsp.Context, params *protocol.DidSaveTextDocumentParams) error {
	server.lock.Lock()
	current, ok := server.documents[params.TextDocument.URI]
	if !ok {
		server.lock.Unlock()
		return nil
	}

	text, err := savedDocumentText(params.Text, params.TextDocument.URI, current.text)
	if err != nil {
		server.lock.Unlock()
		return err
	}

	server.documents[params.TextDocument.URI] = document{
		text:     text,
		version:  current.version,
		analysis: analyzer.AnalyzeDocumentAtInRoot(text, documentPath(params.TextDocument.URI), server.importRootDir(documentPath(params.TextDocument.URI))),
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
	document, ok := server.documentForPosition(params.TextDocument.URI, params.Position)
	if !ok {
		return []protocol.CompletionItem{}, nil
	}

	return analyzer.CompletionItems(document.text, document.analysis, params.TextDocument.URI, params.Position), nil
}

func (server *Server) hover(context *glsp.Context, params *protocol.HoverParams) (*protocol.Hover, error) {
	document, ok := server.documentForPosition(params.TextDocument.URI, params.Position)
	if !ok {
		return nil, nil
	}

	return analyzer.Hover(document.text, document.analysis, params.Position), nil
}

func (server *Server) definition(context *glsp.Context, params *protocol.DefinitionParams) (any, error) {
	document, ok := server.documentForPosition(params.TextDocument.URI, params.Position)
	if !ok {
		return nil, nil
	}

	location, ok := analyzer.Definition(document.analysis, params.Position)
	if !ok {
		return nil, nil
	}

	return location, nil
}

func (server *Server) prepareRename(context *glsp.Context, params *protocol.PrepareRenameParams) (any, error) {
	document, ok := server.documentForPosition(params.TextDocument.URI, params.Position)
	if !ok {
		return nil, nil
	}

	rangeValue, ok := analyzer.PrepareRename(document.analysis, params.Position)
	if !ok {
		return nil, nil
	}
	return rangeValue, nil
}

func (server *Server) rename(context *glsp.Context, params *protocol.RenameParams) (*protocol.WorkspaceEdit, error) {
	document, ok := server.documentForPosition(params.TextDocument.URI, params.Position)
	if !ok {
		return nil, nil
	}

	edit, ok := analyzer.Rename(document.text, document.analysis, params.TextDocument.URI, params.Position, params.NewName)
	if !ok {
		return nil, nil
	}
	return edit, nil
}

func (server *Server) documentSymbols(context *glsp.Context, params *protocol.DocumentSymbolParams) (any, error) {
	document, ok := server.document(params.TextDocument.URI)
	if !ok {
		return []protocol.DocumentSymbol{}, nil
	}

	return analyzer.DocumentSymbols(document.text, document.analysis), nil
}

func (server *Server) formatDocument(context *glsp.Context, params *protocol.DocumentFormattingParams) ([]protocol.TextEdit, error) {
	document, ok := server.document(params.TextDocument.URI)
	if !ok {
		return []protocol.TextEdit{}, nil
	}

	formatted := analyzer.FormatDocumentText(document.text)
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

func (server *Server) codeActions(context *glsp.Context, params *protocol.CodeActionParams) (any, error) {
	document, ok := server.document(params.TextDocument.URI)
	if !ok {
		return []protocol.CodeAction{}, nil
	}

	return analyzer.CodeActions(document.analysis, params.TextDocument.URI, params.Range), nil
}

func (server *Server) publishDiagnostics(context *glsp.Context, uri protocol.DocumentUri) {
	document, ok := server.document(uri)
	if !ok {
		return
	}

	server.notifyDiagnostics(context, protocol.PublishDiagnosticsParams{
		URI:         uri,
		Version:     analyzer.Ptr(document.version),
		Diagnostics: analyzer.Diagnostics(document.analysis),
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

func (server *Server) documentForPosition(uri protocol.DocumentUri, position protocol.Position) (document, bool) {
	document, ok := server.document(uri)
	if !ok {
		return document, false
	}

	if analyzer.HasParsedFile(document.analysis) {
		return document, true
	}

	document.analysis = analyzer.AnalyzeCompletionContextInRoot(document.text, documentPath(uri), server.importRootDir(documentPath(uri)), position)
	return document, true
}

func documentPath(uri protocol.DocumentUri) string {
	return analyzer.DocumentPath(uri)
}

func workspaceRootDir(params *protocol.InitializeParams) string {
	if params != nil {
		for _, folder := range params.WorkspaceFolders {
			if path := analyzer.DocumentPath(folder.URI); path != "" {
				return path
			}
		}
		if params.RootURI != nil {
			if path := analyzer.DocumentPath(*params.RootURI); path != "" {
				return path
			}
		}
		if params.RootPath != nil && *params.RootPath != "" {
			return *params.RootPath
		}
	}

	importRootDir, err := os.Getwd()
	if err != nil {
		return "."
	}
	return importRootDir
}

func (server *Server) importRootDir(documentPath string) string {
	if server.workspaceRootDir != "" {
		if documentPath == "" {
			return server.workspaceRootDir
		}
		relativePath, err := filepath.Rel(server.workspaceRootDir, documentPath)
		if err == nil && relativePath != ".." && !strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
			return server.workspaceRootDir
		}
		return filepath.Dir(documentPath)
	}
	if documentPath != "" {
		return filepath.Dir(documentPath)
	}
	return "."
}

func fileURI(path string) string {
	path = filepath.ToSlash(path)
	if len(path) >= 2 && path[1] == ':' {
		path = "/" + path
	}

	return (&url.URL{
		Scheme: "file",
		Path:   path,
	}).String()
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

func savedDocumentText(savedText *string, uri protocol.DocumentUri, fallback string) (string, error) {
	if savedText != nil {
		return *savedText, nil
	}

	path := documentPath(uri)
	if path == "" {
		return fallback, nil
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	return string(contents), nil
}

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
