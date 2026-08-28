package codegen

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/amisonnet8/amifl/internal/ast"
)

// entryFunc is the AMIVM function name user code's `fn main` compiles to.
// AmiFL's `main` returns Int (and, from step 5, takes List[String] args),
// which is incompatible with Go's argument-less, return-less
// `func main()` — so, following Seed/Cascade/Weave's precedent (CLAUDE.md
// "過去に踏まれた地雷"), user code compiles under this internal name and
// Generate emits a thin `!main` wrapper that calls it and passes the
// result to os.Exit.
const entryFunc = "amifl_main"

// goTypeNames maps AmiFL's canonical scalar type names (sema has already
// resolved aliases like "Int" -> "Int64" by the time codegen sees them)
// to the Go type name AMIVM should declare.
var goTypeNames = map[string]string{
	"Int8": "int8", "Int16": "int16", "Int32": "int32", "Int64": "int64",
	"UInt8": "uint8", "UInt16": "uint16", "UInt32": "uint32", "UInt64": "uint64",
	"Float32": "float32", "Float64": "float64",
	"Bool": "bool", "String": "string",
}

// Generate lowers a sema-checked AmiFL file into AMIVM-IR text.
func Generate(f *ast.File) (string, error) {
	main := findMain(f)
	if main == nil {
		return "", fmt.Errorf("codegen: no fn main (sema should have rejected this)")
	}

	var b strings.Builder
	fmt.Fprintf(&b, "FUNC\t!%s\t:\t^int64\n", entryFunc)
	g := &gen{b: &b}
	if err := g.genBlock(main.Body.Exprs); err != nil {
		return "", err
	}
	b.WriteString("ENDFUNC\n\n")

	// os.Exit takes Go's native (platform-width) int, not AmiFL's
	// fixed-width Int64 — %code is cast down via the CALL-as-Go-type-
	// conversion trick (CLAUDE.md's "過去に踏まれた地雷" #5) before being
	// passed on. Found in step 1 by actually running this through amivm
	// -> go build (Seed's §6.1 lesson), not by inspecting the IR.
	b.WriteString("FUNC\t!main\t:\n")
	b.WriteString("\tVAR\t%code\t^int64\n")
	b.WriteString("\tVAR\t%exitCode\t^int\n")
	fmt.Fprintf(&b, "\tCALL\t%%code\t:\t!%s\n", entryFunc)
	b.WriteString("\tCALL\t%exitCode\t:\t?int\t%code\n")
	b.WriteString("\tCALL\t:\t?os.Exit\t%exitCode\n")
	b.WriteString("ENDFUNC\n")

	return b.String(), nil
}

func findMain(f *ast.File) *ast.FuncDecl {
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name == "main" {
			return fn
		}
	}
	return nil
}

// gen holds the per-function codegen state. Step 4's if/while compile to
// AMIVM's IF/LOOP, which are themselves genuinely nested Go blocks (not
// goto-based), so there is still nothing to hoist VAR declarations past —
// each `let` emits its VAR right where it's declared, at whatever nesting
// depth that is. Revisit this once an actually goto-producing construct
// (switch's break-out-of-multiple-cases, for-in's continue) exists; see
// CLAUDE.md's "過去に踏まれた地雷" #1.
type gen struct {
	b      *strings.Builder
	tmpSeq int
}

// newTemp mints a fresh internal variable name for a binary/unary
// operator's intermediate result. The "amifl_tmp" prefix is reserved for
// codegen's own use and isn't one a `let`/`const` name can collide with in
// practice; step 4's nested-scope work (CLAUDE.md's "過去に踏まれた地雷"
// #4) is the natural place to fold this into a single, fully
// collision-proof name-minting scheme shared with user declarations (the
// way Cascade's freshName does), once shadowing makes that necessary
// anyway.
func (g *gen) newTemp() string {
	g.tmpSeq++
	return fmt.Sprintf("amifl_tmp%d", g.tmpSeq)
}

// genBlock emits a function body's statements, treating the last
// expression as the enclosing function's return value (sema's checkBlock
// guarantees this typechecks against the function's declared return
// type).
func (g *gen) genBlock(exprs []ast.Expr) error {
	if len(exprs) == 0 {
		return fmt.Errorf("codegen: empty block (sema should have rejected this)")
	}
	if err := g.genStmtBlock(exprs[:len(exprs)-1]); err != nil {
		return err
	}
	return g.genReturn(exprs[len(exprs)-1])
}

