package codegen

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/amisonnet8/amifl/internal/ast"
)

// entryFunc is the AMIVM function name user code's `fn main` compiles to.
// AmiFL's `main` returns Int (and, from step 7, takes List[String] args),
// which is incompatible with Go's argument-less, return-less
// `func main()` — so, following Seed/Cascade/Weave's precedent (CLAUDE.md
// "過去に踏まれた地雷"), user code compiles under this internal name and
// Generate emits a thin `!main` wrapper that calls it and passes the
// result to os.Exit. Must match sema's reservedMainName constant — see
// its doc comment for why the two packages each keep their own copy
// rather than sharing one.
const entryFunc = "amifl_main"

// unitType mirrors sema's unitType sentinel string exactly (ast is the
// only vocabulary codegen and sema share — see CLAUDE.md's リポジトリ構成
// — so this is a second, independent copy of the same convention, not an
// import). Step 5 is the first time codegen itself needs to recognize
// "Unit" as a value: a function/closure whose return type resolved to
// Unit compiles to a Go function with zero result values (no `goTypeNames`
// entry exists for it, nor should one — Unit has no runtime
// representation at all, amifl-spec.md section 2.2).
const unitType = "Unit"

// goTypeNames maps AmiFL's canonical scalar type names (sema has already
// resolved aliases like "Int" -> "Int64" by the time codegen sees them)
// to the Go type name AMIVM should declare.
var goTypeNames = map[string]string{
	"Int8": "int8", "Int16": "int16", "Int32": "int32", "Int64": "int64",
	"UInt8": "uint8", "UInt16": "uint16", "UInt32": "uint32", "UInt64": "uint64",
	"Float32": "float32", "Float64": "float64",
	"Bool": "bool", "String": "string",
}

// Generate lowers a sema-checked AmiFL file into AMIVM-IR text. Step 5
// generalizes this from "compile the single `fn main`" to "compile every
// top-level `fn`" — each becomes its own FUNC/ENDFUNC block (main's own
// under its internal entryFunc name, same as before), in file order.
func Generate(f *ast.File) (string, error) {
	main := findMain(f)
	if main == nil {
		return "", fmt.Errorf("codegen: no fn main (sema should have rejected this)")
	}

	prog := &program{}
	// User `struct` declarations are emitted first, ahead of every
	// function body — see genStructDecl's doc comment.
	for _, decl := range f.Decls {
		if st, ok := decl.(*ast.StructDecl); ok {
			genStructDecl(prog, st)
		}
	}

	var funcsBuf strings.Builder
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if err := genFuncDecl(prog, &funcsBuf, fn); err != nil {
			return "", err
		}
		funcsBuf.WriteString("\n")
	}

	var b strings.Builder
	// Any closure literal encountered while generating the functions above
	// registered its shape's FNTYPE line into prog — emitted here, ahead
	// of every FUNC block, matching every amivm FNTYPE/CLOS example seen
	// (CLAUDE.md's AMIVM-IR reference); Go's own package-level
	// declarations are order-independent, but nothing establishes amivm's
	// own IR parsing is, so this plays it safe rather than relying on that.
	if prog.typeHeader.Len() > 0 {
		b.WriteString(prog.typeHeader.String())
		b.WriteString("\n")
	}
	b.WriteString(funcsBuf.String())

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

