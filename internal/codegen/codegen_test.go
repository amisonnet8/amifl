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

// TestGenerate_RangeExprConstructsNormalizedStruct covers ex2's `a..b` /
// `a..=b` (ast.RangeExpr) — the runtime representation is always a
// half-open [From,To) AmiflRange struct, so the inclusive form's own To
// bound is bumped by one at construction time (structs.go's
// genRangeValue) rather than carrying an "inclusive" flag any further.
func TestGenerate_RangeExprConstructsNormalizedStruct(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "r", Token: "%r_1", ResolvedType: "Range", Value: &ast.RangeExpr{From: &ast.IntLit{Value: 0}, To: &ast.IntLit{Value: 10}, Inclusive: true}},
		&ast.IntLit{Value: 0},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{
		"STTYPE\t^AmiflRange",
		"FIELD\t>From\t^int64",
		"FIELD\t>To\t^int64",
		"ENDSTTYPE",
		"ADD\t%amifl_tmp1\t10\t1", // Inclusive: To bumped by one
		"FSET\t%amifl_tmp2\t>From\t0",
		"FSET\t%amifl_tmp2\t>To\t%amifl_tmp1",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
}

// TestGenerate_ForOverRangeCountsInInt64 covers ex2's `for i in a..b { ...
// }` — unlike List/Array/Set/Map (prepareForIteration/
// emitIndexLoopHeader, `^int`-typed throughout), the loop counter here is
// `^int64` (ast.RangeExpr's Int64-only bounds) and *is* the user-visible
// loop variable itself, with no separate AGET/index needed at all.
func TestGenerate_ForOverRangeCountsInInt64(t *testing.T) {
	f := mainFile(
		&ast.ForExpr{
			Items:     &ast.RangeExpr{From: &ast.IntLit{Value: 0}, To: &ast.IntLit{Value: 5}},
			ItemsType: "Range",
			Body:      &ast.Block{Exprs: []ast.Expr{&ast.CallExpr{Callee: "print", Args: []ast.Expr{&ast.StringLit{Value: "hi"}}}}},
			ElemType:  "Int64",
			VarToken:  "%i_2",
		},
		&ast.IntLit{Value: 0},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{
		"FGET\t%amifl_tmp2\t%amifl_tmp1\t>From",
		"FGET\t%amifl_tmp3\t%amifl_tmp1\t>To",
		"VAR\t%i_2\t^int64",
		"SUB\t%i_2\t%amifl_tmp2\t1",
		"LOOP",
		"ADD\t%i_2\t%i_2\t1",
		"GTE\t%amifl_tmp4\t%i_2\t%amifl_tmp3",
		"ENDLOOP",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
	// The increment must come before the bounds check, not after the body
	// (CLAUDE.md's "過去に踏まれた地雷" #3), exactly like every other
	// `for` shape's loop header.
	loopIdx := strings.Index(ir, "LOOP\n")
	incIdx := strings.Index(ir, "ADD\t%i_2\t%i_2\t1")
	gteIdx := strings.Index(ir, "GTE\t%amifl_tmp4")
	if !(loopIdx < incIdx && incIdx < gteIdx) {
		t.Errorf("expected LOOP, then increment, then bounds check, in that order; got:\n%s", ir)
	}
}

// TestGenerate_ForYieldOverRangeClampsNegativeLength covers ex2's `for i
// in a..b yield expr` — SLMAKE's length must never go negative (Go's
// `make([]T, n)` panics on n < 0), so To-From is clamped to >= 0 via
// selectValue before the cast down to `^int`, even though From > To
// itself is a perfectly valid (empty) range, never a type/runtime error.
func TestGenerate_ForYieldOverRangeClampsNegativeLength(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{
			Name: "xs", Token: "%xs_1", ResolvedType: "List(Int64)",
			Value: &ast.ForExpr{
				Items:        &ast.RangeExpr{From: &ast.IntLit{Value: 0}, To: &ast.IntLit{Value: 5}},
				ItemsType:    "Range",
				Yield:        &ast.IdentExpr{Name: "i", ResolvedType: "Int64", Token: "%i_2"},
				ElemType:     "Int64",
				VarToken:     "%i_2",
				ResolvedType: "List(Int64)",
			},
		},
		&ast.IntLit{Value: 0},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{
		"SUB\t%amifl_tmp4\t%amifl_tmp3\t%amifl_tmp2", // rawCount = To - From
		"GT\t%amifl_tmp5\t%amifl_tmp4\t0",            // selectValue's clamp condition
		"IF\t%amifl_tmp5",
		"SET\t%amifl_tmp6\t%amifl_tmp4",
		"ELSE",
		"SET\t%amifl_tmp6\t0",
		"ENDIF",
		"CALL\t%amifl_tmp7\t:\t?int\t%amifl_tmp6",
		"SLMAKE\t%amifl_tmp8\t^AmiflList1\t%amifl_tmp7",
		"ASET\t%amifl_tmp8\t%amifl_tmp9\t%i_2",
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

// TestGenerate_MainWithListStringArgsBridgesArgv covers amifl-spec.md
// section 14's `fn main(args: List[String]) -> Int` form: the `!main`
// wrapper must build the arg list via amiflrt.Args() and pass it into
// amifl_main, rather than the zero-arg form's plain `CALL %code : !amifl_
// main` (still covered separately by TestGenerate_HelloWorld above).
func TestGenerate_MainWithListStringArgsBridgesArgv(t *testing.T) {
	f := &ast.File{Decls: []ast.TopLevelDecl{
		&ast.FuncDecl{
			Name:               "main",
			Params:             []ast.Param{{Name: "args", Type: &ast.ListType{Elem: nt("String")}, ResolvedType: "List(String)"}},
			ReturnType:         nt("Int"),
			ResolvedReturnType: "Int64",
			Body:               &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 0}}},
		},
	}}

	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	for _, want := range []string{
		"SLTYPE\t^AmiflList1\t^string",
		"FUNC\t!amifl_main\t^AmiflList1\t:\t^int64",
		"VAR\t%args\t^AmiflList1",
		"CALL\t%args\t:\t?amiflrt.Args",
		"CALL\t%code\t:\t!amifl_main\t%args",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}

	// The synthesized List[String] Go type must be minted exactly once —
	// resolveGoType("List(String)") is called a second time while
	// building the !main wrapper (to look up the already-minted type
	// name), and that second call must be a pure cache hit rather than
	// emitting a second SLTYPE that lands after prog.typeHeader has
	// already been flushed into the output (this codebase's step-13
	// "STTYPE nested type declaration" lesson, generalized — see
	// codegen.go's GenerateProgram comment at the call site).
	if n := strings.Count(ir, "SLTYPE"); n != 1 {
		t.Errorf("expected exactly one SLTYPE declaration, got %d; IR:\n%s", n, ir)
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

// TestGenerate_ClosureLitWithTupleParamUsesSynthesizedTupleGoType is a
// regression test (step 15's examples expansion, examples/
// run_length_encode.aml) for a bug found via that example: a ClosureLit's
// own params/return were assumed always-scalar (step 5's original scope)
// and genClosureLitInto looked them up directly in goTypeNames, which has
// no entry at all for a synthesized Tuple type — a closure passed to
// reduce whose accumulator is a Tuple2[...] (perfectly legal since step 11
// lets a List/Array element's own type, which may be compound, flow
// through as a closure param) crashed codegen with "unknown type". Fixed
// by routing through resolveGoType, same as every other type site.
func TestGenerate_ClosureLitWithTupleParamUsesSynthesizedTupleGoType(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "sumCounts", Token: "%sumCounts_1", Value: &ast.ClosureLit{
			Params: []ast.Param{
				{Name: "acc", Type: nt("Int"), ResolvedType: "Int64"},
				{Name: "p", Type: nt("Tuple2"), ResolvedType: "Tuple(Int64,Int64)"},
			},
			ReturnType:         nt("Int"),
			ResolvedReturnType: "Int64",
			ResolvedType:       "fn(Int64,Tuple(Int64,Int64))->Int64",
			Body: &ast.Block{Exprs: []ast.Expr{
				&ast.IdentExpr{Name: "acc", ResolvedType: "Int64", Token: "&1-1"},
			}},
		}},
		&ast.IntLit{Value: 0},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{
		"STTYPE\t^AmiflTuple1",
		"FNTYPE\t^AmiflFunc1\t^int64\t^AmiflTuple1\t:\t^int64",
		"VAR\t%sumCounts_1\t^AmiflFunc1",
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

// TestGenerate_TwoClosureLitsOfSameShapeShareOneFuncType locks in ex3's
// load-bearing change to genClosureLitInto (funcGoTypeName, replacing step
// 5's original always-fresh newFuncTypeDecl call): two closure literals of
// the identical Func shape must compile to the *same* named Go function
// type, not two structurally-identical-but-distinct ones — Go requires two
// named function types to be identical (not just alike) for one to be
// assignable to the other, so this is correctness-critical the moment a
// Func-typed value can flow between two different closures of the same
// shape (a parameter, a reassignment, ...), not merely an optimization.
func TestGenerate_TwoClosureLitsOfSameShapeShareOneFuncType(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "square", Token: "%square_1", Value: &ast.ClosureLit{
			Params:             []ast.Param{{Name: "x", Type: nt("Int"), ResolvedType: "Int64"}},
			ReturnType:         nt("Int"),
			ResolvedReturnType: "Int64",
			ResolvedType:       "fn(Int64)->Int64",
			Body: &ast.Block{Exprs: []ast.Expr{
				&ast.IdentExpr{Name: "x", ResolvedType: "Int64", Token: "&1-1"},
			}},
		}},
		&ast.LetExpr{Name: "cube", Token: "%cube_2", Value: &ast.ClosureLit{
			Params:             []ast.Param{{Name: "x", Type: nt("Int"), ResolvedType: "Int64"}},
			ReturnType:         nt("Int"),
			ResolvedReturnType: "Int64",
			ResolvedType:       "fn(Int64)->Int64",
			Body: &ast.Block{Exprs: []ast.Expr{
				&ast.IdentExpr{Name: "x", ResolvedType: "Int64", Token: "&1-1"},
			}},
		}},
		&ast.IntLit{Value: 0},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if strings.Count(ir, "FNTYPE\t^AmiflFunc1\t^int64\t:\t^int64") != 1 {
		t.Errorf("expected exactly one FNTYPE declaration shared by both closures; got:\n%s", ir)
	}
	if strings.Count(ir, "FNTYPE") != 1 {
		t.Errorf("expected no second, distinct FNTYPE for the second closure; got:\n%s", ir)
	}
	for _, want := range []string{
		"VAR\t%square_1\t^AmiflFunc1",
		"VAR\t%cube_2\t^AmiflFunc1",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
}

// TestGenerate_PipeInlineClosureMintsClosureThenCallsThroughItsToken covers
// ex4's calleeToken branch: a CallExpr with InlineClosure set (the shape
// parser.parsePipeRHS produces for `x |> fn(v) -> R {...}`) mints the
// closure into a fresh temp via genClosureLitInto — VAR+CLOS...ENDCLOS,
// exactly as a `let`-bound closure would — and then calls through that
// same temp token, rather than through any pre-existing binding.
func TestGenerate_PipeInlineClosureMintsClosureThenCallsThroughItsToken(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "a", Token: "%a_1", ResolvedType: "Int64", Value: &ast.CallExpr{
			InlineClosure: &ast.ClosureLit{
				Params:             []ast.Param{{Name: "v", Type: nt("Int"), ResolvedType: "Int64"}},
				ReturnType:         nt("Int"),
				ResolvedReturnType: "Int64",
				ResolvedType:       "fn(Int64)->Int64",
				Body: &ast.Block{Exprs: []ast.Expr{
					&ast.BinaryExpr{Op: "+", ResolvedType: "Int64",
						Left:  &ast.IdentExpr{Name: "v", ResolvedType: "Int64", Token: "&1-1"},
						Right: &ast.IntLit{Value: 1},
					},
				}},
			},
			Args:         []ast.Expr{&ast.IntLit{Value: 5}},
			ResolvedType: "Int64",
		}},
		&ast.IntLit{Value: 0},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{
		"VAR\t%amifl_tmp1\t^AmiflFunc1",
		"CLOS\t%amifl_tmp1\t^int64\t:\t^int64",
		"ENDCLOS",
		// The closure body's own temp (the ADD result) is minted first —
		// amifl_tmp2 — so the CALL result that follows ENDCLOS lands on
		// amifl_tmp3, not amifl_tmp2.
		"CALL\t%amifl_tmp3\t:\t%amifl_tmp1\t5",
		"SET\t%a_1\t%amifl_tmp3",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
}

// TestGenerate_PipeInlineClosureSharesFuncTypeWithOrdinaryClosureLit
// confirms the ex3 shared, deduplicated funcGoTypeName cache (locked in by
// TestGenerate_TwoClosureLitsOfSameShapeShareOneFuncType for two `let`-bound
// closures) applies identically when one of the two producers is instead an
// ex4 inline pipe closure reached through calleeToken rather than
// genLetStmt — both routes fall through to the same genClosureLitInto, so
// there is only one code path here to potentially diverge, but this locks
// in that it doesn't.
func TestGenerate_PipeInlineClosureSharesFuncTypeWithOrdinaryClosureLit(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "square", Token: "%square_1", Value: &ast.ClosureLit{
			Params:             []ast.Param{{Name: "x", Type: nt("Int"), ResolvedType: "Int64"}},
			ReturnType:         nt("Int"),
			ResolvedReturnType: "Int64",
			ResolvedType:       "fn(Int64)->Int64",
			Body: &ast.Block{Exprs: []ast.Expr{
				&ast.IdentExpr{Name: "x", ResolvedType: "Int64", Token: "&1-1"},
			}},
		}},
		&ast.DiscardExpr{Value: &ast.CallExpr{
			InlineClosure: &ast.ClosureLit{
				Params:             []ast.Param{{Name: "v", Type: nt("Int"), ResolvedType: "Int64"}},
				ReturnType:         nt("Int"),
				ResolvedReturnType: "Int64",
				ResolvedType:       "fn(Int64)->Int64",
				Body: &ast.Block{Exprs: []ast.Expr{
					&ast.IdentExpr{Name: "v", ResolvedType: "Int64", Token: "&1-1"},
				}},
			},
			Args:         []ast.Expr{&ast.IntLit{Value: 5}},
			ResolvedType: "Int64",
		}},
		&ast.IntLit{Value: 0},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if strings.Count(ir, "FNTYPE") != 1 {
		t.Errorf("expected exactly one FNTYPE shared by both closures; got:\n%s", ir)
	}
}

// TestGenerate_TopLevelFnReferencedAsValueEmitsFuncval covers ex3's
// genFuncRefValue: a bare reference to a top-level `fn` (sema's
// resolveIdentExpr sets IsFuncRef, leaving FuncRefCallee "" — codegen
// derives "!"+name itself, exactly like calleeToken() does for an
// ordinary call).
func TestGenerate_TopLevelFnReferencedAsValueEmitsFuncval(t *testing.T) {
	f := &ast.File{Decls: []ast.TopLevelDecl{
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
		&ast.FuncDecl{
			Name:               "main",
			ReturnType:         nt("Int"),
			ResolvedReturnType: "Int64",
			Body: &ast.Block{Exprs: []ast.Expr{
				&ast.LetExpr{Name: "f", Token: "%f_1", ResolvedType: "fn(Int64,Int64)->Int64", Value: &ast.IdentExpr{
					Name: "add", ResolvedType: "fn(Int64,Int64)->Int64", IsFuncRef: true,
				}},
				&ast.IntLit{Value: 0},
			}},
		},
	}}
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if !strings.Contains(ir, "FUNCVAL\t%amifl_tmp1\t!add") {
		t.Errorf("expected FUNCVAL against the derived \"!add\" callname; got:\n%s", ir)
	}
	if !strings.Contains(ir, "SET\t%f_1\t%amifl_tmp1") {
		t.Errorf("expected the FUNCVAL result to be SET into the `let`'s own token; got:\n%s", ir)
	}
}

// TestGenerate_ExternBindReferencedAsValueEmitsFuncvalWithResolvedCallee
// covers genFuncRefValue's other branch: FuncRefCallee already holds the
// fully-resolved "?alias.GoName" callname (sema's resolveIdentExpr, mirroring
// CallExpr.CalleeToken's identical convention for an extern plain-callee
// bind), so codegen must use it verbatim rather than deriving "!"+Name.
func TestGenerate_ExternBindReferencedAsValueEmitsFuncvalWithResolvedCallee(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "f", Token: "%f_1", ResolvedType: "fn(String)->String", Value: &ast.IdentExpr{
			Name: "ToUpperCased", ResolvedType: "fn(String)->String", IsFuncRef: true, FuncRefCallee: "?strs.ToUpper",
		}},
		&ast.IntLit{Value: 0},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if !strings.Contains(ir, "FUNCVAL\t%amifl_tmp1\t?strs.ToUpper") {
		t.Errorf("expected FUNCVAL against the pre-resolved \"?strs.ToUpper\" callname; got:\n%s", ir)
	}
}

// TestGenerate_FuncTypedParamResolvesToSharedFuncGoType covers a
// FUNC header's own parameter type going through resolveGoType's new
// isFuncType dispatch (structs.go) — a Func-typed parameter must resolve
// to the identical funcGoTypeName-cached type a closure literal of the
// same shape would.
func TestGenerate_FuncTypedParamResolvesToSharedFuncGoType(t *testing.T) {
	f := &ast.File{Decls: []ast.TopLevelDecl{
		&ast.FuncDecl{
			Name:               "apply",
			Params:             []ast.Param{{Name: "f", Type: nt("_"), ResolvedType: "fn(Int64)->Int64"}, {Name: "x", Type: nt("Int"), ResolvedType: "Int64"}},
			ReturnType:         nt("Int"),
			ResolvedReturnType: "Int64",
			Body: &ast.Block{Exprs: []ast.Expr{
				&ast.CallExpr{Callee: "f", CalleeToken: "$1", ResolvedType: "Int64", Args: []ast.Expr{
					&ast.IdentExpr{Name: "x", ResolvedType: "Int64", Token: "$2"},
				}},
			}},
		},
		&ast.FuncDecl{
			Name:               "main",
			ReturnType:         nt("Int"),
			ResolvedReturnType: "Int64",
			Body:               &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 0}}},
		},
	}}
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if !strings.Contains(ir, "FUNC\t!apply\t^AmiflFunc1\t^int64\t:\t^int64") {
		t.Errorf("expected apply's FUNC header to declare f's parameter as the synthesized Func Go type; got:\n%s", ir)
	}
	if !strings.Contains(ir, "FNTYPE\t^AmiflFunc1\t^int64\t:\t^int64") {
		t.Errorf("expected exactly one FNTYPE minted for fn(Int64)->Int64; got:\n%s", ir)
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

func TestGenerate_IsErrorEmitsNeqNil(t *testing.T) {
	call := &ast.CallExpr{
		Callee: "isError", Builtin: "isError", ResolvedType: "Bool",
		Args:     []ast.Expr{&ast.IdentExpr{Name: "e", ResolvedType: "Error", Token: "%e_1"}},
		ArgTypes: []string{"Error"},
	}
	f := mainFile(&ast.LetExpr{Name: "ok", Token: "%ok_1", ResolvedType: "Bool", Value: call}, &ast.IntLit{Value: 0})
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{"VAR\t%amifl_tmp1\t^bool", "NEQ\t%amifl_tmp1\t%e_1\tnil"} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
}

func TestGenerate_TapEmitsGenericCallAndReturnsItsFirstArg(t *testing.T) {
	call := &ast.CallExpr{
		Callee: "tap", Builtin: "tap", ResolvedType: "Int64",
		Args:     []ast.Expr{&ast.IntLit{Value: 5}, &ast.StringLit{Value: "label"}},
		ArgTypes: []string{"Int64", "String"},
	}
	f := mainFile(&ast.LetExpr{Name: "r", Token: "%r_1", ResolvedType: "Int64", Value: call}, &ast.IntLit{Value: 0})
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{"VAR\t%amifl_tmp1\t^int64", "CALL\t%amifl_tmp1\t:\t?amiflrt.Tap\t^int64\t:\t5\t\"label\""} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
}

func TestGenerate_PeekEmitsGenericCallAndReturnsItsArg(t *testing.T) {
	call := &ast.CallExpr{
		Callee: "peek", Builtin: "peek", ResolvedType: "String",
		Args:     []ast.Expr{&ast.StringLit{Value: "hi"}},
		ArgTypes: []string{"String"},
	}
	f := mainFile(&ast.LetExpr{Name: "r", Token: "%r_1", ResolvedType: "String", Value: call}, &ast.IntLit{Value: 0})
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{"VAR\t%amifl_tmp1\t^string", "CALL\t%amifl_tmp1\t:\t?amiflrt.Peek\t^string\t:\t\"hi\""} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
}

func TestGenerate_CastEmitsTypeConversionCall(t *testing.T) {
	call := &ast.CallExpr{
		Callee: "cast", Builtin: "cast", ResolvedType: "Float64",
		Args:     []ast.Expr{&ast.IntLit{Value: 7}},
		ArgTypes: []string{"Int64"},
	}
	f := mainFile(&ast.LetExpr{Name: "x", Token: "%x_1", ResolvedType: "Float64", Value: call}, &ast.IntLit{Value: 0})
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{"VAR\t%amifl_tmp1\t^float64", "CALL\t%amifl_tmp1\t:\t?float64\t7"} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
}

func TestGenerate_ParseNarrowsResultAndBuildsTuple2(t *testing.T) {
	call := &ast.CallExpr{
		Callee: "parse", Builtin: "parse", ResolvedType: "Tuple(Int8,Error)", ResolvedTypeArg: "Int8",
		Args:     []ast.Expr{&ast.StringLit{Value: "5"}},
		ArgTypes: []string{"String"},
	}
	f := mainFile(&ast.LetExpr{Name: "r", Token: "%r_1", ResolvedType: "Tuple(Int8,Error)", Value: call}, &ast.IntLit{Value: 0})
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	// strconv.ParseInt always returns a raw int64 regardless of T's own
	// width (parseTargetInfo) — Int8 needs one extra CALL-as-conversion to
	// narrow amifl_tmp1 down before it goes into the tuple's F0 field.
	for _, want := range []string{
		"VAR\t%amifl_tmp1\t^int64",
		"VAR\t%amifl_tmp2\t^error",
		"CALL\t%amifl_tmp1\t%amifl_tmp2\t:\t?strconv.ParseInt\t\"5\"\t10\t8",
		"CALL\t%amifl_tmp3\t:\t?int8\t%amifl_tmp1",
		"FSET\t%amifl_tmp4\t>F0\t%amifl_tmp3",
		"FSET\t%amifl_tmp4\t>F1\t%amifl_tmp2",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
}

func TestGenerate_TryOperatorEmitsEarlyReturnTuple(t *testing.T) {
	tryExpr := &ast.TryExpr{
		Value:    &ast.IdentExpr{Name: "r", ResolvedType: "Tuple(Int64,Error)", Token: "%r_1"},
		ElemType: "Int64",
	}
	fn := &ast.FuncDecl{
		Name:               "f",
		ResolvedReturnType: "Tuple(Int64,Error)",
		Body: &ast.Block{Exprs: []ast.Expr{
			&ast.LetExpr{Name: "x", Token: "%x_2", ResolvedType: "Int64", Value: tryExpr},
			&ast.IdentExpr{Name: "x", ResolvedType: "Int64", Token: "%x_2"},
		}},
	}
	f := mainFile(&ast.IntLit{Value: 0})
	f.Decls = append(f.Decls, fn)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{
		"FGET\t%amifl_tmp1\t%r_1\t>F0",
		"FGET\t%amifl_tmp2\t%r_1\t>F1",
		"NEQ\t%amifl_tmp3\t%amifl_tmp2\tnil",
		"IF\t%amifl_tmp3",
		"FSET\t%amifl_tmp4\t>F1\t%amifl_tmp2",
		"RET\t%amifl_tmp4",
		"ENDIF",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
}

func TestGenerate_TryOperatorOnBareErrorEmitsEarlyReturnOfErrorItself(t *testing.T) {
	tryExpr := &ast.TryExpr{
		Value:       &ast.IdentExpr{Name: "e", ResolvedType: "Error", Token: "%e_1"},
		IsBareError: true,
	}
	fn := &ast.FuncDecl{
		Name:               "f",
		ResolvedReturnType: "Error",
		Body: &ast.Block{Exprs: []ast.Expr{
			tryExpr,
			&ast.IdentExpr{Name: "e2", ResolvedType: "Error", Token: "%e2_9"},
		}},
	}
	f := mainFile(&ast.IntLit{Value: 0})
	f.Decls = append(f.Decls, fn)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{
		"NEQ\t%amifl_tmp1\t%e_1\tnil",
		"IF\t%amifl_tmp1",
		"RET\t%e_1",
		"ENDIF",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
	if strings.Contains(ir, "FGET") {
		t.Errorf("bare-Error `?` shouldn't FGET anything (no Tuple2 payload to extract); got:\n%s", ir)
	}
}

// Phase 11b (amifl-spec.md section 13.4) codegen tests.

func TestGenerate_ContainsOnMapUsesMgetOkForm(t *testing.T) {
	mapType := "Map(String,Int64)"
	call := &ast.CallExpr{
		Callee: "contains", Builtin: "contains", ResolvedType: "Bool",
		Args:     []ast.Expr{&ast.IdentExpr{Name: "m", ResolvedType: mapType, Token: "%m_1"}, &ast.StringLit{Value: "a"}},
		ArgTypes: []string{mapType, "String"},
	}
	f := mainFile(&ast.LetExpr{Name: "ok", Token: "%ok_1", ResolvedType: "Bool", Value: call}, &ast.IntLit{Value: 0})
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{
		"VAR\t%amifl_tmp1\t^int64",
		"VAR\t%amifl_tmp2\t^bool",
		"MGET\t%amifl_tmp1\t%amifl_tmp2\t%m_1\t\"a\"",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
	if strings.Contains(ir, "amiflrt") {
		t.Errorf("contains() on Map/Set shouldn't need amiflrt (MGET's ok-form is native); got:\n%s", ir)
	}
}

func TestGenerate_ContainsOnSetUsesMgetOkFormWithBoolValueType(t *testing.T) {
	setType := "Set(Int64)"
	call := &ast.CallExpr{
		Callee: "contains", Builtin: "contains", ResolvedType: "Bool",
		Args:     []ast.Expr{&ast.IdentExpr{Name: "s", ResolvedType: setType, Token: "%s_1"}, &ast.IntLit{Value: 1}},
		ArgTypes: []string{setType, "Int64"},
	}
	f := mainFile(&ast.LetExpr{Name: "ok", Token: "%ok_1", ResolvedType: "Bool", Value: call}, &ast.IntLit{Value: 0})
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{
		"VAR\t%amifl_tmp1\t^bool",
		"MGET\t%amifl_tmp1\t%amifl_tmp2\t%s_1\t1",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
}

func TestGenerate_FlattenPassesTwoTypeArguments(t *testing.T) {
	innerType := "List(Int64)"
	outerType := "List(" + innerType + ")"
	call := &ast.CallExpr{
		Callee: "flatten", Builtin: "flatten", ResolvedType: innerType,
		Args:     []ast.Expr{&ast.IdentExpr{Name: "xss", ResolvedType: outerType, Token: "%xss_1"}},
		ArgTypes: []string{outerType},
	}
	f := mainFile(&ast.LetExpr{Name: "flat", Token: "%flat_1", ResolvedType: innerType, Value: call}, &ast.IntLit{Value: 0})
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	// Flatten[S,E] needs S (the outer list's own named Go type, "AmiflList2"
	// here — AmiflList1 is the inner List(Int64)) *and* E (int64) as
	// explicit type arguments — a single type parameter isn't enough
	// (amiflrt/collections.go's Flatten doc comment).
	if !strings.Contains(ir, "CALL\t%amifl_tmp1\t:\t?amiflrt.Flatten\t^AmiflList1\t^int64\t:\t%xss_1") {
		t.Errorf("generated IR missing the two-type-argument Flatten call; got:\n%s", ir)
	}
}

func TestGenerate_ReverseOnArrayEmitsIndexLoopNotAmiflrt(t *testing.T) {
	arrType := "Array(Int64;3)"
	call := &ast.CallExpr{
		Callee: "reverse", Builtin: "reverse", ResolvedType: arrType,
		Args:     []ast.Expr{&ast.IdentExpr{Name: "arr", ResolvedType: arrType, Token: "%arr_1"}},
		ArgTypes: []string{arrType},
	}
	f := mainFile(&ast.LetExpr{Name: "r", Token: "%r_1", ResolvedType: arrType, Value: call}, &ast.IntLit{Value: 0})
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{"LOOP", "AGET", "ASET", "ENDLOOP"} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
	if strings.Contains(ir, "amiflrt") {
		t.Errorf("reverse() on Array should use a fixed-size index loop, not amiflrt.ReverseSlice; got:\n%s", ir)
	}
}

func TestGenerate_ZipBuildsTuple2ViaFsetNotAmiflrt(t *testing.T) {
	listType := "List(Int64)"
	pairType := "Tuple(Int64,Int64)"
	call := &ast.CallExpr{
		Callee: "zip", Builtin: "zip", ResolvedType: "List(" + pairType + ")",
		Args: []ast.Expr{
			&ast.IdentExpr{Name: "xs", ResolvedType: listType, Token: "%xs_1"},
			&ast.IdentExpr{Name: "ys", ResolvedType: listType, Token: "%ys_2"},
		},
		ArgTypes: []string{listType, listType},
	}
	f := mainFile(&ast.LetExpr{Name: "zipped", Token: "%zipped_1", ResolvedType: "List(" + pairType + ")", Value: call}, &ast.IntLit{Value: 0})
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{
		"CALL\t%amifl_tmp1\t:\t?len\t%xs_1",
		"CALL\t%amifl_tmp2\t:\t?len\t%ys_2",
		"LT\t%amifl_tmp4",
		"SLMAKE",
		"FSET\t%amifl_tmp10\t>F0",
		"FSET\t%amifl_tmp10\t>F1",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
}

// Phase 11c (amifl-spec.md sections 13.5/13.6) codegen tests.

func TestGenerate_SetAddEmitsMsetWithLiteralTrue(t *testing.T) {
	setType := "Set(Int64)"
	call := &ast.CallExpr{
		Callee: "add", Builtin: "add", ResolvedType: "Unit",
		Args:     []ast.Expr{&ast.IdentExpr{Name: "s", ResolvedType: setType, Token: "%s_1"}, &ast.IntLit{Value: 5}},
		ArgTypes: []string{setType, "Int64"},
	}
	f := mainFile(call, &ast.IntLit{Value: 0})
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if !strings.Contains(ir, "MSET\t%s_1\t5\ttrue") {
		t.Errorf("generated IR missing the MSET; got:\n%s", ir)
	}
}

func TestGenerate_SetDiscardEmitsDeleteBuiltin(t *testing.T) {
	setType := "Set(Int64)"
	call := &ast.CallExpr{
		Callee: "discard", Builtin: "discard", ResolvedType: "Unit",
		Args:     []ast.Expr{&ast.IdentExpr{Name: "s", ResolvedType: setType, Token: "%s_1"}, &ast.IntLit{Value: 5}},
		ArgTypes: []string{setType, "Int64"},
	}
	f := mainFile(call, &ast.IntLit{Value: 0})
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if !strings.Contains(ir, "CALL\t:\t?delete\t%s_1\t5") {
		t.Errorf("generated IR missing the ?delete CALL; got:\n%s", ir)
	}
}

func TestGenerate_MapGetUsesMgetOkFormThenIfElseForDefault(t *testing.T) {
	mapType := "Map(String,Int64)"
	call := &ast.CallExpr{
		Callee: "get", Builtin: "get", ResolvedType: "Int64",
		Args: []ast.Expr{
			&ast.IdentExpr{Name: "m", ResolvedType: mapType, Token: "%m_1"},
			&ast.StringLit{Value: "a"},
			&ast.IntLit{Value: 0},
		},
		ArgTypes: []string{mapType, "String", "Int64"},
	}
	f := mainFile(&ast.LetExpr{Name: "v", Token: "%v_1", ResolvedType: "Int64", Value: call}, &ast.IntLit{Value: 0})
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{
		"MGET\t%amifl_tmp1\t%amifl_tmp2\t%m_1\t\"a\"",
		"IF\t%amifl_tmp2",
		"SET\t%amifl_tmp3\t%amifl_tmp1",
		"ELSE",
		"SET\t%amifl_tmp3\t0",
		"ENDIF",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
}

func TestGenerate_MapEntriesBuildsTuple2ViaMpkeysAndMget(t *testing.T) {
	mapType := "Map(String,Int64)"
	pairType := "Tuple(String,Int64)"
	call := &ast.CallExpr{
		Callee: "entries", Builtin: "entries", ResolvedType: "List(" + pairType + ")",
		Args:     []ast.Expr{&ast.IdentExpr{Name: "m", ResolvedType: mapType, Token: "%m_1"}},
		ArgTypes: []string{mapType},
	}
	f := mainFile(&ast.LetExpr{Name: "es", Token: "%es_1", ResolvedType: "List(" + pairType + ")", Value: call}, &ast.IntLit{Value: 0})
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{
		"MPKEYS\t%amifl_tmp1\t%m_1",
		"MGET\t%amifl_tmp7\t%m_1\t%amifl_tmp6",
		"FSET\t%amifl_tmp8\t>F0\t%amifl_tmp6",
		"FSET\t%amifl_tmp8\t>F1\t%amifl_tmp7",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
}

// Phase 11d (amifl-spec.md sections 13.7/13.9) codegen tests.

func TestGenerate_MinEmitsLtThenIfElse(t *testing.T) {
	call := &ast.CallExpr{
		Callee: "min", Builtin: "min", ResolvedType: "Int64",
		Args:     []ast.Expr{&ast.IdentExpr{Name: "a", ResolvedType: "Int64", Token: "%a_1"}, &ast.IdentExpr{Name: "b", ResolvedType: "Int64", Token: "%b_2"}},
		ArgTypes: []string{"Int64", "Int64"},
	}
	f := mainFile(&ast.LetExpr{Name: "r", Token: "%r_1", ResolvedType: "Int64", Value: call}, &ast.IntLit{Value: 0})
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{
		"LT\t%amifl_tmp1\t%a_1\t%b_2",
		"IF\t%amifl_tmp1",
		"SET\t%amifl_tmp2\t%a_1",
		"ELSE",
		"SET\t%amifl_tmp2\t%b_2",
		"ENDIF",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
}

func TestGenerate_AbsUsesZeroLiteralMatchingType(t *testing.T) {
	call := &ast.CallExpr{
		Callee: "abs", Builtin: "abs", ResolvedType: "Float64",
		Args:     []ast.Expr{&ast.IdentExpr{Name: "v", ResolvedType: "Float64", Token: "%v_1"}},
		ArgTypes: []string{"Float64"},
	}
	f := mainFile(&ast.LetExpr{Name: "r", Token: "%r_1", ResolvedType: "Float64", Value: call}, &ast.IntLit{Value: 0})
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if !strings.Contains(ir, "LT\t%amifl_tmp1\t%v_1\t0.0") {
		t.Errorf("generated IR missing the 0.0-literal comparison (Float64 must use \"0.0\", not \"0\"); got:\n%s", ir)
	}
}

func TestGenerate_PowNarrowsFloat32ThroughFloat64(t *testing.T) {
	call := &ast.CallExpr{
		Callee: "pow", Builtin: "pow", ResolvedType: "Float32",
		Args:     []ast.Expr{&ast.IdentExpr{Name: "base", ResolvedType: "Float32", Token: "%base_1"}, &ast.IdentExpr{Name: "exp", ResolvedType: "Float32", Token: "%exp_2"}},
		ArgTypes: []string{"Float32", "Float32"},
	}
	f := mainFile(&ast.LetExpr{Name: "r", Token: "%r_1", ResolvedType: "Float32", Value: call}, &ast.IntLit{Value: 0})
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{
		"CALL\t%amifl_tmp1\t:\t?float64\t%base_1",
		"CALL\t%amifl_tmp2\t:\t?float64\t%exp_2",
		"CALL\t%amifl_tmp3\t:\t?math.Pow\t%amifl_tmp1\t%amifl_tmp2",
		"CALL\t%amifl_tmp4\t:\t?float32\t%amifl_tmp3",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
}

func TestGenerate_UnwrapPanicsOnNonNilError(t *testing.T) {
	tupleType := "Tuple(Int64,Error)"
	call := &ast.CallExpr{
		Callee: "unwrap", Builtin: "unwrap", ResolvedType: "Int64",
		Args:     []ast.Expr{&ast.IdentExpr{Name: "t", ResolvedType: tupleType, Token: "%t_1"}},
		ArgTypes: []string{tupleType},
	}
	f := mainFile(&ast.LetExpr{Name: "x", Token: "%x_1", ResolvedType: "Int64", Value: call}, &ast.IntLit{Value: 0})
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{
		"FGET\t%amifl_tmp1\t%t_1\t>F0",
		"FGET\t%amifl_tmp2\t%t_1\t>F1",
		"NEQ\t%amifl_tmp3\t%amifl_tmp2\tnil",
		"IF\t%amifl_tmp3",
		"CALL\t:\t?panic\t%amifl_tmp2",
		"ENDIF",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
}

func TestGenerate_OkOrPicksDefaultOnError(t *testing.T) {
	tupleType := "Tuple(Int64,Error)"
	call := &ast.CallExpr{
		Callee: "okOr", Builtin: "okOr", ResolvedType: "Int64",
		Args: []ast.Expr{
			&ast.IdentExpr{Name: "t", ResolvedType: tupleType, Token: "%t_1"},
			&ast.IntLit{Value: 0},
		},
		ArgTypes: []string{tupleType, "Int64"},
	}
	f := mainFile(&ast.LetExpr{Name: "x", Token: "%x_1", ResolvedType: "Int64", Value: call}, &ast.IntLit{Value: 0})
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{
		"FGET\t%amifl_tmp1\t%t_1\t>F0",
		"FGET\t%amifl_tmp2\t%t_1\t>F1",
		"NEQ\t%amifl_tmp3\t%amifl_tmp2\tnil",
		"IF\t%amifl_tmp3",
		"SET\t%amifl_tmp4\t0",
		"ELSE",
		"SET\t%amifl_tmp4\t%amifl_tmp1",
		"ENDIF",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
	if strings.Contains(ir, "panic") {
		t.Errorf("okOr should never panic (that's unwrap's job); got:\n%s", ir)
	}
}

// --- step 12: Chan[T]/Stream[T]/File built-ins ---

func TestGenerate_ChanEmitsChtypeAndChmake(t *testing.T) {
	call := &ast.CallExpr{
		Callee: "chan", Builtin: "chan", ResolvedType: "Chan(Int64)",
		Args: []ast.Expr{&ast.IntLit{Value: 0}},
	}
	f := mainFile(&ast.LetExpr{Name: "ch", Token: "%ch_1", ResolvedType: "Chan(Int64)", Value: call}, &ast.IntLit{Value: 0})
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{
		"CHTYPE\t^AmiflChan1\t^int64",
		"VAR\t%amifl_tmp1\t^AmiflChan1",
		"CHMAKE\t%amifl_tmp1\t^AmiflChan1\t0",
		"SET\t%ch_1\t%amifl_tmp1",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
}

func TestGenerate_SendEmitsChsend(t *testing.T) {
	call := &ast.CallExpr{
		Callee: "send", Builtin: "send", ResolvedType: "Unit",
		Args:     []ast.Expr{&ast.IdentExpr{Name: "ch", ResolvedType: "Chan(Int64)", Token: "%ch_1"}, &ast.IntLit{Value: 7}},
		ArgTypes: []string{"Chan(Int64)", "Int64"},
	}
	f := mainFile(call, &ast.IntLit{Value: 0})
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if want := "CHSEND\t%ch_1\t7"; !strings.Contains(ir, want) {
		t.Errorf("generated IR missing %q; got:\n%s", want, ir)
	}
}

func TestGenerate_RecvEmitsChrecvAndAssemblesTuple2(t *testing.T) {
	call := &ast.CallExpr{
		Callee: "recv", Builtin: "recv", ResolvedType: "Tuple(Int64,Bool)",
		Args:     []ast.Expr{&ast.IdentExpr{Name: "ch", ResolvedType: "Chan(Int64)", Token: "%ch_1"}},
		ArgTypes: []string{"Chan(Int64)"},
	}
	f := mainFile(&ast.LetExpr{Name: "r", Token: "%r_1", ResolvedType: "Tuple(Int64,Bool)", Value: call}, &ast.IntLit{Value: 0})
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{
		"VAR\t%amifl_tmp1\t^int64",
		"VAR\t%amifl_tmp2\t^bool",
		"CHRECV\t%amifl_tmp1\t%amifl_tmp2\t%ch_1",
		"FSET\t%amifl_tmp3\t>F0\t%amifl_tmp1",
		"FSET\t%amifl_tmp3\t>F1\t%amifl_tmp2",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
}

func TestGenerate_SpawnEmitsBareSpawnOfClosureToken(t *testing.T) {
	call := &ast.CallExpr{
		Callee: "spawn", Builtin: "spawn", ResolvedType: "Unit",
		Args:     []ast.Expr{&ast.IdentExpr{Name: "f", ResolvedType: "fn()->Unit", Token: "%f_1"}},
		ArgTypes: []string{"fn()->Unit"},
	}
	f := mainFile(call, &ast.IntLit{Value: 0})
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if want := "SPAWN\t%f_1"; !strings.Contains(ir, want) {
		t.Errorf("generated IR missing %q; got:\n%s", want, ir)
	}
}

func TestGenerate_TakeEmitsDeferCloseRelayClosure(t *testing.T) {
	call := &ast.CallExpr{
		Callee: "take", Builtin: "take", ResolvedType: "Stream(String)",
		Args: []ast.Expr{
			&ast.IdentExpr{Name: "s", ResolvedType: "Stream(String)", Token: "%s_1"},
			&ast.IntLit{Value: 2},
		},
		ArgTypes: []string{"Stream(String)", "Int64"},
	}
	f := mainFile(&ast.LetExpr{Name: "tk", Token: "%tk_1", ResolvedType: "Stream(String)", Value: call}, &ast.IntLit{Value: 0})
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{
		"CHTYPE\t^AmiflStream1\t^string",
		"CHMAKE\t%amifl_tmp1\t^AmiflStream1\t0",
		"FNTYPE\t^AmiflFunc1\t:",
		"CLOS\t%amifl_tmp2\t:",
		"DEFER\t?close\t%amifl_tmp1",
		"GTE\t%amifl_tmp4\t%amifl_tmp3\t2",
		"CHRECV\t%amifl_tmp5\t%amifl_tmp6\t%s_1",
		"NOT\t%amifl_tmp7\t%amifl_tmp6",
		"CHSEND\t%amifl_tmp1\t%amifl_tmp5",
		"RET\n\tENDCLOS\n\tSPAWN\t%amifl_tmp2",
		"SET\t%tk_1\t%amifl_tmp1",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
}

func TestGenerate_CollectEmitsChrecvLoopWithPush(t *testing.T) {
	call := &ast.CallExpr{
		Callee: "collect", Builtin: "collect", ResolvedType: "List(String)",
		Args:     []ast.Expr{&ast.IdentExpr{Name: "s", ResolvedType: "Stream(String)", Token: "%s_1"}},
		ArgTypes: []string{"Stream(String)"},
	}
	f := mainFile(&ast.LetExpr{Name: "xs", Token: "%xs_1", ResolvedType: "List(String)", Value: call}, &ast.IntLit{Value: 0})
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{
		"SLMAKE\t%amifl_tmp1\t^AmiflList1\t0",
		"CHRECV\t%amifl_tmp2\t%amifl_tmp3\t%s_1",
		"NOT\t%amifl_tmp4\t%amifl_tmp3",
		"CALL\t%amifl_tmp1\t:\t?amiflrt.Push\t^string\t:\t%amifl_tmp1\t%amifl_tmp2",
		"SET\t%xs_1\t%amifl_tmp1",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
}

func TestGenerate_OpenAssemblesTuple2FromOpenFileCall(t *testing.T) {
	call := &ast.CallExpr{
		Callee: "open", Builtin: "open", ResolvedType: "Tuple(File,Error)",
		Args:     []ast.Expr{&ast.StringLit{Value: "/tmp/x"}, &ast.StringLit{Value: "r"}},
		ArgTypes: []string{"String", "String"},
	}
	f := mainFile(&ast.LetExpr{Name: "o", Token: "%o_1", ResolvedType: "Tuple(File,Error)", Value: call}, &ast.IntLit{Value: 0})
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{
		"VAR\t%amifl_tmp1\t^*amiflrt.FileHandle",
		"VAR\t%amifl_tmp2\t^error",
		"CALL\t%amifl_tmp1\t%amifl_tmp2\t:\t?amiflrt.OpenFile\t\"/tmp/x\"\t\"r\"",
		"FSET\t%amifl_tmp3\t>F0\t%amifl_tmp1",
		"FSET\t%amifl_tmp3\t>F1\t%amifl_tmp2",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
}

func TestGenerate_LinesEmitsRelayClosureCallingReadLineFile(t *testing.T) {
	call := &ast.CallExpr{
		Callee: "lines", Builtin: "lines", ResolvedType: "Stream(String)",
		Args:     []ast.Expr{&ast.IdentExpr{Name: "f", ResolvedType: "File", Token: "%f_1"}},
		ArgTypes: []string{"File"},
	}
	f := mainFile(&ast.LetExpr{Name: "s6", Token: "%s6_1", ResolvedType: "Stream(String)", Value: call}, &ast.IntLit{Value: 0})
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{
		"CHTYPE\t^AmiflStream1\t^string",
		"DEFER\t?close\t%amifl_tmp1",
		"CALL\t%amifl_tmp3\t%amifl_tmp4\t:\t?amiflrt.ReadLineFile\t%f_1",
		"NEQ\t%amifl_tmp5\t%amifl_tmp4\tnil",
		"CHSEND\t%amifl_tmp1\t%amifl_tmp3",
		"SPAWN\t%amifl_tmp2",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
}

func TestGenerate_ForOverStreamEmitsChrecvLoop(t *testing.T) {
	forExpr := &ast.ForExpr{
		Var: "line", VarToken: "%line_2", ElemType: "String", ItemsType: "Stream(String)",
		Items: &ast.IdentExpr{Name: "s", ResolvedType: "Stream(String)", Token: "%s_1"},
		Body: &ast.Block{Exprs: []ast.Expr{
			&ast.CallExpr{Callee: "print", Args: []ast.Expr{&ast.IdentExpr{Name: "line", ResolvedType: "String", Token: "%line_2"}}},
		}},
	}
	f := mainFile(forExpr, &ast.IntLit{Value: 0})
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{
		"VAR\t%line_2\t^string",
		"CHRECV\t%line_2\t%amifl_tmp1\t%s_1",
		"NOT\t%amifl_tmp2\t%amifl_tmp1",
		"IF\t%amifl_tmp2\n\tBREAK\n\tENDIF",
		"CALL\t:\t?fmt.Println\t%line_2",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
}

func TestGenerate_WriteAssemblesTuple2FromWriteFileCall(t *testing.T) {
	call := &ast.CallExpr{
		Callee: "write", Builtin: "write", ResolvedType: "Tuple(Int64,Error)",
		Args: []ast.Expr{
			&ast.IdentExpr{Name: "f", ResolvedType: "File", Token: "%f_1"},
			&ast.IdentExpr{Name: "data", ResolvedType: "List(UInt8)", Token: "%data_2"},
		},
		ArgTypes: []string{"File", "List(UInt8)"},
	}
	f := mainFile(&ast.LetExpr{Name: "wr", Token: "%wr_1", ResolvedType: "Tuple(Int64,Error)", Value: call}, &ast.IntLit{Value: 0})
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if want := "CALL\t%amifl_tmp1\t%amifl_tmp2\t:\t?amiflrt.WriteFile\t%f_1\t%data_2"; !strings.Contains(ir, want) {
		t.Errorf("generated IR missing %q; got:\n%s", want, ir)
	}
}

// --- step 13: extern/bind (Any/extern value boundary, CLAUDE.md design issue 1) ---

func TestGenerate_ExternPlainBindEmitsQualifiedCallAndAssemblesTuple2(t *testing.T) {
	call := &ast.CallExpr{
		Callee: "Marshal", ResolvedType: "Tuple(List(UInt8),Error)",
		CalleeToken:      "?json.Marshal",
		Args:             []ast.Expr{&ast.IdentExpr{Name: "v", ResolvedType: "String", Token: "%v_1"}},
		ExternParamTypes: []string{"Any"},
	}
	f := mainFile(&ast.LetExpr{Name: "m", Token: "%m_1", ResolvedType: "Tuple(List(UInt8),Error)", Value: call}, &ast.IntLit{Value: 0})
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{
		"SLTYPE\t^AmiflList1\t^uint8",
		"CALL\t%amifl_tmp1\t%amifl_tmp2\t:\t?json.Marshal\t%v_1",
		"FSET\t%amifl_tmp3\t>F0\t%amifl_tmp1",
		"FSET\t%amifl_tmp3\t>F1\t%amifl_tmp2",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
}

func TestGenerate_ExternBindRenameUsesGoTargetAsCallname(t *testing.T) {
	// sema always resolves CalleeToken to the effective Go-side name
	// (GoTarget if the `bind` gave one, else the bind's own Name — see
	// registerExternBind) — codegen never sees GoTarget itself, only the
	// already-resolved CalleeToken, so this is really the same code path
	// as the plain-bind test above; kept as its own test to document that
	// a rename is genuinely indistinguishable from an un-renamed bind by
	// the time codegen runs (CLAUDE.md's "semaが計算した情報をASTへアノ
	// テーションし、codegenは読むだけ" pattern).
	call := &ast.CallExpr{
		Callee: "Marshal2", ResolvedType: "Unit",
		CalleeToken:      "?json.Marshal",
		Args:             []ast.Expr{&ast.IdentExpr{Name: "v", ResolvedType: "String", Token: "%v_1"}},
		ExternParamTypes: []string{"Any"},
	}
	f := mainFile(call, &ast.IntLit{Value: 0})
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if want := "CALL\t:\t?json.Marshal\t%v_1"; !strings.Contains(ir, want) {
		t.Errorf("generated IR missing %q; got:\n%s", want, ir)
	}
}

func TestGenerate_ExternMethodBindZeroArgsEmitsMethvalThenCall(t *testing.T) {
	call := &ast.CallExpr{
		Callee: "TimeUnix", ResolvedType: "Int64",
		ExternMethod:     "Unix",
		Args:             []ast.Expr{&ast.IdentExpr{Name: "t", ResolvedType: "Time", Token: "%t_1"}},
		ExternParamTypes: []string{"Time"},
	}
	f := mainFile(&ast.LetExpr{Name: "u", Token: "%u_1", ResolvedType: "Int64", Value: call}, &ast.IntLit{Value: 0})
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{
		"METHVAL\t%amifl_tmp1\t%t_1\t<Unix",
		"CALL\t%amifl_tmp2\t:\t%amifl_tmp1\n",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
}

func TestGenerate_ExternMethodBindWithExtraArgEmitsMethvalThenCallWithRemainingArgs(t *testing.T) {
	call := &ast.CallExpr{
		Callee: "TimeFormat", ResolvedType: "String",
		ExternMethod: "Format",
		Args: []ast.Expr{
			&ast.IdentExpr{Name: "t", ResolvedType: "Time", Token: "%t_1"},
			&ast.StringLit{Value: "2006"},
		},
		ExternParamTypes: []string{"Time", "String"},
	}
	f := mainFile(&ast.LetExpr{Name: "s", Token: "%s_1", ResolvedType: "String", Value: call}, &ast.IntLit{Value: 0})
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{
		"METHVAL\t%amifl_tmp1\t%t_1\t<Format",
		"CALL\t%amifl_tmp2\t:\t%amifl_tmp1\t\"2006\"",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
}

func TestGenerate_ExternAnyParamBoxesBareIntLiteral(t *testing.T) {
	// A bare integer literal passed directly to an Any-typed extern
	// parameter must be explicitly cast to int64 before boxing — Go's own
	// untyped-constant default ("int") would otherwise silently disagree
	// with the "int64" sema's literal-defaulting actually resolved it to.
	// Self-caught by actually running examples/extern.aml's typeName(5)
	// through the full pipeline (CLAUDE.md's step-13 notes) before ever
	// writing this test — not derived from reading the code alone.
	call := &ast.CallExpr{
		Callee: "Marshal", ResolvedType: "Unit",
		CalleeToken:      "?json.Marshal",
		Args:             []ast.Expr{&ast.IntLit{Value: 5}},
		ExternParamTypes: []string{"Any"},
	}
	f := mainFile(call, &ast.IntLit{Value: 0})
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{
		"VAR\t%amifl_tmp1\t^int64",
		"CALL\t%amifl_tmp1\t:\t?int64\t5",
		"CALL\t:\t?json.Marshal\t%amifl_tmp1",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
}

func TestGenerate_ExternAnyParamLeavesVariableArgUnboxed(t *testing.T) {
	call := &ast.CallExpr{
		Callee: "Marshal", ResolvedType: "Unit",
		CalleeToken:      "?json.Marshal",
		Args:             []ast.Expr{&ast.IdentExpr{Name: "v", ResolvedType: "Int64", Token: "%v_1"}},
		ExternParamTypes: []string{"Any"},
	}
	f := mainFile(call, &ast.IntLit{Value: 0})
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if want := "CALL\t:\t?json.Marshal\t%v_1"; !strings.Contains(ir, want) {
		t.Errorf("generated IR missing %q; got:\n%s", want, ir)
	}
	if strings.Contains(ir, "?int64") {
		t.Errorf("didn't expect an int64 cast for an already-typed variable argument; got:\n%s", ir)
	}
}

func TestGenerate_TypeNameBoxesBareIntLiteralButNotAVariable(t *testing.T) {
	litCall := &ast.CallExpr{Callee: "typeName", Builtin: "typeName", ResolvedType: "String", Args: []ast.Expr{&ast.IntLit{Value: 5}}, ArgTypes: []string{"Int64"}}
	f := mainFile(&ast.LetExpr{Name: "tn", Token: "%tn_1", ResolvedType: "String", Value: litCall}, &ast.IntLit{Value: 0})
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{
		"CALL\t%amifl_tmp1\t:\t?int64\t5",
		"CALL\t%amifl_tmp2\t:\t?fmt.Sprintf\t\"%T\"\t%amifl_tmp1",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}

	varCall := &ast.CallExpr{Callee: "typeName", Builtin: "typeName", ResolvedType: "String", Args: []ast.Expr{&ast.IdentExpr{Name: "v", ResolvedType: "Int64", Token: "%v_1"}}, ArgTypes: []string{"Int64"}}
	f2 := mainFile(&ast.LetExpr{Name: "tn", Token: "%tn_1", ResolvedType: "String", Value: varCall}, &ast.IntLit{Value: 0})
	ir2, err := Generate(f2)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if want := "CALL\t%amifl_tmp1\t:\t?fmt.Sprintf\t\"%T\"\t%v_1"; !strings.Contains(ir2, want) {
		t.Errorf("generated IR missing %q; got:\n%s", want, ir2)
	}
	if strings.Contains(ir2, "?int64") {
		t.Errorf("didn't expect an int64 cast for an already-typed variable argument; got:\n%s", ir2)
	}
}

func TestGenerate_ExternTypeResolvesToAliasQualifiedGoType(t *testing.T) {
	f := &ast.File{
		Decls: []ast.TopLevelDecl{
			&ast.ExternDecl{Path: "time", Alias: "time", Types: []ast.ExternTypeDecl{{Name: "Time"}}},
			&ast.FuncDecl{
				Name: "main", ReturnType: nt("Int"), ResolvedReturnType: "Int64",
				Body: &ast.Block{Exprs: []ast.Expr{
					&ast.LetExpr{Name: "now", Token: "%now_1", ResolvedType: "Time", Value: &ast.CallExpr{
						Callee: "Now", ResolvedType: "Time", CalleeToken: "?time.Now",
					}},
					&ast.IntLit{Value: 0},
				}},
			},
		},
	}
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if want := "VAR\t%now_1\t^time.Time"; !strings.Contains(ir, want) {
		t.Errorf("generated IR missing %q; got:\n%s", want, ir)
	}
}

// --- step 14: modules ---

func TestGenerateProgram_NonRootFuncAndStructNamesGetPrefixed(t *testing.T) {
	util := Unit{
		Prefix: "util_",
		Decls: []ast.TopLevelDecl{
			&ast.StructDecl{Name: "Point", Fields: []ast.Param{{Name: "x", ResolvedType: "Int64"}}},
			&ast.FuncDecl{
				Name: "Helper", ReturnType: nt("Int"), ResolvedReturnType: "Int64",
				Body: &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 1}}},
			},
		},
	}
	root := Unit{Decls: mainFile(
		&ast.FieldExpr{
			Target: &ast.IdentExpr{Name: "util"}, Field: "Helper",
			Args: []ast.StructLitField{}, IsQualifiedCall: true,
			QualifiedCallee: "!util_Helper", ResolvedType: "Int64",
		},
	).Decls}

	ir, err := GenerateProgram([]Unit{util, root})
	if err != nil {
		t.Fatalf("GenerateProgram() error: %v", err)
	}
	for _, want := range []string{
		"STTYPE\t^util_Point",
		"FUNC\t!util_Helper",
		"CALL\t%amifl_tmp1\t:\t!util_Helper",
		"FUNC\t!amifl_main\t:\t^int64",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
}

func TestGenerateProgram_NonRootPackageOwnMainIsNotEntryPoint(t *testing.T) {
	util := Unit{
		Prefix: "util_",
		Decls: []ast.TopLevelDecl{
			&ast.FuncDecl{
				Name: "main", ReturnType: nt("Bool"), ResolvedReturnType: "Bool",
				Body: &ast.Block{Exprs: []ast.Expr{&ast.BoolLit{Value: true}}},
			},
		},
	}
	root := Unit{Decls: mainFile(&ast.IntLit{Value: 0}).Decls}

	ir, err := GenerateProgram([]Unit{util, root})
	if err != nil {
		t.Fatalf("GenerateProgram() error: %v", err)
	}
	if want := "FUNC\t!util_main\t:\t^bool"; !strings.Contains(ir, want) {
		t.Errorf("generated IR missing %q (non-root package's own main should just be prefixed like any other fn); got:\n%s", want, ir)
	}
	if strings.Contains(ir, "!main\t:\t^bool") {
		t.Errorf("a non-root package's own `main` must never become the program's entry point; got:\n%s", ir)
	}
}

func TestGenerate_QualifiedConstReferenceInlinesLiteral(t *testing.T) {
	f := mainFile(&ast.FieldExpr{
		Target: &ast.IdentExpr{Name: "util"}, Field: "Max",
		ResolvedType: "Int64", QualifiedConstValue: &ast.IntLit{Value: 100},
	})
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if want := "RET\t100"; !strings.Contains(ir, want) {
		t.Errorf("generated IR missing %q (a qualified const reference should inline its literal, no FGET); got:\n%s", want, ir)
	}
	if strings.Contains(ir, "FGET") {
		t.Errorf("didn't expect FGET for a qualified const reference; got:\n%s", ir)
	}
}

func TestGenerate_QualifiedUnitCallEmitsCallWithNoResultOperand(t *testing.T) {
	call := &ast.FieldExpr{
		Target: &ast.IdentExpr{Name: "util"}, Field: "Log",
		Args:            []ast.StructLitField{{Value: &ast.StringLit{Value: "hi"}}},
		IsQualifiedCall: true, QualifiedCallee: "!util_Log", ResolvedType: unitType,
	}
	f := mainFile(call, &ast.IntLit{Value: 0})
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if want := "CALL\t:\t!util_Log\t\"hi\""; !strings.Contains(ir, want) {
		t.Errorf("generated IR missing %q (a Unit-returning qualified call has no result operand); got:\n%s", want, ir)
	}
}

// --- ex5: cross-package struct/enum references (amifl-spec.md section
// 12.2) ---

// TestGenerate_QualifiedStructLitUsesGoNameVerbatim covers resolveGoType's
// new isQualifiedType branch (structs.go): a struct literal whose
// ResolvedType sema resolved to the qualified canonical string ("Qualified
// (geo_Point)", types.go's makeQualifiedType) must emit VAR/FSET against
// "geo_Point" verbatim — the internal envelope itself must never leak into
// generated IR.
func TestGenerate_QualifiedStructLitUsesGoNameVerbatim(t *testing.T) {
	f := mainFile(
		&ast.LetExpr{Name: "p", Token: "%p_1", ResolvedType: "Qualified(geo_Point)", Value: &ast.StructLit{
			Qualifier: "geo", TypeName: "Point", ResolvedType: "Qualified(geo_Point)",
			Fields: []ast.StructLitField{
				{Name: "x", Value: &ast.IntLit{Value: 1}},
				{Name: "y", Value: &ast.IntLit{Value: 2}},
			},
		}},
		&ast.IntLit{Value: 0},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if !strings.Contains(ir, "VAR\t%amifl_tmp1\t^geo_Point") {
		t.Errorf("expected the qualified struct literal to use geo_Point verbatim; got:\n%s", ir)
	}
	if strings.Contains(ir, "Qualified(") {
		t.Errorf("didn't expect the internal \"Qualified(...)\" envelope to leak into generated IR; got:\n%s", ir)
	}
}

// TestGenerate_QualifiedEnumVariantConstructionUsesGoNameVerbatim is
// TestGenerate_QualifiedStructLitUsesGoNameVerbatim's enum-construction
// counterpart (genEnumVariantValue, enum.go) — that function already
// routes through resolveGoType generically, so this needed no codegen
// changes of its own beyond the same isQualifiedType branch.
func TestGenerate_QualifiedEnumVariantConstructionUsesGoNameVerbatim(t *testing.T) {
	construction := &ast.FieldExpr{
		Field: "Circle", ResolvedType: "Qualified(geo_Shape)", IsEnumVariant: true, VariantIndex: 0,
		Args: []ast.StructLitField{{Name: "radius", Value: &ast.IntLit{Value: 5}}},
	}
	f := mainFile(
		&ast.LetExpr{Name: "s", Token: "%s_1", ResolvedType: "Qualified(geo_Shape)", Value: construction},
		&ast.IntLit{Value: 0},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if !strings.Contains(ir, "VAR\t%amifl_tmp1\t^geo_Shape") {
		t.Errorf("expected the qualified enum variant construction to use geo_Shape verbatim; got:\n%s", ir)
	}
	if !strings.Contains(ir, "FSET\t%amifl_tmp1\t>Circle_radius\t5") {
		t.Errorf("expected FSET into the mechanically-named Circle_radius field; got:\n%s", ir)
	}
}

// TestGenerateProgram_QualifiedStructTypeAvoidsDoublePrefixing confirms a
// qualified struct/enum type's already-complete Go name isn't prefixed a
// second time by whichever *importing* package happens to be generating
// at the moment resolveGoType resolves it — app's own "app_" prefix must
// never appear anywhere near geo_Point.
func TestGenerateProgram_QualifiedStructTypeAvoidsDoublePrefixing(t *testing.T) {
	app := Unit{
		Prefix: "app_",
		Decls: []ast.TopLevelDecl{
			&ast.FuncDecl{
				Name: "MakePoint", ReturnType: nt("Int"), ResolvedReturnType: "Int64",
				Body: &ast.Block{Exprs: []ast.Expr{
					&ast.LetExpr{Name: "p", Token: "%p_1", ResolvedType: "Qualified(geo_Point)", Value: &ast.StructLit{
						Qualifier: "geo", TypeName: "Point", ResolvedType: "Qualified(geo_Point)",
						Fields: []ast.StructLitField{{Name: "x", Value: &ast.IntLit{Value: 1}}},
					}},
					&ast.IntLit{Value: 0},
				}},
			},
		},
	}
	root := Unit{Decls: mainFile(&ast.IntLit{Value: 0}).Decls}
	ir, err := GenerateProgram([]Unit{app, root})
	if err != nil {
		t.Fatalf("GenerateProgram() error: %v", err)
	}
	if !strings.Contains(ir, "VAR\t%p_1\t^geo_Point") {
		t.Errorf("expected the qualified struct type to resolve to geo_Point verbatim, without app_'s own prefix applied on top; got:\n%s", ir)
	}
	if strings.Contains(ir, "app_geo_Point") || strings.Contains(ir, "Qualified(") {
		t.Errorf("didn't expect app_'s own prefix or the internal envelope to leak into the Go type name; got:\n%s", ir)
	}
}

// TestGenerateProgram_ListOfQualifiedStructSharesGoTypeWithDeclaringPackage
// is a regression test for a real bug found while implementing ex5: geo's
// own internal codegen resolves a List[Point] parameter via the bare
// canonical string "List(Point)", while an importer resolves the identical
// element type via the qualified "List(Qualified(geo_Point))" — two
// different AmiFL canonical strings for what must be one shared Go slice
// type. listGoTypeName (and, by the same fix, every other compound-type
// minting function — arrayGoTypeName/setGoTypeName/mapGoTypeName/
// chanGoTypeName/streamGoTypeName/tupleGoTypeName/funcGoTypeName) now
// caches by the *already-resolved* element Go type rather than the raw
// canonical string, which is what this test locks in: exactly one SLTYPE
// must be emitted for both, not two structurally-identical-but-distinct
// ones (Go would then reject assigning one to the other, exactly the
// "AmiflList2 vs AmiflList1" mismatch actually hit — by hand, through the
// real amivm -> go build pipeline — before this fix existed).
func TestGenerateProgram_ListOfQualifiedStructSharesGoTypeWithDeclaringPackage(t *testing.T) {
	geo := Unit{
		Prefix: "geo_",
		Decls: []ast.TopLevelDecl{
			&ast.StructDecl{Name: "Point", Fields: []ast.Param{{Name: "x", ResolvedType: "Int64"}}},
			&ast.FuncDecl{
				Name:               "Count",
				Params:             []ast.Param{{Name: "points", ResolvedType: "List(Point)"}},
				ReturnType:         nt("Int"),
				ResolvedReturnType: "Int64",
				Body:               &ast.Block{Exprs: []ast.Expr{&ast.IntLit{Value: 0}}},
			},
		},
	}
	root := Unit{Decls: mainFile(
		&ast.LetExpr{Name: "pts", Token: "%pts_1", ResolvedType: "List(Qualified(geo_Point))", Value: &ast.ListLit{
			ResolvedType: "List(Qualified(geo_Point))",
		}},
		&ast.IntLit{Value: 0},
	).Decls}
	ir, err := GenerateProgram([]Unit{geo, root})
	if err != nil {
		t.Fatalf("GenerateProgram() error: %v", err)
	}
	if strings.Count(ir, "SLTYPE") != 1 {
		t.Errorf("expected exactly one shared SLTYPE for List(Point)/List(Qualified(geo_Point)) (both wrapping geo_Point); got:\n%s", ir)
	}
}

// --- ex6: print/eprint/format/formatWith/exit (amifl-spec.md section
// 13.1) ---

// TestGenerate_PrintAcceptsNonStringValueDirectly confirms ex6's
// generalization needed zero codegen change: fmt.Println's own `...any`
// parameter already accepts a concrete Go value (here an Int64) with no
// boxing/VAR-of-any machinery, unlike typeName's own literal-boxing
// concern (which only matters for %T, not %v).
func TestGenerate_PrintAcceptsNonStringValueDirectly(t *testing.T) {
	call := &ast.CallExpr{Callee: "print", ResolvedType: unitType, Args: []ast.Expr{&ast.IntLit{Value: 5}}}
	f := mainFile(call, &ast.IntLit{Value: 0})
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if want := "CALL\t:\t?fmt.Println\t5"; !strings.Contains(ir, want) {
		t.Errorf("generated IR missing %q; got:\n%s", want, ir)
	}
}

func TestGenerate_EprintCallsAmiflrtEprint(t *testing.T) {
	call := &ast.CallExpr{Callee: "eprint", Builtin: "eprint", ResolvedType: unitType, Args: []ast.Expr{&ast.StringLit{Value: "oops"}}}
	f := mainFile(call, &ast.IntLit{Value: 0})
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if want := "CALL\t:\t?amiflrt.Eprint\t\"oops\""; !strings.Contains(ir, want) {
		t.Errorf("generated IR missing %q; got:\n%s", want, ir)
	}
}

func TestGenerate_FormatCallsAmiflrtFormat(t *testing.T) {
	call := &ast.CallExpr{Callee: "format", Builtin: "format", ResolvedType: "String", Args: []ast.Expr{&ast.IntLit{Value: 5}}}
	f := mainFile(&ast.LetExpr{Name: "s", Token: "%s_1", ResolvedType: "String", Value: call}, &ast.IntLit{Value: 0})
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{
		"VAR\t%amifl_tmp1\t^string",
		"CALL\t%amifl_tmp1\t:\t?amiflrt.Format\t5",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
}

func TestGenerate_FormatWithCallsAmiflrtFormatWith(t *testing.T) {
	call := &ast.CallExpr{
		Callee: "formatWith", Builtin: "formatWith", ResolvedType: "String",
		Args: []ast.Expr{&ast.StringLit{Value: "hi {}"}, &ast.IntLit{Value: 5}},
	}
	f := mainFile(&ast.LetExpr{Name: "s", Token: "%s_1", ResolvedType: "String", Value: call}, &ast.IntLit{Value: 0})
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if want := "CALL\t%amifl_tmp1\t:\t?amiflrt.FormatWith\t\"hi {}\"\t5"; !strings.Contains(ir, want) {
		t.Errorf("generated IR missing %q; got:\n%s", want, ir)
	}
}

// --- ex7: hex/octal/binary integer literals, digit-separator `_`
// (amifl-spec.md section 3.1) ---

// TestGenerate_IntLitEmitsItsOwnRawTokenText confirms codegen prefers
// ast.IntLit.Token (the exact source text, ex7) over re-deriving decimal
// digits from Value — amivm's own upgraded literal grammar
// (ignored/amivm/amivm_spec.md section 6) accepts a hex/octal/binary/
// underscored token "そのまま" (as-is), so there's no reason to lose that
// formatting on the way through.
func TestGenerate_IntLitEmitsItsOwnRawTokenText(t *testing.T) {
	f := mainFile(&ast.IntLit{Value: 26, Token: "0x1_A"})
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if !strings.Contains(ir, "RET\t0x1_A") {
		t.Errorf("expected the raw token 0x1_A to pass through unchanged; got:\n%s", ir)
	}
}

// TestGenerate_IntLitWithNoTokenFallsBackToDecimal is a regression guard
// for every hand-built *ast.IntLit{Value: N} across this test file (and
// any other Token-less node) — Token empty must still render Value in
// plain decimal, exactly as codegen did before ex7.
func TestGenerate_IntLitWithNoTokenFallsBackToDecimal(t *testing.T) {
	f := mainFile(&ast.IntLit{Value: 42})
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if !strings.Contains(ir, "RET\t42") {
		t.Errorf("expected a Token-less IntLit to render its Value in decimal; got:\n%s", ir)
	}
}

// TestGenerate_UnaryMinusOfHexLiteralInlinesAsNegativeRawToken mirrors
// TestGenerate_UnaryMinusOfLiteralInlinesAsNegativeLiteral, but over a hex
// literal — amivm's own grammar documents "-0x1A" as a valid signed
// literal token (section 6), and literalToken's existing "prepend '-' to
// whatever the literal's own token text is" logic (codegen.go) needed no
// change at all to produce it.
func TestGenerate_UnaryMinusOfHexLiteralInlinesAsNegativeRawToken(t *testing.T) {
	f := mainFile(
		&ast.UnaryExpr{Op: "-", Operand: &ast.IntLit{Value: 26, Token: "0x1A"}, ResolvedType: "Int64"},
	)
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if !strings.Contains(ir, "RET\t-0x1A") {
		t.Errorf("expected -0x1A to inline as a bare negative literal (no VAR/SUB); got:\n%s", ir)
	}
	if strings.Contains(ir, "SUB") {
		t.Errorf("did not expect a SUB instruction for negating a literal; got:\n%s", ir)
	}
}

// TestGenerate_ExitCastsInt64ToNativeIntBeforeOsExit is a regression test
// for the same "os.Exit takes Go's native int, not AmiFL's fixed-width
// Int64" gotcha codegen.go's own `!main` wrapper already bridges (CLAUDE.md's
// "過去に踏まれた地雷" #5) — exit's own codegen must apply the identical
// CALL-as-conversion cast rather than passing an Int64 value straight to
// ?os.Exit, which go/types would reject.
func TestGenerate_ExitCastsInt64ToNativeIntBeforeOsExit(t *testing.T) {
	call := &ast.CallExpr{Callee: "exit", Builtin: "exit", ResolvedType: unitType, Args: []ast.Expr{&ast.IntLit{Value: 1}}}
	f := mainFile(call, &ast.IntLit{Value: 0})
	ir, err := Generate(f)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	for _, want := range []string{
		"VAR\t%amifl_tmp1\t^int",
		"CALL\t%amifl_tmp1\t:\t?int\t1",
		"CALL\t:\t?os.Exit\t%amifl_tmp1",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("generated IR missing %q; got:\n%s", want, ir)
		}
	}
}
