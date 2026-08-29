package sema

import (
	"strings"
	"testing"

	"github.com/amisonnet8/amifl/internal/ast"
)

// nt builds a plain named-type annotation (ast.NamedType) — step 7
// introduced ast.TypeExpr in place of a bare string for every Type/
// ReturnType field; every test in this file that only ever needs a
// scalar/struct name (never List[...]/Array[...;...]) uses this rather
// than constructing *ast.NamedType by hand everywhere.
func nt(name string) ast.TypeExpr {
	return &ast.NamedType{Name: name}
}

func mainFile(exprs ...ast.Expr) *ast.File {
	return &ast.File{
		Decls: []ast.TopLevelDecl{
			&ast.FuncDecl{
				Name:       "main",
				ReturnType: nt("Int"),
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

func TestCheck_MultipleTopLevelFuncsAreAllowed(t *testing.T) {
	// Step 5 lifts the earlier "only fn main" restriction: any number of
	// top-level functions may coexist, called from main by name.
	f := mainFile(
		&ast.DiscardExpr{Value: &ast.CallExpr{Callee: "helper"}},
		&ast.IntLit{Value: 0},
	)
	f.Decls = append(f.Decls, &ast.FuncDecl{
		Name:       "helper",
		ReturnType: nt("Int"),
		Body:       &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 0}}},
	})
	if err := Check(f); err != nil {
		t.Fatalf("unexpected error for multiple top-level funcs: %v", err)
	}
}

func TestCheck_WrongReturnTypeIsAnError(t *testing.T) {
	f := mainFile(&ast.IntLit{Value: 0})
	f.Decls[0].(*ast.FuncDecl).ReturnType = nt("String")
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a non-Int return type")
	}
}

func TestCheck_UnknownReturnTypeIsAnError(t *testing.T) {
	f := mainFile(&ast.IntLit{Value: 0})
	f.Decls[0].(*ast.FuncDecl).ReturnType = nt("Nope")
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

func TestCheck_UndefinedCallIsAnError(t *testing.T) {
	f := mainFile(&ast.CallExpr{Callee: "someUndefinedFunction", Args: []ast.Expr{&ast.StringLit{Value: "x"}}}, &ast.IntLit{Value: 0})
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a call to an undefined name")
	}
}

// TestCheck_PrintWithNonStringArgIsFine documents ex6's generalization of
// print's argument from String to Any (amifl-spec.md section 13.1) — a
// call this same test used to reject before ex6.
func TestCheck_PrintWithNonStringArgIsFine(t *testing.T) {
	f := mainFile(&ast.CallExpr{Callee: "print", Args: []ast.Expr{&ast.IntLit{Value: 1}}}, &ast.IntLit{Value: 0})
	if err := Check(f); err != nil {
		t.Fatalf("print(1) should type-check after ex6's Any generalization: %v", err)
	}
}

func TestCheck_PrintWithUnitArgIsAnError(t *testing.T) {
	f := mainFile(&ast.CallExpr{Callee: "print", Args: []ast.Expr{
		&ast.CallExpr{Callee: "print", Args: []ast.Expr{&ast.StringLit{Value: "x"}}},
	}}, &ast.IntLit{Value: 0})
	if err := Check(f); err == nil {
		t.Fatal("expected an error for print(Unit-typed value)")
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
		&ast.LetExpr{Name: "x", Type: nt("Int"), Value: &ast.IntLit{Value: 42}},
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
		&ast.LetExpr{Name: "x", Type: nt("Bool"), Value: &ast.IntLit{Value: 5}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for let x: Bool = 5")
	}
}

func TestCheck_FloatLiteralCannotNarrowToIntType(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "x", Type: nt("Int"), Value: &ast.FloatLit{Value: 1.5}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for let x: Int = 1.5")
	}
}

func TestCheck_IntLiteralFitsFloatType(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "x", Type: nt("Float"), Value: &ast.IntLit{Value: 5}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_IntLiteralOverflowIsAnError(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "x", Type: nt("Int8"), Value: &ast.IntLit{Value: 200}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for let x: Int8 = 200 (overflow)")
	}
}

func TestCheck_UnknownTypeAnnotationIsAnError(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "x", Type: nt("Nope"), Value: &ast.IntLit{Value: 1}},
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
		&ast.LetExpr{Name: "x", Type: nt("Int"), Value: &ast.IntLit{Value: 1}},
		&ast.AssignExpr{Name: "x", Value: &ast.StringLit{Value: "nope"}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for assigning a String to an Int variable")
	}
}

func TestCheck_ReassigningLetIsValid(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "x", Type: nt("Int"), Value: &ast.IntLit{Value: 1}},
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
			&ast.ConstDecl{Name: "Greeting", Type: nt("String"), Value: &ast.StringLit{Value: "hi"}},
			&ast.FuncDecl{
				Name:       "main",
				ReturnType: nt("Int"),
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

func TestCheck_ArithmeticBetweenSameTypesIsValid(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "x", Type: nt("Int8"), Value: &ast.IntLit{Value: 5}},
		&ast.LetExpr{Name: "y", Value: &ast.BinaryExpr{Op: "+", Left: &ast.IdentExpr{Name: "x"}, Right: &ast.IntLit{Value: 1}}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	let := f.Decls[0].(*ast.FuncDecl).Body.Exprs[1].(*ast.LetExpr)
	if let.ResolvedType != "Int8" {
		t.Fatalf("got ResolvedType %q, want Int8 (the literal 1 should adapt to x's type)", let.ResolvedType)
	}
}

func TestCheck_LiteralAdaptsRegardlessOfOperandOrder(t *testing.T) {
	// 1 + x must type-check exactly like x + 1 — the literal should adapt
	// to whichever side is concretely typed, not just the left side
	// (CLAUDE.md's "確定した設計判断" for step 3).
	f := mainFile(
		&ast.LetExpr{Name: "x", Type: nt("Int8"), Value: &ast.IntLit{Value: 5}},
		&ast.LetExpr{Name: "y", Value: &ast.BinaryExpr{Op: "+", Left: &ast.IntLit{Value: 1}, Right: &ast.IdentExpr{Name: "x"}}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_ArithmeticBetweenDifferentTypesIsAnError(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "x", Type: nt("Int8"), Value: &ast.IntLit{Value: 5}},
		&ast.LetExpr{Name: "y", Type: nt("Int16"), Value: &ast.IntLit{Value: 5}},
		&ast.LetExpr{Name: "z", Value: &ast.BinaryExpr{Op: "+", Left: &ast.IdentExpr{Name: "x"}, Right: &ast.IdentExpr{Name: "y"}}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for Int8 + Int16 (no implicit conversion, principle 2)")
	}
}

func TestCheck_StringConcatWithPlusIsValid(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "s", Value: &ast.BinaryExpr{Op: "+", Left: &ast.StringLit{Value: "a"}, Right: &ast.StringLit{Value: "b"}}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	let := f.Decls[0].(*ast.FuncDecl).Body.Exprs[0].(*ast.LetExpr)
	if let.ResolvedType != "String" {
		t.Fatalf("got ResolvedType %q, want String", let.ResolvedType)
	}
}

func TestCheck_MinusOnStringsIsAnError(t *testing.T) {
	// Only `+` is Concatenable on String (amifl-spec.md section 6); `-` is
	// Numeric-only.
	f := mainFile(
		&ast.LetExpr{Name: "s", Value: &ast.BinaryExpr{Op: "-", Left: &ast.StringLit{Value: "a"}, Right: &ast.StringLit{Value: "b"}}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for \"a\" - \"b\"")
	}
}

func TestCheck_BitwiseOnFloatIsAnError(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "x", Value: &ast.BinaryExpr{Op: "&", Left: &ast.FloatLit{Value: 1}, Right: &ast.FloatLit{Value: 2}}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a bitwise operator on Float operands")
	}
}

func TestCheck_ShiftCountDefaultsToUInt(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "x", Value: &ast.BinaryExpr{Op: "<<", Left: &ast.IntLit{Value: 1}, Right: &ast.IntLit{Value: 2}}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_ShiftCountMustBeUInt(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "n", Value: &ast.FloatLit{Value: 2}},
		&ast.LetExpr{Name: "x", Value: &ast.BinaryExpr{Op: "<<", Left: &ast.IntLit{Value: 1}, Right: &ast.IdentExpr{Name: "n"}}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a Float shift count")
	}
}

func TestCheck_ComparisonProducesBool(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "ok", Type: nt("Bool"), Value: &ast.BinaryExpr{Op: "<", Left: &ast.IntLit{Value: 1}, Right: &ast.IntLit{Value: 2}}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_OrderedComparisonOnBoolIsAnError(t *testing.T) {
	// Bool has no Ordered capability (amifl-spec.md section 2.3) — only
	// == and != are defined for it.
	f := mainFile(
		&ast.LetExpr{Name: "ok", Value: &ast.BinaryExpr{Op: "<", Left: &ast.BoolLit{Value: true}, Right: &ast.BoolLit{Value: false}}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for Bool < Bool")
	}
}

func TestCheck_EqualityOnBoolIsValid(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "ok", Value: &ast.BinaryExpr{Op: "==", Left: &ast.BoolLit{Value: true}, Right: &ast.BoolLit{Value: false}}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_LogicalOperatorsRequireBool(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "x", Value: &ast.BinaryExpr{Op: "&&", Left: &ast.IntLit{Value: 1}, Right: &ast.BoolLit{Value: true}}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a non-Bool operand to &&")
	}
}

func TestCheck_UnaryNotRequiresBool(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "x", Value: &ast.UnaryExpr{Op: "!", Operand: &ast.IntLit{Value: 1}}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for !1")
	}
}

func TestCheck_UnaryMinusOnLetVariable(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "x", Type: nt("Int"), Value: &ast.IntLit{Value: 5}},
		&ast.LetExpr{Name: "y", Value: &ast.UnaryExpr{Op: "-", Operand: &ast.IdentExpr{Name: "x"}}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_NegatedInt8MinBoundaryIsValid(t *testing.T) {
	// -128 is Int8's minimum, even though the bare literal 128 alone
	// overflows Int8's positive range (max 127) — resolveNegatedIntLit's
	// whole reason to exist.
	f := mainFile(
		&ast.LetExpr{Name: "x", Type: nt("Int8"), Value: &ast.UnaryExpr{Op: "-", Operand: &ast.IntLit{Value: 128}}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_NegatedInt8PastMinBoundaryIsAnError(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "x", Type: nt("Int8"), Value: &ast.UnaryExpr{Op: "-", Operand: &ast.IntLit{Value: 129}}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for -129: Int8 (out of range)")
	}
}

func TestCheck_BitwiseNotOnFloatIsAnError(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "x", Value: &ast.UnaryExpr{Op: "~", Operand: &ast.FloatLit{Value: 1}}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for ~1.0")
	}
}

func TestCheck_ConstInitializerCanUseOperators(t *testing.T) {
	f := mainFile(&ast.IntLit{Value: 0})
	f.Decls = []ast.TopLevelDecl{
		&ast.ConstDecl{Name: "A", Value: &ast.IntLit{Value: 40}},
		&ast.ConstDecl{Name: "B", Value: &ast.IntLit{Value: 2}},
		&ast.ConstDecl{Name: "C", Value: &ast.BinaryExpr{Op: "+", Left: &ast.IdentExpr{Name: "A"}, Right: &ast.IdentExpr{Name: "B"}}},
		f.Decls[0],
	}
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_ConstInitializerCannotReferenceALetThroughAnOperator(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "x", Value: &ast.IntLit{Value: 1}},
		&ast.ConstDecl{Name: "Y", Value: &ast.BinaryExpr{Op: "+", Left: &ast.IdentExpr{Name: "x"}, Right: &ast.IntLit{Value: 1}}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a const initializer referencing a `let` through an operator expression")
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

func boolIdent(name string) *ast.IdentExpr { return &ast.IdentExpr{Name: name} }

func TestCheck_IfWithElseUnifiesBranchTypes(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "cond", Type: nt("Bool"), Value: &ast.BoolLit{Value: true}},
		&ast.LetExpr{Name: "x", Value: &ast.IfExpr{
			Cond: boolIdent("cond"),
			Then: &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 1}}},
			Else: &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 2}}},
		}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	let := f.Decls[0].(*ast.FuncDecl).Body.Exprs[1].(*ast.LetExpr)
	if let.ResolvedType != "Int64" {
		t.Fatalf("got ResolvedType %q, want Int64", let.ResolvedType)
	}
}

func TestCheck_IfBranchLiteralAdaptsToSiblingsConcreteType(t *testing.T) {
	// Mirrors step 3's binary-operand fix: a literal branch must adapt to
	// a sibling's concrete type regardless of which branch is written
	// first.
	for _, thenIsLiteral := range []bool{true, false} {
		var then, elseBlk *ast.Block
		lit := &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 1}}}
		concrete := &ast.Block{Exprs: []ast.Expr{&ast.IdentExpr{Name: "x"}}}
		if thenIsLiteral {
			then, elseBlk = lit, concrete
		} else {
			then, elseBlk = concrete, lit
		}
		f := mainFile(
			&ast.LetExpr{Name: "x", Type: nt("Int8"), Value: &ast.IntLit{Value: 5}},
			&ast.LetExpr{Name: "cond", Type: nt("Bool"), Value: &ast.BoolLit{Value: true}},
			&ast.LetExpr{Name: "y", Value: &ast.IfExpr{Cond: boolIdent("cond"), Then: then, Else: elseBlk}},
			&ast.IntLit{Value: 0},
		)
		if err := Check(f); err != nil {
			t.Fatalf("Check() error (thenIsLiteral=%v): %v", thenIsLiteral, err)
		}
	}
}

func TestCheck_IfWithoutElseMustBeUnit(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "cond", Type: nt("Bool"), Value: &ast.BoolLit{Value: true}},
		&ast.LetExpr{Name: "x", Value: &ast.IfExpr{
			Cond: boolIdent("cond"),
			Then: &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 1}}},
		}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for binding an else-less if (Unit-typed) to a let")
	}
}

func TestCheck_IfWithoutElseUsedAsAStatementIsValid(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "cond", Type: nt("Bool"), Value: &ast.BoolLit{Value: true}},
		&ast.IfExpr{
			Cond: boolIdent("cond"),
			Then: &ast.Block{Exprs: []ast.Expr{printStr("a")}},
		},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_IfBranchTypeMismatchIsAnError(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "cond", Type: nt("Bool"), Value: &ast.BoolLit{Value: true}},
		&ast.LetExpr{Name: "x", Value: &ast.IfExpr{
			Cond: boolIdent("cond"),
			Then: &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 1}}},
			Else: &ast.Block{Exprs: []ast.Expr{&ast.StringLit{Value: "a"}}},
		}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for if/else branches of different types")
	}
}

func TestCheck_ElifChainIsCheckedThroughTheDesugaredElse(t *testing.T) {
	// if a {1} elif b {2} else {3} — an IfExpr nested in Else, exactly as
	// the parser desugars elif (CLAUDE.md's "過去に踏まれた地雷" #2).
	f := mainFile(
		&ast.LetExpr{Name: "a", Type: nt("Bool"), Value: &ast.BoolLit{Value: true}},
		&ast.LetExpr{Name: "b", Type: nt("Bool"), Value: &ast.BoolLit{Value: false}},
		&ast.LetExpr{Name: "x", Value: &ast.IfExpr{
			Cond: boolIdent("a"),
			Then: &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 1}}},
			Else: &ast.IfExpr{
				Cond: boolIdent("b"),
				Then: &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 2}}},
				Else: &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 3}}},
			},
		}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_IfConditionMustBeBool(t *testing.T) {
	f := mainFile(
		&ast.IfExpr{
			Cond: &ast.IntLit{Value: 1},
			Then: &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 0}}},
		},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a non-Bool if condition")
	}
}

func TestCheck_NestedLetShadowsOuterLet(t *testing.T) {
	// The inner `x` is String-typed; printing it only type-checks (print
	// requires a String argument) if the reference resolves to the inner
	// shadow, not the outer Int `x`.
	f := mainFile(
		&ast.LetExpr{Name: "x", Type: nt("Int"), Value: &ast.IntLit{Value: 1}},
		&ast.IfExpr{
			Cond: &ast.BoolLit{Value: true},
			Then: &ast.Block{Exprs: []ast.Expr{
				&ast.LetExpr{Name: "x", Type: nt("String"), Value: &ast.StringLit{Value: "inner"}},
				&ast.CallExpr{Callee: "print", Args: []ast.Expr{&ast.IdentExpr{Name: "x"}}},
			}},
		},
		&ast.IdentExpr{Name: "x"},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_InnerScopeDeclarationDoesNotLeakOut(t *testing.T) {
	f := mainFile(
		&ast.IfExpr{
			Cond: &ast.BoolLit{Value: true},
			Then: &ast.Block{Exprs: []ast.Expr{
				&ast.LetExpr{Name: "y", Value: &ast.IntLit{Value: 1}},
			}},
		},
		&ast.IdentExpr{Name: "y"},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for referencing a name only declared inside an if-branch")
	}
}

func TestCheck_WhileConditionMustBeBool(t *testing.T) {
	f := mainFile(
		&ast.WhileExpr{Cond: &ast.IntLit{Value: 1}, Body: &ast.Block{}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a non-Bool while condition")
	}
}

func TestCheck_WhileBodyMustBeUnit(t *testing.T) {
	f := mainFile(
		&ast.WhileExpr{
			Cond: &ast.BoolLit{Value: true},
			Body: &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 1}}},
		},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a while body whose last expression isn't Unit")
	}
}

