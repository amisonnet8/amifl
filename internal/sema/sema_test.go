package sema

import (
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

func printStr(s string) *ast.CallExpr {
	return &ast.CallExpr{Callee: "print", Args: []ast.Expr{&ast.StringLit{Value: s}}}
}

func TestCheck_HelloWorldIsValid(t *testing.T) {
	f := mainFile(printStr("Hello, AmiFL!"), &ast.IntLit{Value: 0})
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_MissingMainIsAnError(t *testing.T) {
	if err := Check(&ast.File{}); err == nil {
		t.Fatal("expected an error for a missing fn main")
	}
}

func TestCheck_DuplicateMainIsAnError(t *testing.T) {
	f := mainFile(&ast.IntLit{Value: 0})
	f.Decls = append(f.Decls, f.Decls[0])
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a duplicate fn main")
	}
}

func TestCheck_NonMainTopLevelFuncIsAnError(t *testing.T) {
	f := mainFile(&ast.IntLit{Value: 0})
	f.Decls = append(f.Decls, &ast.FuncDecl{
		Name:       "helper",
		ReturnType: "Int",
		Body:       &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 0}}},
	})
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a non-main top-level fn (step 2 limitation)")
	}
}

func TestCheck_WrongReturnTypeIsAnError(t *testing.T) {
	f := mainFile(&ast.IntLit{Value: 0})
	f.Decls[0].(*ast.FuncDecl).ReturnType = "String"
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a non-Int return type")
	}
}

func TestCheck_UnknownReturnTypeIsAnError(t *testing.T) {
	f := mainFile(&ast.IntLit{Value: 0})
	f.Decls[0].(*ast.FuncDecl).ReturnType = "Nope"
	if err := Check(f); err == nil {
		t.Fatal("expected an error for an unknown return type")
	}
}

func TestCheck_EmptyBodyIsAnError(t *testing.T) {
	if err := Check(mainFile()); err == nil {
		t.Fatal("expected an error for an empty body (Unit != Int)")
	}
}

func TestCheck_LastExprMustMatchReturnType(t *testing.T) {
	if err := Check(mainFile(&ast.StringLit{Value: "nope"})); err == nil {
		t.Fatal("expected an error when the last expression isn't Int-typed")
	}
}

func TestCheck_NonPrintCallIsAnError(t *testing.T) {
	f := mainFile(&ast.CallExpr{Callee: "eprint", Args: []ast.Expr{&ast.StringLit{Value: "x"}}}, &ast.IntLit{Value: 0})
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a non-print call (step 2 limitation)")
	}
}

func TestCheck_PrintWithNonStringArgIsAnError(t *testing.T) {
	f := mainFile(&ast.CallExpr{Callee: "print", Args: []ast.Expr{&ast.IntLit{Value: 1}}}, &ast.IntLit{Value: 0})
	if err := Check(f); err == nil {
		t.Fatal("expected an error for print(non-string)")
	}
}

func TestCheck_NonUnitNonFinalExprIsAnError(t *testing.T) {
	f := mainFile(&ast.IntLit{Value: 1}, &ast.IntLit{Value: 0})
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a stray non-Unit expression in non-final position")
	}
}

func TestCheck_DiscardSilencesNonUnitNonFinalExpr(t *testing.T) {
	f := mainFile(&ast.DiscardExpr{Value: &ast.IntLit{Value: 1}}, &ast.IntLit{Value: 0})
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_LetWithTypeAnnotationAndUse(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "x", Type: "Int", Value: &ast.IntLit{Value: 42}},
		&ast.IdentExpr{Name: "x"},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_LetInfersTypeFromLiteral(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "x", Value: &ast.FloatLit{Value: 1.5}}, // infers Float64
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	let := f.Decls[0].(*ast.FuncDecl).Body.Exprs[0].(*ast.LetExpr)
	if let.ResolvedType != "Float64" {
		t.Fatalf("got ResolvedType %q, want Float64", let.ResolvedType)
	}
}

func TestCheck_LetTypeMismatchIsAnError(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "x", Type: "Bool", Value: &ast.IntLit{Value: 5}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for let x: Bool = 5")
	}
}

func TestCheck_FloatLiteralCannotNarrowToIntType(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "x", Type: "Int", Value: &ast.FloatLit{Value: 1.5}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for let x: Int = 1.5")
	}
}

func TestCheck_IntLiteralFitsFloatType(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "x", Type: "Float", Value: &ast.IntLit{Value: 5}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_IntLiteralOverflowIsAnError(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "x", Type: "Int8", Value: &ast.IntLit{Value: 200}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for let x: Int8 = 200 (overflow)")
	}
}

