package lsp

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/assert"
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

var tAssert *assert.Assertions

func TestLSP(t *testing.T) {
	tAssert = assert.New(t)
	RunSpecs(t, "LSP Suite")
}

type capturedNotification struct {
	method string
	params any
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

	return fileURI(path)
}

func fileURI(path string) string {
	return (&url.URL{
		Scheme: "file",
		Path:   filepath.ToSlash(path),
	}).String()
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

var _ = Describe("LSP server", func() {
	const uri = "file:///workspace/test.mace"

	It("advertises core capabilities during initialize", func() {
		server := New()

		resultValue, validMethod, validParams, err := invoke(server.Handler(), protocol.MethodInitialize, protocol.InitializeParams{}, nil)
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
		tAssert.NotNil(result.Capabilities.CompletionProvider)
		tAssert.Equal(true, result.Capabilities.HoverProvider)
		tAssert.Equal(true, result.Capabilities.DocumentSymbolProvider)
		tAssert.Equal(true, result.Capabilities.DocumentFormattingProvider)
	})

	It("rejects requests before initialize", func() {
		server := New()

		_, validMethod, validParams, err := invoke(server.Handler(), protocol.MethodTextDocumentHover, protocol.HoverParams{}, nil)
		tAssert.True(validMethod)
		tAssert.True(validParams)
		tAssert.ErrorContains(err, "server not initialized")
	})

	It("accepts the initialized notification", func() {
		server := New()
		initializeServer(server)

		_, validMethod, validParams, err := invoke(server.Handler(), protocol.MethodInitialized, protocol.InitializedParams{}, nil)
		tAssert.True(validMethod)
		tAssert.True(validParams)
		tAssert.NoError(err)
	})

	It("updates the trace level", func() {
		server := New()
		initializeServer(server)

		_, validMethod, validParams, err := invoke(server.Handler(), protocol.MethodSetTrace, protocol.SetTraceParams{
			Value: protocol.TraceValueVerbose,
		}, nil)
		tAssert.True(validMethod)
		tAssert.True(validParams)
		tAssert.NoError(err)
		tAssert.Equal(protocol.TraceValueVerbose, protocol.GetTraceValue())
	})

	It("shuts down and rejects later requests", func() {
		server := New()
		initializeServer(server)

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
		server := New()
		initializeServer(server)
		notifications := []capturedNotification{}

		didOpen(server, uri, `[output = data] { result: 1 + 2; }`, &notifications)

		if tAssert.Len(notifications, 1) {
			params := requireDiagnostics(notifications[0])
			tAssert.Equal(uri, params.URI)
			tAssert.Empty(params.Diagnostics)
		}
	})

	It("publishes syntax diagnostics when an invalid document opens", func() {
		server := New()
		initializeServer(server)
		notifications := []capturedNotification{}

		didOpen(server, uri, `[output = data] { result: ; }`, &notifications)

		if tAssert.Len(notifications, 1) {
			params := requireDiagnostics(notifications[0])
			tAssert.Len(params.Diagnostics, 1)
			tAssert.Contains(params.Diagnostics[0].Message, "parser:")
			tAssert.Equal(protocol.DiagnosticSeverityError, *params.Diagnostics[0].Severity)
		}
	})

	It("publishes processor diagnostics for invalid variable declarations", func() {
		server := New()
		initializeServer(server)
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
		}
	})

	It("publishes variable mismatch diagnostics for the failing declaration", func() {
		server := New()
		initializeServer(server)
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
		server := New()
		initializeServer(server)
		notifications := []capturedNotification{}

		didOpen(server, uri, `[output = data] { result: ; }`, &notifications)
		didChange(server, uri, 2, `[output = data] { result: 3; }`, &notifications)

		if tAssert.Len(notifications, 2) {
			params := requireDiagnostics(notifications[1])
			tAssert.Empty(params.Diagnostics)
		}
	})

	It("clears processor diagnostics when a variable declaration is fixed on change", func() {
		server := New()
		initializeServer(server)
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

	It("publishes processor diagnostics for invalid output data structures", func() {
		server := New()
		initializeServer(server)
		notifications := []capturedNotification{}

		didOpen(server, uri, `|===|
schema Point = { x: int; y: int; };
schema Plot = { points: array<Point>; };
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
		server := New()
		initializeServer(server)
		notifications := []capturedNotification{}

		didOpen(server, uri, `|===|
schema Point = { x: int; y: int; };
schema Plot = { points: array<Point>; };
|===|
[output = data, schema = Plot]
{
  points: [
    { x: 1; y: 2; },
    { x: 3; }
  ];
}`, &notifications)
		didChange(server, uri, 2, `|===|
schema Point = { x: int; y: int; };
schema Plot = { points: array<Point>; };
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
		server := New()
		initializeServer(server)
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

	It("returns keyword completions using the current prefix", func() {
		server := New()
		initializeServer(server)
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, "sche", nil)

		resultValue, validMethod, validParams, err := invoke(server.Handler(), protocol.MethodTextDocumentCompletion, protocol.CompletionParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri},
				Position:     protocol.Position{Line: 0, Character: 4},
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

	It("only suggests import after a valid from path", func() {
		server := New()
		initializeServer(server)

		workspace, err := os.MkdirTemp("", "mace-lsp-import-*")
		tAssert.NoError(err)

		writeWorkspaceFile(workspace, "shared.mace", `[output = schema]
{
  User: { name: string; };
  Config: string;
}`)
		uri := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", `from "./shared.mace" imp`))

		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `from "./shared.mace" imp`, nil)

		labels := completeLabels(server, uri, 0, uint32(len(`from "./shared.mace" imp`)))
		tAssert.Equal([]string{"import"}, labels)
	})

	It("uses the current directory as the default import path baseline", func() {
		server := New()
		initializeServer(server)

		workspace, err := os.MkdirTemp("", "mace-lsp-import-path-*")
		tAssert.NoError(err)

		writeWorkspaceFile(workspace, "shared.mace", `[output = data] { name: "Ada"; }`)
		writeWorkspaceFile(workspace, "nested/roles.mace", `[output = data] { role: "admin"; }`)
		uri := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", ``))

		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `from "`, nil)

		labels := completeLabels(server, uri, 0, uint32(len(`from "`)))
		tAssert.Contains(labels, "./shared.mace")
		tAssert.Contains(labels, "./nested/")
	})

	It("suggests parent relative import paths while the from string changes", func() {
		server := New()
		initializeServer(server)

		workspace, err := os.MkdirTemp("", "mace-lsp-parent-import-path-*")
		tAssert.NoError(err)

		writeWorkspaceFile(workspace, "shared.mace", `[output = data] { name: "Ada"; }`)
		consumerURI := protocol.DocumentUri(writeWorkspaceFile(workspace, "nested/consumer.mace", ``))

		openEmptyDocument(server, consumerURI, nil)
		didChange(server, consumerURI, 2, `from "../`, nil)

		labels := completeLabels(server, consumerURI, 0, uint32(len(`from "../`)))
		tAssert.Contains(labels, "../shared.mace")
	})

	It("suggests import after a valid from path change with trailing space", func() {
		server := New()
		initializeServer(server)

		workspace, err := os.MkdirTemp("", "mace-lsp-import-space-*")
		tAssert.NoError(err)

		writeWorkspaceFile(workspace, "shared.mace", `[output = schema]
{
  User: { name: string; };
}`)
		uri := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", ``))

		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `from "./shared.mace" `, nil)

		labels := completeLabels(server, uri, 0, uint32(len(`from "./shared.mace" `)))
		tAssert.Equal([]string{"import"}, labels)
	})

	It("only suggests identifiers exported by the import path after change", func() {
		server := New()
		initializeServer(server)

		workspace, err := os.MkdirTemp("", "mace-lsp-imported-*")
		tAssert.NoError(err)

		writeWorkspaceFile(workspace, "shared.mace", `[output = schema]
{
  User: { name: string; };
  Config: string;
}`)
		uri := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", `from "./shared.mace" import U`))

		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `from "./shared.mace" import U`, nil)

		labels := completeLabels(server, uri, 0, uint32(len(`from "./shared.mace" import U`)))
		tAssert.Equal([]string{"User"}, labels)
	})

	It("suggests all exported identifiers after import changes", func() {
		server := New()
		initializeServer(server)

		workspace, err := os.MkdirTemp("", "mace-lsp-imported-all-*")
		tAssert.NoError(err)

		writeWorkspaceFile(workspace, "shared.mace", `[output = schema]
{
  User: { name: string; };
  Config: string;
}`)
		uri := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", ``))

		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `from "./shared.mace" import `, nil)

		labels := completeLabels(server, uri, 0, uint32(len(`from "./shared.mace" import `)))
		tAssert.Equal([]string{"Config", "User"}, labels)
	})

	It("only suggests directives inside directive delimiters", func() {
		server := New()
		initializeServer(server)
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `out`, nil)

		labels := completeLabels(server, uri, 0, 3)
		tAssert.NotContains(labels, "output")

		didChange(server, uri, 3, `[out`, nil)
		labels = completeLabels(server, uri, 0, 4)
		tAssert.Equal([]string{"output"}, labels)
	})

	It("suggests schema and schema_file after output schema and a comma", func() {
		server := New()
		initializeServer(server)
		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `[output = schema, s`, nil)

		labels := completeLabels(server, uri, 0, uint32(len(`[output = schema, s`)))
		tAssert.Equal([]string{"schema", "schema_file"}, labels)
	})

	It("suggests local and imported schemas after schema directive", func() {
		server := New()
		initializeServer(server)

		workspace, err := os.MkdirTemp("", "mace-lsp-schema-ref-*")
		tAssert.NoError(err)

		writeWorkspaceFile(workspace, "shared.mace", `[output = schema]
{
  ImportedUser: { name: string; };
}`)
		uri := protocol.DocumentUri(writeWorkspaceFile(workspace, "consumer.mace", ``))

		openEmptyDocument(server, uri, nil)
		didChange(server, uri, 2, `from "./shared.mace" import ImportedUser;
|===|
schema LocalUser = { id: int; };
|===|
[output = schema, schema = `, nil)

		labels := completeLabels(server, uri, 4, uint32(len(`[output = schema, schema = `)))
		tAssert.Equal([]string{"ImportedUser", "LocalUser"}, labels)
	})

	It("suggests schema files and excludes files already imported", func() {
		server := New()
		initializeServer(server)

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
		didChange(server, uri, 2, `from "./shared.mace" import ImportedUser;
[output = schema, schema_file = "`, nil)

		labels := completeLabels(server, uri, 1, uint32(len(`[output = schema, schema_file = "`)))
		tAssert.NotContains(labels, "./shared.mace")
		tAssert.Contains(labels, "./other.mace")
	})

	It("returns hover documentation for language keywords", func() {
		server := New()
		initializeServer(server)
		didOpen(server, uri, `|===|
schema User = { name: string; };
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
	})

	It("returns hover details for user declarations", func() {
		server := New()
		initializeServer(server)
		didOpen(server, uri, `|===|
string env = "dev";
|===|
[output = data] { result: env; }`, nil)

		resultValue, validMethod, validParams, err := invoke(server.Handler(), protocol.MethodTextDocumentHover, protocol.HoverParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri},
				Position:     protocol.Position{Line: 3, Character: 25},
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
	})

	It("returns hierarchical document symbols", func() {
		server := New()
		initializeServer(server)
		didOpen(server, uri, `|===|
schema User = { name: string; age?: int; };
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

	It("formats a document into canonical source", func() {
		server := New()
		initializeServer(server)
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
		server := New()
		initializeServer(server)
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
})