// genStmtBlock emits every expression in exprs purely for whatever side
// effect it has, discarding any value each one produces — used for a
// Unit-typed block body (an if/while branch used as a statement) and for
// a function body's non-final statements (genBlock), which is exactly
// the same thing: nothing downstream reads their value.
func (g *gen) genStmtBlock(exprs []ast.Expr) error {
	for _, e := range exprs {
		if err := g.genStmt(e); err != nil {
			return err
		}
	}
	return nil
}

// genValueBlock is genStmtBlock's counterpart for a value-producing
// branch of an if-expression: exprs[:len-1] run purely for effect exactly
// like genStmtBlock, and the final expression's value is computed and
// written into dest (a temp genIfValue already declared) with SET, rather
// than RET like a function body's genBlock — an if-branch's "return" is a
// value flowing out of a control-flow block, not out of a function.
func (g *gen) genValueBlock(exprs []ast.Expr, dest string) error {
	if len(exprs) == 0 {
		return fmt.Errorf("codegen: empty block used as a value (sema should have rejected this)")
	}
	if err := g.genStmtBlock(exprs[:len(exprs)-1]); err != nil {
		return err
	}
	val, err := g.genValue(exprs[len(exprs)-1])
	if err != nil {
		return err
	}
	fmt.Fprintf(g.b, "\tSET\t%%%s\t%s\n", dest, val)
	return nil
}

func (g *gen) genReturn(e ast.Expr) error {
	val, err := g.genValue(e)
	if err != nil {
		return err
	}
	fmt.Fprintf(g.b, "\tRET\t%s\n", val)
	return nil
}

// genStmt emits whatever instructions are needed to run e purely for its
// side effect, discarding any value it produces. This doubles as both
// "emit a Unit-typed expression found in statement position" (the case
// for every expression this function normally sees, since sema already
// requires a block's non-final expressions — and, once desugared, an
// if/while used as a statement — to be Unit-typed) and `_ = expr`'s own
// codegen (ast.DiscardExpr recurses right back into genStmt): discarding
// a value is exactly "run it for effect, ignore the result", the same
// operation either way. That equivalence matters concretely once a
// discarded expression can contain control flow — e.g. `_ = if c {
// print("x"); 1 } else { 2 }` — where the discarded if's *own* type isn't
// Unit (its branches resolve to Int64), but a side effect (print) buried
// inside one of its branches still has to run; routing it through
// genIfBranch (the same statement-form emitter normal Unit-typed ifs use)
// via this function is what makes that happen.
func (g *gen) genStmt(e ast.Expr) error {
	switch v := e.(type) {
	case *ast.CallExpr:
		return g.genCallStmt(v)
	case *ast.LetExpr:
		return g.genLetStmt(v)
	case *ast.ConstDecl:
		// Consts have no runtime representation — every reference to one
		// was already inlined by sema (ast.IdentExpr.ConstValue).
		return nil
	case *ast.AssignExpr:
		return g.genAssignStmt(v)
	case *ast.DiscardExpr:
		return g.genStmt(v.Value)
	case *ast.IfExpr:
		return g.genIfBranch(v)
	case *ast.WhileExpr:
		return g.genWhileStmt(v)
	case *ast.BreakExpr:
		g.b.WriteString("\tBREAK\n")
		return nil
	case *ast.ContinueExpr:
		g.b.WriteString("\tCONTINUE\n")
		return nil
	case *ast.IntLit, *ast.FloatLit, *ast.BoolLit, *ast.StringLit,
		*ast.IdentExpr, *ast.BinaryExpr, *ast.UnaryExpr:
		// Pure value expressions have no side effect to run at all. A
		// block's own forced-Unit non-final position never produces one of
		// these directly (none of these kinds ever resolves to Unit in
		// sema), but a discarded non-Unit expression's tail can — e.g. the
		// `2` ending the else-branch in the doc comment's example above.
		return nil
	default:
		return fmt.Errorf("codegen: unsupported statement %T", e)
	}
}

func (g *gen) genCallStmt(c *ast.CallExpr) error {
	if c.Callee != "print" {
		return fmt.Errorf("codegen: unsupported call %q (sema should have rejected this)", c.Callee)
	}
	arg, err := g.genValue(c.Args[0])
	if err != nil {
		return err
	}
	// The operand is passed as CALL's own value argument, never
	// concatenated into the format string — an operand containing a
	// literal '%' would otherwise be misread as a format verb
	// (CLAUDE.md's "過去に踏まれた地雷" #12).
	fmt.Fprintf(g.b, "\tCALL\t:\t?fmt.Println\t%s\n", arg)
	return nil
}

