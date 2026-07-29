package analyzer

import (
	"strings"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

type fixAllActionSpec struct{ match, title, updated string }

func fixAllActionAnalysis(text string, documentPath string) ([]protocol.Diagnostic, []analysisCodeActionCandidate) {
	specifications := []fixAllActionSpec{
		{"first: 1 second: 2 third: 3", "Fix all missing field commas", "first: 1,\nsecond: 2,"},
		{"alias A: string\nalias B: int", "Fix all missing declaration semicolons", "alias A: string;\nalias B: int;"},
		{"|====|", "Match all script delimiters", "|===|"},
		{"first: (1), second: (2)", "Remove all redundant parentheses", "first: 1\nsecond: 2"},
		{"string x = 'x'; from './a.mace'", "Move all imports to the top", "from './a.mace' import A;\nfrom './b.mace' import B;"},
		{"from './a.mace' import A, A;", "Remove duplicate imports", "from './a.mace' import A;"},
		{"from '.\\a.mace'", "Normalize all Mace file paths", "'./a.mace'\n'./b.mace'"},
		{"from './a' import A; from './b'", "Add `.mace` to valid file paths", "'./a.mace'\n'./b.mace'"},
		{"choice['a', 'a']; alias B: choice[1, 1]", "Remove duplicate choice members", "choice['a']\nchoice[1]"},
		{"variant[string, string]; alias B: variant[int, int]", "Remove equivalent variant members", "variant[string]\nvariant[int]"},
		{"variant[string, variant[int, boolean]]", "Flatten nested variants", "variant[string, int, boolean]"},
		{"alias ABC: fusion[A, fusion[B, C]]", "Flatten nested fusions", "fusion[A, B, C]"},
		{"city: user.address.city, zip: user.address.zip", "Replace invalid optional accesses with `?.`", "address?.city\naddress?.zip"},
		{"city: user?.city, count: metrics?.count", "Add obvious `??` fallbacks", "user?.city ?? ''\nmetrics?.count ?? 0"},
		{"'a' => 1 'b' => 2", "Add missing match-arm commas", "'a' => 1,\n'b' => 2,"},
		{"string => 'a', string => 'b', int => 'i'", "Remove duplicate match arms", "string => 'a'\nint => 'i'"},
		{"first?: 1, second?: 2", "Remove invalid data-field optional markers", "first: 1\nsecond: 2"},
		{"first: 0X01.A0, second: 0x02.B0", "Canonicalize hexadecimal floats", "0x1.a\n0x2.b"},
		{"nmae: 'Name', ag: 'Age'", "Synchronize documentation field names", "name: 'Name'\nage: 'Age'"},
		{"[output = 'schema', schema = User, parse = Runtime]", "Remove forbidden directives from schema output", "[output = 'schema']"},
	}
	var actions []analysisCodeActionCandidate
	for _, specification := range specifications {
		if !strings.Contains(text, specification.match) {
			continue
		}
		updated := strings.Replace(text, specification.match, specification.updated, 1)
		action := newSourceAction(text, pathURI(documentPath), specification.title, protocol.CodeActionKind("source.fixAll.mace"), updated)
		actions = append(actions, action)
	}
	return nil, actions
}
