package codegen

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
				Name:               "main",
				ReturnType:         nt("Int"),
				ResolvedReturnType: "Int64",
				Body:               &ast.Block{Exprs: exprs},
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
		&ast.LetExpr{Name: "x", Token: "%x", ResolvedType: "Int64", Value: &ast.IntLit{Value: 42}},
		&ast.IdentExpr{Name: "x", Token: "%x", ResolvedType: "Int64"},
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
		&ast.LetExpr{Name: "x", Token: "%x", ResolvedType: "Int64", Value: &ast.IntLit{Value: 1}},
		&ast.AssignExpr{Name: "x", Token: "%x", Value: &ast.IntLit{Value: 2}},
		&ast.IdentExpr{Name: "x", Token: "%x", ResolvedType: "Int64"},
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
		&ast.LetExpr{Name: "x", Token: "%x", ResolvedType: "Float64", Value: &ast.FloatLit{Value: 5}},
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
		&ast.LetExpr{Name: "ok", Token: "%ok", ResolvedType: "Bool", Value: &ast.BoolLit{Value: true}},
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

func TestGenerate_BinaryExprEmitsTempAndInstruction(t *testing.T) {
	f := mainFile(
		&ast.BinaryExpr{Op: "+", Left: &ast.IntLit{Value: 1}, Right: &ast.IntLit{Value: 2}, ResolvedType: "Int64"},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{
		"VAR\t%amifl_tmp1\t^int64",
		"ADD\t%amifl_tmp1\t1\t2",
		"RET\t%amifl_tmp1",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
}

func TestGenerate_StringPlusEmitsConcat(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "s", Token: "%s", ResolvedType: "String", Value: &ast.BinaryExpr{
			Op: "+", Left: &ast.StringLit{Value: "a"}, Right: &ast.StringLit{Value: "b"}, ResolvedType: "String",
		}},
		&ast.IntLit{Value: 0},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if !strings.Contains(ir, `CONCAT	%amifl_tmp1	"a"	"b"`) {
		t.Errorf("generated IR missing CONCAT; got:\n%s", ir)
	}
}

func TestGenerate_ComparisonDeclaresBoolTemp(t *testing.T) {
	f := mainFile(
		&ast.BinaryExpr{Op: "<", Left: &ast.IntLit{Value: 1}, Right: &ast.IntLit{Value: 2}, ResolvedType: "Int64"},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if !strings.Contains(ir, "VAR\t%amifl_tmp1\t^bool") || !strings.Contains(ir, "LT\t%amifl_tmp1\t1\t2") {
		t.Errorf("generated IR missing bool-typed LT temp; got:\n%s", ir)
	}
}

func TestGenerate_UnaryMinusOfLiteralInlinesAsNegativeLiteral(t *testing.T) {
	f := mainFile(
		&ast.UnaryExpr{Op: "-", Operand: &ast.IntLit{Value: 5}, ResolvedType: "Int64"},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if !strings.Contains(ir, "RET\t-5") {
		t.Errorf("expected -5 to inline as a bare negative literal (no VAR/SUB); got:\n%s", ir)
	}
	if strings.Contains(ir, "SUB") {
		t.Errorf("did not expect a SUB instruction for negating a literal; got:\n%s", ir)
	}
}

func TestGenerate_DoubleUnaryMinusOfLiteralCollapses(t *testing.T) {
	f := mainFile(
		&ast.UnaryExpr{Op: "-", ResolvedType: "Int64", Operand: &ast.UnaryExpr{
			Op: "-", Operand: &ast.IntLit{Value: 5}, ResolvedType: "Int64",
		}},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if !strings.Contains(ir, "RET\t5") || strings.Contains(ir, "RET\t--5") {
		t.Errorf("expected -(-5) to collapse to a bare 5; got:\n%s", ir)
	}
}

func TestGenerate_UnaryMinusOfVariableEmitsSub(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "x", Token: "%x", ResolvedType: "Int64", Value: &ast.IntLit{Value: 5}},
		&ast.UnaryExpr{Op: "-", Operand: &ast.IdentExpr{Name: "x", Token: "%x", ResolvedType: "Int64"}, ResolvedType: "Int64"},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if !strings.Contains(ir, "SUB\t%amifl_tmp1\t0\t%x") {
		t.Errorf("expected negating a variable to emit SUB 0 %%x; got:\n%s", ir)
	}
}

func TestGenerate_BitwiseNotEmitsBnot(t *testing.T) {
	f := mainFile(
		&ast.UnaryExpr{Op: "~", Operand: &ast.IntLit{Value: 5}, ResolvedType: "Int64"},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if !strings.Contains(ir, "BNOT\t%amifl_tmp1\t5") {
		t.Errorf("generated IR missing BNOT; got:\n%s", ir)
	}
}

func TestGenerate_NestedBinaryExprChainsTemps(t *testing.T) {
	// 1 + 2 * 3 should compute the multiplication into one temp and then
	// the addition into a second temp that references it.
	f := mainFile(
		&ast.BinaryExpr{
			Op: "+", ResolvedType: "Int64",
			Left: &ast.IntLit{Value: 1},
			Right: &ast.BinaryExpr{
				Op: "*", ResolvedType: "Int64",
				Left: &ast.IntLit{Value: 2}, Right: &ast.IntLit{Value: 3},
			},
		},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{
		"MUL\t%amifl_tmp1\t2\t3",
		"ADD\t%amifl_tmp2\t1\t%amifl_tmp1",
		"RET\t%amifl_tmp2",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
}

func TestGenerate_ConstOperatorExpressionRegeneratesAtUseSite(t *testing.T) {
	// A const whose initializer is an operator expression has no runtime
	// storage of its own (CLAUDE.md's "確定した設計判断"): each reference
	// re-emits the same computation rather than reading a %ConstName var.
	sum := &ast.BinaryExpr{Op: "+", ResolvedType: "Int64", Left: &ast.IntLit{Value: 40}, Right: &ast.IntLit{Value: 2}}
	f := mainFile(
		&ast.IdentExpr{Name: "X", ResolvedType: "Int64", ConstValue: sum},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if strings.Contains(ir, "VAR\t%X") {
		t.Errorf("const should not get a VAR declaration; got:\n%s", ir)
	}
	if !strings.Contains(ir, "ADD\t%amifl_tmp1\t40\t2") || !strings.Contains(ir, "RET\t%amifl_tmp1") {
		t.Errorf("expected the const's operator expression regenerated inline; got:\n%s", ir)
	}
}

func TestGenerate_IfWithoutElseEmitsIfEndif(t *testing.T) {
	f := mainFile(
		&ast.IfExpr{
			Cond: &ast.BoolLit{Value: true},
			Then: &ast.Block{Exprs: []ast.Expr{
				&ast.CallExpr{Callee: "print", Args: []ast.Expr{&ast.StringLit{Value: "hi"}}},
			}},
		},
		&ast.IntLit{Value: 0},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{"IF\ttrue", `CALL	:	?fmt.Println	"hi"`, "ENDIF"} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
	if strings.Contains(ir, "ELSE") {
		t.Errorf("an else-less if should emit no ELSE; got:\n%s", ir)
	}
}

func TestGenerate_IfWithElseEmitsValueViaTemp(t *testing.T) {
	f := mainFile(
		&ast.IfExpr{
			Cond:         &ast.BoolLit{Value: true},
			Then:         &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 1}}},
			Else:         &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 2}}},
			ResolvedType: "Int64",
		},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{
		"VAR\t%amifl_tmp1\t^int64",
		"IF\ttrue",
		"SET\t%amifl_tmp1\t1",
		"ELSE",
		"SET\t%amifl_tmp1\t2",
		"ENDIF",
		"RET\t%amifl_tmp1",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
}

func TestGenerate_ElifEmitsNestedIfInsideElseNotAmivmElif(t *testing.T) {
	f := mainFile(
		&ast.IfExpr{
			Cond: &ast.BoolLit{Value: true},
			Then: &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 1}}},
			Else: &ast.IfExpr{
				Cond: &ast.BoolLit{Value: false},
				Then: &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 2}}},
				Else: &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 3}}},
			},
			ResolvedType: "Int64",
		},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if strings.Contains(ir, "ELIF") {
		t.Errorf("elif must desugar to ELSE + a nested IF, never AMIVM's ELIF; got:\n%s", ir)
	}
	// Exactly one temp declared and threaded through every branch (not one
	// per branch) — the nested if reuses the outer if's dest.
	if strings.Count(ir, "VAR\t%amifl_tmp1") != 1 {
		t.Errorf("expected exactly one VAR for the shared result temp; got:\n%s", ir)
	}
	if strings.Count(ir, "SET\t%amifl_tmp1") != 3 {
		t.Errorf("expected all three branches to SET the same temp; got:\n%s", ir)
	}
}

