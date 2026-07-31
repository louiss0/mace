package analyzer

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

var _ = Describe("Unicode NFC language tooling", func() {
	It("resolves definitions across canonical spellings", func() {
		text := "|===|\nstring café = \"ok\";\n|===|\n[output = 'data'] { value: café, }"
		snapshot := AnalyzeDocumentAt(text, "file:///unicode.mace")
		line := "[output = 'data'] { value: café, }"
		position := protocol.Position{Line: 3, Character: uint32(strings.Index(line, "café") + 1)}
		definition, ok := Definition(snapshot, position)
		if !tAssert.True(ok) {
			return
		}
		tAssert.Equal(uint32(1), definition.Range.Start.Line)
	})

	It("filters completion using canonical prefixes and inserts NFC", func() {
		text := "|===|\nstring café = \"ok\";\n|===|\n[output = 'data'] { value: café, }"
		snapshot := AnalyzeDocumentAt(text, "file:///unicode.mace")
		line := "[output = 'data'] { value: café, }"
		prefixStart := strings.Index(line, "café")
		items := CompletionItems(text, snapshot, "file:///unicode.mace", protocol.Position{Line: 3, Character: uint32(prefixStart + len("café"))})
		found := false
		for _, item := range items {
			if item.Label == "café" {
				found = true
				if textEdit, ok := item.TextEdit.(protocol.TextEdit); ok {
					tAssert.Equal("café", textEdit.NewText)
				}
			}
		}
		tAssert.True(found)
	})

	It("emits NFC spelling for rename edits requested with NFD", func() {
		text := "|===|\nstring café = \"ok\";\n|===|\n[output = 'data'] { value: café, }"
		snapshot := AnalyzeDocumentAt(text, "file:///unicode.mace")
		edit, ok := Rename(text, snapshot, "file:///unicode.mace", protocol.Position{Line: 1, Character: 8}, "café")
		if !tAssert.True(ok) {
			return
		}
		for _, edits := range edit.Changes {
			for _, textEdit := range edits {
				tAssert.Equal("café", textEdit.NewText)
			}
		}
	})

	It("keeps UTF-16 ranges based on the raw identifier token", func() {
		text := "|===|\nstring café = \"ok\";\nstring next = café;\n|===|\n[output = 'data'] { value: next, }"
		snapshot := AnalyzeDocumentAt(text, "file:///unicode.mace")
		rangeValue, ok := identifierRangeAt(text, protocol.Position{Line: 1, Character: 10})
		if !tAssert.True(ok) {
			return
		}
		tAssert.Equal(uint32(7), rangeValue.Start.Character)
		tAssert.Equal(uint32(12), rangeValue.End.Character)
		tAssert.NotNil(snapshot)
	})
})
