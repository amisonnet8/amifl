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

// Generate lowers a sema-checked AmiFL file into AMIVM-IR text.
func Generate(f *ast.File) (string, error) {
	main := findMain(f)
	if main == nil {
		return "", fmt.Errorf("codegen: no fn main (sema should have rejected this)")
	}

	var b strings.Builder
	fmt.Fprintf(&b, "FUNC\t!%s\t:\t^int64\n", entryFunc)
	for _, e := range main.Body.Exprs {
		if err := genMainStmt(&b, e); err != nil {
			return "", err
		}
	}
	b.WriteString("ENDFUNC\n\n")

	// os.Exit takes Go's native (platform-width) int, not AmiFL's fixed-
	// width Int64 — %code is cast down via the CALL-as-Go-type-conversion
	// trick (CLAUDE.md's "過去に踏まれた地雷" #5) before being passed on.
	// Found by actually running this through amivm -> go build (Seed's
	// §6.1 lesson): "cannot use ... (variable of type int64) as int
	// value" only shows up once go/types sees the generated code.
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
	for _, fn := range f.Funcs {
		if fn.Name == "main" {
			return fn
		}
	}
	return nil
}

func genMainStmt(b *strings.Builder, e ast.Expr) error {
	switch v := e.(type) {
	case *ast.IntLit:
		fmt.Fprintf(b, "\tRET\t%d\n", v.Value)
		return nil
	case *ast.CallExpr:
		return genCallStmt(b, v)
	default:
		return fmt.Errorf("codegen: unsupported expression %T", e)
	}
}

func genCallStmt(b *strings.Builder, c *ast.CallExpr) error {
	if c.Callee != "print" {
		return fmt.Errorf("codegen: unsupported call %q (sema should have rejected this)", c.Callee)
	}
	s, ok := c.Args[0].(*ast.StringLit)
	if !ok {
		return fmt.Errorf("codegen: print argument must be a string literal (sema should have rejected this)")
	}
	// The operand is passed as strconv.Quote's own %s argument, never
	// concatenated into the format string — an operand containing a
	// literal '%' would otherwise be misread as a format verb
	// (CLAUDE.md's "過去に踏まれた地雷" #12).
	fmt.Fprintf(b, "\tCALL\t:\t?fmt.Println\t%s\n", strconv.Quote(s.Value))
	return nil
}