func TestCheck_WhileIsAlwaysUnit(t *testing.T) {
	f := mainFile(
		&ast.WhileExpr{Cond: &ast.BoolLit{Value: false}, Body: &ast.Block{}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_BreakInsideWhileIsValid(t *testing.T) {
	f := mainFile(
		&ast.WhileExpr{
			Cond: &ast.BoolLit{Value: true},
			Body: &ast.Block{Exprs: []ast.Expr{&ast.BreakExpr{}}},
		},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_ContinueInsideWhileIsValid(t *testing.T) {
	f := mainFile(
		&ast.WhileExpr{
			Cond: &ast.BoolLit{Value: true},
			Body: &ast.Block{Exprs: []ast.Expr{&ast.ContinueExpr{}}},
		},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_BreakOutsideLoopIsAnError(t *testing.T) {
	f := mainFile(&ast.BreakExpr{}, &ast.IntLit{Value: 0})
	if err := Check(f); err == nil {
		t.Fatal("expected an error for break outside of a loop")
	}
}

func TestCheck_ContinueOutsideLoopIsAnError(t *testing.T) {
	f := mainFile(&ast.ContinueExpr{}, &ast.IntLit{Value: 0})
	if err := Check(f); err == nil {
		t.Fatal("expected an error for continue outside of a loop")
	}
}

func TestCheck_BreakInsideIfInsideWhileIsValid(t *testing.T) {
	// break/continue only need *some* enclosing loop, not necessarily the
	// immediately enclosing block.
	f := mainFile(
		&ast.WhileExpr{
			Cond: &ast.BoolLit{Value: true},
			Body: &ast.Block{Exprs: []ast.Expr{
				&ast.IfExpr{
					Cond: &ast.BoolLit{Value: true},
					Then: &ast.Block{Exprs: []ast.Expr{&ast.BreakExpr{}}},
				},
			}},
		},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_BreakOutsideLoopButInsideIfIsAnError(t *testing.T) {
	f := mainFile(
		&ast.IfExpr{
			Cond: &ast.BoolLit{Value: true},
			Then: &ast.Block{Exprs: []ast.Expr{&ast.BreakExpr{}}},
		},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for break inside an if but outside any loop")
	}
}

// TestCheck_ReturnWithValueIsValid (ex11) checks a plain `return expr`
// matching the enclosing function's declared return type.
func TestCheck_ReturnWithValueIsValid(t *testing.T) {
	f := mainFile(
		&ast.IfExpr{
			Cond: &ast.BoolLit{Value: true},
			Then: &ast.Block{Exprs: []ast.Expr{&ast.ReturnExpr{Value: &ast.IntLit{Value: 5}}}},
		},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_ReturnValueTypeMismatchIsAnError(t *testing.T) {
	f := mainFile(&ast.ReturnExpr{Value: &ast.StringLit{Value: "wrong"}}, &ast.IntLit{Value: 0})
	if err := Check(f); err == nil {
		t.Fatal("expected an error: main returns Int, not String")
	}
}

func TestCheck_BareReturnInUnitFuncIsValid(t *testing.T) {
	f := &ast.File{Decls: []ast.TopLevelDecl{
		&ast.FuncDecl{
			Name:       "f",
			ReturnType: nt("Unit"),
			Body: &ast.Block{Exprs: []ast.Expr{
				&ast.IfExpr{Cond: &ast.BoolLit{Value: true}, Then: &ast.Block{Exprs: []ast.Expr{&ast.ReturnExpr{}}}},
				&ast.CallExpr{Callee: "print", Args: []ast.Expr{&ast.StringLit{Value: "after"}}},
			}},
		},
		&ast.FuncDecl{Name: "main", ReturnType: nt("Int"), Body: &ast.Block{Exprs: []ast.Expr{
			&ast.DiscardExpr{Value: &ast.CallExpr{Callee: "f"}},
			&ast.IntLit{Value: 0},
		}}},
	}}
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_BareReturnInNonUnitFuncIsAnError(t *testing.T) {
	f := mainFile(&ast.ReturnExpr{}, &ast.IntLit{Value: 0})
	if err := Check(f); err == nil {
		t.Fatal("expected an error: bare `return` needs a value since main returns Int, not Unit")
	}
}

// TestCheck_ReturnAsIfBranchIsValid (ex11's main point) checks
// `if done { return 5 } else { 10 }` — a return branch alongside a sibling
// of a real, unrelated type.
func TestCheck_ReturnAsIfBranchIsValid(t *testing.T) {
	f := mainFile(
		&ast.IfExpr{
			Cond: &ast.BoolLit{Value: true},
			Then: &ast.Block{Exprs: []ast.Expr{&ast.ReturnExpr{Value: &ast.IntLit{Value: 5}}}},
			Else: &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 10}}},
		},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

// TestCheck_BreakAsIfBranchIsValid (ex11 generalized break/continue to
// neverType too) checks `if done { break } else { 5 }` inside a loop —
// previously rejected (step 4's own doc comment invited revisiting this
// "once return/Never's design is settled for real").
func TestCheck_BreakAsIfBranchIsValid(t *testing.T) {
	f := mainFile(
		&ast.WhileExpr{
			Cond: &ast.BoolLit{Value: true},
			Body: &ast.Block{Exprs: []ast.Expr{
				&ast.DiscardExpr{Value: &ast.IfExpr{
					Cond: &ast.BoolLit{Value: true},
					Then: &ast.Block{Exprs: []ast.Expr{&ast.BreakExpr{}}},
					Else: &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 5}}},
				}},
			}},
		},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

// TestCheck_AllBranchesDivergeResolvesToNever confirms an if/else whose
// every branch diverges (here, both return) doesn't force a spurious type
// mismatch against a differently-typed outer expected — there's genuinely
// no value produced either way.
func TestCheck_AllBranchesDivergeResolvesToNever(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "x", Value: &ast.IfExpr{
			Cond: &ast.BoolLit{Value: true},
			Then: &ast.Block{Exprs: []ast.Expr{&ast.ReturnExpr{Value: &ast.IntLit{Value: 1}}}},
			Else: &ast.Block{Exprs: []ast.Expr{&ast.ReturnExpr{Value: &ast.IntLit{Value: 2}}}},
		}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error: `x` can never actually be bound to a value (every branch diverges)")
	}
}

// TestCheck_LetBoundDirectlyToReturnIsAnError confirms `let x = return 5`
// (Value *directly* a ReturnExpr, not nested inside an if/switch branch)
// is rejected — there is no value left to bind once control diverges.
func TestCheck_LetBoundDirectlyToReturnIsAnError(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "x", Value: &ast.ReturnExpr{Value: &ast.IntLit{Value: 5}}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error binding a let directly to a return")
	}
}

// TestCheck_AssignDirectlyFromReturnIsAnError is
// TestCheck_LetBoundDirectlyToReturnIsAnError's counterpart for plain
// reassignment.
func TestCheck_AssignDirectlyFromReturnIsAnError(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "x", Value: &ast.IntLit{Value: 0}},
		&ast.AssignExpr{Name: "x", Value: &ast.ReturnExpr{Value: &ast.IntLit{Value: 5}}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error assigning directly from a return")
	}
}

// TestCheck_ReturnInsideClosureUsesClosureRetType confirms a `return`
// inside a closure body checks against the *closure's* own declared return
// type, not the enclosing function's — the same closure-boundary save/
// restore `?`'s fc.retType handling already established (resolveClosureLit).
func TestCheck_ReturnInsideClosureUsesClosureRetType(t *testing.T) {
	f := mainFile(
		// Enclosing main() returns Int, but the closure returns Bool — a
		// `return true` inside the closure must check against Bool, not Int.
		&ast.LetExpr{Name: "f", Value: &ast.ClosureLit{
			Params:     []ast.Param{{Name: "x", Type: nt("Int")}},
			ReturnType: nt("Bool"),
			Body: &ast.Block{Exprs: []ast.Expr{
				&ast.IfExpr{
					Cond: &ast.BinaryExpr{Op: "<", Left: &ast.IdentExpr{Name: "x"}, Right: &ast.IntLit{Value: 0}},
					Then: &ast.Block{Exprs: []ast.Expr{&ast.ReturnExpr{Value: &ast.BoolLit{Value: true}}}},
					Else: &ast.Block{Exprs: []ast.Expr{&ast.BoolLit{Value: false}}},
				},
			}},
		}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_LoopDepthRestoredAfterWhileExits(t *testing.T) {
	// A break after a while loop has finished checking must still be
	// rejected — loopDepth must be decremented back down, not just ever
	// incremented.
	f := mainFile(
		&ast.WhileExpr{Cond: &ast.BoolLit{Value: true}, Body: &ast.Block{Exprs: []ast.Expr{&ast.BreakExpr{}}}},
		&ast.BreakExpr{},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for break after the while it was inside of has ended")
	}
}

func TestCheck_FuncWithParamsIsCallable(t *testing.T) {
	f := mainFile(
		&ast.CallExpr{Callee: "addNums", Args: []ast.Expr{&ast.IntLit{Value: 1}, &ast.IntLit{Value: 2}}},
	)
	f.Decls = append(f.Decls, &ast.FuncDecl{
		Name:       "addNums",
		Params:     []ast.Param{{Name: "a", Type: nt("Int")}, {Name: "b", Type: nt("Int")}},
		ReturnType: nt("Int"),
		Body: &ast.Block{Exprs: []ast.Expr{
			&ast.BinaryExpr{Op: "+", Left: &ast.IdentExpr{Name: "a"}, Right: &ast.IdentExpr{Name: "b"}},
		}},
	})
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_CallWrongArgCountIsAnError(t *testing.T) {
	f := mainFile(&ast.CallExpr{Callee: "addNums", Args: []ast.Expr{&ast.IntLit{Value: 1}}})
	f.Decls = append(f.Decls, &ast.FuncDecl{
		Name:       "addNums",
		Params:     []ast.Param{{Name: "a", Type: nt("Int")}, {Name: "b", Type: nt("Int")}},
		ReturnType: nt("Int"),
		Body:       &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 0}}},
	})
	if err := Check(f); err == nil {
		t.Fatal("expected an error for calling addNums with the wrong number of arguments")
	}
}

func TestCheck_CallWrongArgTypeIsAnError(t *testing.T) {
	f := mainFile(&ast.CallExpr{Callee: "addNums", Args: []ast.Expr{&ast.StringLit{Value: "x"}, &ast.IntLit{Value: 2}}})
	f.Decls = append(f.Decls, &ast.FuncDecl{
		Name:       "addNums",
		Params:     []ast.Param{{Name: "a", Type: nt("Int")}, {Name: "b", Type: nt("Int")}},
		ReturnType: nt("Int"),
		Body:       &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 0}}},
	})
	if err := Check(f); err == nil {
		t.Fatal("expected an error for calling addNums with a String where an Int is expected")
	}
}

func TestCheck_DuplicateParamNameIsAnError(t *testing.T) {
	f := mainFile(&ast.IntLit{Value: 0})
	f.Decls = append(f.Decls, &ast.FuncDecl{
		Name:       "bad",
		Params:     []ast.Param{{Name: "a", Type: nt("Int")}, {Name: "a", Type: nt("Int")}},
		ReturnType: nt("Int"),
		Body:       &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 0}}},
	})
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a duplicate parameter name")
	}
}

func TestCheck_ParamIsNotReassignable(t *testing.T) {
	f := mainFile(&ast.IntLit{Value: 0})
	f.Decls = append(f.Decls, &ast.FuncDecl{
		Name:       "bad",
		Params:     []ast.Param{{Name: "a", Type: nt("Int")}},
		ReturnType: nt("Int"),
		Body: &ast.Block{Exprs: []ast.Expr{
			&ast.AssignExpr{Name: "a", Value: &ast.IntLit{Value: 5}},
			&ast.IdentExpr{Name: "a"},
		}},
	})
	if err := Check(f); err == nil {
		t.Fatal("expected an error for reassigning a function parameter")
	}
}

func TestCheck_SelfRecursionIsValid(t *testing.T) {
	f := mainFile(&ast.IntLit{Value: 0})
	f.Decls = append(f.Decls, &ast.FuncDecl{
		Name:       "loop",
		Params:     []ast.Param{{Name: "n", Type: nt("Int")}},
		ReturnType: nt("Int"),
		Body: &ast.Block{Exprs: []ast.Expr{
			&ast.CallExpr{Callee: "loop", Args: []ast.Expr{&ast.IdentExpr{Name: "n"}}},
		}},
	})
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_ForwardReferenceAndMutualRecursionAreValid(t *testing.T) {
	// `even` calls `odd`, declared *after* it in the file — and `odd`
	// calls back into `even` — both must resolve regardless of order,
	// since every signature is registered before any body is checked.
	f := mainFile(&ast.IntLit{Value: 0})
	f.Decls = append(f.Decls,
		&ast.FuncDecl{
			Name:       "even",
			Params:     []ast.Param{{Name: "n", Type: nt("Int")}},
			ReturnType: nt("Bool"),
			Body: &ast.Block{Exprs: []ast.Expr{
				&ast.CallExpr{Callee: "odd", Args: []ast.Expr{&ast.IdentExpr{Name: "n"}}},
			}},
		},
		&ast.FuncDecl{
			Name:       "odd",
			Params:     []ast.Param{{Name: "n", Type: nt("Int")}},
			ReturnType: nt("Bool"),
			Body: &ast.Block{Exprs: []ast.Expr{
				&ast.CallExpr{Callee: "even", Args: []ast.Expr{&ast.IdentExpr{Name: "n"}}},
			}},
		},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_DuplicateFuncNameIsAnError(t *testing.T) {
	f := mainFile(&ast.IntLit{Value: 0})
	dup := &ast.FuncDecl{Name: "helper", ReturnType: nt("Int"), Body: &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 0}}}}
	f.Decls = append(f.Decls, dup, dup)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a duplicate function name")
	}
}

func TestCheck_FuncNameCollidingWithConstIsAnError(t *testing.T) {
	f := mainFile(&ast.IntLit{Value: 0})
	f.Decls = append(f.Decls,
		&ast.ConstDecl{Name: "X", Value: &ast.IntLit{Value: 1}},
		&ast.FuncDecl{Name: "X", ReturnType: nt("Int"), Body: &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 0}}}},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a fn name colliding with a top-level const")
	}
}

func TestCheck_ReservedMainNameIsAnError(t *testing.T) {
	f := mainFile(&ast.IntLit{Value: 0})
	f.Decls = append(f.Decls, &ast.FuncDecl{
		Name: "amifl_main", ReturnType: nt("Int"), Body: &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 0}}},
	})
	if err := Check(f); err == nil {
		t.Fatal("expected an error for declaring a fn named the reserved amifl_main")
	}
}

func TestCheck_MainWithWrongSingleParamTypeIsAnError(t *testing.T) {
	// A single parameter is allowed (amifl-spec.md section 14's
	// `fn main(args: List[String])` form) but only if it's exactly
	// List[String] — anything else, including a bare scalar like Int, is
	// still rejected.
	f := &ast.File{Decls: []ast.TopLevelDecl{
		&ast.FuncDecl{
			Name:       "main",
			Params:     []ast.Param{{Name: "x", Type: nt("Int")}},
			ReturnType: nt("Int"),
			Body:       &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 0}}},
		},
	}}
	if err := Check(f); err == nil {
		t.Fatal("expected an error for fn main's single parameter not being List[String]")
	}
}

func TestCheck_MainWithTwoParamsIsAnError(t *testing.T) {
	f := &ast.File{Decls: []ast.TopLevelDecl{
		&ast.FuncDecl{
			Name:       "main",
			Params:     []ast.Param{{Name: "x", Type: nt("Int")}, {Name: "y", Type: nt("Int")}},
			ReturnType: nt("Int"),
			Body:       &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 0}}},
		},
	}}
	if err := Check(f); err == nil {
		t.Fatal("expected an error for fn main taking more than one parameter")
	}
}

func TestCheck_MainWithListStringArgsParamIsValid(t *testing.T) {
	// amifl-spec.md section 14's `fn main(args: List[String]) -> Int` form.
	f := &ast.File{Decls: []ast.TopLevelDecl{
		&ast.FuncDecl{
			Name:       "main",
			Params:     []ast.Param{{Name: "args", Type: lt(nt("String"))}},
			ReturnType: nt("Int"),
			Body: &ast.Block{Exprs: []ast.Expr{
				&ast.CallExpr{Callee: "len", Args: []ast.Expr{&ast.IdentExpr{Name: "args"}}},
			}},
		},
	}}
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_UnitReturnTypeIsValidOnlyForFunctions(t *testing.T) {
	f := mainFile(&ast.CallExpr{Callee: "log", Args: []ast.Expr{&ast.StringLit{Value: "hi"}}}, &ast.IntLit{Value: 0})
	f.Decls = append(f.Decls, &ast.FuncDecl{
		Name:       "log",
		Params:     []ast.Param{{Name: "msg", Type: nt("String")}},
		ReturnType: nt("Unit"),
		Body:       &ast.Block{Exprs: []ast.Expr{printStr("logged")}},
	})
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_LetTypeAnnotationCannotBeUnit(t *testing.T) {
	f := mainFile(&ast.LetExpr{Name: "x", Type: nt("Unit"), Value: &ast.IntLit{Value: 0}}, &ast.IntLit{Value: 0})
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a `let` annotated Unit (only a function's own return type may be Unit)")
	}
}

func TestCheck_ClosureLitBoundToLetIsCallable(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "square", Value: &ast.ClosureLit{
			Params:     []ast.Param{{Name: "x", Type: nt("Int")}},
			ReturnType: nt("Int"),
			Body: &ast.Block{Exprs: []ast.Expr{
				&ast.BinaryExpr{Op: "*", Left: &ast.IdentExpr{Name: "x"}, Right: &ast.IdentExpr{Name: "x"}},
			}},
		}},
		&ast.CallExpr{Callee: "square", Args: []ast.Expr{&ast.IntLit{Value: 5}}},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_ClosureCapturesOuterLet(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "base", Type: nt("Int"), Value: &ast.IntLit{Value: 10}},
		&ast.LetExpr{Name: "addBase", Value: &ast.ClosureLit{
			Params:     []ast.Param{{Name: "x", Type: nt("Int")}},
			ReturnType: nt("Int"),
			Body: &ast.Block{Exprs: []ast.Expr{
				&ast.BinaryExpr{Op: "+", Left: &ast.IdentExpr{Name: "x"}, Right: &ast.IdentExpr{Name: "base"}},
			}},
		}},
		&ast.CallExpr{Callee: "addBase", Args: []ast.Expr{&ast.IntLit{Value: 5}}},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_ClosureLitWithNonFuncTypeAnnotationIsAnError(t *testing.T) {
	// "Int" isn't even a Func-shaped annotation at all — still rejected,
	// same as ever, though ex3 changed *why* (a mismatch against the
	// closure's own inferred type, not "annotations are always forbidden"
	// — see TestCheck_ClosureLitWithMatchingFuncTypeAnnotationIsValid for
	// the now-valid case a bare rejection-of-all-annotations would miss).
	f := mainFile(
		&ast.LetExpr{Name: "square", Type: nt("Int"), Value: &ast.ClosureLit{
			Params:     []ast.Param{{Name: "x", Type: nt("Int")}},
			ReturnType: nt("Int"),
			Body:       &ast.Block{Exprs: []ast.Expr{&ast.IdentExpr{Name: "x"}}},
		}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a closure-valued `let` annotated with a non-Func type")
	}
}

func TestCheck_ClosureLitWithMatchingFuncTypeAnnotationIsValid(t *testing.T) {
	// Ex3: a closure-valued `let` may now carry a FuncType annotation, as
	// long as it agrees with the closure's own self-inferred signature.
	f := mainFile(
		&ast.LetExpr{Name: "square", Type: ft([]ast.TypeExpr{nt("Int")}, nt("Int")), Value: &ast.ClosureLit{
			Params:     []ast.Param{{Name: "x", Type: nt("Int")}},
			ReturnType: nt("Int"),
			Body:       &ast.Block{Exprs: []ast.Expr{&ast.IdentExpr{Name: "x"}}},
		}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_ClosureLitWithMismatchedFuncTypeAnnotationIsAnError(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "square", Type: ft([]ast.TypeExpr{nt("Int")}, nt("Bool")), Value: &ast.ClosureLit{
			Params:     []ast.Param{{Name: "x", Type: nt("Int")}},
			ReturnType: nt("Int"),
			Body:       &ast.Block{Exprs: []ast.Expr{&ast.IdentExpr{Name: "x"}}},
		}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a closure literal's inferred type disagreeing with its own annotation")
	}
}

func TestCheck_ClosureLitAsCallArgumentIsAnError(t *testing.T) {
	// An inline ClosureLit only supports being a `let`'s direct value —
	// not a call argument (deferred, tracked separately from ex3's own
	// scope — see ast.ClosureLit's doc comment).
	f := mainFile(
		&ast.CallExpr{Callee: "print", Args: []ast.Expr{&ast.ClosureLit{
			ReturnType: nt("Int"),
			Body:       &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 0}}},
		}}},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a closure literal used as a call argument")
	}
}

// TestCheck_PipeInlineClosureRHSIsValid builds the CallExpr shape
// parser.parsePipeRHS produces for `x |> fn(v: Int) -> Int { v + 1 }`
// (InlineClosure set, Callee left at the "<closure>" display placeholder,
// Args always exactly [lhs]) — ex4.
func TestCheck_PipeInlineClosureRHSIsValid(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "x", Type: nt("Int"), Value: &ast.IntLit{Value: 5}},
		&ast.LetExpr{Name: "a", Value: &ast.CallExpr{
			Callee: "<closure>",
			InlineClosure: &ast.ClosureLit{
				Params:     []ast.Param{{Name: "v", Type: nt("Int")}},
				ReturnType: nt("Int"),
				Body: &ast.Block{Exprs: []ast.Expr{
					&ast.BinaryExpr{Op: "+", Left: &ast.IdentExpr{Name: "v"}, Right: &ast.IntLit{Value: 1}},
				}},
			},
			Args: []ast.Expr{&ast.IdentExpr{Name: "x"}},
		}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_PipeInlineClosureWrongArityIsAnError(t *testing.T) {
	// The closure takes 2 params, but a pipe's RHS always supplies exactly
	// 1 argument (lhs) — checkCallArgs' ordinary arity check catches this
	// exactly as it would for any other call.
	f := mainFile(
		&ast.LetExpr{Name: "x", Type: nt("Int"), Value: &ast.IntLit{Value: 5}},
		&ast.DiscardExpr{Value: &ast.CallExpr{
			Callee: "<closure>",
			InlineClosure: &ast.ClosureLit{
				Params:     []ast.Param{{Name: "a", Type: nt("Int")}, {Name: "b", Type: nt("Int")}},
				ReturnType: nt("Int"),
				Body:       &ast.Block{Exprs: []ast.Expr{&ast.IdentExpr{Name: "a"}}},
			},
			Args: []ast.Expr{&ast.IdentExpr{Name: "x"}},
		}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for an inline pipe closure declaring 2 params but receiving 1 argument")
	}
}

// TestCheck_PipeInlineClosureStageMismatchProducesStageNumberedError mirrors
// TestCheck_PipeStageMismatchProducesStageNumberedError but with an inline
// closure as the mismatching stage, confirming amifl-spec.md section 9.1's
// diagnostic covers this RHS form too (checkCallArgs routes InlineClosure's
// arguments through checkExprPipeAware exactly like any other call).
func TestCheck_PipeInlineClosureStageMismatchProducesStageNumberedError(t *testing.T) {
	labels := []string{"data", "parseIt", "<closure>"}
	stageA := &ast.CallExpr{
		Callee: "parseIt", Args: []ast.Expr{&ast.IdentExpr{Name: "data"}},
		PipeStage: 1, PipeArgIndex: 0, PipeChainLabels: labels,
	}
	stageB := &ast.CallExpr{
		Callee: "<closure>",
		InlineClosure: &ast.ClosureLit{
			Params:     []ast.Param{{Name: "s", Type: nt("String")}},
			ReturnType: nt("Int"),
			Body:       &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 1}}},
		},
		Args:            []ast.Expr{stageA},
		PipeStage:       2,
		PipeArgIndex:    0,
		PipeChainLabels: labels,
	}
	f := &ast.File{Decls: []ast.TopLevelDecl{
		&ast.FuncDecl{
			Name: "parseIt", Params: []ast.Param{{Name: "s", Type: nt("String")}}, ReturnType: nt("Int"),
			Body: &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 5}}},
		},
		&ast.FuncDecl{
			Name: "main", ReturnType: nt("Int"),
			Body: &ast.Block{Exprs: []ast.Expr{
				&ast.LetExpr{Name: "data", Type: nt("String"), Value: &ast.StringLit{Value: "hi"}},
				&ast.DiscardExpr{Value: stageB},
				&ast.IntLit{Value: 0},
			}},
		},
	}}
	err := Check(f)
	if err == nil {
		t.Fatal("expected a pipeline type-mismatch error: parseIt returns Int, the inline closure expects String")
	}
	msg := err.Error()
	for _, want := range []string{
		"pipeline type mismatch at stage 2 (<closure>)",
		"pipeline: data |> parseIt |> <closure>",
		"stage 1 (parseIt) outputs: Int64",
		"stage 2 (<closure>) expects: String",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error message %q doesn't contain %q", msg, want)
		}
	}
}

// TestCheck_PipeInlineClosureBreakInsideIsAnError confirms an inline pipe
// closure still counts as a closure boundary for break/continue (resolveCallExpr
// routes InlineClosure through the exact same resolveClosureLit a `let`
// uses, which saves/resets loopDepth around the body) even when a `while`
// loop syntactically encloses the whole pipe expression.
func TestCheck_PipeInlineClosureBreakInsideIsAnError(t *testing.T) {
	f := mainFile(
		&ast.WhileExpr{
			Cond: &ast.BoolLit{Value: true},
			Body: &ast.Block{Exprs: []ast.Expr{
				&ast.DiscardExpr{Value: &ast.CallExpr{
					Callee: "<closure>",
					InlineClosure: &ast.ClosureLit{
						Params:     []ast.Param{{Name: "v", Type: nt("Int")}},
						ReturnType: nt("Int"),
						Body:       &ast.Block{Exprs: []ast.Expr{&ast.BreakExpr{}}},
					},
					Args: []ast.Expr{&ast.IntLit{Value: 5}},
				}},
				&ast.BreakExpr{},
			}},
		},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for break inside an inline pipe closure, even with an enclosing while loop")
	}
}

func TestCheck_TopLevelFnReferencedByNameAsValueIsValid(t *testing.T) {
	// Ex3: `let f = add` where `add` is a top-level `fn`, previously
	// deferred (step 5's "トップレベル関数を名前で値として渡す" scope cut).
	f := &ast.File{Decls: []ast.TopLevelDecl{
		&ast.FuncDecl{
			Name:       "add",
			Params:     []ast.Param{{Name: "a", Type: nt("Int")}, {Name: "b", Type: nt("Int")}},
			ReturnType: nt("Int"),
			Body:       &ast.Block{Exprs: []ast.Expr{&ast.BinaryExpr{Op: "+", Left: &ast.IdentExpr{Name: "a"}, Right: &ast.IdentExpr{Name: "b"}}}},
		},
		&ast.FuncDecl{
			Name:       "main",
			ReturnType: nt("Int"),
			Body: &ast.Block{Exprs: []ast.Expr{
				&ast.LetExpr{Name: "f", Value: &ast.IdentExpr{Name: "add"}},
				&ast.CallExpr{Callee: "f", Args: []ast.Expr{&ast.IntLit{Value: 3}, &ast.IntLit{Value: 4}}},
			}},
		},
	}}
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_TopLevelFnReferencedWithMismatchedFuncTypeAnnotationIsAnError(t *testing.T) {
	f := &ast.File{Decls: []ast.TopLevelDecl{
		&ast.FuncDecl{
			Name:       "add",
			Params:     []ast.Param{{Name: "a", Type: nt("Int")}, {Name: "b", Type: nt("Int")}},
			ReturnType: nt("Int"),
			Body:       &ast.Block{Exprs: []ast.Expr{&ast.BinaryExpr{Op: "+", Left: &ast.IdentExpr{Name: "a"}, Right: &ast.IdentExpr{Name: "b"}}}},
		},
		&ast.FuncDecl{
			Name:       "main",
			ReturnType: nt("Int"),
			Body: &ast.Block{Exprs: []ast.Expr{
				&ast.LetExpr{Name: "f", Type: ft([]ast.TypeExpr{nt("Int")}, nt("Bool")), Value: &ast.IdentExpr{Name: "add"}},
				&ast.IntLit{Value: 0},
			}},
		},
	}}
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a top-level `fn` reference whose actual signature doesn't match its `let` annotation")
	}
}

func TestCheck_MethodStyleExternBindReferencedAsValueIsAnError(t *testing.T) {
	f := &ast.File{Decls: []ast.TopLevelDecl{
		&ast.ExternDecl{
			Path:  "time",
			Alias: "time",
			Types: []ast.ExternTypeDecl{{Name: "Time"}},
			Binds: []ast.ExternBindDecl{
				{Name: "Now", ReturnType: nt("Time")},
				{Name: "TimeUnix", Params: []ast.Param{{Name: "t", Type: nt("Time")}}, ReturnType: nt("Int"), GoTarget: "Time.Unix"},
			},
		},
		&ast.FuncDecl{
			Name:       "main",
			ReturnType: nt("Int"),
			Body: &ast.Block{Exprs: []ast.Expr{
				&ast.LetExpr{Name: "f", Value: &ast.IdentExpr{Name: "TimeUnix"}},
				&ast.IntLit{Value: 0},
			}},
		},
	}}
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a method-style extern bind referenced by name as a value")
	}
}