// genLetStmt emits a `let`'s VAR/SET under its InternalName (sema's
// freshInternalName), never its bare surface Name — see
// ast.LetExpr.InternalName for why a shadowing `let` needs a distinct Go
// name even though AmiFL itself lets it reuse the outer name.
func (g *gen) genLetStmt(v *ast.LetExpr) error {
	goType, ok := goTypeNames[v.ResolvedType]
	if !ok {
		return fmt.Errorf("codegen: unknown type %q (sema should have rejected this)", v.ResolvedType)
	}
	val, err := g.genValue(v.Value)
	if err != nil {
		return err
	}
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", v.InternalName, goType)
	fmt.Fprintf(g.b, "\tSET\t%%%s\t%s\n", v.InternalName, val)
	return nil
}

func (g *gen) genAssignStmt(v *ast.AssignExpr) error {
	val, err := g.genValue(v.Value)
	if err != nil {
		return err
	}
	fmt.Fprintf(g.b, "\tSET\t%%%s\t%s\n", v.InternalName, val)
	return nil
}

// genIfBranch emits one IfExpr as AMIVM control flow only — its branches
// run purely for effect (genStmtBlock), and no value is captured from any
// of them. This is what a Unit-typed if/elif/else (including a discarded
// non-Unit one — see genStmt's doc comment) compiles to. `elif` was
// already desugared into a nested Else IfExpr at parse time, so the
// recursive case here is what actually emits the chain as nested
// IF/ELSE/ENDIF rather than AMIVM's ELIF (CLAUDE.md's "過去に踏まれた地雷"
// #2).
func (g *gen) genIfBranch(v *ast.IfExpr) error {
	condVal, err := g.genValue(v.Cond)
	if err != nil {
		return err
	}
	fmt.Fprintf(g.b, "\tIF\t%s\n", condVal)
	if err := g.genStmtBlock(v.Then.Exprs); err != nil {
		return err
	}
	if err := g.genElseStmt(v.Else); err != nil {
		return err
	}
	g.b.WriteString("\tENDIF\n")
	return nil
}

func (g *gen) genElseStmt(e ast.ElseBody) error {
	switch v := e.(type) {
	case nil:
		return nil
	case *ast.Block:
		g.b.WriteString("\tELSE\n")
		return g.genStmtBlock(v.Exprs)
	case *ast.IfExpr:
		g.b.WriteString("\tELSE\n")
		return g.genIfBranch(v)
	default:
		return fmt.Errorf("codegen: unexpected else-body %T", e)
	}
}

// genIfValue emits a value-producing IfExpr (sema guarantees it has an
// else — see genIfValueBranch's default case): AMIVM's IF has no value of
// its own (it's pure control flow, unlike Go's own if-as-expression-
// adjacent ternary), so the result has to flow out through a
// pre-declared temp that every branch SETs as its last step instead.
func (g *gen) genIfValue(v *ast.IfExpr) (string, error) {
	goType, ok := goTypeNames[v.ResolvedType]
	if !ok {
		return "", fmt.Errorf("codegen: unknown type %q (sema should have rejected this)", v.ResolvedType)
	}
	dest := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", dest, goType)
	if err := g.genIfValueBranch(v, dest); err != nil {
		return "", err
	}
	return "%" + dest, nil
}

func (g *gen) genIfValueBranch(v *ast.IfExpr, dest string) error {
	condVal, err := g.genValue(v.Cond)
	if err != nil {
		return err
	}
	fmt.Fprintf(g.b, "\tIF\t%s\n", condVal)
	if err := g.genValueBlock(v.Then.Exprs, dest); err != nil {
		return err
	}
	switch e := v.Else.(type) {
	case *ast.Block:
		g.b.WriteString("\tELSE\n")
		if err := g.genValueBlock(e.Exprs, dest); err != nil {
			return err
		}
	case *ast.IfExpr:
		g.b.WriteString("\tELSE\n")
		if err := g.genIfValueBranch(e, dest); err != nil {
			return err
		}
	default:
		return fmt.Errorf("codegen: if used as a value must have an else (sema should have rejected this)")
	}
	g.b.WriteString("\tENDIF\n")
	return nil
}

