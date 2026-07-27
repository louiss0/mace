package processor

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Imports", func() {
	DescribeTable("merges imported declarations",
		func(file string, expected expectedValue) {
			processor := New()
			result, err := processor.ProcessInDir(file, "../..")
			tAssert.NoError(err)
			assertExpectedValue(requireOutputValue(result, "result"), expected)
		},
		Entry("imports types and schemas", `|===|
from 'fixtures/processor/imports/base.mace' import Name, User;
Name name = "Ada";
User result = { name: name, age: 30, };
|===|
[output = 'data']
{ result: result, }`, expectedValue{kind: ValueRecord, record: map[string]expectedValue{"name": {kind: ValueString, string: "Ada"}, "age": {kind: ValueInt, int64: 30}}}),
		Entry("imports values surfaced through output", `|===|
from 'fixtures/processor/imports/values.mace' import count;
|===|
[output = 'data']
{ result: count + 2, }`, expectedValue{kind: ValueInt, int64: 5}),
	)

	DescribeTable("resolves kebab-case imports",
		func(importDeclaration, expression string) {
			document := `|===|
` + importDeclaration + `
|===|
[output = 'data']
{ result: ` + expression + `, }`

			result, err := New().ProcessInDir(document, "../..")

			tAssert.NoError(err)
			tAssert.Equal("Ada", requireOutputValue(result, "result").String)
		},
		Entry("destructured name", `from 'fixtures/processor/imports/kebab.mace' import display-name;`, "display-name"),
		Entry("destructured alias", `from 'fixtures/processor/imports/kebab.mace' import display-name:imported-name;`, "imported-name"),
		Entry("bind alias", `from 'fixtures/processor/imports/kebab.mace' bind imported-data;`, "imported-data.display-name"),
	)

	It("keeps hidden declarations internal", func() {
		processor := New()
		_, err := processor.ProcessInDir(`|===|
from 'fixtures/processor/imports/base.mace' import Internal;
|===|
[output = 'data'] {}`, "../..")
		tAssert.Error(err)
		tAssert.ErrorContains(err, "imported identifier")
	})

	It("treats destructured optional imports as variables", func() {
		unguardedDocument := `|===|
from 'fixtures/processor/imports/optional_profile.mace' import profile;
|===|
[output = 'data']
{ city: profile.city, }`
		_, err := New().ProcessInDir(unguardedDocument, "../..")
		requireOptionalFieldAccessError(err)

		optionalChainDocument := `|===|
from 'fixtures/processor/imports/optional_profile.mace' import profile;
|===|
[output = 'data']
{ city?: profile?.city, }`
		_, err = New().ProcessInDir(optionalChainDocument, "../..")
		requireOptionalFieldAccessError(err)

		guardedDocument := `|===|
from 'fixtures/processor/imports/optional_profile.mace' import profile;
|===|
[output = 'data']
{ city: profile ? profile.city : "", }`
		result, err := New().ProcessInDir(guardedDocument, "../..")
		tAssert.NoError(err)
		tAssert.Equal("Paris", requireOutputValue(result, "city").String)
	})

	It("validates possibly absent expressions in imported data outputs", func() {
		document := `|===|
from 'fixtures/processor/imports/unguarded_optional_city.mace' import city;
|===|
[output = 'data']
{ city: city, }`

		_, err := New().ProcessInDir(document, "../..")

		requireAbsentValueError(err)
	})

	It("tracks optional properties from imported schemas as possibly absent", func() {
		unguardedDocument := `|===|
from 'fixtures/processor/imports/base.mace' import User;
User user = { name: "Ada", age: 30, };
|===|
[output = 'data']
{ profile: user.profile, }`
		_, err := New().ProcessInDir(unguardedDocument, "../..")
		requireOptionalFieldAccessError(err)

		resolvedDocument := `|===|
from 'fixtures/processor/imports/base.mace' import User;
User user = { name: "Ada", age: 30, };
|===|
[output = 'data']
{ bio: user?.profile?.bio ?? "unknown", }`
		result, err := New().ProcessInDir(resolvedDocument, "../..")
		tAssert.NoError(err)
		tAssert.Equal("unknown", requireOutputValue(result, "bio").String)
	})

	It("covers remote import helper branches", func() {
		workspace, err := os.MkdirTemp("", "processor-imports-*")
		tAssert.NoError(err)
		defer func() { _ = os.RemoveAll(workspace) }()

		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/schema.mace":
				_, _ = io.WriteString(writer, `[output = 'schema']
{ Remote: string, }`)
			case "/nested/schema.mace":
				_, _ = io.WriteString(writer, `[output = 'schema']
{ Nested: string, }`)
			default:
				writer.WriteHeader(http.StatusNotFound)
			}
		}))
		defer server.Close()

		_, _ = resolveImportPath(workspace, filepath.Join(workspace, "abs.mace"))
		_, _ = resolveImportPath(server.URL+"/", "./schema.mace")
		_, _ = resolveBoundedRemotePath(server.URL+"/", server.URL+"/", "./schema.mace", server.URL+"/schema.mace")
		_, _ = parseRemoteURL(server.URL + "/schema.mace")
		_, _ = readMaceSource(filepath.Join(workspace, "missing.mace"))
		_, _ = readMaceSource(server.URL + "/missing.mace")
		_, _ = loadImportExports(filepath.Join(workspace, "missing.mace"), workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
	})
})