func TestCheck_FuncTypedParamIsCallableInsideFunctionBody(t *testing.T) {
	// `fn apply(f: fn(Int) -> Int, x: Int) -> Int { f(x) }` — a genuine
	// user-defined higher-order function, ex3's core new capability.
	f := &ast.File{Decls: []ast.TopLevelDecl{
		&ast.FuncDecl{
			Name: "apply",
			Params: []ast.Param{
				{Name: "f", Type: ft([]ast.TypeExpr{nt("Int")}, nt("Int"))},
				{Name: "x", Type: nt("Int")},
			},
			ReturnType: nt("Int"),
			Body: &ast.Block{Exprs: []ast.Expr{
				&ast.CallExpr{Callee: "f", Args: []ast.Expr{&ast.IdentExpr{Name: "x"}}},
			}},
		},
		&ast.FuncDecl{
			Name:       "double",
			Params:     []ast.Param{{Name: "n", Type: nt("Int")}},
			ReturnType: nt("Int"),
			Body:       &ast.Block{Exprs: []ast.Expr{&ast.BinaryExpr{Op: "*", Left: &ast.IdentExpr{Name: "n"}, Right: &ast.IntLit{Value: 2}}}},
		},
		&ast.FuncDecl{
			Name:       "main",
			ReturnType: nt("Int"),
			Body: &ast.Block{Exprs: []ast.Expr{
				&ast.CallExpr{Callee: "apply", Args: []ast.Expr{&ast.IdentExpr{Name: "double"}, &ast.IntLit{Value: 5}}},
			}},
		},
	}}
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_FuncTypedParamRejectsWrongSignature(t *testing.T) {
	f := &ast.File{Decls: []ast.TopLevelDecl{
		&ast.FuncDecl{
			Name:       "apply",
			Params:     []ast.Param{{Name: "f", Type: ft([]ast.TypeExpr{nt("Int")}, nt("Int"))}, {Name: "x", Type: nt("Int")}},
			ReturnType: nt("Int"),
			Body:       &ast.Block{Exprs: []ast.Expr{&ast.CallExpr{Callee: "f", Args: []ast.Expr{&ast.IdentExpr{Name: "x"}}}}},
		},
		&ast.FuncDecl{
			Name:       "isPositive",
			Params:     []ast.Param{{Name: "n", Type: nt("Int")}},
			ReturnType: nt("Bool"),
			Body:       &ast.Block{Exprs: []ast.Expr{&ast.BinaryExpr{Op: ">", Left: &ast.IdentExpr{Name: "n"}, Right: &ast.IntLit{Value: 0}}}},
		},
		&ast.FuncDecl{
			Name:       "main",
			ReturnType: nt("Int"),
			Body: &ast.Block{Exprs: []ast.Expr{
				&ast.CallExpr{Callee: "apply", Args: []ast.Expr{&ast.IdentExpr{Name: "isPositive"}, &ast.IntLit{Value: 5}}},
			}},
		},
	}}
	if err := Check(f); err == nil {
		t.Fatal("expected an error for passing fn(Int)->Bool where fn(Int)->Int is required")
	}
}

func TestCheck_NestedFuncTypedParamResolvesCorrectly(t *testing.T) {
	// `fn compose(g: fn(Int) -> Int, x: Int) -> Int { g(x) }` — a Func type
	// nested inside another Func type's own parameter list. This is
	// exactly the shape (examples/higher_order_functions.aml's `compose`)
	// that turned out to need funcTypeParts' depth-aware ")->" fix: a naive
	// strings.Index(t, ")->") finds the *inner* Func type's own ")->" and
	// silently truncates the outer parameter list to one entry instead of
	// two — this test locks that fix in at the sema layer directly (the
	// example file exercises it end-to-end through actual codegen too).
	f := &ast.File{Decls: []ast.TopLevelDecl{
		&ast.FuncDecl{
			Name: "compose",
			Params: []ast.Param{
				{Name: "g", Type: ft([]ast.TypeExpr{nt("Int")}, nt("Int"))},
				{Name: "x", Type: nt("Int")},
			},
			ReturnType: nt("Int"),
			Body: &ast.Block{Exprs: []ast.Expr{
				&ast.CallExpr{Callee: "g", Args: []ast.Expr{&ast.IdentExpr{Name: "x"}}},
			}},
		},
		&ast.FuncDecl{
			Name:       "double",
			Params:     []ast.Param{{Name: "n", Type: nt("Int")}},
			ReturnType: nt("Int"),
			Body:       &ast.Block{Exprs: []ast.Expr{&ast.BinaryExpr{Op: "*", Left: &ast.IdentExpr{Name: "n"}, Right: &ast.IntLit{Value: 2}}}},
		},
		&ast.FuncDecl{
			Name:       "main",
			ReturnType: nt("Int"),
			Body: &ast.Block{Exprs: []ast.Expr{
				// compose expects exactly 2 arguments — if the depth-aware
				// fix regressed, sema would instead report the truncated
				// arity (1) here and this call would spuriously fail.
				&ast.CallExpr{Callee: "compose", Args: []ast.Expr{&ast.IdentExpr{Name: "double"}, &ast.IntLit{Value: 7}}},
			}},
		},
	}}
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_TopLevelFnReturningFuncTypeIsValid(t *testing.T) {
	// `fn makeAdder(n: Int) -> fn(Int) -> Int { let f = fn(x: Int) -> Int { x + n }; f }`
	f := &ast.File{Decls: []ast.TopLevelDecl{
		&ast.FuncDecl{
			Name:       "makeAdder",
			Params:     []ast.Param{{Name: "n", Type: nt("Int")}},
			ReturnType: ft([]ast.TypeExpr{nt("Int")}, nt("Int")),
			Body: &ast.Block{Exprs: []ast.Expr{
				&ast.LetExpr{Name: "f", Value: &ast.ClosureLit{
					Params:     []ast.Param{{Name: "x", Type: nt("Int")}},
					ReturnType: nt("Int"),
					Body:       &ast.Block{Exprs: []ast.Expr{&ast.BinaryExpr{Op: "+", Left: &ast.IdentExpr{Name: "x"}, Right: &ast.IdentExpr{Name: "n"}}}},
				}},
				&ast.IdentExpr{Name: "f"},
			}},
		},
		&ast.FuncDecl{
			Name:       "main",
			ReturnType: nt("Int"),
			Body: &ast.Block{Exprs: []ast.Expr{
				&ast.LetExpr{Name: "addThree", Value: &ast.CallExpr{Callee: "makeAdder", Args: []ast.Expr{&ast.IntLit{Value: 3}}}},
				&ast.CallExpr{Callee: "addThree", Args: []ast.Expr{&ast.IntLit{Value: 5}}},
			}},
		},
	}}
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_CallingNonFunctionIsAnError(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "x", Type: nt("Int"), Value: &ast.IntLit{Value: 1}},
		&ast.CallExpr{Callee: "x", Args: nil},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for calling a non-function value")
	}
}

func TestCheck_CallingUndefinedFunctionIsAnError(t *testing.T) {
	f := mainFile(&ast.CallExpr{Callee: "nope", Args: nil})
	if err := Check(f); err == nil {
		t.Fatal("expected an error for calling an undefined function")
	}
}

func TestCheck_FuncValuesCannotBeCompared(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "a", Value: &ast.ClosureLit{
			ReturnType: nt("Int"),
			Body:       &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 0}}},
		}},
		&ast.LetExpr{Name: "b", Value: &ast.ClosureLit{
			ReturnType: nt("Int"),
			Body:       &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 0}}},
		}},
		&ast.BinaryExpr{Op: "==", Left: &ast.IdentExpr{Name: "a"}, Right: &ast.IdentExpr{Name: "b"}},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for comparing two function values with ==")
	}
}

func TestCheck_LocalClosureShadowsSameNamedTopLevelFunc(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "helper", Value: &ast.ClosureLit{
			ReturnType: nt("Int"),
			Body:       &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 99}}},
		}},
		&ast.CallExpr{Callee: "helper", Args: nil},
	)
	f.Decls = append(f.Decls, &ast.FuncDecl{
		Name:       "helper",
		ReturnType: nt("String"),
		Body:       &ast.Block{Exprs: []ast.Expr{&ast.StringLit{Value: "top-level"}}},
	})
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func pointStructDecl() *ast.StructDecl {
	return &ast.StructDecl{
		Name: "Point",
		Fields: []ast.Param{
			{Name: "x", Type: nt("Int")},
			{Name: "y", Type: nt("Int")},
		},
	}
}

func pointLit(x, y uint64) *ast.StructLit {
	return &ast.StructLit{
		TypeName: "Point",
		Fields: []ast.StructLitField{
			{Name: "x", Value: &ast.IntLit{Value: x}},
			{Name: "y", Value: &ast.IntLit{Value: y}},
		},
	}
}

func TestCheck_StructLitAndFieldAccessAreValid(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "p", Value: pointLit(1, 2)},
		&ast.FieldExpr{Target: &ast.IdentExpr{Name: "p"}, Field: "x"},
	)
	f.Decls = append(f.Decls, pointStructDecl())
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_StructFieldAccessResolvesFieldType(t *testing.T) {
	field := &ast.FieldExpr{Target: &ast.IdentExpr{Name: "p"}, Field: "x"}
	f := mainFile(
		&ast.LetExpr{Name: "p", Value: pointLit(1, 2)},
		&ast.DiscardExpr{Value: field},
		&ast.IntLit{Value: 0},
	)
	f.Decls = append(f.Decls, pointStructDecl())
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if field.ResolvedType != "Int64" {
		t.Fatalf("got ResolvedType %q, want Int64", field.ResolvedType)
	}
	if field.AmivmField != "x" {
		t.Fatalf("got AmivmField %q, want \"x\" (struct field verbatim)", field.AmivmField)
	}
}

func TestCheck_DuplicateStructNameIsAnError(t *testing.T) {
	f := mainFile(&ast.IntLit{Value: 0})
	f.Decls = append(f.Decls, pointStructDecl(), pointStructDecl())
	if err := Check(f); err == nil {
		t.Fatal("expected an error for two structs with the same name")
	}
}

func TestCheck_DuplicateStructFieldNameIsAnError(t *testing.T) {
	f := mainFile(&ast.IntLit{Value: 0})
	f.Decls = append(f.Decls, &ast.StructDecl{
		Name: "Bad",
		Fields: []ast.Param{
			{Name: "x", Type: nt("Int")},
			{Name: "x", Type: nt("Int")},
		},
	})
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a struct with two fields named the same")
	}
}

func TestCheck_StructFieldWithUnknownTypeIsAnError(t *testing.T) {
	f := mainFile(&ast.IntLit{Value: 0})
	f.Decls = append(f.Decls, &ast.StructDecl{
		Name:   "Bad",
		Fields: []ast.Param{{Name: "x", Type: nt("Nope")}},
	})
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a struct field with an unknown type")
	}
}

func TestCheck_StructFieldsCanForwardReferenceAnotherStruct(t *testing.T) {
	// Line's `to` field names Point, declared *after* Line in Decls order —
	// registerStructName's pass runs for every struct before
	// registerStructFields resolves any of them, so this must not depend
	// on file order.
	f := mainFile(&ast.IntLit{Value: 0})
	f.Decls = append(f.Decls,
		&ast.StructDecl{Name: "Line", Fields: []ast.Param{{Name: "to", Type: nt("Point")}}},
		pointStructDecl(),
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_StructLitMissingFieldIsAnError(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "p", Value: &ast.StructLit{
			TypeName: "Point",
			Fields:   []ast.StructLitField{{Name: "x", Value: &ast.IntLit{Value: 1}}},
		}},
		&ast.IntLit{Value: 0},
	)
	f.Decls = append(f.Decls, pointStructDecl())
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a struct literal missing a field")
	}
}

func TestCheck_StructLitDuplicateFieldIsAnError(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "p", Value: &ast.StructLit{
			TypeName: "Point",
			Fields: []ast.StructLitField{
				{Name: "x", Value: &ast.IntLit{Value: 1}},
				{Name: "x", Value: &ast.IntLit{Value: 2}},
			},
		}},
		&ast.IntLit{Value: 0},
	)
	f.Decls = append(f.Decls, pointStructDecl())
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a struct literal repeating a field")
	}
}

func TestCheck_StructLitUnknownFieldIsAnError(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "p", Value: &ast.StructLit{
			TypeName: "Point",
			Fields: []ast.StructLitField{
				{Name: "x", Value: &ast.IntLit{Value: 1}},
				{Name: "z", Value: &ast.IntLit{Value: 2}},
			},
		}},
		&ast.IntLit{Value: 0},
	)
	f.Decls = append(f.Decls, pointStructDecl())
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a struct literal naming a field the struct doesn't have")
	}
}

func TestCheck_StructLitFieldValueTypeMismatchIsAnError(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "p", Value: &ast.StructLit{
			TypeName: "Point",
			Fields: []ast.StructLitField{
				{Name: "x", Value: &ast.StringLit{Value: "nope"}},
				{Name: "y", Value: &ast.IntLit{Value: 2}},
			},
		}},
		&ast.IntLit{Value: 0},
	)
	f.Decls = append(f.Decls, pointStructDecl())
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a struct field value of the wrong type")
	}
}

func TestCheck_UndefinedStructTypeIsAnError(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "p", Value: &ast.StructLit{TypeName: "Nope"}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a struct literal naming an undeclared struct type")
	}
}

func TestCheck_FieldAccessOnScalarIsAnError(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "x", Type: nt("Int"), Value: &ast.IntLit{Value: 1}},
		&ast.DiscardExpr{Value: &ast.FieldExpr{Target: &ast.IdentExpr{Name: "x"}, Field: "0"}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for field access on a scalar")
	}
}

func TestCheck_StructReservedNameIsAnError(t *testing.T) {
	f := mainFile(&ast.IntLit{Value: 0})
	f.Decls = append(f.Decls, &ast.StructDecl{Name: reservedMainName})
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a struct declared with the reserved amifl_main name")
	}
}

func tupleLit(elems ...ast.Expr) *ast.TupleLit {
	return &ast.TupleLit{Elems: elems}
}

func TestCheck_TupleLitAndFieldAccessAreValid(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "t", Value: tupleLit(&ast.IntLit{Value: 1}, &ast.StringLit{Value: "a"}, &ast.BoolLit{Value: true})},
		&ast.DiscardExpr{Value: &ast.FieldExpr{Target: &ast.IdentExpr{Name: "t"}, Field: "1"}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_TupleFieldAccessResolvesElementTypeAndSynthesizesFieldName(t *testing.T) {
	field := &ast.FieldExpr{Target: &ast.IdentExpr{Name: "t"}, Field: "1"}
	f := mainFile(
		&ast.LetExpr{Name: "t", Value: tupleLit(&ast.IntLit{Value: 1}, &ast.StringLit{Value: "a"})},
		&ast.DiscardExpr{Value: field},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if field.ResolvedType != "String" {
		t.Fatalf("got ResolvedType %q, want String", field.ResolvedType)
	}
	if field.AmivmField != "F1" {
		t.Fatalf("got AmivmField %q, want \"F1\" (synthesized tuple field name)", field.AmivmField)
	}
}

func TestCheck_TupleWithOneElementIsAnError(t *testing.T) {
	// The parser produces a 1-element TupleLit for `(x,)` (its own doc
	// comment) — sema is where Tuple2~Tuple8's actual range is enforced.
	f := mainFile(
		&ast.LetExpr{Name: "t", Value: tupleLit(&ast.IntLit{Value: 1})},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a 1-element tuple")
	}
}

func TestCheck_TupleWithNineElementsIsAnError(t *testing.T) {
	elems := make([]ast.Expr, 9)
	for i := range elems {
		elems[i] = &ast.IntLit{Value: uint64(i)}
	}
	f := mainFile(
		&ast.LetExpr{Name: "t", Value: tupleLit(elems...)},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a tuple with more than 8 elements")
	}
}

func TestCheck_TupleFieldIndexOutOfRangeIsAnError(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "t", Value: tupleLit(&ast.IntLit{Value: 1}, &ast.IntLit{Value: 2})},
		&ast.DiscardExpr{Value: &ast.FieldExpr{Target: &ast.IdentExpr{Name: "t"}, Field: "5"}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a tuple field index out of range")
	}
}

func TestCheck_NestedTupleElementIsAnError(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "t", Value: tupleLit(
			tupleLit(&ast.IntLit{Value: 1}, &ast.IntLit{Value: 2}),
			&ast.IntLit{Value: 3},
		)},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a tuple element that is itself a tuple")
	}
}

func TestCheck_FuncTypedTupleElementIsAnError(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "clos", Value: &ast.ClosureLit{
			ReturnType: nt("Int"),
			Body:       &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 0}}},
		}},
		&ast.LetExpr{Name: "t", Value: tupleLit(&ast.IdentExpr{Name: "clos"}, &ast.IntLit{Value: 1})},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a tuple element that is a function value")
	}
}

func TestCheck_StructEqualityIsValid(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "a", Value: pointLit(1, 2)},
		&ast.LetExpr{Name: "b", Value: pointLit(1, 2)},
		&ast.DiscardExpr{Value: &ast.BinaryExpr{Op: "==", Left: &ast.IdentExpr{Name: "a"}, Right: &ast.IdentExpr{Name: "b"}}},
		&ast.IntLit{Value: 0},
	)
	f.Decls = append(f.Decls, pointStructDecl())
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_TupleEqualityIsValid(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "a", Value: tupleLit(&ast.IntLit{Value: 1}, &ast.IntLit{Value: 2})},
		&ast.LetExpr{Name: "b", Value: tupleLit(&ast.IntLit{Value: 1}, &ast.IntLit{Value: 2})},
		&ast.DiscardExpr{Value: &ast.BinaryExpr{Op: "==", Left: &ast.IdentExpr{Name: "a"}, Right: &ast.IdentExpr{Name: "b"}}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_ConstStructLitIsValid(t *testing.T) {
	f := mainFile(
		&ast.DiscardExpr{Value: &ast.FieldExpr{Target: &ast.IdentExpr{Name: "Origin"}, Field: "x"}},
		&ast.IntLit{Value: 0},
	)
	f.Decls = append(f.Decls, pointStructDecl(), &ast.ConstDecl{Name: "Origin", Value: pointLit(0, 0)})
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_ConstStructLitReferencingNonConstIsAnError(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "n", Type: nt("Int"), Value: &ast.IntLit{Value: 1}},
		&ast.IntLit{Value: 0},
	)
	f.Decls = append(f.Decls, pointStructDecl(), &ast.ConstDecl{
		Name: "Bad",
		Value: &ast.StructLit{
			TypeName: "Point",
			Fields: []ast.StructLitField{
				{Name: "x", Value: &ast.IdentExpr{Name: "n"}},
				{Name: "y", Value: &ast.IntLit{Value: 0}},
			},
		},
	})
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a const struct literal referencing a non-const (a `let`)")
	}
}

func lt(elem ast.TypeExpr) ast.TypeExpr {
	return &ast.ListType{Elem: elem}
}

func at(elem ast.TypeExpr, size uint64) ast.TypeExpr {
	return &ast.ArrayType{Elem: elem, Size: &ast.IntLit{Value: size}}
}

// ft builds a Func type annotation (ast.FuncType, ex3) — `fn(params) -> ret`.
func ft(params []ast.TypeExpr, ret ast.TypeExpr) ast.TypeExpr {
	return &ast.FuncType{Params: params, Ret: ret}
}

// qt builds a cross-package type annotation (ast.QualifiedType, ex5) —
// `alias.name`.
func qt(alias, name string) ast.TypeExpr {
	return &ast.QualifiedType{Alias: alias, Name: name}
}

func intListLit(vals ...uint64) *ast.ListLit {
	elems := make([]ast.Expr, len(vals))
	for i, v := range vals {
		elems[i] = &ast.IntLit{Value: v}
	}
	return &ast.ListLit{Elems: elems}
}

func TestCheck_ListLitInferredTypeIsValid(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "xs", Value: intListLit(1, 2, 3)},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_EmptyListLitWithoutAnnotationIsAnError(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "xs", Value: &ast.ListLit{}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for an empty list literal with no type annotation")
	}
}

func TestCheck_EmptyListLitWithAnnotationIsValid(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "xs", Type: lt(nt("Int")), Value: &ast.ListLit{}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_ArrayLitFromListLiteralIsValid(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "xs", Type: at(nt("Int"), 3), Value: intListLit(1, 2, 3)},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_ArrayLitWrongElementCountIsAnError(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "xs", Type: at(nt("Int"), 3), Value: intListLit(1, 2)},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for an array literal with the wrong element count")
	}
}

func TestCheck_ArraySizeCanReferenceATopLevelConst(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "xs", Type: at(nt("Int"), 0 /* placeholder, overwritten below */), Value: intListLit(1, 2, 3)},
		&ast.IntLit{Value: 0},
	)
	// Swap the placeholder literal size for a reference to a const.
	f.Decls[0].(*ast.FuncDecl).Body.Exprs[0].(*ast.LetExpr).Type.(*ast.ArrayType).Size = &ast.IdentExpr{Name: "N"}
	f.Decls = append(f.Decls, &ast.ConstDecl{Name: "N", Value: &ast.IntLit{Value: 3}})
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_ArraySizeReferencingNonConstIsAnError(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "n", Type: nt("Int"), Value: &ast.IntLit{Value: 3}},
		&ast.LetExpr{Name: "xs", Type: at(nt("Int"), 0), Value: intListLit(1, 2, 3)},
		&ast.IntLit{Value: 0},
	)
	f.Decls[0].(*ast.FuncDecl).Body.Exprs[1].(*ast.LetExpr).Type.(*ast.ArrayType).Size = &ast.IdentExpr{Name: "n"}
	if err := Check(f); err == nil {
		t.Fatal("expected an error for an array size referencing a non-const `let`")
	}
}

func TestCheck_IndexExprResolvesElementType(t *testing.T) {
	idx := &ast.IndexExpr{Target: &ast.IdentExpr{Name: "xs"}, Index: &ast.IntLit{Value: 0}}
	f := mainFile(
		&ast.LetExpr{Name: "xs", Value: intListLit(1, 2, 3)},
		idx,
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if idx.ResolvedType != "Int64" {
		t.Fatalf("got ResolvedType %q, want Int64", idx.ResolvedType)
	}
}

func TestCheck_IndexOnScalarIsAnError(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "x", Type: nt("Int"), Value: &ast.IntLit{Value: 1}},
		&ast.DiscardExpr{Value: &ast.IndexExpr{Target: &ast.IdentExpr{Name: "x"}, Index: &ast.IntLit{Value: 0}}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for indexing into a scalar")
	}
}

