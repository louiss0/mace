package main

import (
	"context"
	"errors"
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
	activeRequests   map[jsonrpc2.ID]*activeRequest
	requestContexts  map[*glsp.Context]*activeRequest
	requestsLock     sync.Mutex
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
		documents:       map[protocol.DocumentUri]document{},
		activeRequests:  map[jsonrpc2.ID]*activeRequest{},
		requestContexts: map[*glsp.Context]*activeRequest{},
	}
	server.handler = protocol.Handler{
		CancelRequest:                  server.cancelRequest,
		Initialize:                     server.initialize,
		Initialized:                    server.initialized,
		Shutdown:                       server.shutdown,
		SetTrace:                       server.setTrace,
		TextDocumentDidOpen:            server.didOpen,
		TextDocumentDidChange:          server.didChange,
		TextDocumentDidSave:            server.didSave,
		TextDocumentDidClose:           server.didClose,
		WorkspaceDidChangeWatchedFiles: server.didChangeWatchedFiles,
		TextDocumentCompletion:         server.complete,
		TextDocumentHover:              server.hover,
		TextDocumentDefinition:         server.definition,
		TextDocumentDocumentSymbol:     server.documentSymbols,
		TextDocumentCodeAction:         server.codeActions,
		TextDocumentRename:             server.rename,
		TextDocumentPrepareRename:      server.prepareRename,
		TextDocumentFormatting:         server.formatDocument,
	}

	return server
}

func (server *Server) Handler() *protocol.Handler {
	return &server.handler
}

func (server *Server) RunStdio() error {
	connectionContext, cancel := context.WithCancel(context.Background())
	defer cancel()
	connection := jsonrpc2.NewConn(
		connectionContext,
		jsonrpc2.NewBufferedStream(stdrwc{}, jsonrpc2.VSCodeObjectCodec{}),
		server.jsonRPCHandler(),
	)

	<-connection.DisconnectNotify()
	return nil
}

func (server *Server) jsonRPCHandler() jsonrpc2.Handler {
	handler := jsonrpc2.HandlerWithError(server.handle).SuppressErrClosed()
	return concurrentRequestHandler{handler: handler}
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
		}

		return aggregate
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

func (server *Server) didChangeWatchedFiles(context *glsp.Context, params *protocol.DidChangeWatchedFilesParams) error {
	if params == nil || len(params.Changes) == 0 {
		return nil
	}

	server.lock.Lock()
	uris := make([]protocol.DocumentUri, 0, len(server.documents))
	for uri, current := range server.documents {
		path := documentPath(uri)
		current.analysis = analyzer.AnalyzeDocumentAtInRoot(
			current.text,
			path,
			server.importRootDir(path),
		)
		server.documents[uri] = current
		uris = append(uris, uri)
	}
	server.lock.Unlock()

	for _, uri := range uris {
		server.publishDiagnostics(context, uri)
	}
	return nil
}

func (server *Server) complete(glspContext *glsp.Context, params *protocol.CompletionParams) (any, error) {
	requestContext := server.requestContext(glspContext)
	document, ok, err := server.documentForPosition(requestContext, params.TextDocument.URI, params.Position)
	if err != nil {
		return nil, err
	}
	if !ok {
		return []protocol.CompletionItem{}, nil
	}

	items := analyzer.CompletionItems(document.text, document.analysis, params.TextDocument.URI, params.Position)
	return items, requestContext.Err()
}

func (server *Server) hover(glspContext *glsp.Context, params *protocol.HoverParams) (*protocol.Hover, error) {
	requestContext := server.requestContext(glspContext)
	document, ok, err := server.documentForPosition(requestContext, params.TextDocument.URI, params.Position)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}

	hover := analyzer.Hover(document.text, document.analysis, params.Position)
	return hover, requestContext.Err()
}

func (server *Server) definition(glspContext *glsp.Context, params *protocol.DefinitionParams) (any, error) {
	requestContext := server.requestContext(glspContext)
	document, ok, err := server.documentForPosition(requestContext, params.TextDocument.URI, params.Position)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}

	location, ok := analyzer.Definition(document.analysis, params.Position)
	if !ok {
		return nil, nil
	}

	return location, requestContext.Err()
}

func (server *Server) prepareRename(glspContext *glsp.Context, params *protocol.PrepareRenameParams) (any, error) {
	requestContext := server.requestContext(glspContext)
	document, ok, err := server.documentForPosition(requestContext, params.TextDocument.URI, params.Position)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}

	rangeValue, ok := analyzer.PrepareRename(document.analysis, params.Position)
	if !ok {
		return nil, nil
	}
	return rangeValue, requestContext.Err()
}

