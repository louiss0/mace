package diagnostic

import (
	"github.com/louiss0/mace/internal/parser/ast"
)

func FromASTRange(source ast.SourceRange) Range {
	return Range{
		Start: Position{Line: source.Start.Line, Column: source.Start.Column},
		End:   Position{Line: source.End.Line, Column: source.End.Column},
	}
}