func TestCheck_IndexAssignExprIsValid(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "xs", Value: intListLit(1, 2, 3)},
		&ast.IndexAssignExpr{Target: &ast.IdentExpr{Name: "xs"}, Index: &ast.IntLit{Value: 0}, Value: &ast.IntLit{Value: 9}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_IndexAssignChainedIndexIsValid(t *testing.T) {
	// matrix[0][0] = 9, matrix: List[List[Int]].
	f := mainFile(
		&ast.LetExpr{Name: "row", Value: intListLit(1, 2)},
		&ast.LetExpr{Name: "matrix", Value: &ast.ListLit{Elems: []ast.Expr{&ast.IdentExpr{Name: "row"}}}},
		&ast.IndexAssignExpr{
			Target: &ast.IndexExpr{Target: &ast.IdentExpr{Name: "matrix"}, Index: &ast.IntLit{Value: 0}},
			Index:  &ast.IntLit{Value: 0},
			Value:  &ast.IntLit{Value: 9},
		},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

// TestCheck_IndexAssignThroughFieldIsValid (ex10) supersedes the old
// "IndexAssignThroughFieldIsAnError" — resolveIndexAssignExpr's
// isAssignableTarget now accepts a FieldExpr layer too (shared with
// resolveFieldAssignExpr, see its own doc comment), so `p.xs[0] = 9` (a
// struct field holding a List) is valid.
func TestCheck_IndexAssignThroughFieldIsValid(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "p", Value: &ast.StructLit{TypeName: "Bag", Fields: []ast.StructLitField{
			{Name: "xs", Value: intListLit(1, 2, 3)},
		}}},
		&ast.IndexAssignExpr{
			Target: &ast.FieldExpr{Target: &ast.IdentExpr{Name: "p"}, Field: "xs"},
			Index:  &ast.IntLit{Value: 0},
			Value:  &ast.IntLit{Value: 9},
		},
		&ast.IntLit{Value: 0},
	)
	f.Decls = append(f.Decls, &ast.StructDecl{Name: "Bag", Fields: []ast.Param{{Name: "xs", Type: lt(nt("Int"))}}})
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_FieldAssignExprIsValid(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "p", Value: pointLit(1, 2)},
		&ast.FieldAssignExpr{Target: &ast.IdentExpr{Name: "p"}, Field: "x", Value: &ast.IntLit{Value: 9}},
		&ast.IntLit{Value: 0},
	)
	f.Decls = append(f.Decls, pointStructDecl())
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

// TestCheck_FieldAssignNestedChainIsValid (ex10) checks `line.to.x = 9` —
// a FieldAssignExpr whose own Target is itself a FieldExpr chain.
func TestCheck_FieldAssignNestedChainIsValid(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "line", Value: &ast.StructLit{TypeName: "Line", Fields: []ast.StructLitField{
			{Name: "to", Value: pointLit(1, 2)},
		}}},
		&ast.FieldAssignExpr{
			Target: &ast.FieldExpr{Target: &ast.IdentExpr{Name: "line"}, Field: "to"},
			Field:  "x",
			Value:  &ast.IntLit{Value: 9},
		},
		&ast.IntLit{Value: 0},
	)
	f.Decls = append(f.Decls,
		&ast.StructDecl{Name: "Line", Fields: []ast.Param{{Name: "to", Type: nt("Point")}}},
		pointStructDecl(),
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

// TestCheck_FieldAssignThroughIndexIsValid (ex10) checks `xs[0].x = 9` — a
// FieldAssignExpr whose own Target is an IndexExpr, the mirror image of
// TestCheck_IndexAssignThroughFieldIsValid above.
func TestCheck_FieldAssignThroughIndexIsValid(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "xs", Value: &ast.ListLit{Elems: []ast.Expr{pointLit(1, 2)}}},
		&ast.FieldAssignExpr{
			Target: &ast.IndexExpr{Target: &ast.IdentExpr{Name: "xs"}, Index: &ast.IntLit{Value: 0}},
			Field:  "x",
			Value:  &ast.IntLit{Value: 9},
		},
		&ast.IntLit{Value: 0},
	)
	f.Decls = append(f.Decls, pointStructDecl())
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_FieldAssignIntoTupleIsAnError(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "t", Value: &ast.TupleLit{Elems: []ast.Expr{&ast.IntLit{Value: 1}, &ast.IntLit{Value: 2}}}},
		&ast.FieldAssignExpr{Target: &ast.IdentExpr{Name: "t"}, Field: "0", Value: &ast.IntLit{Value: 9}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error assigning into a tuple's element (tuples are immutable)")
	}
}

// TestCheck_FieldAssignThroughCallIsAnError confirms a call result (not a
// plain identifier or an Index/Field chain over one) is still rejected as
// an assignment target base.
func TestCheck_FieldAssignThroughCallIsAnError(t *testing.T) {
	f := mainFile(
		&ast.FieldAssignExpr{
			Target: &ast.CallExpr{Callee: "makePoint", ResolvedType: "Point"},
			Field:  "x",
			Value:  &ast.IntLit{Value: 9},
		},
		&ast.IntLit{Value: 0},
	)
	f.Decls = append(f.Decls, pointStructDecl(), &ast.FuncDecl{
		Name:       "makePoint",
		ReturnType: nt("Point"),
		Body:       &ast.Block{Exprs: []ast.Expr{pointLit(1, 2)}},
	})
	if err := Check(f); err == nil {
		t.Fatal("expected an error for an assignment target that isn't a plain variable or an index/field chain over one")
	}
}

func TestCheck_IndexAssignIntoNonReassignableParamIsValid(t *testing.T) {
	// Element mutation through a non-reassignable (function parameter)
	// binding is allowed — it never rebinds the parameter variable itself.
	f := mainFile(
		&ast.DiscardExpr{Value: &ast.CallExpr{Callee: "setFirst", Args: []ast.Expr{intListLit(1, 2, 3)}}},
		&ast.IntLit{Value: 0},
	)
	f.Decls = append(f.Decls, &ast.FuncDecl{
		Name:       "setFirst",
		Params:     []ast.Param{{Name: "xs", Type: lt(nt("Int"))}},
		ReturnType: nt("Unit"),
		Body: &ast.Block{Exprs: []ast.Expr{
			&ast.IndexAssignExpr{Target: &ast.IdentExpr{Name: "xs"}, Index: &ast.IntLit{Value: 0}, Value: &ast.IntLit{Value: 9}},
		}},
	})
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_SliceExprAlwaysResolvesToList(t *testing.T) {
	sl := &ast.SliceExpr{Target: &ast.IdentExpr{Name: "arr"}, From: &ast.IntLit{Value: 0}, To: &ast.IntLit{Value: 2}}
	f := mainFile(
		&ast.LetExpr{Name: "arr", Type: at(nt("Int"), 3), Value: intListLit(1, 2, 3)},
		&ast.DiscardExpr{Value: sl},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if sl.ResolvedType != "List(Int64)" {
		t.Fatalf("got ResolvedType %q, want List(Int64) even though Target was an Array", sl.ResolvedType)
	}
}

func TestCheck_SliceExprOmittedBoundsAreValid(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "xs", Value: intListLit(1, 2, 3)},
		&ast.DiscardExpr{Value: &ast.SliceExpr{Target: &ast.IdentExpr{Name: "xs"}}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_ForExprIsValid(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "xs", Value: intListLit(1, 2, 3)},
		&ast.ForExpr{
			Var:   "x",
			Items: &ast.IdentExpr{Name: "xs"},
			Body:  &ast.Block{Exprs: []ast.Expr{&ast.DiscardExpr{Value: &ast.IdentExpr{Name: "x"}}}},
		},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_ForOverScalarIsAnError(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "x", Type: nt("Int"), Value: &ast.IntLit{Value: 1}},
		&ast.ForExpr{Var: "y", Items: &ast.IdentExpr{Name: "x"}, Body: &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 0}}}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for `for` iterating over a scalar")
	}
}

func TestCheck_RangeExprLetBindingIsValid(t *testing.T) {
	// ex2: `let r = 0..10` — Range is inferred, never annotated (no
	// surface type-annotation syntax exists for it, ast.RangeExpr's doc
	// comment).
	f := mainFile(
		&ast.LetExpr{Name: "r", Value: &ast.RangeExpr{From: &ast.IntLit{Value: 0}, To: &ast.IntLit{Value: 10}}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_RangeExprNonInt64BoundIsAnError(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "n", Type: nt("Int8"), Value: &ast.IntLit{Value: 5}},
		&ast.LetExpr{Name: "r", Value: &ast.RangeExpr{From: &ast.IntLit{Value: 0}, To: &ast.IdentExpr{Name: "n"}}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a Range bound that isn't Int64")
	}
}

func TestCheck_ForOverRangeBindsInt64ElemType(t *testing.T) {
	f := mainFile(
		&ast.ForExpr{
			Var:   "i",
			Items: &ast.RangeExpr{From: &ast.IntLit{Value: 0}, To: &ast.IntLit{Value: 10}},
			Body: &ast.Block{Exprs: []ast.Expr{
				&ast.LetExpr{Name: "doubled", Type: nt("Int"), Value: &ast.BinaryExpr{Op: "+", Left: &ast.IdentExpr{Name: "i"}, Right: &ast.IdentExpr{Name: "i"}}},
			}},
		},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_ForYieldOverRangeResolvesToListInt(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{
			Name: "xs",
			Value: &ast.ForExpr{
				Var:   "i",
				Items: &ast.RangeExpr{From: &ast.IntLit{Value: 0}, To: &ast.IntLit{Value: 10}},
				Yield: &ast.IdentExpr{Name: "i"},
			},
		},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	forExpr := f.Decls[0].(*ast.FuncDecl).Body.Exprs[0].(*ast.LetExpr).Value.(*ast.ForExpr)
	if forExpr.ResolvedType != "List(Int64)" {
		t.Errorf("got ResolvedType %q, want List(Int64)", forExpr.ResolvedType)
	}
}

func TestCheck_ForTwoVarsOverRangeIsAnError(t *testing.T) {
	f := mainFile(
		&ast.ForExpr{
			Var: "i", Var2: "j",
			Items: &ast.RangeExpr{From: &ast.IntLit{Value: 0}, To: &ast.IntLit{Value: 10}},
			Body:  &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 0}}},
		},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for `for i, j in 0..10`")
	}
}

func TestCheck_RangeIsNotAWritableTypeAnnotation(t *testing.T) {
	f := &ast.File{Decls: []ast.TopLevelDecl{
		&ast.FuncDecl{Name: "f", Params: []ast.Param{{Name: "r", Type: nt("Range")}}, ReturnType: nt("Int"), Body: &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 0}}}},
		&ast.FuncDecl{Name: "main", ReturnType: nt("Int"), Body: &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 0}}}},
	}}
	if err := Check(f); err == nil {
		t.Fatal("expected an error for `Range` used as a parameter type annotation")
	}
}

func TestCheck_ForVarIsNotReassignable(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "xs", Value: intListLit(1, 2, 3)},
		&ast.ForExpr{
			Var:   "x",
			Items: &ast.IdentExpr{Name: "xs"},
			Body:  &ast.Block{Exprs: []ast.Expr{&ast.AssignExpr{Name: "x", Value: &ast.IntLit{Value: 9}}}},
		},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error assigning to a for-loop variable")
	}
}

func TestCheck_ForVarScopedToBody(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "xs", Value: intListLit(1, 2, 3)},
		&ast.ForExpr{
			Var:   "x",
			Items: &ast.IdentExpr{Name: "xs"},
			Body:  &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 0}}},
		},
		&ast.DiscardExpr{Value: &ast.IdentExpr{Name: "x"}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error referencing the for-loop variable outside the loop body")
	}
}

func TestCheck_NestedListLitTypeInference(t *testing.T) {
	// [[1,2],[3,4]] with no annotation: List(List(Int64)).
	outer := &ast.ListLit{Elems: []ast.Expr{intListLit(1, 2), intListLit(3, 4)}}
	f := mainFile(
		&ast.LetExpr{Name: "m", Value: outer},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if outer.ResolvedType != "List(List(Int64))" {
		t.Fatalf("got ResolvedType %q, want List(List(Int64))", outer.ResolvedType)
	}
}

// statusEnumDecl builds `enum Status { Ok  Retry(delay: Int)
// Failed(reason: String) }` — step 8's test fixture, mirroring
// pointStructDecl's role for struct tests.
func statusEnumDecl() *ast.EnumDecl {
	return &ast.EnumDecl{
		Name: "Status",
		Variants: []ast.EnumVariant{
			{Name: "Ok"},
			{Name: "Retry", Fields: []ast.Param{{Name: "delay", Type: nt("Int")}}},
			{Name: "Failed", Fields: []ast.Param{{Name: "reason", Type: nt("String")}}},
		},
	}
}

// statusVariant builds `Status.<variant>(args...)` (nil args for a
// zero-field variant reference with no parens at all).
func statusVariant(variant string, args ...ast.StructLitField) *ast.FieldExpr {
	return &ast.FieldExpr{Target: &ast.IdentExpr{Name: "Status"}, Field: variant, Args: args}
}

func statusCase(variant string, bindings []string, body ast.Expr) ast.SwitchCase {
	return ast.SwitchCase{EnumName: "Status", Variant: variant, Bindings: bindings, Body: &ast.Block{Exprs: []ast.Expr{body}}}
}

func TestCheck_EnumVariantZeroFieldConstructionIsValid(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "s", Value: statusVariant("Ok")},
		&ast.IntLit{Value: 0},
	)
	f.Decls = append(f.Decls, statusEnumDecl())
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_EnumVariantConstructionWithFieldsIsValid(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "s", Value: statusVariant("Retry", ast.StructLitField{Name: "delay", Value: &ast.IntLit{Value: 5}})},
		&ast.IntLit{Value: 0},
	)
	f.Decls = append(f.Decls, statusEnumDecl())
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_EnumVariantConstructionMissingFieldIsAnError(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "s", Value: statusVariant("Retry")},
		&ast.IntLit{Value: 0},
	)
	f.Decls = append(f.Decls, statusEnumDecl())
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a variant construction missing a required field")
	}
}

func TestCheck_EnumVariantConstructionUnknownVariantIsAnError(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "s", Value: statusVariant("Bogus")},
		&ast.IntLit{Value: 0},
	)
	f.Decls = append(f.Decls, statusEnumDecl())
	if err := Check(f); err == nil {
		t.Fatal("expected an error for an unknown enum variant")
	}
}

func TestCheck_EnumVariantConstructionWrongFieldTypeIsAnError(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "s", Value: statusVariant("Retry", ast.StructLitField{Name: "delay", Value: &ast.StringLit{Value: "nope"}})},
		&ast.IntLit{Value: 0},
	)
	f.Decls = append(f.Decls, statusEnumDecl())
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a field value of the wrong type")
	}
}

func TestCheck_EnumVariantConstructionUnknownFieldIsAnError(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "s", Value: statusVariant("Retry", ast.StructLitField{Name: "bogus", Value: &ast.IntLit{Value: 5}})},
		&ast.IntLit{Value: 0},
	)
	f.Decls = append(f.Decls, statusEnumDecl())
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a field name the variant doesn't declare")
	}
}

func TestCheck_EnumEqualityIsAnError(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "a", Value: statusVariant("Ok")},
		&ast.LetExpr{Name: "b", Value: statusVariant("Ok")},
		&ast.DiscardExpr{Value: &ast.BinaryExpr{Op: "==", Left: &ast.IdentExpr{Name: "a"}, Right: &ast.IdentExpr{Name: "b"}}},
		&ast.IntLit{Value: 0},
	)
	f.Decls = append(f.Decls, statusEnumDecl())
	if err := Check(f); err == nil {
		t.Fatal("expected an error comparing two enum values with ==")
	}
}

func TestCheck_ListEqualityIsAnError(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "a", Type: lt(nt("Int")), Value: intListLit(1, 2)},
		&ast.LetExpr{Name: "b", Type: lt(nt("Int")), Value: intListLit(1, 2)},
		&ast.DiscardExpr{Value: &ast.BinaryExpr{Op: "==", Left: &ast.IdentExpr{Name: "a"}, Right: &ast.IdentExpr{Name: "b"}}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error comparing two List values with ==")
	}
}

func TestCheck_MapEqualityIsAnError(t *testing.T) {
	m := func() *ast.SetOrMapLit {
		return &ast.SetOrMapLit{Entries: []ast.MapLitEntry{{Key: &ast.StringLit{Value: "a"}, Value: &ast.IntLit{Value: 1}}}}
	}
	f := mainFile(
		&ast.LetExpr{Name: "a", Type: mapt(nt("String"), nt("Int")), Value: m()},
		&ast.LetExpr{Name: "b", Type: mapt(nt("String"), nt("Int")), Value: m()},
		&ast.DiscardExpr{Value: &ast.BinaryExpr{Op: "!=", Left: &ast.IdentExpr{Name: "a"}, Right: &ast.IdentExpr{Name: "b"}}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error comparing two Map values with !=")
	}
}

func TestCheck_SetEqualityIsAnError(t *testing.T) {
	s := func() *ast.SetOrMapLit { return &ast.SetOrMapLit{Elems: []ast.Expr{&ast.IntLit{Value: 1}}} }
	f := mainFile(
		&ast.LetExpr{Name: "a", Type: sett(nt("Int")), Value: s()},
		&ast.LetExpr{Name: "b", Type: sett(nt("Int")), Value: s()},
		&ast.DiscardExpr{Value: &ast.BinaryExpr{Op: "==", Left: &ast.IdentExpr{Name: "a"}, Right: &ast.IdentExpr{Name: "b"}}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error comparing two Set values with ==")
	}
}

func TestCheck_ArrayEqualityIsValid(t *testing.T) {
	arr := func() *ast.ListLit {
		return &ast.ListLit{Elems: []ast.Expr{&ast.IntLit{Value: 1}, &ast.IntLit{Value: 2}}}
	}
	f := mainFile(
		&ast.LetExpr{Name: "a", Type: at(nt("Int"), 2), Value: arr()},
		&ast.LetExpr{Name: "b", Type: at(nt("Int"), 2), Value: arr()},
		&ast.DiscardExpr{Value: &ast.BinaryExpr{Op: "==", Left: &ast.IdentExpr{Name: "a"}, Right: &ast.IdentExpr{Name: "b"}}},
		&ast.IntLit{Value: 0},
	)
	// Array (a Go native fixed-size array) compares element-wise and stays
	// unaffected by the List/Map/Set == rejection above — verified against
	// the real amivm/go build pipeline during this review (amifl-spec.md
	// section 2.3).
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_EnumStructNameCollisionIsAnError(t *testing.T) {
	f := mainFile(&ast.IntLit{Value: 0})
	f.Decls = append(f.Decls, statusEnumDecl(), &ast.StructDecl{Name: "Status"})
	if err := Check(f); err == nil {
		t.Fatal("expected an error for an enum and a struct sharing one name")
	}
}

func TestCheck_SwitchExhaustiveWithoutDefaultIsValid(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "s", Value: statusVariant("Ok")},
		&ast.LetExpr{Name: "n", Value: &ast.SwitchExpr{
			Subject: &ast.IdentExpr{Name: "s"},
			Cases: []ast.SwitchCase{
				statusCase("Ok", nil, &ast.IntLit{Value: 1}),
				statusCase("Retry", []string{"delay"}, &ast.IdentExpr{Name: "delay"}),
				statusCase("Failed", []string{"reason"}, &ast.IntLit{Value: 2}),
			},
		}},
		&ast.IntLit{Value: 0},
	)
	f.Decls = append(f.Decls, statusEnumDecl())
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_SwitchNonExhaustiveWithoutDefaultIsAnError(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "s", Value: statusVariant("Ok")},
		&ast.LetExpr{Name: "n", Value: &ast.SwitchExpr{
			Subject: &ast.IdentExpr{Name: "s"},
			Cases: []ast.SwitchCase{
				statusCase("Ok", nil, &ast.IntLit{Value: 1}),
			},
		}},
		&ast.IntLit{Value: 0},
	)
	f.Decls = append(f.Decls, statusEnumDecl())
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a non-exhaustive switch with no default")
	}
}

func TestCheck_SwitchWithDefaultIsValid(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "s", Value: statusVariant("Ok")},
		&ast.LetExpr{Name: "n", Value: &ast.SwitchExpr{
			Subject: &ast.IdentExpr{Name: "s"},
			Cases: []ast.SwitchCase{
				statusCase("Retry", []string{"delay"}, &ast.IdentExpr{Name: "delay"}),
			},
			Default: &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 0}}},
		}},
		&ast.IntLit{Value: 0},
	)
	f.Decls = append(f.Decls, statusEnumDecl())
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_SwitchBindingNameMustMatchFieldNameIsAnError(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "s", Value: statusVariant("Retry", ast.StructLitField{Name: "delay", Value: &ast.IntLit{Value: 5}})},
		&ast.LetExpr{Name: "n", Value: &ast.SwitchExpr{
			Subject: &ast.IdentExpr{Name: "s"},
			Cases: []ast.SwitchCase{
				// "d" doesn't match the variant's declared field name "delay".
				statusCase("Retry", []string{"d"}, &ast.IdentExpr{Name: "d"}),
			},
			Default: &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 0}}},
		}},
		&ast.IntLit{Value: 0},
	)
	f.Decls = append(f.Decls, statusEnumDecl())
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a binding name that doesn't match its field's declared name")
	}
}

func TestCheck_SwitchDuplicateCaseIsAnError(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "s", Value: statusVariant("Ok")},
		&ast.LetExpr{Name: "n", Value: &ast.SwitchExpr{
			Subject: &ast.IdentExpr{Name: "s"},
			Cases: []ast.SwitchCase{
				statusCase("Ok", nil, &ast.IntLit{Value: 1}),
				statusCase("Ok", nil, &ast.IntLit{Value: 2}),
			},
			Default: &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 0}}},
		}},
		&ast.IntLit{Value: 0},
	)
	f.Decls = append(f.Decls, statusEnumDecl())
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a duplicate case matching the same variant twice")
	}
}

func TestCheck_SwitchSubjectMustBeEnumIsAnError(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "n", Type: nt("Int"), Value: &ast.IntLit{Value: 1}},
		&ast.DiscardExpr{Value: &ast.SwitchExpr{
			Subject: &ast.IdentExpr{Name: "n"},
			Cases: []ast.SwitchCase{
				statusCase("Ok", nil, &ast.CallExpr{Callee: "print", Args: []ast.Expr{&ast.StringLit{Value: "x"}}}),
			},
		}},
		&ast.IntLit{Value: 0},
	)
	f.Decls = append(f.Decls, statusEnumDecl())
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a switch subject that isn't an enum type")
	}
}

func TestCheck_SwitchBindingScopedToCaseBody(t *testing.T) {
	// `delay` bound inside the Retry case must not be visible after the
	// switch expression ends.
	f := mainFile(
		&ast.LetExpr{Name: "s", Value: statusVariant("Retry", ast.StructLitField{Name: "delay", Value: &ast.IntLit{Value: 5}})},
		&ast.DiscardExpr{Value: &ast.SwitchExpr{
			Subject: &ast.IdentExpr{Name: "s"},
			Cases: []ast.SwitchCase{
				statusCase("Ok", nil, &ast.CallExpr{Callee: "print", Args: []ast.Expr{&ast.StringLit{Value: "x"}}}),
				statusCase("Retry", []string{"delay"}, &ast.CallExpr{Callee: "print", Args: []ast.Expr{&ast.StringLit{Value: "y"}}}),
				statusCase("Failed", []string{"reason"}, &ast.CallExpr{Callee: "print", Args: []ast.Expr{&ast.StringLit{Value: "z"}}}),
			},
		}},
		&ast.DiscardExpr{Value: &ast.IdentExpr{Name: "delay"}},
		&ast.IntLit{Value: 0},
	)
	f.Decls = append(f.Decls, statusEnumDecl())
	if err := Check(f); err == nil {
		t.Fatal("expected an error referencing a case binding outside its own case body")
	}
}

