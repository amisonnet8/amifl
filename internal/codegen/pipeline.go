// pipeline.go compiles amifl-spec.md section 13.8's pipeline-DX helpers
// (tap/peek, step 15) — both `(v: T, ...) -> T` over an entirely
// unconstrained T, so each is one amiflrt.Tap[T]/amiflrt.Peek[T] generic
// call (CLAUDE.md's step-11 "確定した設計判断" pattern: AMIVM's `CALL`
// explicit-type-argument extension, sema/builtins_pipeline.go's ArgTypes
// supplying T).
package codegen

import (
	"fmt"

	"github.com/amisonnet8/amifl/internal/ast"
)

// genTapValue emits `tap(v, label) -> T` (amifl-spec.md section 13.8).
func (g *gen) genTapValue(c *ast.CallExpr) (string, error) {
	vVal, err := g.genValue(c.Args[0])
	if err != nil {
		return "", err
	}
	labelVal, err := g.genValue(c.Args[1])
	if err != nil {
		return "", err
	}
	goType := g.prog.resolveGoType(c.ResolvedType)
	tmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", tmp, goType)
	g.writeGenericCall([]string{"%" + tmp}, "?amiflrt.Tap", []string{goType}, []string{vVal, labelVal})
	return "%" + tmp, nil
}

// genPeekValue emits `peek(v) -> T` (amifl-spec.md section 13.8).
func (g *gen) genPeekValue(c *ast.CallExpr) (string, error) {
	vVal, err := g.genValue(c.Args[0])
	if err != nil {
		return "", err
	}
	goType := g.prog.resolveGoType(c.ResolvedType)
	tmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", tmp, goType)
	g.writeGenericCall([]string{"%" + tmp}, "?amiflrt.Peek", []string{goType}, []string{vVal})
	return "%" + tmp, nil
}
