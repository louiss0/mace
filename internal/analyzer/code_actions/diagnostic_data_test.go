package code_actions_test

import (
	"encoding/json"
	"strings"

	"github.com/louiss0/mace/internal/analyzer"
	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/assert"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

var _ = Describe("Code action diagnostic data", func() {
	DescribeTable("attaches stable codes and the semantic identifiers required by providers",
		func(source string, code string, requiredKeys []string) {
			assertions := assert.New(GinkgoT())
			fixture := newCodeActionFixture(source, nil)
			diagnostic, found := findDiagnosticByCode(analyzer.Diagnostics(fixture.snapshot), code)
			if !found {
				targetRange := protocol.Range{
					Start: protocol.Position{},
					End:   protocol.Position{Line: protocol.UInteger(len(strings.Split(source, "\n")) + 1)},
				}
				for _, action := range analyzer.CodeActions(fixture.snapshot, fixture.uri, targetRange) {
					diagnostic, found = findDiagnosticByCode(action.Diagnostics, code)
					if found {
						break
					}
				}
			}
			if !assertions.True(found, "expected diagnostic %q", code) {
				return
			}

			encoded, err := json.Marshal(diagnostic)
			if !assertions.NoError(err) {
				return
			}
			payload := map[string]any{}
			if !assertions.NoError(json.Unmarshal(encoded, &payload)) {
				return
			}
			data, ok := payload["data"].(map[string]any)
			if !assertions.True(ok, "diagnostic must carry structured data") {
				return
			}
			for _, key := range requiredKeys {
				assertions.Contains(data, key)
			}
		},
		Entry("syntax node", "[output = 'data'] { first: 1 second: 2, }", "mace.syntax.unexpected-token", []string{"code", "nodeID"}),
		Entry("unknown symbol candidates", "[output = 'data'] { value: unknown, }", "mace.type.unknown-identifier", []string{"code", "nodeID", "symbolID", "candidateSymbols"}),
		Entry("type mismatch", "|===|\nint value = 'text';\n|===|\n[output = 'data'] { value: value, }", "mace.type.initializer-type-mismatch", []string{"code", "nodeID", "symbolID", "expectedType", "actualType"}),
		Entry("missing match members", "|===|\nvariant[string, int] value = 1; string result = match (value) { string => 's', };\n|===|\n[output = 'data'] { result: result, }", "mace.match.not-exhaustive", []string{"code", "nodeID", "missingMembers"}),
		Entry("schema fields", "|===|\nschema User: { name: string, }; User user = {};\n|===|\n[output = 'data'] { user: user, }", "mace.type.record-does-not-match-schema", []string{"code", "nodeID", "missingFields", "selectedSchema"}),
		Entry("unknown fields", "|===|\nschema User: { name: string, }; User user = { name: 'Mace', extra: 1, };\n|===|\n[output = 'data'] { user: user, }", "mace.type.record-does-not-match-schema", []string{"code", "nodeID", "unknownFields", "selectedSchema"}),
		Entry("candidate files", "[output = 'data'] { value: Shared, }", "mace.type.unknown-identifier", []string{"code", "candidateSymbols", "candidateFiles", "relatedLocations"}),
		Entry("output mode and schema", "[output = 'data', schema = Missing] {}", "mace.directive.unknown-schema-name", []string{"code", "outputMode", "selectedSchema"}),
		Entry("parse type", "[output = 'data', parse = Missing] {}", "mace.directive.unknown-parse-name", []string{"code", "outputMode", "selectedParseType"}),
	)
})
