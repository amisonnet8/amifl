// module.go compiles step 14's cross-package qualified reference
// `alias.Name` (amifl-spec.md section 12.2): a function call
// (`alias.Name(args...)`) or a const reference (`alias.NAME`), both
// represented by sema (resolveFieldExpr's qualified-reference branch) as
// an *ast.FieldExpr with IsQualifiedCall/QualifiedCallee/QualifiedArgTypes
// or QualifiedConstValue set — see FieldExpr's own doc comment for why
// this reuses the same node as tuple/struct field access and enum variant
// construction rather than a dedicated node. See codegen.go's package doc
// for the surrounding step-by-step scope.
package codegen

import (
	"fmt"

	"github.com/amisonnet8/amifl/internal/ast"
)

// genQualifiedCallStmt emits a qualified function call purely for effect,
// discarding any result — the ast.FieldExpr counterpart of genCallStmt,
// needed because a Unit-returning call has no result operand at all (see
// genStmt's own doc comment on why it can't be routed through genValue).
func (g *gen) genQualifiedCallStmt(v *ast.FieldExpr) error {
	argVals, err := g.genQualifiedArgValues(v)
	if err != nil {
		return err
	}
	g.writeCall("", v.QualifiedCallee, argVals)
	return nil
}

// genQualifiedCallValue is genQualifiedCallStmt's counterpart for a call
// used as a value: declares a fresh temp of the call's result type and
// receives the CALL's result into it — QualifiedCallee is already the
// exporting package's own fully-mangled AMIVM callname token (sema's
// resolveQualifiedReference computed it once, from that package's already-
// finished Exports — see ast.FieldExpr's doc comment), so this needs no
// prefix logic of its own, unlike an ordinary same-package call
// (calleeToken).
func (g *gen) genQualifiedCallValue(v *ast.FieldExpr) (string, error) {
	argVals, err := g.genQualifiedArgValues(v)
	if err != nil {
		return "", err
	}
	goType := g.prog.resolveGoType(v.ResolvedType)
	tmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", tmp, goType)
	g.writeCall("%"+tmp, v.QualifiedCallee, argVals)
	return "%" + tmp, nil
}

func (g *gen) genQualifiedArgValues(v *ast.FieldExpr) ([]string, error) {
	vals := make([]string, len(v.Args))
	for i, a := range v.Args {
		val, err := g.genValue(a.Value)
		if err != nil {
			return nil, err
		}
		vals[i] = val
	}
	return vals, nil
}
