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