func TestGenerate_WhileLowersToLoopIfBreak(t *testing.T) {
	f := mainFile(
		&ast.WhileExpr{
			Cond: &ast.BoolLit{Value: true},
			Body: &ast.Block{Exprs: []ast.Expr{&ast.BreakExpr{}}},
		},
		&ast.IntLit{Value: 0},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{"LOOP", "NOT\t%amifl_tmp1\ttrue", "IF\t%amifl_tmp1", "BREAK", "ENDIF", "ENDLOOP"} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
	// The loop body's own break must appear after the condition-check
	// IF/ENDIF, i.e. twice total ("BREAK" appears once for the
	// condition-exit and once for the user's own break).
	if strings.Count(ir, "\tBREAK\n") != 2 {
		t.Errorf("expected 2 BREAKs (loop-exit + user break), got IR:\n%s", ir)
	}
}

func TestGenerate_ContinueEmitsContinue(t *testing.T) {
	f := mainFile(
		&ast.WhileExpr{
			Cond: &ast.BoolLit{Value: true},
			Body: &ast.Block{Exprs: []ast.Expr{&ast.ContinueExpr{}}},
		},
		&ast.IntLit{Value: 0},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if !strings.Contains(ir, "\tCONTINUE\n") {
		t.Errorf("generated IR missing CONTINUE; got:\n%s", ir)
	}
}

func TestGenerate_ShadowingLetGetsDistinctInternalNames(t *testing.T) {
	// Regression test for a real bug found via the full amivm -> go build
	// pipeline (CLAUDE.md's "確定した設計判断" for step 4): two `let`s
	// sharing the same emitted Go name — even though real block shadowing
	// makes that legal Go — broke amivm's unused-variable self-healing,
	// which assumes one declaration per name per function. Every `let`
	// must get its own Token, whether or not it shadows anything.
	f := mainFile(
		&ast.LetExpr{Name: "x", Token: "%x_1", ResolvedType: "Int64", Value: &ast.IntLit{Value: 1}},
		&ast.IfExpr{
			Cond: &ast.BoolLit{Value: true},
			Then: &ast.Block{Exprs: []ast.Expr{
				&ast.LetExpr{Name: "x", Token: "%x_2", ResolvedType: "Int64", Value: &ast.IntLit{Value: 2}},
			}},
		},
		&ast.IdentExpr{Name: "x", Token: "%x_1", ResolvedType: "Int64"},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if !strings.Contains(ir, "VAR\t%x_1\t^int64") || !strings.Contains(ir, "VAR\t%x_2\t^int64") {
		t.Errorf("expected distinct VAR declarations for x_1 and x_2; got:\n%s", ir)
	}
	if strings.Contains(ir, "%x\t") || strings.Contains(ir, "%x\n") {
		t.Errorf("did not expect the bare (unsuffixed) name %%x anywhere; got:\n%s", ir)
	}
}

func TestGenerate_DiscardOfNonUnitIfStillRunsSideEffectsInsideIt(t *testing.T) {
	// `_ = if c { print("x"); 1 } else { 2 }` — the if's own type is
	// Int64 (not Unit), so it's not a CallExpr and the old genDiscardStmt
	// would have silently dropped it (and the print inside it) entirely.
	f := mainFile(
		&ast.DiscardExpr{Value: &ast.IfExpr{
			Cond: &ast.BoolLit{Value: true},
			Then: &ast.Block{Exprs: []ast.Expr{
				&ast.CallExpr{Callee: "print", Args: []ast.Expr{&ast.StringLit{Value: "x"}}},
				&ast.IntLit{Value: 1},
			}},
			Else:         &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 2}}},
			ResolvedType: "Int64",
		}},
		&ast.IntLit{Value: 0},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if !strings.Contains(ir, `CALL	:	?fmt.Println	"x"`) {
		t.Errorf("expected the print inside the discarded if's branch to still run; got:\n%s", ir)
	}
	// The branch values themselves are never captured anywhere (nothing
	// reads the if's result), so no VAR/SET should be emitted for them.
	if strings.Contains(ir, "SET") {
		t.Errorf("discarding the if's value should emit no SET at all; got:\n%s", ir)
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

func TestGenerate_TopLevelFuncWithParamsUsesDollarTokens(t *testing.T) {
	f := &ast.File{Decls: []ast.TopLevelDecl{
		&ast.FuncDecl{
			Name:               "main",
			ReturnType:         nt("Int"),
			ResolvedReturnType: "Int64",
			Body:               &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 0}}},
		},
		&ast.FuncDecl{
			Name:               "add",
			Params:             []ast.Param{{Name: "a", Type: nt("Int"), ResolvedType: "Int64"}, {Name: "b", Type: nt("Int"), ResolvedType: "Int64"}},
			ReturnType:         nt("Int"),
			ResolvedReturnType: "Int64",
			Body: &ast.Block{Exprs: []ast.Expr{
				&ast.BinaryExpr{Op: "+", ResolvedType: "Int64",
					Left:  &ast.IdentExpr{Name: "a", ResolvedType: "Int64", Token: "$1"},
					Right: &ast.IdentExpr{Name: "b", ResolvedType: "Int64", Token: "$2"},
				},
			}},
		},
	}}
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{
		"FUNC\t!add\t^int64\t^int64\t:\t^int64",
		"ADD\t%amifl_tmp1\t$1\t$2",
		"RET\t%amifl_tmp1",
		"ENDFUNC",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
	// No VAR is ever declared for a parameter — $N is implicit via FUNC's
	// own header, unlike a `let`.
	if strings.Contains(ir, "VAR\t$1") || strings.Contains(ir, "VAR\t$2") {
		t.Errorf("did not expect a VAR declaration for a parameter; got:\n%s", ir)
	}
}

func TestGenerate_CallToTopLevelFuncEmitsBangToken(t *testing.T) {
	f := &ast.File{Decls: []ast.TopLevelDecl{
		&ast.FuncDecl{
			Name:               "main",
			ReturnType:         nt("Int"),
			ResolvedReturnType: "Int64",
			Body: &ast.Block{Exprs: []ast.Expr{
				&ast.CallExpr{Callee: "helper", ResolvedType: "Int64"},
			}},
		},
		&ast.FuncDecl{
			Name:               "helper",
			ReturnType:         nt("Int"),
			ResolvedReturnType: "Int64",
			Body:               &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 7}}},
		},
	}}
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if !strings.Contains(ir, "CALL\t%amifl_tmp1\t:\t!helper\n") {
		t.Errorf("expected a value-producing CALL to !helper; got:\n%s", ir)
	}
}

