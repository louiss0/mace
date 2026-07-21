package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	"github.com/samber/lo"
	"github.com/sourcegraph/jsonrpc2"
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

type capturedNotification struct {
	method string
	params any
}

func newTestLSPServer() *Server {
	return newLSPServer()
}

func New() *Server {
	return newTestLSPServer()
}

func invoke(handler *protocol.Handler, method string, params any, notifications *[]capturedNotification) (any, bool, bool, error) {
	payload := []byte("{}")
	if params != nil {
		encoded, err := json.Marshal(params)
		tAssert.NoError(err)
		payload = encoded
	}

	context := &glsp.Context{
		Method: method,
		Params: payload,
		Notify: func(method string, params any) {
			if notifications == nil {
				return
			}

			*notifications = append(*notifications, capturedNotification{
				method: method,
				params: params,
			})
		},
	}

	return handler.Handle(context)
}

func initializeServer(server *Server) {
	_, validMethod, validParams, err := invoke(server.Handler(), protocol.MethodInitialize, protocol.InitializeParams{}, nil)
	tAssert.True(validMethod)
	tAssert.True(validParams)
	tAssert.NoError(err)
}

func didOpen(server *Server, uri protocol.DocumentUri, text string, notifications *[]capturedNotification) {
	_, validMethod, validParams, err := invoke(server.Handler(), protocol.MethodTextDocumentDidOpen, protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:        uri,
			LanguageID: "mace",
			Version:    1,
			Text:       text,
		},
	}, notifications)
	tAssert.True(validMethod)
	tAssert.True(validParams)
	tAssert.NoError(err)
}

func openEmptyDocument(server *Server, uri protocol.DocumentUri, notifications *[]capturedNotification) {
	didOpen(server, uri, "", notifications)
}

func cancelID(id jsonrpc2.ID) protocol.IntegerOrString {
	if id.IsString {
		return protocol.IntegerOrString{Value: id.Str}
	}

	return protocol.IntegerOrString{Value: protocol.Integer(id.Num)}
}

func didChange(server *Server, uri protocol.DocumentUri, version int32, text string, notifications *[]capturedNotification) {
	_, validMethod, validParams, err := invoke(server.Handler(), protocol.MethodTextDocumentDidChange, protocol.DidChangeTextDocumentParams{
		TextDocument: protocol.VersionedTextDocumentIdentifier{
			Version: version,
			TextDocumentIdentifier: protocol.TextDocumentIdentifier{
				URI: uri,
			},
		},
		ContentChanges: []any{
			protocol.TextDocumentContentChangeEvent{
				Text: text,
			},
		},
	}, notifications)
	tAssert.True(validMethod)
	tAssert.True(validParams)
	tAssert.NoError(err)
}

func didSave(server *Server, uri protocol.DocumentUri, text *string, notifications *[]capturedNotification) {
	params := protocol.DidSaveTextDocumentParams{
		TextDocument: protocol.TextDocumentIdentifier{
			URI: uri,
		},
	}
	if text != nil {
		params.Text = text
	}

	_, validMethod, validParams, err := invoke(server.Handler(), protocol.MethodTextDocumentDidSave, params, notifications)
	tAssert.True(validMethod)
	tAssert.True(validParams)
	tAssert.NoError(err)
}

func didClose(server *Server, uri protocol.DocumentUri, notifications *[]capturedNotification) {
	_, validMethod, validParams, err := invoke(server.Handler(), protocol.MethodTextDocumentDidClose, protocol.DidCloseTextDocumentParams{
		TextDocument: protocol.TextDocumentIdentifier{
			URI: uri,
		},
	}, notifications)
	tAssert.True(validMethod)
	tAssert.True(validParams)
	tAssert.NoError(err)
}

func completeLabels(server *Server, uri protocol.DocumentUri, line uint32, character uint32) []string {
	resultValue, validMethod, validParams, err := invoke(server.Handler(), protocol.MethodTextDocumentCompletion, protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position: protocol.Position{
				Line:      protocol.UInteger(line),
				Character: protocol.UInteger(character),
			},
		},
	}, nil)
	return requireCompletionLabels(resultValue, validMethod, validParams, err)
}

func requireCompletionLabels(resultValue any, validMethod bool, validParams bool, err error) []string {
	tAssert.True(validMethod)
	tAssert.True(validParams)
	tAssert.NoError(err)

	items, ok := resultValue.([]protocol.CompletionItem)
	tAssert.True(ok)
	if !ok {
		return nil
	}

	labels := make([]string, 0, len(items))
	for _, item := range items {
		labels = append(labels, item.Label)
	}

	return labels
}

func writeWorkspaceFile(root string, relativePath string, contents string) string {
	path := filepath.Join(root, relativePath)
	err := os.MkdirAll(filepath.Dir(path), 0o755)
	tAssert.NoError(err)
	err = os.WriteFile(path, []byte(contents), 0o600)
	tAssert.NoError(err)

	return testFileURI(path)
}

func testFileURI(path string) string {
	return fileURI(path)
}

func requireDiagnostics(notification capturedNotification) protocol.PublishDiagnosticsParams {
	tAssert.Equal(protocol.ServerTextDocumentPublishDiagnostics, notification.method)

	params, ok := notification.params.(protocol.PublishDiagnosticsParams)
	tAssert.True(ok)
	if !ok {
		return protocol.PublishDiagnosticsParams{}
	}

	return params
}

func nestedSelfDocument(depth int) string {
	keys := make([]string, 0, depth)
	for index := range depth {
		keys = append(keys, fmt.Sprintf("level%d", index+1))
	}

	leaf := `{ final: 9, }`
	for index := len(keys) - 1; index >= 0; index-- {
		leaf = fmt.Sprintf("{ %s: %s, }", keys[index], leaf)
	}

	return fmt.Sprintf(`[output = 'data']
{
  tree: %s,
  result: $self.tree.%s.
}`, leaf, strings.Join(keys, "."))
}