func TestCheck_ForYieldResolvesToListOfYieldType(t *testing.T) {
	forExpr := &ast.ForExpr{
		Var:   "x",
		Items: &ast.IdentExpr{Name: "xs"},
		Yield: &ast.BinaryExpr{Op: "*", Left: &ast.IdentExpr{Name: "x"}, Right: &ast.IntLit{Value: 2}},
	}
	f := mainFile(
		&ast.LetExpr{Name: "xs", Value: intListLit(1, 2, 3)},
		&ast.LetExpr{Name: "ys", Value: forExpr},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if forExpr.ResolvedType != "List(Int64)" {
		t.Fatalf("got ResolvedType %q, want List(Int64)", forExpr.ResolvedType)
	}
}

func TestCheck_ForYieldAdaptsToExpectedListElemType(t *testing.T) {
	// Yield's own bare literal (Int8-incompatible-by-default Int64) must
	// adapt to the `let`'s List[Int8] annotation, the same expected-type
	// threading resolveListLit already gets for a plain list literal.
	forExpr := &ast.ForExpr{
		Var:   "x",
		Items: &ast.IdentExpr{Name: "xs"},
		Yield: &ast.IntLit{Value: 5},
	}
	f := mainFile(
		&ast.LetExpr{Name: "xs", Value: intListLit(1, 2, 3)},
		&ast.LetExpr{Name: "ys", Type: lt(nt("Int8")), Value: forExpr},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if forExpr.ResolvedType != "List(Int8)" {
		t.Fatalf("got ResolvedType %q, want List(Int8)", forExpr.ResolvedType)
	}
}

func TestCheck_ForYieldBreakIsAnError(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "xs", Value: intListLit(1, 2, 3)},
		&ast.LetExpr{Name: "ys", Value: &ast.ForExpr{
			Var:   "x",
			Items: &ast.IdentExpr{Name: "xs"},
			Yield: &ast.BreakExpr{},
		}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for `break` used inside a `yield` expression")
	}
}

func TestCheck_ForYieldBreakInsideOuterLoopIsStillAnError(t *testing.T) {
	// break/continue must be rejected inside `yield` even when the
	// yield-for is lexically nested inside an unrelated enclosing loop —
	// loopDepth is suppressed (not just absent), mirroring a closure body.
	f := mainFile(
		&ast.LetExpr{Name: "xs", Value: intListLit(1, 2, 3)},
		&ast.WhileExpr{
			Cond: &ast.BoolLit{Value: true},
			Body: &ast.Block{Exprs: []ast.Expr{
				&ast.DiscardExpr{Value: &ast.ForExpr{
					Var:   "x",
					Items: &ast.IdentExpr{Name: "xs"},
					Yield: &ast.BreakExpr{},
				}},
				&ast.BreakExpr{},
			}},
		},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for `break` inside a `yield` expression nested in an outer while loop")
	}
}

func TestCheck_ForYieldUnitTypedIsAnError(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "xs", Value: intListLit(1, 2, 3)},
		&ast.DiscardExpr{Value: &ast.ForExpr{
			Var:   "x",
			Items: &ast.IdentExpr{Name: "xs"},
			Yield: printStr("side effect"),
		}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a Unit-typed `yield` value")
	}
}

func TestCheck_ForYieldVarNotVisibleOutsideYield(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "xs", Value: intListLit(1, 2, 3)},
		&ast.DiscardExpr{Value: &ast.ForExpr{
			Var:   "x",
			Items: &ast.IdentExpr{Name: "xs"},
			Yield: &ast.IdentExpr{Name: "x"},
		}},
		&ast.DiscardExpr{Value: &ast.IdentExpr{Name: "x"}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error referencing the for-yield variable outside its own scope")
	}
}

// sett/mapt build Set[T]/Map[K,V] type annotations, mirroring lt/at above.
func sett(elem ast.TypeExpr) ast.TypeExpr {
	return &ast.SetType{Elem: elem}
}

func mapt(key, val ast.TypeExpr) ast.TypeExpr {
	return &ast.MapType{Key: key, Value: val}
}

func intSetLit(vals ...uint64) *ast.SetOrMapLit {
	elems := make([]ast.Expr, len(vals))
	for i, v := range vals {
		elems[i] = &ast.IntLit{Value: v}
	}
	return &ast.SetOrMapLit{Elems: elems}
}

func TestCheck_SetLitInferredTypeIsValid(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "s", Value: intSetLit(1, 2, 3)},
		&ast.IntLit{Value: 0},
	)
	let := f.Decls[0].(*ast.FuncDecl).Body.Exprs[0].(*ast.LetExpr)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if let.ResolvedType != "Set(Int64)" {
		t.Fatalf("got ResolvedType %q, want Set(Int64)", let.ResolvedType)
	}
}

func TestCheck_SetLitNonComparableElemIsAnError(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "s", Value: &ast.SetOrMapLit{Elems: []ast.Expr{
			&ast.StructLit{TypeName: "Point", Fields: []ast.StructLitField{
				{Name: "x", Value: &ast.IntLit{Value: 1}},
				{Name: "y", Value: &ast.IntLit{Value: 2}},
			}},
		}}},
		&ast.IntLit{Value: 0},
	)
	f.Decls = append([]ast.TopLevelDecl{pointStructDecl()}, f.Decls...)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a Set[Point] (struct isn't a comparable key type)")
	}
}

func TestCheck_SetTypeAnnotationRejectsNonComparableElem(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "s", Type: sett(nt("Point")), Value: &ast.SetOrMapLit{}},
		&ast.IntLit{Value: 0},
	)
	f.Decls = append([]ast.TopLevelDecl{pointStructDecl()}, f.Decls...)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a Set[Point] type annotation")
	}
}

func TestCheck_MapLitInferredTypeIsValid(t *testing.T) {
	m := &ast.SetOrMapLit{Entries: []ast.MapLitEntry{
		{Key: &ast.StringLit{Value: "a"}, Value: &ast.IntLit{Value: 1}},
		{Key: &ast.StringLit{Value: "b"}, Value: &ast.IntLit{Value: 2}},
	}}
	f := mainFile(
		&ast.LetExpr{Name: "m", Value: m},
		&ast.IntLit{Value: 0},
	)
	let := f.Decls[0].(*ast.FuncDecl).Body.Exprs[0].(*ast.LetExpr)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if let.ResolvedType != "Map(String,Int64)" {
		t.Fatalf("got ResolvedType %q, want Map(String,Int64)", let.ResolvedType)
	}
}

func TestCheck_MapLitNonComparableKeyIsAnError(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "m", Type: mapt(nt("Point"), nt("Int")), Value: &ast.SetOrMapLit{}},
		&ast.IntLit{Value: 0},
	)
	f.Decls = append([]ast.TopLevelDecl{pointStructDecl()}, f.Decls...)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a Map[Point,Int] type annotation")
	}
}

func TestCheck_MapLitUnitValueIsAnError(t *testing.T) {
	m := &ast.SetOrMapLit{Entries: []ast.MapLitEntry{
		{Key: &ast.StringLit{Value: "a"}, Value: printStr("side effect")},
	}}
	f := mainFile(
		&ast.LetExpr{Name: "m", Value: m},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a Map value resolving to Unit")
	}
}

func TestCheck_EmptyBraceLitWithoutAnnotationIsAnError(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "x", Value: &ast.SetOrMapLit{}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for an un-annotated empty {}")
	}
}

func TestCheck_EmptyBraceLitAdaptsToSetAnnotation(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "s", Type: sett(nt("Int")), Value: &ast.SetOrMapLit{}},
		&ast.IntLit{Value: 0},
	)
	let := f.Decls[0].(*ast.FuncDecl).Body.Exprs[0].(*ast.LetExpr)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if let.ResolvedType != "Set(Int64)" {
		t.Fatalf("got ResolvedType %q, want Set(Int64)", let.ResolvedType)
	}
}

func TestCheck_EmptyBraceLitAdaptsToMapAnnotation(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "m", Type: mapt(nt("String"), nt("Int")), Value: &ast.SetOrMapLit{}},
		&ast.IntLit{Value: 0},
	)
	let := f.Decls[0].(*ast.FuncDecl).Body.Exprs[0].(*ast.LetExpr)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if let.ResolvedType != "Map(String,Int64)" {
		t.Fatalf("got ResolvedType %q, want Map(String,Int64)", let.ResolvedType)
	}
}

func TestCheck_ForOverSetIsValid(t *testing.T) {
	forExpr := &ast.ForExpr{
		Var:   "x",
		Items: &ast.IdentExpr{Name: "s"},
		Body:  &ast.Block{Exprs: []ast.Expr{&ast.DiscardExpr{Value: &ast.IdentExpr{Name: "x"}}}},
	}
	f := mainFile(
		&ast.LetExpr{Name: "s", Value: intSetLit(1, 2, 3)},
		forExpr,
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if forExpr.ElemType != "Int64" {
		t.Fatalf("got ElemType %q, want Int64", forExpr.ElemType)
	}
}

func TestCheck_ForTwoVarsOverMapIsValid(t *testing.T) {
	m := &ast.SetOrMapLit{Entries: []ast.MapLitEntry{
		{Key: &ast.StringLit{Value: "a"}, Value: &ast.IntLit{Value: 1}},
	}}
	forExpr := &ast.ForExpr{
		Var:   "k",
		Var2:  "v",
		Items: &ast.IdentExpr{Name: "m"},
		Body: &ast.Block{Exprs: []ast.Expr{
			&ast.DiscardExpr{Value: &ast.IdentExpr{Name: "k"}},
			&ast.DiscardExpr{Value: &ast.IdentExpr{Name: "v"}},
		}},
	}
	f := mainFile(
		&ast.LetExpr{Name: "m", Value: m},
		forExpr,
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if forExpr.ElemType != "String" {
		t.Fatalf("got ElemType (key) %q, want String", forExpr.ElemType)
	}
	if forExpr.Var2Type != "Int64" {
		t.Fatalf("got Var2Type (value) %q, want Int64", forExpr.Var2Type)
	}
}

func TestCheck_ForTwoVarsOverNonMapIsAnError(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "xs", Value: intListLit(1, 2, 3)},
		&ast.ForExpr{
			Var:   "k",
			Var2:  "v",
			Items: &ast.IdentExpr{Name: "xs"},
			Body:  &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 0}}},
		},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for `for k, v in xs` where xs is a List, not a Map")
	}
}

func TestCheck_ForSingleVarOverMapIsAnError(t *testing.T) {
	m := &ast.SetOrMapLit{Entries: []ast.MapLitEntry{
		{Key: &ast.StringLit{Value: "a"}, Value: &ast.IntLit{Value: 1}},
	}}
	f := mainFile(
		&ast.LetExpr{Name: "m", Value: m},
		&ast.ForExpr{
			Var:   "k",
			Items: &ast.IdentExpr{Name: "m"},
			Body:  &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 0}}},
		},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a single-variable `for` over a Map (needs `for k, v in m`)")
	}
}

// tuple2t builds a Tuple2[a,b] type annotation (ast.TupleType) — step 11's
// counterpart to sett/mapt above.
func tuple2t(a, b ast.TypeExpr) ast.TypeExpr {
	return &ast.TupleType{Elems: []ast.TypeExpr{a, b}}
}

func TestCheck_TupleTypeAnnotationResolves(t *testing.T) {
	f := mainFile(&ast.IntLit{Value: 0})
	f.Decls = append(f.Decls, &ast.FuncDecl{
		Name:       "f",
		ReturnType: tuple2t(nt("Int"), nt("Error")),
		Body: &ast.Block{Exprs: []ast.Expr{
			&ast.TupleLit{Elems: []ast.Expr{&ast.IntLit{Value: 1}, &ast.IntLit{Value: 2}}},
		}},
	})
	fn := f.Decls[1].(*ast.FuncDecl)
	// Body deliberately returns a bogus Tuple2[Int,Int] (not Tuple2[Int,
	// Error]) so this test only exercises ReturnType resolution — a
	// mismatched body should fail Check, proving ResolvedReturnType really
	// was compared against.
	if err := Check(f); err == nil {
		t.Fatal("expected an error: body returns Tuple2[Int,Int], not the declared Tuple2[Int,Error]")
	}
	if fn.ResolvedReturnType != "Tuple(Int64,Error)" {
		t.Fatalf("got ResolvedReturnType %q, want Tuple(Int64,Error)", fn.ResolvedReturnType)
	}
}

func TestCheck_IsErrorRequiresErrorArg(t *testing.T) {
	f := mainFile(
		&ast.DiscardExpr{Value: &ast.CallExpr{Callee: "isError", Args: []ast.Expr{&ast.IntLit{Value: 1}}}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for isError(Int) — Int isn't Error")
	}
}

func TestCheck_CastResolvesToTypeArg(t *testing.T) {
	call := &ast.CallExpr{Callee: "cast", TypeArg: nt("Float64"), Args: []ast.Expr{&ast.IntLit{Value: 7}}}
	f := mainFile(&ast.LetExpr{Name: "x", Value: call}, &ast.IntLit{Value: 0})
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if call.ResolvedType != "Float64" {
		t.Fatalf("got ResolvedType %q, want Float64", call.ResolvedType)
	}
}

func TestCheck_CastRejectsNonNumericTypeArg(t *testing.T) {
	call := &ast.CallExpr{Callee: "cast", TypeArg: nt("String"), Args: []ast.Expr{&ast.IntLit{Value: 7}}}
	f := mainFile(&ast.DiscardExpr{Value: call}, &ast.IntLit{Value: 0})
	if err := Check(f); err == nil {
		t.Fatal("expected an error for cast[String](...) — String isn't Numeric")
	}
}

func TestCheck_CastWithoutTypeArgIsAnError(t *testing.T) {
	call := &ast.CallExpr{Callee: "cast", Args: []ast.Expr{&ast.IntLit{Value: 7}}}
	f := mainFile(&ast.DiscardExpr{Value: call}, &ast.IntLit{Value: 0})
	if err := Check(f); err == nil {
		t.Fatal("expected an error for cast(...) with no bracketed type argument")
	}
}

func TestCheck_ParseReturnsTuple2OfTargetAndError(t *testing.T) {
	call := &ast.CallExpr{Callee: "parse", TypeArg: nt("Int"), Args: []ast.Expr{&ast.StringLit{Value: "42"}}}
	f := mainFile(&ast.LetExpr{Name: "r", Value: call}, &ast.IntLit{Value: 0})
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if call.ResolvedType != "Tuple(Int64,Error)" {
		t.Fatalf("got ResolvedType %q, want Tuple(Int64,Error)", call.ResolvedType)
	}
}

func TestCheck_ParseRejectsNonNumericNonBoolTypeArg(t *testing.T) {
	call := &ast.CallExpr{Callee: "parse", TypeArg: nt("String"), Args: []ast.Expr{&ast.StringLit{Value: "x"}}}
	f := mainFile(&ast.DiscardExpr{Value: call}, &ast.IntLit{Value: 0})
	if err := Check(f); err == nil {
		t.Fatal("expected an error for parse[String](...) — String is neither Numeric nor Bool")
	}
}

func TestCheck_TryOperatorUnwrapsTuple2Payload(t *testing.T) {
	tryExpr := &ast.TryExpr{Value: &ast.CallExpr{Callee: "parse", TypeArg: nt("Int"), Args: []ast.Expr{&ast.IdentExpr{Name: "s"}}}}
	f := &ast.File{Decls: []ast.TopLevelDecl{
		&ast.FuncDecl{
			Name:       "f",
			Params:     []ast.Param{{Name: "s", Type: nt("String")}},
			ReturnType: tuple2t(nt("Int"), nt("Error")),
			Body: &ast.Block{Exprs: []ast.Expr{
				&ast.LetExpr{Name: "x", Value: tryExpr},
				&ast.TupleLit{Elems: []ast.Expr{&ast.IdentExpr{Name: "x"}, &ast.IdentExpr{Name: "x"}}},
			}},
		},
		&ast.FuncDecl{Name: "main", ReturnType: nt("Int"), Body: &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 0}}}},
	}}
	// The tail TupleLit deliberately returns (x, x) (Tuple2[Int,Int]), not
	// Tuple2[Int,Error] — Check should still fail on *that* mismatch, but
	// resolveTryExpr's own work (unwrapping the payload type) must already
	// have completed without error before that final check runs.
	if err := Check(f); err == nil {
		t.Fatal("expected an error: body's tail returns Tuple2[Int,Int], not the declared Tuple2[Int,Error]")
	}
	if tryExpr.ElemType != "Int64" {
		t.Fatalf("got ElemType %q, want Int64", tryExpr.ElemType)
	}
	if tryExpr.IsBareError {
		t.Fatal("got IsBareError true, want false for a Tuple2[Int,Error] operand")
	}
}

func TestCheck_TryOperatorOutsideFallibleFuncIsAnError(t *testing.T) {
	tryExpr := &ast.TryExpr{Value: &ast.CallExpr{Callee: "parse", TypeArg: nt("Int"), Args: []ast.Expr{&ast.StringLit{Value: "1"}}}}
	f := mainFile(&ast.DiscardExpr{Value: tryExpr}, &ast.IntLit{Value: 0})
	if err := Check(f); err == nil {
		t.Fatal("expected an error: `?` used inside `main`, which returns plain Int, not Tuple2[_,Error]/Error")
	}
}

func TestCheck_TryOperatorOnNonTuple2NonErrorIsAnError(t *testing.T) {
	tryExpr := &ast.TryExpr{Value: &ast.IntLit{Value: 1}}
	f := mainFile(&ast.DiscardExpr{Value: tryExpr}, &ast.IntLit{Value: 0})
	if err := Check(f); err == nil {
		t.Fatal("expected an error: `?` on a plain Int isn't Tuple2[_,Error] or Error")
	}
}

// Phase 11b (amifl-spec.md section 13.4) — capability dispatch tests.

func TestCheck_LenAcceptsEveryLenableType(t *testing.T) {
	for _, tc := range []struct {
		name string
		let  *ast.LetExpr
	}{
		{"String", &ast.LetExpr{Name: "s", Value: &ast.StringLit{Value: "hi"}}},
		{"List", &ast.LetExpr{Name: "s", Value: intListLit(1, 2)}},
		{"Set", &ast.LetExpr{Name: "s", Value: intSetLit(1, 2)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			call := &ast.CallExpr{Callee: "len", Args: []ast.Expr{&ast.IdentExpr{Name: "s"}}}
			f := mainFile(tc.let, &ast.DiscardExpr{Value: call}, &ast.IntLit{Value: 0})
			if err := Check(f); err != nil {
				t.Fatalf("Check() error: %v", err)
			}
			if call.ResolvedType != "Int64" {
				t.Fatalf("got ResolvedType %q, want Int64", call.ResolvedType)
			}
			if call.Builtin != "len" {
				t.Fatalf("got Builtin %q, want \"len\"", call.Builtin)
			}
		})
	}
}

func TestCheck_LenRejectsUnsupportedType(t *testing.T) {
	call := &ast.CallExpr{Callee: "len", Args: []ast.Expr{&ast.IntLit{Value: 1}}}
	f := mainFile(&ast.DiscardExpr{Value: call}, &ast.IntLit{Value: 0})
	if err := Check(f); err == nil {
		t.Fatal("expected an error for len(Int) — Int isn't Lenable")
	}
}

func TestCheck_ContainsDispatchesPerCapability(t *testing.T) {
	m := &ast.SetOrMapLit{Entries: []ast.MapLitEntry{{Key: &ast.StringLit{Value: "a"}, Value: &ast.IntLit{Value: 1}}}}
	f := mainFile(
		&ast.LetExpr{Name: "m", Value: m},
		&ast.DiscardExpr{Value: &ast.CallExpr{Callee: "contains", Args: []ast.Expr{&ast.IdentExpr{Name: "m"}, &ast.StringLit{Value: "a"}}}},
		&ast.DiscardExpr{Value: &ast.CallExpr{Callee: "contains", Args: []ast.Expr{&ast.StringLit{Value: "hello"}, &ast.StringLit{Value: "ell"}}}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_ContainsOnMapRequiresKeyTypeMatch(t *testing.T) {
	m := &ast.SetOrMapLit{Entries: []ast.MapLitEntry{{Key: &ast.StringLit{Value: "a"}, Value: &ast.IntLit{Value: 1}}}}
	f := mainFile(
		&ast.LetExpr{Name: "m", Value: m},
		&ast.DiscardExpr{Value: &ast.CallExpr{Callee: "contains", Args: []ast.Expr{&ast.IdentExpr{Name: "m"}, &ast.IntLit{Value: 1}}}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error: Map[String,Int]'s key is String, not Int")
	}
}

func TestCheck_MapBuiltinRequiresClosureMatchingElemType(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "xs", Value: intListLit(1, 2, 3)},
		&ast.LetExpr{Name: "f", Value: &ast.ClosureLit{
			Params:     []ast.Param{{Name: "x", Type: nt("String")}},
			ReturnType: nt("Int"),
			Body:       &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 0}}},
		}},
		&ast.DiscardExpr{Value: &ast.CallExpr{Callee: "map", Args: []ast.Expr{&ast.IdentExpr{Name: "xs"}, &ast.IdentExpr{Name: "f"}}}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error: f takes String but xs's elements are Int")
	}
}

