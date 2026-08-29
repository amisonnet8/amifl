// builtins_pipeline.go type-checks amifl-spec.md section 13.8's pipeline-DX
// helpers (design issue 8, step 15's own scope per CLAUDE.md's
// implementation plan, deferred out of step 11/12 despite living in the
// same spec section as the rest of 13.8's Chan/Stream built-ins). tap/peek
// are both `(v: T, ...) -> T` over an entirely unconstrained T — unlike
// every other built-in here, there's no capability group to dispatch on
// (2.3節 doesn't list one), so there's nothing to check about T itself
// beyond letting it resolve to whatever it already is.
package sema

import (
	"fmt"

	"github.com/amisonnet8/amifl/internal/ast"
)

func init() {
	builtinFuncs["tap"] = resolveTap
	builtinFuncs["peek"] = resolvePeek
}

// resolveTap type-checks `tap(v: T, label: String) -> T` (amifl-spec.md
// section 13.8).
func resolveTap(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 2 {
		return "", arityError(v, 2)
	}
	vTyp, err := fc.checkExpr(v.Args[0], "")
	if err != nil {
		return "", err
	}
	if vTyp == unitType {
		return "", fmt.Errorf("line %d: tap: v must not be Unit-typed (nothing to pass through)", v.Line)
	}
	if _, err := fc.checkExprPipeAware(v, 1, v.Args[1], "String"); err != nil {
		return "", err
	}
	v.ArgTypes = []string{vTyp, "String"}
	return vTyp, nil
}

// resolvePeek type-checks `peek(v: T) -> T` (amifl-spec.md section 13.8).
func resolvePeek(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 1 {
		return "", arityError(v, 1)
	}
	vTyp, err := fc.checkExpr(v.Args[0], "")
	if err != nil {
		return "", err
	}
	if vTyp == unitType {
		return "", fmt.Errorf("line %d: peek: v must not be Unit-typed (nothing to pass through)", v.Line)
	}
	v.ArgTypes = []string{vTyp}
	return vTyp, nil
}
