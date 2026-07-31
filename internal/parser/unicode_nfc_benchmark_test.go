package parser

import "testing"

func BenchmarkParseUnicodeDocument(b *testing.B) {
	source := "|===|\nstring café = \"ok\";\nstring café_value = café;\nschema Profile: { prénom: string, };\n|===|\n[output = 'data'] { value: café_value, }"
	b.ReportAllocs()
	for range b.N {
		tokens, err := lexInput(source)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := New(tokens).ParseFile(); err != nil {
			b.Fatal(err)
		}
	}
}
