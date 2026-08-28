package parser

import (
	"testing"

	"github.com/amisonnet8/amifl/internal/ast"
)

func TestParse_HelloWorld(t *testing.T) {
	src := "fn main() -> Int {\n" +
		"    print(\"Hello, AmiFL!\")\n" +
		"    0\n" +
		"}\n"

	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if len(f.Funcs) != 1 {
		t.Fatalf("got %d funcs, want 1", len(f.Funcs))
	}
	fn := f.Funcs[0]
	if fn.Name != "main" || fn.ReturnType != "Int" {
		t.Fatalf("got FuncDecl{Name: %q, ReturnType: %q}", fn.Name, fn.ReturnType)
	}
	if len(fn.Body.Exprs) != 2 {
		t.Fatalf("got %d body exprs, want 2", len(fn.Body.Exprs))
	}

	call, ok := fn.Body.Exprs[0].(*ast.CallExpr)
	if !ok {
		t.Fatalf("body[0]: got %T, want *ast.CallExpr", fn.Body.Exprs[0])
	}
	if call.Callee != "print" || len(call.Args) != 1 {
		t.Fatalf("got CallExpr{Callee: %q, len(Args): %d}", call.Callee, len(call.Args))
	}
	str, ok := call.Args[0].(*ast.StringLit)
	if !ok || str.Value != "Hello, AmiFL!" {
		t.Fatalf("got print arg %#v, want StringLit{Value: \"Hello, AmiFL!\"}", call.Args[0])
	}

	last, ok := fn.Body.Exprs[1].(*ast.IntLit)
	if !ok || last.Value != 0 {
		t.Fatalf("got body[1] %#v, want IntLit{Value: 0}", fn.Body.Exprs[1])
	}
}

func TestParse_MultiArgCall(t *testing.T) {
	src := "fn main() -> Int {\n" +
		"    f(1, 2, 3)\n" +
		"    0\n" +
		"}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	call := f.Funcs[0].Body.Exprs[0].(*ast.CallExpr)
	if len(call.Args) != 3 {
		t.Fatalf("got %d args, want 3", len(call.Args))
	}
}

func TestParse_MissingArrowIsAnError(t *testing.T) {
	if _, err := Parse("fn main() Int { 0 }"); err == nil {
		t.Fatal("expected an error for a missing '->'")
	}
}

func TestParse_UnclosedBlockIsAnError(t *testing.T) {
	if _, err := Parse("fn main() -> Int {\n0\n"); err == nil {
		t.Fatal("expected an error for an unclosed block")
	}
}

func TestParse_MissingNewlineBetweenExprsIsAnError(t *testing.T) {
	if _, err := Parse("fn main() -> Int { print(\"a\") 0 }"); err == nil {
		t.Fatal("expected an error when expressions on one line aren't newline-separated")
	}
}
