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

// gen holds the per-function codegen state. Step 2 has no control flow
// yet (that lands in step 4), so there is nothing to hoist VAR
// declarations past — each `let` emits its VAR right where it's
// declared. Revisit this once goto-producing constructs (switch's break,
// for-in's continue) exist; see CLAUDE.md's "過去に踏まれた地雷" #1.
type gen struct {
	b *strings.Builder
}

// genBlock emits a block's statements, treating the last expression as
// the enclosing function's return value (sema's checkBlock guarantees
// this typechecks against the function's declared return type).
func (g *gen) genBlock(exprs []ast.Expr) error {
	if len(exprs) == 0 {
		return fmt.Errorf("codegen: empty block (sema should have rejected this)")
	}
	for i, e := range exprs {
		if i == len(exprs)-1 {
			return g.genReturn(e)
		}
		if err := g.genStmt(e); err != nil {
			return err
		}
	}
	panic("unreachable")
}

func (g *gen) genReturn(e ast.Expr) error {
	val, err := g.genValue(e)
	if err != nil {
		return err
	}
	fmt.Fprintf(g.b, "\tRET\t%s\n", val)
	return nil
}

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
		return g.genDiscardStmt(v)
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

func (g *gen) genLetStmt(v *ast.LetExpr) error {
	goType, ok := goTypeNames[v.ResolvedType]
	if !ok {
		return fmt.Errorf("codegen: unknown type %q (sema should have rejected this)", v.ResolvedType)
	}
	val, err := g.genValue(v.Value)
	if err != nil {
		return err
	}
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", v.Name, goType)
	fmt.Fprintf(g.b, "\tSET\t%%%s\t%s\n", v.Name, val)
	return nil
}

func (g *gen) genAssignStmt(v *ast.AssignExpr) error {
	val, err := g.genValue(v.Value)
	if err != nil {
		return err
	}
	fmt.Fprintf(g.b, "\tSET\t%%%s\t%s\n", v.Name, val)
	return nil
}

// genDiscardStmt handles `_ = expr`. Step 2's discardable values
// (literals, variable/const reads) have no side effects, so there's
// nothing to emit for them — amivm's own unused-variable self-healing
// (CLAUDE.md's amivm reference: "未使用変数の救済方法") takes care of any
// resulting unused Go variable, so codegen doesn't need to synthesize a
// Go `_ = x` either. A discarded *call*, however, still needs its side
// effect to run — there's no such call yet since print is already
// Unit-typed, but this keeps codegen forward-compatible with step 5's
// general (non-Unit-returning) function calls.
func (g *gen) genDiscardStmt(v *ast.DiscardExpr) error {
	if c, ok := v.Value.(*ast.CallExpr); ok {
		return g.genCallStmt(c)
	}
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
		return "%" + v.Name, nil
	default:
		return "", fmt.Errorf("codegen: %T is not a value expression (sema should have rejected this)", e)
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
