package codegen

import (
	"strings"
	"testing"

	"github.com/amisonnet8/amifl/internal/ast"
)

func mainFile(exprs ...ast.Expr) *ast.File {
	return &ast.File{
		Decls: []ast.TopLevelDecl{
			&ast.FuncDecl{
				Name:               "main",
				ReturnType:         "Int",
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
			ReturnType:         "Int",
			ResolvedReturnType: "Int64",
			Body:               &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 0}}},
		},
		&ast.FuncDecl{
			Name:               "add",
			Params:             []ast.Param{{Name: "a", Type: "Int", ResolvedType: "Int64"}, {Name: "b", Type: "Int", ResolvedType: "Int64"}},
			ReturnType:         "Int",
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
			ReturnType:         "Int",
			ResolvedReturnType: "Int64",
			Body: &ast.Block{Exprs: []ast.Expr{
				&ast.CallExpr{Callee: "helper", ResolvedType: "Int64"},
			}},
		},
		&ast.FuncDecl{
			Name:               "helper",
			ReturnType:         "Int",
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
			ReturnType:         "Int",
			ResolvedReturnType: "Int64",
			Body:               &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 0}}},
		},
		&ast.FuncDecl{
			Name:               "log",
			Params:             []ast.Param{{Name: "msg", Type: "String", ResolvedType: "String"}},
			ReturnType:         "Unit",
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
			Params:             []ast.Param{{Name: "x", Type: "Int", ResolvedType: "Int64"}},
			ReturnType:         "Int",
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
			Params:             []ast.Param{{Name: "x", Type: "Int", ResolvedType: "Int64"}},
			ReturnType:         "Int",
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
