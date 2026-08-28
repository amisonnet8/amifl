// builtins_numeric.go type-checks amifl-spec.md section 13.7's numeric
// built-ins and section 13.9's error-handling built-ins (step 11 phase
// 11d, the last phase of step 11) — same discipline as every other
// builtins_*.go file: inspect the already-resolved argument type(s), pick
// the one capability group a call belongs to, record what codegen needs.
package sema

import (
	"fmt"

	"github.com/amisonnet8/amifl/internal/ast"
)

func init() {
	for name, resolver := range map[string]builtinResolver{
		"min":   resolveMin,
		"max":   resolveMax,
		"abs":   resolveAbs,
		"clamp": resolveClamp,
		"round": resolveRound,
		"floor": resolveFloor,
		"ceil":  resolveCeil,
		"pow":   resolvePow,
		"sqrt":  resolveSqrt,
	} {
		builtinFuncs[name] = resolver
	}
	// unwrap/okOr (section 13.9) replace builtins.go's phase-11a
	// notYetImplemented placeholders now that this phase is wiring them up.
	builtinFuncs["unwrap"] = resolveUnwrap
	builtinFuncs["okOr"] = resolveOkOr
}

// resolveMinMax is shared by min/max — both `(a: Numeric, b: Numeric) ->
// Numeric` (amifl-spec.md section 13.7), restricted to Numeric rather than
// the broader Ordered capability (2.3節) since 13.7 is specifically the
// numeric-functions section — String comparison has no "min"/"max" listed
// there. resolveOperandTypes (binary-operator literal adaptation, step 3)
// is reused here so `min(x, 0)` adapts the literal `0` to x's own type
// exactly the way `x < 0` already would.
func resolveMinMax(fc *funcChecker, v *ast.CallExpr, name string) (string, error) {
	if len(v.Args) != 2 {
		return "", arityError(v, 2)
	}
	aTyp, bTyp, err := fc.resolveOperandTypes(v.Args[0], v.Args[1], "")
	if err != nil {
		return "", err
	}
	if aTyp != bTyp {
		return "", fmt.Errorf("line %d: %s requires both arguments to have the same type, got %s and %s", v.Line, name, aTyp, bTyp)
	}
	if !isIntType(aTyp) && !isFloatType(aTyp) {
		return "", fmt.Errorf("line %d: %s: %s isn't a Numeric type", v.Line, name, aTyp)
	}
	v.ArgTypes = []string{aTyp, bTyp}
	return aTyp, nil
}

func resolveMin(fc *funcChecker, v *ast.CallExpr) (string, error) { return resolveMinMax(fc, v, "min") }
func resolveMax(fc *funcChecker, v *ast.CallExpr) (string, error) { return resolveMinMax(fc, v, "max") }

// resolveAbs type-checks `abs(v) -> Numeric` (amifl-spec.md section 13.7).
func resolveAbs(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 1 {
		return "", arityError(v, 1)
	}
	typ, err := fc.checkExpr(v.Args[0], "")
	if err != nil {
		return "", err
	}
	if !isIntType(typ) && !isFloatType(typ) {
		return "", fmt.Errorf("line %d: abs: %s isn't a Numeric type", v.Line, typ)
	}
	v.ArgTypes = []string{typ}
	return typ, nil
}

// resolveClamp type-checks `clamp(v, lo, hi) -> Numeric` (amifl-spec.md
// section 13.7) — lo/hi both adapt to v's own type (v resolved first,
// unconditionally, unlike min/max's symmetric pair — clamp's own argument
// order already puts the single "data" value first per 9.2節's now-settled
// convention, so there's no ambiguity about which side to resolve first
// the way a fully symmetric 2-arg function has).
func resolveClamp(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 3 {
		return "", arityError(v, 3)
	}
	vTyp, err := fc.checkExpr(v.Args[0], "")
	if err != nil {
		return "", err
	}
	if !isIntType(vTyp) && !isFloatType(vTyp) {
		return "", fmt.Errorf("line %d: clamp: %s isn't a Numeric type", v.Line, vTyp)
	}
	if _, err := fc.checkExpr(v.Args[1], vTyp); err != nil {
		return "", err
	}
	if _, err := fc.checkExpr(v.Args[2], vTyp); err != nil {
		return "", err
	}
	v.ArgTypes = []string{vTyp, vTyp, vTyp}
	return vTyp, nil
}

