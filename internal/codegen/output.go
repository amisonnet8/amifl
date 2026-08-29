// output.go compiles the remainder of amifl-spec.md section 13.1 (出力・
// 終了) that ex6 adds — `print` itself stays codegen.go's own hardcoded
// `?fmt.Println` call in genCallStmt (unchanged by ex6: Println's `...any`
// parameter already accepts any concrete Go value directly, so
// generalizing print's *sema* signature from String to Any needed no
// codegen change at all). eprint/format/formatWith/exit are ordinary
// c.Builtin-dispatched entries (builtins.go's genBuiltinStmt/
// genBuiltinValue switches), same as every other section-13 function.
package codegen

import (
	"fmt"

	"github.com/amisonnet8/amifl/internal/ast"
)

// genEprintStmt emits `eprint(v)` (amifl-spec.md section 13.1) — always
// Unit-typed (sema/builtins_output.go's resolveEprint), so this is reached
// only via genBuiltinStmt, never genBuiltinValue, exactly like setAt/add/
// discard/set/delete/send/spawn above it.
func (g *gen) genEprintStmt(c *ast.CallExpr) error {
	vVal, err := g.genValue(c.Args[0])
	if err != nil {
		return err
	}
	g.writeCall("", "?amiflrt.Eprint", []string{vVal})
	return nil
}

// genFormatValue emits `format(v) -> String` (amifl-spec.md section 13.1).
func (g *gen) genFormatValue(c *ast.CallExpr) (string, error) {
	vVal, err := g.genValue(c.Args[0])
	if err != nil {
		return "", err
	}
	tmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^string\n", tmp)
	g.writeCall("%"+tmp, "?amiflrt.Format", []string{vVal})
	return "%" + tmp, nil
}

// genFormatWithValue emits `formatWith(template, v) -> String` (amifl-spec.md
// section 13.1).
func (g *gen) genFormatWithValue(c *ast.CallExpr) (string, error) {
	templateVal, err := g.genValue(c.Args[0])
	if err != nil {
		return "", err
	}
	vVal, err := g.genValue(c.Args[1])
	if err != nil {
		return "", err
	}
	tmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^string\n", tmp)
	g.writeCall("%"+tmp, "?amiflrt.FormatWith", []string{templateVal, vVal})
	return "%" + tmp, nil
}

// genExitStmt emits `exit(code)` (amifl-spec.md section 13.1) — always
// Unit-typed (sema/builtins_output.go's resolveExit), reached only via
// genBuiltinStmt. code's AmiFL type is fixed Int64, but Go's os.Exit takes
// the platform-native (width-unspecified) `int` — the exact same mismatch
// codegen.go's own `!main` wrapper already bridges for its own exit code,
// via the identical CALL-as-Go-type-conversion cast (CLAUDE.md's "過去に
// 踏まれた地雷" #5) rather than a new mechanism.
func (g *gen) genExitStmt(c *ast.CallExpr) error {
	codeVal, err := g.genValue(c.Args[0])
	if err != nil {
		return err
	}
	tmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^int\n", tmp)
	g.writeCall("%"+tmp, "?int", []string{codeVal})
	g.writeCall("", "?os.Exit", []string{"%" + tmp})
	return nil
}
