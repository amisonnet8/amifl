package codegen

import (
	"strings"
	"testing"

	"github.com/amisonnet8/amifl/internal/ast"
)

func mainFile(exprs ...ast.Expr) *ast.File {
	return &ast.File{
		Decls: []ast.TopLevelDecl{
			&ast.FuncDecl{
				Name:       "main",
				ReturnType: "Int",
				Body:       &ast.Block{Exprs: exprs},
			},
		},
	}
}

func TestGenerate_HelloWorld(t *testing.T) {
	f := mainFile(
		&ast.CallExpr{Callee: "print", Args: []ast.Expr{&ast.StringLit{Value: "Hello, AmiFL!"}}},
		&ast.IntLit{Value: 0},
	)

	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	// Structural spot-checks rather than an exact-text comparison, so
	// unrelated formatting changes don't make this test brittle
	// (CLAUDE.md's testing precedent from Seed/Cascade/Weave).
	for _, want := range []string{
		"FUNC\t!amifl_main\t:\t^int64",
		`CALL	:	?fmt.Println	"Hello, AmiFL!"`,
		"RET\t0",
		"ENDFUNC",
		"FUNC\t!main\t:",
		"CALL\t%code\t:\t!amifl_main",
		"CALL\t%exitCode\t:\t?int\t%code",
		"CALL\t:\t?os.Exit\t%exitCode",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
}

func TestGenerate_NoMainIsAnError(t *testing.T) {
	if _, err := Generate(&ast.File{}); err == nil {
		t.Fatal("expected an error when there is no fn main")
	}
}

func TestGenerate_StringWithQuoteIsEscaped(t *testing.T) {
	f := mainFile(
		&ast.CallExpr{Callee: "print", Args: []ast.Expr{&ast.StringLit{Value: `say "hi"`}}},
		&ast.IntLit{Value: 0},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if !strings.Contains(ir, `"say \"hi\""`) {
		t.Errorf("expected an escaped string literal in IR, got:\n%s", ir)
	}
}

func TestGenerate_LetEmitsVarAndSet(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "x", ResolvedType: "Int64", Value: &ast.IntLit{Value: 42}},
		&ast.IdentExpr{Name: "x", ResolvedType: "Int64"},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{
		"VAR\t%x\t^int64",
		"SET\t%x\t42",
		"RET\t%x",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
}

func TestGenerate_AssignEmitsSet(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "x", ResolvedType: "Int64", Value: &ast.IntLit{Value: 1}},
		&ast.AssignExpr{Name: "x", Value: &ast.IntLit{Value: 2}},
		&ast.IdentExpr{Name: "x", ResolvedType: "Int64"},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if !strings.Contains(ir, "SET\t%x\t2") {
		t.Errorf("generated IR missing reassignment SET; got:\n%s", ir)
	}
}

func TestGenerate_ConstIsInlinedNotDeclared(t *testing.T) {
	lit := &ast.IntLit{Value: 7}
	f := mainFile(
		&ast.IdentExpr{Name: "X", ResolvedType: "Int64", ConstValue: lit},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if strings.Contains(ir, "VAR\t%X") {
		t.Errorf("const should not get a VAR declaration; got:\n%s", ir)
	}
	if !strings.Contains(ir, "RET\t7") {
		t.Errorf("expected the const's literal value inlined into RET; got:\n%s", ir)
	}
}

func TestGenerate_FloatLiteralAlwaysHasADecimalPoint(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "x", ResolvedType: "Float64", Value: &ast.FloatLit{Value: 5}},
		&ast.IntLit{Value: 0},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if !strings.Contains(ir, "SET\t%x\t5.0") {
		t.Errorf("expected a whole-number float literal to print with a decimal point; got:\n%s", ir)
	}
}

func TestGenerate_BoolLiteral(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "ok", ResolvedType: "Bool", Value: &ast.BoolLit{Value: true}},
		&ast.IntLit{Value: 0},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if !strings.Contains(ir, "VAR\t%ok\t^bool") || !strings.Contains(ir, "SET\t%ok\ttrue") {
		t.Errorf("generated IR missing bool VAR/SET; got:\n%s", ir)
	}
}

