package processor

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"

	"github.com/louiss0/mace/internal/parser/ast"
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Path helpers", func() {
	It("clones and preserves nested contexts", func() {
		original := newProcessContext("/base", "/root")
		original.optionalParseVars["x"] = struct{}{}
		cloned := original.clone()
		tAssert.Equal(original.importBaseDir, cloned.importBaseDir)
		tAssert.Equal(original.importRootDir, cloned.importRootDir)
		tAssert.NotNil(cloned.symbols)
		tAssert.NotNil(cloned.types)
		tAssert.NotNil(cloned.schemas)
		tAssert.NotNil(cloned.variables)
		tAssert.NotNil(cloned.environment)
		tAssert.Contains(cloned.optionalParseVars, "x")
	})

	It("formats local and remote import roots", func() {
		tAssert.Equal("./", formatImportRoot(""))
		tAssert.Equal("./", formatImportRoot("."))
		tAssert.Equal("workspace/", formatImportRoot(filepath.Join("/tmp", "workspace")))
		tAssert.Equal("https://example.com/root/", formatImportRoot("https://example.com/root/"))
	})

	It("clones empty process contexts safely", func() {
		var empty processContext
		cloned := empty.clone()
		tAssert.Equal(processContext{}, cloned)
	})

	It("parses remote URLs and derives base directories", func() {
		remote, ok := parseRemoteURL("https://example.com/root/file.mace")
		tAssert.True(ok)
		tAssert.Equal("https", remote.Scheme)
		tAssert.Equal("example.com", remote.Host)

		_, ok = parseRemoteURL("file:///tmp/file.mace")
		tAssert.False(ok)
		tAssert.Equal("https://example.com/root/", basePathDir("https://example.com/root/file.mace"))
		tAssert.Equal(filepath.Dir("/tmp/file.mace"), basePathDir("/tmp/file.mace"))
	})

	It("resolves import paths within and outside bounded scopes", func() {
		resolved, err := resolveImportPath("/workspace", "nested/file.mace")
		tAssert.NoError(err)
		tAssert.Contains(resolved, "nested")

		resolved, err = resolveImportPath("https://example.com/root/", "child/file.mace")
		tAssert.NoError(err)
		tAssert.Equal("https://example.com/root/child/file.mace", resolved)

		absolutePath, pathErr := filepath.Abs("absolute/file.mace")
		tAssert.NoError(pathErr)
		_, err = resolveImportPath("/workspace", absolutePath)
		tAssert.ErrorContains(err, "must be relative")

		bounded, err := resolveImportPathInScope("/workspace", "/workspace", "nested/file.mace", true)
		tAssert.NoError(err)
		tAssert.Contains(bounded, "nested")

		_, err = resolveBoundedPath("/workspace", "/workspace", "../escape.mace")
		tAssert.ErrorContains(err, "escapes root")

		boundedRemote, err := resolveBoundedRemotePath("https://example.com/root/", "https://example.com/root/", "child/file.mace", "https://example.com/root/child/file.mace")
		tAssert.NoError(err)
		tAssert.Equal("https://example.com/root/child/file.mace", boundedRemote)
		_, err = resolveBoundedRemotePath("https://example.com/root/", "https://example.com/root/", "child/file.mace", "https://evil.example.com/root/child/file.mace")
		tAssert.ErrorContains(err, "escapes root")
	})

	It("validates mace source paths", func() {
		tAssert.NoError(validateMaceSourcePath("config.mace"))
		tAssert.ErrorContains(validateMaceSourcePath("config.txt"), "must end in .mace")
	})

	It("reads local and remote mace sources", func() {
		localDir, err := os.MkdirTemp("", "mace-local-*")
		tAssert.NoError(err)
		localPath := filepath.Join(localDir, "config.mace")
		tAssert.NoError(os.WriteFile(localPath, []byte("local"), 0o600))

		contents, err := readMaceSource(localPath)
		tAssert.NoError(err)
		tAssert.Equal("local", contents)

		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			_, _ = writer.Write([]byte("remote"))
		}))
		defer server.Close()

		contents, err = readMaceSource(server.URL + "/config.mace")
		tAssert.NoError(err)
		tAssert.Equal("remote", contents)
	})
})