// genWhileStmt lowers `while cond { ... }` (amifl-spec.md section 7) into
// AMIVM's documented while-loop pattern (`ignored/amivm/amivm_spec.md`
// section 4.11: "条件付きループ(while相当)はLOOPの中でIFとBREAKを組み合わ
// せて表現する") — AMIVM has no conditional-loop instruction of its own.
// cond is (re)computed fresh at the top of every iteration since its
// instructions are emitted inside the LOOP/ENDLOOP span, and Go's native
// `continue` (AMIVM's CONTINUE) jumps back to exactly that recheck — while
// never needs the goto-based continue-target workaround CLAUDE.md's "過去
// に踏まれた地雷" #3 warns about, because that problem is specifically
// about a loop with required work *after* the body (a for-loop's index
// increment), and a while's only "required work" (the condition check) is
// already at the top.
func (g *gen) genWhileStmt(v *ast.WhileExpr) error {
	g.b.WriteString("\tLOOP\n")
	condVal, err := g.genValue(v.Cond)
	if err != nil {
		return err
	}
	notTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^bool\n", notTmp)
	fmt.Fprintf(g.b, "\tNOT\t%%%s\t%s\n", notTmp, condVal)
	fmt.Fprintf(g.b, "\tIF\t%%%s\n", notTmp)
	g.b.WriteString("\tBREAK\n")
	g.b.WriteString("\tENDIF\n")
	if err := g.genStmtBlock(v.Body.Exprs); err != nil {
		return err
	}
	g.b.WriteString("\tENDLOOP\n")
	return nil
}

// genValue returns the AMIVM value token for e: a literal, a variable
// reference, or (for a const) the inlined literal it stands for.
func (g *gen) genValue(e ast.Expr) (string, error) {
	switch v := e.(type) {
	case *ast.IntLit:
		return strconv.FormatUint(v.Value, 10), nil
	case *ast.FloatLit:
		return formatFloatLit(v.Value), nil
	case *ast.BoolLit:
		if v.Value {
			return "true", nil
		}
		return "false", nil
	case *ast.StringLit:
		return strconv.Quote(v.Value), nil
	case *ast.IdentExpr:
		if v.ConstValue != nil {
			return g.genValue(v.ConstValue)
		}
		return "%" + v.InternalName, nil
	case *ast.BinaryExpr:
		return g.genBinaryValue(v)
	case *ast.UnaryExpr:
		return g.genUnaryValue(v)
	case *ast.IfExpr:
		return g.genIfValue(v)
	default:
		return "", fmt.Errorf("codegen: %T is not a value expression (sema should have rejected this)", e)
	}
}

// binaryInstr maps a BinaryExpr's Op to its AMIVM instruction. "+" is
// special-cased in genBinaryValue: it's ADD for Numeric operands but
// CONCAT for String's Concatenable "+" (amifl-spec.md section 6).
var binaryInstr = map[string]string{
	"+": "ADD", "-": "SUB", "*": "MUL", "/": "DIV", "%": "MOD",
	"&": "BAND", "|": "BOR", "^": "BXOR", "&^": "BCLEAR",
	"<<": "SHL", ">>": "SHR",
	"==": "EQ", "!=": "NEQ", "<": "LT", "<=": "LTE", ">": "GT", ">=": "GTE",
	"&&": "AND", "||": "OR",
}

// binaryResultIsBool is the set of operators whose AMIVM instruction
// always produces a bool, regardless of their operands' (equal) type —
// comparisons and logical operators. Every other operator's result has
// the same type as its operands (BinaryExpr.ResolvedType).
var binaryResultIsBool = map[string]bool{
	"==": true, "!=": true, "<": true, "<=": true, ">": true, ">=": true,
	"&&": true, "||": true,
}

// genBinaryValue lowers a BinaryExpr to a sequence of instructions
// computing it into a fresh temp variable, returning that variable as the
// value token for the enclosing context — AMIVM's arithmetic/comparison/
// logical instructions are strictly three-address (`single = a op b`),
// so a nested expression like `a + b * c` has to be flattened into one
// instruction per operator, each writing to its own temp.
func (g *gen) genBinaryValue(v *ast.BinaryExpr) (string, error) {
	leftVal, err := g.genValue(v.Left)
	if err != nil {
		return "", err
	}
	rightVal, err := g.genValue(v.Right)
	if err != nil {
		return "", err
	}

	instr, ok := binaryInstr[v.Op]
	if !ok {
		return "", fmt.Errorf("codegen: unsupported operator %q", v.Op)
	}
	if v.Op == "+" && v.ResolvedType == "String" {
		instr = "CONCAT"
	}

	resultType := v.ResolvedType
	if binaryResultIsBool[v.Op] {
		resultType = "Bool"
	}
	goType, ok := goTypeNames[resultType]
	if !ok {
		return "", fmt.Errorf("codegen: unknown type %q (sema should have rejected this)", resultType)
	}

	tmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", tmp, goType)
	fmt.Fprintf(g.b, "\t%s\t%%%s\t%s\t%s\n", instr, tmp, leftVal, rightVal)
	return "%" + tmp, nil
}