var _ = Describe("LSP server", func() {
	It("covers stdrwc file methods", func() {
		stdinFile, err := os.CreateTemp("", "mace-stdin-*")
		tAssert.NoError(err)
		_, err = stdinFile.WriteString("x")
		tAssert.NoError(err)
		_, err = stdinFile.Seek(0, 0)
		tAssert.NoError(err)

		stdoutFile, err := os.CreateTemp("", "mace-stdout-*")
		tAssert.NoError(err)

		previousStdin := os.Stdin
		previousStdout := os.Stdout
		defer func() {
			os.Stdin = previousStdin
			os.Stdout = previousStdout
		}()

		os.Stdin = stdinFile
		os.Stdout = stdoutFile

		buffer := make([]byte, 1)
		_, _ = stdrwc{}.Read(buffer)
		_, _ = stdrwc{}.Write([]byte("x"))
		tAssert.NoError(stdrwc{}.Close())
	})

	const uri = "file:///workspace/test.mace"

	var server *Server
	var uninitializedServer *Server

	BeforeEach(func() {
		uninitializedServer = New()
		server = New()
		initializeServer(server)
	})

	AfterEach(func() {
		protocol.SetTraceValue(protocol.TraceValueOff)
		server = nil
		uninitializedServer = nil
	})

	It("advertises core capabilities during initialize", func() {
		resultValue, validMethod, validParams, err := invoke(uninitializedServer.Handler(), protocol.MethodInitialize, protocol.InitializeParams{}, nil)
		tAssert.True(validMethod)
		tAssert.True(validParams)
		tAssert.NoError(err)

		result, ok := resultValue.(protocol.InitializeResult)
		tAssert.True(ok)
		if !ok {
			return
		}

		tAssert.Equal(serverName, result.ServerInfo.Name)
		tAssert.Equal(serverVersion, *result.ServerInfo.Version)

		syncOptions, ok := result.Capabilities.TextDocumentSync.(*protocol.TextDocumentSyncOptions)
		tAssert.True(ok)
		if !ok {
			return
		}

		tAssert.True(*syncOptions.OpenClose)
		tAssert.Equal(protocol.TextDocumentSyncKindFull, *syncOptions.Change)
		saveOptions, ok := syncOptions.Save.(*protocol.SaveOptions)
		tAssert.True(ok)
		if ok {
			tAssert.NotNil(saveOptions.IncludeText)
			tAssert.True(*saveOptions.IncludeText)
		}
		tAssert.NotNil(result.Capabilities.CompletionProvider)
		if result.Capabilities.CompletionProvider != nil {
			tAssert.Equal([]string{".", ":", "=", "$"}, result.Capabilities.CompletionProvider.TriggerCharacters)
		}
		tAssert.Equal(true, result.Capabilities.HoverProvider)
		tAssert.Equal(true, result.Capabilities.DefinitionProvider)
		tAssert.Equal(true, result.Capabilities.DocumentSymbolProvider)
		tAssert.Equal(true, result.Capabilities.CodeActionProvider)
		tAssert.Equal(true, result.Capabilities.RenameProvider)
		tAssert.Equal(true, result.Capabilities.DocumentFormattingProvider)
	})

	It("rejects requests before initialize", func() {
		_, validMethod, validParams, err := invoke(uninitializedServer.Handler(), protocol.MethodTextDocumentHover, protocol.HoverParams{}, nil)
		tAssert.True(validMethod)
		tAssert.True(validParams)
		tAssert.ErrorContains(err, "server not initialized")
	})

	It("accepts the initialized notification", func() {

		_, validMethod, validParams, err := invoke(server.Handler(), protocol.MethodInitialized, protocol.InitializedParams{}, nil)
		tAssert.True(validMethod)
		tAssert.True(validParams)
		tAssert.NoError(err)
	})

	It("accepts request cancellation notifications", func() {
		_, validMethod, validParams, err := invoke(server.Handler(), protocol.MethodCancelRequest, protocol.CancelParams{
			ID: protocol.IntegerOrString{Value: protocol.Integer(42)},
		}, nil)

		tAssert.True(validMethod)
		tAssert.True(validParams)
		tAssert.NoError(err)
	})

	It("cancels running requests with integer and string ids", func() {
		negativeID := protocol.Integer(-42)
		for _, id := range []jsonrpc2.ID{
			{Num: 42},
			{Num: uint64(negativeID)},
			{Str: "completion-42", IsString: true},
		} {
			started := make(chan struct{})
			server.handler.TextDocumentHover = func(glspContext *glsp.Context, params *protocol.HoverParams) (*protocol.Hover, error) {
				requestContext := server.requestContext(glspContext)
				close(started)
				<-requestContext.Done()
				return nil, requestContext.Err()
			}

			requestParams := json.RawMessage(`{}`)
			result := make(chan error, 1)
			go func(requestID jsonrpc2.ID) {
				_, err := server.handle(context.Background(), nil, &jsonrpc2.Request{
					Method: protocol.MethodTextDocumentHover,
					Params: &requestParams,
					ID:     requestID,
				})
				result <- err
			}(id)

			<-started
			cancelParams, err := json.Marshal(protocol.CancelParams{ID: cancelID(id)})
			tAssert.NoError(err)
			cancelPayload := json.RawMessage(cancelParams)
			_, err = server.handle(context.Background(), nil, &jsonrpc2.Request{
				Method: protocol.MethodCancelRequest,
				Params: &cancelPayload,
				Notif:  true,
			})
			tAssert.NoError(err)

			var rpcError *jsonrpc2.Error
			tAssert.ErrorAs(<-result, &rpcError)
			if rpcError != nil {
				tAssert.Equal(int64(-32800), rpcError.Code)
			}
			tAssert.Zero(server.activeRequestCount())
		}
	})

	It("ignores cancellation for unknown and completed requests", func() {
		cancelPayload := json.RawMessage(`{"id":404}`)
		_, err := server.handle(context.Background(), nil, &jsonrpc2.Request{
			Method: protocol.MethodCancelRequest,
			Params: &cancelPayload,
			Notif:  true,
		})
		tAssert.NoError(err)

		requestParams := json.RawMessage(`{}`)
		_, err = server.handle(context.Background(), nil, &jsonrpc2.Request{
			Method: protocol.MethodTextDocumentHover,
			Params: &requestParams,
			ID:     jsonrpc2.ID{Num: 404},
		})
		tAssert.NoError(err)
		tAssert.Zero(server.activeRequestCount())

		_, err = server.handle(context.Background(), nil, &jsonrpc2.Request{
			Method: protocol.MethodCancelRequest,
			Params: &cancelPayload,
			Notif:  true,
		})
		tAssert.NoError(err)
		tAssert.Zero(server.activeRequestCount())
	})

	It("removes active requests after handler failures", func() {
		server.handler.TextDocumentHover = func(context *glsp.Context, params *protocol.HoverParams) (*protocol.Hover, error) {
			return nil, fmt.Errorf("hover failed")
		}
		requestParams := json.RawMessage(`{}`)

		_, err := server.handle(context.Background(), nil, &jsonrpc2.Request{
			Method: protocol.MethodTextDocumentHover,
			Params: &requestParams,
			ID:     jsonrpc2.ID{Num: 1},
		})

		tAssert.Error(err)
		tAssert.Zero(server.activeRequestCount())
	})

	It("cancels concurrent requests independently", func() {
		started := make(chan jsonrpc2.ID, 2)
		release := make(chan struct{})
		server.handler.TextDocumentHover = func(glspContext *glsp.Context, params *protocol.HoverParams) (*protocol.Hover, error) {
			requestContext := server.requestContext(glspContext)
			requestID, ok := server.requestID(glspContext)
			tAssert.True(ok)
			started <- requestID
			select {
			case <-requestContext.Done():
				return nil, requestContext.Err()
			case <-release:
				return &protocol.Hover{}, nil
			}
		}

		requestParams := json.RawMessage(`{}`)
		results := make(chan error, 2)
		for _, id := range []jsonrpc2.ID{{Num: 1}, {Num: 2}} {
			go func(requestID jsonrpc2.ID) {
				_, err := server.handle(context.Background(), nil, &jsonrpc2.Request{
					Method: protocol.MethodTextDocumentHover,
					Params: &requestParams,
					ID:     requestID,
				})
				results <- err
			}(id)
		}
		<-started
		<-started

		cancelPayload := json.RawMessage(`{"id":1}`)
		_, err := server.handle(context.Background(), nil, &jsonrpc2.Request{
			Method: protocol.MethodCancelRequest,
			Params: &cancelPayload,
			Notif:  true,
		})
		tAssert.NoError(err)
		close(release)

		errors := []error{<-results, <-results}
		cancelled := 0
		succeeded := 0
		for _, resultErr := range errors {
			if resultErr == nil {
				succeeded++
				continue
			}
			var rpcError *jsonrpc2.Error
			if tAssert.ErrorAs(resultErr, &rpcError) && rpcError.Code == -32800 {
				cancelled++
			}
		}
		tAssert.Equal(1, cancelled)
		tAssert.Equal(1, succeeded)
		tAssert.Zero(server.activeRequestCount())
	})

	It("allows request completion to race safely with cancellation", func() {
		for iteration := 0; iteration < 50; iteration++ {
			release := make(chan struct{})
			started := make(chan struct{})
			server.handler.TextDocumentHover = func(glspContext *glsp.Context, params *protocol.HoverParams) (*protocol.Hover, error) {
				requestContext := server.requestContext(glspContext)
				close(started)
				select {
				case <-requestContext.Done():
					return nil, requestContext.Err()
				case <-release:
					return &protocol.Hover{}, nil
				}
			}

			requestParams := json.RawMessage(`{}`)
			result := make(chan error, 1)
			go func() {
				_, err := server.handle(context.Background(), nil, &jsonrpc2.Request{
					Method: protocol.MethodTextDocumentHover,
					Params: &requestParams,
					ID:     jsonrpc2.ID{Num: 7},
				})
				result <- err
			}()
			<-started

			cancelPayload := json.RawMessage(`{"id":7}`)
			cancelDone := make(chan struct{})
			go func() {
				_, _ = server.handle(context.Background(), nil, &jsonrpc2.Request{
					Method: protocol.MethodCancelRequest,
					Params: &cancelPayload,
					Notif:  true,
				})
				close(cancelDone)
			}()
			close(release)

			err := <-result
			<-cancelDone
			if err != nil {
				var rpcError *jsonrpc2.Error
				tAssert.ErrorAs(err, &rpcError)
				if rpcError != nil {
					tAssert.Equal(requestCancelledCode, rpcError.Code)
				}
			}
			tAssert.Zero(server.activeRequestCount())
		}
	})

	It("does not respond to cancellation notifications", func() {
		serverSide, clientSide := net.Pipe()
		connection := jsonrpc2.NewConn(
			context.Background(),
			jsonrpc2.NewBufferedStream(serverSide, jsonrpc2.VSCodeObjectCodec{}),
			server.jsonRPCHandler(),
		)
		defer func() { _ = connection.Close() }()
		defer func() { _ = clientSide.Close() }()

		payload := `{"jsonrpc":"2.0","method":"$/cancelRequest","params":{"id":"unknown"}}`
		_, err := fmt.Fprintf(clientSide, "Content-Length: %d\r\n\r\n%s", len(payload), payload)
		tAssert.NoError(err)
		tAssert.NoError(clientSide.SetReadDeadline(time.Now().Add(200 * time.Millisecond)))

		buffer := make([]byte, 1)
		_, err = clientSide.Read(buffer)
		var networkError net.Error
		tAssert.ErrorAs(err, &networkError)
		if networkError != nil {
			tAssert.True(networkError.Timeout())
		}
	})

	It("reanalyzes open documents when watched Mace files change", func() {
		didOpen(server, uri, `[output = 'data'] { value: 1 }`, nil)
		notifications := []capturedNotification{}

		_, validMethod, validParams, err := invoke(
			server.Handler(),
			protocol.MethodWorkspaceDidChangeWatchedFiles,
			protocol.DidChangeWatchedFilesParams{
				Changes: []protocol.FileEvent{{URI: uri, Type: protocol.FileChangeTypeChanged}},
			},
			&notifications,
		)
		tAssert.True(validMethod)
		tAssert.True(validParams)
		tAssert.NoError(err)
		if tAssert.Len(notifications, 1) {
			tAssert.Equal(protocol.ServerTextDocumentPublishDiagnostics, notifications[0].method)
		}
	})

	It("updates the trace level", func() {

		_, validMethod, validParams, err := invoke(server.Handler(), protocol.MethodSetTrace, protocol.SetTraceParams{
			Value: protocol.TraceValueVerbose,
		}, nil)
		tAssert.True(validMethod)
		tAssert.True(validParams)
		tAssert.NoError(err)
		tAssert.Equal(protocol.TraceValueVerbose, protocol.GetTraceValue())
	})

	It("shuts down and rejects later requests", func() {

		_, validMethod, validParams, err := invoke(server.Handler(), protocol.MethodShutdown, nil, nil)
		tAssert.True(validMethod)
		tAssert.True(validParams)
		tAssert.NoError(err)

		_, validMethod, validParams, err = invoke(server.Handler(), protocol.MethodTextDocumentHover, protocol.HoverParams{}, nil)
		tAssert.True(validMethod)
		tAssert.True(validParams)
		tAssert.ErrorContains(err, "server not initialized")
	})

	It("publishes empty diagnostics when a valid document opens", func() {
		notifications := []capturedNotification{}

		didOpen(server, uri, `[output = 'data'] { result: 1 + 2, }`, &notifications)

		if tAssert.Len(notifications, 1) {
			params := requireDiagnostics(notifications[0])
			tAssert.Equal(uri, params.URI)
			tAssert.Empty(params.Diagnostics)
		}
	})

	It("resolves script imports relative to the opened file", func() {
		notifications := []capturedNotification{}

		workspace, err := os.MkdirTemp("", "mace-lsp-import-diagnostics-*")
		tAssert.NoError(err)

		writeWorkspaceFile(workspace, "shared.mace", `[output = 'schema']
{
  Name: string,
}`)
		uri := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", `|===|
from './shared.mace' import Name;
Name user = "Ada";
|===|
[output = 'data']
{
  user: user,
}`))

		didOpen(server, uri, `|===|
from './shared.mace' import Name;
Name user = "Ada";
|===|
[output = 'data']
{
  user: user,
}`, &notifications)

		if tAssert.Len(notifications, 1) {
			params := requireDiagnostics(notifications[0])
			tAssert.Empty(params.Diagnostics)
		}
	})

	It("publishes syntax diagnostics when an invalid document opens", func() {
		notifications := []capturedNotification{}

		didOpen(server, uri, `[output = 'data'] { result: , }`, &notifications)

		if tAssert.Len(notifications, 1) {
			params := requireDiagnostics(notifications[0])
			tAssert.Len(params.Diagnostics, 1)
			tAssert.Contains(params.Diagnostics[0].Message, "parser:")
			tAssert.Equal(protocol.DiagnosticSeverityError, *params.Diagnostics[0].Severity)
			tAssert.NotNil(params.Diagnostics[0].Code)
		}
	})

	It("publishes invalid output optionality errors at the field name", func() {
		notifications := []capturedNotification{}

		didOpen(server, uri, `|===|
schema User: { name: string, };
|===|
[output = 'data', schema = User]
{ name?: 'Ada', }`, &notifications)

		if tAssert.Len(notifications, 1) {
			params := requireDiagnostics(notifications[0])
			if tAssert.Len(params.Diagnostics, 1) {
				tAssert.Contains(params.Diagnostics[0].Message, "field \"name\" is not optional")
				tAssert.Equal("mace.type.data-field-optional-marker", params.Diagnostics[0].Code.Value)
				tAssert.Equal(protocol.Range{
					Start: protocol.Position{Line: 4, Character: 2},
					End:   protocol.Position{Line: 4, Character: 6},
				}, params.Diagnostics[0].Range)
			}
		}
	})

	It("publishes optional field access errors at the access operator", func() {
		notifications := []capturedNotification{}

		didOpen(server, uri, `|===|
schema Address: {
  city: string,
};
schema User: {
  address?: Address,
};
User user = { address: { city: 'Paris', }, };
string city = user.address.city;
|===|
[output = 'data'] { city: city, }`, &notifications)

		if tAssert.Len(notifications, 1) {
			params := requireDiagnostics(notifications[0])
			if tAssert.Len(params.Diagnostics, 1) {
				tAssert.Contains(params.Diagnostics[0].Message, "use optional chaining '?.'")
				tAssert.Equal("mace.type.optional-field-access", params.Diagnostics[0].Code.Value)
				tAssert.Equal(protocol.Range{
					Start: protocol.Position{Line: 8, Character: 26},
					End:   protocol.Position{Line: 8, Character: 27},
				}, params.Diagnostics[0].Range)
			}
		}
	})

	It("publishes invalid null usage errors at the null literal", func() {
		notifications := []capturedNotification{}

		didOpen(server, uri, `|===|
string value = null;
|===|
[output = 'data'] { value: value, }`, &notifications)

		if tAssert.Len(notifications, 1) {
			params := requireDiagnostics(notifications[0])
			if tAssert.Len(params.Diagnostics, 1) {
				tAssert.Contains(params.Diagnostics[0].Message, "null is only allowed in output")
				tAssert.Equal("mace.type.invalid-null-usage", params.Diagnostics[0].Code.Value)
				tAssert.Equal(protocol.Range{
					Start: protocol.Position{Line: 1, Character: 15},
					End:   protocol.Position{Line: 1, Character: 19},
				}, params.Diagnostics[0].Range)
			}
		}
	})

	It("publishes missing output field separators at the preceding field", func() {
		notifications := []capturedNotification{}

		didOpen(server, uri, `[output = 'data']
{
  first: 1
  second: 2,
}`, &notifications)

		if tAssert.Len(notifications, 1) {
			params := requireDiagnostics(notifications[0])
			if tAssert.Len(params.Diagnostics, 1) {
				tAssert.Contains(params.Diagnostics[0].Message, "expected ',' after output field")
				tAssert.Equal(protocol.Range{
					Start: protocol.Position{Line: 2, Character: 2},
					End:   protocol.Position{Line: 2, Character: 7},
				}, params.Diagnostics[0].Range)
			}
		}
	})

	It("publishes mixed numeric family errors at the operator", func() {
		notifications := []capturedNotification{}

		didOpen(server, uri, `|===|
hex_int value = 0x2 + 3;
|===|
[output = 'data'] { value, }`, &notifications)

		if tAssert.Len(notifications, 1) {
			params := requireDiagnostics(notifications[0])
			if tAssert.Len(params.Diagnostics, 1) {
				tAssert.Contains(params.Diagnostics[0].Message, "expected hexadecimal operands for operator")
				tAssert.Equal("mace.type.mixed-numeric-family", params.Diagnostics[0].Code.Value)
				tAssert.Equal(protocol.Range{
					Start: protocol.Position{Line: 1, Character: 20},
					End:   protocol.Position{Line: 1, Character: 21},
				}, params.Diagnostics[0].Range)
			}
		}
	})

	It("publishes variant literal pattern errors at the invalid arm pattern", func() {
		notifications := []capturedNotification{}

		didOpen(server, uri, `|===|
variant[string, int] value = 1;
string result = match (value) {
  'text' => 'text',
  int => 'number',
};
|===|
[output = 'data'] { result: result, }`, &notifications)

		if tAssert.Len(notifications, 1) {
			params := requireDiagnostics(notifications[0])
			if tAssert.Len(params.Diagnostics, 1) {
				tAssert.Contains(params.Diagnostics[0].Message, "variant match arms require a type pattern")
				tAssert.Equal("mace.match.variant-literal-pattern", params.Diagnostics[0].Code.Value)
				tAssert.Equal(protocol.Range{
					Start: protocol.Position{Line: 3, Character: 2},
					End:   protocol.Position{Line: 3, Character: 8},
				}, params.Diagnostics[0].Range)
			}
		}
	})

	It("publishes out-of-domain match errors at the invalid arm pattern", func() {
		notifications := []capturedNotification{}

		didOpen(server, uri, `|===|
variant[string, int] value = 1;
string result = match (value) {
  string => 'text',
  boolean => 'flag',
  int => 'number',
};
|===|
[output = 'data'] { result: result, }`, &notifications)

		if tAssert.Len(notifications, 1) {
			params := requireDiagnostics(notifications[0])
			if tAssert.Len(params.Diagnostics, 1) {
				tAssert.Contains(params.Diagnostics[0].Message, "match pattern boolean is not a member")
				tAssert.Equal("mace.match.pattern-outside-domain", params.Diagnostics[0].Code.Value)
				tAssert.Equal(protocol.Range{
					Start: protocol.Position{Line: 4, Character: 2},
					End:   protocol.Position{Line: 4, Character: 9},
				}, params.Diagnostics[0].Range)
			}
		}
	})

	It("publishes non-exhaustive match errors at the match expression", func() {
		notifications := []capturedNotification{}

		didOpen(server, uri, `|===|
variant[string, int] value = 1;
string result = match (value) {
  string => 'text',
};
|===|
[output = 'data'] { result: result, }`, &notifications)

		if tAssert.Len(notifications, 1) {
			params := requireDiagnostics(notifications[0])
			if tAssert.Len(params.Diagnostics, 1) {
				tAssert.Contains(params.Diagnostics[0].Message, "match expression must be exhaustive")
				tAssert.Equal("mace.match.not-exhaustive", params.Diagnostics[0].Code.Value)
				tAssert.Equal(protocol.Range{
					Start: protocol.Position{Line: 2, Character: 16},
					End:   protocol.Position{Line: 4, Character: 1},
				}, params.Diagnostics[0].Range)
			}
		}
	})

	It("publishes duplicate match errors at the repeated pattern", func() {
		notifications := []capturedNotification{}

		didOpen(server, uri, `|===|
variant[string, int] value = 1;
string result = match (value) {
  string => 'text',
  string => 'again',
};
|===|
[output = 'data'] { result: result, }`, &notifications)

		if tAssert.Len(notifications, 1) {
			params := requireDiagnostics(notifications[0])
			if tAssert.Len(params.Diagnostics, 1) {
				tAssert.Contains(params.Diagnostics[0].Message, "duplicate match pattern string")
				tAssert.Equal("mace.match.duplicate-pattern", params.Diagnostics[0].Code.Value)
				tAssert.Equal(protocol.Range{
					Start: protocol.Position{Line: 4, Character: 2},
					End:   protocol.Position{Line: 4, Character: 8},
				}, params.Diagnostics[0].Range)
			}
		}
	})

	It("publishes concrete match input errors at the input expression", func() {
		notifications := []capturedNotification{}

		didOpen(server, uri, `|===|
string value = 'text'; string result = match (value) { string => 'text', int => 'number', };
|===|
[output = 'data'] { result: result, }`, &notifications)

		if tAssert.Len(notifications, 1) {
			params := requireDiagnostics(notifications[0])
			if tAssert.Len(params.Diagnostics, 1) {
				tAssert.Contains(params.Diagnostics[0].Message, "match input must be a variant or choice")
				tAssert.Equal("mace.match.concrete-input", params.Diagnostics[0].Code.Value)
				tAssert.Equal(protocol.Range{
					Start: protocol.Position{Line: 1, Character: 46},
					End:   protocol.Position{Line: 1, Character: 51},
				}, params.Diagnostics[0].Range)
			}
		}
	})

	It("publishes choice type pattern errors at the invalid pattern", func() {
		notifications := []capturedNotification{}

		didOpen(server, uri, `|===|
choice['on', 'off'] value = 'on';
int selected = match (value) {
  string => 1,
  'off' => 0,
};
|===|
[output = 'data'] { result: selected, }`, &notifications)

		if tAssert.Len(notifications, 1) {
			params := requireDiagnostics(notifications[0])
			if tAssert.Len(params.Diagnostics, 1) {
				tAssert.Contains(params.Diagnostics[0].Message, "choice match arms require a literal pattern")
				tAssert.Equal("mace.match.choice-type-pattern", params.Diagnostics[0].Code.Value)
				tAssert.Equal(protocol.Range{
					Start: protocol.Position{Line: 3, Character: 2},
					End:   protocol.Position{Line: 3, Character: 8},
				}, params.Diagnostics[0].Range)
			}
		}
	})

	It("publishes mismatched script delimiters at the closing delimiter", func() {
		notifications := []capturedNotification{}

		didOpen(server, uri, `|===|
string name = "Ada";
|====|
[output = 'data'] { result: name, }`, &notifications)

		if tAssert.Len(notifications, 1) {
			params := requireDiagnostics(notifications[0])
			if tAssert.Len(params.Diagnostics, 1) {
				tAssert.Equal("mace.syntax.inconsistent-script-delimiters", params.Diagnostics[0].Code.Value)
				tAssert.Contains(params.Diagnostics[0].Message, `at 3:1 near "|====|"`)
				tAssert.Equal(protocol.Range{
					Start: protocol.Position{Line: 2, Character: 0},
					End:   protocol.Position{Line: 2, Character: 6},
				}, params.Diagnostics[0].Range)
			}
		}
	})

	It("publishes duplicate schema fields at the repeated name", func() {
		notifications := []capturedNotification{}

		didOpen(server, uri, `|===|
schema User: { name: string, name: string, };
|===|
[output = 'schema'] {}`, &notifications)

		if tAssert.Len(notifications, 1) {
			params := requireDiagnostics(notifications[0])
			if tAssert.Len(params.Diagnostics, 1) {
				tAssert.Equal("mace.declaration.duplicate-schema-field", params.Diagnostics[0].Code.Value)
				tAssert.Equal(protocol.Range{
					Start: protocol.Position{Line: 1, Character: 29},
					End:   protocol.Position{Line: 1, Character: 33},
				}, params.Diagnostics[0].Range)
			}
		}
	})

	It("publishes numeric operand errors at the operator", func() {
		notifications := []capturedNotification{}

		didOpen(server, uri, `|===|
boolean value = true + false;
|===|
[output = 'data'] { result: value, }`, &notifications)

		if tAssert.Len(notifications, 1) {
			params := requireDiagnostics(notifications[0])
			if tAssert.Len(params.Diagnostics, 1) {
				tAssert.Contains(params.Diagnostics[0].Message, "expected numeric operands for operator")
				tAssert.Equal("mace.type.invalid-binary-operator", params.Diagnostics[0].Code.Value)
				tAssert.Equal(protocol.Range{
					Start: protocol.Position{Line: 1, Character: 21},
					End:   protocol.Position{Line: 1, Character: 22},
				}, params.Diagnostics[0].Range)
			}
		}
	})

	It("publishes scalar interpolation errors at the interpolation", func() {
		notifications := []capturedNotification{}

		didOpen(server, uri, `|===|
schema User: { name: string, };
User user = { name: "Ada", };
string message = "Hello $(user)!";
|===|
[output = 'data'] { result: message, }`, &notifications)

		if tAssert.Len(notifications, 1) {
			params := requireDiagnostics(notifications[0])
			if tAssert.Len(params.Diagnostics, 1) {
				tAssert.Contains(params.Diagnostics[0].Message, "interpolation requires a scalar value")
				tAssert.Equal("mace.string.nonscalar-interpolation", params.Diagnostics[0].Code.Value)
				tAssert.Equal(protocol.Range{
					Start: protocol.Position{Line: 3, Character: 24},
					End:   protocol.Position{Line: 3, Character: 31},
				}, params.Diagnostics[0].Range)
			}
		}
	})

	It("publishes processor diagnostics for invalid variable declarations", func() {
		notifications := []capturedNotification{}

		didOpen(server, uri, `|===|
int count = "Ada";
|===|
[output = 'data']
{
  result: count,
}`, &notifications)

		if tAssert.Len(notifications, 1) {
			params := requireDiagnostics(notifications[0])
			tAssert.Len(params.Diagnostics, 1)
			tAssert.Contains(params.Diagnostics[0].Message, `processor: type mismatch: expected int, got string`)
			tAssert.Equal(protocol.UInteger(1), params.Diagnostics[0].Range.Start.Line)
			tAssert.Equal(protocol.UInteger(4), params.Diagnostics[0].Range.Start.Character)
			tAssert.NotNil(params.Diagnostics[0].Code)
		}
	})

	It("publishes processor diagnostics for invalid variant assignments", func() {
		notifications := []capturedNotification{}

		didOpen(server, uri, `|===|
schema EmailLogin: { email: string, };
schema ApiKeyLogin: { api_key: string, };
alias Login: variant[EmailLogin, ApiKeyLogin];
Login login = {
  email: "ada@example.com",
  api_key: "secret",
};
|===|
[output = 'data']
{
  result: login,
}`, &notifications)

		if tAssert.Len(notifications, 1) {
			params := requireDiagnostics(notifications[0])
			tAssert.Len(params.Diagnostics, 1)
			tAssert.Contains(params.Diagnostics[0].Message, `processor: type mismatch: expected variant[EmailLogin, ApiKeyLogin], got record`)
			tAssert.NotNil(params.Diagnostics[0].Code)
		}
	})

	It("publishes processor diagnostics for invalid fusion declarations", func() {
		notifications := []capturedNotification{}

		didOpen(server, uri, `|===|
alias Broken: fusion[string, int];
|===|
[output = 'data'] {}`, &notifications)

		if tAssert.Len(notifications, 1) {
			params := requireDiagnostics(notifications[0])
			tAssert.Len(params.Diagnostics, 1)
			tAssert.Contains(params.Diagnostics[0].Message, `processor: fusion members must be schemas`)
			tAssert.NotNil(params.Diagnostics[0].Code)
		}
	})

	It("publishes variable mismatch diagnostics for the failing declaration", func() {
		notifications := []capturedNotification{}

		didOpen(server, uri, `|===|
string name = "Ada";
int count = "seven";
|===|
[output = 'data']
{
  result: name,
}`, &notifications)

		if tAssert.Len(notifications, 1) {
			params := requireDiagnostics(notifications[0])
			tAssert.Len(params.Diagnostics, 1)
			tAssert.Contains(params.Diagnostics[0].Message, `processor: type mismatch: expected int, got string`)
			tAssert.Equal(protocol.UInteger(2), params.Diagnostics[0].Range.Start.Line)
			tAssert.Equal(protocol.UInteger(4), params.Diagnostics[0].Range.Start.Character)
		}
	})

	It("replaces document content on change and clears diagnostics", func() {
		notifications := []capturedNotification{}

		didOpen(server, uri, `[output = 'data'] { result: , }`, &notifications)
		didChange(server, uri, 2, `[output = 'data'] { result: 3, }`, &notifications)

		if tAssert.Len(notifications, 2) {
			params := requireDiagnostics(notifications[1])
			tAssert.Empty(params.Diagnostics)
		}
	})

	It("clears processor diagnostics when a variable declaration is fixed on change", func() {
		notifications := []capturedNotification{}

		didOpen(server, uri, `|===|
int count = "Ada";
|===|
[output = 'data']
{
  result: count,
}`, &notifications)
		didChange(server, uri, 2, `|===|
int count = 7;
|===|
[output = 'data']
{
  result: count,
}`, &notifications)

		if tAssert.Len(notifications, 2) {
			params := requireDiagnostics(notifications[1])
			tAssert.Empty(params.Diagnostics)
		}
	})

	It("does not report mixed array diagnostics for string arrays", func() {
		notifications := []capturedNotification{}

		didOpen(server, uri, `|===|
array<string> names = ['Kyle', 'Tyrone', 'Luke'];
|===|
[output = 'data']
{
  names: names
}`, &notifications)

		if tAssert.Len(notifications, 1) {
			params := requireDiagnostics(notifications[0])
			tAssert.Empty(params.Diagnostics)
		}
	})

	It("refreshes diagnostics when a document is saved", func() {
		notifications := []capturedNotification{}

		workspace, err := os.MkdirTemp("", "mace-lsp-save-diagnostics-*")
		tAssert.NoError(err)

		path := writeWorkspaceFile(workspace, "consumer.mace", `[output = 'data'] { result: , }`)
		uri := protocol.DocumentUri(path)

		didOpen(server, uri, `[output = 'data'] { result: , }`, &notifications)

		fixedText := `[output = 'data'] { result: 3, }`
		err = os.WriteFile(filepath.FromSlash(documentPath(uri)), []byte(fixedText), 0o600)
		tAssert.NoError(err)

		didSave(server, uri, nil, &notifications)

		if tAssert.Len(notifications, 2) {
			params := requireDiagnostics(notifications[1])
			tAssert.Empty(params.Diagnostics)
		}
	})

	It("warns when parse directives require runtime input", func() {
		notifications := []capturedNotification{}

		didOpen(server, uri, `|===|
schema Package: { name: string, project: string, };
|===|
[output = 'data', parse = Package]
{
  result: "ok",
}`, &notifications)

		if tAssert.Len(notifications, 1) {
			params := requireDiagnostics(notifications[0])
			if tAssert.Len(params.Diagnostics, 1) {
				tAssert.Contains(params.Diagnostics[0].Message, "host-provided input that satisfies the selected schema")
				tAssert.Equal(protocol.DiagnosticSeverityWarning, *params.Diagnostics[0].Severity)
				tAssert.NotNil(params.Diagnostics[0].Code)
			}
		}
	})

	It("warns when parse_file directives require runtime input", func() {
		notifications := []capturedNotification{}
		workspace, err := os.MkdirTemp("", "mace-lsp-parse-ignore-*")
		tAssert.NoError(err)

		writeWorkspaceFile(workspace, "runtime.mace", `[output = 'schema']
{
  Package: { project: string, },
}`)
		uri := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", ``))

		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `[output = 'data', parse_file = './runtime.mace']
{
  result: "ok",
}`, &notifications)

		if tAssert.Len(notifications, 1) {
			params := requireDiagnostics(notifications[0])
			if tAssert.Len(params.Diagnostics, 1) {
				tAssert.Contains(params.Diagnostics[0].Message, "host-provided input that satisfies the selected schema")
				tAssert.Equal(protocol.DiagnosticSeverityWarning, *params.Diagnostics[0].Severity)
				tAssert.NotNil(params.Diagnostics[0].Code)
			}
		}
	})

	It("publishes processor diagnostics for invalid output data structures", func() {
		notifications := []capturedNotification{}

		didOpen(server, uri, `|===|
schema Point: { x: int, y: int, };
schema Plot: { points: array<Point>, };
|===|
[output = 'data', schema = Plot]
{
  points: [
    { x: 1, y: 2, },
    { x: 3, }
  ],
}`, &notifications)

		if tAssert.Len(notifications, 1) {
			params := requireDiagnostics(notifications[0])
			tAssert.Len(params.Diagnostics, 1)
			tAssert.Contains(params.Diagnostics[0].Message, `processor: missing required field "y" for schema "Point"`)
			tAssert.Equal(protocol.UInteger(1), params.Diagnostics[0].Range.Start.Line)
			tAssert.Equal(protocol.UInteger(7), params.Diagnostics[0].Range.Start.Character)
		}
	})

	It("clears output structure diagnostics when nested data is fixed on change", func() {
		notifications := []capturedNotification{}

		didOpen(server, uri, `|===|
schema Point: { x: int, y: int, };
schema Plot: { points: array<Point>, };
|===|
[output = 'data', schema = Plot]
{
  points: [
    { x: 1, y: 2, },
    { x: 3, }
  ],
}`, &notifications)
		didChange(server, uri, 2, `|===|
schema Point: { x: int, y: int, };
schema Plot: { points: array<Point>, };
|===|
[output = 'data', schema = Plot]
{
  points: [
    { x: 1, y: 2, },
    { x: 3, y: 4, }
  ],
}`, &notifications)

		if tAssert.Len(notifications, 2) {
			params := requireDiagnostics(notifications[1])
			tAssert.Empty(params.Diagnostics)
		}
	})

	It("drops document state on close and clears diagnostics", func() {
		notifications := []capturedNotification{}

		didOpen(server, uri, `[output = 'data'] { result: 1, }`, &notifications)
		didClose(server, uri, &notifications)

		if tAssert.Len(notifications, 2) {
			params := requireDiagnostics(notifications[1])
			tAssert.Empty(params.Diagnostics)
		}

		resultValue, validMethod, validParams, err := invoke(server.Handler(), protocol.MethodTextDocumentHover, protocol.HoverParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			},
		}, nil)
		tAssert.True(validMethod)
		tAssert.True(validParams)
		tAssert.NoError(err)
		tAssert.Nil(resultValue)
	})

	It("returns script keyword completions only inside the script block", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, "|===|\nsche", nil)

		resultValue, validMethod, validParams, err := invoke(server.Handler(), protocol.MethodTextDocumentCompletion, protocol.CompletionParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri},
				Position:     protocol.Position{Line: 1, Character: 4},
			},
		}, nil)
		tAssert.True(validMethod)
		tAssert.True(validParams)
		tAssert.NoError(err)

		items, ok := resultValue.([]protocol.CompletionItem)
		tAssert.True(ok)
		if !ok {
			return
		}

		tAssert.NotEmpty(items)
		tAssert.Equal("schema", items[0].Label)
	})

	It("returns import completions only in script scope", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, "fr", nil)

		labels := completeLabels(server, uri, 0, 2)
		tAssert.Empty(labels)

		didChange(server, uri, 3, "|===|\nfr", nil)
		labels = completeLabels(server, uri, 1, 2)
		tAssert.Contains(labels, "from")
	})

	It("only suggests import after a valid from path", func() {

		workspace, err := os.MkdirTemp("", "mace-lsp-import-*")
		tAssert.NoError(err)

		writeWorkspaceFile(workspace, "shared.mace", `[output = 'schema']
{
  User: { name: string, },
  Config: string,
}`)
		uri := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", `|===|
from './shared.mace' imp`))

		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
from './shared.mace' imp`, nil)

		labels := completeLabels(server, uri, 1, uint32(len(`from './shared.mace' imp`)))
		tAssert.Equal([]string{"import"}, labels)
	})

	It("uses the current directory as the default import path baseline", func() {

		workspace, err := os.MkdirTemp("", "mace-lsp-import-path-*")
		tAssert.NoError(err)

		writeWorkspaceFile(workspace, "shared.mace", `[output = 'data'] { name: "Ada", }`)
		writeWorkspaceFile(workspace, "nested/roles.mace", `[output = 'data'] { role: "admin", }`)
		uri := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", ``))

		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
from '`, nil)

		labels := completeLabels(server, uri, 1, uint32(len(`from '`)))
		tAssert.Contains(labels, "./shared.mace")
		tAssert.Contains(labels, "./nested/")
	})

	It("suggests parent relative import paths while the from string changes", func() {

		workspace, err := os.MkdirTemp("", "mace-lsp-parent-import-path-*")
		tAssert.NoError(err)

		writeWorkspaceFile(workspace, "shared.mace", `[output = 'data'] { name: "Ada", }`)
		consumerURI := protocol.DocumentUri(writeWorkspaceFile(workspace, "nested/consumer.mace", ``))

		openEmptyDocument(server, consumerURI, nil)
		didChange(server, consumerURI, 2, `|===|
from '../`, nil)

		labels := completeLabels(server, consumerURI, 1, uint32(len(`from '../`)))
		tAssert.Contains(labels, "../shared.mace")
	})

	It("suggests import after a valid from path change with trailing space", func() {

		workspace, err := os.MkdirTemp("", "mace-lsp-import-space-*")
		tAssert.NoError(err)

		writeWorkspaceFile(workspace, "shared.mace", `[output = 'schema']
{
  User: { name: string, },
}`)
		uri := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", ``))

		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
from './shared.mace' `, nil)

		labels := completeLabels(server, uri, 1, uint32(len(`from './shared.mace' `)))
		tAssert.Equal([]string{"import"}, labels)
	})

	It("only suggests identifiers exported by the import path after change", func() {

		workspace, err := os.MkdirTemp("", "mace-lsp-imported-*")
		tAssert.NoError(err)

		writeWorkspaceFile(workspace, "shared.mace", `[output = 'schema']
{
  User: { name: string, },
  Config: string,
}`)
		uri := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", `|===|
from './shared.mace' import U`))

		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
from './shared.mace' import U`, nil)

		labels := completeLabels(server, uri, 1, uint32(len(`from './shared.mace' import U`)))
		tAssert.Equal([]string{"User"}, labels)
	})

	It("suggests all exported identifiers after import changes", func() {

		workspace, err := os.MkdirTemp("", "mace-lsp-imported-all-*")
		tAssert.NoError(err)

		writeWorkspaceFile(workspace, "shared.mace", `[output = 'schema']
{
  User: { name: string, },
  Config: string,
}`)
		uri := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", ``))

		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
from './shared.mace' import `, nil)

		labels := completeLabels(server, uri, 1, uint32(len(`from './shared.mace' import `)))
		tAssert.Equal([]string{"Config", "User"}, labels)
	})

	It("suggests imported identifiers inside the script block", func() {

		workspace, err := os.MkdirTemp("", "mace-lsp-imported-script-*")
		tAssert.NoError(err)

		writeWorkspaceFile(workspace, "shared.mace", `[output = 'schema']
{
  User: { name: string, },
}`)
		uri := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", ``))

		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
from './shared.mace' import User;
Us
|===|
[output = 'data'] {}`, nil)

		labels := completeLabels(server, uri, 2, 2)
		tAssert.Contains(labels, "User")
	})

	It("only suggests directives inside directive delimiters", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `out`, nil)

		labels := completeLabels(server, uri, 0, 3)
		tAssert.NotContains(labels, "output")

		didChange(server, uri, 3, `[out`, nil)
		labels = completeLabels(server, uri, 0, 4)
		tAssert.Equal([]string{"output"}, labels)
	})

	It("assumes output data when suggesting additional directives", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `[output, p`, nil)

		labels := completeLabels(server, uri, 0, uint32(len(`[output, p`)))
		tAssert.Contains(labels, "parse")
		tAssert.Contains(labels, "parse_file")
		tAssert.NotContains(labels, "output")
	})

	It("does not suggest script keywords in the output block", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `[output = 'data']
{
  str
}`, nil)

		labels := completeLabels(server, uri, 2, 5)
		tAssert.NotContains(labels, "string")
	})

	It("suggests choice in script scope", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
ch
|===|
[output = 'data'] {}`, nil)

		labels := completeLabels(server, uri, 1, 2)
		tAssert.Contains(labels, "choice")
	})

	It("suggests choice values for script variable initializers", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
alias Fruit: choice["Apple", "Strawberry"];
Fruit favorite =
|===|
[output = 'data'] {}`, nil)

		labels := completeLabels(server, uri, 2, uint32(len(`Fruit favorite =`)))
		tAssert.Contains(labels, `"Apple"`)
		tAssert.Contains(labels, `"Strawberry"`)
	})

	It("suggests unquoted choice values inside script strings", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
alias Fruit: choice["Apple", "Strawberry"];
Fruit favorite = "A
|===|
[output = 'data'] {}`, nil)

		labels := completeLabels(server, uri, 2, uint32(len(`Fruit favorite = "A`)))
		tAssert.Contains(labels, "Apple")
		tAssert.NotContains(labels, `"Apple"`)
		tAssert.NotContains(labels, "Strawberry")
	})

	It("suggests choice values for script variable variants", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
alias Status: choice["pending", "approved"];
alias Label: variant[Status, string];
Label label =
|===|
[output = 'data'] {}`, nil)

		labels := completeLabels(server, uri, 3, uint32(len(`Label label =`)))
		tAssert.Contains(labels, `"approved"`)
		tAssert.Contains(labels, `"pending"`)
	})

	It("suggests choice values for script variable record fields", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
alias Fruit: choice["Apple", "Strawberry"];
schema Basket: { favorite_fruit: Fruit, };
Basket basket = {
  favorite_fruit:
};
|===|
[output = 'data'] {}`, nil)

		labels := completeLabels(server, uri, 4, uint32(len(`  favorite_fruit: `)))
		tAssert.Contains(labels, `"Apple"`)
		tAssert.Contains(labels, `"Strawberry"`)
	})

	It("suggests unquoted choice values inside record field strings", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
alias Fruit: choice["Apple", "Strawberry"];
schema Basket: { favorite_fruit: Fruit, };
Basket basket = {
  favorite_fruit: "Str
};
|===|
[output = 'data'] {}`, nil)

		labels := completeLabels(server, uri, 4, uint32(len(`  favorite_fruit: "Str`)))
		tAssert.Equal([]string{"Strawberry"}, labels)
	})

	It("suggests unquoted choice values inside array element strings", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
alias Fruit: choice["Apple", "Strawberry"];
array<Fruit> favorites = ["A
|===|
[output = 'data'] {}`, nil)

		labels := completeLabels(server, uri, 2, uint32(len(`array<Fruit> favorites = ["A`)))
		tAssert.Contains(labels, "Apple")
		tAssert.NotContains(labels, `"Apple"`)
		tAssert.NotContains(labels, "Strawberry")
	})

	It("suggests schema record literals for nested schema fields after a record colon", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
schema Profile: { name: string, age?: int, };
schema Basket: { owner: Profile, };
Basket basket = {
  owner:
};
|===|
[output = 'data'] {}`, nil)

		labels := completeLabels(server, uri, 4, uint32(len(`  owner: `)))
		tAssert.Equal([]string{`{ name: "", age?: 0 }`}, labels)
	})

	It("suggests parse schema fields as output variables", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
schema Runtime: { env: string, region: string, };
|===|
[output = 'data', parse = Runtime]
{
  result:
}`, nil)

		labels := completeLabels(server, uri, 5, uint32(len(`  result: `)))
		tAssert.Contains(labels, "$env")
		tAssert.Contains(labels, "$region")
	})

	It("suggests parse_file output schema fields as output variables", func() {
		workspace, err := os.MkdirTemp("", "mace-lsp-parse-file-*")
		tAssert.NoError(err)

		writeWorkspaceFile(workspace, "runtime.mace", `[output = 'schema']
{
  Runtime: { env: string, region: string, },
}`)
		uri := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", ``))

		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `[output = 'data', parse_file = './runtime.mace']
{
  result:
}`, nil)

		labels := completeLabels(server, uri, 2, uint32(len(`  result: `)))
		tAssert.Contains(labels, "$env")
		tAssert.Contains(labels, "$region")
		tAssert.NotContains(labels, "Runtime")
	})

	It("only suggests top-level parse schema fields as output variables", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
schema Runtime: {
  env: string,
  profile: { name: string, email: string, },
};
|===|
[output = 'data', parse = Runtime]
{
  result:
}`, nil)

		resultValue, validMethod, validParams, err := invoke(server.Handler(), protocol.MethodTextDocumentCompletion, protocol.CompletionParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri},
				Position: protocol.Position{
					Line:      8,
					Character: uint32(len(`  result: `)),
				},
			},
		}, nil)
		tAssert.True(validMethod)
		tAssert.True(validParams)
		tAssert.NoError(err)

		items, ok := resultValue.([]protocol.CompletionItem)
		tAssert.True(ok)
		if !ok {
			return
		}

		labels := lo.Map(items, func(item protocol.CompletionItem, _ int) string { return item.Label })
		details := map[string]string{}
		for _, item := range items {
			if item.Detail != nil {
				details[item.Label] = *item.Detail
			}
		}

		tAssert.Contains(labels, "$env")
		tAssert.Contains(labels, "$profile")
		tAssert.NotContains(labels, "name")
		tAssert.NotContains(labels, "email")
		tAssert.Equal("string", details["$env"])
		tAssert.Equal("{ name: string, email: string }", details["$profile"])
	})

	It("only suggests top-level parse_file schema fields as output variables", func() {
		workspace, err := os.MkdirTemp("", "mace-lsp-parse-file-top-level-*")
		tAssert.NoError(err)

		writeWorkspaceFile(workspace, "runtime.mace", `[output = 'schema']
{
  Runtime: {
    env: string,
    profile: { name: string, email: string, },
  },
}`)
		uri := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", ``))

		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `[output = 'data', parse_file = './runtime.mace']
{
  result:
}`, nil)

		labels := completeLabels(server, uri, 2, uint32(len(`  result: `)))
		tAssert.Contains(labels, "$env")
		tAssert.Contains(labels, "$profile")
		tAssert.NotContains(labels, "name")
		tAssert.NotContains(labels, "email")
		tAssert.NotContains(labels, "Runtime")
	})

	It("suggests parse_file output schema field members as output variables", func() {
		workspace, err := os.MkdirTemp("", "mace-lsp-parse-file-members-*")
		tAssert.NoError(err)

		writeWorkspaceFile(workspace, "runtime.mace", `[output = 'schema']
{
  Runtime: { user: { name: string, home: { street: string, city: string, }, }, },
}`)
		uri := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", ``))

		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `[output = 'data', parse_file = './runtime.mace']
{
  result: $user.
}`, nil)

		labels := completeLabels(server, uri, 2, uint32(len(`  result: $user.`)))
		tAssert.Contains(labels, "name")
		tAssert.Contains(labels, "home")
	})

	It("completes recursive nested parse values through member access", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
schema Contact: {
  email: string,
  phone: string,
};
schema Profile: {
  title: string,
  contact: Contact,
};
schema User: {
  name: string,
  manager: User,
  profile: Profile,
};
|===|
[output = 'data', parse = User]
{
  result: $manager.manager.profile.contact.
}`, nil)

		labels := completeLabels(server, uri, 17, uint32(len(`  result: $manager.manager.profile.contact.`)))
		tAssert.Contains(labels, "email")
		tAssert.Contains(labels, "phone")
	})

	It("only suggests exported parse_file props as output variables", func() {
		workspace, err := os.MkdirTemp("", "mace-lsp-parse-file-exports-*")
		tAssert.NoError(err)

		writeWorkspaceFile(workspace, "nx_inputs.mace", `|===|
schema Project: {
  name: string,
  root: string,
};
schema Workspace: {
  name: string,
  root: string,
};
|===|
[output = 'schema']
{
  project: Project,
  workspace: Workspace,
}`)
		uri := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", ``))

		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `[output = 'data', parse_file = './nx_inputs.mace']
{
  
}`, nil)

		labels := completeLabels(server, uri, 2, 2)
		tAssert.Contains(labels, "$project")
		tAssert.Contains(labels, "$workspace")
		tAssert.NotContains(labels, "name")
		tAssert.NotContains(labels, "root")
		tAssert.NotContains(labels, "cwd")
	})

	It("completes members for exported parse_file props", func() {
		workspace, err := os.MkdirTemp("", "mace-lsp-parse-file-export-members-*")
		tAssert.NoError(err)

		writeWorkspaceFile(workspace, "nx_inputs.mace", `|===|
schema Project: {
  name: string,
  root: string,
};
schema Workspace: {
  name: string,
  root: string,
};
|===|
[output = 'schema']
{
  project: Project,
  workspace: Workspace,
}`)
		uri := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", ``))

		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `[output = 'data', parse_file = './nx_inputs.mace']
{
  result: $project.
}`, nil)

		labels := completeLabels(server, uri, 2, uint32(len(`  result: $project.`)))
		tAssert.Contains(labels, "name")
		tAssert.Contains(labels, "root")
		tAssert.NotContains(labels, "$project")
		tAssert.NotContains(labels, "$workspace")
	})

	It("does not suggest schema names as output expressions", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
schema Runtime: { env: string, };
|===|
[output = 'data']
{
  result:
}`, nil)

		labels := completeLabels(server, uri, 5, uint32(len(`  result: `)))
		tAssert.NotContains(labels, "Runtime")
	})

	It("does not suggest imported schema names as output expressions", func() {
		workspace, err := os.MkdirTemp("", "mace-lsp-output-schema-*")
		tAssert.NoError(err)

		writeWorkspaceFile(workspace, "shared.mace", `[output = 'schema']
{
  Runtime: { env: string, },
}`)
		uri := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", ``))

		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
from './shared.mace' import Runtime;
|===|
[output = 'data']
{
  result:
}`, nil)

		labels := completeLabels(server, uri, 5, uint32(len(`  result: `)))
		tAssert.NotContains(labels, "Runtime")
	})

	It("suggests parse variables when previous output fields use commas", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
schema Runtime: { env: string, region: string, };
|===|
[output = 'data', parse = Runtime]
{
  name: "mace",
  result:
}`, nil)

		labels := completeLabels(server, uri, 6, uint32(len(`  result: `)))
		tAssert.Contains(labels, "$env")
		tAssert.Contains(labels, "$region")
	})

	It("suggests parse_file variables when previous output fields use commas", func() {
		workspace, err := os.MkdirTemp("", "mace-lsp-parse-commas-*")
		tAssert.NoError(err)

		writeWorkspaceFile(workspace, "runtime.mace", `[output = 'schema']
{
  Runtime: { env: string, region: string, },
}`)
		uri := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", ``))

		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `[output = 'data', parse_file = './runtime.mace']
{
  name: "mace",
  result:
}`, nil)

		labels := completeLabels(server, uri, 3, uint32(len(`  result: `)))
		tAssert.Contains(labels, "$env")
		tAssert.Contains(labels, "$region")
		tAssert.NotContains(labels, "Runtime")
	})

	It("suggests choice values for output schema fields", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
 alias Fruit: choice["Apple", "Strawberry"];
 schema Basket: { favorite_fruit: Fruit, };
|===|
[output = 'data', schema = Basket]
{
  favorite_fruit:
}`, nil)

		labels := completeLabels(server, uri, 6, uint32(len(`  favorite_fruit: `)))
		tAssert.Contains(labels, "$self")
		tAssert.Contains(labels, `"Apple"`)
		tAssert.Contains(labels, `"Strawberry"`)
	})

	It("suggests choice values after earlier self member access", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
 alias Fruit: choice["Apple", "Strawberry"];
 schema Basket: { previous: Fruit, favorite_fruit: Fruit, };
|===|
[output = 'data', schema = Basket]
{
  favorite_fruit: true ? $self.previous :
}`, nil)

		labels := completeLabels(server, uri, 6, uint32(len(`  favorite_fruit: true ? $self.previous : `)))
		tAssert.Contains(labels, `"Apple"`)
		tAssert.Contains(labels, `"Strawberry"`)
	})

	It("suggests choice values inside variants while keeping imprecise alternatives", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
 alias Role: choice["Admin", "Member"];
 schema User: { name: string, };
 alias Identity: variant[Role, User];
 schema Envelope: { value: Identity, };
 schema Response: { payload: Envelope, };
|===|
[output = 'data', schema = Response]
{
  payload: {
    value:
  },
}`, nil)

		labels := completeLabels(server, uri, 10, uint32(len(`    value: `)))
		tAssert.Contains(labels, "$self")
		tAssert.Contains(labels, `"Admin"`)
		tAssert.Contains(labels, `"Member"`)
		tAssert.Contains(labels, `{ name: "" }`)
	})

	It("suggests composed schema literals for nested output fusion aliases", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
schema Profile: { name: string, };
schema Audit: { created_at: string, };
alias User: fusion[Profile, Audit];
schema Envelope: { value: User, };
schema Response: { payload: Envelope, };
|===|
[output = 'data', schema = Response]
{
  payload: {
    value:
  },
}`, nil)

		labels := completeLabels(server, uri, 10, uint32(len(`    value: `)))
		tAssert.Contains(labels, "$self")
		tAssert.Contains(labels, `{ name: "", created_at: "" }`)
	})

	It("keeps typed output completions alongside $self in output schema fields", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
 alias Fruit: choice["Apple", "Strawberry"];
 schema Basket: { favorite_fruit: Fruit, };
|===|
[output = 'data', schema = Basket]
{
  favorite_fruit:
}`, nil)

		labels := completeLabels(server, uri, 6, uint32(len(`  favorite_fruit: `)))
		tAssert.Contains(labels, "$self")
		tAssert.Contains(labels, `"Apple"`)
		tAssert.Contains(labels, `"Strawberry"`)
	})

	It("does not suggest schema directives after output schema and a comma", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `[output = 'schema', s`, nil)

		labels := completeLabels(server, uri, 0, uint32(len(`[output = 'schema', s`)))
		tAssert.Empty(labels)
	})

	It("suggests local and imported schemas after schema directive", func() {

		workspace, err := os.MkdirTemp("", "mace-lsp-schema-ref-*")
		tAssert.NoError(err)

		writeWorkspaceFile(workspace, "shared.mace", `[output = 'schema']
{
  ImportedUser: { name: string, },
}`)
		uri := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", ``))

		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
from './shared.mace' import ImportedUser;
schema LocalUser: { id: int, };
|===|
[output = 'data', schema = `, nil)

		labels := completeLabels(server, uri, 4, uint32(len(`[output = 'data', schema = `)))
		tAssert.Equal([]string{"ImportedUser", "LocalUser"}, labels)
	})

	It("suggests schema files and excludes files already imported", func() {

		workspace, err := os.MkdirTemp("", "mace-lsp-schema-file-*")
		tAssert.NoError(err)

		writeWorkspaceFile(workspace, "shared.mace", `[output = 'schema']
{
  ImportedUser: { name: string, },
}`)
		writeWorkspaceFile(workspace, "other.mace", `[output = 'schema']
{
  OtherUser: { name: string, },
}`)
		uri := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", ``))

		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
from './shared.mace' import ImportedUser;
|===|
[output = 'data', schema_file = '`, nil)

		labels := completeLabels(server, uri, 3, uint32(len(`[output = 'data', schema_file = '`)))
		tAssert.NotContains(labels, "./shared.mace")
		tAssert.Contains(labels, "./other.mace")
	})

	It("suggests $self in an empty output expression", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `[output = 'data']
{
  base: 1,
  result:
}`, nil)

		labels := completeLabels(server, uri, 3, uint32(len(`  result: `)))
		tAssert.Contains(labels, "$self")
	})

	It("does not suggest previous output fields without self access", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `[output = 'data']
{
  base: 1,
  profile: { name: "Ada", },
  result:
}`, nil)

		labels := completeLabels(server, uri, 3, uint32(len(`  result: `)))
		tAssert.NotContains(labels, "base")
		tAssert.NotContains(labels, "$profile")
		tAssert.Contains(labels, "$self")
	})

	It("suggests $self after typing a dollar in the output block", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `[output = 'data']
{
  result: $
}`, nil)

		labels := completeLabels(server, uri, 2, uint32(len(`  result: $`)))
		tAssert.Equal([]string{"$self"}, labels)
	})

	It("filters $self completion by typed prefix in the output block", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `[output = 'data']
{
  result: $s
}`, nil)

		labels := completeLabels(server, uri, 2, uint32(len(`  result: $s`)))
		tAssert.Equal([]string{"$self"}, labels)
	})

	It("suggests only previously evaluated output fields after $self dot", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `[output = 'data']
{
  base: 4,
  profile: { name: "Ada", },
  result: $self.
}`, nil)

		labels := completeLabels(server, uri, 4, uint32(len(`  result: $self.`)))
		tAssert.Equal([]string{"base", "profile"}, labels)
	})

	It("suggests nested keys from previously evaluated self fields", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `[output = 'data']
{
  profile: { name: "Ada", details: { age: 30, }, },
  result: $self.profile.
}`, nil)

		labels := completeLabels(server, uri, 3, uint32(len(`  result: $self.profile.`)))
		tAssert.Equal([]string{"details", "name"}, labels)
	})

	It("suggests nested keys from uppercase self paths", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `[output = 'data']
{
  User: { profile: { age: 30, }, },
  result: $self.User.profile.
}`, nil)

		labels := completeLabels(server, uri, 3, uint32(len(`  result: $self.User.profile.`)))
		tAssert.Equal([]string{"age"}, labels)
	})

	DescribeTable("suggests recursive keys from deeply nested self paths",
		func(depth int) {
			openEmptyDocument(server, uri, nil)

			text := nestedSelfDocument(depth)
			didChange(server, uri, 2, text, nil)

			lines := strings.Split(text, "\n")
			line := uint32(len(lines) - 2)
			character := uint32(len(lines[len(lines)-2]))
			labels := completeLabels(server, uri, line, character)
			tAssert.Equal([]string{"final"}, labels)
		},
		Entry("level 3", 3),
		Entry("level 4", 4),
		Entry("level 5", 5),
		Entry("level 6", 6),
		Entry("level 7", 7),
		Entry("level 8", 8),
		Entry("level 9", 9),
		Entry("level 10", 10),
		Entry("level 11", 11),
		Entry("level 12", 12),
	)

	It("suggests recursive keys when prior fields combine into a nested calculation source", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `[output = 'data']
{
  profile: { stats: { base: 2, multiplier: 3, }, },
  summary: {
    stats: {
      total: $self.profile.stats.base * $self.profile.stats.multiplier,
      base: $self.profile.stats.base,
    },
  },
  result: $self.summary.stats.
}`, nil)

		labels := completeLabels(server, uri, 9, uint32(len(`  result: $self.summary.stats.`)))
		tAssert.Equal([]string{"base", "total"}, labels)
	})

	It("suggests recursive keys when nested records reuse self values across multiple places", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `[output = 'data']
{
  account: { balance: 10, bonus: 5, },
  ledger: {
    previous: $self.account.balance,
    next: $self.account.balance + $self.account.bonus,
    nested: { delta: $self.account.bonus, },
  },
  result: $self.ledger.
}`, nil)

		labels := completeLabels(server, uri, 8, uint32(len(`  result: $self.ledger.`)))
		tAssert.Equal([]string{"nested", "next", "previous"}, labels)
	})

	It("returns hover documentation for language keywords", func() {
		didOpen(server, uri, `|===|
schema User: { name: string, };
alias Identity: variant[string, int];
alias UserRecord: fusion[User, Profile];
alias Status: choice["active", "inactive"];
schema Profile: { age: int, };
|===|
[output = 'data'] { name: "Ada", }`, nil)

		resultValue, validMethod, validParams, err := invoke(server.Handler(), protocol.MethodTextDocumentHover, protocol.HoverParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri},
				Position:     protocol.Position{Line: 1, Character: 2},
			},
		}, nil)
		tAssert.True(validMethod)
		tAssert.True(validParams)
		tAssert.NoError(err)

		hover, ok := resultValue.(*protocol.Hover)
		tAssert.True(ok)
		if !ok || hover == nil {
			return
		}

		content, ok := hover.Contents.(protocol.MarkupContent)
		tAssert.True(ok)
		if ok {
			tAssert.Contains(content.Value, "record schema")
		}

		resultValue, validMethod, validParams, err = invoke(server.Handler(), protocol.MethodTextDocumentHover, protocol.HoverParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri},
				Position:     protocol.Position{Line: 2, Character: 17},
			},
		}, nil)
		tAssert.True(validMethod)
		tAssert.True(validParams)
		tAssert.NoError(err)

		hover, ok = resultValue.(*protocol.Hover)
		tAssert.True(ok)
		if !ok || hover == nil {
			return
		}

		content, ok = hover.Contents.(protocol.MarkupContent)
		tAssert.True(ok)
		if ok {
			tAssert.Contains(content.Value, "closed variant type")
		}

		resultValue, validMethod, validParams, err = invoke(server.Handler(), protocol.MethodTextDocumentHover, protocol.HoverParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri},
				Position:     protocol.Position{Line: 3, Character: 19},
			},
		}, nil)
		tAssert.True(validMethod)
		tAssert.True(validParams)
		tAssert.NoError(err)

		hover, ok = resultValue.(*protocol.Hover)
		tAssert.True(ok)
		if !ok || hover == nil {
			return
		}

		content, ok = hover.Contents.(protocol.MarkupContent)
		tAssert.True(ok)
		if ok {
			tAssert.Contains(content.Value, "schema composition")
		}

		resultValue, validMethod, validParams, err = invoke(server.Handler(), protocol.MethodTextDocumentHover, protocol.HoverParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri},
				Position:     protocol.Position{Line: 4, Character: 14},
			},
		}, nil)
		tAssert.True(validMethod)
		tAssert.True(validParams)
		tAssert.NoError(err)

		hover, ok = resultValue.(*protocol.Hover)
		tAssert.True(ok)
		if !ok || hover == nil {
			return
		}

		content, ok = hover.Contents.(protocol.MarkupContent)
		tAssert.True(ok)
		if ok {
			tAssert.Contains(content.Value, "finite literal choice type")
		}
	})

	It("returns directive-aware hover documentation for schema inside output directives", func() {
		didOpen(server, uri, `[output = 'data', schema = User]
{
  result: 1,
}`, nil)

		resultValue, validMethod, validParams, err := invoke(server.Handler(), protocol.MethodTextDocumentHover, protocol.HoverParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri},
				Position:     protocol.Position{Line: 0, Character: 17},
			},
		}, nil)
		tAssert.True(validMethod)
		tAssert.True(validParams)
		tAssert.NoError(err)

		hover, ok := resultValue.(*protocol.Hover)
		tAssert.True(ok)
		if !ok || hover == nil {
			return
		}

		content, ok := hover.Contents.(protocol.MarkupContent)
		tAssert.True(ok)
		if ok {
			tAssert.Contains(content.Value, "does not switch output mode")
		}
	})

	DescribeTable("resolves member hover from its receiver variable",
		func(target string, expectedDetail string) {
			text := `|===|
schema TextHolder: { records: record<string>, text_only?: string, };
schema NumberHolder: { records: array<int>, number_only?: int, };
TextHolder text_holder = { records: { primary: "active", }, };
NumberHolder number_holder = { records: [1, 2], };
boolean configured = true;
|===|
[output = 'data']
{
  records: configured ? text_holder.records : {},
  numbers: configured ? number_holder.records : [],
}`
			didOpen(server, uri, text, nil)

			targetIndex := strings.Index(text, target)
			tAssert.GreaterOrEqual(targetIndex, 0)
			position := positionFromIndex(text, targetIndex+strings.LastIndex(target, ".")+1)
			resultValue, validMethod, validParams, err := invoke(server.Handler(), protocol.MethodTextDocumentHover, protocol.HoverParams{
				TextDocumentPositionParams: protocol.TextDocumentPositionParams{
					TextDocument: protocol.TextDocumentIdentifier{URI: uri},
					Position:     position,
				},
			}, nil)
			tAssert.True(validMethod)
			tAssert.True(validParams)
			tAssert.NoError(err)

			hover, ok := resultValue.(*protocol.Hover)
			tAssert.True(ok)
			if !ok || hover == nil {
				return
			}

			content, ok := hover.Contents.(protocol.MarkupContent)
			tAssert.True(ok)
			if ok {
				tAssert.Contains(content.Value, expectedDetail)
				tAssert.NotContains(content.Value, "output records")
			}
		},
		Entry("record member", "text_holder.records : {}", "text_holder.records: record<string>"),
		Entry("array member", "number_holder.records : []", "number_holder.records: array<int>"),
	)

	DescribeTable("completes output member access from its receiver variable",
		func(target string, expected []string, excluded string) {
			text := fmt.Sprintf(`|===|
schema TextHolder: { records: record<string>, text_only?: string, };
schema NumberHolder: { records: array<int>, number_only?: int, };
TextHolder text_holder = { records: { primary: "active", }, };
NumberHolder number_holder = { records: [1, 2], };
|===|
[output = 'data']
{
  result: %s.
}`, target)
			didOpen(server, uri, text, nil)

			labels := completeLabels(server, uri, 8, uint32(len("  result: "+target+".")))
			tAssert.ElementsMatch(expected, labels)
			tAssert.NotContains(labels, excluded)
		},
		Entry("record variable", "text_holder", []string{"records", "text_only"}, "number_only"),
		Entry("array-bearing variable", "number_holder", []string{"number_only", "records"}, "text_only"),
	)

	It("surfaces every conditional branch type in output hover", func() {
		text := `|===|
variant[string, int, boolean] selected_value = true ? "primary" : false ? 10 : true;
|===|
[output = 'data']
{
  selected: selected_value,
}`
		didOpen(server, uri, text, nil)

		resultValue, validMethod, validParams, err := invoke(server.Handler(), protocol.MethodTextDocumentHover, protocol.HoverParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri},
				Position:     protocol.Position{Line: 5, Character: 4},
			},
		}, nil)
		tAssert.True(validMethod)
		tAssert.True(validParams)
		tAssert.NoError(err)

		hover, ok := resultValue.(*protocol.Hover)
		tAssert.True(ok)
		if !ok || hover == nil {
			return
		}

		content, ok := hover.Contents.(protocol.MarkupContent)
		tAssert.True(ok)
		if ok {
			tAssert.Contains(content.Value, `output selected: variant[string, int, boolean] = "primary"`)
		}
	})

	It("infers a string from coalesced and empty-string conditional branches", func() {
		text := `|===|
schema Address: { city?: string, };
schema Profile: { address: Address, };
schema User: { profile: Profile, };
User user = { profile: { address: { city: "Paris", }, }, };
string fallback = "";
|===|
[output = 'data']
{
  city: user ? user.profile.address?.city ?? fallback : "",
}`
		didOpen(server, uri, text, nil)

		resultValue, validMethod, validParams, err := invoke(server.Handler(), protocol.MethodTextDocumentHover, protocol.HoverParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri},
				Position:     protocol.Position{Line: 9, Character: 4},
			},
		}, nil)
		tAssert.True(validMethod)
		tAssert.True(validParams)
		tAssert.NoError(err)

		hover, ok := resultValue.(*protocol.Hover)
		tAssert.True(ok)
		if !ok || hover == nil {
			return
		}

		content, ok := hover.Contents.(protocol.MarkupContent)
		tAssert.True(ok)
		if ok {
			tAssert.Contains(content.Value, `output city: string`)
			tAssert.NotContains(content.Value, ` = `)
		}
	})

	DescribeTable("omits evaluated values from hover for empty literals",
		func(text string, line uint32, expectedDetail string) {
			didOpen(server, uri, text, nil)

			resultValue, validMethod, validParams, err := invoke(server.Handler(), protocol.MethodTextDocumentHover, protocol.HoverParams{
				TextDocumentPositionParams: protocol.TextDocumentPositionParams{
					TextDocument: protocol.TextDocumentIdentifier{URI: uri},
					Position:     protocol.Position{Line: line, Character: 4},
				},
			}, nil)
			tAssert.True(validMethod)
			tAssert.True(validParams)
			tAssert.NoError(err)

			hover, ok := resultValue.(*protocol.Hover)
			tAssert.True(ok)
			if !ok || hover == nil {
				return
			}

			content, ok := hover.Contents.(protocol.MarkupContent)
			tAssert.True(ok)
			if ok {
				tAssert.Contains(content.Value, expectedDetail)
				tAssert.NotContains(content.Value, ` = `)
			}
		},
		Entry("empty string", `[output = 'data']
{
  value: "",
}`, uint32(2), `output value: string`),
		Entry("contextual empty array ternary", `|===|
schema Result: { value: array<string>, };
|===|
[output = 'data', schema = Result]
{
  value: true ? ["configured"] : [],
}`, uint32(5), `output value: array<string>`),
		Entry("contextual empty record ternary", `|===|
schema Result: { value: { name?: string, }, };
|===|
[output = 'data', schema = Result]
{
  value: true ? { name: "Ada", } : {},
}`, uint32(5), `output value: { name?: string }`),
	)

	DescribeTable("does not infer schema-less output ternaries containing empty collections",
		func(declaration string, expression string) {
			text := fmt.Sprintf(`|===|
boolean configured = true;
%s
|===|
[output = 'data']
{
  value: %s,
}`, declaration, expression)
			didOpen(server, uri, text, nil)

			resultValue, validMethod, validParams, err := invoke(server.Handler(), protocol.MethodTextDocumentHover, protocol.HoverParams{
				TextDocumentPositionParams: protocol.TextDocumentPositionParams{
					TextDocument: protocol.TextDocumentIdentifier{URI: uri},
					Position:     protocol.Position{Line: 6, Character: 4},
				},
			}, nil)
			tAssert.True(validMethod)
			tAssert.True(validParams)
			tAssert.NoError(err)

			hover, ok := resultValue.(*protocol.Hover)
			tAssert.True(ok)
			if !ok || hover == nil {
				return
			}

			content, ok := hover.Contents.(protocol.MarkupContent)
			tAssert.True(ok)
			if ok {
				tAssert.Contains(content.Value, "output value")
				tAssert.NotContains(content.Value, "output value:")
			}
		},
		Entry(
			"empty array",
			`array<string> configured_value = ["configured"];`,
			`configured ? configured_value : []`,
		),
		Entry(
			"empty record",
			`record<string> configured_value = { primary: "active", };`,
			`configured ? configured_value : {}`,
		),
	)

	DescribeTable("uses schema_file types for output ternaries containing empty collections",
		func(fieldType string, expression string, expectedDetail string) {
			workspace, err := os.MkdirTemp("", "mace-lsp-hover-empty-output-*")
			tAssert.NoError(err)
			defer func() { _ = os.RemoveAll(workspace) }()

			writeWorkspaceFile(workspace, "schema.mace", fmt.Sprintf(`[output = 'schema']
{
  value: %s,
}`, fieldType))
			text := fmt.Sprintf(`[output = 'data', schema_file = './schema.mace']
{
  value: %s,
}`, expression)
			documentURI := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", text))
			didOpen(server, documentURI, text, nil)

			resultValue, validMethod, validParams, err := invoke(server.Handler(), protocol.MethodTextDocumentHover, protocol.HoverParams{
				TextDocumentPositionParams: protocol.TextDocumentPositionParams{
					TextDocument: protocol.TextDocumentIdentifier{URI: documentURI},
					Position:     protocol.Position{Line: 2, Character: 4},
				},
			}, nil)
			tAssert.True(validMethod)
			tAssert.True(validParams)
			tAssert.NoError(err)

			hover, ok := resultValue.(*protocol.Hover)
			tAssert.True(ok)
			if !ok || hover == nil {
				return
			}

			content, ok := hover.Contents.(protocol.MarkupContent)
			tAssert.True(ok)
			if ok {
				tAssert.Contains(content.Value, expectedDetail)
				tAssert.NotContains(content.Value, ` = `)
			}
		},
		Entry(
			"empty array",
			`array<string>`,
			`true ? ["configured"] : []`,
			`output value: array<string>`,
		),
		Entry(
			"empty record",
			`{ name?: string, }`,
			`true ? { name: "Ada", } : {}`,
			`output value: { name?: string }`,
		),
	)

	It("surfaces conditional variants imported from Mace files", func() {
		workspace, err := os.MkdirTemp("", "mace-lsp-hover-import-conditional-*")
		tAssert.NoError(err)

		writeWorkspaceFile(workspace, "shared.mace", `[output = 'data']
{
  selected: true ? "primary" : false ? 10 : true,
}`)
		text := `|===|
from './shared.mace' import selected;
variant[string, int, boolean] current = selected;
|===|
[output = 'data'] { result: current, }`
		uri := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", text))
		didOpen(server, uri, text, nil)

		resultValue, validMethod, validParams, err := invoke(server.Handler(), protocol.MethodTextDocumentHover, protocol.HoverParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri},
				Position:     protocol.Position{Line: 2, Character: 43},
			},
		}, nil)
		tAssert.True(validMethod)
		tAssert.True(validParams)
		tAssert.NoError(err)

		hover, ok := resultValue.(*protocol.Hover)
		tAssert.True(ok)
		if !ok || hover == nil {
			return
		}

		content, ok := hover.Contents.(protocol.MarkupContent)
		tAssert.True(ok)
		if ok {
			tAssert.Contains(content.Value, `import selected: variant[string, int, boolean]`)
		}
	})

	It("returns hover details for user declarations", func() {
		didOpen(server, uri, `|===|
string env = "dev";
schema Profile: { name: string, };
schema Audit: { created_at: string, };
alias Identity: variant[string, int];
alias User: fusion[Profile, Audit];
Identity id = "Ada";
User user = { name: "Ada", created_at: "2026-04-09", };
|===|
[output = 'data'] { result: env, chosen: id, record: user, }`, nil)

		resultValue, validMethod, validParams, err := invoke(server.Handler(), protocol.MethodTextDocumentHover, protocol.HoverParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri},
				Position:     protocol.Position{Line: 8, Character: 25},
			},
		}, nil)
		tAssert.True(validMethod)
		tAssert.True(validParams)
		tAssert.NoError(err)

		hover, ok := resultValue.(*protocol.Hover)
		tAssert.True(ok)
		if !ok || hover == nil {
			return
		}

		content, ok := hover.Contents.(protocol.MarkupContent)
		tAssert.True(ok)
		if ok {
			tAssert.Contains(content.Value, "string env")
		}

		resultValue, validMethod, validParams, err = invoke(server.Handler(), protocol.MethodTextDocumentHover, protocol.HoverParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri},
				Position:     protocol.Position{Line: 8, Character: 37},
			},
		}, nil)
		tAssert.True(validMethod)
		tAssert.True(validParams)
		tAssert.NoError(err)

		hover, ok = resultValue.(*protocol.Hover)
		tAssert.True(ok)
		if !ok || hover == nil {
			return
		}

		content, ok = hover.Contents.(protocol.MarkupContent)
		tAssert.True(ok)
		if ok {
			tAssert.Contains(content.Value, `variant[string, int] id = "Ada"`)
		}

		resultValue, validMethod, validParams, err = invoke(server.Handler(), protocol.MethodTextDocumentHover, protocol.HoverParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri},
				Position:     protocol.Position{Line: 8, Character: 49},
			},
		}, nil)
		tAssert.True(validMethod)
		tAssert.True(validParams)
		tAssert.NoError(err)

		hover, ok = resultValue.(*protocol.Hover)
		tAssert.True(ok)
		if !ok || hover == nil {
			return
		}

		content, ok = hover.Contents.(protocol.MarkupContent)
		tAssert.True(ok)
		if ok {
			tAssert.Contains(content.Value, `fusion[Profile, Audit] user = record literal`)
		}
	})

	It("includes gen_doc details for choice types in hover", func() {
		didOpen(server, uri, `|===|
 alias Flavor: choice["Vanilla", "Chocolate"];
 gen_doc Flavor {
   summary: "Selectable flavor values",
   description: "Use autocomplete to choose a supported flavor.",
 };
 Flavor current = "Vanilla";
|===|
[output = 'data'] { result: current, }`, nil)

		resultValue, validMethod, validParams, err := invoke(server.Handler(), protocol.MethodTextDocumentHover, protocol.HoverParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri},
				Position:     protocol.Position{Line: 5, Character: 1},
			},
		}, nil)
		tAssert.True(validMethod)
		tAssert.True(validParams)
		tAssert.NoError(err)

		hover, ok := resultValue.(*protocol.Hover)
		tAssert.True(ok)
		if !ok || hover == nil {
			return
		}

		content, ok := hover.Contents.(protocol.MarkupContent)
		tAssert.True(ok)
		if ok {
			tAssert.Contains(content.Value, `alias Flavor: choice["Vanilla", "Chocolate"];`)
			tAssert.Contains(content.Value, `Selectable flavor values`)
			tAssert.Contains(content.Value, `Use autocomplete to choose a supported flavor.`)
		}
	})

	It("includes inline type descriptions in hover details when the type is used", func() {
		didOpen(server, uri, `|===|
alias UserID: string /# A stable user identifier;
UserID current = "user_1";
|===|
[output = 'data'] { result: current, }`, nil)

		resultValue, validMethod, validParams, err := invoke(server.Handler(), protocol.MethodTextDocumentHover, protocol.HoverParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri},
				Position:     protocol.Position{Line: 2, Character: 1},
			},
		}, nil)
		tAssert.True(validMethod)
		tAssert.True(validParams)
		tAssert.NoError(err)

		hover, ok := resultValue.(*protocol.Hover)
		tAssert.True(ok)
		if !ok || hover == nil {
			return
		}

		content, ok := hover.Contents.(protocol.MarkupContent)
		tAssert.True(ok)
		if ok {
			tAssert.Contains(content.Value, `alias UserID: string;`)
			tAssert.Contains(content.Value, `A stable user identifier`)
		}
	})

	It("includes exported inline descriptions in imported hover details", func() {
		workspace, err := os.MkdirTemp("", "mace-lsp-hover-import-inline-doc-*")
		tAssert.NoError(err)

		writeWorkspaceFile(workspace, "shared.mace", `[output = 'schema']
{
  User: { name: string, } /# Public user schema,
}`)
		uri := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", `|===|
from './shared.mace' import User;
User current = { name: "Ada", };
|===|
[output = 'data'] { result: current, }`))

		didOpen(server, uri, `|===|
from './shared.mace' import User;
User current = { name: "Ada", };
|===|
[output = 'data'] { result: current, }`, nil)

		resultValue, validMethod, validParams, err := invoke(server.Handler(), protocol.MethodTextDocumentHover, protocol.HoverParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri},
				Position:     protocol.Position{Line: 2, Character: 1},
			},
		}, nil)
		tAssert.True(validMethod)
		tAssert.True(validParams)
		tAssert.NoError(err)

		hover, ok := resultValue.(*protocol.Hover)
		tAssert.True(ok)
		if !ok || hover == nil {
			return
		}

		content, ok := hover.Contents.(protocol.MarkupContent)
		tAssert.True(ok)
		if ok {
			tAssert.Contains(content.Value, `schema User: { name: string }`)
			tAssert.Contains(content.Value, `Public user schema`)
		}
	})

	It("includes documentation declaration metadata in hover details", func() {
		didOpen(server, uri, `|===|
schema User: { name: string, };

schema_doc User {
  summary: "Represents a user.",
  description: """
# User
Reusable schema.
""",
  fields: {
    name: "The user's display name",
  },
};
|===|
[output = 'schema']
{ user: User, }`, nil)

		resultValue, validMethod, validParams, err := invoke(server.Handler(), protocol.MethodTextDocumentHover, protocol.HoverParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri},
				Position:     protocol.Position{Line: 15, Character: 8},
			},
		}, nil)
		tAssert.True(validMethod)
		tAssert.True(validParams)
		tAssert.NoError(err)

		hover, ok := resultValue.(*protocol.Hover)
		tAssert.True(ok)
		if !ok || hover == nil {
			return
		}

		content, ok := hover.Contents.(protocol.MarkupContent)
		tAssert.True(ok)
		if ok {
			tAssert.Contains(content.Value, `schema User: { name: string };`)
			tAssert.Contains(content.Value, `Represents a user.`)
			tAssert.Contains(content.Value, `# User`)
			tAssert.Contains(content.Value, `Fields:`)
			tAssert.Contains(content.Value, "`name`: The user's display name")
		}
	})

	It("loads hover documentation from the docs fixture", func() {
		didOpen(server, uri, `|===|
schema User: {
  name: string,
};

string greeting = "Hello";

gen_doc greeting {
  summary: "Rendered greeting",
};

schema_doc User {
  summary: "Represents a user",
  description: """
# User

Hover should surface this documentation.
""",
  fields: {
    name: "The user's display name",
  },
};
|===|
[output = 'schema']
"""
# User Output
"""
{
  user: User /# Public user schema,
}
`, nil)

		resultValue, validMethod, validParams, err := invoke(server.Handler(), protocol.MethodTextDocumentHover, protocol.HoverParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri},
				Position:     protocol.Position{Line: 28, Character: 9},
			},
		}, nil)
		tAssert.True(validMethod)
		tAssert.True(validParams)
		tAssert.NoError(err)

		hover, ok := resultValue.(*protocol.Hover)
		tAssert.True(ok)
		if !ok || hover == nil {
			return
		}

		content, ok := hover.Contents.(protocol.MarkupContent)
		tAssert.True(ok)
		if ok {
			tAssert.Contains(content.Value, `schema User: { name: string };`)
			tAssert.Contains(content.Value, `Represents a user`)
			tAssert.Contains(content.Value, `Hover should surface this documentation.`)
			tAssert.Contains(content.Value, `Fields:`)
			tAssert.Contains(content.Value, "`name`: The user's display name")
		}
	})

	It("prefers output field hover details over same-named schema declarations", func() {
		didOpen(server, uri, `|===|
schema User: { name: string, };
|===|
[output = 'data']
{
  User: { name: "Ada", },
}`, nil)

		resultValue, validMethod, validParams, err := invoke(server.Handler(), protocol.MethodTextDocumentHover, protocol.HoverParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri},
				Position:     protocol.Position{Line: 5, Character: 3},
			},
		}, nil)
		tAssert.True(validMethod)
		tAssert.True(validParams)
		tAssert.NoError(err)

		hover, ok := resultValue.(*protocol.Hover)
		tAssert.True(ok)
		if !ok || hover == nil {
			return
		}

		content, ok := hover.Contents.(protocol.MarkupContent)
		tAssert.True(ok)
		if ok {
			tAssert.Contains(content.Value, "output User")
			tAssert.NotContains(content.Value, "schema User")
		}
	})

	It("returns hover details for nested output record fields", func() {
		didOpen(server, uri, `|===|
alias Name: string;
schema Profile: { age: int, };
schema User: { name: Name, profile: Profile, };
Name default_name = "Ada";
int default_age = 30;
|===|
[output = 'data']
{
  User: {
    name: default_name,
    profile: { age: default_age, },
  },
}`, nil)

		resultValue, validMethod, validParams, err := invoke(server.Handler(), protocol.MethodTextDocumentHover, protocol.HoverParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri},
				Position:     protocol.Position{Line: 11, Character: 8},
			},
		}, nil)
		tAssert.True(validMethod)
		tAssert.True(validParams)
		tAssert.NoError(err)

		hover, ok := resultValue.(*protocol.Hover)
		tAssert.True(ok)
		if !ok || hover == nil {
			return
		}

		content, ok := hover.Contents.(protocol.MarkupContent)
		tAssert.True(ok)
		if ok {
			tAssert.Contains(content.Value, `output User.profile: { age: int } = { age: 30 }`)
		}
	})

	It("prefers output field hover details when the same name is reused later in self references", func() {
		didOpen(server, uri, `|===|
alias Name: string;
schema Profile: { age: int, };
schema User: { name: Name, profile: Profile, };
Name default_name = "Ada";
int default_age = 30;
|===|
[output = 'data']
{
  User: {
    name: default_name,
    profile: { age: default_age, },
  },
  foo: $self.User.profile.age,
  bar: $self.foo,
  baz: $self.User.name,
}`, nil)

		resultValue, validMethod, validParams, err := invoke(server.Handler(), protocol.MethodTextDocumentHover, protocol.HoverParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri},
				Position:     protocol.Position{Line: 8, Character: 3},
			},
		}, nil)
		tAssert.True(validMethod)
		tAssert.True(validParams)
		tAssert.NoError(err)

		hover, ok := resultValue.(*protocol.Hover)
		tAssert.True(ok)
		if !ok || hover == nil {
			return
		}

		content, ok := hover.Contents.(protocol.MarkupContent)
		tAssert.True(ok)
		if ok {
			tAssert.Contains(content.Value, `output User: { name: string, profile: { age: int, } }`)
			tAssert.Contains(content.Value, `name: "Ada"`)
			tAssert.NotContains(content.Value, "schema User")
		}
	})

	It("returns hover details for nested self references", func() {
		didOpen(server, uri, `[output = 'data']
{
  User: { profile: { age: 30, }, },
  foo: $self.User.profile.age,
}`, nil)

		resultValue, validMethod, validParams, err := invoke(server.Handler(), protocol.MethodTextDocumentHover, protocol.HoverParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri},
				Position:     protocol.Position{Line: 3, Character: 20},
			},
		}, nil)
		tAssert.True(validMethod)
		tAssert.True(validParams)
		tAssert.NoError(err)

		hover, ok := resultValue.(*protocol.Hover)
		tAssert.True(ok)
		if !ok || hover == nil {
			return
		}

		content, ok := hover.Contents.(protocol.MarkupContent)
		tAssert.True(ok)
		if ok {
			tAssert.Contains(content.Value, `output User.profile: { age: int } = { age: 30 }`)
		}
	})

	It("returns hover details for deeply nested self record references", func() {
		didOpen(server, uri, `[output = 'data']
{
  summary: {
    stats: {
      totals: { users: 3, },
    },
  },
  result: $self.summary.stats.totals.users,
}`, nil)

		resultValue, validMethod, validParams, err := invoke(server.Handler(), protocol.MethodTextDocumentHover, protocol.HoverParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri},
				Position:     protocol.Position{Line: 7, Character: 31},
			},
		}, nil)
		tAssert.True(validMethod)
		tAssert.True(validParams)
		tAssert.NoError(err)

		hover, ok := resultValue.(*protocol.Hover)
		tAssert.True(ok)
		if !ok || hover == nil {
			return
		}

		content, ok := hover.Contents.(protocol.MarkupContent)
		tAssert.True(ok)
		if ok {
			tAssert.Contains(content.Value, `output summary.stats.totals: { users: int } = { users: 3 }`)
		}
	})

	It("returns hover details for imported choice declarations", func() {

		workspace, err := os.MkdirTemp("", "mace-lsp-hover-import-choice-*")
		tAssert.NoError(err)

		writeWorkspaceFile(workspace, "shared.mace", `|===|
 alias Flavor: choice["Vanilla", "Chocolate"];
|===|
[output = 'schema']
{
  Flavor: Flavor,
}`)
		uri := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", `|===|
from './shared.mace' import Flavor;
Flavor current = "Vanilla";
|===|
[output = 'data'] { result: current, }`))

		didOpen(server, uri, `|===|
from './shared.mace' import Flavor;
Flavor current = "Vanilla";
|===|
[output = 'data'] { result: current, }`, nil)

		resultValue, validMethod, validParams, err := invoke(server.Handler(), protocol.MethodTextDocumentHover, protocol.HoverParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri},
				Position:     protocol.Position{Line: 2, Character: 1},
			},
		}, nil)
		tAssert.True(validMethod)
		tAssert.True(validParams)
		tAssert.NoError(err)

		hover, ok := resultValue.(*protocol.Hover)
		tAssert.True(ok)
		if !ok || hover == nil {
			return
		}

		content, ok := hover.Contents.(protocol.MarkupContent)
		tAssert.True(ok)
		if ok {
			tAssert.Contains(content.Value, `alias Flavor: choice["Vanilla", "Chocolate"];`)
		}
	})

	It("returns hover details for imported declarations", func() {

		workspace, err := os.MkdirTemp("", "mace-lsp-hover-import-*")
		tAssert.NoError(err)

		writeWorkspaceFile(workspace, "shared.mace", `[output = 'schema']
{
  User: { name: string, },
}`)
		uri := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", `|===|
from './shared.mace' import User;
User current = { name: "Ada", };
|===|
[output = 'data'] { result: current, }`))

		didOpen(server, uri, `|===|
from './shared.mace' import User;
User current = { name: "Ada", };
|===|
[output = 'data'] { result: current, }`, nil)

		resultValue, validMethod, validParams, err := invoke(server.Handler(), protocol.MethodTextDocumentHover, protocol.HoverParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri},
				Position:     protocol.Position{Line: 2, Character: 1},
			},
		}, nil)
		tAssert.True(validMethod)
		tAssert.True(validParams)
		tAssert.NoError(err)

		hover, ok := resultValue.(*protocol.Hover)
		tAssert.True(ok)
		if !ok || hover == nil {
			return
		}

		content, ok := hover.Contents.(protocol.MarkupContent)
		tAssert.True(ok)
		if ok {
			tAssert.Contains(content.Value, "schema User")
		}
	})

	It("returns definition locations for imported symbols", func() {

		workspace, err := os.MkdirTemp("", "mace-lsp-definition-import-*")
		tAssert.NoError(err)

		importPath := writeWorkspaceFile(workspace, "shared.mace", `[output = 'schema']
{
  User: { name: string, },
}`)
		uri := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", `|===|
from './shared.mace' import User;
User current = { name: "Ada", };
|===|
[output = 'data'] { result: current, }`))

		didOpen(server, uri, `|===|
from './shared.mace' import User;
User current = { name: "Ada", };
|===|
[output = 'data'] { result: current, }`, nil)

		resultValue, validMethod, validParams, err := invoke(server.Handler(), protocol.MethodTextDocumentDefinition, protocol.DefinitionParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri},
				Position:     protocol.Position{Line: 2, Character: 1},
			},
		}, nil)
		tAssert.True(validMethod)
		tAssert.True(validParams)
		tAssert.NoError(err)

		location, ok := resultValue.(protocol.Location)
		tAssert.True(ok)
		if !ok {
			return
		}

		tAssert.Equal(protocol.DocumentUri(importPath), location.URI)
	})

	It("prefers output field definitions over same-named schema declarations", func() {
		didOpen(server, uri, `|===|
schema User: { name: string, };
|===|
[output = 'data']
{
  User: { name: "Ada", },
}`, nil)

		resultValue, validMethod, validParams, err := invoke(server.Handler(), protocol.MethodTextDocumentDefinition, protocol.DefinitionParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri},
				Position:     protocol.Position{Line: 5, Character: 3},
			},
		}, nil)
		tAssert.True(validMethod)
		tAssert.True(validParams)
		tAssert.NoError(err)

		location, ok := resultValue.(protocol.Location)
		tAssert.True(ok)
		if !ok {
			return
		}

		tAssert.Equal(uri, location.URI)
		tAssert.Equal(protocol.UInteger(5), location.Range.Start.Line)
		tAssert.Equal(protocol.UInteger(2), location.Range.Start.Character)
	})

	It("prefers current document definitions over imported symbols with matching coordinates", func() {

		workspace, err := os.MkdirTemp("", "mace-lsp-definition-coordinates-*")
		tAssert.NoError(err)

		importPath := writeWorkspaceFile(workspace, "shared.mace", `[output = 'data']
{




       qux: 1,
}`)
		uri := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", `|===|
from './shared.mace' import qux;
int qux = 2;
|===|

{
  bar: qux,
}`))

		didOpen(server, uri, `|===|
from './shared.mace' import qux;
int qux = 2;
|===|

{
  bar: qux,
}`, nil)

		resultValue, validMethod, validParams, err := invoke(server.Handler(), protocol.MethodTextDocumentDefinition, protocol.DefinitionParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri},
				Position:     protocol.Position{Line: 6, Character: 7},
			},
		}, nil)
		tAssert.True(validMethod)
		tAssert.True(validParams)
		tAssert.NoError(err)

		location, ok := resultValue.(protocol.Location)
		tAssert.True(ok)
		if !ok {
			return
		}

		tAssert.Equal(uri, location.URI)
		tAssert.NotEqual(protocol.DocumentUri(importPath), location.URI)
		tAssert.Equal(protocol.UInteger(2), location.Range.Start.Line)
		tAssert.Equal(protocol.UInteger(4), location.Range.Start.Character)
	})

	It("returns code actions for import path fixes", func() {

		workspace, err := os.MkdirTemp("", "mace-lsp-code-action-*")
		tAssert.NoError(err)

		writeWorkspaceFile(workspace, "shared.mace", `[output = 'data'] { name: "Ada", }`)
		uri := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", `|===|
from './shared' import name;
|===|
[output = 'data']
{
  result: name,
}`))

		didOpen(server, uri, `|===|
from './shared' import name;
|===|
[output = 'data']
{
  result: name,
}`, nil)

		resultValue, validMethod, validParams, err := invoke(server.Handler(), protocol.MethodTextDocumentCodeAction, protocol.CodeActionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Range: protocol.Range{
				Start: protocol.Position{Line: 1, Character: 0},
				End:   protocol.Position{Line: 1, Character: 20},
			},
			Context: protocol.CodeActionContext{},
		}, nil)
		tAssert.True(validMethod)
		tAssert.True(validParams)
		tAssert.NoError(err)

		actions, ok := resultValue.([]protocol.CodeAction)
		tAssert.True(ok)
		if !ok || !tAssert.NotEmpty(actions) {
			return
		}

		tAssert.Equal("Append .mace to import path", actions[0].Title)
	})

	It("returns code actions for schema and schema_file conflicts", func() {

		workspace, err := os.MkdirTemp("", "mace-lsp-schema-file-conflict-*")
		tAssert.NoError(err)

		uri := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", `|===|
from './shared.mace' import User;
schema User: { name: string, };
|===|
[output = 'data', schema = User, schema_file = './shared.mace']
{
  result: { name: "Ada", },
}`))

		didOpen(server, uri, `|===|
from './shared.mace' import User;
schema User: { name: string, };
|===|
[output = 'data', schema = User, schema_file = './shared.mace']
{
  result: { name: "Ada", },
}`, nil)

		resultValue, validMethod, validParams, err := invoke(server.Handler(), protocol.MethodTextDocumentCodeAction, protocol.CodeActionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Range: protocol.Range{
				Start: protocol.Position{Line: 4, Character: 0},
				End:   protocol.Position{Line: 4, Character: 60},
			},
			Context: protocol.CodeActionContext{},
		}, nil)
		tAssert.True(validMethod)
		tAssert.True(validParams)
		tAssert.NoError(err)

		actions, ok := resultValue.([]protocol.CodeAction)
		tAssert.True(ok)
		if !ok || !tAssert.Len(actions, 2) {
			return
		}

		tAssert.Equal("Remove schema_file directive", actions[0].Title)
		tAssert.Equal("Remove imports and script block", actions[1].Title)
	})

	It("does not rename unrelated field keys", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
string name = "Ada";
|===|
[output = 'data']
{
  name: { name: name, },
}`, nil)

		resultValue, validMethod, validParams, err := invoke(server.Handler(), protocol.MethodTextDocumentRename, protocol.RenameParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri},
				Position:     protocol.Position{Line: 5, Character: 17},
			},
			NewName: "username",
		}, nil)
		tAssert.True(validMethod)
		tAssert.True(validParams)
		tAssert.NoError(err)

		edit, ok := resultValue.(*protocol.WorkspaceEdit)
		tAssert.True(ok)
		if !ok || !tAssert.NotNil(edit) {
			return
		}
		edits := edit.Changes[uri]
		tAssert.Len(edits, 2)
		for _, edit := range edits {
			tAssert.NotEqual(protocol.UInteger(2), edit.Range.Start.Character)
			tAssert.NotEqual(protocol.UInteger(10), edit.Range.Start.Character)
		}
	})

	It("prepares rename on the local imported symbol usage range", func() {
		workspace, err := os.MkdirTemp("", "mace-lsp-prepare-import-*")
		tAssert.NoError(err)
		writeWorkspaceFile(workspace, "shared.mace", `[output = 'schema']
{
  User: { name: string, },
}`)
		consumerURI := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", ``))
		openEmptyDocument(server, consumerURI, nil)
		didChange(server, consumerURI, 2, `|===|
from './shared.mace' import User;
|===|
[output = 'data', schema = User]
{
  result: { name: "Ada", },
}`, nil)

		resultValue, validMethod, validParams, err := invoke(server.Handler(), protocol.MethodTextDocumentPrepareRename, protocol.PrepareRenameParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: consumerURI},
				Position:     protocol.Position{Line: 3, Character: 27},
			},
		}, nil)
		tAssert.True(validMethod)
		tAssert.True(validParams)
		tAssert.NoError(err)

		expectedRange := protocol.Range{Start: protocol.Position{Line: 3, Character: 27}, End: protocol.Position{Line: 3, Character: 31}}
		switch rangeValue := resultValue.(type) {
		case protocol.Range:
			tAssert.Equal(expectedRange, rangeValue)
		case *protocol.Range:
			tAssert.Equal(expectedRange, *rangeValue)
		default:
			tAssert.Failf("unexpected prepare rename result", "%T", resultValue)
		}
	})

	It("renames local variables from a usage", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
string name = "Ada";
string greeting = name;
|===|
[output = 'data']
{
  result: name,
}`, nil)

		resultValue, validMethod, validParams, err := invoke(server.Handler(), protocol.MethodTextDocumentRename, protocol.RenameParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri},
				Position:     protocol.Position{Line: 6, Character: 11},
			},
			NewName: "username",
		}, nil)
		tAssert.True(validMethod)
		tAssert.True(validParams)
		tAssert.NoError(err)

		edit, ok := resultValue.(*protocol.WorkspaceEdit)
		tAssert.True(ok)
		if !ok || !tAssert.NotNil(edit) {
			return
		}
		edits := edit.Changes[uri]
		tAssert.Len(edits, 3)
	})

	It("renames imported symbols and exported keys", func() {
		workspace, err := os.MkdirTemp("", "mace-lsp-rename-import-*")
		tAssert.NoError(err)
		writeWorkspaceFile(workspace, "shared.mace", `[output = 'schema']
{
  User: { name: string, },
}`)
		consumerPath := writeWorkspaceFile(workspace, "consumer.mace", `|===|
from './shared.mace' import User;
|===|
[output = 'data', schema = User]
{
  result: { name: "Ada", },
}`)
		consumerURI := protocol.DocumentUri(consumerPath)
		openEmptyDocument(server, consumerURI, nil)
		didChange(server, consumerURI, 2, `|===|
from './shared.mace' import User;
|===|
[output = 'data', schema = User]
{
  result: { name: "Ada", },
}`, nil)

		resultValue, validMethod, validParams, err := invoke(server.Handler(), protocol.MethodTextDocumentRename, protocol.RenameParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: consumerURI},
				Position:     protocol.Position{Line: 1, Character: 29},
			},
			NewName: "Person",
		}, nil)
		tAssert.True(validMethod)
		tAssert.True(validParams)
		tAssert.NoError(err)

		edit, ok := resultValue.(*protocol.WorkspaceEdit)
		tAssert.True(ok)
		if !ok || !tAssert.NotNil(edit) {
			return
		}
		tAssert.NotEmpty(edit.Changes[consumerURI])
		sharedEdits := []protocol.TextEdit{}
		for uri, edits := range edit.Changes {
			if strings.Contains(string(uri), "shared.mace") {
				sharedEdits = edits
			}
		}
		tAssert.NotEmpty(sharedEdits)
	})

	It("renames import aliases without renaming matching output keys", func() {
		workspace, err := os.MkdirTemp("", "mace-lsp-rename-import-alias-*")
		tAssert.NoError(err)
		writeWorkspaceFile(workspace, "shared.mace", `[output = 'schema']
{
  Scripts: { name: string, },
}`)
		consumerPath := writeWorkspaceFile(workspace, "consumer.mace", `|===|
from './shared.mace' import Scripts:MyScripts;
|===|
[output = 'schema']
{
  scripts: MyScripts,
  Scripts: { name: string, },
}`)
		consumerURI := protocol.DocumentUri(consumerPath)
		openEmptyDocument(server, consumerURI, nil)
		didChange(server, consumerURI, 2, `|===|
from './shared.mace' import Scripts:MyScripts;
|===|
[output = 'schema']
{
  scripts: MyScripts,
  Scripts: { name: string, },
}`, nil)

		resultValue, validMethod, validParams, err := invoke(server.Handler(), protocol.MethodTextDocumentRename, protocol.RenameParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: consumerURI},
				Position:     protocol.Position{Line: 5, Character: 11},
			},
			NewName: "UserScripts",
		}, nil)
		tAssert.True(validMethod)
		tAssert.True(validParams)
		tAssert.NoError(err)

		edit, ok := resultValue.(*protocol.WorkspaceEdit)
		tAssert.True(ok)
		if !ok || !tAssert.NotNil(edit) {
			return
		}

		edits := edit.Changes[consumerURI]
		tAssert.Len(edits, 2)
		newTexts := lo.Map(edits, func(edit protocol.TextEdit, _ int) string {
			return edit.NewText
		})
		tAssert.Equal([]string{"UserScripts", "UserScripts"}, newTexts)
		for _, edit := range edits {
			tAssert.NotEqual(protocol.Position{Line: 6, Character: 2}, edit.Range.Start)
			tAssert.NotEqual(protocol.Position{Line: 5, Character: 2}, edit.Range.Start)
		}
		tAssert.Len(edit.Changes, 1)
	})

	It("returns hierarchical document symbols", func() {
		didOpen(server, uri, `|===|
schema User: { name: string, age?: int, };
string env = "dev";
|===|
[output = 'data']
{
  result: env,
}`, nil)

		resultValue, validMethod, validParams, err := invoke(server.Handler(), protocol.MethodTextDocumentDocumentSymbol, protocol.DocumentSymbolParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
		}, nil)
		tAssert.True(validMethod)
		tAssert.True(validParams)
		tAssert.NoError(err)

		symbols, ok := resultValue.([]protocol.DocumentSymbol)
		tAssert.True(ok)
		if !ok {
			return
		}

		if tAssert.Len(symbols, 3) {
			tAssert.Equal("User", symbols[0].Name)
			tAssert.Equal("env", symbols[1].Name)
			tAssert.Equal("output", symbols[2].Name)
			tAssert.NotEmpty(symbols[0].Children)
			tAssert.NotEmpty(symbols[2].Children)
		}
	})

	It("includes choice details in hierarchical document symbols", func() {
		didOpen(server, uri, `|===|
 alias Flavor: choice["Vanilla", "Chocolate"];
|===|
[output = 'data']
{
  result: "Vanilla",
}`, nil)

		resultValue, validMethod, validParams, err := invoke(server.Handler(), protocol.MethodTextDocumentDocumentSymbol, protocol.DocumentSymbolParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
		}, nil)
		tAssert.True(validMethod)
		tAssert.True(validParams)
		tAssert.NoError(err)

		symbols, ok := resultValue.([]protocol.DocumentSymbol)
		tAssert.True(ok)
		if !ok || !tAssert.NotEmpty(symbols) {
			return
		}

		tAssert.Equal("Flavor", symbols[0].Name)
		tAssert.Equal(protocol.SymbolKindClass, symbols[0].Kind)
		tAssert.Equal(`choice["Vanilla", "Chocolate"]`, lo.FromPtr(symbols[0].Detail))
	})

	It("publishes errors for script variables in schema output mode", func() {
		notifications := []capturedNotification{}

		didOpen(server, uri, `|===|
schema User: { name: string, };
string value = "Ada";
|===|
[output = 'schema']
{
  User: User,
}`, &notifications)

		if tAssert.Len(notifications, 1) {
			params := requireDiagnostics(notifications[0])
			tAssert.Len(params.Diagnostics, 1)
			tAssert.Equal(protocol.DiagnosticSeverityError, *params.Diagnostics[0].Severity)
			tAssert.Equal("mace.directive.schema-output-variable-ignored", params.Diagnostics[0].Code.Value)
		}
	})

	It("formats a document into canonical source", func() {
		didOpen(server, uri, `[output = 'data']{result:1+2,}`, nil)

		resultValue, validMethod, validParams, err := invoke(server.Handler(), protocol.MethodTextDocumentFormatting, protocol.DocumentFormattingParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Options: protocol.FormattingOptions{
				protocol.FormattingOptionInsertSpaces: true,
				protocol.FormattingOptionTabSize:      2,
			},
		}, nil)
		tAssert.True(validMethod)
		tAssert.True(validParams)
		tAssert.NoError(err)

		edits, ok := resultValue.([]protocol.TextEdit)
		tAssert.True(ok)
		if !ok {
			return
		}

		if tAssert.Len(edits, 1) {
			tAssert.Equal(`[output = 'data']{result:1+2,}`, edits[0].NewText)
		}
	})

	It("preserves source text while resizing script fences", func() {
		didOpen(server, uri, `|===|
string display_name = "Ada";
|===|
[output = 'data']
{
  result: [{ profile: { name: "Ada", }, }, { profile: { name: "Bob", }, }],
}`, nil)

		resultValue, validMethod, validParams, err := invoke(server.Handler(), protocol.MethodTextDocumentFormatting, protocol.DocumentFormattingParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Options: protocol.FormattingOptions{
				protocol.FormattingOptionInsertSpaces: true,
				protocol.FormattingOptionTabSize:      2,
			},
		}, nil)
		tAssert.True(validMethod)
		tAssert.True(validParams)
		tAssert.NoError(err)

		edits, ok := resultValue.([]protocol.TextEdit)
		tAssert.True(ok)
		if !ok {
			return
		}

		if tAssert.Len(edits, 1) {
			tAssert.Equal(`|============================|
string display_name = "Ada";
|============================|
[output = 'data']
{
  result: [{ profile: { name: "Ada", }, }, { profile: { name: "Bob", }, }],
}`, edits[0].NewText)
		}
	})

	It("builds the lsp command metadata", func() {
		command := newLSPCommand()

		tAssert.Equal("lsp", command.Use)
		tAssert.Contains(command.Short, "language server")
		tAssert.NoError(command.Args(command, nil))
		tAssert.Error(command.Args(command, []string{"extra"}))
	})

	It("resolves workspace and import roots", func() {
		root, err := os.MkdirTemp("", "mace-lsp-roots-*")
		tAssert.NoError(err)
		workspace := filepath.Join(root, "project")
		nestedDocument := filepath.Join(workspace, "src", "main.mace")
		workspaceURI := protocol.DocumentUri(fileURI(workspace))
		rootURIPath := filepath.Join(root, "root")
		rootURI := protocol.DocumentUri(fileURI(rootURIPath))
		rootPath := filepath.Join(root, "root-path")

		tAssert.Equal(workspace, workspaceRootDir(&protocol.InitializeParams{
			WorkspaceFolders: []protocol.WorkspaceFolder{{URI: workspaceURI}},
			RootURI:          &rootURI,
			RootPath:         &rootPath,
		}))
		tAssert.Equal(rootURIPath, workspaceRootDir(&protocol.InitializeParams{
			RootURI:  &rootURI,
			RootPath: &rootPath,
		}))
		tAssert.Equal(rootPath, workspaceRootDir(&protocol.InitializeParams{
			RootPath: &rootPath,
		}))

		server.workspaceRootDir = workspace
		tAssert.Equal(workspace, server.importRootDir(nestedDocument))
		tAssert.Equal(filepath.Dir(filepath.Join("elsewhere", "main.mace")), server.importRootDir(filepath.Join("elsewhere", "main.mace")))
		tAssert.Equal(workspace, server.importRootDir(""))

		server.workspaceRootDir = ""
		tAssert.Equal(filepath.Dir(nestedDocument), server.importRootDir(nestedDocument))
		tAssert.Equal(".", server.importRootDir(""))
	})

	It("ignores unsupported document change payloads", func() {
		server.documents[protocol.DocumentUri(uri)] = document{text: `[output = 'data'] {}`}

		err := server.didChange(&glsp.Context{}, &protocol.DidChangeTextDocumentParams{
			TextDocument: protocol.VersionedTextDocumentIdentifier{
				Version: 2,
				TextDocumentIdentifier: protocol.TextDocumentIdentifier{
					URI: protocol.DocumentUri(uri),
				},
			},
			ContentChanges: []any{"unsupported"},
		})

		tAssert.NoError(err)
	})

	It("accepts save notifications with and without explicit text", func() {
		root, err := os.MkdirTemp("", "mace-lsp-save-*")
		tAssert.NoError(err)
		path := filepath.Join(root, "document.mace")
		err = os.WriteFile(path, []byte(`[output = 'data'] {}`), 0o600)
		tAssert.NoError(err)
		uriValue := protocol.DocumentUri(fileURI(path))
		didOpen(server, uriValue, `[output = 'data'] {}`, nil)
		didSave(server, uriValue, nil, nil)
		saved := `|===|
int value = 1;
|===|
[output = 'data']
{ value: value, }`
		didSave(server, uriValue, &saved, nil)
	})

	It("returns empty completion results for unopened documents", func() {
		labels := completeLabels(server, protocol.DocumentUri(uri), 1, 1)
		tAssert.Empty(labels)
	})

	It("handles unsupported json-rpc methods", func() {
		_, err := server.handle(context.Background(), nil, &jsonrpc2.Request{
			Method: "mace/unknown",
		})

		tAssert.Error(err)
		var rpcError *jsonrpc2.Error
		tAssert.ErrorAs(err, &rpcError)
		if rpcError != nil {
			tAssert.Equal(int64(jsonrpc2.CodeMethodNotFound), rpcError.Code)
		}
	})

	It("returns method not found for unknown requests", func() {
		_, err := uninitializedServer.handle(context.Background(), nil, &jsonrpc2.Request{Method: "mace/unknown"})
		tAssert.Error(err)
	})

	It("returns invalid params errors for malformed requests", func() {
		params := json.RawMessage(`[]`)

		_, err := server.handle(context.Background(), nil, &jsonrpc2.Request{
			Method: protocol.MethodTextDocumentDidOpen,
			Params: &params,
		})
		tAssert.Error(err)
		var rpcError *jsonrpc2.Error
		tAssert.ErrorAs(err, &rpcError)
		if rpcError != nil {
			tAssert.Equal(int64(jsonrpc2.CodeInvalidParams), rpcError.Code)
		}
	})

	It("runs the language server command until exit", func() {
		previousStdin := os.Stdin
		previousStdout := os.Stdout
		defer func() {
			os.Stdin = previousStdin
			os.Stdout = previousStdout
		}()

		stdinRead, stdinWrite, err := os.Pipe()
		tAssert.NoError(err)
		stdoutRead, stdoutWrite, err := os.Pipe()
		tAssert.NoError(err)

		os.Stdin = stdinRead
		os.Stdout = stdoutWrite

		drained := make(chan struct{})
		go func() {
			_, _ = io.Copy(io.Discard, stdoutRead)
			close(drained)
		}()

		done := make(chan error, 1)
		go func() {
			command := newLSPCommand()
			command.SetArgs([]string{})
			done <- command.Execute()
		}()

		payload := `{"jsonrpc":"2.0","method":"exit"}`
		_, err = fmt.Fprintf(stdinWrite, "Content-Length: %d\r\n\r\n%s", len(payload), payload)
		tAssert.NoError(err)
		tAssert.NoError(stdinWrite.Close())

		select {
		case err := <-done:
			tAssert.NoError(err)
		case <-time.After(5 * time.Second):
			tAssert.Fail("lsp command did not exit")
		}

		select {
		case <-drained:
		case <-time.After(5 * time.Second):
			tAssert.Fail("stdout was not drained")
		}
	})

	It("covers remaining LSP server branches", func() {
		uriValue := protocol.DocumentUri(uri)
		server.documents[uriValue] = document{
			text:    `[output = 'data'] { value: 1, }`,
			version: 1,
		}

		textEdit := protocol.TextDocumentContentChangeEventWhole{Text: `[output = 'data'] { value: 2, }`}
		err := server.didChange(&glsp.Context{}, &protocol.DidChangeTextDocumentParams{
			TextDocument: protocol.VersionedTextDocumentIdentifier{
				Version:                2,
				TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: uriValue},
			},
			ContentChanges: []any{textEdit},
		})
		tAssert.NoError(err)

		err = server.didChange(&glsp.Context{}, &protocol.DidChangeTextDocumentParams{
			TextDocument: protocol.VersionedTextDocumentIdentifier{
				Version:                3,
				TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: uriValue},
			},
			ContentChanges: []any{protocol.TextDocumentContentChangeEvent{
				Range: &protocol.Range{
					Start: protocol.Position{Line: 0, Character: 20},
					End:   protocol.Position{Line: 0, Character: 1},
				},
				Text: "x",
			}},
		})
		tAssert.ErrorContains(err, "invalid text change range")

		err = server.didChange(&glsp.Context{}, &protocol.DidChangeTextDocumentParams{
			TextDocument: protocol.VersionedTextDocumentIdentifier{
				Version:                4,
				TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: uriValue},
			},
			ContentChanges: []any{protocol.TextDocumentContentChangeEvent{Text: `[output = 'data'] { value: 3, }`}},
		})
		tAssert.NoError(err)

		err = server.didChange(&glsp.Context{}, &protocol.DidChangeTextDocumentParams{
			TextDocument: protocol.VersionedTextDocumentIdentifier{
				Version:                5,
				TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: uriValue},
			},
			ContentChanges: []any{protocol.TextDocumentContentChangeEvent{Text: `[output = 'data'] { value: 4, }`}},
		})
		tAssert.NoError(err)

		err = server.didChange(&glsp.Context{}, &protocol.DidChangeTextDocumentParams{
			TextDocument: protocol.VersionedTextDocumentIdentifier{
				Version:                6,
				TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: uriValue},
			},
			ContentChanges: []any{protocol.TextDocumentContentChangeEvent{
				Range: &protocol.Range{
					Start: protocol.Position{Line: 0, Character: 0},
					End:   protocol.Position{Line: 0, Character: 0},
				},
				Text: "(",
			}},
		})
		tAssert.NoError(err)

		updatedDocument, ok := server.document(uriValue)
		tAssert.True(ok)
		tAssert.True(strings.HasPrefix(updatedDocument.text, "("))

		err = server.didChange(&glsp.Context{}, &protocol.DidChangeTextDocumentParams{
			TextDocument: protocol.VersionedTextDocumentIdentifier{
				Version:                7,
				TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: uriValue},
			},
			ContentChanges: []any{
				protocol.TextDocumentContentChangeEvent{
					Range: &protocol.Range{
						Start: protocol.Position{Line: 0, Character: 20},
						End:   protocol.Position{Line: 0, Character: 1},
					},
					Text: "x",
				},
				protocol.TextDocumentContentChangeEventWhole{Text: "ignored"},
			},
		})
		tAssert.ErrorContains(err, "invalid text change range")

		rawParams := json.RawMessage(fmt.Sprintf(`{"textDocument":{"uri":%q,"version":8},"contentChanges":[{"range":{"start":{"line":0,"character":20},"end":{"line":0,"character":1}},"text":"x"}]}`, string(uriValue)))
		_, err = server.handle(context.Background(), nil, &jsonrpc2.Request{Method: protocol.MethodTextDocumentDidChange, Params: &rawParams})
		tAssert.Error(err)
		var rpcError *jsonrpc2.Error
		tAssert.ErrorAs(err, &rpcError)
		if rpcError != nil {
			tAssert.Equal(int64(jsonrpc2.CodeInvalidRequest), rpcError.Code)
		}

		missingURI := protocol.DocumentUri("file:///workspace/missing.mace")
		tAssert.NoError(server.didSave(&glsp.Context{}, &protocol.DidSaveTextDocumentParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: missingURI},
		}))

		missingFileURI := protocol.DocumentUri(fileURI(filepath.Join("missing", "document.mace")))
		server.documents[missingFileURI] = document{text: `[output = 'data'] {}`, version: 1}
		err = server.didSave(&glsp.Context{}, &protocol.DidSaveTextDocumentParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: missingFileURI},
		})
		tAssert.Error(err)

		result, err := server.documentSymbols(nil, &protocol.DocumentSymbolParams{TextDocument: protocol.TextDocumentIdentifier{URI: missingURI}})
		tAssert.NoError(err)
		tAssert.Empty(result)

		formatResult, err := server.formatDocument(nil, &protocol.DocumentFormattingParams{TextDocument: protocol.TextDocumentIdentifier{URI: missingURI}})
		tAssert.NoError(err)
		tAssert.Empty(formatResult)

		codeActions, err := server.codeActions(nil, &protocol.CodeActionParams{TextDocument: protocol.TextDocumentIdentifier{URI: missingURI}})
		tAssert.NoError(err)
		tAssert.Empty(codeActions)

		definition, err := server.definition(nil, &protocol.DefinitionParams{TextDocumentPositionParams: protocol.TextDocumentPositionParams{TextDocument: protocol.TextDocumentIdentifier{URI: missingURI}}})
		tAssert.NoError(err)
		tAssert.Nil(definition)

		renameURI := protocol.DocumentUri(fileURI(filepath.Join("workspace", "rename.mace")))
		server.documents[renameURI] = document{text: `[output = 'data'] { value: 1, }`, version: 1}
		definition, err = server.definition(nil, &protocol.DefinitionParams{TextDocumentPositionParams: protocol.TextDocumentPositionParams{TextDocument: protocol.TextDocumentIdentifier{URI: renameURI}, Position: protocol.Position{Line: 0, Character: 0}}})
		tAssert.NoError(err)
		tAssert.Nil(definition)

		prepareRename, err := server.prepareRename(nil, &protocol.PrepareRenameParams{TextDocumentPositionParams: protocol.TextDocumentPositionParams{TextDocument: protocol.TextDocumentIdentifier{URI: missingURI}}})
		tAssert.NoError(err)
		tAssert.Nil(prepareRename)

		prepareRename, err = server.prepareRename(nil, &protocol.PrepareRenameParams{TextDocumentPositionParams: protocol.TextDocumentPositionParams{TextDocument: protocol.TextDocumentIdentifier{URI: renameURI}, Position: protocol.Position{Line: 0, Character: 0}}})
		tAssert.NoError(err)
		tAssert.Nil(prepareRename)

		rename, err := server.rename(nil, &protocol.RenameParams{TextDocumentPositionParams: protocol.TextDocumentPositionParams{TextDocument: protocol.TextDocumentIdentifier{URI: missingURI}}, NewName: "value"})
		tAssert.NoError(err)
		tAssert.Nil(rename)

		rename, err = server.rename(nil, &protocol.RenameParams{TextDocumentPositionParams: protocol.TextDocumentPositionParams{TextDocument: protocol.TextDocumentIdentifier{URI: renameURI}, Position: protocol.Position{Line: 0, Character: 0}}, NewName: "value"})
		tAssert.NoError(err)
		tAssert.Nil(rename)

		context := &glsp.Context{}
		server.publishDiagnostics(context, missingURI)
		server.notifyDiagnostics(context, protocol.PublishDiagnosticsParams{URI: missingURI})
		server.publishDiagnostics(context, renameURI)

		position := positionFromIndex("a\nb", 2)
		tAssert.Equal(protocol.Position{Line: 1, Character: 0}, position)
		position = positionFromIndex("a\nb", 1)
		tAssert.Equal(protocol.Position{Line: 0, Character: 1}, position)
		position = positionFromIndex("a😀b", len("a😀"))
		tAssert.Equal(protocol.Position{Line: 0, Character: 3}, position)
	})

	It("returns initialized results through the json-rpc bridge", func() {
		params := json.RawMessage(`{}`)

		result, err := server.handle(context.Background(), nil, &jsonrpc2.Request{
			Method: protocol.MethodInitialize,
			Params: &params,
		})
		tAssert.NoError(err)
		tAssert.NotNil(result)
	})

	It("returns invalid params without a parse error when request params are missing", func() {
		_, err := server.handle(context.Background(), nil, &jsonrpc2.Request{
			Method: protocol.MethodTextDocumentDidOpen,
		})
		tAssert.Error(err)
		var rpcError *jsonrpc2.Error
		tAssert.ErrorAs(err, &rpcError)
		if rpcError != nil {
			tAssert.Equal(int64(jsonrpc2.CodeInvalidParams), rpcError.Code)
		}
	})

	It("forwards notifications through the json-rpc bridge", func() {
		left, right := net.Pipe()
		defer func() { tAssert.NoError(left.Close()) }()
		defer func() { tAssert.NoError(right.Close()) }()

		connection := jsonrpc2.NewConn(context.Background(), jsonrpc2.NewBufferedStream(left, jsonrpc2.VSCodeObjectCodec{}), nil)
		defer func() { tAssert.NoError(connection.Close()) }()

		go func() {
			_, _ = io.Copy(io.Discard, right)
		}()

		params := json.RawMessage(fmt.Sprintf(`{"textDocument":{"uri":%q,"languageId":"mace","version":1,"text":"[output = 'data'] { value: 1, }"}}`, uri))
		_, err := server.handle(context.Background(), connection, &jsonrpc2.Request{
			Method: protocol.MethodTextDocumentDidOpen,
			Params: &params,
		})
		tAssert.NoError(err)
	})

	It("returns stdin close errors from stdrwc", func() {
		previousStdin := os.Stdin
		previousStdout := os.Stdout
		defer func() {
			os.Stdin = previousStdin
			os.Stdout = previousStdout
		}()

		stdinFile, err := os.CreateTemp("", "mace-stdin-close-*")
		tAssert.NoError(err)
		stdoutFile, err := os.CreateTemp("", "mace-stdout-close-*")
		tAssert.NoError(err)

		os.Stdin = stdinFile
		os.Stdout = stdoutFile

		tAssert.NoError(stdinFile.Close())
		tAssert.Error(stdrwc{}.Close())
		tAssert.NoError(stdoutFile.Close())
	})

	It("loads saved document text fallbacks and file errors", func() {
		saved := `[output = 'data'] {}`
		text, err := savedDocumentText(&saved, protocol.DocumentUri(uri), "fallback")
		tAssert.NoError(err)
		tAssert.Equal(saved, text)

		text, err = savedDocumentText(nil, protocol.DocumentUri("not a uri"), "fallback")
		tAssert.NoError(err)
		tAssert.Equal("fallback", text)

		_, err = savedDocumentText(nil, protocol.DocumentUri(fileURI(filepath.Join("missing", "document.mace"))), "fallback")
		tAssert.Error(err)
	})
})