// resolveFloatUnary is shared by round/floor/ceil/sqrt — all `(v: Float)
// -> Float` (amifl-spec.md section 13.7), restricted to the Float family
// (an Int's round/floor/ceil is itself, and sqrt of an Int isn't
// generally an Int — both are deliberately kept out of scope rather than
// silently widening/narrowing, principle 2).
func resolveFloatUnary(v *ast.CallExpr, fc *funcChecker, name string) (string, error) {
	if len(v.Args) != 1 {
		return "", arityError(v, 1)
	}
	typ, err := fc.checkExpr(v.Args[0], "")
	if err != nil {
		return "", err
	}
	if !isFloatType(typ) {
		return "", fmt.Errorf("line %d: %s: %s isn't a Float type", v.Line, name, typ)
	}
	v.ArgTypes = []string{typ}
	return typ, nil
}

func resolveRound(fc *funcChecker, v *ast.CallExpr) (string, error) {
	return resolveFloatUnary(v, fc, "round")
}
func resolveFloor(fc *funcChecker, v *ast.CallExpr) (string, error) {
	return resolveFloatUnary(v, fc, "floor")
}
func resolveCeil(fc *funcChecker, v *ast.CallExpr) (string, error) {
	return resolveFloatUnary(v, fc, "ceil")
}
func resolveSqrt(fc *funcChecker, v *ast.CallExpr) (string, error) {
	return resolveFloatUnary(v, fc, "sqrt")
}

// resolvePow type-checks `pow(base, exp) -> Float` (amifl-spec.md section
// 13.7) — both arguments restricted to the same Float type (Go's
// math.Pow's own (float64,float64)->float64 shape, generalized to
// Float32 via a narrowing cast the same way parse[T] already narrows
// strconv's fixed-width results).
func resolvePow(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 2 {
		return "", arityError(v, 2)
	}
	baseTyp, expTyp, err := fc.resolveOperandTypes(v.Args[0], v.Args[1], "")
	if err != nil {
		return "", err
	}
	if baseTyp != expTyp {
		return "", fmt.Errorf("line %d: pow requires both arguments to have the same type, got %s and %s", v.Line, baseTyp, expTyp)
	}
	if !isFloatType(baseTyp) {
		return "", fmt.Errorf("line %d: pow: %s isn't a Float type", v.Line, baseTyp)
	}
	v.ArgTypes = []string{baseTyp, expTyp}
	return baseTyp, nil
}

// resolveUnwrap type-checks `unwrap[T](t: Tuple2[T,Error]) -> T`
// (amifl-spec.md section 13.9) — panics at runtime if t's error is
// non-nil ("エラーがあればその場でパニック", prototyping-only per the
// spec's own description).
func resolveUnwrap(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 1 {
		return "", arityError(v, 1)
	}
	tTyp, err := fc.checkExpr(v.Args[0], "")
	if err != nil {
		return "", err
	}
	payload, ok := tuple2ErrorPayload(tTyp)
	if !ok {
		return "", fmt.Errorf("line %d: unwrap: %s isn't Tuple2[T,Error]", v.Line, tTyp)
	}
	if v.TypeArg != nil {
		targetTyp, err := fc.resolveTypeExpr(v.TypeArg)
		if err != nil {
			return "", err
		}
		if targetTyp != payload {
			return "", fmt.Errorf("line %d: unwrap[%s]: argument's payload type is %s, not %s", v.Line, targetTyp, payload, targetTyp)
		}
		v.ResolvedTypeArg = targetTyp
	}
	v.ArgTypes = []string{tTyp}
	return payload, nil
}

// resolveOkOr type-checks `okOr[T](t: Tuple2[T,Error], default: T) -> T`
// (amifl-spec.md section 13.9).
func resolveOkOr(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 2 {
		return "", arityError(v, 2)
	}
	tTyp, err := fc.checkExpr(v.Args[0], "")
	if err != nil {
		return "", err
	}
	payload, ok := tuple2ErrorPayload(tTyp)
	if !ok {
		return "", fmt.Errorf("line %d: okOr: %s isn't Tuple2[T,Error]", v.Line, tTyp)
	}
	if _, err := fc.checkExpr(v.Args[1], payload); err != nil {
		return "", err
	}
	if v.TypeArg != nil {
		targetTyp, err := fc.resolveTypeExpr(v.TypeArg)
		if err != nil {
			return "", err
		}
		if targetTyp != payload {
			return "", fmt.Errorf("line %d: okOr[%s]: argument's payload type is %s, not %s", v.Line, targetTyp, payload, targetTyp)
		}
		v.ResolvedTypeArg = targetTyp
	}
	v.ArgTypes = []string{tTyp, payload}
	return payload, nil
}
