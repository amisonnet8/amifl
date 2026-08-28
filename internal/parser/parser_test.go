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

// parseStmtExpr parses stmtSrc as the sole non-return statement of a
// function body (its trailing `0` return is fixed), returning that
// statement's Expr node — for tests that care about exactly what a
// statement-position expression (assignment, in particular) parses to.
func parseStmtExpr(t *testing.T, stmtSrc string) ast.Expr {
	t.Helper()
	full := "fn main() -> Int {\n    " + stmtSrc + "\n    0\n}\n"
	f, err := Parse(full)
	if err != nil {
		t.Fatalf("Parse(%q) error: %v", stmtSrc, err)
	}
	return parseFuncMain(t, f).Body.Exprs[0]
}

// parseExprSrc parses exprSrc as a discarded value expression (`_ =
// exprSrc`), returning the parsed Expr — for tests that only care about
// how a value expression (never itself assignment) parses.
func parseExprSrc(t *testing.T, exprSrc string) ast.Expr {
	t.Helper()
	return parseStmtExpr(t, "_ = "+exprSrc).(*ast.DiscardExpr).Value
}

func TestParse_ArithmeticPrecedence(t *testing.T) {
	// 1 + 2 * 3 must read as 1 + (2 * 3), not (1 + 2) * 3.
	expr := parseExprSrc(t, "1 + 2 * 3")
	bin, ok := expr.(*ast.BinaryExpr)
	if !ok || bin.Op != "+" {
		t.Fatalf("got %#v, want top-level BinaryExpr{Op: \"+\"}", expr)
	}
	if _, ok := bin.Left.(*ast.IntLit); !ok {
		t.Fatalf("left: got %T, want *ast.IntLit", bin.Left)
	}
	rightBin, ok := bin.Right.(*ast.BinaryExpr)
	if !ok || rightBin.Op != "*" {
		t.Fatalf("right: got %#v, want BinaryExpr{Op: \"*\"}", bin.Right)
	}
}

func TestParse_SubtractionIsLeftAssociative(t *testing.T) {
	// 1 - 2 - 3 must read as (1 - 2) - 3, not 1 - (2 - 3).
	expr := parseExprSrc(t, "1 - 2 - 3")
	bin, ok := expr.(*ast.BinaryExpr)
	if !ok || bin.Op != "-" {
		t.Fatalf("got %#v, want top-level BinaryExpr{Op: \"-\"}", expr)
	}
	if _, ok := bin.Right.(*ast.IntLit); !ok {
		t.Fatalf("right: got %T, want *ast.IntLit (the associativity is wrong if it's a BinaryExpr)", bin.Right)
	}
	leftBin, ok := bin.Left.(*ast.BinaryExpr)
	if !ok || leftBin.Op != "-" {
		t.Fatalf("left: got %#v, want BinaryExpr{Op: \"-\"}", bin.Left)
	}
}

func TestParse_ComparisonBindsTighterThanEquality(t *testing.T) {
	// amifl-spec.md section 6: `< <= > >=` sits above `== !=` in the
	// precedence table, so 1 < 2 == true reads as (1 < 2) == true.
	expr := parseExprSrc(t, "1 < 2 == true")
	bin, ok := expr.(*ast.BinaryExpr)
	if !ok || bin.Op != "==" {
		t.Fatalf("got %#v, want top-level BinaryExpr{Op: \"==\"}", expr)
	}
	leftBin, ok := bin.Left.(*ast.BinaryExpr)
	if !ok || leftBin.Op != "<" {
		t.Fatalf("left: got %#v, want BinaryExpr{Op: \"<\"}", bin.Left)
	}
}

