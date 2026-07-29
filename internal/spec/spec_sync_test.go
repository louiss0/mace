package spec

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestRFCEmbedsCanonicalGrammarExactly(t *testing.T) {
	root := filepath.Join("..", "..")
	grammar, err := os.ReadFile(filepath.Join(root, "mace.ebnf"))
	if err != nil {
		t.Fatal(err)
	}
	rfc, err := os.ReadFile(filepath.Join(root, "Mace Language Spec RFC.md"))
	if err != nil {
		t.Fatal(err)
	}

	opening := []byte("```ebnf\n")
	start := bytes.Index(rfc, opening)
	if start < 0 {
		t.Fatal("RFC has no EBNF code block")
	}
	start += len(opening)
	endOffset := bytes.Index(rfc[start:], []byte("```"))
	if endOffset < 0 {
		t.Fatal("RFC EBNF code block is unterminated")
	}
	embedded := rfc[start : start+endOffset]

	if !bytes.Equal(grammar, embedded) {
		t.Fatal("RFC EBNF block differs from mace.ebnf")
	}
	for _, artifact := range [][]byte{
		[]byte("&quot;"), []byte("&#x27;"), []byte("&#x2f;"),
		[]byte("&lt;"), []byte("&gt;"), []byte("`r`n"),
	} {
		if bytes.Contains(embedded, artifact) {
			t.Fatalf("RFC EBNF contains export artifact %q", artifact)
		}
	}
}