// genFuncDecl emits one top-level `fn` as `FUNC !name paramType1 ... :
// [retType] ... ENDFUNC` into out, using name (fn.Name, or entryFunc for
// the user's own `fn main` — see entryFunc's doc comment). A Unit-
// returning function has no result type at all in its FUNC header (Go's
// own "func f(...) { ... }" with no result list) and its body runs
// entirely for effect (genStmtBlock, exactly like a while body) followed
// by a bare RET (amifl-spec.md section 2.2, Unit has no runtime value to
// return) — every other function keeps step 1's genBlock (tail expression
// becomes RET's operand).
func genFuncDecl(prog *program, out *strings.Builder, fn *ast.FuncDecl) error {
	name := fn.Name
	if name == "main" {
		name = entryFunc
	}

	out.WriteString("FUNC\t!" + name)
	for _, p := range fn.Params {
		out.WriteString("\t^" + prog.resolveGoType(p.ResolvedType))
	}
	out.WriteString("\t:")
	if fn.ResolvedReturnType != unitType {
		out.WriteString("\t^" + prog.resolveGoType(fn.ResolvedReturnType))
	}
	out.WriteString("\n")

	g := &gen{b: out, prog: prog}
	if fn.ResolvedReturnType == unitType {
		if err := g.genStmtBlock(fn.Body.Exprs); err != nil {
			return err
		}
		out.WriteString("\tRET\n")
	} else if err := g.genBlock(fn.Body.Exprs); err != nil {
		return err
	}
	out.WriteString("ENDFUNC\n")
	return nil
}

// gen holds the per-function codegen state. Step 4's if/while compile to
// AMIVM's IF/LOOP, which are themselves genuinely nested Go blocks (not
// goto-based), so there is still nothing to hoist VAR declarations past —
// each `let` emits its VAR right where it's declared, at whatever nesting
// depth that is. Revisit this once an actually goto-producing construct
// (switch's break-out-of-multiple-cases, for-in's continue) exists; see
// CLAUDE.md's "過去に踏まれた地雷" #1.
//
// Step 5's CLOS is, like IF/LOOP, a genuinely nested Go block (a nested
// Go func literal) — so a closure literal's body is generated by
// recursing into the *same* gen (see genClosureLitInto in closure.go),
// sharing tmpSeq (Go temp names climbing monotonically across an outer
// function and any closures nested in it causes no collision — they're
// simply different, non-overlapping numbers, and Go's own block scoping
// keeps them in fully separate namespaces regardless) and prog (so every
// closure's FNTYPE, at any nesting depth, lands in the same shared
// top-level header — see Generate).
type gen struct {
	b      *strings.Builder
	tmpSeq int
	prog   *program
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
	case *ast.TupleLit, *ast.StructLit, *ast.FieldExpr:
		// Unlike the pure-value kinds above, a tuple/struct literal's
		// elements/fields (or a field-access target) may themselves be an
		// effectful call — `_ = Point{x: sideEffecting(), y: 2}` must still
		// run sideEffecting() even though the resulting Point is discarded —
		// so this constructs the value in full via the normal genValue path
		// and discards the token it hands back (an unused temp amivm's own
		// self-healing cleans up, same as every other discard here —
		// CLAUDE.md's step-2 "確定した設計判断").
		_, err := g.genValue(v)
		return err
	default:
		return fmt.Errorf("codegen: unsupported statement %T", e)
	}
}

// genCallStmt emits `callee(args...)` purely for effect, discarding any
// result — a bare CALL with no result operand (Go's own "callee(args)"
// used as a statement, discarding whatever it returns, is always legal
// regardless of the callee's return type), so this same code path handles
// a Unit-returning call (the only kind sema lets appear undiscarded in
// statement position) and a discarded non-Unit one (reached via
// DiscardExpr -> genStmt's recursion) identically — see calleeToken for
// how print/closure-call/top-level-fn-call are told apart.
func (g *gen) genCallStmt(c *ast.CallExpr) error {
	if c.Callee == "print" {
		arg, err := g.genValue(c.Args[0])
		if err != nil {
			return err
		}
		g.writeCall("", "?fmt.Println", []string{arg})
		return nil
	}
	calleeToken, err := g.calleeToken(c)
	if err != nil {
		return err
	}
	argVals, err := g.genArgValues(c.Args)
	if err != nil {
		return err
	}
	g.writeCall("", calleeToken, argVals)
	return nil
}

