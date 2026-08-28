// errors.go compiles amifl-spec.md section 2.2's Error type and section
// 3.3's postfix `?` operator (step 11) — see codegen.go's package doc for
// the surrounding step-by-step scope.
package codegen

import (
	"fmt"

	"github.com/amisonnet8/amifl/internal/ast"
)

// genTryValue lowers the postfix `?` operator (amifl-spec.md section 3.3):
// evaluate v.Value, pull its Error (and, unless v.IsBareError, its payload)
// out via FGET, then emit an `IF <error> != nil` block that RETs an
// early-return value shaped by g.retType (emitEarlyReturn) when the error
// is non-nil. Falling through the IF (the common case), the result is the
// unwrapped payload — or, for the bare-Error form, an empty token that's
// never actually read (v's own sema-resolved type is Unit there, so every
// caller reaches this only through genStmt's discard-and-ignore path, never
// a context that needs a real value — see ast.TryExpr's doc comment).
func (g *gen) genTryValue(v *ast.TryExpr) (string, error) {
	val, err := g.genValue(v.Value)
	if err != nil {
		return "", err
	}

	var errVal, payloadVal string
	if v.IsBareError {
		errVal = val
	} else {
		payloadGoType := g.prog.resolveGoType(v.ElemType)
		payloadTmp := g.newTemp()
		fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", payloadTmp, payloadGoType)
		fmt.Fprintf(g.b, "\tFGET\t%%%s\t%s\t>F0\n", payloadTmp, val)
		payloadVal = "%" + payloadTmp

		errGoType := g.prog.resolveGoType("Error")
		errTmp := g.newTemp()
		fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", errTmp, errGoType)
		fmt.Fprintf(g.b, "\tFGET\t%%%s\t%s\t>F1\n", errTmp, val)
		errVal = "%" + errTmp
	}

	condTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^bool\n", condTmp)
	fmt.Fprintf(g.b, "\tNEQ\t%%%s\t%s\tnil\n", condTmp, errVal)
	fmt.Fprintf(g.b, "\tIF\t%%%s\n", condTmp)
	g.emitEarlyReturn(errVal)
	g.b.WriteString("\tENDIF\n")

	return payloadVal, nil
}

// emitEarlyReturn RETs a value shaped by g.retType, carrying errVal as its
// error — the propagation step-11's `?` performs when it finds a non-nil
// error (amifl-spec.md: "エラーがあればゼロ値＋エラーで即returnする糖衣").
// A bare `VAR` declaration already zero-initializes every other field of
// g.retType's Go type (CLAUDE.md's established zero-value convention), so
// building the early-return value never needs more than one FSET — the
// error slot itself — regardless of what the payload type actually is.
func (g *gen) emitEarlyReturn(errVal string) {
	if g.retType == "Error" {
		fmt.Fprintf(g.b, "\tRET\t%s\n", errVal)
		return
	}
	goType := g.prog.resolveGoType(g.retType)
	tmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", tmp, goType)
	fmt.Fprintf(g.b, "\tFSET\t%%%s\t>F1\t%s\n", tmp, errVal)
	fmt.Fprintf(g.b, "\tRET\t%%%s\n", tmp)
}