// unaryInstr maps a UnaryExpr's Op to its AMIVM instruction. There is no
// entry for "-": AMIVM has no unary-negate instruction (only BNOT/NOT for
// ~/!), so genUnaryValue synthesizes it instead — either as an inline
// negative literal token when possible, or via SUB otherwise (see
// genUnaryValue's doc comment).
var unaryInstr = map[string]string{
	"!": "NOT",
	"~": "BNOT",
}

// genUnaryValue lowers a UnaryExpr. `!`/`~` map directly to AMIVM's
// NOT/BNOT. `-` has no AMIVM instruction of its own (CLAUDE.md's "AMIVM-IR
// の書き方" instruction list has no unary negate) — but amivm's `integer`/
// `number` operand categories directly accept a leading '-'
// (`ignored/amivm/amivm_spec.md` section 5's `integer1 integer2` /
// `number1 number2` rows), so a `-` applied to a literal (or a chain of
// `-` over a literal, or a const that ultimately is one — literalToken
// unwraps all of that) needs no instruction at all: it's just rendered as
// a signed literal token, exactly like any other literal. Only `-` applied
// to a non-literal value (a variable, a call, another operator's result)
// needs an actual instruction, synthesized as `0 - operand` since that's
// the one arithmetic instruction AMIVM does have.
func (g *gen) genUnaryValue(v *ast.UnaryExpr) (string, error) {
	if v.Op == "-" {
		if tok, ok := literalToken(v, false); ok {
			return tok, nil
		}
	}

	operandVal, err := g.genValue(v.Operand)
	if err != nil {
		return "", err
	}
	goType, ok := goTypeNames[v.ResolvedType]
	if !ok {
		return "", fmt.Errorf("codegen: unknown type %q (sema should have rejected this)", v.ResolvedType)
	}

	tmp := g.newTemp()
	switch v.Op {
	case "!", "~":
		fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", tmp, goType)
		fmt.Fprintf(g.b, "\t%s\t%%%s\t%s\n", unaryInstr[v.Op], tmp, operandVal)
	case "-":
		zero := "0"
		if v.ResolvedType == "Float32" || v.ResolvedType == "Float64" {
			zero = "0.0"
		}
		fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", tmp, goType)
		fmt.Fprintf(g.b, "\tSUB\t%%%s\t%s\t%s\n", tmp, zero, operandVal)
	default:
		return "", fmt.Errorf("codegen: unsupported unary operator %q", v.Op)
	}
	return "%" + tmp, nil
}

// literalToken tries to render e as a single inline signed AMIVM literal
// token, following const references and collapsing any chain of unary `-`
// over a literal (toggling negate each time) into one sign. It reports
// false when e isn't reducible this way (a variable, a call, a binary
// expression, ...), meaning the caller must fall back to emitting real
// instructions.
func literalToken(e ast.Expr, negate bool) (string, bool) {
	switch v := e.(type) {
	case *ast.IntLit:
		s := strconv.FormatUint(v.Value, 10)
		if negate {
			return "-" + s, true
		}
		return s, true
	case *ast.FloatLit:
		s := formatFloatLit(v.Value)
		if negate {
			return "-" + s, true
		}
		return s, true
	case *ast.IdentExpr:
		if v.ConstValue != nil {
			return literalToken(v.ConstValue, negate)
		}
		return "", false
	case *ast.UnaryExpr:
		if v.Op == "-" {
			return literalToken(v.Operand, !negate)
		}
		return "", false
	default:
		return "", false
	}
}

// formatFloatLit renders v as an AMIVM float-shaped literal token,
// guaranteeing a decimal point ('f' format never uses scientific
// notation, so this can't accidentally strand a bare "e10" suffix) — a
// whole-number float like 5.0 would otherwise print as "5", which reads
// as an integer literal.
func formatFloatLit(v float64) string {
	s := strconv.FormatFloat(v, 'f', -1, 64)
	if !strings.Contains(s, ".") {
		s += ".0"
	}
	return s
}