func TestGenerate_BinaryExprEmitsTempAndInstruction(t *testing.T) {
	f := mainFile(
		&ast.BinaryExpr{Op: "+", Left: &ast.IntLit{Value: 1}, Right: &ast.IntLit{Value: 2}, ResolvedType: "Int64"},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{
		"VAR\t%amifl_tmp1\t^int64",
		"ADD\t%amifl_tmp1\t1\t2",
		"RET\t%amifl_tmp1",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
}

func TestGenerate_StringPlusEmitsConcat(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "s", ResolvedType: "String", Value: &ast.BinaryExpr{
			Op: "+", Left: &ast.StringLit{Value: "a"}, Right: &ast.StringLit{Value: "b"}, ResolvedType: "String",
		}},
		&ast.IntLit{Value: 0},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if !strings.Contains(ir, `CONCAT	%amifl_tmp1	"a"	"b"`) {
		t.Errorf("generated IR missing CONCAT; got:\n%s", ir)
	}
}

func TestGenerate_ComparisonDeclaresBoolTemp(t *testing.T) {
	f := mainFile(
		&ast.BinaryExpr{Op: "<", Left: &ast.IntLit{Value: 1}, Right: &ast.IntLit{Value: 2}, ResolvedType: "Int64"},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if !strings.Contains(ir, "VAR\t%amifl_tmp1\t^bool") || !strings.Contains(ir, "LT\t%amifl_tmp1\t1\t2") {
		t.Errorf("generated IR missing bool-typed LT temp; got:\n%s", ir)
	}
}

func TestGenerate_UnaryMinusOfLiteralInlinesAsNegativeLiteral(t *testing.T) {
	f := mainFile(
		&ast.UnaryExpr{Op: "-", Operand: &ast.IntLit{Value: 5}, ResolvedType: "Int64"},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if !strings.Contains(ir, "RET\t-5") {
		t.Errorf("expected -5 to inline as a bare negative literal (no VAR/SUB); got:\n%s", ir)
	}
	if strings.Contains(ir, "SUB") {
		t.Errorf("did not expect a SUB instruction for negating a literal; got:\n%s", ir)
	}
}

func TestGenerate_DoubleUnaryMinusOfLiteralCollapses(t *testing.T) {
	f := mainFile(
		&ast.UnaryExpr{Op: "-", ResolvedType: "Int64", Operand: &ast.UnaryExpr{
			Op: "-", Operand: &ast.IntLit{Value: 5}, ResolvedType: "Int64",
		}},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if !strings.Contains(ir, "RET\t5") || strings.Contains(ir, "RET\t--5") {
		t.Errorf("expected -(-5) to collapse to a bare 5; got:\n%s", ir)
	}
}

func TestGenerate_UnaryMinusOfVariableEmitsSub(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "x", ResolvedType: "Int64", Value: &ast.IntLit{Value: 5}},
		&ast.UnaryExpr{Op: "-", Operand: &ast.IdentExpr{Name: "x", ResolvedType: "Int64"}, ResolvedType: "Int64"},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if !strings.Contains(ir, "SUB\t%amifl_tmp1\t0\t%x") {
		t.Errorf("expected negating a variable to emit SUB 0 %%x; got:\n%s", ir)
	}
}

func TestGenerate_BitwiseNotEmitsBnot(t *testing.T) {
	f := mainFile(
		&ast.UnaryExpr{Op: "~", Operand: &ast.IntLit{Value: 5}, ResolvedType: "Int64"},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if !strings.Contains(ir, "BNOT\t%amifl_tmp1\t5") {
		t.Errorf("generated IR missing BNOT; got:\n%s", ir)
	}
}

func TestGenerate_NestedBinaryExprChainsTemps(t *testing.T) {
	// 1 + 2 * 3 should compute the multiplication into one temp and then
	// the addition into a second temp that references it.
	f := mainFile(
		&ast.BinaryExpr{
			Op: "+", ResolvedType: "Int64",
			Left: &ast.IntLit{Value: 1},
			Right: &ast.BinaryExpr{
				Op: "*", ResolvedType: "Int64",
				Left: &ast.IntLit{Value: 2}, Right: &ast.IntLit{Value: 3},
			},
		},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{
		"MUL\t%amifl_tmp1\t2\t3",
		"ADD\t%amifl_tmp2\t1\t%amifl_tmp1",
		"RET\t%amifl_tmp2",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
}

func TestGenerate_ConstOperatorExpressionRegeneratesAtUseSite(t *testing.T) {
	// A const whose initializer is an operator expression has no runtime
	// storage of its own (CLAUDE.md's "確定した設計判断"): each reference
	// re-emits the same computation rather than reading a %ConstName var.
	sum := &ast.BinaryExpr{Op: "+", ResolvedType: "Int64", Left: &ast.IntLit{Value: 40}, Right: &ast.IntLit{Value: 2}}
	f := mainFile(
		&ast.IdentExpr{Name: "X", ResolvedType: "Int64", ConstValue: sum},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if strings.Contains(ir, "VAR\t%X") {
		t.Errorf("const should not get a VAR declaration; got:\n%s", ir)
	}
	if !strings.Contains(ir, "ADD\t%amifl_tmp1\t40\t2") || !strings.Contains(ir, "RET\t%amifl_tmp1") {
		t.Errorf("expected the const's operator expression regenerated inline; got:\n%s", ir)
	}
}

func TestGenerate_DiscardOfLiteralEmitsNothingExtra(t *testing.T) {
	withDiscard := mainFile(
		&ast.DiscardExpr{Value: &ast.IntLit{Value: 1}},
		&ast.IntLit{Value: 0},
	)
	withoutDiscard := mainFile(&ast.IntLit{Value: 0})

	irWith, err := Generate(withDiscard)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	irWithout, err := Generate(withoutDiscard)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if irWith != irWithout {
		t.Errorf("discarding a side-effect-free literal should generate no extra IR;\nwith:\n%s\nwithout:\n%s", irWith, irWithout)
	}
}
