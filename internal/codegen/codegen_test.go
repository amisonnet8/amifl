package codegen

import (
	"strings"
	"testing"

	"github.com/amisonnet8/amifl/internal/ast"
)

func TestGenerate_HelloWorld(t *testing.T) {
	f := &ast.File{
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
	f := &ast.File{
		Funcs: []*ast.FuncDecl{
			{
				Name:       "main",
				ReturnType: "Int",
				Body: &ast.Block{
					Exprs: []ast.Expr{
						&ast.CallExpr{Callee: "print", Args: []ast.Expr{&ast.StringLit{Value: `say "hi"`}}},
						&ast.IntLit{Value: 0},
					},
				},
			},
		},
	}
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if !strings.Contains(ir, `"say \"hi\""`) {
		t.Errorf("expected an escaped string literal in IR, got:\n%s", ir)
	}
}
