package parser

import (
	"testing"

	"github.com/amisonnet8/amifl/internal/ast"
)

func parseFuncMain(t *testing.T, f *ast.File) *ast.FuncDecl {
	t.Helper()
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name == "main" {
			return fn
		}
	}
	t.Fatal("no fn main found")
	return nil
}

func TestParse_HelloWorld(t *testing.T) {
	src := "fn main() -> Int {\n" +
		"    print(\"Hello, AmiFL!\")\n" +
		"    0\n" +
		"}\n"

	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if len(f.Decls) != 1 {
		t.Fatalf("got %d decls, want 1", len(f.Decls))
	}
	fn := parseFuncMain(t, f)
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
	call := parseFuncMain(t, f).Body.Exprs[0].(*ast.CallExpr)
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

func TestParse_LetExprWithTypeAnnotation(t *testing.T) {
	src := "fn main() -> Int {\n" +
		"    let x: Int = 42\n" +
		"    x\n" +
		"}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	let, ok := parseFuncMain(t, f).Body.Exprs[0].(*ast.LetExpr)
	if !ok {
		t.Fatalf("body[0]: got %T, want *ast.LetExpr", parseFuncMain(t, f).Body.Exprs[0])
	}
	if let.Name != "x" || let.Type != "Int" {
		t.Fatalf("got LetExpr{Name: %q, Type: %q}", let.Name, let.Type)
	}
	lit, ok := let.Value.(*ast.IntLit)
	if !ok || lit.Value != 42 {
		t.Fatalf("got let value %#v, want IntLit{Value: 42}", let.Value)
	}

	ident, ok := parseFuncMain(t, f).Body.Exprs[1].(*ast.IdentExpr)
	if !ok || ident.Name != "x" {
		t.Fatalf("body[1]: got %#v, want IdentExpr{Name: \"x\"}", parseFuncMain(t, f).Body.Exprs[1])
	}
}

func TestParse_LetExprWithoutTypeAnnotation(t *testing.T) {
	src := "fn main() -> Int {\n    let x = 42\n    x\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	let := parseFuncMain(t, f).Body.Exprs[0].(*ast.LetExpr)
	if let.Type != "" {
		t.Fatalf("got Type %q, want empty (inferred)", let.Type)
	}
}

func TestParse_ConstDeclAtTopLevel(t *testing.T) {
	src := "const Pi: Float = 3.14\n\nfn main() -> Int {\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if len(f.Decls) != 2 {
		t.Fatalf("got %d decls, want 2", len(f.Decls))
	}
	cd, ok := f.Decls[0].(*ast.ConstDecl)
	if !ok {
		t.Fatalf("decl[0]: got %T, want *ast.ConstDecl", f.Decls[0])
	}
	if cd.Name != "Pi" || cd.Type != "Float" {
		t.Fatalf("got ConstDecl{Name: %q, Type: %q}", cd.Name, cd.Type)
	}
}

func TestParse_ConstDeclInsideBlock(t *testing.T) {
	src := "fn main() -> Int {\n    const X = 1\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if _, ok := parseFuncMain(t, f).Body.Exprs[0].(*ast.ConstDecl); !ok {
		t.Fatalf("body[0]: got %T, want *ast.ConstDecl", parseFuncMain(t, f).Body.Exprs[0])
	}
}

func TestParse_TopLevelLetIsAnError(t *testing.T) {
	if _, err := Parse("let x = 1\n\nfn main() -> Int {\n    0\n}\n"); err == nil {
		t.Fatal("expected an error for a top-level `let`")
	}
}

func TestParse_AssignExpr(t *testing.T) {
	src := "fn main() -> Int {\n    let x = 1\n    x = 2\n    x\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	assign, ok := parseFuncMain(t, f).Body.Exprs[1].(*ast.AssignExpr)
	if !ok {
		t.Fatalf("body[1]: got %T, want *ast.AssignExpr", parseFuncMain(t, f).Body.Exprs[1])
	}
	if assign.Name != "x" {
		t.Fatalf("got AssignExpr{Name: %q}", assign.Name)
	}
}

func TestParse_DiscardExpr(t *testing.T) {
	src := "fn main() -> Int {\n    _ = 1\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	discard, ok := parseFuncMain(t, f).Body.Exprs[0].(*ast.DiscardExpr)
	if !ok {
		t.Fatalf("body[0]: got %T, want *ast.DiscardExpr", parseFuncMain(t, f).Body.Exprs[0])
	}
	if _, ok := discard.Value.(*ast.IntLit); !ok {
		t.Fatalf("got discard value %#v, want IntLit", discard.Value)
	}
}

func TestParse_BoolLiterals(t *testing.T) {
	src := "fn main() -> Int {\n    let a = true\n    let b = false\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	exprs := parseFuncMain(t, f).Body.Exprs
	a := exprs[0].(*ast.LetExpr).Value.(*ast.BoolLit)
	b := exprs[1].(*ast.LetExpr).Value.(*ast.BoolLit)
	if !a.Value || b.Value {
		t.Fatalf("got a=%v b=%v, want a=true b=false", a.Value, b.Value)
	}
}

func TestParse_FloatLiterals(t *testing.T) {
	for _, src := range []string{"3.14", "1.23e4", "1.5e-3", "2E2"} {
		full := "fn main() -> Int {\n    let x = " + src + "\n    0\n}\n"
		f, err := Parse(full)
		if err != nil {
			t.Fatalf("Parse(%q) error: %v", src, err)
		}
		let := parseFuncMain(t, f).Body.Exprs[0].(*ast.LetExpr)
		if _, ok := let.Value.(*ast.FloatLit); !ok {
			t.Fatalf("Parse(%q): got %T, want *ast.FloatLit", src, let.Value)
		}
	}
}

func TestParse_IntLiteralIsNotFloat(t *testing.T) {
	f, err := Parse("fn main() -> Int {\n    let x = 42\n    0\n}\n")
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	let := parseFuncMain(t, f).Body.Exprs[0].(*ast.LetExpr)
	if _, ok := let.Value.(*ast.IntLit); !ok {
		t.Fatalf("got %T, want *ast.IntLit", let.Value)
	}
}