func TestCheck_MapBuiltinResolvesToListOfClosureReturnType(t *testing.T) {
	call := &ast.CallExpr{Callee: "map", Args: []ast.Expr{&ast.IdentExpr{Name: "xs"}, &ast.IdentExpr{Name: "f"}}}
	f := mainFile(
		&ast.LetExpr{Name: "xs", Value: intListLit(1, 2, 3)},
		&ast.LetExpr{Name: "f", Value: &ast.ClosureLit{
			Params:     []ast.Param{{Name: "x", Type: nt("Int")}},
			ReturnType: nt("Bool"),
			Body:       &ast.Block{Exprs: []ast.Expr{&ast.BoolLit{Value: true}}},
		}},
		&ast.LetExpr{Name: "r", Value: call},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if call.ResolvedType != "List(Bool)" {
		t.Fatalf("got ResolvedType %q, want List(Bool)", call.ResolvedType)
	}
}

func TestCheck_SetAtRejectsArray(t *testing.T) {
	call := &ast.CallExpr{Callee: "setAt", Args: []ast.Expr{&ast.IdentExpr{Name: "arr"}, &ast.IntLit{Value: 0}, &ast.IntLit{Value: 1}}}
	f := mainFile(
		&ast.LetExpr{Name: "arr", Type: at(nt("Int"), 3), Value: &ast.ListLit{Elems: []ast.Expr{&ast.IntLit{Value: 1}, &ast.IntLit{Value: 2}, &ast.IntLit{Value: 3}}}},
		call,
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error: setAt is List-only, Array must use x[i]=v instead (amifl-spec.md section 13.4)")
	}
}

func TestCheck_PushIsNonDestructiveReturningListType(t *testing.T) {
	call := &ast.CallExpr{Callee: "push", Args: []ast.Expr{&ast.IdentExpr{Name: "xs"}, &ast.IntLit{Value: 4}}}
	f := mainFile(
		&ast.LetExpr{Name: "xs", Value: intListLit(1, 2, 3)},
		&ast.LetExpr{Name: "ys", Value: call},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if call.ResolvedType != "List(Int64)" {
		t.Fatalf("got ResolvedType %q, want List(Int64)", call.ResolvedType)
	}
}

func TestCheck_PopReturnsTuple2OfListAndElem(t *testing.T) {
	call := &ast.CallExpr{Callee: "pop", Args: []ast.Expr{&ast.IdentExpr{Name: "xs"}}}
	f := mainFile(
		&ast.LetExpr{Name: "xs", Value: intListLit(1, 2, 3)},
		&ast.LetExpr{Name: "r", Value: call},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if call.ResolvedType != "Tuple(List(Int64),Int64)" {
		t.Fatalf("got ResolvedType %q, want Tuple(List(Int64),Int64)", call.ResolvedType)
	}
}

func TestCheck_ZipRequiresBothArgsToBeList(t *testing.T) {
	call := &ast.CallExpr{Callee: "zip", Args: []ast.Expr{&ast.IdentExpr{Name: "xs"}, &ast.IdentExpr{Name: "arr"}}}
	f := mainFile(
		&ast.LetExpr{Name: "xs", Value: intListLit(1, 2, 3)},
		&ast.LetExpr{Name: "arr", Type: at(nt("Int"), 3), Value: &ast.ListLit{Elems: []ast.Expr{&ast.IntLit{Value: 1}, &ast.IntLit{Value: 2}, &ast.IntLit{Value: 3}}}},
		&ast.DiscardExpr{Value: call},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error: zip's second argument is an Array, not a List (13.4's table restricts zip to List)")
	}
}

func TestCheck_ReverseOnArrayPreservesArrayType(t *testing.T) {
	call := &ast.CallExpr{Callee: "reverse", Args: []ast.Expr{&ast.IdentExpr{Name: "arr"}}}
	f := mainFile(
		&ast.LetExpr{Name: "arr", Type: at(nt("Int"), 3), Value: &ast.ListLit{Elems: []ast.Expr{&ast.IntLit{Value: 1}, &ast.IntLit{Value: 2}, &ast.IntLit{Value: 3}}}},
		&ast.LetExpr{Name: "r", Value: call},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if call.ResolvedType != "Array(Int64;3)" {
		t.Fatalf("got ResolvedType %q, want Array(Int64;3) — reverse must preserve an Array's fixed size", call.ResolvedType)
	}
}

func TestCheck_ContainsRejectsNonComparableElement(t *testing.T) {
	nested := &ast.LetExpr{Name: "xss", Value: &ast.ListLit{Elems: []ast.Expr{intListLit(1, 2)}}}
	call := &ast.CallExpr{Callee: "contains", Args: []ast.Expr{&ast.IdentExpr{Name: "xss"}, intListLit(1, 2)}}
	f := mainFile(nested, &ast.DiscardExpr{Value: call}, &ast.IntLit{Value: 0})
	if err := Check(f); err == nil {
		t.Fatal("expected an error: List[List[Int]]'s element type (List[Int]) isn't comparable")
	}
}

// Phase 11c (amifl-spec.md sections 13.5/13.6) — Set/Map built-ins.

func TestCheck_SetAddDiscardAreUnitTyped(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "s", Value: intSetLit(1, 2, 3)},
		&ast.CallExpr{Callee: "add", Args: []ast.Expr{&ast.IdentExpr{Name: "s"}, &ast.IntLit{Value: 4}}},
		&ast.CallExpr{Callee: "discard", Args: []ast.Expr{&ast.IdentExpr{Name: "s"}, &ast.IntLit{Value: 1}}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_SetUnionRequiresMatchingSetTypes(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "a", Value: intSetLit(1, 2)},
		&ast.LetExpr{Name: "b", Value: &ast.SetOrMapLit{Elems: []ast.Expr{&ast.StringLit{Value: "x"}}}},
		&ast.DiscardExpr{Value: &ast.CallExpr{Callee: "union", Args: []ast.Expr{&ast.IdentExpr{Name: "a"}, &ast.IdentExpr{Name: "b"}}}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error: union(Set[Int], Set[String]) — mismatched element types")
	}
}

func TestCheck_SetIntersectDifferenceReturnSetType(t *testing.T) {
	for _, name := range []string{"intersect", "difference"} {
		t.Run(name, func(t *testing.T) {
			call := &ast.CallExpr{Callee: name, Args: []ast.Expr{&ast.IdentExpr{Name: "a"}, &ast.IdentExpr{Name: "b"}}}
			f := mainFile(
				&ast.LetExpr{Name: "a", Value: intSetLit(1, 2)},
				&ast.LetExpr{Name: "b", Value: intSetLit(2, 3)},
				&ast.LetExpr{Name: "r", Value: call},
				&ast.IntLit{Value: 0},
			)
			if err := Check(f); err != nil {
				t.Fatalf("Check() error: %v", err)
			}
			if call.ResolvedType != "Set(Int64)" {
				t.Fatalf("got ResolvedType %q, want Set(Int64)", call.ResolvedType)
			}
		})
	}
}

func TestCheck_SetToListReturnsListOfElemType(t *testing.T) {
	call := &ast.CallExpr{Callee: "toList", Args: []ast.Expr{&ast.IdentExpr{Name: "s"}}}
	f := mainFile(
		&ast.LetExpr{Name: "s", Value: intSetLit(1, 2, 3)},
		&ast.LetExpr{Name: "xs", Value: call},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if call.ResolvedType != "List(Int64)" {
		t.Fatalf("got ResolvedType %q, want List(Int64)", call.ResolvedType)
	}
}

func mapOfStringInt() *ast.SetOrMapLit {
	return &ast.SetOrMapLit{Entries: []ast.MapLitEntry{
		{Key: &ast.StringLit{Value: "a"}, Value: &ast.IntLit{Value: 1}},
	}}
}

func TestCheck_MapKeysValuesEntriesTypes(t *testing.T) {
	keysCall := &ast.CallExpr{Callee: "keys", Args: []ast.Expr{&ast.IdentExpr{Name: "m"}}}
	valuesCall := &ast.CallExpr{Callee: "values", Args: []ast.Expr{&ast.IdentExpr{Name: "m"}}}
	entriesCall := &ast.CallExpr{Callee: "entries", Args: []ast.Expr{&ast.IdentExpr{Name: "m"}}}
	f := mainFile(
		&ast.LetExpr{Name: "m", Value: mapOfStringInt()},
		&ast.LetExpr{Name: "ks", Value: keysCall},
		&ast.LetExpr{Name: "vs", Value: valuesCall},
		&ast.LetExpr{Name: "es", Value: entriesCall},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if keysCall.ResolvedType != "List(String)" {
		t.Fatalf("got keys() ResolvedType %q, want List(String)", keysCall.ResolvedType)
	}
	if valuesCall.ResolvedType != "List(Int64)" {
		t.Fatalf("got values() ResolvedType %q, want List(Int64)", valuesCall.ResolvedType)
	}
	if entriesCall.ResolvedType != "List(Tuple(String,Int64))" {
		t.Fatalf("got entries() ResolvedType %q, want List(Tuple(String,Int64))", entriesCall.ResolvedType)
	}
}

func TestCheck_MapGetRequiresDefaultMatchingValueType(t *testing.T) {
	call := &ast.CallExpr{Callee: "get", Args: []ast.Expr{&ast.IdentExpr{Name: "m"}, &ast.StringLit{Value: "a"}, &ast.StringLit{Value: "nope"}}}
	f := mainFile(
		&ast.LetExpr{Name: "m", Value: mapOfStringInt()},
		&ast.DiscardExpr{Value: call},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error: get(Map[String,Int], _, default: String) — default must be Int")
	}
}

func TestCheck_MapGetReturnsValueType(t *testing.T) {
	call := &ast.CallExpr{Callee: "get", Args: []ast.Expr{&ast.IdentExpr{Name: "m"}, &ast.StringLit{Value: "a"}, &ast.IntLit{Value: 0}}}
	f := mainFile(
		&ast.LetExpr{Name: "m", Value: mapOfStringInt()},
		&ast.LetExpr{Name: "v", Value: call},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if call.ResolvedType != "Int64" {
		t.Fatalf("got ResolvedType %q, want Int64", call.ResolvedType)
	}
}

func TestCheck_MapSetDeleteAreUnitTyped(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "m", Value: mapOfStringInt()},
		&ast.CallExpr{Callee: "set", Args: []ast.Expr{&ast.IdentExpr{Name: "m"}, &ast.StringLit{Value: "b"}, &ast.IntLit{Value: 2}}},
		&ast.CallExpr{Callee: "delete", Args: []ast.Expr{&ast.IdentExpr{Name: "m"}, &ast.StringLit{Value: "a"}}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_MapBuiltinsRejectNonMapArg(t *testing.T) {
	for _, name := range []string{"keys", "values", "entries"} {
		t.Run(name, func(t *testing.T) {
			call := &ast.CallExpr{Callee: name, Args: []ast.Expr{&ast.IntLit{Value: 1}}}
			f := mainFile(&ast.DiscardExpr{Value: call}, &ast.IntLit{Value: 0})
			if err := Check(f); err == nil {
				t.Fatalf("expected an error: %s(Int) — Int isn't a Map", name)
			}
		})
	}
}

// Phase 11d (amifl-spec.md sections 13.7/13.9) — numeric and error-
// handling built-ins, the final phase of step 11.

func TestCheck_MinMaxAdaptLiteralToNonLiteralOperand(t *testing.T) {
	call := &ast.CallExpr{Callee: "min", Args: []ast.Expr{&ast.IdentExpr{Name: "x"}, &ast.IntLit{Value: 0}}}
	f := mainFile(
		&ast.LetExpr{Name: "x", Type: nt("Int8"), Value: &ast.IntLit{Value: 5}},
		&ast.LetExpr{Name: "r", Value: call},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if call.ResolvedType != "Int8" {
		t.Fatalf("got ResolvedType %q, want Int8 (the literal 0 should adapt to x's own Int8 type)", call.ResolvedType)
	}
}

func TestCheck_MinRejectsMismatchedTypes(t *testing.T) {
	call := &ast.CallExpr{Callee: "min", Args: []ast.Expr{&ast.IdentExpr{Name: "x"}, &ast.IdentExpr{Name: "y"}}}
	f := mainFile(
		&ast.LetExpr{Name: "x", Type: nt("Int8"), Value: &ast.IntLit{Value: 5}},
		&ast.LetExpr{Name: "y", Type: nt("Int32"), Value: &ast.IntLit{Value: 5}},
		&ast.DiscardExpr{Value: call},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error: min(Int8, Int32) — no implicit conversion between differently-sized ints")
	}
}

func TestCheck_MinMaxRejectString(t *testing.T) {
	// 13.7 is specifically the numeric-functions section — min/max are
	// restricted to Numeric, not the broader Ordered capability (String
	// included) that `<`/`<=` use.
	call := &ast.CallExpr{Callee: "min", Args: []ast.Expr{&ast.StringLit{Value: "a"}, &ast.StringLit{Value: "b"}}}
	f := mainFile(&ast.DiscardExpr{Value: call}, &ast.IntLit{Value: 0})
	if err := Check(f); err == nil {
		t.Fatal("expected an error: min(String, String) — String isn't Numeric")
	}
}

func TestCheck_ClampResolvesLoHiAgainstFirstArgType(t *testing.T) {
	call := &ast.CallExpr{Callee: "clamp", Args: []ast.Expr{&ast.IdentExpr{Name: "v"}, &ast.IntLit{Value: 0}, &ast.IntLit{Value: 100}}}
	f := mainFile(
		&ast.LetExpr{Name: "v", Type: nt("UInt16"), Value: &ast.IntLit{Value: 5}},
		&ast.LetExpr{Name: "r", Value: call},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if call.ResolvedType != "UInt16" {
		t.Fatalf("got ResolvedType %q, want UInt16", call.ResolvedType)
	}
}

func TestCheck_RoundFloorCeilSqrtRejectInt(t *testing.T) {
	for _, name := range []string{"round", "floor", "ceil", "sqrt"} {
		t.Run(name, func(t *testing.T) {
			call := &ast.CallExpr{Callee: name, Args: []ast.Expr{&ast.IdentExpr{Name: "n"}}}
			f := mainFile(
				&ast.LetExpr{Name: "n", Value: &ast.IntLit{Value: 5}},
				&ast.DiscardExpr{Value: call},
				&ast.IntLit{Value: 0},
			)
			if err := Check(f); err == nil {
				t.Fatalf("expected an error: %s(Int) — restricted to Float", name)
			}
		})
	}
}

func TestCheck_PowRequiresMatchingFloatTypes(t *testing.T) {
	call := &ast.CallExpr{Callee: "pow", Args: []ast.Expr{&ast.IdentExpr{Name: "base"}, &ast.FloatLit{Value: 2}}}
	f := mainFile(
		&ast.LetExpr{Name: "base", Type: nt("Float32"), Value: &ast.FloatLit{Value: 2}},
		&ast.LetExpr{Name: "r", Value: call},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if call.ResolvedType != "Float32" {
		t.Fatalf("got ResolvedType %q, want Float32", call.ResolvedType)
	}
}

func TestCheck_UnwrapReturnsPayloadType(t *testing.T) {
	call := &ast.CallExpr{Callee: "unwrap", Args: []ast.Expr{&ast.CallExpr{Callee: "parse", TypeArg: nt("Int"), Args: []ast.Expr{&ast.StringLit{Value: "5"}}}}}
	f := mainFile(&ast.LetExpr{Name: "x", Value: call}, &ast.IntLit{Value: 0})
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if call.ResolvedType != "Int64" {
		t.Fatalf("got ResolvedType %q, want Int64", call.ResolvedType)
	}
}

func TestCheck_UnwrapWithExplicitTypeArgMustMatchPayload(t *testing.T) {
	call := &ast.CallExpr{
		Callee: "unwrap", TypeArg: nt("String"),
		Args: []ast.Expr{&ast.CallExpr{Callee: "parse", TypeArg: nt("Int"), Args: []ast.Expr{&ast.StringLit{Value: "5"}}}},
	}
	f := mainFile(&ast.DiscardExpr{Value: call}, &ast.IntLit{Value: 0})
	if err := Check(f); err == nil {
		t.Fatal("expected an error: unwrap[String](Tuple2[Int,Error]) — explicit type arg doesn't match the payload type")
	}
}

func TestCheck_OkOrRequiresDefaultMatchingPayloadType(t *testing.T) {
	call := &ast.CallExpr{
		Callee: "okOr",
		Args: []ast.Expr{
			&ast.CallExpr{Callee: "parse", TypeArg: nt("Int"), Args: []ast.Expr{&ast.StringLit{Value: "5"}}},
			&ast.StringLit{Value: "nope"},
		},
	}
	f := mainFile(&ast.DiscardExpr{Value: call}, &ast.IntLit{Value: 0})
	if err := Check(f); err == nil {
		t.Fatal("expected an error: okOr(Tuple2[Int,Error], String) — default must be Int")
	}
}

func TestCheck_OkOrReturnsPayloadType(t *testing.T) {
	call := &ast.CallExpr{
		Callee: "okOr",
		Args: []ast.Expr{
			&ast.CallExpr{Callee: "parse", TypeArg: nt("Int"), Args: []ast.Expr{&ast.StringLit{Value: "5"}}},
			&ast.IntLit{Value: 0},
		},
	}
	f := mainFile(&ast.LetExpr{Name: "x", Value: call}, &ast.IntLit{Value: 0})
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if call.ResolvedType != "Int64" {
		t.Fatalf("got ResolvedType %q, want Int64", call.ResolvedType)
	}
}

func TestCheck_UnwrapRejectsNonTuple2Arg(t *testing.T) {
	call := &ast.CallExpr{Callee: "unwrap", Args: []ast.Expr{&ast.IntLit{Value: 5}}}
	f := mainFile(&ast.DiscardExpr{Value: call}, &ast.IntLit{Value: 0})
	if err := Check(f); err == nil {
		t.Fatal("expected an error: unwrap(Int) — Int isn't Tuple2[T,Error]")
	}
}

// --- step 12: Chan[T]/Stream[T]/File built-ins ---

func chanIntLit(buf int) *ast.CallExpr {
	return &ast.CallExpr{Callee: "chan", TypeArg: nt("Int"), Args: []ast.Expr{&ast.IntLit{Value: uint64(buf)}}}
}

func TestCheck_ChanLitResolvesToChanType(t *testing.T) {
	call := chanIntLit(0)
	f := mainFile(&ast.LetExpr{Name: "ch", Value: call}, &ast.IntLit{Value: 0})
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if call.ResolvedType != "Chan(Int64)" {
		t.Fatalf("got ResolvedType %q, want Chan(Int64)", call.ResolvedType)
	}
}

func TestCheck_ChanWithoutTypeArgIsAnError(t *testing.T) {
	call := &ast.CallExpr{Callee: "chan", Args: []ast.Expr{&ast.IntLit{Value: 0}}}
	f := mainFile(&ast.DiscardExpr{Value: call}, &ast.IntLit{Value: 0})
	if err := Check(f); err == nil {
		t.Fatal("expected an error for chan(...) with no bracketed type argument")
	}
}

func TestCheck_SendRequiresMatchingElemType(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "ch", Value: chanIntLit(0)},
		&ast.DiscardExpr{Value: &ast.CallExpr{Callee: "send", Args: []ast.Expr{
			&ast.IdentExpr{Name: "ch"}, &ast.StringLit{Value: "nope"},
		}}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error: send(Chan[Int], String) — elem type mismatch")
	}
}

func TestCheck_RecvReturnsTuple2OfElemAndBool(t *testing.T) {
	call := &ast.CallExpr{Callee: "recv", Args: []ast.Expr{&ast.IdentExpr{Name: "ch"}}}
	f := mainFile(
		&ast.LetExpr{Name: "ch", Value: chanIntLit(0)},
		&ast.LetExpr{Name: "r", Value: call},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if call.ResolvedType != "Tuple(Int64,Bool)" {
		t.Fatalf("got ResolvedType %q, want Tuple(Int64,Bool)", call.ResolvedType)
	}
}

func TestCheck_RecvNonChanArgIsAnError(t *testing.T) {
	call := &ast.CallExpr{Callee: "recv", Args: []ast.Expr{&ast.IntLit{Value: 1}}}
	f := mainFile(&ast.DiscardExpr{Value: call}, &ast.IntLit{Value: 0})
	if err := Check(f); err == nil {
		t.Fatal("expected an error: recv(Int) — Int isn't a Chan[T]")
	}
}

func TestCheck_SpawnRequiresZeroArgUnitClosure(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "f", Value: &ast.ClosureLit{
			Params:     []ast.Param{{Name: "x", Type: nt("Int")}},
			ReturnType: nt("Unit"),
			Body:       &ast.Block{Exprs: []ast.Expr{printStr("x")}},
		}},
		&ast.DiscardExpr{Value: &ast.CallExpr{Callee: "spawn", Args: []ast.Expr{&ast.IdentExpr{Name: "f"}}}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error: spawn requires fn() -> Unit, got fn(Int) -> Unit")
	}
}

func TestCheck_SpawnAcceptsZeroArgUnitClosure(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "f", Value: &ast.ClosureLit{
			ReturnType: nt("Unit"),
			Body:       &ast.Block{Exprs: []ast.Expr{printStr("x")}},
		}},
		&ast.DiscardExpr{Value: &ast.CallExpr{Callee: "spawn", Args: []ast.Expr{&ast.IdentExpr{Name: "f"}}}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func linesOfStdin() *ast.CallExpr {
	return &ast.CallExpr{Callee: "lines", Args: []ast.Expr{&ast.CallExpr{Callee: "stdin"}}}
}

func TestCheck_LinesReturnsStreamString(t *testing.T) {
	call := linesOfStdin()
	f := mainFile(&ast.LetExpr{Name: "s", Value: call}, &ast.IntLit{Value: 0})
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if call.ResolvedType != "Stream(String)" {
		t.Fatalf("got ResolvedType %q, want Stream(String)", call.ResolvedType)
	}
}

func TestCheck_ParallelRequiresStreamArg(t *testing.T) {
	call := &ast.CallExpr{Callee: "parallel", Args: []ast.Expr{&ast.IntLit{Value: 1}, &ast.IntLit{Value: 2}}}
	f := mainFile(&ast.DiscardExpr{Value: call}, &ast.IntLit{Value: 0})
	if err := Check(f); err == nil {
		t.Fatal("expected an error: parallel(Int, Int) — first argument isn't a Stream[T]")
	}
}

func TestCheck_ParallelReturnsSameStreamType(t *testing.T) {
	call := &ast.CallExpr{Callee: "parallel", Args: []ast.Expr{linesOfStdin(), &ast.IntLit{Value: 4}}}
	f := mainFile(&ast.LetExpr{Name: "ps", Value: call}, &ast.IntLit{Value: 0})
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if call.ResolvedType != "Stream(String)" {
		t.Fatalf("got ResolvedType %q, want Stream(String)", call.ResolvedType)
	}
}

func TestCheck_CollectReturnsListOfStreamElem(t *testing.T) {
	call := &ast.CallExpr{Callee: "collect", Args: []ast.Expr{linesOfStdin()}}
	f := mainFile(&ast.LetExpr{Name: "xs", Value: call}, &ast.IntLit{Value: 0})
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if call.ResolvedType != "List(String)" {
		t.Fatalf("got ResolvedType %q, want List(String)", call.ResolvedType)
	}
}

func TestCheck_TakeSkipReturnStreamType(t *testing.T) {
	for _, name := range []string{"take", "skip"} {
		call := &ast.CallExpr{Callee: name, Args: []ast.Expr{linesOfStdin(), &ast.IntLit{Value: 2}}}
		f := mainFile(&ast.LetExpr{Name: "s2", Value: call}, &ast.IntLit{Value: 0})
		if err := Check(f); err != nil {
			t.Fatalf("Check() error for %s: %v", name, err)
		}
		if call.ResolvedType != "Stream(String)" {
			t.Fatalf("%s: got ResolvedType %q, want Stream(String)", name, call.ResolvedType)
		}
	}
}

func TestCheck_OpenReturnsTuple2FileError(t *testing.T) {
	call := &ast.CallExpr{Callee: "open", Args: []ast.Expr{&ast.StringLit{Value: "/tmp/x"}, &ast.StringLit{Value: "r"}}}
	f := mainFile(&ast.LetExpr{Name: "r", Value: call}, &ast.IntLit{Value: 0})
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if call.ResolvedType != "Tuple(File,Error)" {
		t.Fatalf("got ResolvedType %q, want Tuple(File,Error)", call.ResolvedType)
	}
}

func TestCheck_CloseReturnsError(t *testing.T) {
	call := &ast.CallExpr{Callee: "close", Args: []ast.Expr{&ast.CallExpr{Callee: "stdin"}}}
	f := mainFile(&ast.DiscardExpr{Value: call}, &ast.IntLit{Value: 0})
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if call.ResolvedType != "Error" {
		t.Fatalf("got ResolvedType %q, want Error", call.ResolvedType)
	}
}

func TestCheck_WriteRequiresBytesArg(t *testing.T) {
	call := &ast.CallExpr{Callee: "write", Args: []ast.Expr{&ast.CallExpr{Callee: "stdout"}, &ast.StringLit{Value: "nope"}}}
	f := mainFile(&ast.DiscardExpr{Value: call}, &ast.IntLit{Value: 0})
	if err := Check(f); err == nil {
		t.Fatal("expected an error: write(File, String) — data must be Bytes (List[UInt8])")
	}
}

func TestCheck_WriteAcceptsBytesAnnotatedListLit(t *testing.T) {
	call := &ast.CallExpr{Callee: "write", Args: []ast.Expr{&ast.CallExpr{Callee: "stdout"}, &ast.IdentExpr{Name: "data"}}}
	f := mainFile(
		&ast.LetExpr{Name: "data", Type: nt("Bytes"), Value: &ast.ListLit{Elems: []ast.Expr{&ast.IntLit{Value: 1}, &ast.IntLit{Value: 2}}}},
		&ast.LetExpr{Name: "r", Value: call},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if call.ResolvedType != "Tuple(Int64,Error)" {
		t.Fatalf("got ResolvedType %q, want Tuple(Int64,Error)", call.ResolvedType)
	}
}

func TestCheck_LenAcceptsChan(t *testing.T) {
	call := &ast.CallExpr{Callee: "len", Args: []ast.Expr{&ast.IdentExpr{Name: "ch"}}}
	f := mainFile(
		&ast.LetExpr{Name: "ch", Value: chanIntLit(0)},
		&ast.LetExpr{Name: "n", Value: call},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_SliceOnStreamReturnsStream(t *testing.T) {
	call := &ast.CallExpr{Callee: "slice", Args: []ast.Expr{linesOfStdin(), &ast.IntLit{Value: 0}, &ast.IntLit{Value: 2}}}
	f := mainFile(&ast.LetExpr{Name: "s2", Value: call}, &ast.IntLit{Value: 0})
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if call.ResolvedType != "Stream(String)" {
		t.Fatalf("got ResolvedType %q, want Stream(String)", call.ResolvedType)
	}
}

func TestCheck_ForOverStreamBindsElemType(t *testing.T) {
	forExpr := &ast.ForExpr{
		Var:   "line",
		Items: &ast.IdentExpr{Name: "s"},
		Body:  &ast.Block{Exprs: []ast.Expr{&ast.DiscardExpr{Value: &ast.IdentExpr{Name: "line"}}}},
	}
	f := mainFile(
		&ast.LetExpr{Name: "s", Value: linesOfStdin()},
		forExpr,
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if forExpr.ElemType != "String" {
		t.Fatalf("got ElemType %q, want String", forExpr.ElemType)
	}
}

func TestCheck_ForYieldOverStreamIsAnError(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "s", Value: linesOfStdin()},
		&ast.DiscardExpr{Value: &ast.ForExpr{
			Var:   "line",
			Items: &ast.IdentExpr{Name: "s"},
			Yield: &ast.IdentExpr{Name: "line"},
		}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error: `for ... yield ...` over a Stream[T] isn't supported")
	}
}

// --- step 13: extern/bind (Any/extern value boundary, CLAUDE.md design issue 1) ---

// externFile builds a *ast.File with ext prepended ahead of `fn main`
// (mirroring TestCheck_TopLevelConstVisibleInMain's own pattern for a
// const), main's body built from mainExprs.
func externFile(ext *ast.ExternDecl, mainExprs ...ast.Expr) *ast.File {
	return &ast.File{
		Decls: []ast.TopLevelDecl{
			ext,
			&ast.FuncDecl{Name: "main", ReturnType: nt("Int"), Body: &ast.Block{Exprs: mainExprs}},
		},
	}
}

func TestCheck_ExternPlainBindCallResolvesAndSetsCalleeToken(t *testing.T) {
	ext := &ast.ExternDecl{
		Path:  "encoding/json",
		Alias: "json",
		Binds: []ast.ExternBindDecl{
			{Name: "Marshal", Params: []ast.Param{{Name: "v", Type: nt("Any")}}, ReturnType: &ast.TupleType{Elems: []ast.TypeExpr{nt("Bytes"), nt("Error")}}},
		},
	}
	call := &ast.CallExpr{Callee: "Marshal", Args: []ast.Expr{&ast.StringLit{Value: "hi"}}}
	f := externFile(ext,
		&ast.LetExpr{Name: "m", Value: call},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if call.CalleeToken != "?json.Marshal" {
		t.Fatalf("got CalleeToken %q, want \"?json.Marshal\"", call.CalleeToken)
	}
	if call.ExternMethod != "" {
		t.Fatalf("got ExternMethod %q, want \"\"", call.ExternMethod)
	}
}

func TestCheck_ExternBindRenameSetsCalleeTokenToGoTarget(t *testing.T) {
	ext := &ast.ExternDecl{
		Path:  "encoding/json",
		Alias: "json",
		Binds: []ast.ExternBindDecl{
			{Name: "Marshal2", GoTarget: "Marshal", Params: []ast.Param{{Name: "v", Type: nt("Any")}}, ReturnType: &ast.TupleType{Elems: []ast.TypeExpr{nt("Bytes"), nt("Error")}}},
		},
	}
	call := &ast.CallExpr{Callee: "Marshal2", Args: []ast.Expr{&ast.StringLit{Value: "hi"}}}
	f := externFile(ext, &ast.DiscardExpr{Value: call}, &ast.IntLit{Value: 0})
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if call.CalleeToken != "?json.Marshal" {
		t.Fatalf("got CalleeToken %q, want \"?json.Marshal\"", call.CalleeToken)
	}
}

func TestCheck_ExternMethodBindSetsExternMethod(t *testing.T) {
	ext := &ast.ExternDecl{
		Path:  "time",
		Alias: "time",
		Types: []ast.ExternTypeDecl{{Name: "Time"}},
		Binds: []ast.ExternBindDecl{
			{Name: "Now", ReturnType: nt("Time")},
			{Name: "TimeUnix", GoTarget: "Time.Unix", Params: []ast.Param{{Name: "t", Type: nt("Time")}}, ReturnType: nt("Int")},
		},
	}
	call := &ast.CallExpr{Callee: "TimeUnix", Args: []ast.Expr{&ast.CallExpr{Callee: "Now"}}}
	f := externFile(ext, &ast.LetExpr{Name: "u", Value: call}, &ast.IntLit{Value: 0})
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if call.ExternMethod != "Unix" {
		t.Fatalf("got ExternMethod %q, want \"Unix\"", call.ExternMethod)
	}
	if call.CalleeToken != "" {
		t.Fatalf("got CalleeToken %q, want \"\" (method-style bind has no fixed callname)", call.CalleeToken)
	}
	if len(call.ExternParamTypes) != 1 || call.ExternParamTypes[0] != "Time" {
		t.Fatalf("got ExternParamTypes %#v, want [\"Time\"]", call.ExternParamTypes)
	}
}

func TestCheck_ExternMethodBindReceiverTypeMismatchIsAnError(t *testing.T) {
	ext := &ast.ExternDecl{
		Path:  "time",
		Alias: "time",
		Types: []ast.ExternTypeDecl{{Name: "Time"}},
		Binds: []ast.ExternBindDecl{
			{Name: "BadUnix", GoTarget: "Time.Unix", Params: []ast.Param{{Name: "s", Type: nt("String")}}, ReturnType: nt("Int")},
		},
	}
	f := externFile(ext, &ast.IntLit{Value: 0})
	if err := Check(f); err == nil {
		t.Fatal("expected an error: method-style bind's first parameter type must match the receiver type in GoTarget")
	}
}

func TestCheck_ExternMethodBindWithNoParamsIsAnError(t *testing.T) {
	ext := &ast.ExternDecl{
		Path:  "time",
		Alias: "time",
		Types: []ast.ExternTypeDecl{{Name: "Time"}},
		Binds: []ast.ExternBindDecl{
			{Name: "BadUnix", GoTarget: "Time.Unix", ReturnType: nt("Int")},
		},
	}
	f := externFile(ext, &ast.IntLit{Value: 0})
	if err := Check(f); err == nil {
		t.Fatal("expected an error: a method-style bind needs at least one parameter to serve as the receiver")
	}
}

func TestCheck_ExternDuplicateAliasIsAnError(t *testing.T) {
	f := &ast.File{
		Decls: []ast.TopLevelDecl{
			&ast.ExternDecl{Path: "a", Alias: "dup"},
			&ast.ExternDecl{Path: "b", Alias: "dup"},
			&ast.FuncDecl{Name: "main", ReturnType: nt("Int"), Body: &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 0}}}},
		},
	}
	if err := Check(f); err == nil {
		t.Fatal("expected an error for two extern blocks reusing one alias")
	}
}

func TestCheck_ExternReservedAliasIsAnError(t *testing.T) {
	ext := &ast.ExternDecl{Path: "somepkg", Alias: "os"}
	f := externFile(ext, &ast.IntLit{Value: 0})
	if err := Check(f); err == nil {
		t.Fatal("expected an error for an extern block claiming the reserved \"os\" alias")
	}
}

func TestCheck_ExternTypeCollidesWithStructIsAnError(t *testing.T) {
	f := &ast.File{
		Decls: []ast.TopLevelDecl{
			&ast.ExternDecl{Path: "time", Alias: "time", Types: []ast.ExternTypeDecl{{Name: "Point"}}},
			&ast.StructDecl{Name: "Point", Fields: []ast.Param{{Name: "x", Type: nt("Int")}}},
			&ast.FuncDecl{Name: "main", ReturnType: nt("Int"), Body: &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 0}}}},
		},
	}
	if err := Check(f); err == nil {
		t.Fatal("expected an error for an extern type name colliding with a struct")
	}
}

func TestCheck_ExternBindNameCollidesWithFuncIsAnError(t *testing.T) {
	ext := &ast.ExternDecl{
		Path:  "strings",
		Alias: "strs",
		Binds: []ast.ExternBindDecl{{Name: "helper", ReturnType: nt("Int")}},
	}
	f := externFile(ext, &ast.IntLit{Value: 0})
	f.Decls = append(f.Decls, &ast.FuncDecl{
		Name: "helper", ReturnType: nt("Int"),
		Body: &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 0}}},
	})
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a bind name colliding with a top-level fn")
	}
}

func TestCheck_ExternAnyParamAcceptsConcreteType(t *testing.T) {
	ext := &ast.ExternDecl{
		Path:  "encoding/json",
		Alias: "json",
		Binds: []ast.ExternBindDecl{
			{Name: "Marshal", Params: []ast.Param{{Name: "v", Type: nt("Any")}}, ReturnType: &ast.TupleType{Elems: []ast.TypeExpr{nt("Bytes"), nt("Error")}}},
		},
	}
	f := externFile(ext,
		&ast.LetExpr{Name: "x", Value: &ast.IntLit{Value: 5}},
		&ast.DiscardExpr{Value: &ast.CallExpr{Callee: "Marshal", Args: []ast.Expr{&ast.IdentExpr{Name: "x"}}}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v (an Any-typed parameter should accept any concrete argument type)", err)
	}
}

func TestCheck_ExternAnyValueCannotNarrowToConcreteType(t *testing.T) {
	ext := &ast.ExternDecl{
		Path:  "encoding/json",
		Alias: "json",
		Binds: []ast.ExternBindDecl{
			{Name: "MakeAny", ReturnType: nt("Any")},
		},
	}
	f := externFile(ext,
		&ast.LetExpr{Name: "x", Type: nt("Int"), Value: &ast.CallExpr{Callee: "MakeAny"}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error: an Any-typed value can't implicitly narrow to a concrete `let` annotation")
	}
}

func TestCheck_TypeNameAcceptsAnyConcreteValue(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "tn", Value: &ast.CallExpr{Callee: "typeName", Args: []ast.Expr{&ast.IntLit{Value: 5}}}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheck_TypeNameWrongArgCountIsAnError(t *testing.T) {
	f := mainFile(
		&ast.DiscardExpr{Value: &ast.CallExpr{Callee: "typeName", Args: []ast.Expr{&ast.IntLit{Value: 1}, &ast.IntLit{Value: 2}}}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error: typeName takes exactly 1 argument")
	}
}

// --- step 14: modules ---

// qualifiedCall builds `alias.field(args...)` (amifl-spec.md section
// 12.2) — Args is always non-nil (a zero-length slice for a zero-arg
// call), matching what parseFieldCallArgs itself always produces (see
// ast.FieldExpr's own doc comment on why nil vs empty distinguishes "no
// trailing (...) at all" from "an empty one").
func qualifiedCall(alias, field string, args ...ast.Expr) *ast.FieldExpr {
	sArgs := make([]ast.StructLitField, len(args))
	for i, a := range args {
		sArgs[i] = ast.StructLitField{Value: a}
	}
	return &ast.FieldExpr{Target: &ast.IdentExpr{Name: alias}, Field: field, Args: sArgs}
}

// qualifiedRef builds `alias.field` with no trailing call at all.
func qualifiedRef(alias, field string) *ast.FieldExpr {
	return &ast.FieldExpr{Target: &ast.IdentExpr{Name: alias}, Field: field}
}

func TestCheckPackage_BuildsExportsForCapitalizedFuncsAndConstsOnly(t *testing.T) {
	f := &ast.File{Decls: []ast.TopLevelDecl{
		&ast.FuncDecl{Name: "Clamp", Params: []ast.Param{{Name: "v", Type: nt("Int")}}, ReturnType: nt("Int"), Body: &ast.Block{Exprs: []ast.Expr{&ast.IdentExpr{Name: "v"}}}},
		&ast.FuncDecl{Name: "helper", ReturnType: nt("Int"), Body: &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 1}}}},
		&ast.ConstDecl{Name: "MaxClamp", Type: nt("Int"), Value: &ast.IntLit{Value: 100}},
		&ast.ConstDecl{Name: "minClamp", Type: nt("Int"), Value: &ast.IntLit{Value: 0}},
	}}
	exports, err := CheckPackage([]*ast.File{f}, "mathutil_", nil)
	if err != nil {
		t.Fatalf("CheckPackage() error: %v", err)
	}
	fn, ok := exports.Funcs["Clamp"]
	if !ok {
		t.Fatal("expected Clamp to be exported")
	}
	if fn.Token != "!mathutil_Clamp" {
		t.Fatalf("got Token %q, want \"!mathutil_Clamp\"", fn.Token)
	}
	if _, ok := exports.Funcs["helper"]; ok {
		t.Fatal("did not expect lowercase helper to be exported")
	}
	if _, ok := exports.Consts["MaxClamp"]; !ok {
		t.Fatal("expected MaxClamp to be exported")
	}
	if _, ok := exports.Consts["minClamp"]; ok {
		t.Fatal("did not expect lowercase minClamp to be exported")
	}
}

func TestCheckPackage_QualifiedCallResolvesToImportedFunc(t *testing.T) {
	imports := map[string]Exports{
		"mathutil": {Funcs: map[string]ExportedFunc{
			"Clamp": {Params: []string{"Int64", "Int64", "Int64"}, Ret: "Int64", Token: "!mathutil_Clamp"},
		}},
	}
	call := qualifiedCall("mathutil", "Clamp", &ast.IntLit{Value: 15}, &ast.IntLit{Value: 0}, &ast.IntLit{Value: 10})
	f := mainFile(&ast.DiscardExpr{Value: call}, &ast.IntLit{Value: 0})
	if _, err := CheckPackage([]*ast.File{f}, "", imports); err != nil {
		t.Fatalf("CheckPackage() error: %v", err)
	}
	if !call.IsQualifiedCall || call.QualifiedCallee != "!mathutil_Clamp" {
		t.Fatalf("got IsQualifiedCall=%v QualifiedCallee=%q, want true/\"!mathutil_Clamp\"", call.IsQualifiedCall, call.QualifiedCallee)
	}
	if call.ResolvedType != "Int64" {
		t.Fatalf("got ResolvedType %q, want \"Int64\"", call.ResolvedType)
	}
}

func TestCheckPackage_QualifiedCallArityMismatchIsAnError(t *testing.T) {
	imports := map[string]Exports{
		"util": {Funcs: map[string]ExportedFunc{"F": {Params: []string{"Int64"}, Ret: "Int64", Token: "!util_F"}}},
	}
	call := qualifiedCall("util", "F", &ast.IntLit{Value: 1}, &ast.IntLit{Value: 2})
	f := mainFile(&ast.DiscardExpr{Value: call}, &ast.IntLit{Value: 0})
	if _, err := CheckPackage([]*ast.File{f}, "", imports); err == nil {
		t.Fatal("expected an arity-mismatch error")
	}
}

func TestCheckPackage_QualifiedCallNamedArgumentIsAnError(t *testing.T) {
	imports := map[string]Exports{
		"util": {Funcs: map[string]ExportedFunc{"F": {Params: []string{"Int64"}, Ret: "Int64", Token: "!util_F"}}},
	}
	call := &ast.FieldExpr{Target: &ast.IdentExpr{Name: "util"}, Field: "F", Args: []ast.StructLitField{{Name: "x", Value: &ast.IntLit{Value: 1}}}}
	f := mainFile(&ast.DiscardExpr{Value: call}, &ast.IntLit{Value: 0})
	if _, err := CheckPackage([]*ast.File{f}, "", imports); err == nil {
		t.Fatal("expected an error: qualified calls take positional arguments only")
	}
}

func TestCheckPackage_UnexportedOrUnknownNameIsAnError(t *testing.T) {
	imports := map[string]Exports{"util": {Funcs: map[string]ExportedFunc{}, Consts: map[string]ExportedConst{}}}
	call := qualifiedCall("util", "hidden")
	f := mainFile(&ast.DiscardExpr{Value: call}, &ast.IntLit{Value: 0})
	if _, err := CheckPackage([]*ast.File{f}, "", imports); err == nil {
		t.Fatal("expected an error for an unexported/unknown qualified name")
	}
}

func TestCheckPackage_QualifiedConstReferenceInlinesExportedValue(t *testing.T) {
	imports := map[string]Exports{
		"util": {Consts: map[string]ExportedConst{"Max": {Typ: "Int64", Value: &ast.IntLit{Value: 100}}}},
	}
	ref := qualifiedRef("util", "Max")
	f := mainFile(ref)
	if _, err := CheckPackage([]*ast.File{f}, "", imports); err != nil {
		t.Fatalf("CheckPackage() error: %v", err)
	}
	if ref.ResolvedType != "Int64" {
		t.Fatalf("got ResolvedType %q, want \"Int64\"", ref.ResolvedType)
	}
	lit, ok := ref.QualifiedConstValue.(*ast.IntLit)
	if !ok || lit.Value != 100 {
		t.Fatalf("got QualifiedConstValue %#v, want IntLit{100}", ref.QualifiedConstValue)
	}
}

func TestCheckPackage_ConstCalledAsFunctionIsAnError(t *testing.T) {
	imports := map[string]Exports{
		"util": {Consts: map[string]ExportedConst{"Answer": {Typ: "Int64", Value: &ast.IntLit{Value: 42}}}},
	}
	call := qualifiedCall("util", "Answer", &ast.IntLit{Value: 1})
	f := mainFile(&ast.DiscardExpr{Value: call}, &ast.IntLit{Value: 0})
	if _, err := CheckPackage([]*ast.File{f}, "", imports); err == nil {
		t.Fatal("expected an error: a const isn't callable")
	}
}

func TestCheckPackage_NonRootPackageOwnMainIsNotValidatedAsEntryPoint(t *testing.T) {
	f := &ast.File{Decls: []ast.TopLevelDecl{
		&ast.FuncDecl{Name: "main", ReturnType: nt("Bool"), Body: &ast.Block{Exprs: []ast.Expr{&ast.BoolLit{Value: true}}}},
	}}
	if _, err := CheckPackage([]*ast.File{f}, "util_", nil); err != nil {
		t.Fatalf("CheckPackage() error: %v (a non-root package's own main should be an ordinary function)", err)
	}
}

func TestCheckPackage_RootPackageStillRequiresValidMain(t *testing.T) {
	if _, err := CheckPackage([]*ast.File{{}}, "", nil); err == nil {
		t.Fatal("expected an error for a root package missing fn main")
	}
}

func TestCheckPackage_MultipleFilesShareOneNamespaceWithNoImportNeeded(t *testing.T) {
	f1 := &ast.File{Decls: []ast.TopLevelDecl{
		&ast.FuncDecl{Name: "main", ReturnType: nt("Int"), Body: &ast.Block{Exprs: []ast.Expr{&ast.CallExpr{Callee: "helper"}}}},
	}}
	f2 := &ast.File{Decls: []ast.TopLevelDecl{
		&ast.FuncDecl{Name: "helper", ReturnType: nt("Int"), Body: &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 1}}}},
	}}
	if _, err := CheckPackage([]*ast.File{f1, f2}, "", nil); err != nil {
		t.Fatalf("CheckPackage() error: %v", err)
	}
}

// --- ex5: cross-package struct/enum references (amifl-spec.md section
// 12.2) ---

func TestCheckPackage_BuildsExportsForStructsAndEnumsCapitalizedOnly(t *testing.T) {
	f := &ast.File{Decls: []ast.TopLevelDecl{
		&ast.FuncDecl{Name: "main", ReturnType: nt("Int"), Body: &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 0}}}},
		&ast.StructDecl{Name: "Point", Fields: []ast.Param{{Name: "x", Type: nt("Int")}, {Name: "y", Type: nt("Int")}}},
		&ast.StructDecl{Name: "hidden", Fields: []ast.Param{{Name: "v", Type: nt("Int")}}},
		&ast.EnumDecl{Name: "Shape", Variants: []ast.EnumVariant{{Name: "Circle", Fields: []ast.Param{{Name: "radius", Type: nt("Int")}}}}},
		&ast.EnumDecl{Name: "invisible", Variants: []ast.EnumVariant{{Name: "Only"}}},
	}}
	exports, err := CheckPackage([]*ast.File{f}, "geo_", nil)
	if err != nil {
		t.Fatalf("CheckPackage() error: %v", err)
	}
	sInfo, ok := exports.Structs["Point"]
	if !ok {
		t.Fatal("expected Point to be exported")
	}
	if sInfo.GoName != "geo_Point" {
		t.Fatalf("got GoName %q, want \"geo_Point\"", sInfo.GoName)
	}
	if len(sInfo.Fields) != 2 {
		t.Fatalf("got %d fields, want 2", len(sInfo.Fields))
	}
	if _, ok := exports.Structs["hidden"]; ok {
		t.Fatal("did not expect lowercase hidden to be exported")
	}
	eInfo, ok := exports.Enums["Shape"]
	if !ok {
		t.Fatal("expected Shape to be exported")
	}
	if eInfo.GoName != "geo_Shape" {
		t.Fatalf("got GoName %q, want \"geo_Shape\"", eInfo.GoName)
	}
	if _, ok := exports.Enums["invisible"]; ok {
		t.Fatal("did not expect lowercase invisible to be exported")
	}
}

