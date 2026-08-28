package sema

import (
	"testing"

	"github.com/amisonnet8/amifl/internal/ast"
)

func helloFile() *ast.File {
	return &ast.File{
		Funcs: []*ast.FuncDecl{
			{
				Name:       "main",
				ReturnType: "Int",
				Body: &ast.Block{
					Exprs: []ast.Expr{
						&ast.CallExpr{Callee: "print", Args: []ast.Expr{&ast.StringLit{Value: "Hello, AmiFL!"}}},
						&ast.IntLit{Value: 0},
					},
				},
			},
		},
	}
}

func TestCheck_HelloWorldIsValid(t *testing.T) {
	if err := Check(helloFile()); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_MissingMainIsAnError(t *testing.T) {
	f := &ast.File{}
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a missing fn main")
	}
}

func TestCheck_DuplicateMainIsAnError(t *testing.T) {
	f := helloFile()
	f.Funcs = append(f.Funcs, f.Funcs[0])
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a duplicate fn main")
	}
}

func TestCheck_WrongReturnTypeIsAnError(t *testing.T) {
	f := helloFile()
	f.Funcs[0].ReturnType = "String"
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a non-Int return type")
	}
}

func TestCheck_EmptyBodyIsAnError(t *testing.T) {
	f := helloFile()
	f.Funcs[0].Body.Exprs = nil
	if err := Check(f); err == nil {
		t.Fatal("expected an error for an empty body")
	}
}

func TestCheck_LastExprMustBeIntLit(t *testing.T) {
	f := helloFile()
	f.Funcs[0].Body.Exprs = []ast.Expr{&ast.StringLit{Value: "not an int"}}
	if err := Check(f); err == nil {
		t.Fatal("expected an error when the last expression isn't an Int literal")
	}
}

func TestCheck_NonPrintCallIsAnError(t *testing.T) {
	f := helloFile()
	f.Funcs[0].Body.Exprs = []ast.Expr{
		&ast.CallExpr{Callee: "eprint", Args: []ast.Expr{&ast.StringLit{Value: "x"}}},
		&ast.IntLit{Value: 0},
	}
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a non-print call")
	}
}

func TestCheck_PrintWithNonStringArgIsAnError(t *testing.T) {
	f := helloFile()
	f.Funcs[0].Body.Exprs = []ast.Expr{
		&ast.CallExpr{Callee: "print", Args: []ast.Expr{&ast.IntLit{Value: 1}}},
		&ast.IntLit{Value: 0},
	}
	if err := Check(f); err == nil {
		t.Fatal("expected an error for print(non-string)")
	}
}
