package processor

import . "github.com/onsi/ginkgo/v2"

var _ = Describe("Unicode NFC identifiers", func() {
	It("resolves NFC declarations through NFD references", func() {
		result, err := New().Process(`|===|
string café = "ok";
|===|
[output = 'data'] { value: café, }`)
		if !tAssert.NoError(err) {
			return
		}
		tAssert.Equal("ok", result.Output["value"].String)
	})

	It("resolves NFD declarations through NFC references", func() {
		result, err := New().Process("|===|\nstring café = \"ok\";\n|===|\n[output = 'data'] { value: café, }")
		if !tAssert.NoError(err) {
			return
		}
		tAssert.Equal("ok", result.Output["value"].String)
	})

	It("rejects canonically equivalent duplicate declarations", func() {
		_, err := New().Process("|===|\nstring café = \"first\";\nstring café = \"second\";\n|===|\n[output = 'data'] {}")
		tAssert.ErrorContains(err, "duplicate declaration")
	})

	It("resolves portable cross-file import examples", func() {
		workspace := newExampleWorkspace(unicodeImportExamples)
		for _, name := range []string{
			"consumer.mace",
			"nfc-consumer.mace",
			"alias-consumer.mace",
			"bind-consumer.mace",
			"schema-file-consumer.mace",
		} {
			result, err := New().ProcessFile(examplePath(workspace, "import-canonical-equivalence/"+name))
			if !tAssert.NoError(err, name) {
				continue
			}
			if name == "schema-file-consumer.mace" {
				tAssert.Equal("ok", result.Output["value"].String, name)
				continue
			}
			tAssert.NotEmpty(result.Output, name)
		}
	})

	It("rejects canonical collisions in imported aliases and local declarations", func() {
		workspace := newExampleWorkspace(unicodeImportExamples)
		for _, name := range []string{"duplicate-aliases.mace", "local-import-collision.mace"} {
			_, err := New().ProcessFile(examplePath(workspace, "import-canonical-equivalence/"+name))
			tAssert.ErrorContains(err, "duplicate")
		}
	})

	It("preserves decomposed import paths", func() {
		workspace := newExampleWorkspace(unicodeImportExamples)
		result, err := New().ProcessFile(examplePath(workspace, "path-not-normalized.mace"))
		if !tAssert.NoError(err) {
			return
		}
		tAssert.Equal("ok", result.Output["value"].String)
	})

	It("rejects canonically equivalent host input keys", func() {
		processor := NewWithInput(map[string]Value{
			"café":  {Kind: ValueString, String: "nfc"},
			"café": {Kind: ValueString, String: "nfd"},
		})
		_, err := processor.Process(`[output = 'data'] { value: $café, }`)
		tAssert.ErrorContains(err, "duplicate input field")
	})

	It("normalizes record keys inside runtime input arrays", func() {
		processor := NewWithInput(map[string]Value{
			"items": {
				Kind: ValueArray,
				Array: []Value{{
					Kind: ValueRecord,
					Record: map[string]Value{
						"café": {Kind: ValueString, String: "ok"},
					},
				}},
			},
		})
		result, err := processor.Process(`|===|
schema Input: { items: array<{ café: string, }>, };
|===|
[output = 'data', parse = Input] { value: $items, }`)
		if !tAssert.NoError(err) {
			return
		}
		tAssert.Equal("ok", result.Output["value"].Array[0].Record["café"].String)
	})

	It("normalizes parsed input keys without changing runtime strings", func() {
		result, err := NewWithInput(map[string]Value{"café": {Kind: ValueString, String: "café"}}).Process(`|===|
schema Input: { café: string, };
|===|
[output = 'data', parse = Input] { value: $café, }`)
		if !tAssert.NoError(err) {
			return
		}
		tAssert.Equal("café", result.Output["value"].String)
	})
})
