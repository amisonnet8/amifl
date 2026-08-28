package sema

import (
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
		&ast.CallExpr{Callee: "add", Args: []ast.Expr{&ast.IntLit{Value: 1}, &ast.IntLit{Value: 2}}},
	)
	f.Decls = append(f.Decls, &ast.FuncDecl{
		Name:       "add",
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
	f := mainFile(&ast.CallExpr{Callee: "add", Args: []ast.Expr{&ast.IntLit{Value: 1}}})
	f.Decls = append(f.Decls, &ast.FuncDecl{
		Name:       "add",
		Params:     []ast.Param{{Name: "a", Type: nt("Int")}, {Name: "b", Type: nt("Int")}},
		ReturnType: nt("Int"),
		Body:       &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 0}}},
	})
	if err := Check(f); err == nil {
		t.Fatal("expected an error for calling add with the wrong number of arguments")
	}
}

func TestCheck_CallWrongArgTypeIsAnError(t *testing.T) {
	f := mainFile(&ast.CallExpr{Callee: "add", Args: []ast.Expr{&ast.StringLit{Value: "x"}, &ast.IntLit{Value: 2}}})
	f.Decls = append(f.Decls, &ast.FuncDecl{
		Name:       "add",
		Params:     []ast.Param{{Name: "a", Type: nt("Int")}, {Name: "b", Type: nt("Int")}},
		ReturnType: nt("Int"),
		Body:       &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 0}}},
	})
	if err := Check(f); err == nil {
		t.Fatal("expected an error for calling add with a String where an Int is expected")
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

func TestCheck_MainWithParamsIsAnError(t *testing.T) {
	f := &ast.File{Decls: []ast.TopLevelDecl{
		&ast.FuncDecl{
			Name:       "main",
			Params:     []ast.Param{{Name: "x", Type: nt("Int")}},
			ReturnType: nt("Int"),
			Body:       &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 0}}},
		},
	}}
	if err := Check(f); err == nil {
		t.Fatal("expected an error for fn main taking parameters in step 5")
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

func TestCheck_ClosureLitWithTypeAnnotationIsAnError(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "square", Type: nt("Int"), Value: &ast.ClosureLit{
			Params:     []ast.Param{{Name: "x", Type: nt("Int")}},
			ReturnType: nt("Int"),
			Body:       &ast.Block{Exprs: []ast.Expr{&ast.IdentExpr{Name: "x"}}},
		}},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for a closure-valued `let` carrying its own type annotation")
	}
}

func TestCheck_ClosureLitAsCallArgumentIsAnError(t *testing.T) {
	// Step 5 only supports a ClosureLit as a `let`'s direct value — not as
	// a call argument (no higher-order functions yet).
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

func TestCheck_IndexAssignThroughFieldIsAnError(t *testing.T) {
	f := mainFile(
		&ast.IndexAssignExpr{
			Target: &ast.FieldExpr{Target: &ast.IdentExpr{Name: "p"}, Field: "xs"},
			Index:  &ast.IntLit{Value: 0},
			Value:  &ast.IntLit{Value: 9},
		},
		&ast.IntLit{Value: 0},
	)
	if err := Check(f); err == nil {
		t.Fatal("expected an error for an index-assignment target that reaches through a struct field")
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
