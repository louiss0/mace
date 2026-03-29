# Handoff

## Tests that are failing
- `tree-sitter test` is still red for the new corpus in `tree-sitter-mace/test/corpus`.
- The grammar is still the placeholder stub in `tree-sitter-mace/grammar.js`, so the corpus is currently all RED by design.
- `go test ./...` was green before this handoff. No known Go test failures remain.

## What bugs are present
- `tree-sitter-mace/grammar.js` does not implement the Mace language yet.
- The Mace comment rule from the spec is context-sensitive (`/=` line comment vs block comment ending in `=/`) and will likely need an external scanner instead of plain regex.
- Some tree-sitter corpus cases currently describe semantic validation rules such as invalid output-directive combinations. Tree-sitter may not be the right layer to reject all of those syntactically, so those expectations may need to move or be rewritten once the real grammar exists.

## What to do next
- Implement the real Tree-sitter grammar in `tree-sitter-mace/grammar.js` using `mace.ebnf` and `mace-spec.md` as the source of truth.
- Add an external scanner for comments so the spec disambiguation rule is handled correctly.
- Run `tree-sitter generate` and then `tree-sitter test`; iterate until the corpus passes or until any semantic-only cases are moved out of the tree-sitter corpus.
- Review `tree-sitter-mace/test/corpus/scripts.txt` and `tree-sitter-mace/test/corpus/outputs.txt` while bringing the grammar up, because those files are the most likely places where expected trees or semantic assumptions may need adjustment.
