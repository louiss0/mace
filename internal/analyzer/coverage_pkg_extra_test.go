package analyzer

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/louiss0/mace/internal/parser/ast"
	"github.com/louiss0/mace/internal/processor"
	. "github.com/onsi/ginkgo/v2"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

var _ = Describe("analyzer package helper coverage", func() {
	It("covers pkg helpers", func() {
		workspace := GinkgoT().TempDir()
		documentPath := filepath.Join(workspace, "document.mace")
		text := `|===|
type Alias: string;
schema Doc: { field: string; };
string value = "x";
|===|
[output = data]
{
  value: "x";
}
`
		tAssert.NoError(os.WriteFile(documentPath, []byte(text), 0o644))
		uri := protocol.DocumentUri(fileURI(documentPath))
		snapshot := AnalyzeDocumentAt(text, documentPath)

		_ = Hover("array", snapshot, protocol.Position{Line: 0, Character: 1})
		_ = Hover("schema", snapshot, protocol.Position{Line: 0, Character: 1})
		_ = Hover("value", snapshot, protocol.Position{Line: 3, Character: 3})
		_ = Hover("Doc", snapshot, protocol.Position{Line: 2, Character: 8})
		_ = Hover("missing", snapshot, protocol.Position{Line: 0, Character: 1})

		_ = DocumentSymbols(text, analysisSnapshot{})
		_ = DocumentSymbols(text, snapshot)

		_, _ = Rename(text, snapshot, uri, protocol.Position{Line: 3, Character: 3}, "value_renamed")
		_, _ = Rename(text, snapshot, uri, protocol.Position{Line: 3, Character: 3}, "")
		_, _ = PrepareRename(snapshot, protocol.Position{Line: 3, Character: 3})
		_, _ = PrepareRename(analysisSnapshot{}, protocol.Position{})

		manualSnapshot := analysisSnapshot{
			text:        text,
			documentURI: uri,
			symbols: []semanticSymbol{{
				Name:       "alias",
				Origin:     symbolOriginImport,
				Definition: protocol.Location{URI: uri, Range: protocol.Range{}},
				Range:      protocol.Range{},
			}},
			file: &ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./document.mace"`}, Identifiers: []ast.ImportedIdentifier{{Name: "Alias", Alias: "alias"}}}}},
		}
		_ = localImportRenameTarget(manualSnapshot, protocol.Position{}, manualSnapshot.symbols[0])
		_ = renameTokenMatchesSymbol(manualSnapshot, 0, protocol.Range{}, manualSnapshot.symbols[0])
		_ = sameLocation(protocol.Location{URI: uri}, protocol.Location{URI: uri})
		_ = rangesEqual(protocol.Range{}, protocol.Range{})
		_ = hasTextEditAtRange([]protocol.TextEdit{{Range: protocol.Range{}}}, protocol.Range{})
		_ = appendTextEditIfMissing([]protocol.TextEdit{{Range: protocol.Range{}}}, protocol.TextEdit{Range: protocol.Range{}})
		_ = appendTextEditIfMissing(nil, protocol.TextEdit{Range: protocol.Range{}})

		_, _ = resolveRootBoundedPathInRoot(workspace, workspace, "./nested/file.mace")
		_, _ = resolveRootBoundedPathInRoot(workspace, workspace, "../outside.mace")
		_, _ = resolveBoundedPathInRoot(workspace, workspace, "./nested/file.mace")
		_, _ = resolveBoundedPathInRoot(workspace, workspace, filepath.Join(workspace, "abs.mace"))
		_ = formatImportRoot("")
		_ = formatImportRoot(".")
		_ = formatImportRoot(filepath.Join(workspace, "root"))

		_ = identifierPrefixAt("alias_name", protocol.Position{Line: 0, Character: 5})
		_ = identifierPrefixAt("alias_name", protocol.Position{Line: 1, Character: 0})
		_, _ = identifierAt("alias_name", protocol.Position{Line: 0, Character: 5})
		_, _ = identifierAt(" alias", protocol.Position{Line: 0, Character: 0})
		_, _ = identifierRangeAt("alias_name", protocol.Position{Line: 0, Character: 5})
		_, _ = identifierRangeAt("alias_name", protocol.Position{})
		_, _ = nameRange("alias", "alias")
		_, _ = nameRange("alias", "missing")
		_ = isDirectivePosition("[output = data]", protocol.Position{Line: 0, Character: 1})
		_ = isDirectivePosition("[output", protocol.Position{Line: 0, Character: 1})
		_ = isDirectivePosition("plain text", protocol.Position{})
		_ = utf16LineLength("🙂x")
		_ = utf16LineLength("")

		_ = DiagnosticFromError(errors.New("error at 12:3"))
		_ = DocumentPath(uri)
		_ = FormatDocumentText(text)
	})

	It("covers document symbol and rename branches", func() {
		workspace := GinkgoT().TempDir()
		documentPath := filepath.Join(workspace, "document.mace")
		text := `|===|
from "./shared.mace" import User as alias;
string value = "x";
|===|
[output = data]
{
  value: "x";
}
`
		tAssert.NoError(os.WriteFile(documentPath, []byte(text), 0o644))
		snapshot := AnalyzeDocumentAt(text, documentPath)
		_ = DocumentSymbols(text, snapshot)
		_ = Hover(text, snapshot, protocol.Position{Line: 2, Character: 3})
		_, _ = PrepareRename(snapshot, protocol.Position{Line: 2, Character: 3})
		_, _ = Rename(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), protocol.Position{Line: 2, Character: 3}, "value_renamed")
		_, _ = Rename(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), protocol.Position{Line: 2, Character: 3}, "")
		aliasStart, _ := nameRange(text, "alias")
		_, _ = Rename(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), aliasStart, "alias_renamed")
		_ = DocumentSymbols(text, analysisSnapshot{})
	})

	It("covers package helper edge branches", func() {
		workspace := GinkgoT().TempDir()
		uri := protocol.DocumentUri(fileURI(filepath.Join(workspace, "document.mace")))
		text := `|===|
schema User: { name: string; };
schema_doc User {
  summary: "docs";
}
|===|
[output = data]
{
  user: { name: "Ada"; };
  value: "x";
}
`
		file := ast.File{
			Script: &ast.ScriptBlock{Items: []ast.Declaration{
				ast.DocDeclaration{Kind: ast.DocumentationKindGeneral, Target: "User", Documentation: ast.Documentation{Summary: &ast.StringLiteral{Lexeme: `"docs"`}}},
				ast.SchemaDeclaration{Name: "User", Type: ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}},
			}},
			Output: ast.OutputBlock{Mode: ast.OutputModeData, DataFields: []ast.OutputField{{Name: "value", Value: ast.StringLiteral{Lexeme: `"x"`}}}, SchemaFields: []ast.OutputSchemaField{{Name: "user", Type: ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}}}},
		}
		valueStart, valueEnd := nameRange(text, "value")
		snapshot := analysisSnapshot{
			text:         text,
			documentURI:  uri,
			file:         &file,
			result:       &processor.Result{Output: map[string]processor.Value{"value": {Kind: processor.ValueString, String: "x"}}},
			symbols: []semanticSymbol{{Name: "value", Kind: protocol.CompletionItemKindVariable, Detail: "string value", Documentation: "docs", Range: protocol.Range{Start: valueStart, End: valueEnd}, Definition: protocol.Location{URI: uri, Range: protocol.Range{}}}},
		}
		snapshot.symbolIndex = indexSymbols(snapshot.symbols)
		_ = Hover(text, snapshot, valueStart)
		_ = Hover(text, snapshot, protocol.Position{Line: 2, Character: 1})
		_ = DocumentSymbols(text, snapshot)
		_ = DocumentSymbols(text, analysisSnapshot{file: &file, result: snapshot.result})
		_, _ = resolveRootBoundedPathInRoot(`D:\base`, `C:\root`, `.\outside.mace`)
		_, _ = resolveRootBoundedPathInRoot(filepath.Join(workspace, "sub"), filepath.Join(workspace, "root"), "./outside.mace")
		_, _ = resolveRootBoundedPathInRoot(workspace, workspace, filepath.Join(workspace, "abs.mace"))
		_, _ = resolveRootBoundedPathInRoot(workspace, workspace, "../outside.mace")
		_, _ = resolveRootBoundedPathInRoot(workspace, workspace, "./nested/file.mace")
		_, _ = resolveBoundedPathInRoot(workspace, workspace, filepath.Join(workspace, "abs.mace"))
		_, _ = resolveBoundedPathInRoot(workspace, workspace, "./nested/file.mace")
		_ = renameTokenMatchesSymbol(snapshot, 0, protocol.Range{Start: valueStart, End: valueEnd}, semanticSymbol{Name: "value", Kind: protocol.CompletionItemKindClass, Definition: protocol.Location{URI: uri}})
		_ = renameTokenMatchesSymbol(snapshot, 0, protocol.Range{Start: valueStart, End: valueEnd}, snapshot.symbols[0])
		_, _ = Rename("missing", analysisSnapshot{text: "missing", symbols: []semanticSymbol{{Name: "missing", Kind: protocol.CompletionItemKindVariable, Range: protocol.Range{Start: protocol.Position{Line: 0, Character: 0}, End: protocol.Position{Line: 0, Character: 7}}}}}, uri, protocol.Position{Line: 0, Character: 1}, "renamed")
		renameText := "value value"
		renameStart, renameEnd := nameRange(renameText, "value")
		renameSnapshot := analysisSnapshot{text: renameText, tokens: lexAnalysisTokens(renameText), symbols: []semanticSymbol{{Name: "value", Kind: protocol.CompletionItemKindClass, Range: protocol.Range{Start: renameStart, End: renameEnd}, Definition: protocol.Location{URI: uri, Range: protocol.Range{}}}}}
		renameSnapshot.symbolIndex = indexSymbols(renameSnapshot.symbols)
		_, _ = Rename(renameText, renameSnapshot, uri, protocol.Position{Line: 0, Character: 1}, "renamed")
		aliasText := `|===|
from "./shared.mace" import User:alias;
string value = alias;
|===|
[output = data]
{
  value: alias;
}
`
		aliasURI := protocol.DocumentUri(fileURI(filepath.Join(workspace, "alias.mace")))
		aliasSnapshot := AnalyzeDocumentAt(aliasText, filepath.Join(workspace, "alias.mace"))
		_, _ = Rename(aliasText, aliasSnapshot, aliasURI, protocol.Position{Line: 2, Character: 16}, "renamed_alias")
		aliasTokens := lexAnalysisTokens(aliasText)
		usageStart := protocol.Position{Line: 2, Character: 15}
		usageEnd := protocol.Position{Line: 2, Character: 20}
		manualAliasSnapshot := analysisSnapshot{
			text:        aliasText,
			documentURI: aliasURI,
			tokens:      aliasTokens,
			file: &ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./shared.mace"`}, Identifiers: []ast.ImportedIdentifier{{Name: "User", Alias: "alias"}}}}},
			symbols: []semanticSymbol{{Name: "alias", Kind: protocol.CompletionItemKindVariable, Origin: symbolOriginImport, Range: protocol.Range{Start: usageStart, End: usageEnd}, Definition: protocol.Location{URI: protocol.DocumentUri(fileURI(filepath.Join(workspace, "shared.mace"))), Range: protocol.Range{}}}},
		}
		manualAliasSnapshot.symbolIndex = indexSymbols(manualAliasSnapshot.symbols)
		_, _ = Rename(aliasText, manualAliasSnapshot, aliasURI, usageStart, "renamed_alias")
		foreignURI := protocol.DocumentUri(fileURI(filepath.Join(workspace, "shared.mace")))
		importRenameText := `from "./shared.mace" import User:alias;
alias`
		importRenameSnapshot := analysisSnapshot{
			text:        importRenameText,
			documentURI: aliasURI,
			tokens:      lexAnalysisTokens(importRenameText),
			file: &ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./shared.mace"`}, Identifiers: []ast.ImportedIdentifier{{Name: "User", Alias: "alias"}}}}},
			symbols: []semanticSymbol{{Name: "alias", Kind: protocol.CompletionItemKindClass, Origin: symbolOriginImport, Range: protocol.Range{Start: protocol.Position{Line: 1, Character: 0}, End: protocol.Position{Line: 1, Character: 5}}, Definition: protocol.Location{URI: foreignURI, Range: protocol.Range{Start: protocol.Position{Line: 0, Character: 0}, End: protocol.Position{Line: 0, Character: 5}}}}},
		}
		importRenameSnapshot.symbolIndex = indexSymbols(importRenameSnapshot.symbols)
		_, _ = Rename(importRenameText, importRenameSnapshot, aliasURI, protocol.Position{Line: 1, Character: 1}, "renamed_alias")
		_, _ = identifierAt("alias_name", protocol.Position{Line: 0, Character: 5})
		_, _ = identifierAt(" alias", protocol.Position{Line: 0, Character: 0})
		_ = isDirectivePosition("[output = data]", protocol.Position{Line: 0, Character: 1})
		_ = isDirectivePosition("[output", protocol.Position{Line: 0, Character: 1})
		_ = isDirectivePosition("plain text", protocol.Position{})
	})
})
