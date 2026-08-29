package parser

import (
	"testing"

	"github.com/amisonnet8/amifl/internal/ast"
)

// namedTypeName returns te's name if it's a plain *ast.NamedType, or ""
// otherwise — a test-only convenience so assertions can keep comparing
// against a bare string the way they did before step 7 introduced
// ast.TypeExpr, for every test here that only ever parses a scalar/struct
// name (never List[...]/Array[...;...]).
func namedTypeName(te ast.TypeExpr) string {
	if n, ok := te.(*ast.NamedType); ok {
		return n.Name
	}
	return ""
}

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
	if fn.Name != "main" || namedTypeName(fn.ReturnType) != "Int" {
		t.Fatalf("got FuncDecl{Name: %q, ReturnType: %v}", fn.Name, fn.ReturnType)
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
	if let.Name != "x" || namedTypeName(let.Type) != "Int" {
		t.Fatalf("got LetExpr{Name: %q, Type: %v}", let.Name, let.Type)
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
	if let.Type != nil {
		t.Fatalf("got Type %v, want nil (inferred)", let.Type)
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
	if cd.Name != "Pi" || namedTypeName(cd.Type) != "Float" {
		t.Fatalf("got ConstDecl{Name: %q, Type: %v}", cd.Name, cd.Type)
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

func TestParse_IfWithoutElse(t *testing.T) {
	expr := parseStmtExpr(t, "if true { 1 }")
	ifExpr, ok := expr.(*ast.IfExpr)
	if !ok {
		t.Fatalf("got %T, want *ast.IfExpr", expr)
	}
	if ifExpr.Else != nil {
		t.Fatalf("got Else %#v, want nil", ifExpr.Else)
	}
	if len(ifExpr.Then.Exprs) != 1 {
		t.Fatalf("got %d then-exprs, want 1", len(ifExpr.Then.Exprs))
	}
}

func TestParse_IfElse(t *testing.T) {
	expr := parseExprSrc(t, "if true { 1 } else { 2 }")
	ifExpr, ok := expr.(*ast.IfExpr)
	if !ok {
		t.Fatalf("got %T, want *ast.IfExpr", expr)
	}
	block, ok := ifExpr.Else.(*ast.Block)
	if !ok {
		t.Fatalf("Else: got %T, want *ast.Block", ifExpr.Else)
	}
	if len(block.Exprs) != 1 {
		t.Fatalf("got %d else-exprs, want 1", len(block.Exprs))
	}
}

func TestParse_ElifDesugarsToNestedElseIf(t *testing.T) {
	expr := parseExprSrc(t, "if a { 1 } elif b { 2 } else { 3 }")
	outer, ok := expr.(*ast.IfExpr)
	if !ok {
		t.Fatalf("got %T, want *ast.IfExpr", expr)
	}
	inner, ok := outer.Else.(*ast.IfExpr)
	if !ok {
		t.Fatalf("Else: got %T, want *ast.IfExpr (elif desugars to a nested if)", outer.Else)
	}
	if _, ok := inner.Cond.(*ast.IdentExpr); !ok {
		t.Fatalf("inner cond: got %T, want *ast.IdentExpr", inner.Cond)
	}
	if _, ok := inner.Else.(*ast.Block); !ok {
		t.Fatalf("inner Else: got %T, want *ast.Block", inner.Else)
	}
}

func TestParse_ElifOnANewLineIsAnError(t *testing.T) {
	// elif/else must directly follow the previous branch's closing '}' on
	// the same line (CLAUDE.md's "確定した設計判断"); on a line of its own
	// it dangles as an unexpected token at statement position.
	src := "fn main() -> Int {\n    if true {\n        1\n    }\n    elif false {\n        2\n    }\n    0\n}\n"
	if _, err := Parse(src); err == nil {
		t.Fatal("expected an error for 'elif' on its own line")
	}
}

func TestParse_WhileExpr(t *testing.T) {
	stmt := parseStmtExpr(t, "while true { break }")
	while, ok := stmt.(*ast.WhileExpr)
	if !ok {
		t.Fatalf("got %T, want *ast.WhileExpr", stmt)
	}
	if len(while.Body.Exprs) != 1 {
		t.Fatalf("got %d body exprs, want 1", len(while.Body.Exprs))
	}
	if _, ok := while.Body.Exprs[0].(*ast.BreakExpr); !ok {
		t.Fatalf("body[0]: got %T, want *ast.BreakExpr", while.Body.Exprs[0])
	}
}

func TestParse_BreakAndContinue(t *testing.T) {
	src := "fn main() -> Int {\n    while true {\n        break\n        continue\n    }\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	while := parseFuncMain(t, f).Body.Exprs[0].(*ast.WhileExpr)
	if _, ok := while.Body.Exprs[0].(*ast.BreakExpr); !ok {
		t.Fatalf("body[0]: got %T, want *ast.BreakExpr", while.Body.Exprs[0])
	}
	if _, ok := while.Body.Exprs[1].(*ast.ContinueExpr); !ok {
		t.Fatalf("body[1]: got %T, want *ast.ContinueExpr", while.Body.Exprs[1])
	}
}

func TestParse_SwitchDesugarsToIfChain(t *testing.T) {
	expr := parseExprSrc(t, `switch {
        case a: 1
        case b: 2
        default: 3
    }`)
	outer, ok := expr.(*ast.IfExpr)
	if !ok {
		t.Fatalf("got %T, want *ast.IfExpr (switch desugars to if/elif/else)", expr)
	}
	if _, ok := outer.Cond.(*ast.IdentExpr); !ok {
		t.Fatalf("outer cond: got %T, want *ast.IdentExpr", outer.Cond)
	}
	middle, ok := outer.Else.(*ast.IfExpr)
	if !ok {
		t.Fatalf("outer Else: got %T, want *ast.IfExpr", outer.Else)
	}
	final, ok := middle.Else.(*ast.Block)
	if !ok {
		t.Fatalf("middle Else: got %T, want *ast.Block (the default clause)", middle.Else)
	}
	if len(final.Exprs) != 1 {
		t.Fatalf("got %d default exprs, want 1", len(final.Exprs))
	}
}

func TestParse_SwitchWithoutDefaultHasNoElse(t *testing.T) {
	expr := parseExprSrc(t, `switch {
        case a: 1
        case b: 2
    }`)
	outer := expr.(*ast.IfExpr)
	inner, ok := outer.Else.(*ast.IfExpr)
	if !ok {
		t.Fatalf("Else: got %T, want *ast.IfExpr", outer.Else)
	}
	if inner.Else != nil {
		t.Fatalf("innermost Else: got %#v, want nil (no default)", inner.Else)
	}
}

func TestParse_SwitchWithNoCasesIsAnError(t *testing.T) {
	if _, err := Parse("fn main() -> Int {\n    _ = switch {}\n    0\n}\n"); err == nil {
		t.Fatal("expected an error for a switch with no cases")
	}
}

func TestParse_SwitchDefaultNotLastIsAnError(t *testing.T) {
	src := "fn main() -> Int {\n    _ = switch {\n        default: 1\n        case a: 2\n    }\n    0\n}\n"
	if _, err := Parse(src); err == nil {
		t.Fatal("expected an error for 'default' not being the last clause")
	}
}

func TestParse_SwitchDuplicateDefaultIsAnError(t *testing.T) {
	src := "fn main() -> Int {\n    _ = switch {\n        case a: 1\n        default: 2\n        default: 3\n    }\n    0\n}\n"
	if _, err := Parse(src); err == nil {
		t.Fatal("expected an error for a duplicate 'default'")
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

func TestParse_FuncDeclWithParams(t *testing.T) {
	src := "fn add(a: Int, b: Int) -> Int {\n    a + b\n}\nfn main() -> Int {\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	var add *ast.FuncDecl
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name == "add" {
			add = fn
		}
	}
	if add == nil {
		t.Fatal("fn add not found")
	}
	if len(add.Params) != 2 {
		t.Fatalf("got %d params, want 2", len(add.Params))
	}
	if add.Params[0].Name != "a" || namedTypeName(add.Params[0].Type) != "Int" {
		t.Errorf("param 0: got %+v", add.Params[0])
	}
	if add.Params[1].Name != "b" || namedTypeName(add.Params[1].Type) != "Int" {
		t.Errorf("param 1: got %+v", add.Params[1])
	}
	if namedTypeName(add.ReturnType) != "Int" {
		t.Errorf("got ReturnType %v, want Int", add.ReturnType)
	}
}

func TestParse_FuncDeclWithNoParams(t *testing.T) {
	f, err := Parse("fn main() -> Int {\n    0\n}\n")
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	fn := parseFuncMain(t, f)
	if len(fn.Params) != 0 {
		t.Fatalf("got %d params, want 0", len(fn.Params))
	}
}

func TestParse_CallWithArgs(t *testing.T) {
	call := parseExprSrc(t, "add(1, 2)").(*ast.CallExpr)
	if call.Callee != "add" || len(call.Args) != 2 {
		t.Fatalf("got CallExpr{Callee: %q, len(Args): %d}", call.Callee, len(call.Args))
	}
}

func TestParse_ClosureLitAsLetValue(t *testing.T) {
	src := "fn main() -> Int {\n    let square = fn(x: Int) -> Int { x * x }\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	let := parseFuncMain(t, f).Body.Exprs[0].(*ast.LetExpr)
	if let.Name != "square" {
		t.Fatalf("got let name %q, want square", let.Name)
	}
	clos, ok := let.Value.(*ast.ClosureLit)
	if !ok {
		t.Fatalf("got %T, want *ast.ClosureLit", let.Value)
	}
	if len(clos.Params) != 1 || clos.Params[0].Name != "x" || namedTypeName(clos.Params[0].Type) != "Int" {
		t.Fatalf("got params %+v", clos.Params)
	}
	if namedTypeName(clos.ReturnType) != "Int" {
		t.Fatalf("got ReturnType %v, want Int", clos.ReturnType)
	}
	if len(clos.Body.Exprs) != 1 {
		t.Fatalf("got %d body exprs, want 1", len(clos.Body.Exprs))
	}
}

func TestParse_ClosureLitWithNoParamsAndUnitReturn(t *testing.T) {
	src := "fn main() -> Int {\n    let noop = fn() -> Unit { print(\"hi\") }\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	let := parseFuncMain(t, f).Body.Exprs[0].(*ast.LetExpr)
	clos := let.Value.(*ast.ClosureLit)
	if len(clos.Params) != 0 {
		t.Fatalf("got %d params, want 0", len(clos.Params))
	}
	if namedTypeName(clos.ReturnType) != "Unit" {
		t.Fatalf("got ReturnType %v, want Unit", clos.ReturnType)
	}
}

func TestParse_ParenIsGroupingNotTuple(t *testing.T) {
	expr := parseExprSrc(t, "(1 + 2)")
	if _, ok := expr.(*ast.BinaryExpr); !ok {
		t.Fatalf("got %T, want the inner *ast.BinaryExpr (grouping unwrapped)", expr)
	}
}

func TestParse_TupleLit(t *testing.T) {
	tup := parseExprSrc(t, "(1, 2, 3)").(*ast.TupleLit)
	if len(tup.Elems) != 3 {
		t.Fatalf("got %d elems, want 3", len(tup.Elems))
	}
	for i, want := range []uint64{1, 2, 3} {
		lit, ok := tup.Elems[i].(*ast.IntLit)
		if !ok || lit.Value != want {
			t.Errorf("elem %d: got %#v, want IntLit{Value: %d}", i, tup.Elems[i], want)
		}
	}
}

func TestParse_TupleLitTrailingComma(t *testing.T) {
	tup := parseExprSrc(t, "(1, 2,)").(*ast.TupleLit)
	if len(tup.Elems) != 2 {
		t.Fatalf("got %d elems, want 2", len(tup.Elems))
	}
}

func TestParse_OneElementTrailingCommaIsTupleLit(t *testing.T) {
	// (x,) is syntactically a TupleLit (sema rejects its arity) — distinct
	// from (x), which parseParenOrTupleExpr resolves as plain grouping.
	tup, ok := parseExprSrc(t, "(1,)").(*ast.TupleLit)
	if !ok {
		t.Fatalf("got %T, want *ast.TupleLit", parseExprSrc(t, "(1,)"))
	}
	if len(tup.Elems) != 1 {
		t.Fatalf("got %d elems, want 1", len(tup.Elems))
	}
}

func TestParse_TupleFieldAccess(t *testing.T) {
	f := parseExprSrc(t, "t.0").(*ast.FieldExpr)
	if f.Field != "0" {
		t.Fatalf("got Field %q, want \"0\"", f.Field)
	}
	target, ok := f.Target.(*ast.IdentExpr)
	if !ok || target.Name != "t" {
		t.Fatalf("got Target %#v, want IdentExpr{Name: \"t\"}", f.Target)
	}
}

func TestParse_ChainedFieldAccess(t *testing.T) {
	// l.from.x: the outer FieldExpr's Target is itself a FieldExpr.
	outer := parseExprSrc(t, "l.from.x").(*ast.FieldExpr)
	if outer.Field != "x" {
		t.Fatalf("got outer Field %q, want \"x\"", outer.Field)
	}
	inner, ok := outer.Target.(*ast.FieldExpr)
	if !ok || inner.Field != "from" {
		t.Fatalf("got inner %#v, want FieldExpr{Field: \"from\"}", outer.Target)
	}
}

func TestParse_StructLit(t *testing.T) {
	lit := parseExprSrc(t, "Point{x: 1, y: 2}").(*ast.StructLit)
	if lit.TypeName != "Point" {
		t.Fatalf("got TypeName %q, want Point", lit.TypeName)
	}
	if len(lit.Fields) != 2 || lit.Fields[0].Name != "x" || lit.Fields[1].Name != "y" {
		t.Fatalf("got Fields %+v", lit.Fields)
	}
}

func TestParse_StructDecl(t *testing.T) {
	src := "struct Point {\n    x: Float,\n    y: Float\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if len(f.Decls) != 1 {
		t.Fatalf("got %d decls, want 1", len(f.Decls))
	}
	st, ok := f.Decls[0].(*ast.StructDecl)
	if !ok {
		t.Fatalf("got %T, want *ast.StructDecl", f.Decls[0])
	}
	if st.Name != "Point" || len(st.Fields) != 2 {
		t.Fatalf("got StructDecl{Name: %q, len(Fields): %d}", st.Name, len(st.Fields))
	}
	if st.Fields[0].Name != "x" || namedTypeName(st.Fields[0].Type) != "Float" {
		t.Errorf("field 0: got %+v", st.Fields[0])
	}
	if st.Fields[1].Name != "y" || namedTypeName(st.Fields[1].Type) != "Float" {
		t.Errorf("field 1: got %+v", st.Fields[1])
	}
}

func TestParse_IfWithBareBoolIdentCondNotSwallowedAsStructLit(t *testing.T) {
	// `if flag { ... }` must still parse `flag` as a plain condition, not
	// attempt to read `{ ... }` as a struct literal's field list (the
	// noCompositeLit ambiguity documented on the parser struct).
	src := "fn main() -> Int {\n    if flag {\n        0\n    }\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	ifExpr := parseFuncMain(t, f).Body.Exprs[0].(*ast.IfExpr)
	cond, ok := ifExpr.Cond.(*ast.IdentExpr)
	if !ok || cond.Name != "flag" {
		t.Fatalf("got Cond %#v, want IdentExpr{Name: \"flag\"}", ifExpr.Cond)
	}
}

func TestParse_StructLitAllowedInParenthesizedCond(t *testing.T) {
	src := "fn main() -> Int {\n    if (Point{x: 1, y: 2} == p) {\n        0\n    }\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	ifExpr := parseFuncMain(t, f).Body.Exprs[0].(*ast.IfExpr)
	bin, ok := ifExpr.Cond.(*ast.BinaryExpr)
	if !ok || bin.Op != "==" {
		t.Fatalf("got Cond %#v, want BinaryExpr{Op: \"==\"}", ifExpr.Cond)
	}
	if _, ok := bin.Left.(*ast.StructLit); !ok {
		t.Fatalf("got Cond.Left %T, want *ast.StructLit", bin.Left)
	}
}

func TestParse_ListTypeAnnotation(t *testing.T) {
	src := "fn main() -> Int {\n    let xs: List[Int] = [1, 2, 3]\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	let := parseFuncMain(t, f).Body.Exprs[0].(*ast.LetExpr)
	lt, ok := let.Type.(*ast.ListType)
	if !ok {
		t.Fatalf("got Type %T, want *ast.ListType", let.Type)
	}
	if namedTypeName(lt.Elem) != "Int" {
		t.Fatalf("got Elem %v, want Int", lt.Elem)
	}
}

func TestParse_ArrayTypeAnnotationSingleDimension(t *testing.T) {
	src := "fn main() -> Int {\n    let xs: Array[Int;5] = [1, 2, 3, 4, 5]\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	let := parseFuncMain(t, f).Body.Exprs[0].(*ast.LetExpr)
	at, ok := let.Type.(*ast.ArrayType)
	if !ok {
		t.Fatalf("got Type %T, want *ast.ArrayType", let.Type)
	}
	if namedTypeName(at.Elem) != "Int" {
		t.Fatalf("got Elem %v, want Int", at.Elem)
	}
	size, ok := at.Size.(*ast.IntLit)
	if !ok || size.Value != 5 {
		t.Fatalf("got Size %#v, want IntLit{Value: 5}", at.Size)
	}
}

func TestParse_ArrayTypeMultiDimensionDesugarsToNestedArrayType(t *testing.T) {
	// Array[Int;2,3] ≡ Array[Array[Int;3];2] — outer Size is the first
	// (2), Elem is the inner ArrayType carrying the second (3).
	src := "fn main() -> Int {\n    let m: Array[Int;2,3] = [[1,2,3],[4,5,6]]\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	let := parseFuncMain(t, f).Body.Exprs[0].(*ast.LetExpr)
	outer, ok := let.Type.(*ast.ArrayType)
	if !ok {
		t.Fatalf("got Type %T, want *ast.ArrayType", let.Type)
	}
	outerSize, ok := outer.Size.(*ast.IntLit)
	if !ok || outerSize.Value != 2 {
		t.Fatalf("got outer Size %#v, want IntLit{Value: 2}", outer.Size)
	}
	inner, ok := outer.Elem.(*ast.ArrayType)
	if !ok {
		t.Fatalf("got outer Elem %T, want *ast.ArrayType", outer.Elem)
	}
	innerSize, ok := inner.Size.(*ast.IntLit)
	if !ok || innerSize.Value != 3 {
		t.Fatalf("got inner Size %#v, want IntLit{Value: 3}", inner.Size)
	}
	if namedTypeName(inner.Elem) != "Int" {
		t.Fatalf("got inner Elem %v, want Int", inner.Elem)
	}
}

func TestParse_ListLit(t *testing.T) {
	lit := parseExprSrc(t, "[1, 2, 3]").(*ast.ListLit)
	if len(lit.Elems) != 3 {
		t.Fatalf("got %d elems, want 3", len(lit.Elems))
	}
}

func TestParse_EmptyListLit(t *testing.T) {
	lit := parseExprSrc(t, "[]").(*ast.ListLit)
	if len(lit.Elems) != 0 {
		t.Fatalf("got %d elems, want 0", len(lit.Elems))
	}
}

func TestParse_IndexExpr(t *testing.T) {
	idx := parseExprSrc(t, "xs[0]").(*ast.IndexExpr)
	target, ok := idx.Target.(*ast.IdentExpr)
	if !ok || target.Name != "xs" {
		t.Fatalf("got Target %#v, want IdentExpr{Name: \"xs\"}", idx.Target)
	}
	index, ok := idx.Index.(*ast.IntLit)
	if !ok || index.Value != 0 {
		t.Fatalf("got Index %#v, want IntLit{Value: 0}", idx.Index)
	}
}

func TestParse_ChainedIndexExpr(t *testing.T) {
	// matrix[i][j]: outer IndexExpr's Target is itself an IndexExpr.
	outer := parseExprSrc(t, "matrix[i][j]").(*ast.IndexExpr)
	inner, ok := outer.Target.(*ast.IndexExpr)
	if !ok {
		t.Fatalf("got outer Target %T, want *ast.IndexExpr", outer.Target)
	}
	innerTarget, ok := inner.Target.(*ast.IdentExpr)
	if !ok || innerTarget.Name != "matrix" {
		t.Fatalf("got inner Target %#v, want IdentExpr{Name: \"matrix\"}", inner.Target)
	}
}

func TestParse_IndexAssignExpr(t *testing.T) {
	assign := parseStmtExpr(t, "xs[0] = 5").(*ast.IndexAssignExpr)
	target, ok := assign.Target.(*ast.IdentExpr)
	if !ok || target.Name != "xs" {
		t.Fatalf("got Target %#v, want IdentExpr{Name: \"xs\"}", assign.Target)
	}
	val, ok := assign.Value.(*ast.IntLit)
	if !ok || val.Value != 5 {
		t.Fatalf("got Value %#v, want IntLit{Value: 5}", assign.Value)
	}
}

func TestParse_InvalidAssignmentTargetIsAnError(t *testing.T) {
	if _, err := Parse("fn main() -> Int {\n    f() = 1\n    0\n}\n"); err == nil {
		t.Fatal("expected an error assigning to a call result")
	}
}

func TestParse_SliceExprBothBounds(t *testing.T) {
	sl := parseExprSrc(t, "xs[1:3]").(*ast.SliceExpr)
	from, ok := sl.From.(*ast.IntLit)
	if !ok || from.Value != 1 {
		t.Fatalf("got From %#v, want IntLit{Value: 1}", sl.From)
	}
	to, ok := sl.To.(*ast.IntLit)
	if !ok || to.Value != 3 {
		t.Fatalf("got To %#v, want IntLit{Value: 3}", sl.To)
	}
}

func TestParse_SliceExprOmittedBounds(t *testing.T) {
	cases := []struct {
		src      string
		wantFrom bool
		wantTo   bool
	}{
		{"xs[1:]", true, false},
		{"xs[:3]", false, true},
		{"xs[:]", false, false},
	}
	for _, c := range cases {
		sl := parseExprSrc(t, c.src).(*ast.SliceExpr)
		if (sl.From != nil) != c.wantFrom {
			t.Errorf("%s: got From %v, want present=%v", c.src, sl.From, c.wantFrom)
		}
		if (sl.To != nil) != c.wantTo {
			t.Errorf("%s: got To %v, want present=%v", c.src, sl.To, c.wantTo)
		}
	}
}

func TestParse_ForExpr(t *testing.T) {
	src := "fn main() -> Int {\n    for x in xs {\n        print(\"hi\")\n    }\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	forExpr := parseFuncMain(t, f).Body.Exprs[0].(*ast.ForExpr)
	if forExpr.Var != "x" {
		t.Fatalf("got Var %q, want \"x\"", forExpr.Var)
	}
	items, ok := forExpr.Items.(*ast.IdentExpr)
	if !ok || items.Name != "xs" {
		t.Fatalf("got Items %#v, want IdentExpr{Name: \"xs\"}", forExpr.Items)
	}
	if len(forExpr.Body.Exprs) != 1 {
		t.Fatalf("got %d body exprs, want 1", len(forExpr.Body.Exprs))
	}
}

func TestParse_ForItemsBareIdentNotSwallowedAsStructLit(t *testing.T) {
	// `for x in items { ... }` must parse `items` as a plain condition,
	// not attempt to read `{ ... }` as a struct literal — the same
	// noCompositeLit ambiguity if/while conditions already guard against.
	src := "fn main() -> Int {\n    for x in items {\n        0\n    }\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	forExpr := parseFuncMain(t, f).Body.Exprs[0].(*ast.ForExpr)
	items, ok := forExpr.Items.(*ast.IdentExpr)
	if !ok || items.Name != "items" {
		t.Fatalf("got Items %#v, want IdentExpr{Name: \"items\"}", forExpr.Items)
	}
}

func findEnumDecl(t *testing.T, f *ast.File, name string) *ast.EnumDecl {
	t.Helper()
	for _, decl := range f.Decls {
		if en, ok := decl.(*ast.EnumDecl); ok && en.Name == name {
			return en
		}
	}
	t.Fatalf("no enum %q found", name)
	return nil
}

func TestParse_EnumDecl(t *testing.T) {
	src := "enum Status {\n" +
		"    Ok\n" +
		"    Retry(delay: Int)\n" +
		"    Failed(reason: String)\n" +
		"}\n" +
		"fn main() -> Int {\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	en := findEnumDecl(t, f, "Status")
	if len(en.Variants) != 3 {
		t.Fatalf("got %d variants, want 3", len(en.Variants))
	}
	if en.Variants[0].Name != "Ok" || len(en.Variants[0].Fields) != 0 {
		t.Fatalf("got variant[0] %#v, want Ok with no fields", en.Variants[0])
	}
	if en.Variants[1].Name != "Retry" || len(en.Variants[1].Fields) != 1 ||
		en.Variants[1].Fields[0].Name != "delay" || namedTypeName(en.Variants[1].Fields[0].Type) != "Int" {
		t.Fatalf("got variant[1] %#v, want Retry(delay: Int)", en.Variants[1])
	}
	if en.Variants[2].Name != "Failed" || len(en.Variants[2].Fields) != 1 ||
		en.Variants[2].Fields[0].Name != "reason" || namedTypeName(en.Variants[2].Fields[0].Type) != "String" {
		t.Fatalf("got variant[2] %#v, want Failed(reason: String)", en.Variants[2])
	}
}

func TestParse_EnumDeclWithNoVariantsIsAnError(t *testing.T) {
	src := "enum Empty {\n}\nfn main() -> Int {\n    0\n}\n"
	if _, err := Parse(src); err == nil {
		t.Fatal("expected an error for an enum with no variants")
	}
}

func TestParse_EnumVariantZeroFieldConstructionHasNilArgs(t *testing.T) {
	src := "fn main() -> Int {\n    let s = Status.Ok\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	let := parseFuncMain(t, f).Body.Exprs[0].(*ast.LetExpr)
	fe, ok := let.Value.(*ast.FieldExpr)
	if !ok || fe.Field != "Ok" {
		t.Fatalf("got %#v, want FieldExpr{Field: \"Ok\"}", let.Value)
	}
	if fe.Args != nil {
		t.Fatalf("got Args %#v, want nil (no parens written)", fe.Args)
	}
}

func TestParse_EnumVariantEmptyParensConstructionHasNonNilArgs(t *testing.T) {
	src := "fn main() -> Int {\n    let s = Status.Ok()\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	let := parseFuncMain(t, f).Body.Exprs[0].(*ast.LetExpr)
	fe := let.Value.(*ast.FieldExpr)
	if fe.Args == nil || len(fe.Args) != 0 {
		t.Fatalf("got Args %#v, want a non-nil, zero-length slice", fe.Args)
	}
}

func TestParse_EnumVariantConstructionWithNamedFields(t *testing.T) {
	src := "fn main() -> Int {\n    let s = Status.Retry(delay: 5)\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	let := parseFuncMain(t, f).Body.Exprs[0].(*ast.LetExpr)
	fe := let.Value.(*ast.FieldExpr)
	if fe.Field != "Retry" || len(fe.Args) != 1 || fe.Args[0].Name != "delay" {
		t.Fatalf("got %#v, want FieldExpr{Field: \"Retry\", Args: [delay: ...]}", fe)
	}
	lit, ok := fe.Args[0].Value.(*ast.IntLit)
	if !ok || lit.Value != 5 {
		t.Fatalf("got arg value %#v, want IntLit{Value: 5}", fe.Args[0].Value)
	}
}

func TestParse_SwitchWithSubjectParsesSwitchExpr(t *testing.T) {
	src := "fn main() -> Int {\n" +
		"    switch s {\n" +
		"        case Status.Ok: 1\n" +
		"        case Status.Retry(delay): delay\n" +
		"        default: 0\n" +
		"    }\n" +
		"}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	sw, ok := parseFuncMain(t, f).Body.Exprs[0].(*ast.SwitchExpr)
	if !ok {
		t.Fatalf("got %#v, want *ast.SwitchExpr", parseFuncMain(t, f).Body.Exprs[0])
	}
	subj, ok := sw.Subject.(*ast.IdentExpr)
	if !ok || subj.Name != "s" {
		t.Fatalf("got Subject %#v, want IdentExpr{Name: \"s\"}", sw.Subject)
	}
	if len(sw.Cases) != 2 {
		t.Fatalf("got %d cases, want 2", len(sw.Cases))
	}
	if sw.Cases[0].EnumName != "Status" || sw.Cases[0].Variant != "Ok" || sw.Cases[0].Bindings != nil {
		t.Fatalf("got case[0] %#v, want Status.Ok with no bindings", sw.Cases[0])
	}
	if sw.Cases[1].EnumName != "Status" || sw.Cases[1].Variant != "Retry" ||
		len(sw.Cases[1].Bindings) != 1 || sw.Cases[1].Bindings[0] != "delay" {
		t.Fatalf("got case[1] %#v, want Status.Retry(delay)", sw.Cases[1])
	}
	if sw.Default == nil || len(sw.Default.Exprs) != 1 {
		t.Fatalf("got Default %#v, want a one-expression block", sw.Default)
	}
}

func TestParse_PipeBareNameDesugarsToSingleArgCall(t *testing.T) {
	src := "fn main() -> Int {\n    let a = x |> f\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	let := parseFuncMain(t, f).Body.Exprs[0].(*ast.LetExpr)
	call, ok := let.Value.(*ast.CallExpr)
	if !ok || call.Callee != "f" || len(call.Args) != 1 {
		t.Fatalf("got %#v, want CallExpr{Callee: \"f\", 1 arg}", let.Value)
	}
	arg, ok := call.Args[0].(*ast.IdentExpr)
	if !ok || arg.Name != "x" {
		t.Fatalf("got arg %#v, want IdentExpr{Name: \"x\"}", call.Args[0])
	}
}

func TestParse_PipeNoUnderscorePrependsAsFirstArg(t *testing.T) {
	src := "fn main() -> Int {\n    let a = x |> f(1, 2)\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	let := parseFuncMain(t, f).Body.Exprs[0].(*ast.LetExpr)
	call := let.Value.(*ast.CallExpr)
	if call.Callee != "f" || len(call.Args) != 3 {
		t.Fatalf("got %#v, want CallExpr{Callee: \"f\", 3 args}", call)
	}
	first, ok := call.Args[0].(*ast.IdentExpr)
	if !ok || first.Name != "x" {
		t.Fatalf("got Args[0] %#v, want IdentExpr{Name: \"x\"} (lhs prepended)", call.Args[0])
	}
}

func TestParse_PipeUnderscoreInjectsAtItsPosition(t *testing.T) {
	src := "fn main() -> Int {\n    let a = x |> f(1, _, 2)\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	let := parseFuncMain(t, f).Body.Exprs[0].(*ast.LetExpr)
	call := let.Value.(*ast.CallExpr)
	if call.Callee != "f" || len(call.Args) != 3 {
		t.Fatalf("got %#v, want CallExpr{Callee: \"f\", 3 args}", call)
	}
	mid, ok := call.Args[1].(*ast.IdentExpr)
	if !ok || mid.Name != "x" {
		t.Fatalf("got Args[1] %#v, want IdentExpr{Name: \"x\"} (lhs injected at '_')", call.Args[1])
	}
	if lit, ok := call.Args[0].(*ast.IntLit); !ok || lit.Value != 1 {
		t.Fatalf("got Args[0] %#v, want IntLit{Value: 1}", call.Args[0])
	}
	if lit, ok := call.Args[2].(*ast.IntLit); !ok || lit.Value != 2 {
		t.Fatalf("got Args[2] %#v, want IntLit{Value: 2}", call.Args[2])
	}
}

func TestParse_PipeChainIsLeftAssociative(t *testing.T) {
	src := "fn main() -> Int {\n    let a = x |> f |> g\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	let := parseFuncMain(t, f).Body.Exprs[0].(*ast.LetExpr)
	outer, ok := let.Value.(*ast.CallExpr)
	if !ok || outer.Callee != "g" || len(outer.Args) != 1 {
		t.Fatalf("got %#v, want outer CallExpr{Callee: \"g\"}", let.Value)
	}
	inner, ok := outer.Args[0].(*ast.CallExpr)
	if !ok || inner.Callee != "f" || len(inner.Args) != 1 {
		t.Fatalf("got inner %#v, want CallExpr{Callee: \"f\"}", outer.Args[0])
	}
	ident, ok := inner.Args[0].(*ast.IdentExpr)
	if !ok || ident.Name != "x" {
		t.Fatalf("got innermost arg %#v, want IdentExpr{Name: \"x\"}", inner.Args[0])
	}
}

func TestParse_PipeInlineClosureRHS(t *testing.T) {
	src := "fn main() -> Int {\n    let a = x |> fn(v: Int) -> Int { v + 1 }\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	let := parseFuncMain(t, f).Body.Exprs[0].(*ast.LetExpr)
	call, ok := let.Value.(*ast.CallExpr)
	if !ok || call.InlineClosure == nil || call.Callee != "<closure>" {
		t.Fatalf("got %#v, want CallExpr{InlineClosure: non-nil, Callee: \"<closure>\"}", let.Value)
	}
	if len(call.Args) != 1 {
		t.Fatalf("got %d args, want exactly 1 (lhs)", len(call.Args))
	}
	arg, ok := call.Args[0].(*ast.IdentExpr)
	if !ok || arg.Name != "x" {
		t.Fatalf("got arg %#v, want IdentExpr{Name: \"x\"}", call.Args[0])
	}
	if len(call.InlineClosure.Params) != 1 || call.InlineClosure.Params[0].Name != "v" {
		t.Fatalf("got closure params %#v, want a single param named \"v\"", call.InlineClosure.Params)
	}
}

func TestParse_PipeInlineClosureChainsWithNamedStages(t *testing.T) {
	// `x |> f |> fn(v) -> R {...}` — an inline closure stage following an
	// ordinary named-callee stage, exercising parsePipeExpr's chain-collection
	// loop with a mix of both RHS shapes.
	src := "fn main() -> Int {\n    let a = x |> f |> fn(v: Int) -> Int { v }\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	let := parseFuncMain(t, f).Body.Exprs[0].(*ast.LetExpr)
	outer, ok := let.Value.(*ast.CallExpr)
	if !ok || outer.InlineClosure == nil {
		t.Fatalf("got outer %#v, want CallExpr{InlineClosure: non-nil}", let.Value)
	}
	inner, ok := outer.Args[0].(*ast.CallExpr)
	if !ok || inner.Callee != "f" || inner.InlineClosure != nil {
		t.Fatalf("got inner %#v, want ordinary CallExpr{Callee: \"f\"}", outer.Args[0])
	}
}

func TestParse_RangeExprHalfOpenAndInclusive(t *testing.T) {
	src := "fn main() -> Int {\n    let a = 0..10\n    let b = 0..=10\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	body := parseFuncMain(t, f).Body.Exprs
	a := body[0].(*ast.LetExpr).Value.(*ast.RangeExpr)
	if a.Inclusive {
		t.Errorf("got Inclusive=true for `0..10`, want false")
	}
	b := body[1].(*ast.LetExpr).Value.(*ast.RangeExpr)
	if !b.Inclusive {
		t.Errorf("got Inclusive=false for `0..=10`, want true")
	}
}

func TestParse_RangeExprBoundsParseFullBinaryExpr(t *testing.T) {
	// `..` sits below the full operator chain — `n - 1` must fully parse
	// as one bound, not leave `..` grabbing just `1`.
	src := "fn main() -> Int {\n    let a = 0..n-1\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	r := parseFuncMain(t, f).Body.Exprs[0].(*ast.LetExpr).Value.(*ast.RangeExpr)
	to, ok := r.To.(*ast.BinaryExpr)
	if !ok || to.Op != "-" {
		t.Fatalf("got To %#v, want BinaryExpr{Op: \"-\"}", r.To)
	}
}

func TestParse_RangeExprBindsTighterThanPipe(t *testing.T) {
	// `0..10 |> f` must parse as `(0..10) |> f`, never `0..(10 |> f)`.
	src := "fn main() -> Int {\n    let a = 0..10 |> f\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	call := parseFuncMain(t, f).Body.Exprs[0].(*ast.LetExpr).Value.(*ast.CallExpr)
	if call.Callee != "f" || len(call.Args) != 1 {
		t.Fatalf("got %#v, want CallExpr{Callee: \"f\", 1 arg}", call)
	}
	if _, ok := call.Args[0].(*ast.RangeExpr); !ok {
		t.Fatalf("got Args[0] %#v, want *ast.RangeExpr", call.Args[0])
	}
}

func TestParse_PipeDuplicateUnderscoreIsAnError(t *testing.T) {
	src := "fn main() -> Int {\n    let a = x |> f(_, _)\n    0\n}\n"
	if _, err := Parse(src); err == nil {
		t.Fatal("expected an error for a pipe call with two '_' placeholders")
	}
}

func TestParse_ForYieldParsesYieldField(t *testing.T) {
	src := "fn main() -> Int {\n    let ys = for x in xs yield x |> double\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	let := parseFuncMain(t, f).Body.Exprs[0].(*ast.LetExpr)
	forExpr, ok := let.Value.(*ast.ForExpr)
	if !ok {
		t.Fatalf("got %#v, want *ast.ForExpr", let.Value)
	}
	if forExpr.Body != nil {
		t.Fatalf("got non-nil Body %#v, want nil for the yield form", forExpr.Body)
	}
	if forExpr.Yield == nil {
		t.Fatal("got nil Yield, want the yielded expression")
	}
	call, ok := forExpr.Yield.(*ast.CallExpr)
	if !ok || call.Callee != "double" {
		t.Fatalf("got Yield %#v, want CallExpr{Callee: \"double\"} (from x |> double)", forExpr.Yield)
	}
}

func TestParse_ForWithoutYieldStillHasBodyNotYield(t *testing.T) {
	src := "fn main() -> Int {\n    for x in xs {\n        print(\"hi\")\n    }\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	forExpr := parseFuncMain(t, f).Body.Exprs[0].(*ast.ForExpr)
	if forExpr.Yield != nil {
		t.Fatalf("got non-nil Yield %#v, want nil for the block form", forExpr.Yield)
	}
	if forExpr.Body == nil {
		t.Fatal("got nil Body, want the block")
	}
}

func TestParse_SetLitParsesAsElems(t *testing.T) {
	src := "fn main() -> Int {\n    let s = {1, 2, 3}\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	let := parseFuncMain(t, f).Body.Exprs[0].(*ast.LetExpr)
	lit, ok := let.Value.(*ast.SetOrMapLit)
	if !ok {
		t.Fatalf("got %#v, want *ast.SetOrMapLit", let.Value)
	}
	if lit.Entries != nil {
		t.Fatalf("got non-nil Entries %#v, want nil for the Set form", lit.Entries)
	}
	if len(lit.Elems) != 3 {
		t.Fatalf("got %d elems, want 3", len(lit.Elems))
	}
}

func TestParse_MapLitParsesAsEntries(t *testing.T) {
	src := "fn main() -> Int {\n    let m = {\"a\": 1, \"b\": 2}\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	let := parseFuncMain(t, f).Body.Exprs[0].(*ast.LetExpr)
	lit, ok := let.Value.(*ast.SetOrMapLit)
	if !ok {
		t.Fatalf("got %#v, want *ast.SetOrMapLit", let.Value)
	}
	if lit.Elems != nil {
		t.Fatalf("got non-nil Elems %#v, want nil for the Map form", lit.Elems)
	}
	if len(lit.Entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(lit.Entries))
	}
	if key, ok := lit.Entries[0].Key.(*ast.StringLit); !ok || key.Value != "a" {
		t.Fatalf("got first entry key %#v, want StringLit{Value: \"a\"}", lit.Entries[0].Key)
	}
}

func TestParse_EmptyBraceLitHasNilElemsAndEntries(t *testing.T) {
	src := "fn main() -> Int {\n    let s: Set[Int] = {}\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	let := parseFuncMain(t, f).Body.Exprs[0].(*ast.LetExpr)
	lit, ok := let.Value.(*ast.SetOrMapLit)
	if !ok {
		t.Fatalf("got %#v, want *ast.SetOrMapLit", let.Value)
	}
	if lit.Elems != nil || lit.Entries != nil {
		t.Fatalf("got Elems=%#v Entries=%#v, want both nil for a bare {}", lit.Elems, lit.Entries)
	}
}

func TestParse_SetTypeAnnotation(t *testing.T) {
	src := "fn main() -> Int {\n    let s: Set[Int] = {1}\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	let := parseFuncMain(t, f).Body.Exprs[0].(*ast.LetExpr)
	st, ok := let.Type.(*ast.SetType)
	if !ok {
		t.Fatalf("got %#v, want *ast.SetType", let.Type)
	}
	if named, ok := st.Elem.(*ast.NamedType); !ok || named.Name != "Int" {
		t.Fatalf("got Elem %#v, want NamedType{Name: \"Int\"}", st.Elem)
	}
}

func TestParse_MapTypeAnnotation(t *testing.T) {
	src := "fn main() -> Int {\n    let m: Map[String, Int] = {\"a\": 1}\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	let := parseFuncMain(t, f).Body.Exprs[0].(*ast.LetExpr)
	mt, ok := let.Type.(*ast.MapType)
	if !ok {
		t.Fatalf("got %#v, want *ast.MapType", let.Type)
	}
	if named, ok := mt.Key.(*ast.NamedType); !ok || named.Name != "String" {
		t.Fatalf("got Key %#v, want NamedType{Name: \"String\"}", mt.Key)
	}
	if named, ok := mt.Value.(*ast.NamedType); !ok || named.Name != "Int" {
		t.Fatalf("got Value %#v, want NamedType{Name: \"Int\"}", mt.Value)
	}
}

func TestParse_ForTwoVarsParsesVar2(t *testing.T) {
	src := "fn main() -> Int {\n    for k, v in m {\n        print(\"hi\")\n    }\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	forExpr := parseFuncMain(t, f).Body.Exprs[0].(*ast.ForExpr)
	if forExpr.Var != "k" || forExpr.Var2 != "v" {
		t.Fatalf("got Var=%q Var2=%q, want Var=\"k\" Var2=\"v\"", forExpr.Var, forExpr.Var2)
	}
}

func TestParse_ForSingleVarLeavesVar2Empty(t *testing.T) {
	src := "fn main() -> Int {\n    for x in xs {\n        print(\"hi\")\n    }\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	forExpr := parseFuncMain(t, f).Body.Exprs[0].(*ast.ForExpr)
	if forExpr.Var2 != "" {
		t.Fatalf("got Var2 %q, want empty for the single-variable form", forExpr.Var2)
	}
}

func TestParse_ForTwoVarsWithYieldIsAnError(t *testing.T) {
	src := "fn main() -> Int {\n    let ys = for k, v in m yield k\n    0\n}\n"
	if _, err := Parse(src); err == nil {
		t.Fatal("expected an error for `for k, v in m yield ...`")
	}
}

func TestParse_SwitchWithoutSubjectStillDesugarsToIfExpr(t *testing.T) {
	// The subject-less Bool-only form (step 4) must keep desugaring into
	// an IfExpr, unaffected by step 8's new subject-carrying form.
	src := "fn main() -> Int {\n" +
		"    switch {\n" +
		"        case true: 1\n" +
		"        default: 0\n" +
		"    }\n" +
		"}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if _, ok := parseFuncMain(t, f).Body.Exprs[0].(*ast.IfExpr); !ok {
		t.Fatalf("got %#v, want *ast.IfExpr", parseFuncMain(t, f).Body.Exprs[0])
	}
}

func TestParse_PostfixTryOperator(t *testing.T) {
	src := "fn main() -> Int {\n    let x = parse[Int](s)?\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	let := parseFuncMain(t, f).Body.Exprs[0].(*ast.LetExpr)
	tryExpr, ok := let.Value.(*ast.TryExpr)
	if !ok {
		t.Fatalf("got %#v, want *ast.TryExpr", let.Value)
	}
	if _, ok := tryExpr.Value.(*ast.CallExpr); !ok {
		t.Fatalf("got %#v, want TryExpr.Value to be *ast.CallExpr", tryExpr.Value)
	}
}

func TestParse_GenericBuiltinCallParsesTypeArg(t *testing.T) {
	src := "fn main() -> Int {\n    let x = cast[Int8](200)\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	let := parseFuncMain(t, f).Body.Exprs[0].(*ast.LetExpr)
	call, ok := let.Value.(*ast.CallExpr)
	if !ok {
		t.Fatalf("got %#v, want *ast.CallExpr", let.Value)
	}
	if call.Callee != "cast" {
		t.Fatalf("got Callee %q, want \"cast\"", call.Callee)
	}
	named, ok := call.TypeArg.(*ast.NamedType)
	if !ok || named.Name != "Int8" {
		t.Fatalf("got TypeArg %#v, want *ast.NamedType{Name: \"Int8\"}", call.TypeArg)
	}
	if len(call.Args) != 1 {
		t.Fatalf("got %d args, want 1", len(call.Args))
	}
}

func TestParse_GenericBuiltinCallWithoutBracketLeavesTypeArgNil(t *testing.T) {
	src := "fn main() -> Int {\n    let x = len(xs)\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	let := parseFuncMain(t, f).Body.Exprs[0].(*ast.LetExpr)
	call := let.Value.(*ast.CallExpr)
	if call.TypeArg != nil {
		t.Fatalf("got TypeArg %#v, want nil for a non-generic call", call.TypeArg)
	}
}

func TestParse_PipeIntoGenericBuiltinNoParens(t *testing.T) {
	src := "fn main() -> Int {\n    let x = t |> unwrap[Int]\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	let := parseFuncMain(t, f).Body.Exprs[0].(*ast.LetExpr)
	call, ok := let.Value.(*ast.CallExpr)
	if !ok || call.Callee != "unwrap" {
		t.Fatalf("got %#v, want *ast.CallExpr{Callee: \"unwrap\"}", let.Value)
	}
	if len(call.Args) != 1 {
		t.Fatalf("got %d args, want 1 (lhs injected as the sole argument)", len(call.Args))
	}
	if named, ok := call.TypeArg.(*ast.NamedType); !ok || named.Name != "Int" {
		t.Fatalf("got TypeArg %#v, want *ast.NamedType{Name: \"Int\"}", call.TypeArg)
	}
}

func TestParse_PipeIntoGenericBuiltinWithParens(t *testing.T) {
	src := "fn main() -> Int {\n    let x = t |> okOr[Int](0)\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	let := parseFuncMain(t, f).Body.Exprs[0].(*ast.LetExpr)
	call := let.Value.(*ast.CallExpr)
	if call.Callee != "okOr" || len(call.Args) != 2 {
		t.Fatalf("got Callee=%q len(Args)=%d, want \"okOr\" with 2 args", call.Callee, len(call.Args))
	}
}

// --- step 15: pipe-chain metadata (amifl-spec.md section 9.1) ---

func TestParse_PipeChainStageMetadataIsFilledIn(t *testing.T) {
	src := "fn main() -> Int {\n    let a = data |> f |> g\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	let := parseFuncMain(t, f).Body.Exprs[0].(*ast.LetExpr)
	outer := let.Value.(*ast.CallExpr) // g, stage 2
	inner := outer.Args[0].(*ast.CallExpr)

	if inner.Callee != "f" || inner.PipeStage != 1 {
		t.Fatalf("got inner Callee=%q PipeStage=%d, want \"f\" stage 1", inner.Callee, inner.PipeStage)
	}
	if outer.Callee != "g" || outer.PipeStage != 2 {
		t.Fatalf("got outer Callee=%q PipeStage=%d, want \"g\" stage 2", outer.Callee, outer.PipeStage)
	}
	wantLabels := []string{"data", "f", "g"}
	for _, call := range []*ast.CallExpr{inner, outer} {
		if len(call.PipeChainLabels) != len(wantLabels) {
			t.Fatalf("got PipeChainLabels %v, want %v", call.PipeChainLabels, wantLabels)
		}
		for i, want := range wantLabels {
			if call.PipeChainLabels[i] != want {
				t.Fatalf("got PipeChainLabels %v, want %v", call.PipeChainLabels, wantLabels)
			}
		}
	}
}

func TestParse_PipeChainPipeArgIndexFollowsUnderscore(t *testing.T) {
	src := "fn main() -> Int {\n    let a = data |> f(1, _, 2)\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	let := parseFuncMain(t, f).Body.Exprs[0].(*ast.LetExpr)
	call := let.Value.(*ast.CallExpr)
	if call.PipeArgIndex != 1 {
		t.Fatalf("got PipeArgIndex %d, want 1 (the '_' position)", call.PipeArgIndex)
	}
}

func TestParse_NonPipeCallHasZeroPipeStage(t *testing.T) {
	src := "fn main() -> Int {\n    let a = f(1)\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	let := parseFuncMain(t, f).Body.Exprs[0].(*ast.LetExpr)
	call := let.Value.(*ast.CallExpr)
	if call.PipeStage != 0 {
		t.Fatalf("got PipeStage %d, want 0 for an ordinary (non-piped) call", call.PipeStage)
	}
}

func TestParse_Tuple2TypeAnnotation(t *testing.T) {
	src := "fn f() -> Tuple2[Int, Error] {\n    (1, 2)\n}\nfn main() -> Int {\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	var fn *ast.FuncDecl
	for _, d := range f.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok && fd.Name == "f" {
			fn = fd
		}
	}
	if fn == nil {
		t.Fatal("fn f not found")
	}
	tt, ok := fn.ReturnType.(*ast.TupleType)
	if !ok {
		t.Fatalf("got %#v, want *ast.TupleType", fn.ReturnType)
	}
	if len(tt.Elems) != 2 {
		t.Fatalf("got %d elems, want 2", len(tt.Elems))
	}
}

func TestParse_TupleTypeArityMismatchIsAnError(t *testing.T) {
	src := "fn f() -> Tuple2[Int, Error, Bool] {\n    (1, 2)\n}\nfn main() -> Int {\n    0\n}\n"
	if _, err := Parse(src); err == nil {
		t.Fatal("expected an error for Tuple2[...] given 3 type arguments")
	}
}

func TestParse_ChanTypeAnnotation(t *testing.T) {
	src := "fn main() -> Int {\n    let ch = chan[Int](0)\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	let := parseFuncMain(t, f).Body.Exprs[0].(*ast.LetExpr)
	call, ok := let.Value.(*ast.CallExpr)
	if !ok {
		t.Fatalf("got %#v, want *ast.CallExpr", let.Value)
	}
	if call.Callee != "chan" {
		t.Fatalf("got Callee %q, want \"chan\"", call.Callee)
	}
	named, ok := call.TypeArg.(*ast.NamedType)
	if !ok || named.Name != "Int" {
		t.Fatalf("got TypeArg %#v, want *ast.NamedType{Name: \"Int\"}", call.TypeArg)
	}
	if len(call.Args) != 1 {
		t.Fatalf("got %d args, want 1", len(call.Args))
	}
}

func TestParse_FuncTypeAnnotation(t *testing.T) {
	src := "fn main() -> Int {\n    let f: fn(Int, String) -> Bool = g\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	let := parseFuncMain(t, f).Body.Exprs[0].(*ast.LetExpr)
	ft, ok := let.Type.(*ast.FuncType)
	if !ok {
		t.Fatalf("got Type %T, want *ast.FuncType", let.Type)
	}
	if len(ft.Params) != 2 || namedTypeName(ft.Params[0]) != "Int" || namedTypeName(ft.Params[1]) != "String" {
		t.Fatalf("got Params %#v, want [Int, String]", ft.Params)
	}
	if namedTypeName(ft.Ret) != "Bool" {
		t.Fatalf("got Ret %v, want Bool", ft.Ret)
	}
}

func TestParse_FuncTypeAnnotationZeroParams(t *testing.T) {
	src := "fn main() -> Int {\n    let f: fn() -> Int = g\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	let := parseFuncMain(t, f).Body.Exprs[0].(*ast.LetExpr)
	ft, ok := let.Type.(*ast.FuncType)
	if !ok {
		t.Fatalf("got Type %T, want *ast.FuncType", let.Type)
	}
	if len(ft.Params) != 0 {
		t.Fatalf("got %d params, want 0", len(ft.Params))
	}
}

func TestParse_FuncTypeAnnotationNested(t *testing.T) {
	// A Func-typed parameter within a Func type — the case that turned out
	// to need funcTypeParts' depth-aware ")->" fix (sema/types.go).
	src := "fn main() -> Int {\n    let f: fn(fn(Int) -> Int, Int) -> Int = g\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	let := parseFuncMain(t, f).Body.Exprs[0].(*ast.LetExpr)
	ft, ok := let.Type.(*ast.FuncType)
	if !ok {
		t.Fatalf("got Type %T, want *ast.FuncType", let.Type)
	}
	if len(ft.Params) != 2 {
		t.Fatalf("got %d params, want 2", len(ft.Params))
	}
	if _, ok := ft.Params[0].(*ast.FuncType); !ok {
		t.Fatalf("got Params[0] %T, want *ast.FuncType", ft.Params[0])
	}
}

func TestParse_FuncTypeAsParamType(t *testing.T) {
	src := "fn apply(f: fn(Int) -> Int, x: Int) -> Int {\n    0\n}\nfn main() -> Int {\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	apply := f.Decls[0].(*ast.FuncDecl)
	if _, ok := apply.Params[0].Type.(*ast.FuncType); !ok {
		t.Fatalf("got Params[0].Type %T, want *ast.FuncType", apply.Params[0].Type)
	}
}

func TestParse_StreamTypeAnnotationParsesAsNestedBracket(t *testing.T) {
	src := "fn main() -> Int {\n    let s: Stream[String] = lines(f)\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	let := parseFuncMain(t, f).Body.Exprs[0].(*ast.LetExpr)
	st, ok := let.Type.(*ast.StreamType)
	if !ok {
		t.Fatalf("got %#v, want *ast.StreamType", let.Type)
	}
	if named, ok := st.Elem.(*ast.NamedType); !ok || named.Name != "String" {
		t.Fatalf("got Elem %#v, want NamedType{Name: \"String\"}", st.Elem)
	}
}

// --- step 13: extern/bind ---

func parseExternDeclNamed(t *testing.T, f *ast.File, alias string) *ast.ExternDecl {
	t.Helper()
	for _, decl := range f.Decls {
		if ext, ok := decl.(*ast.ExternDecl); ok && ext.Alias == alias {
			return ext
		}
	}
	t.Fatalf("no extern block aliased %q found", alias)
	return nil
}

func TestParse_ExternPlainFunctionBind(t *testing.T) {
	src := "extern \"encoding/json\" as json {\n" +
		"    bind Marshal(v: Any) -> Tuple2[Bytes, Error]\n" +
		"}\n" +
		"fn main() -> Int {\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	ext := parseExternDeclNamed(t, f, "json")
	if ext.Path != "encoding/json" {
		t.Fatalf("got Path %q, want \"encoding/json\"", ext.Path)
	}
	if len(ext.Binds) != 1 {
		t.Fatalf("got %d binds, want 1", len(ext.Binds))
	}
	b := ext.Binds[0]
	if b.Name != "Marshal" || b.GoTarget != "" {
		t.Fatalf("got Name=%q GoTarget=%q, want Name=\"Marshal\" GoTarget=\"\"", b.Name, b.GoTarget)
	}
	if len(b.Params) != 1 || b.Params[0].Name != "v" || namedTypeName(b.Params[0].Type) != "Any" {
		t.Fatalf("got Params %#v, want one param v:Any", b.Params)
	}
	tt, ok := b.ReturnType.(*ast.TupleType)
	if !ok || len(tt.Elems) != 2 {
		t.Fatalf("got ReturnType %#v, want Tuple2[...]", b.ReturnType)
	}
}

func TestParse_ExternMethodStyleBindAsTypeDotMethod(t *testing.T) {
	src := "extern \"time\" as time {\n" +
		"    type Time\n" +
		"    bind TimeUnix(t: Time) -> Int as Time.Unix\n" +
		"}\n" +
		"fn main() -> Int {\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	ext := parseExternDeclNamed(t, f, "time")
	if len(ext.Types) != 1 || ext.Types[0].Name != "Time" {
		t.Fatalf("got Types %#v, want [Time]", ext.Types)
	}
	if len(ext.Binds) != 1 {
		t.Fatalf("got %d binds, want 1", len(ext.Binds))
	}
	b := ext.Binds[0]
	if b.Name != "TimeUnix" || b.GoTarget != "Time.Unix" {
		t.Fatalf("got Name=%q GoTarget=%q, want Name=\"TimeUnix\" GoTarget=\"Time.Unix\"", b.Name, b.GoTarget)
	}
}

func TestParse_ExternBindRenameWithoutDot(t *testing.T) {
	src := "extern \"encoding/json\" as json {\n" +
		"    bind Marshal2(v: Any) -> Tuple2[Bytes, Error] as Marshal\n" +
		"}\n" +
		"fn main() -> Int {\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	ext := parseExternDeclNamed(t, f, "json")
	if ext.Binds[0].GoTarget != "Marshal" {
		t.Fatalf("got GoTarget %q, want \"Marshal\"", ext.Binds[0].GoTarget)
	}
}

func TestParse_ExternRejectsUnknownEntry(t *testing.T) {
	src := "extern \"foo\" as foo {\n    let x = 1\n}\nfn main() -> Int {\n    0\n}\n"
	if _, err := Parse(src); err == nil {
		t.Fatal("expected an error for a non type/bind entry inside an extern block")
	}
}

func TestParse_ExternRequiresAsAlias(t *testing.T) {
	src := "extern \"foo\" {\n    bind F() -> Int\n}\nfn main() -> Int {\n    0\n}\n"
	if _, err := Parse(src); err == nil {
		t.Fatal("expected an error for an extern block missing 'as alias'")
	}
}

func TestParse_ChanElemTypeAnnotation(t *testing.T) {
	src := "fn main() -> Int {\n    let ch: Chan[Int] = chan[Int](0)\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	let := parseFuncMain(t, f).Body.Exprs[0].(*ast.LetExpr)
	ct, ok := let.Type.(*ast.ChanType)
	if !ok {
		t.Fatalf("got %#v, want *ast.ChanType", let.Type)
	}
	if named, ok := ct.Elem.(*ast.NamedType); !ok || named.Name != "Int" {
		t.Fatalf("got Elem %#v, want NamedType{Name: \"Int\"}", ct.Elem)
	}
}

// --- step 14: import / qualified references ---

func TestParse_ImportDecl(t *testing.T) {
	src := "import mathutil \"./mathutil\"\nfn main() -> Int {\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	var imp *ast.ImportDecl
	for _, decl := range f.Decls {
		if d, ok := decl.(*ast.ImportDecl); ok {
			imp = d
		}
	}
	if imp == nil {
		t.Fatal("no ImportDecl found")
	}
	if imp.Alias != "mathutil" || imp.Path != "./mathutil" {
		t.Fatalf("got Alias=%q Path=%q, want Alias=\"mathutil\" Path=\"./mathutil\"", imp.Alias, imp.Path)
	}
}

func TestParse_ImportRequiresStringPath(t *testing.T) {
	src := "import mathutil mathutil\nfn main() -> Int {\n    0\n}\n"
	if _, err := Parse(src); err == nil {
		t.Fatal("expected an error for an import path that isn't a string literal")
	}
}

// TestParse_QualifiedCallParsesPositionalArgs is the parser-level half of
// step 14's `alias.Name(args...)` (amifl-spec.md section 12.2): unlike
// enum variant construction's `EnumType.Variant(field: v, ...)`, every
// argument here is a plain positional value, with no leading `name:`
// label — parseFieldCallArgs's own doc comment explains how each argument
// tells the two shapes apart with no lookahead beyond parsing itself.
func TestParse_QualifiedCallParsesPositionalArgs(t *testing.T) {
	src := "fn main() -> Int {\n    mathutil.clamp(15, 0, 10)\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	fe, ok := parseFuncMain(t, f).Body.Exprs[0].(*ast.FieldExpr)
	if !ok {
		t.Fatalf("got %#v, want *ast.FieldExpr", parseFuncMain(t, f).Body.Exprs[0])
	}
	if fe.Field != "clamp" {
		t.Fatalf("got Field %q, want \"clamp\"", fe.Field)
	}
	if len(fe.Args) != 3 {
		t.Fatalf("got %d args, want 3", len(fe.Args))
	}
	for i, a := range fe.Args {
		if a.Name != "" {
			t.Fatalf("arg %d: got Name %q, want \"\" (positional)", i, a.Name)
		}
	}
}

func TestParse_QualifiedZeroArgCallParsesEmptyArgs(t *testing.T) {
	src := "fn main() -> Int {\n    util.Reset()\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	fe, ok := parseFuncMain(t, f).Body.Exprs[0].(*ast.FieldExpr)
	if !ok {
		t.Fatalf("got %#v, want *ast.FieldExpr", parseFuncMain(t, f).Body.Exprs[0])
	}
	if fe.Args == nil || len(fe.Args) != 0 {
		t.Fatalf("got Args %#v, want a non-nil, zero-length slice", fe.Args)
	}
}

// TestParse_EnumVariantNamedArgsStillParseAfterUnification is a regression
// check for parseFieldCallArgs's unification (this parser used to have a
// dedicated parseEnumVariantArgs that only ever accepted the named-field
// form) — enum variant construction's own named-argument syntax must keep
// working exactly as before.
func TestParse_EnumVariantNamedArgsStillParseAfterUnification(t *testing.T) {
	src := "enum Status {\n    Retry(delay: Int)\n}\nfn main() -> Int {\n    Status.Retry(delay: 5)\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	fe, ok := parseFuncMain(t, f).Body.Exprs[0].(*ast.FieldExpr)
	if !ok {
		t.Fatalf("got %#v, want *ast.FieldExpr", parseFuncMain(t, f).Body.Exprs[0])
	}
	if len(fe.Args) != 1 || fe.Args[0].Name != "delay" {
		t.Fatalf("got Args %#v, want one named arg \"delay\"", fe.Args)
	}
}

// ex5: cross-package struct/enum references (amifl-spec.md section 12.2).

func TestParse_QualifiedStructLitParsesQualifierAndFields(t *testing.T) {
	src := "fn main() -> Int {\n    let p = mathutil.Point{x: 1, y: 2}\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	let := parseFuncMain(t, f).Body.Exprs[0].(*ast.LetExpr)
	sl, ok := let.Value.(*ast.StructLit)
	if !ok || sl.Qualifier != "mathutil" || sl.TypeName != "Point" {
		t.Fatalf("got %#v, want StructLit{Qualifier: \"mathutil\", TypeName: \"Point\"}", let.Value)
	}
	if len(sl.Fields) != 2 {
		t.Fatalf("got %d fields, want 2", len(sl.Fields))
	}
}

// TestParse_QualifiedStructLitAllowsFurtherPostfixChaining confirms the
// qualified-literal branch inside parsePostfixExpr's dot-loop `continue`s
// back into the same loop (rather than returning early) — a field access
// right after the closing '}' must still parse.
func TestParse_QualifiedStructLitAllowsFurtherPostfixChaining(t *testing.T) {
	src := "fn main() -> Int {\n    mathutil.Point{x: 1, y: 2}.x\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	fe, ok := parseFuncMain(t, f).Body.Exprs[0].(*ast.FieldExpr)
	if !ok || fe.Field != "x" {
		t.Fatalf("got %#v, want FieldExpr{Field: \"x\"}", parseFuncMain(t, f).Body.Exprs[0])
	}
	if _, ok := fe.Target.(*ast.StructLit); !ok {
		t.Fatalf("got Target %#v, want *ast.StructLit", fe.Target)
	}
}

// TestParse_QualifiedStructLitSuppressedInHeaderPosition confirms
// noCompositeLit still wins over the new qualified-literal check — an
// if-header ending in `alias.field` must leave the following '{' for the
// if-body, exactly like the pre-existing unqualified case does.
func TestParse_QualifiedStructLitSuppressedInHeaderPosition(t *testing.T) {
	src := "fn main() -> Int {\n    if mathutil.enabled {\n        1\n    } else {\n        0\n    }\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	ifExpr, ok := parseFuncMain(t, f).Body.Exprs[0].(*ast.IfExpr)
	if !ok {
		t.Fatalf("got %#v, want *ast.IfExpr", parseFuncMain(t, f).Body.Exprs[0])
	}
	if _, ok := ifExpr.Cond.(*ast.FieldExpr); !ok {
		t.Fatalf("got Cond %#v, want a plain *ast.FieldExpr (not a qualified struct literal)", ifExpr.Cond)
	}
}

// TestParse_QualifiedEnumVariantConstructionChainsThroughFieldExpr confirms
// `alias.EnumType.Variant(...)` needs no dedicated grammar at all — the
// existing dot-chaining loop in parsePostfixExpr already produces the
// right nested FieldExpr shape (resolveFieldExpr, sema, is what interprets
// it as ex5's cross-package enum construction).
func TestParse_QualifiedEnumVariantConstructionChainsThroughFieldExpr(t *testing.T) {
	src := "fn main() -> Int {\n    mathutil.Status.Retry(delay: 5)\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	outer, ok := parseFuncMain(t, f).Body.Exprs[0].(*ast.FieldExpr)
	if !ok || outer.Field != "Retry" || len(outer.Args) != 1 {
		t.Fatalf("got %#v, want FieldExpr{Field: \"Retry\", 1 arg}", parseFuncMain(t, f).Body.Exprs[0])
	}
	inner, ok := outer.Target.(*ast.FieldExpr)
	if !ok || inner.Field != "Status" || inner.Args != nil {
		t.Fatalf("got Target %#v, want FieldExpr{Field: \"Status\", Args: nil}", outer.Target)
	}
	alias, ok := inner.Target.(*ast.IdentExpr)
	if !ok || alias.Name != "mathutil" {
		t.Fatalf("got innermost Target %#v, want IdentExpr{Name: \"mathutil\"}", inner.Target)
	}
}

func TestParse_QualifiedTypeAnnotation(t *testing.T) {
	src := "fn f(p: mathutil.Point) -> Int {\n    0\n}\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	fn := f.Decls[0].(*ast.FuncDecl)
	qt, ok := fn.Params[0].Type.(*ast.QualifiedType)
	if !ok || qt.Alias != "mathutil" || qt.Name != "Point" {
		t.Fatalf("got %#v, want QualifiedType{Alias: \"mathutil\", Name: \"Point\"}", fn.Params[0].Type)
	}
}

// --- ex7: hex/octal/binary integer literals, digit-separator `_`
// (amifl-spec.md section 3.1) ---

// TestParse_HexOctalBinaryIntLiteralsResolveToTheirDecimalValue confirms
// the parser's strconv.ParseUint(text, 0, 64) call (base 0, so the
// lexer's own token text picks the base) computes the right magnitude
// regardless of which base the source used, and that IntLit.Token keeps
// the exact source text for codegen (ast.IntLit's own doc comment).
func TestParse_HexOctalBinaryIntLiteralsResolveToTheirDecimalValue(t *testing.T) {
	for _, tc := range []struct {
		src       string
		wantValue uint64
	}{
		{"0x1A", 26},
		{"0o17", 15},
		{"0b101", 5},
		{"1_000_000", 1000000},
		{"0x1_A", 26},
	} {
		f, err := Parse("fn main() -> Int {\n    " + tc.src + "\n}\n")
		if err != nil {
			t.Fatalf("Parse(%q) error: %v", tc.src, err)
		}
		lit, ok := parseFuncMain(t, f).Body.Exprs[0].(*ast.IntLit)
		if !ok {
			t.Fatalf("Parse(%q): body[0] is %T, want *ast.IntLit", tc.src, parseFuncMain(t, f).Body.Exprs[0])
		}
		if lit.Value != tc.wantValue {
			t.Errorf("Parse(%q): got Value %d, want %d", tc.src, lit.Value, tc.wantValue)
		}
		if lit.Token != tc.src {
			t.Errorf("Parse(%q): got Token %q, want %q", tc.src, lit.Token, tc.src)
		}
	}
}

// TestParse_MalformedDigitForBaseIsAnError confirms a digit invalid for
// its own base (lexed as one token, per lexer_test.go's
// TestNext_MalformedPrefixedLiteralLexesAsOneTokenNotSplit) is caught here
// by strconv.ParseUint's own error, wrapped into the parser's usual
// "invalid integer literal" message.
func TestParse_MalformedDigitForBaseIsAnError(t *testing.T) {
	_, err := Parse("fn main() -> Int {\n    0o18\n}\n")
	if err == nil {
		t.Fatal("expected an error: 8 isn't a valid octal digit")
	}
}