// TestCheckPackage_QualifiedTypeAnnotationAgreesWithConstructedValue is a
// regression test for the bug found while implementing ex5: a same-package
// function's parameter naming a struct type ("Point") and a cross-package
// construction of that same struct ("mathutil.Point{...}") must resolve to
// the *identical* canonical type string once seen from the importer's own
// perspective, or a perfectly well-typed cross-package call is rejected.
// This chains two real CheckPackage runs (mathutil's own, then main's,
// exactly like modloader.Load's dependency-order pipeline does) rather
// than hand-building an Exports literal — a hand-built one could trivially
// "cheat" by writing the already-correct rewritten form directly, without
// ever exercising buildExports' own exportTypeString rewriting at all.
func TestCheckPackage_QualifiedTypeAnnotationAgreesWithConstructedValue(t *testing.T) {
	mathutilFile := &ast.File{Decls: []ast.TopLevelDecl{
		&ast.StructDecl{Name: "Point", Fields: []ast.Param{{Name: "x", Type: nt("Int")}, {Name: "y", Type: nt("Int")}}},
		&ast.FuncDecl{
			Name: "SumPoint", Params: []ast.Param{{Name: "p", Type: nt("Point")}}, ReturnType: nt("Int"),
			Body: &ast.Block{Exprs: []ast.Expr{
				&ast.BinaryExpr{Op: "+",
					Left:  &ast.FieldExpr{Target: &ast.IdentExpr{Name: "p"}, Field: "x"},
					Right: &ast.FieldExpr{Target: &ast.IdentExpr{Name: "p"}, Field: "y"},
				},
			}},
		},
	}}
	mathutilExports, err := CheckPackage([]*ast.File{mathutilFile}, "mathutil_", nil)
	if err != nil {
		t.Fatalf("CheckPackage(mathutil) error: %v", err)
	}

	structLit := &ast.StructLit{Qualifier: "mathutil", TypeName: "Point", Fields: []ast.StructLitField{
		{Name: "x", Value: &ast.IntLit{Value: 3}},
		{Name: "y", Value: &ast.IntLit{Value: 4}},
	}}
	sumCall := qualifiedCall("mathutil", "SumPoint", structLit)
	mainFileAST := mainFile(&ast.DiscardExpr{Value: sumCall}, &ast.IntLit{Value: 0})
	imports := map[string]Exports{"mathutil": mathutilExports}
	if _, err := CheckPackage([]*ast.File{mainFileAST}, "", imports); err != nil {
		t.Fatalf("CheckPackage(main) error: %v (a value built from mathutil.Point{...} should type-check against SumPoint's own p: Point parameter)", err)
	}
	if !sumCall.IsQualifiedCall || sumCall.ResolvedType != "Int64" {
		t.Fatalf("got IsQualifiedCall=%v ResolvedType=%q, want true/\"Int64\"", sumCall.IsQualifiedCall, sumCall.ResolvedType)
	}
}