var _ = Describe("Path helper coverage", func() {
	It("covers remaining path and import helper branches", func() {
		workspace, setupErr := os.MkdirTemp("", "processor-paths-*")
		tAssert.NoError(setupErr)
		var err error
		defer func() { _ = os.RemoveAll(workspace) }()

		emptyImports, err := resolveImportsWithState(ast.File{}, workspace, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		tAssert.NoError(err)
		tAssert.Nil(emptyImports)

		_, err = resolveImportsWithState(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"bad path"`}}}}, workspace, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		tAssert.Error(err)

		_, err = loadImportExports(filepath.Join(workspace, "does-not-exist.mace"), workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		tAssert.Error(err)

		brokenContext := newProcessContext(workspace, workspace)
		brokenContext.schemas.Add("User", ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: nil}}})
		brokenContext.symbols.Add("User", symbolKindSchema)
		_, err = collectImportExports(ast.OutputBlock{Mode: ast.OutputModeData, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: `"User"`}}}, brokenContext)
		tAssert.Error(err)

		remoteServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/root/imports/base.mace":
				_, _ = io.WriteString(w, `[output = schema]
{ Thing: string; }`)
			case "/root/imports/child.mace":
				_, _ = io.WriteString(w, `[output = schema]
{ Child: string; }`)
			case "/import.mace":
				_, _ = io.WriteString(w, `[output = schema]
{ Remote: string; }`)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer remoteServer.Close()

		localFile := writeFixtureFile(workspace, "source.mace", `[output = schema]
{ Local: string; }`)
		_, ok := parseRemoteURL("https://example.com/root/file.mace")
		tAssert.True(ok)
		_, err = resolveImportPath(workspace, "child.mace")
		tAssert.NoError(err)
		_, err = resolveImportPath(remoteServer.URL+"/root/", "./imports/base.mace")
		tAssert.NoError(err)
		_, err = resolveImportPath(workspace, localFile)
		tAssert.Error(err)
		_, err = resolveBoundedPath(workspace, workspace, "../escape.mace")
		tAssert.Error(err)
		resolvedRemote, err := resolveBoundedPath(remoteServer.URL+"/root/", remoteServer.URL+"/root/", "./imports/base.mace")
		tAssert.NoError(err)
		tAssert.Contains(resolvedRemote, "/root/imports/base.mace")
		_, err = resolveBoundedRemotePath(remoteServer.URL+"/root/", remoteServer.URL+"/root/", "../escape.mace", remoteServer.URL+"/escape.mace")
		tAssert.Error(err)
		tAssert.Equal("./", formatImportRoot("."))
		tAssert.Equal("./", formatImportRoot(""))
		tAssert.Equal(remoteServer.URL+"/root/", formatImportRoot(remoteServer.URL+"/root/"))
		tAssert.Contains(formatImportRoot(workspace), filepath.Base(workspace))
		localContents, err := readMaceSource(localFile)
		tAssert.NoError(err)
		tAssert.Contains(localContents, "Local")
		_, err = readMaceSource(remoteServer.URL + "/missing.mace")
		tAssert.Error(err)
		_, ok = parseRemoteURL("ftp://example.com/file.mace")
		tAssert.False(ok)
		_, ok = parseRemoteURL("https:///missing-host")
		tAssert.False(ok)

		imports := []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./source.mace"`}, Identifiers: []ast.ImportedIdentifier{{Name: "Local"}}}}
		resolvedImports, err := resolveImportsWithState(ast.File{Imports: imports}, workspace, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		tAssert.NoError(err)
		tAssert.Len(resolvedImports, 1)
		_, err = resolveImportsWithState(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./source.mace"`}, Identifiers: []ast.ImportedIdentifier{{Name: "Missing"}}}}}, workspace, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		tAssert.Error(err)
		_, err = resolveImportsWithState(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./source.mace"`}, ImportAs: &ast.ImportedIdentifier{Name: "Local"}}, {Path: ast.StringLiteral{Lexeme: `"./source.mace"`}, ImportAs: &ast.ImportedIdentifier{Name: "Local"}}}}, workspace, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		tAssert.Error(err)
		decl, err := importFileAsDeclaration("thing", map[string]importedDeclaration{"Local": {name: "Local", kind: symbolKindVariable, value: Value{Kind: ValueString, String: "Ada"}, vtype: valueType{kind: ValueString}}})
		tAssert.NoError(err)
		tAssert.Equal(symbolKindVariable, decl.kind)
		decl, err = importFileAsDeclaration("thing", map[string]importedDeclaration{"Local": {name: "Local", kind: symbolKindSchema, record: ast.RecordType{Fields: []ast.SchemaField{{Name: "Local", Type: ast.PrimitiveType{Name: "string"}}}}}})
		tAssert.NoError(err)
		tAssert.Equal(symbolKindSchema, decl.kind)
		_, err = importFileAsDeclaration("thing", map[string]importedDeclaration{"Local": {name: "Local", kind: symbolKind(99)}})
		tAssert.Error(err)

		ref := typeReferenceFromValueType(valueType{choiceValues: []Value{{Kind: ValueString, String: "Ada"}}})
		tAssert.NotNil(ref)
		ref = typeReferenceFromValueType(valueType{members: []valueType{{kind: ValueString}, {kind: ValueInt}}})
		tAssert.NotNil(ref)
		ref = typeReferenceFromValueType(valueType{kind: ValueArray, element: &valueType{kind: ValueInt}})
		tAssert.NotNil(ref)
		ref = typeReferenceFromValueType(valueType{kind: ValueRecord, schemaName: "User"})
		tAssert.NotNil(ref)
		ref = typeReferenceFromValueType(valueType{})
		tAssert.NotNil(ref)
	})

	It("covers remaining path and import edge branches", func() {
		tAssert.Equal("./", formatImportRoot("."))
		tAssert.Equal("root/", formatImportRoot("/tmp/root"))
		tAssert.Equal("https://example.com/root", formatImportRoot("https://example.com/root"))
		_, ok := parseRemoteURL("not-a-url")
		tAssert.False(ok)
		absPath, err := filepath.Abs("path.mace")
		tAssert.NoError(err)
		_, err = resolveImportPath(".", absPath)
		tAssert.Error(err)
		root, err := os.MkdirTemp("", "processor-remote-*")
		tAssert.NoError(err)
		defer func() { tAssert.NoError(os.RemoveAll(root)) }()
		_, err = resolveBoundedPath(root, root, "../outside.mace")
		tAssert.Error(err)
		_, err = resolveBoundedRemotePath("https://example.com/base", "https://example.com/root", "https://evil.com/x.mace", "https://evil.com/x.mace")
		tAssert.Error(err)

		missing := filepath.Join(root, "missing.mace")
		_, err = readMaceSource(missing)
		tAssert.Error(err)
		_, err = loadImportExports(missing, root, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		tAssert.Error(err)
		_, err = loadSchemaFileDeclarations(missing, root, map[string]map[string]ast.Declaration{}, map[string]struct{}{})
		tAssert.Error(err)
		_, err = loadOutputSchemaRecord(missing, root, "schema_file")
		tAssert.Error(err)

		httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, "hello")
		}))
		defer httpServer.Close()
		body, err := readMaceSource(httpServer.URL)
		tAssert.NoError(err)
		tAssert.Equal("hello", body)

		_, err = resolveImportsWithState(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./base.mace"`}, ImportAs: &ast.ImportedIdentifier{Name: "User"}}, {Path: ast.StringLiteral{Lexeme: `"./base.mace"`}, ImportAs: &ast.ImportedIdentifier{Name: "User"}}}}, root, root, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		tAssert.Error(err)
		_, err = resolveImportsWithState(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./base.mace"`}, Identifiers: []ast.ImportedIdentifier{{Name: "Missing"}}}}}, root, root, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		tAssert.Error(err)
	})
})