// genCallValue is genCallStmt's counterpart for a call used as a value
// (its return type is non-Unit): declares a fresh temp of the call's
// result type and receives the CALL's result into it.
func (g *gen) genCallValue(c *ast.CallExpr) (string, error) {
	calleeToken, err := g.calleeToken(c)
	if err != nil {
		return "", err
	}
	argVals, err := g.genArgValues(c.Args)
	if err != nil {
		return "", err
	}
	goType := g.prog.resolveGoType(c.ResolvedType)
	tmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", tmp, goType)
	g.writeCall("%"+tmp, calleeToken, argVals)
	return "%" + tmp, nil
}

// calleeToken returns c's callname operand. sema's resolveCallExpr sets
// CalleeToken only for a call through a closure-valued variable (its
// binding's own token, whatever shape — "%f_3"/"$1"/"&1-1"); anything
// else must be a top-level `fn` call by name, so codegen derives "!" +
// name here directly, substituting the internal entry-point name for
// "main" (ast.CallExpr.CalleeToken's doc comment explains why this
// substitution lives here rather than in sema).
func (g *gen) calleeToken(c *ast.CallExpr) (string, error) {
	if c.CalleeToken != "" {
		return c.CalleeToken, nil
	}
	name := c.Callee
	if name == "main" {
		name = entryFunc
	}
	return "!" + name, nil
}

func (g *gen) genArgValues(args []ast.Expr) ([]string, error) {
	vals := make([]string, len(args))
	for i, a := range args {
		v, err := g.genValue(a)
		if err != nil {
			return nil, err
		}
		vals[i] = v
	}
	return vals, nil
}

// writeCall emits one CALL instruction. dest is the result operand
// ("%tmp") or "" to discard the result entirely (amivm_spec.md section
// 4.13: "multi1が無い場合はcallname(value1, value2 ...)").
func (g *gen) writeCall(dest, calleeToken string, args []string) {
	g.b.WriteString("\tCALL\t")
	if dest != "" {
		g.b.WriteString(dest)
		g.b.WriteString("\t")
	}
	g.b.WriteString(":\t")
	g.b.WriteString(calleeToken)
	for _, a := range args {
		g.b.WriteString("\t")
		g.b.WriteString(a)
	}
	g.b.WriteString("\n")
}

// genLetStmt emits a `let`'s VAR/SET under its Token (sema's
// freshInternalName, prefixed with "%"), never its bare surface Name —
// see ast.LetExpr.Token for why a shadowing `let` needs a distinct Go
// name even though AmiFL itself lets it reuse the outer name. A closure-
// literal value is handled entirely separately (genClosureLitInto,
// closure.go): CLOS emits directly into a pre-declared target rather than
// producing a value genValue can hand back and SET afterward the way
// every other expression kind does.
func (g *gen) genLetStmt(v *ast.LetExpr) error {
	if clos, ok := v.Value.(*ast.ClosureLit); ok {
		return g.genClosureLitInto(v.Token, clos)
	}
	goType := g.prog.resolveGoType(v.ResolvedType)
	val, err := g.genValue(v.Value)
	if err != nil {
		return err
	}
	fmt.Fprintf(g.b, "\tVAR\t%s\t^%s\n", v.Token, goType)
	fmt.Fprintf(g.b, "\tSET\t%s\t%s\n", v.Token, val)
	return nil
}

func (g *gen) genAssignStmt(v *ast.AssignExpr) error {
	val, err := g.genValue(v.Value)
	if err != nil {
		return err
	}
	fmt.Fprintf(g.b, "\tSET\t%s\t%s\n", v.Token, val)
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
	goType := g.prog.resolveGoType(v.ResolvedType)
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
		return v.Token, nil
	case *ast.BinaryExpr:
		return g.genBinaryValue(v)
	case *ast.UnaryExpr:
		return g.genUnaryValue(v)
	case *ast.IfExpr:
		return g.genIfValue(v)
	case *ast.CallExpr:
		return g.genCallValue(v)
	case *ast.TupleLit:
		return g.genTupleLitValue(v)
	case *ast.StructLit:
		return g.genStructLitValue(v)
	case *ast.FieldExpr:
		return g.genFieldValue(v)
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