func TestGenerate_UnitReturningFuncHasNoResultTypeAndBareRet(t *testing.T) {
	f := &ast.File{Decls: []ast.TopLevelDecl{
		&ast.FuncDecl{
			Name:               "main",
			ReturnType:         nt("Int"),
			ResolvedReturnType: "Int64",
			Body:               &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 0}}},
		},
		&ast.FuncDecl{
			Name:               "log",
			Params:             []ast.Param{{Name: "msg", Type: nt("String"), ResolvedType: "String"}},
			ReturnType:         nt("Unit"),
			ResolvedReturnType: unitType,
			Body: &ast.Block{Exprs: []ast.Expr{
				&ast.CallExpr{Callee: "print", Args: []ast.Expr{&ast.IdentExpr{Name: "msg", ResolvedType: "String", Token: "$1"}}},
			}},
		},
	}}
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if !strings.Contains(ir, "FUNC\t!log\t^string\t:\n") {
		t.Errorf("expected a Unit-returning FUNC header with no result type; got:\n%s", ir)
	}
	if !strings.Contains(ir, "\tRET\n") {
		t.Errorf("expected a bare RET for a Unit-returning function; got:\n%s", ir)
	}
}

func TestGenerate_ClosureLitEmitsFntypeVarClos(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "square", Token: "%square_1", Value: &ast.ClosureLit{
			Params:             []ast.Param{{Name: "x", Type: nt("Int"), ResolvedType: "Int64"}},
			ReturnType:         nt("Int"),
			ResolvedReturnType: "Int64",
			ResolvedType:       "fn(Int64)->Int64",
			Body: &ast.Block{Exprs: []ast.Expr{
				&ast.BinaryExpr{Op: "*", ResolvedType: "Int64",
					Left:  &ast.IdentExpr{Name: "x", ResolvedType: "Int64", Token: "&1-1"},
					Right: &ast.IdentExpr{Name: "x", ResolvedType: "Int64", Token: "&1-1"},
				},
			}},
		}},
		&ast.CallExpr{Callee: "square", CalleeToken: "%square_1", ResolvedType: "Int64", Args: []ast.Expr{&ast.IntLit{Value: 5}}},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{
		"FNTYPE\t^AmiflFunc1\t^int64\t:\t^int64",
		"VAR\t%square_1\t^AmiflFunc1",
		"CLOS\t%square_1\t^int64\t:\t^int64",
		"MUL\t%amifl_tmp1\t&1-1\t&1-1",
		"RET\t%amifl_tmp1",
		"ENDCLOS",
		"CALL\t%amifl_tmp2\t:\t%square_1\t5",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
}

func TestGenerate_ClosureCapturesOuterLetTokenDirectly(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "base", Token: "%base_1", ResolvedType: "Int64", Value: &ast.IntLit{Value: 10}},
		&ast.LetExpr{Name: "addBase", Token: "%addBase_2", Value: &ast.ClosureLit{
			Params:             []ast.Param{{Name: "x", Type: nt("Int"), ResolvedType: "Int64"}},
			ReturnType:         nt("Int"),
			ResolvedReturnType: "Int64",
			ResolvedType:       "fn(Int64)->Int64",
			Body: &ast.Block{Exprs: []ast.Expr{
				&ast.BinaryExpr{Op: "+", ResolvedType: "Int64",
					Left:  &ast.IdentExpr{Name: "x", ResolvedType: "Int64", Token: "&1-1"},
					Right: &ast.IdentExpr{Name: "base", ResolvedType: "Int64", Token: "%base_1"},
				},
			}},
		}},
		&ast.IntLit{Value: 0},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if !strings.Contains(ir, "ADD\t%amifl_tmp1\t&1-1\t%base_1") {
		t.Errorf("expected the closure body to reference the captured outer %%base_1 token directly; got:\n%s", ir)
	}
}

