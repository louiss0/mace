package processor

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"

	"github.com/louiss0/mace/internal/parser/ast"
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Security", func() {
	It("covers path and remote boundary helpers", func() {
		workspace, err := os.MkdirTemp("", "processor-security-*")
		tAssert.NoError(err)
		defer func() { _ = os.RemoveAll(workspace) }()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/schema.mace":
				_, _ = io.WriteString(w, `[output = schema]
{ Foo: Foo, }`)
			case "/nested/schema.mace":
				_, _ = io.WriteString(w, `[output = schema]
{ Nested: Nested, }`)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer server.Close()

		_, _ = resolveBoundedPath(workspace, workspace, "../escape.mace")
		_, _ = resolveBoundedRemotePath(server.URL+"/", server.URL+"/", "./schema.mace", "https://other.example.com/schema.mace")
		_, _ = parseRemoteURL("ftp://example.com/schema.mace")
		_, _ = readMaceSource(filepath.Join(workspace, "missing.mace"))
		_, _ = resolveSchemaFileDeclarations([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: strconv.Quote("schema.txt")}}, workspace, workspace)
		_, _ = loadOutputSchemaRecord(filepath.Join(workspace, "missing.mace"), workspace, "schema_file")
	})

	It("treats runtime input strings as data, not paths", func() {
		processor := NewWithInput(map[string]Value{"path": {Kind: ValueString, String: "./schema.mace"}})
		result, err := processor.Process(`|===|
nullable string path = "./schema.mace";
|===|
[output = data]
{
  path: (path),
}`)
		tAssert.NoError(err)
		assertExpectedValue(result.Output["path"], expectedValue{kind: ValueString, string: "./schema.mace"})
	})

	It("reports the path helper error branch", func() {
		cause := errors.New("root cause")
		err := DiagnosticError{Message: "wrapped", Cause: cause}
		tAssert.Equal(cause, errors.Unwrap(err))
	})
})
