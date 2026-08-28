// builtins_numeric.go compiles amifl-spec.md section 13.7's numeric
// built-ins and section 13.9's error-handling built-ins (step 11 phase
// 11d) — the codegen half of sema/builtins_numeric.go's dispatch.
package codegen

import (
	"fmt"

	"github.com/amisonnet8/amifl/internal/ast"
)

// zeroLiteralFor returns the AMIVM literal token for typ's zero value —
// codegen.go's genUnaryValue established this int-vs-float distinction
// for unary `-`; abs/clamp need the identical token here.
func zeroLiteralFor(typ string) string {
	if typ == "Float32" || typ == "Float64" {
		return "0.0"
	}
	return "0"
}

// genMinMaxValue emits min/max(a, b) -> Numeric (amifl-spec.md section
// 13.7) via a direct LT/GT comparison and an IF/ELSE picking the winning
// operand — no amiflrt needed, type-independent by construction (the
// same LT/GT instructions genBinaryValue already emits for `<`/`>`).
func (g *gen) genMinMaxValue(c *ast.CallExpr, instr string) (string, error) {
	aVal, err := g.genValue(c.Args[0])
	if err != nil {
		return "", err
	}
	bVal, err := g.genValue(c.Args[1])
	if err != nil {
		return "", err
	}
	goType := g.prog.resolveGoType(c.ResolvedType)

	condTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^bool\n", condTmp)
	fmt.Fprintf(g.b, "\t%s\t%%%s\t%s\t%s\n", instr, condTmp, aVal, bVal)

	resultTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", resultTmp, goType)
	fmt.Fprintf(g.b, "\tIF\t%%%s\n", condTmp)
	fmt.Fprintf(g.b, "\tSET\t%%%s\t%s\n", resultTmp, aVal)
	g.b.WriteString("\tELSE\n")
	fmt.Fprintf(g.b, "\tSET\t%%%s\t%s\n", resultTmp, bVal)
	g.b.WriteString("\tENDIF\n")
	return "%" + resultTmp, nil
}

// genAbsValue emits `abs(v) -> Numeric` (amifl-spec.md section 13.7) as
// `IF v < 0 { 0 - v } ELSE { v }` — works uniformly regardless of
// signedness (an unsigned v < 0 is always false, so the IF branch is
// simply never taken; codegen.go's genUnaryValue established the same
// "no per-signedness branching needed" property for unary `-`).
func (g *gen) genAbsValue(c *ast.CallExpr) (string, error) {
	vVal, err := g.genValue(c.Args[0])
	if err != nil {
		return "", err
	}
	goType := g.prog.resolveGoType(c.ResolvedType)
	zero := zeroLiteralFor(c.ResolvedType)

	condTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^bool\n", condTmp)
	fmt.Fprintf(g.b, "\tLT\t%%%s\t%s\t%s\n", condTmp, vVal, zero)

	resultTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", resultTmp, goType)
	fmt.Fprintf(g.b, "\tIF\t%%%s\n", condTmp)
	negTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", negTmp, goType)
	fmt.Fprintf(g.b, "\tSUB\t%%%s\t%s\t%s\n", negTmp, zero, vVal)
	fmt.Fprintf(g.b, "\tSET\t%%%s\t%%%s\n", resultTmp, negTmp)
	g.b.WriteString("\tELSE\n")
	fmt.Fprintf(g.b, "\tSET\t%%%s\t%s\n", resultTmp, vVal)
	g.b.WriteString("\tENDIF\n")
	return "%" + resultTmp, nil
}

// genClampValue emits `clamp(v, lo, hi) -> Numeric` (amifl-spec.md
// section 13.7) as `max(lo, min(v, hi))`, composed directly from the same
// LT/GT-plus-IF/ELSE shape genMinMaxValue uses (inlined here rather than
// calling genMinMaxValue twice, since clamp's operands are already-
// generated value tokens, not ast.Expr nodes genMinMaxValue would need to
// re-evaluate).
func (g *gen) genClampValue(c *ast.CallExpr) (string, error) {
	vVal, err := g.genValue(c.Args[0])
	if err != nil {
		return "", err
	}
	loVal, err := g.genValue(c.Args[1])
	if err != nil {
		return "", err
	}
	hiVal, err := g.genValue(c.Args[2])
	if err != nil {
		return "", err
	}
	goType := g.prog.resolveGoType(c.ResolvedType)

	minTmp := g.selectValue(goType, "LT", vVal, hiVal, vVal, hiVal)
	maxTmp := g.selectValue(goType, "GT", loVal, "%"+minTmp, loVal, "%"+minTmp)
	return "%" + maxTmp, nil
}

// selectValue emits `IF a <instr> b { winner = whenTrue } ELSE { winner =
// whenFalse }`, returning winner's temp name — the shared shape behind
// genMinMaxValue/genClampValue/genAbsValue's branch-and-pick pattern.
func (g *gen) selectValue(goType, instr, a, b, whenTrue, whenFalse string) string {
	condTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^bool\n", condTmp)
	fmt.Fprintf(g.b, "\t%s\t%%%s\t%s\t%s\n", instr, condTmp, a, b)
	resultTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", resultTmp, goType)
	fmt.Fprintf(g.b, "\tIF\t%%%s\n", condTmp)
	fmt.Fprintf(g.b, "\tSET\t%%%s\t%s\n", resultTmp, whenTrue)
	g.b.WriteString("\tELSE\n")
	fmt.Fprintf(g.b, "\tSET\t%%%s\t%s\n", resultTmp, whenFalse)
	g.b.WriteString("\tENDIF\n")
	return resultTmp
}