func TestGenerate_StructDeclEmitsSttype(t *testing.T) {
	f := mainFile(&ast.IntLit{Value: 0})
	f.Decls = append(f.Decls, &ast.StructDecl{
		Name: "Point",
		Fields: []ast.Param{
			{Name: "x", Type: nt("Int"), ResolvedType: "Int64"},
			{Name: "y", Type: nt("Int"), ResolvedType: "Int64"},
		},
	})
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{
		"STTYPE\t^Point",
		"FIELD\t>x\t^int64",
		"FIELD\t>y\t^int64",
		"ENDSTTYPE",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
}

func pointStructLit(x, y uint64) *ast.StructLit {
	return &ast.StructLit{
		TypeName:     "Point",
		ResolvedType: "Point",
		Fields: []ast.StructLitField{
			{Name: "x", Value: &ast.IntLit{Value: x}},
			{Name: "y", Value: &ast.IntLit{Value: y}},
		},
	}
}

func TestGenerate_StructLitEmitsVarAndFset(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "p", Token: "%p_1", ResolvedType: "Point", Value: pointStructLit(1, 2)},
		&ast.IntLit{Value: 0},
	)
	f.Decls = append(f.Decls, &ast.StructDecl{
		Name: "Point",
		Fields: []ast.Param{
			{Name: "x", Type: nt("Int"), ResolvedType: "Int64"},
			{Name: "y", Type: nt("Int"), ResolvedType: "Int64"},
		},
	})
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{
		"VAR\t%amifl_tmp1\t^Point",
		"FSET\t%amifl_tmp1\t>x\t1",
		"FSET\t%amifl_tmp1\t>y\t2",
		"VAR\t%p_1\t^Point",
		"SET\t%p_1\t%amifl_tmp1",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
}

func TestGenerate_StructFieldAccessEmitsFget(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "p", Token: "%p_1", ResolvedType: "Point", Value: pointStructLit(1, 2)},
		&ast.FieldExpr{Target: &ast.IdentExpr{Name: "p", ResolvedType: "Point", Token: "%p_1"}, Field: "x", ResolvedType: "Int64", AmivmField: "x"},
	)
	f.Decls = append(f.Decls, &ast.StructDecl{
		Name: "Point",
		Fields: []ast.Param{
			{Name: "x", Type: nt("Int"), ResolvedType: "Int64"},
			{Name: "y", Type: nt("Int"), ResolvedType: "Int64"},
		},
	})
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if !strings.Contains(ir, "FGET\t%amifl_tmp2\t%p_1\t>x") {
		t.Errorf("expected FGET reading field x from %%p_1; got:\n%s", ir)
	}
}

func TestGenerate_TupleLitEmitsSynthesizedSttypeWithNumberedFields(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "t", Token: "%t_1", ResolvedType: "Tuple(Int64,String)", Value: &ast.TupleLit{
			ResolvedType: "Tuple(Int64,String)",
			Elems:        []ast.Expr{&ast.IntLit{Value: 1}, &ast.StringLit{Value: "a"}},
		}},
		&ast.IntLit{Value: 0},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{
		"STTYPE\t^AmiflTuple1",
		"FIELD\t>F0\t^int64",
		"FIELD\t>F1\t^string",
		"ENDSTTYPE",
		"VAR\t%amifl_tmp1\t^AmiflTuple1",
		"FSET\t%amifl_tmp1\t>F0\t1",
		"FSET\t%amifl_tmp1\t>F1\t\"a\"",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
}