func TestParse_ParensOverridePrecedence(t *testing.T) {
	// (1 + 2) * 3 must read as (1 + 2) * 3, not 1 + (2 * 3).
	expr := parseExprSrc(t, "(1 + 2) * 3")
	bin, ok := expr.(*ast.BinaryExpr)
	if !ok || bin.Op != "*" {
		t.Fatalf("got %#v, want top-level BinaryExpr{Op: \"*\"}", expr)
	}
	if _, ok := bin.Left.(*ast.BinaryExpr); !ok {
		t.Fatalf("left: got %T, want *ast.BinaryExpr (grouping should have survived)", bin.Left)
	}
}

func TestParse_UnaryMinusIsRightAssociative(t *testing.T) {
	expr := parseExprSrc(t, "- -5")
	outer, ok := expr.(*ast.UnaryExpr)
	if !ok || outer.Op != "-" {
		t.Fatalf("got %#v, want top-level UnaryExpr{Op: \"-\"}", expr)
	}
	if _, ok := outer.Operand.(*ast.UnaryExpr); !ok {
		t.Fatalf("operand: got %T, want *ast.UnaryExpr", outer.Operand)
	}
}

func TestParse_UnaryBindsTighterThanBinary(t *testing.T) {
	// -1 + 2 must read as (-1) + 2, not -(1 + 2).
	expr := parseExprSrc(t, "-1 + 2")
	bin, ok := expr.(*ast.BinaryExpr)
	if !ok || bin.Op != "+" {
		t.Fatalf("got %#v, want top-level BinaryExpr{Op: \"+\"}", expr)
	}
	if _, ok := bin.Left.(*ast.UnaryExpr); !ok {
		t.Fatalf("left: got %T, want *ast.UnaryExpr", bin.Left)
	}
}

func TestParse_CallAsBinaryOperand(t *testing.T) {
	expr := parseExprSrc(t, "f(1) + 2")
	bin, ok := expr.(*ast.BinaryExpr)
	if !ok || bin.Op != "+" {
		t.Fatalf("got %#v, want top-level BinaryExpr{Op: \"+\"}", expr)
	}
	if _, ok := bin.Left.(*ast.CallExpr); !ok {
		t.Fatalf("left: got %T, want *ast.CallExpr", bin.Left)
	}
}

func TestParse_AssignWithBinaryRHS(t *testing.T) {
	stmt := parseStmtExpr(t, "x = 1 + 2")
	assign, ok := stmt.(*ast.AssignExpr)
	if !ok {
		t.Fatalf("got %T, want *ast.AssignExpr", stmt)
	}
	if _, ok := assign.Value.(*ast.BinaryExpr); !ok {
		t.Fatalf("value: got %T, want *ast.BinaryExpr", assign.Value)
	}
}

func TestParse_IdentStartsABinaryExpr(t *testing.T) {
	// A bare identifier not immediately followed by '=' must still be
	// usable as the left-hand side of a binary expression (x + 1), not
	// just a call or standalone read.
	expr := parseExprSrc(t, "x + 1")
	bin, ok := expr.(*ast.BinaryExpr)
	if !ok || bin.Op != "+" {
		t.Fatalf("got %#v, want top-level BinaryExpr{Op: \"+\"}", expr)
	}
	if _, ok := bin.Left.(*ast.IdentExpr); !ok {
		t.Fatalf("left: got %T, want *ast.IdentExpr", bin.Left)
	}
}

func TestParse_AllOperatorTokensParse(t *testing.T) {
	for _, src := range []string{
		"1 + 2", "1 - 2", "1 * 2", "1 / 2", "1 % 2",
		"1 & 2", "1 | 2", "1 ^ 2", "1 &^ 2",
		"1 << 2", "1 >> 2",
		"true && false", "true || false",
		"1 == 2", "1 != 2", "1 < 2", "1 <= 2", "1 > 2", "1 >= 2",
		"!true", "-1", "~1",
	} {
		if _, err := Parse("fn main() -> Int {\n    _ = " + src + "\n    0\n}\n"); err != nil {
			t.Errorf("Parse(%q) error: %v", src, err)
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