func TestCheck_UnknownTypeAnnotationIsAnError(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "x", Type: "Nope", Value: &ast.IntLit{Value: 1}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for an unknown type annotation")
	}
}

func TestCheck_DuplicateLetInSameScopeIsAnError(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "x", Value: &ast.IntLit{Value: 1}},
		&ast.LetExpr{Name: "x", Value: &ast.IntLit{Value: 2}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for redeclaring `x` in the same scope")
	}
}

func TestCheck_UndefinedNameIsAnError(t *testing.T) {
	if err := Check(mainFile(&ast.IdentExpr{Name: "nope"})); err == nil {
		t.Fatal("expected an error for an undefined name")
	}
}

func TestCheck_AssignToUndeclaredIsAnError(t *testing.T) {
	f := mainFile(
		&ast.AssignExpr{Name: "x", Value: &ast.IntLit{Value: 1}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for assigning to an undeclared name")
	}
}

func TestCheck_AssignTypeMismatchIsAnError(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "x", Type: "Int", Value: &ast.IntLit{Value: 1}},
		&ast.AssignExpr{Name: "x", Value: &ast.StringLit{Value: "nope"}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for assigning a String to an Int variable")
	}
}

func TestCheck_ReassigningLetIsValid(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "x", Type: "Int", Value: &ast.IntLit{Value: 1}},
		&ast.AssignExpr{Name: "x", Value: &ast.IntLit{Value: 2}},
		&ast.IdentExpr{Name: "x"},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_ConstIsInlinedAtUseSite(t *testing.T) {
	f := mainFile(
		&ast.ConstDecl{Name: "X", Value: &ast.IntLit{Value: 7}},
		&ast.IdentExpr{Name: "X"},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	ident := f.Decls[0].(*ast.FuncDecl).Body.Exprs[1].(*ast.IdentExpr)
	lit, ok := ident.ConstValue.(*ast.IntLit)
	if !ok || lit.Value != 7 {
		t.Fatalf("got ConstValue %#v, want IntLit{Value: 7}", ident.ConstValue)
	}
}

func TestCheck_AssignToConstIsAnError(t *testing.T) {
	f := mainFile(
		&ast.ConstDecl{Name: "X", Value: &ast.IntLit{Value: 7}},
		&ast.AssignExpr{Name: "X", Value: &ast.IntLit{Value: 8}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for assigning to a const")
	}
}

func TestCheck_ConstReferencingLaterConstIsAnError(t *testing.T) {
	f := mainFile(&ast.IntLit{Value: 0})
	f.Decls = []ast.TopLevelDecl{
		&ast.ConstDecl{Name: "A", Value: &ast.IdentExpr{Name: "B"}},
		&ast.ConstDecl{Name: "B", Value: &ast.IntLit{Value: 1}},
		f.Decls[0],
	}
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a const referencing a later const")
	}
}

func TestCheck_ConstInitializerMustBeLiteralOrConstRef(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "x", Value: &ast.IntLit{Value: 1}},
		&ast.ConstDecl{Name: "Y", Value: &ast.IdentExpr{Name: "x"}}, // x is a let, not a const
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a const initializer referencing a `let` variable")
	}
}

func TestCheck_TopLevelConstVisibleInMain(t *testing.T) {
	f := &ast.File{
		Decls: []ast.TopLevelDecl{
			&ast.ConstDecl{Name: "Greeting", Type: "String", Value: &ast.StringLit{Value: "hi"}},
			&ast.FuncDecl{
				Name:       "main",
				ReturnType: "Int",
				Body: &ast.Block{Exprs: []ast.Expr{
					&ast.CallExpr{Callee: "print", Args: []ast.Expr{&ast.IdentExpr{Name: "Greeting"}}},
					&ast.IntLit{Value: 0},
				}},
			},
		},
	}
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_DuplicateTopLevelConstIsAnError(t *testing.T) {
	f := mainFile(&ast.IntLit{Value: 0})
	f.Decls = []ast.TopLevelDecl{
		&ast.ConstDecl{Name: "X", Value: &ast.IntLit{Value: 1}},
		&ast.ConstDecl{Name: "X", Value: &ast.IntLit{Value: 2}},
		f.Decls[0],
	}
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a duplicate top-level const")
	}
}

func TestCheck_LetBoundToUnitIsAnError(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "x", Value: printStr("a")},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for binding a let to a Unit-typed value")
	}
}