func TestGenerate_SameShapedTuplesShareOneSttype(t *testing.T) {
	tupType := "Tuple(Int64,Int64)"
	f := mainFile(
		&ast.LetExpr{Name: "a", Token: "%a_1", ResolvedType: tupType, Value: &ast.TupleLit{
			ResolvedType: tupType,
			Elems:        []ast.Expr{&ast.IntLit{Value: 1}, &ast.IntLit{Value: 2}},
		}},
		&ast.LetExpr{Name: "b", Token: "%b_2", ResolvedType: tupType, Value: &ast.TupleLit{
			ResolvedType: tupType,
			Elems:        []ast.Expr{&ast.IntLit{Value: 3}, &ast.IntLit{Value: 4}},
		}},
		&ast.IntLit{Value: 0},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if strings.Count(ir, "STTYPE\t^AmiflTuple1") != 1 {
		t.Errorf("expected exactly one STTYPE for the shared tuple shape; got:\n%s", ir)
	}
	if !strings.Contains(ir, "VAR\t%a_1\t^AmiflTuple1") || !strings.Contains(ir, "VAR\t%b_2\t^AmiflTuple1") {
		t.Errorf("expected both tuples to use the same synthesized type; got:\n%s", ir)
	}
}

func TestGenerate_TupleFieldAccessEmitsFgetWithSynthesizedFieldName(t *testing.T) {
	tupType := "Tuple(Int64,String)"
	f := mainFile(
		&ast.LetExpr{Name: "t", Token: "%t_1", ResolvedType: tupType, Value: &ast.TupleLit{
			ResolvedType: tupType,
			Elems:        []ast.Expr{&ast.IntLit{Value: 1}, &ast.StringLit{Value: "a"}},
		}},
		&ast.DiscardExpr{Value: &ast.FieldExpr{
			Target:       &ast.IdentExpr{Name: "t", ResolvedType: tupType, Token: "%t_1"},
			Field:        "1",
			ResolvedType: "String",
			AmivmField:   "F1",
		}},
		&ast.IntLit{Value: 0},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if !strings.Contains(ir, "FGET\t%amifl_tmp2\t%t_1\t>F1") {
		t.Errorf("expected FGET reading field F1 from %%t_1; got:\n%s", ir)
	}
}

func TestGenerate_ListLitEmitsSltypeSlmakeAset(t *testing.T) {
	listType := "List(Int64)"
	f := mainFile(
		&ast.LetExpr{Name: "xs", Token: "%xs_1", ResolvedType: listType, Value: &ast.ListLit{
			ResolvedType: listType,
			Elems:        []ast.Expr{&ast.IntLit{Value: 1}, &ast.IntLit{Value: 2}, &ast.IntLit{Value: 3}},
		}},
		&ast.IntLit{Value: 0},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{
		"SLTYPE\t^AmiflList1\t^int64",
		"VAR\t%amifl_tmp1\t^AmiflList1",
		"SLMAKE\t%amifl_tmp1\t^AmiflList1\t3",
		"ASET\t%amifl_tmp1\t0\t1",
		"ASET\t%amifl_tmp1\t1\t2",
		"ASET\t%amifl_tmp1\t2\t3",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
}

func TestGenerate_ArrayLitEmitsArtypeVarAsetNoSlmake(t *testing.T) {
	arrType := "Array(Int64;3)"
	f := mainFile(
		&ast.LetExpr{Name: "xs", Token: "%xs_1", ResolvedType: arrType, Value: &ast.ListLit{
			ResolvedType: arrType,
			Elems:        []ast.Expr{&ast.IntLit{Value: 1}, &ast.IntLit{Value: 2}, &ast.IntLit{Value: 3}},
		}},
		&ast.IntLit{Value: 0},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{
		"ARTYPE\t^AmiflArray1\t^int64\t3",
		"VAR\t%amifl_tmp1\t^AmiflArray1",
		"ASET\t%amifl_tmp1\t0\t1",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
	if strings.Contains(ir, "SLMAKE") {
		t.Errorf("an Array literal must not emit SLMAKE; got:\n%s", ir)
	}
}

func TestGenerate_NestedArrayEmitsNestedArtype(t *testing.T) {
	innerType := "Array(Int64;3)"
	outerType := "Array(" + innerType + ";2)"
	f := mainFile(
		&ast.LetExpr{Name: "grid", Token: "%grid_1", ResolvedType: outerType, Value: &ast.ListLit{
			ResolvedType: outerType,
			Elems: []ast.Expr{
				&ast.ListLit{ResolvedType: innerType, Elems: []ast.Expr{&ast.IntLit{Value: 1}, &ast.IntLit{Value: 2}, &ast.IntLit{Value: 3}}},
				&ast.ListLit{ResolvedType: innerType, Elems: []ast.Expr{&ast.IntLit{Value: 4}, &ast.IntLit{Value: 5}, &ast.IntLit{Value: 6}}},
			},
		}},
		&ast.IntLit{Value: 0},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{
		"ARTYPE\t^AmiflArray1\t^int64\t3",
		"ARTYPE\t^AmiflArray2\t^AmiflArray1\t2",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
}

func TestGenerate_IndexExprEmitsAget(t *testing.T) {
	listType := "List(Int64)"
	f := mainFile(
		&ast.LetExpr{Name: "xs", Token: "%xs_1", ResolvedType: listType, Value: &ast.ListLit{ResolvedType: listType, Elems: []ast.Expr{&ast.IntLit{Value: 1}}}},
		&ast.IndexExpr{
			Target:       &ast.IdentExpr{Name: "xs", ResolvedType: listType, Token: "%xs_1"},
			Index:        &ast.IntLit{Value: 0},
			ResolvedType: "Int64",
		},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if !strings.Contains(ir, "AGET\t%amifl_tmp2\t%xs_1\t0") {
		t.Errorf("expected AGET reading xs[0]; got:\n%s", ir)
	}
}

func TestGenerate_IndexAssignEmitsAset(t *testing.T) {
	listType := "List(Int64)"
	f := mainFile(
		&ast.LetExpr{Name: "xs", Token: "%xs_1", ResolvedType: listType, Value: &ast.ListLit{ResolvedType: listType, Elems: []ast.Expr{&ast.IntLit{Value: 1}}}},
		&ast.IndexAssignExpr{
			Target: &ast.IdentExpr{Name: "xs", ResolvedType: listType, Token: "%xs_1"},
			Index:  &ast.IntLit{Value: 0},
			Value:  &ast.IntLit{Value: 9},
		},
		&ast.IntLit{Value: 0},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if !strings.Contains(ir, "ASET\t%xs_1\t0\t9") {
		t.Errorf("expected ASET writing xs[0]=9 directly into %%xs_1 (no copy for a plain identifier target); got:\n%s", ir)
	}
}

func TestGenerate_ChainedIndexAssignEmitsReadModifyWriteBack(t *testing.T) {
	innerType := "List(Int64)"
	outerType := "List(" + innerType + ")"
	f := mainFile(
		&ast.LetExpr{Name: "matrix", Token: "%matrix_1", ResolvedType: outerType, Value: &ast.ListLit{ResolvedType: outerType}},
		&ast.IndexAssignExpr{
			Target: &ast.IndexExpr{
				Target:       &ast.IdentExpr{Name: "matrix", ResolvedType: outerType, Token: "%matrix_1"},
				Index:        &ast.IntLit{Value: 0},
				ResolvedType: innerType,
			},
			Index: &ast.IntLit{Value: 0},
			Value: &ast.IntLit{Value: 9},
		},
		&ast.IntLit{Value: 0},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	// Reads matrix[0] into a temp, ASETs 9 into that temp, then writes the
	// mutated temp back into matrix[0] — see collections.go's
	// genIndexAssignStmt doc comment for why a single direct ASET into
	// matrix[0] isn't always correct (it happens to work for a List, but
	// this must generalize to an Array intermediate too).
	for _, want := range []string{
		"AGET\t%amifl_tmp2\t%matrix_1\t0",
		"ASET\t%amifl_tmp2\t0\t9",
		"ASET\t%matrix_1\t0\t%amifl_tmp2",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
}

func TestGenerate_SliceExprOmittedBoundsEmitUnderscore(t *testing.T) {
	listType := "List(Int64)"
	f := mainFile(
		&ast.LetExpr{Name: "xs", Token: "%xs_1", ResolvedType: listType, Value: &ast.ListLit{ResolvedType: listType, Elems: []ast.Expr{&ast.IntLit{Value: 1}}}},
		&ast.DiscardExpr{Value: &ast.SliceExpr{
			Target:       &ast.IdentExpr{Name: "xs", ResolvedType: listType, Token: "%xs_1"},
			ResolvedType: listType,
		}},
		&ast.IntLit{Value: 0},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if !strings.Contains(ir, "SLICE\t%amifl_tmp2\t%xs_1\t_\t_") {
		t.Errorf("expected SLICE with both bounds omitted as '_'; got:\n%s", ir)
	}
}

func TestGenerate_ForStmtEmitsIncrementFirstLoop(t *testing.T) {
	listType := "List(Int64)"
	f := mainFile(
		&ast.LetExpr{Name: "xs", Token: "%xs_1", ResolvedType: listType, Value: &ast.ListLit{ResolvedType: listType, Elems: []ast.Expr{&ast.IntLit{Value: 1}}}},
		&ast.ForExpr{
			Items:    &ast.IdentExpr{Name: "xs", ResolvedType: listType, Token: "%xs_1"},
			Body:     &ast.Block{Exprs: []ast.Expr{&ast.CallExpr{Callee: "print", Args: []ast.Expr{&ast.StringLit{Value: "hi"}}}}},
			ElemType: "Int64",
			VarToken: "%x_2",
		},
		&ast.IntLit{Value: 0},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	// xs's own ListLit already consumed amifl_tmp1, so the for-loop's own
	// temps start at amifl_tmp2 (len result), amifl_tmp3 (idx), amifl_tmp4
	// (bounds-check bool).
	for _, want := range []string{
		"CALL\t%amifl_tmp2\t:\t?len\t%xs_1",
		"SET\t%amifl_tmp3\t-1",
		"LOOP",
		"ADD\t%amifl_tmp3\t%amifl_tmp3\t1",
		"GTE\t%amifl_tmp4\t%amifl_tmp3\t%amifl_tmp2",
		"AGET\t%x_2\t%xs_1\t%amifl_tmp3",
		"ENDLOOP",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
	// The increment must come before the length/bounds check inside the
	// loop, not after the body — CLAUDE.md's "過去に踏まれた地雷" #3.
	loopIdx := strings.Index(ir, "LOOP\n")
	incIdx := strings.Index(ir, "ADD\t%amifl_tmp3\t%amifl_tmp3\t1")
	gteIdx := strings.Index(ir, "GTE\t%amifl_tmp4")
	if !(loopIdx < incIdx && incIdx < gteIdx) {
		t.Errorf("expected LOOP, then increment, then bounds check, in that order; got:\n%s", ir)
	}
}

// statusEnumDecl builds `enum Status { Ok  Retry(delay: Int) }` — the
// step 8 test fixture, mirroring the struct tests' pointStructDecl (an
// ast.EnumDecl already carries every field's ResolvedType, exactly like
// ast.StructDecl.Fields does, since codegen tests operate on
// already-sema-resolved AST throughout this file).
func statusEnumDecl() *ast.EnumDecl {
	return &ast.EnumDecl{
		Name: "Status",
		Variants: []ast.EnumVariant{
			{Name: "Ok"},
			{Name: "Retry", Fields: []ast.Param{{Name: "delay", Type: nt("Int"), ResolvedType: "Int64"}}},
		},
	}
}

func TestGenerate_EnumDeclEmitsSttypeWithTagAndQualifiedFields(t *testing.T) {
	f := mainFile(&ast.IntLit{Value: 0})
	f.Decls = append(f.Decls, statusEnumDecl())
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{
		"STTYPE\t^Status",
		"FIELD\t>Tag\t^int",
		"FIELD\t>Retry_delay\t^int64",
		"ENDSTTYPE",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
}

func TestGenerate_EnumVariantConstructionEmitsFsetTagAndFields(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "s", Token: "%s_1", ResolvedType: "Status", Value: &ast.FieldExpr{
			Target: &ast.IdentExpr{Name: "Status"}, Field: "Retry",
			Args:          []ast.StructLitField{{Name: "delay", Value: &ast.IntLit{Value: 5}}},
			ResolvedType:  "Status",
			IsEnumVariant: true,
			VariantIndex:  1,
		}},
		&ast.IntLit{Value: 0},
	)
	f.Decls = append(f.Decls, statusEnumDecl())
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{
		"VAR\t%amifl_tmp1\t^Status",
		"FSET\t%amifl_tmp1\t>Tag\t1",
		"FSET\t%amifl_tmp1\t>Retry_delay\t5",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
}

func TestGenerate_SwitchValueEmitsIfElseChainTestingTag(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "n", Token: "%n_1", ResolvedType: "Int64", Value: &ast.SwitchExpr{
			Subject:      &ast.IdentExpr{Name: "s", ResolvedType: "Status", Token: "%s_1"},
			ResolvedType: "Int64",
			EnumName:     "Status",
			Cases: []ast.SwitchCase{
				{Variant: "Ok", VariantIndex: 0, Body: &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 1}}}},
				{
					Variant: "Retry", VariantIndex: 1,
					Bindings: []string{"delay"}, BindingTokens: []string{"%delay_2"}, BindingTypes: []string{"Int64"},
					Body: &ast.Block{Exprs: []ast.Expr{&ast.IdentExpr{Name: "delay", ResolvedType: "Int64", Token: "%delay_2"}}},
				},
			},
		}},
		&ast.IntLit{Value: 0},
	)
	f.Decls = append(f.Decls, statusEnumDecl())
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{
		"FGET\t%amifl_tmp2\t%s_1\t>Tag",
		"EQ\t%amifl_tmp3\t%amifl_tmp2\t0",
		"IF\t%amifl_tmp3",
		"SET\t%amifl_tmp1\t1",
		"ELSE",
		"EQ\t%amifl_tmp5\t%amifl_tmp4\t1",
		"FGET\t%delay_2\t%s_1\t>Retry_delay",
		"SET\t%amifl_tmp1\t%delay_2",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
	// Exhaustive (no Default), 2 cases: the first case's IF gets an ELSE
	// leading into the second case's IF, but the second (last) case's IF
	// must NOT get a trailing ELSE — sema guarantees its Tag always
	// matches once the first case's didn't, mirroring an if-expression
	// with no else. So exactly one fewer ELSE than IF.
	ifCount := strings.Count(ir, "IF\t")
	elseCount := strings.Count(ir, "ELSE\n")
	if ifCount-elseCount != 1 {
		t.Errorf("expected exactly one fewer ELSE than IF (no dangling final ELSE), got %d IF and %d ELSE in:\n%s", ifCount, elseCount, ir)
	}
}

func TestGenerate_SwitchStmtWithDefaultEmitsFinalElse(t *testing.T) {
	f := mainFile(
		&ast.SwitchExpr{
			Subject:  &ast.IdentExpr{Name: "s", ResolvedType: "Status", Token: "%s_1"},
			EnumName: "Status",
			Cases: []ast.SwitchCase{
				{
					Variant: "Retry", VariantIndex: 1,
					Bindings: []string{"delay"}, BindingTokens: []string{"%delay_2"}, BindingTypes: []string{"Int64"},
					Body: &ast.Block{Exprs: []ast.Expr{&ast.CallExpr{Callee: "print", Args: []ast.Expr{&ast.StringLit{Value: "retry"}}}}},
				},
			},
			Default: &ast.Block{Exprs: []ast.Expr{&ast.CallExpr{Callee: "print", Args: []ast.Expr{&ast.StringLit{Value: "other"}}}}},
		},
		&ast.IntLit{Value: 0},
	)
	f.Decls = append(f.Decls, statusEnumDecl())
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{
		"EQ\t%amifl_tmp2\t%amifl_tmp1\t1",
		"IF\t%amifl_tmp2",
		"FGET\t%delay_2\t%s_1\t>Retry_delay",
		"CALL\t:\t?fmt.Println\t\"retry\"",
		"ELSE",
		"CALL\t:\t?fmt.Println\t\"other\"",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
}

func TestGenerate_ForYieldEmitsPreallocatedSlmakeAndAset(t *testing.T) {
	listType := "List(Int64)"
	f := mainFile(
		&ast.LetExpr{Name: "xs", Token: "%xs_1", ResolvedType: listType, Value: &ast.ListLit{ResolvedType: listType, Elems: []ast.Expr{&ast.IntLit{Value: 1}}}},
		&ast.LetExpr{Name: "ys", Token: "%ys_2", ResolvedType: listType, Value: &ast.ForExpr{
			Items:        &ast.IdentExpr{Name: "xs", ResolvedType: listType, Token: "%xs_1"},
			Yield:        &ast.BinaryExpr{Op: "*", Left: &ast.IdentExpr{Name: "x", ResolvedType: "Int64", Token: "%x_3"}, Right: &ast.IntLit{Value: 2}, ResolvedType: "Int64"},
			ElemType:     "Int64",
			VarToken:     "%x_3",
			ResolvedType: listType,
		}},
		&ast.IntLit{Value: 0},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	// xs's own ListLit consumes amifl_tmp1; the yield-for's own temps start
	// at amifl_tmp2 (len result).
	if !strings.Contains(ir, "CALL\t%amifl_tmp2\t:\t?len\t%xs_1") {
		t.Errorf("generated IR missing the length CALL; got:\n%s", ir)
	}
	if !strings.Contains(ir, "SLMAKE\t%amifl_tmp3\t^AmiflList1\t%amifl_tmp2") {
		t.Errorf("generated IR missing a length-preallocated SLMAKE; got:\n%s", ir)
	}
	if !strings.Contains(ir, "AGET\t%x_3\t%xs_1\t%amifl_tmp4") {
		t.Errorf("generated IR missing AGET into the loop variable; got:\n%s", ir)
	}
	if !strings.Contains(ir, "ASET\t%amifl_tmp3\t%amifl_tmp4\t%amifl_tmp6") {
		t.Errorf("generated IR missing ASET writing the yielded value into the preallocated list; got:\n%s", ir)
	}
	// Increment-first, matching genForStmt's own precedent.
	loopIdx := strings.Index(ir, "LOOP\n")
	incIdx := strings.Index(ir, "ADD\t%amifl_tmp4\t%amifl_tmp4\t1")
	gteIdx := strings.Index(ir, "GTE\t%amifl_tmp5")
	if !(loopIdx >= 0 && loopIdx < incIdx && incIdx < gteIdx) {
		t.Errorf("expected LOOP, then increment, then bounds check, in that order; got:\n%s", ir)
	}
}

func TestGenerate_ForYieldDiscardedInStatementPositionStillRuns(t *testing.T) {
	// Yield must be Int64-typed here, not Unit (sema now rejects a
	// Unit-typed `yield` value — see TestCheck_ForYieldUnitTypedIsAnError
	// — since genValue has no Go value to collect for one); a call to a
	// side-effecting, Int64-returning top-level fn is what demonstrates
	// "still runs when the resulting list is discarded" instead.
	listType := "List(Int64)"
	f := &ast.File{Decls: []ast.TopLevelDecl{
		&ast.FuncDecl{
			Name: "main", ReturnType: nt("Int"), ResolvedReturnType: "Int64",
			Body: &ast.Block{Exprs: []ast.Expr{
				&ast.LetExpr{Name: "xs", Token: "%xs_1", ResolvedType: listType, Value: &ast.ListLit{ResolvedType: listType, Elems: []ast.Expr{&ast.IntLit{Value: 1}}}},
				&ast.DiscardExpr{Value: &ast.ForExpr{
					Items:        &ast.IdentExpr{Name: "xs", ResolvedType: listType, Token: "%xs_1"},
					Yield:        &ast.CallExpr{Callee: "sideEffecting", ResolvedType: "Int64"},
					ElemType:     "Int64",
					VarToken:     "%x_2",
					ResolvedType: listType,
				}},
				&ast.IntLit{Value: 0},
			}},
		},
		&ast.FuncDecl{
			Name: "sideEffecting", ReturnType: nt("Int"), ResolvedReturnType: "Int64",
			Body: &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 1}}},
		},
	}}
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if !strings.Contains(ir, "CALL\t%amifl_tmp6\t:\t!sideEffecting") {
		t.Errorf("expected the yield expression's call to still run when discarded; got:\n%s", ir)
	}
	if !strings.Contains(ir, "SLMAKE") {
		t.Errorf("expected SLMAKE even though the resulting list is discarded; got:\n%s", ir)
	}
}

func TestGenerate_SetLitEmitsMptypeMpmakeMset(t *testing.T) {
	setType := "Set(Int64)"
	f := mainFile(
		&ast.LetExpr{Name: "s", Token: "%s_1", ResolvedType: setType, Value: &ast.SetOrMapLit{
			ResolvedType: setType,
			Elems:        []ast.Expr{&ast.IntLit{Value: 1}, &ast.IntLit{Value: 2}, &ast.IntLit{Value: 3}},
		}},
		&ast.IntLit{Value: 0},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{
		"MPTYPE\t^AmiflSet1\t^int64\t^bool",
		"VAR\t%amifl_tmp1\t^AmiflSet1",
		"MPMAKE\t%amifl_tmp1\t^AmiflSet1",
		"MSET\t%amifl_tmp1\t1\ttrue",
		"MSET\t%amifl_tmp1\t2\ttrue",
		"MSET\t%amifl_tmp1\t3\ttrue",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
}

func TestGenerate_MapLitEmitsMptypeMpmakeMset(t *testing.T) {
	mapType := "Map(String,Int64)"
	f := mainFile(
		&ast.LetExpr{Name: "m", Token: "%m_1", ResolvedType: mapType, Value: &ast.SetOrMapLit{
			ResolvedType: mapType,
			Entries: []ast.MapLitEntry{
				{Key: &ast.StringLit{Value: "a"}, Value: &ast.IntLit{Value: 1}},
				{Key: &ast.StringLit{Value: "b"}, Value: &ast.IntLit{Value: 2}},
			},
		}},
		&ast.IntLit{Value: 0},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{
		"MPTYPE\t^AmiflMap1\t^string\t^int64",
		"VAR\t%amifl_tmp1\t^AmiflMap1",
		"MPMAKE\t%amifl_tmp1\t^AmiflMap1",
		"MSET\t%amifl_tmp1\t\"a\"\t1",
		"MSET\t%amifl_tmp1\t\"b\"\t2",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
}

// TestGenerate_SetTypeAndMapTypeShareNoMptypeEvenWhenStructurallyIdentical
// pins down setGoTypeName's documented "keyed under Set[T]'s own canonical
// string, never Map[T,Bool]'s" choice: a Set[Int] and a Map[Int,Bool] would
// produce the identical Go type (map[int64]bool), but each still mints its
// own separate MPTYPE rather than sharing one.
func TestGenerate_SetTypeAndMapTypeShareNoMptypeEvenWhenStructurallyIdentical(t *testing.T) {
	setType := "Set(Int64)"
	mapType := "Map(Int64,Bool)"
	f := mainFile(
		&ast.LetExpr{Name: "s", Token: "%s_1", ResolvedType: setType, Value: &ast.SetOrMapLit{ResolvedType: setType, Elems: []ast.Expr{&ast.IntLit{Value: 1}}}},
		&ast.LetExpr{Name: "m", Token: "%m_2", ResolvedType: mapType, Value: &ast.SetOrMapLit{ResolvedType: mapType, Entries: []ast.MapLitEntry{
			{Key: &ast.IntLit{Value: 1}, Value: &ast.BoolLit{Value: true}},
		}}},
		&ast.IntLit{Value: 0},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{
		"MPTYPE\t^AmiflSet1\t^int64\t^bool",
		"MPTYPE\t^AmiflMap1\t^int64\t^bool",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
}

func TestGenerate_ForOverSetUsesMpkeysThenAget(t *testing.T) {
	setType := "Set(Int64)"
	f := mainFile(
		&ast.LetExpr{Name: "s", Token: "%s_1", ResolvedType: setType, Value: &ast.SetOrMapLit{ResolvedType: setType, Elems: []ast.Expr{&ast.IntLit{Value: 1}}}},
		&ast.ForExpr{
			Items:     &ast.IdentExpr{Name: "s", ResolvedType: setType, Token: "%s_1"},
			ItemsType: setType,
			Body:      &ast.Block{Exprs: []ast.Expr{&ast.CallExpr{Callee: "print", Args: []ast.Expr{&ast.StringLit{Value: "hi"}}}}},
			ElemType:  "Int64",
			VarToken:  "%x_2",
		},
		&ast.IntLit{Value: 0},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	// s's own SetOrMapLit consumes amifl_tmp1; the for-loop's own temps
	// start at amifl_tmp2 (len), amifl_tmp3 (the MPKEYS-collected keys
	// list — a Set isn't index-addressable, so it's never AGET'd directly).
	for _, want := range []string{
		"SLTYPE\t^AmiflList1\t^int64",
		"VAR\t%amifl_tmp3\t^AmiflList1",
		"MPKEYS\t%amifl_tmp3\t%s_1",
		"CALL\t%amifl_tmp2\t:\t?len\t%amifl_tmp3",
		"SET\t%amifl_tmp4\t-1",
		"AGET\t%x_2\t%amifl_tmp3\t%amifl_tmp4",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
}

func TestGenerate_ForTwoVarsOverMapEmitsMpkeysAgetMget(t *testing.T) {
	mapType := "Map(String,Int64)"
	f := mainFile(
		&ast.LetExpr{Name: "m", Token: "%m_1", ResolvedType: mapType, Value: &ast.SetOrMapLit{ResolvedType: mapType, Entries: []ast.MapLitEntry{
			{Key: &ast.StringLit{Value: "a"}, Value: &ast.IntLit{Value: 1}},
		}}},
		&ast.ForExpr{
			Var2:      "v",
			Items:     &ast.IdentExpr{Name: "m", ResolvedType: mapType, Token: "%m_1"},
			ItemsType: mapType,
			Body:      &ast.Block{Exprs: []ast.Expr{&ast.CallExpr{Callee: "print", Args: []ast.Expr{&ast.StringLit{Value: "hi"}}}}},
			ElemType:  "String",
			VarToken:  "%k_2",
			Var2Type:  "Int64",
			Var2Token: "%v_3",
		},
		&ast.IntLit{Value: 0},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{
		"SLTYPE\t^AmiflList1\t^string",
		"MPKEYS\t%amifl_tmp3\t%m_1",
		"CALL\t%amifl_tmp2\t:\t?len\t%amifl_tmp3",
		"VAR\t%k_2\t^string",
		"AGET\t%k_2\t%amifl_tmp3\t%amifl_tmp4",
		"VAR\t%v_3\t^int64",
		"MGET\t%v_3\t%m_1\t%k_2",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
}