func TestCheckPackage_QualifiedStructLitConstructsAndTypeChecks(t *testing.T) {
	imports := map[string]Exports{
		"geo": {Structs: map[string]ExportedStruct{
			"Point": {Fields: []fieldInfo{{Name: "x", Typ: "Int64"}, {Name: "y", Typ: "Int64"}}, GoName: "geo_Point"},
		}},
	}
	lit := &ast.StructLit{Qualifier: "geo", TypeName: "Point", Fields: []ast.StructLitField{
		{Name: "x", Value: &ast.IntLit{Value: 1}},
		{Name: "y", Value: &ast.IntLit{Value: 2}},
	}}
	f := mainFile(&ast.LetExpr{Name: "p", Type: qt("geo", "Point"), Value: lit}, &ast.IntLit{Value: 0})
	if _, err := CheckPackage([]*ast.File{f}, "", imports); err != nil {
		t.Fatalf("CheckPackage() error: %v", err)
	}
	if lit.ResolvedType != "Qualified(geo_Point)" {
		t.Fatalf("got ResolvedType %q, want \"Qualified(geo_Point)\"", lit.ResolvedType)
	}
}

func TestCheckPackage_QualifiedStructLitMissingFieldIsAnError(t *testing.T) {
	imports := map[string]Exports{
		"geo": {Structs: map[string]ExportedStruct{
			"Point": {Fields: []fieldInfo{{Name: "x", Typ: "Int64"}, {Name: "y", Typ: "Int64"}}, GoName: "geo_Point"},
		}},
	}
	lit := &ast.StructLit{Qualifier: "geo", TypeName: "Point", Fields: []ast.StructLitField{
		{Name: "x", Value: &ast.IntLit{Value: 1}},
	}}
	f := mainFile(&ast.DiscardExpr{Value: lit}, &ast.IntLit{Value: 0})
	if _, err := CheckPackage([]*ast.File{f}, "", imports); err == nil {
		t.Fatal("expected an error for a qualified struct literal missing a field")
	}
}

func TestCheckPackage_QualifiedStructLitUnknownStructIsAnError(t *testing.T) {
	imports := map[string]Exports{"geo": {Structs: map[string]ExportedStruct{}}}
	lit := &ast.StructLit{Qualifier: "geo", TypeName: "Circle", Fields: nil}
	f := mainFile(&ast.DiscardExpr{Value: lit}, &ast.IntLit{Value: 0})
	if _, err := CheckPackage([]*ast.File{f}, "", imports); err == nil {
		t.Fatal("expected an error for a qualified struct literal naming an unknown/unexported struct")
	}
}

func TestCheckPackage_QualifiedEnumVariantConstructsAndTypeChecks(t *testing.T) {
	imports := map[string]Exports{
		"geo": {Enums: map[string]ExportedEnum{
			"Shape": {
				Variants: []variantInfo{{Name: "Circle", Fields: []fieldInfo{{Name: "radius", Typ: "Int64"}}}},
				GoName:   "geo_Shape",
			},
		}},
	}
	construction := &ast.FieldExpr{
		Target: &ast.FieldExpr{Target: &ast.IdentExpr{Name: "geo"}, Field: "Shape"},
		Field:  "Circle",
		Args:   []ast.StructLitField{{Name: "radius", Value: &ast.IntLit{Value: 5}}},
	}
	f := mainFile(&ast.LetExpr{Name: "s", Value: construction}, &ast.IntLit{Value: 0})
	if _, err := CheckPackage([]*ast.File{f}, "", imports); err != nil {
		t.Fatalf("CheckPackage() error: %v", err)
	}
	if !construction.IsEnumVariant || construction.VariantIndex != 0 {
		t.Fatalf("got IsEnumVariant=%v VariantIndex=%d, want true/0", construction.IsEnumVariant, construction.VariantIndex)
	}
	if construction.ResolvedType != "Qualified(geo_Shape)" {
		t.Fatalf("got ResolvedType %q, want \"Qualified(geo_Shape)\"", construction.ResolvedType)
	}
}

func TestCheckPackage_QualifiedEnumVariantUnknownVariantIsAnError(t *testing.T) {
	imports := map[string]Exports{
		"geo": {Enums: map[string]ExportedEnum{"Shape": {Variants: []variantInfo{{Name: "Circle"}}, GoName: "geo_Shape"}}},
	}
	construction := &ast.FieldExpr{
		Target: &ast.FieldExpr{Target: &ast.IdentExpr{Name: "geo"}, Field: "Shape"},
		Field:  "Square",
		Args:   []ast.StructLitField{},
	}
	f := mainFile(&ast.DiscardExpr{Value: construction}, &ast.IntLit{Value: 0})
	if _, err := CheckPackage([]*ast.File{f}, "", imports); err == nil {
		t.Fatal("expected an error for an unknown variant on a cross-package enum")
	}
}

func TestCheck_SwitchOverCrossPackageEnumIsAnError(t *testing.T) {
	imports := map[string]Exports{
		"geo": {Enums: map[string]ExportedEnum{"Shape": {Variants: []variantInfo{{Name: "Circle"}}, GoName: "geo_Shape"}}},
	}
	construction := &ast.FieldExpr{
		Target: &ast.FieldExpr{Target: &ast.IdentExpr{Name: "geo"}, Field: "Shape"},
		Field:  "Circle",
		Args:   []ast.StructLitField{},
	}
	sw := &ast.SwitchExpr{
		Subject: construction,
		Cases: []ast.SwitchCase{
			{EnumName: "Shape", Variant: "Circle", Body: &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 0}}}},
		},
	}
	f := mainFile(&ast.DiscardExpr{Value: sw}, &ast.IntLit{Value: 0})
	if _, err := CheckPackage([]*ast.File{f}, "", imports); err == nil {
		t.Fatal("expected an error: switch over a cross-package enum subject isn't supported yet")
	}
}

func TestCheckPackage_QualifiedFieldAccessOnConstructedStruct(t *testing.T) {
	imports := map[string]Exports{
		"geo": {Structs: map[string]ExportedStruct{
			"Point": {Fields: []fieldInfo{{Name: "x", Typ: "Int64"}, {Name: "y", Typ: "Int64"}}, GoName: "geo_Point"},
		}},
	}
	lit := &ast.StructLit{Qualifier: "geo", TypeName: "Point", Fields: []ast.StructLitField{
		{Name: "x", Value: &ast.IntLit{Value: 1}},
		{Name: "y", Value: &ast.IntLit{Value: 2}},
	}}
	access := &ast.FieldExpr{Target: lit, Field: "x"}
	f := mainFile(&ast.LetExpr{Name: "v", Value: access}, &ast.IntLit{Value: 0})
	if _, err := CheckPackage([]*ast.File{f}, "", imports); err != nil {
		t.Fatalf("CheckPackage() error: %v", err)
	}
	if access.ResolvedType != "Int64" || access.AmivmField != "x" {
		t.Fatalf("got ResolvedType=%q AmivmField=%q, want \"Int64\"/\"x\"", access.ResolvedType, access.AmivmField)
	}
}

// TestCheckPackage_ExportTypeStringRewritesNestedStructInsideCompoundType
// exercises exportTypeString's recursive case directly (not just the
// top-level struct name — module.go's own doc comment): a struct field
// typed List[Point] must export as "List(Qualified(<prefix>Point))", not
// the bare "List(Point)" an importer's own c.structs has no entry for.
func TestCheckPackage_ExportTypeStringRewritesNestedStructInsideCompoundType(t *testing.T) {
	f := &ast.File{Decls: []ast.TopLevelDecl{
		&ast.FuncDecl{Name: "main", ReturnType: nt("Int"), Body: &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 0}}}},
		&ast.StructDecl{Name: "Point", Fields: []ast.Param{{Name: "x", Type: nt("Int")}}},
		&ast.StructDecl{Name: "Path", Fields: []ast.Param{{Name: "points", Type: &ast.ListType{Elem: nt("Point")}}}},
		&ast.FuncDecl{
			Name: "MakePath", Params: []ast.Param{{Name: "points", Type: &ast.ListType{Elem: nt("Point")}}}, ReturnType: nt("Path"),
			Body: &ast.Block{Exprs: []ast.Expr{&ast.StructLit{TypeName: "Path", Fields: []ast.StructLitField{{Name: "points", Value: &ast.IdentExpr{Name: "points"}}}}}},
		},
	}}
	exports, err := CheckPackage([]*ast.File{f}, "geo_", nil)
	if err != nil {
		t.Fatalf("CheckPackage() error: %v", err)
	}
	pathFields := exports.Structs["Path"].Fields
	if len(pathFields) != 1 || pathFields[0].Typ != "List(Qualified(geo_Point))" {
		t.Fatalf("got Path.Fields %#v, want [{points List(Qualified(geo_Point))}]", pathFields)
	}
	makePathParams := exports.Funcs["MakePath"].Params
	if len(makePathParams) != 1 || makePathParams[0] != "List(Qualified(geo_Point))" {
		t.Fatalf("got MakePath.Params %#v, want [List(Qualified(geo_Point))]", makePathParams)
	}
}

// --- step 15: pipeline DX (amifl-spec.md section 9.1's stage-numbered
// diagnostic, section 13.8's tap/peek) ---

// TestCheck_PipeStageMismatchProducesStageNumberedError builds the same
// shape parser.parsePipeExpr produces for `data |> stageA |> stageB`
// (stageA: String -> Int, stageB expects String) by hand — sema doesn't
// care how PipeStage/PipeArgIndex/PipeChainLabels got set, only that
// checkCallArgs reads them off the call that actually receives the
// mismatched value.
func TestCheck_PipeStageMismatchProducesStageNumberedError(t *testing.T) {
	labels := []string{"data", "stageA", "stageB"}
	stageA := &ast.CallExpr{
		Callee: "stageA", Args: []ast.Expr{&ast.IdentExpr{Name: "data"}},
		PipeStage: 1, PipeArgIndex: 0, PipeChainLabels: labels,
	}
	stageB := &ast.CallExpr{
		Callee: "stageB", Args: []ast.Expr{stageA},
		PipeStage: 2, PipeArgIndex: 0, PipeChainLabels: labels,
	}
	f := &ast.File{Decls: []ast.TopLevelDecl{
		&ast.FuncDecl{
			Name: "stageA", Params: []ast.Param{{Name: "s", Type: nt("String")}}, ReturnType: nt("Int"),
			Body: &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 1}}},
		},
		&ast.FuncDecl{
			Name: "stageB", Params: []ast.Param{{Name: "n", Type: nt("String")}}, ReturnType: nt("String"),
			Body: &ast.Block{Exprs: []ast.Expr{&ast.IdentExpr{Name: "n"}}},
		},
		&ast.FuncDecl{
			Name: "main", ReturnType: nt("Int"),
			Body: &ast.Block{Exprs: []ast.Expr{
				&ast.LetExpr{Name: "data", Type: nt("String"), Value: &ast.StringLit{Value: "hi"}},
				&ast.DiscardExpr{Value: stageB},
				&ast.IntLit{Value: 0},
			}},
		},
	}}
	err := Check(f)
	if err == nil {
		t.Fatal("expected a pipeline type-mismatch error: stageA returns Int, stageB expects String")
	}
	msg := err.Error()
	for _, want := range []string{
		"pipeline type mismatch at stage 2 (stageB)",
		"pipeline: data |> stageA |> stageB",
		"stage 1 (stageA) outputs: Int64",
		"stage 2 (stageB) expects: String",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error message %q doesn't contain %q", msg, want)
		}
	}
}

// TestCheck_NonPipeArgMismatchKeepsThePlainMessage confirms an ordinary
// (non-pipe, PipeStage == 0) call's own type mismatch is entirely
// unaffected by checkExprPipeAware — still the plain "expected X, got Y"
// checkExpr already gave before step 15.
func TestCheck_NonPipeArgMismatchKeepsThePlainMessage(t *testing.T) {
	f := mainFile(&ast.CallExpr{Callee: "addNums", Args: []ast.Expr{&ast.StringLit{Value: "x"}, &ast.IntLit{Value: 2}}})
	f.Decls = append(f.Decls, &ast.FuncDecl{
		Name:       "addNums",
		Params:     []ast.Param{{Name: "a", Type: nt("Int")}, {Name: "b", Type: nt("Int")}},
		ReturnType: nt("Int"),
		Body:       &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 0}}},
	})
	err := Check(f)
	if err == nil {
		t.Fatal("expected a type mismatch error")
	}
	if strings.Contains(err.Error(), "pipeline type mismatch") {
		t.Fatalf("got pipeline-flavored error %q for a call that was never part of a `|>` chain", err.Error())
	}
}

func TestCheck_TapIsIdentityAndAcceptsAnyValueType(t *testing.T) {
	call := &ast.CallExpr{Callee: "tap", Args: []ast.Expr{&ast.IntLit{Value: 5}, &ast.StringLit{Value: "label"}}}
	f := mainFile(&ast.LetExpr{Name: "r", Value: call}, &ast.IdentExpr{Name: "r"})
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if call.ResolvedType != "Int64" {
		t.Fatalf("got ResolvedType %q, want Int64 (tap returns its own first argument's type)", call.ResolvedType)
	}
}

func TestCheck_TapRejectsNonStringLabel(t *testing.T) {
	call := &ast.CallExpr{Callee: "tap", Args: []ast.Expr{&ast.IntLit{Value: 5}, &ast.IntLit{Value: 1}}}
	f := mainFile(&ast.DiscardExpr{Value: call}, &ast.IntLit{Value: 0})
	if err := Check(f); err == nil {
		t.Fatal("expected an error: tap's label must be a String")
	}
}

func TestCheck_PeekIsIdentity(t *testing.T) {
	call := &ast.CallExpr{Callee: "peek", Args: []ast.Expr{&ast.StringLit{Value: "hi"}}}
	f := mainFile(&ast.LetExpr{Name: "r", Value: call}, &ast.IntLit{Value: 0})
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if call.ResolvedType != "String" {
		t.Fatalf("got ResolvedType %q, want String (peek returns its own argument's type unchanged)", call.ResolvedType)
	}
}

func TestCheck_TapRejectsUnitTypedValue(t *testing.T) {
	call := &ast.CallExpr{Callee: "tap", Args: []ast.Expr{printStr("x"), &ast.StringLit{Value: "label"}}}
	f := mainFile(&ast.DiscardExpr{Value: call}, &ast.IntLit{Value: 0})
	if err := Check(f); err == nil {
		t.Fatal("expected an error: tap's v must not be Unit-typed")
	}
}

// TestCheck_ReduceAcceptsTupleTypedAccumulator is a regression test (step
// 15's examples expansion, examples/run_length_encode.aml) for a bug found
// via that example: funcTypeParts split a closure's own encoded type
// string ("fn(Int64,Tuple(Int64,Int64))->Int64") with a plain
// strings.Split on "," — which over-splits a Tuple-typed parameter at the
// comma *inside* its own "Tuple(...)", so reduce wrongly rejected a
// closure whose accumulator is a Tuple2[Int,Int] even though every
// parameter/return type genuinely matched. Fixed by splitTopLevelCommas
// (depth-aware, mirroring mapKeyValueTypes' existing technique).
func TestCheck_ReduceAcceptsTupleTypedAccumulator(t *testing.T) {
	pair := func(a, b uint64) ast.Expr {
		return &ast.TupleLit{Elems: []ast.Expr{&ast.IntLit{Value: a}, &ast.IntLit{Value: b}}}
	}
	call := &ast.CallExpr{Callee: "reduce", Args: []ast.Expr{
		&ast.IdentExpr{Name: "pairs"},
		&ast.IdentExpr{Name: "zero"},
		&ast.IdentExpr{Name: "sumCounts"},
	}}
	f := mainFile(
		&ast.LetExpr{Name: "pairs", Value: &ast.ListLit{Elems: []ast.Expr{pair(1, 2), pair(3, 4)}}},
		&ast.LetExpr{Name: "zero", Type: tuple2t(nt("Int"), nt("Int")), Value: pair(0, 0)},
		&ast.LetExpr{Name: "sumCounts", Value: &ast.ClosureLit{
			Params: []ast.Param{
				{Name: "acc", Type: tuple2t(nt("Int"), nt("Int"))},
				{Name: "p", Type: tuple2t(nt("Int"), nt("Int"))},
			},
			ReturnType: tuple2t(nt("Int"), nt("Int")),
			Body:       &ast.Block{Exprs: []ast.Expr{&ast.IdentExpr{Name: "acc"}}},
		}},
		&ast.LetExpr{Name: "r", Value: call},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v (a Tuple2[Int,Int]-typed accumulator should type-check)", err)
	}
	if call.ResolvedType != "Tuple(Int64,Int64)" {
		t.Fatalf("got ResolvedType %q, want Tuple(Int64,Int64)", call.ResolvedType)
	}
}

// --- ex6: print/eprint/format/formatWith/exit (amifl-spec.md section
// 13.1) ---

func TestCheck_EprintAcceptsAnyConcreteValue(t *testing.T) {
	call := &ast.CallExpr{Callee: "eprint", Args: []ast.Expr{&ast.IntLit{Value: 5}}}
	f := mainFile(call, &ast.IntLit{Value: 0})
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if call.ResolvedType != unitType {
		t.Fatalf("got ResolvedType %q, want %q", call.ResolvedType, unitType)
	}
}

func TestCheck_EprintRejectsUnitTypedValue(t *testing.T) {
	call := &ast.CallExpr{Callee: "eprint", Args: []ast.Expr{printStr("x")}}
	f := mainFile(call, &ast.IntLit{Value: 0})
	if err := Check(f); err == nil {
		t.Fatal("expected an error: eprint's v must not be Unit-typed")
	}
}

func TestCheck_FormatReturnsString(t *testing.T) {
	call := &ast.CallExpr{Callee: "format", Args: []ast.Expr{&ast.BoolLit{Value: true}}}
	f := mainFile(&ast.LetExpr{Name: "s", Value: call}, &ast.IntLit{Value: 0})
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if call.ResolvedType != "String" {
		t.Fatalf("got ResolvedType %q, want String", call.ResolvedType)
	}
}

func TestCheck_FormatRejectsUnitTypedValue(t *testing.T) {
	call := &ast.CallExpr{Callee: "format", Args: []ast.Expr{printStr("x")}}
	f := mainFile(&ast.DiscardExpr{Value: call}, &ast.IntLit{Value: 0})
	if err := Check(f); err == nil {
		t.Fatal("expected an error: format's v must not be Unit-typed")
	}
}

func TestCheck_FormatWithReturnsStringAndRequiresStringTemplate(t *testing.T) {
	call := &ast.CallExpr{Callee: "formatWith", Args: []ast.Expr{&ast.StringLit{Value: "{}"}, &ast.IntLit{Value: 5}}}
	f := mainFile(&ast.LetExpr{Name: "s", Value: call}, &ast.IntLit{Value: 0})
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if call.ResolvedType != "String" {
		t.Fatalf("got ResolvedType %q, want String", call.ResolvedType)
	}

	bad := &ast.CallExpr{Callee: "formatWith", Args: []ast.Expr{&ast.IntLit{Value: 1}, &ast.IntLit{Value: 5}}}
	f2 := mainFile(&ast.DiscardExpr{Value: bad}, &ast.IntLit{Value: 0})
	if err := Check(f2); err == nil {
		t.Fatal("expected an error: formatWith's template must be a String")
	}
}

func TestCheck_ExitAcceptsIntReturnsUnit(t *testing.T) {
	call := &ast.CallExpr{Callee: "exit", Args: []ast.Expr{&ast.IntLit{Value: 1}}}
	f := mainFile(call, &ast.IntLit{Value: 0})
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if call.ResolvedType != unitType {
		t.Fatalf("got ResolvedType %q, want %q", call.ResolvedType, unitType)
	}
}

func TestCheck_ExitRejectsNonIntArg(t *testing.T) {
	call := &ast.CallExpr{Callee: "exit", Args: []ast.Expr{&ast.StringLit{Value: "1"}}}
	f := mainFile(call, &ast.IntLit{Value: 0})
	if err := Check(f); err == nil {
		t.Fatal("expected an error: exit's code must be Int")
	}
}

// TestCheck_ExitAsIfBranchFallbackRequiresUnitSiblingBranch documents ex6's
// deliberate scope cut (CLAUDE.md's ex6 design note, amifl-spec.md section
// 17.2): exit is plain Unit-typed, not a Never-like "fits any expected
// type" the way Go's panic does structurally, so it can stand alongside
// another Unit branch...
func TestCheck_ExitAsIfBranchFallbackRequiresUnitSiblingBranch(t *testing.T) {
	okBranch := &ast.Block{Exprs: []ast.Expr{printStr("ok")}}
	exitBranch := &ast.Block{Exprs: []ast.Expr{&ast.CallExpr{Callee: "exit", Args: []ast.Expr{&ast.IntLit{Value: 1}}}}}
	ifExpr := &ast.IfExpr{Cond: &ast.BoolLit{Value: true}, Then: okBranch, Else: exitBranch}
	f := mainFile(ifExpr, &ast.IntLit{Value: 0})
	if err := Check(f); err != nil {
		t.Fatalf("Check() error: %v (both branches are Unit-typed, so this should be fine)", err)
	}
}

// ...but not alongside a non-Unit branch (unlike Go's panic).
func TestCheck_ExitAsNonUnitIfBranchFallbackIsAnError(t *testing.T) {
	okBranch := &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 5}}}
	exitBranch := &ast.Block{Exprs: []ast.Expr{&ast.CallExpr{Callee: "exit", Args: []ast.Expr{&ast.IntLit{Value: 1}}}}}
	ifExpr := &ast.IfExpr{Cond: &ast.BoolLit{Value: true}, Then: okBranch, Else: exitBranch}
	f := mainFile(&ast.LetExpr{Name: "r", Value: ifExpr}, &ast.IntLit{Value: 0})
	if err := Check(f); err == nil {
		t.Fatal("expected an error: exit is Unit-typed, it can't stand in for an Int branch")
	}
}
