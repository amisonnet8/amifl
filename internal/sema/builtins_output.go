// builtins_output.go type-checks the remainder of amifl-spec.md section
// 13.1 (出力・終了) that ex6 implements — `print` itself stays
// resolveCallExpr's own hardcoded special case (expr.go), unchanged in
// shape since step 1, but eprint/format/formatWith/exit were never
// special-cased anywhere and so are ordinary builtinFuncs entries, exactly
// like every other section-13 function since step 11.
package sema

import (
	"fmt"

	"github.com/amisonnet8/amifl/internal/ast"
)

func init() {
	builtinFuncs["eprint"] = resolveEprint
	builtinFuncs["format"] = resolveFormat
	builtinFuncs["formatWith"] = resolveFormatWith
	builtinFuncs["exit"] = resolveExit
}

// resolveEprint type-checks `eprint(v: Any) -> Unit` (amifl-spec.md section
// 13.1) — print's own stderr counterpart, same Any-typed argument and same
// Unit-rejection (nothing to write) as print's hardcoded special case in
// expr.go's resolveCallExpr.
func resolveEprint(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 1 {
		return "", arityError(v, 1)
	}
	argTyp, err := fc.checkExpr(v.Args[0], "Any")
	if err != nil {
		return "", err
	}
	if argTyp == unitType {
		return "", fmt.Errorf("line %d: eprint: v must not be Unit-typed (nothing to print)", v.Line)
	}
	v.ArgTypes = []string{argTyp}
	return unitType, nil
}

// resolveFormat type-checks `format(v: Any) -> String` (amifl-spec.md
// section 13.1) — the same %v rendering print/eprint write directly,
// returned as a value instead.
func resolveFormat(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 1 {
		return "", arityError(v, 1)
	}
	argTyp, err := fc.checkExpr(v.Args[0], "Any")
	if err != nil {
		return "", err
	}
	if argTyp == unitType {
		return "", fmt.Errorf("line %d: format: v must not be Unit-typed (nothing to format)", v.Line)
	}
	v.ArgTypes = []string{argTyp}
	return "String", nil
}

// resolveFormatWith type-checks `formatWith(template: String, v: Any) ->
// String` (amifl-spec.md section 13.1): replaces the first "{}" in
// template with v's formatted text. AmiFL has no variadic arguments
// (principle 7), so unlike a printf-style function this only ever fills in
// one placeholder per call — filling more than one means chaining calls,
// typically via `|>` (e.g. `"{} and {}" |> formatWith(_, a) |>
// formatWith(_, b)`; each call replaces the *leftmost* remaining "{}",
// see codegen/output.go's genFormatWithValue), the same composition-over-
// a-built-in-multi-arg-form choice section 13.4's other built-ins already
// make throughout (map/filter/reduce over a single element at a time,
// chained rather than batched).
func resolveFormatWith(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 2 {
		return "", arityError(v, 2)
	}
	if _, err := fc.checkExprPipeAware(v, 0, v.Args[0], "String"); err != nil {
		return "", err
	}
	argTyp, err := fc.checkExpr(v.Args[1], "Any")
	if err != nil {
		return "", err
	}
	if argTyp == unitType {
		return "", fmt.Errorf("line %d: formatWith: v must not be Unit-typed (nothing to format)", v.Line)
	}
	v.ArgTypes = []string{"String", argTyp}
	return "String", nil
}

// resolveExit type-checks `exit(code: Int) -> Unit` (amifl-spec.md section
// 13.1) — deliberately Unit-typed rather than a Never-like "fits any
// expected type" (CLAUDE.md's ex6 design note flags this exact tension):
// AmiFL has no `return`/`Never` (section 17.1) and resolveType never
// threads an `expected` type down into resolveCallExpr/resolveBuiltinCall
// at all (every other builtin determines its own result type unassisted
// and gets checked against `expected` afterward, in checkExpr, uniformly)
// — plumbing one through for this single builtin would be a structurally
// invasive change to the call-resolution pipeline for a single narrow
// benefit. So exit is usable anywhere Unit already is: a bare statement, or
// the tail of a Unit-typed block/if-branch (`if bad { eprint("fatal");
// exit(1) } else { ... }`, both branches Unit) — but not as a same-typed
// fallback value alongside a non-Unit branch (`if ok { 5 } else {
// exit(1) }` is rejected, unlike Go's `panic` fitting any return type
// structurally). Recorded as a known limitation in amifl-spec.md section
// 17.2.
func resolveExit(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 1 {
		return "", arityError(v, 1)
	}
	if _, err := fc.checkExprPipeAware(v, 0, v.Args[0], "Int64"); err != nil {
		return "", err
	}
	v.ArgTypes = []string{"Int64"}
	return unitType, nil
}