// genFloatUnaryValue emits round/floor/ceil/sqrt(v) -> Float (amifl-
// spec.md section 13.7) via the matching math.* function.
func (g *gen) genFloatUnaryValue(c *ast.CallExpr, goFn string) (string, error) {
	vVal, err := g.genValue(c.Args[0])
	if err != nil {
		return "", err
	}
	goType := g.prog.resolveGoType(c.ResolvedType)
	tmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", tmp, goType)
	if goType == "float32" {
		raw64Tmp := g.newTemp()
		fmt.Fprintf(g.b, "\tVAR\t%%%s\t^float64\n", raw64Tmp)
		castTmp := g.newTemp()
		fmt.Fprintf(g.b, "\tVAR\t%%%s\t^float64\n", castTmp)
		g.writeCall("%"+castTmp, "?float64", []string{vVal})
		g.writeCall("%"+raw64Tmp, goFn, []string{"%" + castTmp})
		g.writeCall("%"+tmp, "?float32", []string{"%" + raw64Tmp})
		return "%" + tmp, nil
	}
	g.writeCall("%"+tmp, goFn, []string{vVal})
	return "%" + tmp, nil
}

// genPowValue emits `pow(base, exp) -> Float` (amifl-spec.md section
// 13.7) via math.Pow — Float32 narrows through float64 the same way
// genFloatUnaryValue does.
func (g *gen) genPowValue(c *ast.CallExpr) (string, error) {
	baseVal, err := g.genValue(c.Args[0])
	if err != nil {
		return "", err
	}
	expVal, err := g.genValue(c.Args[1])
	if err != nil {
		return "", err
	}
	goType := g.prog.resolveGoType(c.ResolvedType)
	if goType == "float32" {
		base64Tmp := g.newTemp()
		fmt.Fprintf(g.b, "\tVAR\t%%%s\t^float64\n", base64Tmp)
		g.writeCall("%"+base64Tmp, "?float64", []string{baseVal})
		exp64Tmp := g.newTemp()
		fmt.Fprintf(g.b, "\tVAR\t%%%s\t^float64\n", exp64Tmp)
		g.writeCall("%"+exp64Tmp, "?float64", []string{expVal})
		raw64Tmp := g.newTemp()
		fmt.Fprintf(g.b, "\tVAR\t%%%s\t^float64\n", raw64Tmp)
		g.writeCall("%"+raw64Tmp, "?math.Pow", []string{"%" + base64Tmp, "%" + exp64Tmp})
		tmp := g.newTemp()
		fmt.Fprintf(g.b, "\tVAR\t%%%s\t^float32\n", tmp)
		g.writeCall("%"+tmp, "?float32", []string{"%" + raw64Tmp})
		return "%" + tmp, nil
	}
	tmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^float64\n", tmp)
	g.writeCall("%"+tmp, "?math.Pow", []string{baseVal, expVal})
	return "%" + tmp, nil
}

// genUnwrapValue emits `unwrap[T](t) -> T` (amifl-spec.md section 13.9):
// FGET the payload and error, panic (Go's own builtin, called via CALL
// exactly like `len`/`delete` above) if the error is non-nil, otherwise
// yield the payload — a prototyping-only escape hatch, per the spec's own
// description.
func (g *gen) genUnwrapValue(c *ast.CallExpr) (string, error) {
	tVal, err := g.genValue(c.Args[0])
	if err != nil {
		return "", err
	}
	payloadGoType := g.prog.resolveGoType(c.ResolvedType)
	payloadTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", payloadTmp, payloadGoType)
	fmt.Fprintf(g.b, "\tFGET\t%%%s\t%s\t>F0\n", payloadTmp, tVal)
	errGoType := g.prog.resolveGoType("Error")
	errTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", errTmp, errGoType)
	fmt.Fprintf(g.b, "\tFGET\t%%%s\t%s\t>F1\n", errTmp, tVal)

	condTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^bool\n", condTmp)
	fmt.Fprintf(g.b, "\tNEQ\t%%%s\t%%%s\tnil\n", condTmp, errTmp)
	fmt.Fprintf(g.b, "\tIF\t%%%s\n", condTmp)
	g.writeCall("", "?panic", []string{"%" + errTmp})
	g.b.WriteString("\tENDIF\n")

	return "%" + payloadTmp, nil
}

// genOkOrValue emits `okOr[T](t, default) -> T` (amifl-spec.md section
// 13.9): the payload if t's error is nil, default otherwise.
func (g *gen) genOkOrValue(c *ast.CallExpr) (string, error) {
	tVal, err := g.genValue(c.Args[0])
	if err != nil {
		return "", err
	}
	defaultVal, err := g.genValue(c.Args[1])
	if err != nil {
		return "", err
	}
	payloadGoType := g.prog.resolveGoType(c.ResolvedType)
	payloadTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", payloadTmp, payloadGoType)
	fmt.Fprintf(g.b, "\tFGET\t%%%s\t%s\t>F0\n", payloadTmp, tVal)
	errGoType := g.prog.resolveGoType("Error")
	errTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", errTmp, errGoType)
	fmt.Fprintf(g.b, "\tFGET\t%%%s\t%s\t>F1\n", errTmp, tVal)

	condTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^bool\n", condTmp)
	fmt.Fprintf(g.b, "\tNEQ\t%%%s\t%%%s\tnil\n", condTmp, errTmp)

	resultTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", resultTmp, payloadGoType)
	fmt.Fprintf(g.b, "\tIF\t%%%s\n", condTmp)
	fmt.Fprintf(g.b, "\tSET\t%%%s\t%s\n", resultTmp, defaultVal)
	g.b.WriteString("\tELSE\n")
	fmt.Fprintf(g.b, "\tSET\t%%%s\t%%%s\n", resultTmp, payloadTmp)
	g.b.WriteString("\tENDIF\n")
	return "%" + resultTmp, nil
}
