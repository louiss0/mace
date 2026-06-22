package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

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

	leaf := `{ final: 9; }`
	for index := len(keys) - 1; index >= 0; index-- {
		leaf = fmt.Sprintf("{ %s: %s; }", keys[index], leaf)
	}

	return fmt.Sprintf(`[output = data]
{
  tree: %s;
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

		didOpen(server, uri, `[output = data] { result: 1 + 2; }`, &notifications)

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

		writeWorkspaceFile(workspace, "shared.mace", `[output = schema]
{
  Name: string;
}`)
		uri := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", `|===|
from "./shared.mace" import Name;
Name user = "Ada";
|===|
[output = data]
{
  user: user;
}`))

		didOpen(server, uri, `|===|
from "./shared.mace" import Name;
Name user = "Ada";
|===|
[output = data]
{
  user: user;
}`, &notifications)

		if tAssert.Len(notifications, 1) {
			params := requireDiagnostics(notifications[0])
			tAssert.Empty(params.Diagnostics)
		}
	})

	It("publishes syntax diagnostics when an invalid document opens", func() {
		notifications := []capturedNotification{}

		didOpen(server, uri, `[output = data] { result: ; }`, &notifications)

		if tAssert.Len(notifications, 1) {
			params := requireDiagnostics(notifications[0])
			tAssert.Len(params.Diagnostics, 1)
			tAssert.Contains(params.Diagnostics[0].Message, "parser:")
			tAssert.Equal(protocol.DiagnosticSeverityError, *params.Diagnostics[0].Severity)
			tAssert.NotNil(params.Diagnostics[0].Code)
		}
	})

	It("publishes processor diagnostics for invalid variable declarations", func() {
		notifications := []capturedNotification{}

		didOpen(server, uri, `|===|
int count = "Ada";
|===|
[output = data]
{
  result: count;
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
schema EmailLogin: { email: string; };
schema ApiKeyLogin: { api_key: string; };
type Login: variant[EmailLogin, ApiKeyLogin];
Login login = {
  email: "ada@example.com";
  api_key: "secret";
};
|===|
[output = data]
{
  result: login;
}`, &notifications)

		if tAssert.Len(notifications, 1) {
			params := requireDiagnostics(notifications[0])
			tAssert.Len(params.Diagnostics, 1)
			tAssert.Contains(params.Diagnostics[0].Message, `processor: type mismatch: expected variant[EmailLogin, ApiKeyLogin], got record`)
			tAssert.NotNil(params.Diagnostics[0].Code)
		}
	})

	It("publishes processor diagnostics for invalid union declarations", func() {
		notifications := []capturedNotification{}

		didOpen(server, uri, `|===|
type Broken: union[string, int];
|===|
[output = data] {}`, &notifications)

		if tAssert.Len(notifications, 1) {
			params := requireDiagnostics(notifications[0])
			tAssert.Len(params.Diagnostics, 1)
			tAssert.Contains(params.Diagnostics[0].Message, `processor: union members must be schemas`)
			tAssert.NotNil(params.Diagnostics[0].Code)
		}
	})

	It("publishes variable mismatch diagnostics for the failing declaration", func() {
		notifications := []capturedNotification{}

		didOpen(server, uri, `|===|
string name = "Ada";
int count = "seven";
|===|
[output = data]
{
  result: name;
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

		didOpen(server, uri, `[output = data] { result: ; }`, &notifications)
		didChange(server, uri, 2, `[output = data] { result: 3; }`, &notifications)

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
[output = data]
{
  result: count;
}`, &notifications)
		didChange(server, uri, 2, `|===|
int count = 7;
|===|
[output = data]
{
  result: count;
}`, &notifications)

		if tAssert.Len(notifications, 2) {
			params := requireDiagnostics(notifications[1])
			tAssert.Empty(params.Diagnostics)
		}
	})

	It("publishes parse diagnostics when an array index is not an int literal", func() {
		notifications := []capturedNotification{}

		didOpen(server, uri, `[output = data]
{
  values: [1, 2, 3],
  result: values[name]
}`, &notifications)

		if tAssert.Len(notifications, 1) {
			params := requireDiagnostics(notifications[0])
			if tAssert.Len(params.Diagnostics, 1) {
				tAssert.Contains(params.Diagnostics[0].Message, `expected integer index in array access`)
				tAssert.Equal(`mace.syntax.invalid-array-access-index`, params.Diagnostics[0].Code.Value)
			}
		}
	})

	It("publishes diagnostics when an array index is out of range", func() {
		notifications := []capturedNotification{}

		didOpen(server, uri, `|===|
array<int> values = [1, 2, 3];
|===|
[output = data]
{
  result: values[9]
}`, &notifications)

		if tAssert.Len(notifications, 1) {
			params := requireDiagnostics(notifications[0])
			if tAssert.Len(params.Diagnostics, 1) {
				tAssert.Contains(params.Diagnostics[0].Message, `array index 9 is out of range`)
				tAssert.Equal(`mace.type.invalid-array-access`, params.Diagnostics[0].Code.Value)
			}
		}
	})

	It("binds out-of-range diagnostics to the array index token", func() {
		notifications := []capturedNotification{}

		didOpen(server, uri, `|===|
int count = 9;
array<int> values = [1, 2, 3];
|===|
[output = data]
{
  result: values[9]
}`, &notifications)

		if tAssert.Len(notifications, 1) {
			params := requireDiagnostics(notifications[0])
			if tAssert.Len(params.Diagnostics, 1) {
				tAssert.Equal(protocol.UInteger(6), params.Diagnostics[0].Range.Start.Line)
				tAssert.Equal(protocol.UInteger(17), params.Diagnostics[0].Range.Start.Character)
				tAssert.Equal(protocol.UInteger(18), params.Diagnostics[0].Range.End.Character)
			}
		}
	})

	It("binds out-of-range diagnostics to the first failing array access", func() {
		notifications := []capturedNotification{}

		didOpen(server, uri, `|===|
array<int> values = [1, 2, 3];
|===|
[output = data]
{
  first: values[9],
  second: values[9]
}`, &notifications)

		if tAssert.Len(notifications, 1) {
			params := requireDiagnostics(notifications[0])
			if tAssert.Len(params.Diagnostics, 1) {
				tAssert.Equal(protocol.UInteger(5), params.Diagnostics[0].Range.Start.Line)
				tAssert.Equal(protocol.UInteger(16), params.Diagnostics[0].Range.Start.Character)
				tAssert.Equal(protocol.UInteger(17), params.Diagnostics[0].Range.End.Character)
			}
		}
	})

	It("binds invalid array access diagnostics to the first failing expression", func() {
		notifications := []capturedNotification{}

		didOpen(server, uri, `[output = data]
{
  first: 1[0],
  second: 2[0]
}`, &notifications)

		if tAssert.Len(notifications, 1) {
			params := requireDiagnostics(notifications[0])
			if tAssert.Len(params.Diagnostics, 1) {
				tAssert.Equal(protocol.UInteger(2), params.Diagnostics[0].Range.Start.Line)
				tAssert.Equal(protocol.UInteger(10), params.Diagnostics[0].Range.Start.Character)
				tAssert.Equal(protocol.UInteger(11), params.Diagnostics[0].Range.End.Character)
			}
		}
	})

	It("does not report mixed array diagnostics for string arrays", func() {
		notifications := []capturedNotification{}

		didOpen(server, uri, `|===|
array<string> names = ['Kyle', 'Tyrone', 'Luke'];
|===|
[output = data]
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

		path := writeWorkspaceFile(workspace, "consumer.mace", `[output = data] { result: ; }`)
		uri := protocol.DocumentUri(path)

		didOpen(server, uri, `[output = data] { result: ; }`, &notifications)

		fixedText := `[output = data] { result: 3; }`
		err = os.WriteFile(filepath.FromSlash(documentPath(uri)), []byte(fixedText), 0o600)
		tAssert.NoError(err)

		didSave(server, uri, nil, &notifications)

		if tAssert.Len(notifications, 2) {
			params := requireDiagnostics(notifications[1])
			tAssert.Empty(params.Diagnostics)
		}
	})

	It("warns when parse directives inject unknown runtime values", func() {
		notifications := []capturedNotification{}

		didOpen(server, uri, `|===|
schema Package: { name: string; project: string; };
|===|
[output = data, parse = Package]
{
  result: "ok";
}`, &notifications)

		if tAssert.Len(notifications, 1) {
			params := requireDiagnostics(notifications[0])
			if tAssert.Len(params.Diagnostics, 1) {
				tAssert.Contains(params.Diagnostics[0].Message, "The analyzer cannot know which runtime values will be injected")
				tAssert.Equal(protocol.DiagnosticSeverityWarning, *params.Diagnostics[0].Severity)
				tAssert.NotNil(params.Diagnostics[0].Code)
			}
		}
	})

	It("warns when parse_file directives inject unknown runtime values", func() {
		notifications := []capturedNotification{}
		workspace, err := os.MkdirTemp("", "mace-lsp-parse-ignore-*")
		tAssert.NoError(err)

		writeWorkspaceFile(workspace, "runtime.mace", `[output = schema]
{
  Package: { project: string; };
}`)
		uri := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", ``))

		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `[output = data, parse_file = "./runtime.mace"]
{
  result: "ok";
}`, &notifications)

		if tAssert.Len(notifications, 1) {
			params := requireDiagnostics(notifications[0])
			if tAssert.Len(params.Diagnostics, 1) {
				tAssert.Contains(params.Diagnostics[0].Message, "The analyzer cannot know which runtime values will be injected")
				tAssert.Equal(protocol.DiagnosticSeverityWarning, *params.Diagnostics[0].Severity)
				tAssert.NotNil(params.Diagnostics[0].Code)
			}
		}
	})

	It("publishes processor diagnostics for invalid output data structures", func() {
		notifications := []capturedNotification{}

		didOpen(server, uri, `|===|
schema Point: { x: int; y: int; };
schema Plot: { points: array<Point>; };
|===|
[output = data, schema = Plot]
{
  points: [
    { x: 1; y: 2; },
    { x: 3; }
  ];
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
schema Point: { x: int; y: int; };
schema Plot: { points: array<Point>; };
|===|
[output = data, schema = Plot]
{
  points: [
    { x: 1; y: 2; },
    { x: 3; }
  ];
}`, &notifications)
		didChange(server, uri, 2, `|===|
schema Point: { x: int; y: int; };
schema Plot: { points: array<Point>; };
|===|
[output = data, schema = Plot]
{
  points: [
    { x: 1; y: 2; },
    { x: 3; y: 4; }
  ];
}`, &notifications)

		if tAssert.Len(notifications, 2) {
			params := requireDiagnostics(notifications[1])
			tAssert.Empty(params.Diagnostics)
		}
	})

	It("drops document state on close and clears diagnostics", func() {
		notifications := []capturedNotification{}

		didOpen(server, uri, `[output = data] { result: 1; }`, &notifications)
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

		writeWorkspaceFile(workspace, "shared.mace", `[output = schema]
{
  User: { name: string; };
  Config: string;
}`)
		uri := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", `|===|
from "./shared.mace" imp`))

		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
from "./shared.mace" imp`, nil)

		labels := completeLabels(server, uri, 1, uint32(len(`from "./shared.mace" imp`)))
		tAssert.Equal([]string{"import"}, labels)
	})

	It("uses the current directory as the default import path baseline", func() {

		workspace, err := os.MkdirTemp("", "mace-lsp-import-path-*")
		tAssert.NoError(err)

		writeWorkspaceFile(workspace, "shared.mace", `[output = data] { name: "Ada"; }`)
		writeWorkspaceFile(workspace, "nested/roles.mace", `[output = data] { role: "admin"; }`)
		uri := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", ``))

		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
from "`, nil)

		labels := completeLabels(server, uri, 1, uint32(len(`from "`)))
		tAssert.Contains(labels, "./shared.mace")
		tAssert.Contains(labels, "./nested/")
	})

	It("suggests parent relative import paths while the from string changes", func() {

		workspace, err := os.MkdirTemp("", "mace-lsp-parent-import-path-*")
		tAssert.NoError(err)

		writeWorkspaceFile(workspace, "shared.mace", `[output = data] { name: "Ada"; }`)
		consumerURI := protocol.DocumentUri(writeWorkspaceFile(workspace, "nested/consumer.mace", ``))

		openEmptyDocument(server, consumerURI, nil)
		didChange(server, consumerURI, 2, `|===|
from "../`, nil)

		labels := completeLabels(server, consumerURI, 1, uint32(len(`from "../`)))
		tAssert.Contains(labels, "../shared.mace")
	})

	It("suggests import after a valid from path change with trailing space", func() {

		workspace, err := os.MkdirTemp("", "mace-lsp-import-space-*")
		tAssert.NoError(err)

		writeWorkspaceFile(workspace, "shared.mace", `[output = schema]
{
  User: { name: string; };
}`)
		uri := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", ``))

		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
from "./shared.mace" `, nil)

		labels := completeLabels(server, uri, 1, uint32(len(`from "./shared.mace" `)))
		tAssert.Equal([]string{"import"}, labels)
	})

	It("only suggests identifiers exported by the import path after change", func() {

		workspace, err := os.MkdirTemp("", "mace-lsp-imported-*")
		tAssert.NoError(err)

		writeWorkspaceFile(workspace, "shared.mace", `[output = schema]
{
  User: { name: string; };
  Config: string;
}`)
		uri := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", `|===|
from "./shared.mace" import U`))

		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
from "./shared.mace" import U`, nil)

		labels := completeLabels(server, uri, 1, uint32(len(`from "./shared.mace" import U`)))
		tAssert.Equal([]string{"User"}, labels)
	})

	It("suggests all exported identifiers after import changes", func() {

		workspace, err := os.MkdirTemp("", "mace-lsp-imported-all-*")
		tAssert.NoError(err)

		writeWorkspaceFile(workspace, "shared.mace", `[output = schema]
{
  User: { name: string; };
  Config: string;
}`)
		uri := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", ``))

		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
from "./shared.mace" import `, nil)

		labels := completeLabels(server, uri, 1, uint32(len(`from "./shared.mace" import `)))
		tAssert.Equal([]string{"Config", "User"}, labels)
	})

	It("suggests imported identifiers inside the script block", func() {

		workspace, err := os.MkdirTemp("", "mace-lsp-imported-script-*")
		tAssert.NoError(err)

		writeWorkspaceFile(workspace, "shared.mace", `[output = schema]
{
  User: { name: string; };
}`)
		uri := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", ``))

		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
from "./shared.mace" import User;
Us
|===|
[output = data] {}`, nil)

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
		didChange(server, uri, 2, `[output = data]
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
[output = data] {}`, nil)

		labels := completeLabels(server, uri, 1, 2)
		tAssert.Contains(labels, "choice")
	})

	It("suggests choice values for script variable initializers", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
type Fruit: choice["Apple", "Strawberry"];
Fruit favorite =
|===|
[output = data] {}`, nil)

		labels := completeLabels(server, uri, 2, uint32(len(`Fruit favorite =`)))
		tAssert.Contains(labels, `"Apple"`)
		tAssert.Contains(labels, `"Strawberry"`)
	})

	It("suggests unquoted choice values inside script strings", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
type Fruit: choice["Apple", "Strawberry"];
Fruit favorite = "A
|===|
[output = data] {}`, nil)

		labels := completeLabels(server, uri, 2, uint32(len(`Fruit favorite = "A`)))
		tAssert.Contains(labels, "Apple")
		tAssert.NotContains(labels, `"Apple"`)
		tAssert.NotContains(labels, "Strawberry")
	})

	It("suggests choice values for script variable variants", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
type Status: choice["pending", "approved"];
type Label: variant[Status, string];
Label label =
|===|
[output = data] {}`, nil)

		labels := completeLabels(server, uri, 3, uint32(len(`Label label =`)))
		tAssert.Contains(labels, `"approved"`)
		tAssert.Contains(labels, `"pending"`)
	})

	It("suggests choice values for script variable record fields", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
type Fruit: choice["Apple", "Strawberry"];
schema Basket: { favorite_fruit: Fruit; };
Basket basket = {
  favorite_fruit:
};
|===|
[output = data] {}`, nil)

		labels := completeLabels(server, uri, 4, uint32(len(`  favorite_fruit: `)))
		tAssert.Contains(labels, `"Apple"`)
		tAssert.Contains(labels, `"Strawberry"`)
	})

	It("suggests unquoted choice values inside record field strings", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
type Fruit: choice["Apple", "Strawberry"];
schema Basket: { favorite_fruit: Fruit; };
Basket basket = {
  favorite_fruit: "Str
};
|===|
[output = data] {}`, nil)

		labels := completeLabels(server, uri, 4, uint32(len(`  favorite_fruit: "Str`)))
		tAssert.Equal([]string{"Strawberry"}, labels)
	})

	It("suggests unquoted choice values inside array element strings", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
type Fruit: choice["Apple", "Strawberry"];
array<Fruit> favorites = ["A
|===|
[output = data] {}`, nil)

		labels := completeLabels(server, uri, 2, uint32(len(`array<Fruit> favorites = ["A`)))
		tAssert.Contains(labels, "Apple")
		tAssert.NotContains(labels, `"Apple"`)
		tAssert.NotContains(labels, "Strawberry")
	})

	It("suggests array indexes for script variables", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
array<string> names = ["Ada", "Linus", "Grace"];
string selected = names[
|===|
[output = data] {}`, nil)

		labels := completeLabels(server, uri, 2, uint32(len(`string selected = names[`)))
		tAssert.Equal([]string{"0", "1", "2"}, labels)
	})

	It("suggests array indexes for imported values in script variables", func() {
		workspace, err := os.MkdirTemp("", "mace-lsp-imported-array-index-*")
		tAssert.NoError(err)

		writeWorkspaceFile(workspace, "shared.mace", `[output = data]
{
  names: ["Ada", "Linus", "Grace"]
}`)
		uri := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", ``))

		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
from "./shared.mace" import names;
string selected = names[
|===|
[output = data] {}`, nil)

		labels := completeLabels(server, uri, 2, uint32(len(`string selected = names[`)))
		tAssert.Equal([]string{"0", "1", "2"}, labels)
	})

	It("suggests array indexes for local arrays despite unrelated script errors", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
array<string> names = ["Ada", "Linus", "Grace"];
string broken = missing;
string selected = names[
|===|
[output = data] {}`, nil)

		labels := completeLabels(server, uri, 3, uint32(len(`string selected = names[`)))
		tAssert.Equal([]string{"0", "1", "2"}, labels)
	})

	It("suggests array indexes for script arrays in output fields", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
array<string> names = ["Ada", "Linus", "Grace"];
|===|
[output = data]
{
  result: names[
}`, nil)

		labels := completeLabels(server, uri, 5, uint32(len(`  result: names[`)))
		tAssert.Equal([]string{"0", "1", "2"}, labels)
	})

	It("suggests array indexes for imported arrays in output fields", func() {
		workspace, err := os.MkdirTemp("", "mace-lsp-output-imported-array-index-*")
		tAssert.NoError(err)

		writeWorkspaceFile(workspace, "shared.mace", `[output = data]
{
  names: ["Ada", "Linus", "Grace"]
}`)
		uri := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", ``))

		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
from "./shared.mace" import names;
|===|
[output = data]
{
  result: names[
}`, nil)

		labels := completeLabels(server, uri, 5, uint32(len(`  result: names[`)))
		tAssert.Equal([]string{"0", "1", "2"}, labels)
	})

	It("suggests schema record literals for nested schema fields after a record colon", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
schema Profile: { name: string; age?: int; };
schema Basket: { owner: Profile; };
Basket basket = {
  owner:
};
|===|
[output = data] {}`, nil)

		labels := completeLabels(server, uri, 4, uint32(len(`  owner: `)))
		tAssert.Equal([]string{`{ name: "", age?: 0 }`}, labels)
	})

	It("suggests parse schema fields as output variables", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
schema Runtime: { env: string; region: string; };
|===|
[output = data, parse = Runtime]
{
  result:
}`, nil)

		labels := completeLabels(server, uri, 5, uint32(len(`  result: `)))
		tAssert.Contains(labels, "env")
		tAssert.Contains(labels, "region")
	})

	It("suggests parse_file output schema fields as output variables", func() {
		workspace, err := os.MkdirTemp("", "mace-lsp-parse-file-*")
		tAssert.NoError(err)

		writeWorkspaceFile(workspace, "runtime.mace", `[output = schema]
{
  Runtime: { env: string; region: string; };
}`)
		uri := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", ``))

		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `[output = data, parse_file = "./runtime.mace"]
{
  result:
}`, nil)

		labels := completeLabels(server, uri, 2, uint32(len(`  result: `)))
		tAssert.NotContains(labels, "env")
		tAssert.NotContains(labels, "region")
		tAssert.Contains(labels, "Runtime")
	})

	It("only suggests top-level parse schema fields as output variables", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
schema Runtime: {
  env: string;
  profile: { name: string; email: string; };
};
|===|
[output = data, parse = Runtime]
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

		tAssert.Contains(labels, "env")
		tAssert.Contains(labels, "profile")
		tAssert.NotContains(labels, "name")
		tAssert.NotContains(labels, "email")
		tAssert.Equal("string", details["env"])
		tAssert.Equal("{ name: string, email: string }", details["profile"])
	})

	It("only suggests top-level parse_file schema fields as output variables", func() {
		workspace, err := os.MkdirTemp("", "mace-lsp-parse-file-top-level-*")
		tAssert.NoError(err)

		writeWorkspaceFile(workspace, "runtime.mace", `[output = schema]
{
  Runtime: {
    env: string;
    profile: { name: string; email: string; };
  };
}`)
		uri := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", ``))

		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `[output = data, parse_file = "./runtime.mace"]
{
  result:
}`, nil)

		labels := completeLabels(server, uri, 2, uint32(len(`  result: `)))
		tAssert.NotContains(labels, "env")
		tAssert.NotContains(labels, "profile")
		tAssert.NotContains(labels, "name")
		tAssert.NotContains(labels, "email")
		tAssert.Contains(labels, "Runtime")
	})

	It("suggests parse_file output schema field members as output variables", func() {
		workspace, err := os.MkdirTemp("", "mace-lsp-parse-file-members-*")
		tAssert.NoError(err)

		writeWorkspaceFile(workspace, "runtime.mace", `[output = schema]
{
  Runtime: { user: { name: string; home: { street: string; city: string; }; }; };
}`)
		uri := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", ``))

		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `[output = data, parse_file = "./runtime.mace"]
{
  result: user.
}`, nil)

		labels := completeLabels(server, uri, 2, uint32(len(`  result: user.`)))
		tAssert.Contains(labels, "name")
		tAssert.Contains(labels, "home")
	})

	It("completes recursive nested parse values through member access", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
schema Contact: {
  email: string;
  phone: string;
};
schema Profile: {
  title: string;
  contact: Contact;
};
schema User: {
  name: string;
  manager: User;
  profile: Profile;
};
|===|
[output = data, parse = User]
{
  result: manager.manager.profile.contact.
}`, nil)

		labels := completeLabels(server, uri, 17, uint32(len(`  result: manager.manager.profile.contact.`)))
		tAssert.Contains(labels, "email")
		tAssert.Contains(labels, "phone")
	})

	It("only suggests exported parse_file props as output variables", func() {
		workspace, err := os.MkdirTemp("", "mace-lsp-parse-file-exports-*")
		tAssert.NoError(err)

		writeWorkspaceFile(workspace, "nx_inputs.mace", `|===|
schema Project: {
  name: string;
  root: string;
};
schema Workspace: {
  name: string;
  root: string;
};
|===|
[output = schema]
{
  project: Project;
  workspace: Workspace;
}`)
		uri := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", ``))

		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `[output = data, parse_file = "./nx_inputs.mace"]
{
  
}`, nil)

		labels := completeLabels(server, uri, 2, 2)
		tAssert.Contains(labels, "project")
		tAssert.Contains(labels, "workspace")
		tAssert.NotContains(labels, "name")
		tAssert.NotContains(labels, "root")
		tAssert.NotContains(labels, "cwd")
	})

	It("completes members for exported parse_file props", func() {
		workspace, err := os.MkdirTemp("", "mace-lsp-parse-file-export-members-*")
		tAssert.NoError(err)

		writeWorkspaceFile(workspace, "nx_inputs.mace", `|===|
schema Project: {
  name: string;
  root: string;
};
schema Workspace: {
  name: string;
  root: string;
};
|===|
[output = schema]
{
  project: Project;
  workspace: Workspace;
}`)
		uri := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", ``))

		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `[output = data, parse_file = "./nx_inputs.mace"]
{
  result: project.
}`, nil)

		labels := completeLabels(server, uri, 2, uint32(len(`  result: project.`)))
		tAssert.Contains(labels, "name")
		tAssert.Contains(labels, "root")
		tAssert.NotContains(labels, "project")
		tAssert.NotContains(labels, "workspace")
	})

	It("does not suggest schema names as output expressions", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
schema Runtime: { env: string; };
|===|
[output = data]
{
  result:
}`, nil)

		labels := completeLabels(server, uri, 5, uint32(len(`  result: `)))
		tAssert.NotContains(labels, "Runtime")
	})

	It("does not suggest imported schema names as output expressions", func() {
		workspace, err := os.MkdirTemp("", "mace-lsp-output-schema-*")
		tAssert.NoError(err)

		writeWorkspaceFile(workspace, "shared.mace", `[output = schema]
{
  Runtime: { env: string; };
}`)
		uri := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", ``))

		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
from "./shared.mace" import Runtime;
|===|
[output = data]
{
  result:
}`, nil)

		labels := completeLabels(server, uri, 5, uint32(len(`  result: `)))
		tAssert.NotContains(labels, "Runtime")
	})

	It("suggests parse variables when previous output fields use commas", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
schema Runtime: { env: string; region: string; };
|===|
[output = data, parse = Runtime]
{
  name: "mace",
  result:
}`, nil)

		labels := completeLabels(server, uri, 6, uint32(len(`  result: `)))
		tAssert.Contains(labels, "env")
		tAssert.Contains(labels, "region")
	})

	It("suggests parse_file variables when previous output fields use commas", func() {
		workspace, err := os.MkdirTemp("", "mace-lsp-parse-commas-*")
		tAssert.NoError(err)

		writeWorkspaceFile(workspace, "runtime.mace", `[output = schema]
{
  Runtime: { env: string; region: string; };
}`)
		uri := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", ``))

		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `[output = data, parse_file = "./runtime.mace"]
{
  name: "mace",
  result:
}`, nil)

		labels := completeLabels(server, uri, 3, uint32(len(`  result: `)))
		tAssert.NotContains(labels, "env")
		tAssert.NotContains(labels, "region")
		tAssert.Contains(labels, "Runtime")
	})

	It("suggests choice values for output schema fields", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
 type Fruit: choice["Apple", "Strawberry"];
 schema Basket: { favorite_fruit: Fruit; };
|===|
[output = data, schema = Basket]
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
 type Fruit: choice["Apple", "Strawberry"];
 schema Basket: { previous: Fruit; favorite_fruit: Fruit; };
|===|
[output = data, schema = Basket]
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
 type Role: choice["Admin", "Member"];
 schema User: { name: string; };
 type Identity: variant[Role, User];
 schema Envelope: { value: Identity; };
 schema Response: { payload: Envelope; };
|===|
[output = data, schema = Response]
{
  payload: {
    value:
  };
}`, nil)

		labels := completeLabels(server, uri, 10, uint32(len(`    value: `)))
		tAssert.Contains(labels, "$self")
		tAssert.Contains(labels, `"Admin"`)
		tAssert.Contains(labels, `"Member"`)
		tAssert.Contains(labels, `{ name: "" }`)
	})

	It("suggests composed schema literals for nested output union aliases", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
schema Profile: { name: string; };
schema Audit: { created_at: string; };
type User: union[Profile, Audit];
schema Envelope: { value: User; };
schema Response: { payload: Envelope; };
|===|
[output = data, schema = Response]
{
  payload: {
    value:
  };
}`, nil)

		labels := completeLabels(server, uri, 10, uint32(len(`    value: `)))
		tAssert.Contains(labels, "$self")
		tAssert.Contains(labels, `{ name: "", created_at: "" }`)
	})

	It("keeps typed output completions alongside $self in output schema fields", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
 type Fruit: choice["Apple", "Strawberry"];
 schema Basket: { favorite_fruit: Fruit; };
|===|
[output = data, schema = Basket]
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
		didChange(server, uri, 2, `[output = schema, s`, nil)

		labels := completeLabels(server, uri, 0, uint32(len(`[output = schema, s`)))
		tAssert.Empty(labels)
	})

	It("suggests local and imported schemas after schema directive", func() {

		workspace, err := os.MkdirTemp("", "mace-lsp-schema-ref-*")
		tAssert.NoError(err)

		writeWorkspaceFile(workspace, "shared.mace", `[output = schema]
{
  ImportedUser: { name: string; };
}`)
		uri := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", ``))

		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
from "./shared.mace" import ImportedUser;
schema LocalUser: { id: int; };
|===|
[output = data, schema = `, nil)

		labels := completeLabels(server, uri, 4, uint32(len(`[output = data, schema = `)))
		tAssert.Equal([]string{"ImportedUser", "LocalUser"}, labels)
	})

	It("suggests schema files and excludes files already imported", func() {

		workspace, err := os.MkdirTemp("", "mace-lsp-schema-file-*")
		tAssert.NoError(err)

		writeWorkspaceFile(workspace, "shared.mace", `[output = schema]
{
  ImportedUser: { name: string; };
}`)
		writeWorkspaceFile(workspace, "other.mace", `[output = schema]
{
  OtherUser: { name: string; };
}`)
		uri := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", ``))

		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `|===|
from "./shared.mace" import ImportedUser;
|===|
[output = data, schema_file = "`, nil)

		labels := completeLabels(server, uri, 3, uint32(len(`[output = data, schema_file = "`)))
		tAssert.NotContains(labels, "./shared.mace")
		tAssert.Contains(labels, "./other.mace")
	})

	It("suggests $self in an empty output expression", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `[output = data]
{
  base: 1;
  result:
}`, nil)

		labels := completeLabels(server, uri, 3, uint32(len(`  result: `)))
		tAssert.Contains(labels, "$self")
	})

	It("does not suggest previous output fields without self access", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `[output = data]
{
  base: 1;
  profile: { name: "Ada"; };
  result:
}`, nil)

		labels := completeLabels(server, uri, 3, uint32(len(`  result: `)))
		tAssert.NotContains(labels, "base")
		tAssert.NotContains(labels, "profile")
		tAssert.Contains(labels, "$self")
	})

	It("suggests $self after typing a dollar in the output block", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `[output = data]
{
  result: $
}`, nil)

		labels := completeLabels(server, uri, 2, uint32(len(`  result: $`)))
		tAssert.Equal([]string{"$self"}, labels)
	})

	It("filters $self completion by typed prefix in the output block", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `[output = data]
{
  result: $s
}`, nil)

		labels := completeLabels(server, uri, 2, uint32(len(`  result: $s`)))
		tAssert.Equal([]string{"$self"}, labels)
	})

	It("suggests only previously evaluated output fields after $self dot", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `[output = data]
{
  base: 4;
  profile: { name: "Ada"; };
  result: $self.
}`, nil)

		labels := completeLabels(server, uri, 4, uint32(len(`  result: $self.`)))
		tAssert.Equal([]string{"base", "profile"}, labels)
	})

	It("suggests nested keys from previously evaluated self fields", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `[output = data]
{
  profile: { name: "Ada"; details: { age: 30; }; };
  result: $self.profile.
}`, nil)

		labels := completeLabels(server, uri, 3, uint32(len(`  result: $self.profile.`)))
		tAssert.Equal([]string{"details", "name"}, labels)
	})

	It("suggests nested keys from uppercase self paths", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `[output = data]
{
  User: { profile: { age: 30; }; };
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
		didChange(server, uri, 2, `[output = data]
{
  profile: { stats: { base: 2; multiplier: 3; }; };
  summary: {
    stats: {
      total: $self.profile.stats.base * $self.profile.stats.multiplier;
      base: $self.profile.stats.base;
    };
  };
  result: $self.summary.stats.
}`, nil)

		labels := completeLabels(server, uri, 9, uint32(len(`  result: $self.summary.stats.`)))
		tAssert.Equal([]string{"base", "total"}, labels)
	})

	It("suggests recursive keys when nested records reuse self values across multiple places", func() {
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `[output = data]
{
  account: { balance: 10; bonus: 5; };
  ledger: {
    previous: $self.account.balance;
    next: $self.account.balance + $self.account.bonus;
    nested: { delta: $self.account.bonus; };
  };
  result: $self.ledger.
}`, nil)

		labels := completeLabels(server, uri, 8, uint32(len(`  result: $self.ledger.`)))
		tAssert.Equal([]string{"nested", "next", "previous"}, labels)
	})

	It("returns hover documentation for language keywords", func() {
		didOpen(server, uri, `|===|
schema User: { name: string; };
type Identity: variant[string, int];
type UserRecord: union[User, Profile];
schema Profile: { age: int; };
|===|
[output = data] { name: "Ada"; }`, nil)

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
				Position:     protocol.Position{Line: 2, Character: 14},
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
		didOpen(server, uri, `[output = data, schema = User]
{
  result: 1;
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

	It("returns hover details for user declarations", func() {
		didOpen(server, uri, `|===|
string env = "dev";
schema Profile: { name: string; };
schema Audit: { created_at: string; };
type Identity: variant[string, int];
type User: union[Profile, Audit];
Identity id = "Ada";
User user = { name: "Ada"; created_at: "2026-04-09"; };
|===|
[output = data] { result: env; chosen: id; record: user; }`, nil)

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
			tAssert.Contains(content.Value, `union[Profile, Audit] user = record literal`)
		}
	})

	It("includes gen_doc details for choice types in hover", func() {
		didOpen(server, uri, `|===|
 type Flavor: choice["Vanilla", "Chocolate"];
 gen_doc Flavor {
   summary: "Selectable flavor values";
   description: "Use autocomplete to choose a supported flavor.";
 };
 Flavor current = "Vanilla";
|===|
[output = data] { result: current; }`, nil)

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
			tAssert.Contains(content.Value, `type Flavor: choice["Vanilla", "Chocolate"];`)
			tAssert.Contains(content.Value, `Selectable flavor values`)
			tAssert.Contains(content.Value, `Use autocomplete to choose a supported flavor.`)
		}
	})

	It("includes inline type descriptions in hover details when the type is used", func() {
		didOpen(server, uri, `|===|
type UserID: string /# A stable user identifier;
UserID current = "user_1";
|===|
[output = data] { result: current; }`, nil)

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
			tAssert.Contains(content.Value, `type UserID: string;`)
			tAssert.Contains(content.Value, `A stable user identifier`)
		}
	})

	It("includes exported inline descriptions in imported hover details", func() {
		workspace, err := os.MkdirTemp("", "mace-lsp-hover-import-inline-doc-*")
		tAssert.NoError(err)

		writeWorkspaceFile(workspace, "shared.mace", `[output = schema]
{
  User: { name: string; } /# Public user schema;
}`)
		uri := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", `|===|
from "./shared.mace" import User;
User current = { name: "Ada"; };
|===|
[output = data] { result: current; }`))

		didOpen(server, uri, `|===|
from "./shared.mace" import User;
User current = { name: "Ada"; };
|===|
[output = data] { result: current; }`, nil)

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
schema User: { name: string; };

schema_doc User {
  summary: "Represents a user.";
  description: """
# User
Reusable schema.
""";
  props: {
    name: "The user's display name";
  };
};
|===|
[output = schema]
{ user: User; }`, nil)

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
			tAssert.Contains(content.Value, `Props:`)
			tAssert.Contains(content.Value, "`name`: The user's display name")
		}
	})

	It("loads hover documentation from the docs fixture", func() {
		didOpen(server, uri, `|===|
schema User: {
  name: string;
};

string greeting = "Hello";

gen_doc greeting {
  summary: "Rendered greeting";
};

schema_doc User {
  summary: "Represents a user";
  description: """
# User

Hover should surface this documentation.
""";
  props: {
    name: "The user's display name";
  };
};
|===|
[output = schema]
"""
# User Output
"""
{
  user: User /# Public user schema;
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
			tAssert.Contains(content.Value, `Props:`)
			tAssert.Contains(content.Value, "`name`: The user's display name")
		}
	})

	It("prefers output field hover details over same-named schema declarations", func() {
		didOpen(server, uri, `|===|
schema User: { name: string; };
|===|
[output = data]
{
  User: { name: "Ada"; };
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
type Name: string;
schema Profile: { age: int; };
schema User: { name: Name; profile: Profile; };
Name default_name = "Ada";
int default_age = 30;
|===|
[output = data]
{
  User: {
    name: default_name;
    profile: { age: default_age; };
  };
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
type Name: string;
schema Profile: { age: int; };
schema User: { name: Name; profile: Profile; };
Name default_name = "Ada";
int default_age = 30;
|===|
[output = data]
{
  User: {
    name: default_name;
    profile: { age: default_age; };
  };
  foo: $self.User.profile.age;
  bar: $self.foo;
  baz: ($self.User.name);
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
			tAssert.Contains(content.Value, `output User: { name: string; profile: { age: int; } }`)
			tAssert.Contains(content.Value, `name: "Ada"`)
			tAssert.NotContains(content.Value, "schema User")
		}
	})

	It("returns hover details for nested self references", func() {
		didOpen(server, uri, `[output = data]
{
  User: { profile: { age: 30; }; };
  foo: $self.User.profile.age;
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
		didOpen(server, uri, `[output = data]
{
  summary: {
    stats: {
      totals: { users: 3; };
    };
  };
  result: $self.summary.stats.totals.users;
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
 type Flavor: choice["Vanilla", "Chocolate"];
|===|
[output = schema]
{
  Flavor: Flavor;
}`)
		uri := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", `|===|
from "./shared.mace" import Flavor;
Flavor current = "Vanilla";
|===|
[output = data] { result: current; }`))

		didOpen(server, uri, `|===|
from "./shared.mace" import Flavor;
Flavor current = "Vanilla";
|===|
[output = data] { result: current; }`, nil)

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
			tAssert.Contains(content.Value, `type Flavor: choice["Vanilla", "Chocolate"];`)
		}
	})

	It("returns hover details for imported declarations", func() {

		workspace, err := os.MkdirTemp("", "mace-lsp-hover-import-*")
		tAssert.NoError(err)

		writeWorkspaceFile(workspace, "shared.mace", `[output = schema]
{
  User: { name: string; };
}`)
		uri := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", `|===|
from "./shared.mace" import User;
User current = { name: "Ada"; };
|===|
[output = data] { result: current; }`))

		didOpen(server, uri, `|===|
from "./shared.mace" import User;
User current = { name: "Ada"; };
|===|
[output = data] { result: current; }`, nil)

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

		importPath := writeWorkspaceFile(workspace, "shared.mace", `[output = schema]
{
  User: { name: string; };
}`)
		uri := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", `|===|
from "./shared.mace" import User;
User current = { name: "Ada"; };
|===|
[output = data] { result: current; }`))

		didOpen(server, uri, `|===|
from "./shared.mace" import User;
User current = { name: "Ada"; };
|===|
[output = data] { result: current; }`, nil)

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
schema User: { name: string; };
|===|
[output = data]
{
  User: { name: "Ada"; };
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

		importPath := writeWorkspaceFile(workspace, "shared.mace", `[output = data]
{




       qux: 1;
}`)
		uri := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", `|===|
from "./shared.mace" import qux;
int qux = 2;
|===|

{
  bar: qux;
}`))

		didOpen(server, uri, `|===|
from "./shared.mace" import qux;
int qux = 2;
|===|

{
  bar: qux;
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

		writeWorkspaceFile(workspace, "shared.mace", `[output = data] { name: "Ada"; }`)
		uri := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", `|===|
from "./shared" import name;
|===|
[output = data]
{
  result: name;
}`))

		didOpen(server, uri, `|===|
from "./shared" import name;
|===|
[output = data]
{
  result: name;
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
from "./shared.mace" import User;
schema User: { name: string; };
|===|
[output = data, schema = User, schema_file = "./shared.mace"]
{
  result: { name: "Ada"; };
}`))

		didOpen(server, uri, `|===|
from "./shared.mace" import User;
schema User: { name: string; };
|===|
[output = data, schema = User, schema_file = "./shared.mace"]
{
  result: { name: "Ada"; };
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
[output = data]
{
  name: { name: name; };
}`, nil)

		resultValue, validMethod, validParams, err := invoke(server.Handler(), protocol.MethodTextDocumentRename, protocol.RenameParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri},
				Position:     protocol.Position{Line: 5, Character: 16},
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
		writeWorkspaceFile(workspace, "shared.mace", `[output = schema]
{
  User: { name: string; };
}`)
		consumerURI := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", ``))
		openEmptyDocument(server, consumerURI, nil)
		didChange(server, consumerURI, 2, `|===|
from "./shared.mace" import User;
|===|
[output = data, schema = User]
{
  result: { name: "Ada"; };
}`, nil)

		resultValue, validMethod, validParams, err := invoke(server.Handler(), protocol.MethodTextDocumentPrepareRename, protocol.PrepareRenameParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: consumerURI},
				Position:     protocol.Position{Line: 3, Character: 25},
			},
		}, nil)
		tAssert.True(validMethod)
		tAssert.True(validParams)
		tAssert.NoError(err)

		expectedRange := protocol.Range{Start: protocol.Position{Line: 3, Character: 25}, End: protocol.Position{Line: 3, Character: 29}}
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
[output = data]
{
  result: name;
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
		writeWorkspaceFile(workspace, "shared.mace", `[output = schema]
{
  User: { name: string; };
}`)
		consumerPath := writeWorkspaceFile(workspace, "consumer.mace", `|===|
from "./shared.mace" import User;
|===|
[output = data, schema = User]
{
  result: { name: "Ada"; };
}`)
		consumerURI := protocol.DocumentUri(consumerPath)
		openEmptyDocument(server, consumerURI, nil)
		didChange(server, consumerURI, 2, `|===|
from "./shared.mace" import User;
|===|
[output = data, schema = User]
{
  result: { name: "Ada"; };
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
		writeWorkspaceFile(workspace, "shared.mace", `[output = schema]
{
  Scripts: { name: string; };
}`)
		consumerPath := writeWorkspaceFile(workspace, "consumer.mace", `|===|
from "./shared.mace" import Scripts:MyScripts;
|===|
[output = schema]
{
  scripts: MyScripts;
  Scripts: { name: string; };
}`)
		consumerURI := protocol.DocumentUri(consumerPath)
		openEmptyDocument(server, consumerURI, nil)
		didChange(server, consumerURI, 2, `|===|
from "./shared.mace" import Scripts:MyScripts;
|===|
[output = schema]
{
  scripts: MyScripts;
  Scripts: { name: string; };
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
schema User: { name: string; age?: int; };
string env = "dev";
|===|
[output = data]
{
  result: env;
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
 type Flavor: choice["Vanilla", "Chocolate"];
|===|
[output = data]
{
  result: "Vanilla";
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
schema User: { name: string; };
string value = "Ada";
|===|
[output = schema]
{
  User: User;
}`, &notifications)

		if tAssert.Len(notifications, 1) {
			params := requireDiagnostics(notifications[0])
			tAssert.Len(params.Diagnostics, 1)
			tAssert.Equal(protocol.DiagnosticSeverityError, *params.Diagnostics[0].Severity)
			tAssert.Equal("mace.directive.schema-output-variable-ignored", params.Diagnostics[0].Code.Value)
		}
	})

	It("formats a document into canonical source", func() {
		didOpen(server, uri, `[output = data]{result:1+2;}`, nil)

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
			tAssert.Equal(`[output = data]{result:1+2;}`, edits[0].NewText)
		}
	})

	It("preserves existing spacing while resizing script delimiters", func() {
		didOpen(server, uri, `|===|
string display_name = "Ada";
|===|
[output = data]
{
  result: [{ profile: { name: "Ada"; }; }, { profile: { name: "Bob"; }; }];
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
[output = data]
{
  result: [{ profile: { name: "Ada"; }; }, { profile: { name: "Bob"; }; }];
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

	It("rejects unsupported document change payloads", func() {
		server.documents[protocol.DocumentUri(uri)] = document{text: `[output = data] {}`}

		err := server.didChange(nil, &protocol.DidChangeTextDocumentParams{
			TextDocument: protocol.VersionedTextDocumentIdentifier{
				Version: 2,
				TextDocumentIdentifier: protocol.TextDocumentIdentifier{
					URI: protocol.DocumentUri(uri),
				},
			},
			ContentChanges: []any{"unsupported"},
		})

		tAssert.ErrorContains(err, "unsupported text change payload")
	})

	It("accepts save notifications with and without explicit text", func() {
		root, err := os.MkdirTemp("", "mace-lsp-save-*")
		tAssert.NoError(err)
		path := filepath.Join(root, "document.mace")
		err = os.WriteFile(path, []byte(`[output = data] {}`), 0o600)
		tAssert.NoError(err)
		uriValue := protocol.DocumentUri(fileURI(path))
		didOpen(server, uriValue, `[output = data] {}`, nil)
		didSave(server, uriValue, nil, nil)
		saved := `|===|
int value = 1;
|===|
[output = data]
{ value: value; }`
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
		params := json.RawMessage(`{"textDocument":{}}`)
		left, right := net.Pipe()
		defer left.Close()
		defer right.Close()
		connection := jsonrpc2.NewConn(context.Background(), jsonrpc2.NewBufferedStream(left, jsonrpc2.VSCodeObjectCodec{}), nil)
		defer connection.Close()

		_, err := server.handle(context.Background(), connection, &jsonrpc2.Request{
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

	It("loads saved document text fallbacks and file errors", func() {
		saved := `[output = data] {}`
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