func (server *Server) rename(glspContext *glsp.Context, params *protocol.RenameParams) (*protocol.WorkspaceEdit, error) {
	requestContext := server.requestContext(glspContext)
	document, ok, err := server.documentForPosition(requestContext, params.TextDocument.URI, params.Position)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}

	edit, ok := analyzer.Rename(document.text, document.analysis, params.TextDocument.URI, params.Position, params.NewName)
	if !ok {
		return nil, nil
	}
	return edit, requestContext.Err()
}

func (server *Server) documentSymbols(glspContext *glsp.Context, params *protocol.DocumentSymbolParams) (any, error) {
	requestContext := server.requestContext(glspContext)
	if err := requestContext.Err(); err != nil {
		return nil, err
	}
	document, ok := server.document(params.TextDocument.URI)
	if !ok {
		return []protocol.DocumentSymbol{}, nil
	}

	symbols := analyzer.DocumentSymbols(document.text, document.analysis)
	return symbols, requestContext.Err()
}

func (server *Server) formatDocument(glspContext *glsp.Context, params *protocol.DocumentFormattingParams) ([]protocol.TextEdit, error) {
	requestContext := server.requestContext(glspContext)
	if err := requestContext.Err(); err != nil {
		return nil, err
	}
	document, ok := server.document(params.TextDocument.URI)
	if !ok {
		return []protocol.TextEdit{}, nil
	}

	formatted, err := analyzer.FormatDocument(document.analysis)
	if err != nil {
		return nil, err
	}
	if err := requestContext.Err(); err != nil {
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

func (server *Server) codeActions(glspContext *glsp.Context, params *protocol.CodeActionParams) (any, error) {
	requestContext := server.requestContext(glspContext)
	if err := requestContext.Err(); err != nil {
		return nil, err
	}
	document, ok := server.document(params.TextDocument.URI)
	if !ok {
		return []protocol.CodeAction{}, nil
	}

	actions := analyzer.CodeActions(document.analysis, params.TextDocument.URI, params.Range)
	return actions, requestContext.Err()
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

func (server *Server) documentForPosition(
	requestContext context.Context,
	uri protocol.DocumentUri,
	position protocol.Position,
) (document, bool, error) {
	if err := requestContext.Err(); err != nil {
		return document{}, false, err
	}
	document, ok := server.document(uri)
	if !ok {
		return document, false, nil
	}

	if analyzer.HasParsedFile(document.analysis) {
		return document, true, requestContext.Err()
	}

	analysis, err := analyzer.AnalyzeCompletionContextInRootContext(
		requestContext,
		document.text,
		documentPath(uri),
		server.importRootDir(documentPath(uri)),
		position,
	)
	if err != nil {
		return document, false, err
	}
	document.analysis = analysis
	return document, true, nil
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

	return cliActivationDir
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
	requestContext context.Context,
	connection *jsonrpc2.Conn,
	request *jsonrpc2.Request,
) (any, error) {
	glspContext := glsp.Context{
		Method: request.Method,
		Notify: func(method string, params any) {
			_ = connection.Notify(requestContext, method, params)
		},
	}
	if !request.Notif {
		var finishRequest func()
		requestContext, finishRequest = server.registerRequest(requestContext, request.ID, &glspContext)
		defer finishRequest()
	}

	if request.Params != nil {
		glspContext.Params = *request.Params
	}

	switch request.Method {
	case protocol.MethodExit:
		_, _, _, _ = server.handler.Handle(&glspContext)
		return nil, connection.Close()
	case protocol.MethodCancelRequest:
		// GLSP v0.2.2 loses IntegerOrString values while unmarshalling.
		if request.Params == nil {
			return nil, &jsonrpc2.Error{Code: jsonrpc2.CodeInvalidParams}
		}
		id, ok := cancellationRequestIDFromJSON(*request.Params)
		if !ok {
			return nil, &jsonrpc2.Error{Code: jsonrpc2.CodeInvalidParams}
		}
		server.cancelRequestID(id)
		return nil, nil
	default:
		result, validMethod, validParams, err := server.handler.Handle(&glspContext)
		if errors.Is(requestContext.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
			return nil, requestCancellationError()
		}
		if !validMethod {
			return nil, &jsonrpc2.Error{
				Code:    jsonrpc2.CodeMethodNotFound,
				Message: fmt.Sprintf("method not supported: %s", request.Method),
			}
		}
		if !validParams {
			message := ""
			if err != nil {
				message = err.Error()
			}

			return nil, &jsonrpc2.Error{Code: jsonrpc2.CodeInvalidParams, Message: message}
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
