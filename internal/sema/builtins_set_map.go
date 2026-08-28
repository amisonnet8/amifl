// builtins_set_map.go type-checks amifl-spec.md sections 13.5 (Set[T])
// and 13.6 (Map[K,V]) built-ins — step 11 phase 11c, following the same
// "inspect the already-resolved argument type, pick the one capability
// group it belongs to" discipline as builtins_data.go's 13.4 resolvers.
package sema

import (
	"fmt"

	"github.com/amisonnet8/amifl/internal/ast"
)

func init() {
	for name, resolver := range map[string]builtinResolver{
		"add":        resolveSetAdd,
		"discard":    resolveSetDiscard,
		"union":      resolveSetUnion,
		"intersect":  resolveSetIntersect,
		"difference": resolveSetDifference,
		"toList":     resolveSetToList,
		"keys":       resolveMapKeys,
		"values":     resolveMapValues,
		"entries":    resolveMapEntries,
		"get":        resolveMapGet,
		"set":        resolveMapSet,
		"delete":     resolveMapDelete,
	} {
		builtinFuncs[name] = resolver
	}
}

// resolveSetAdd type-checks `add(s, v) -> Unit` (amifl-spec.md section
// 13.5) — mutates s in place (a Set is a Go map under the hood, step 10;
// mutating in place is exactly how Map's own set()/delete() already
// behave, so Set's add/discard follow the same convention rather than
// push/pop's non-destructive one — a Set has no "position" to preserve the
// way a List does, so there's no equivalent expectation of immutability).
func resolveSetAdd(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 2 {
		return "", arityError(v, 2)
	}
	sTyp, err := fc.checkExpr(v.Args[0], "")
	if err != nil {
		return "", err
	}
	elem, ok := setElemType(sTyp)
	if !ok {
		return "", fmt.Errorf("line %d: add: unsupported type %s (must be a Set)", v.Line, sTyp)
	}
	if _, err := fc.checkExpr(v.Args[1], elem); err != nil {
		return "", err
	}
	v.ArgTypes = []string{sTyp, elem}
	return unitType, nil
}

// resolveSetDiscard type-checks `discard(s, v) -> Unit` (amifl-spec.md
// section 13.5).
func resolveSetDiscard(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 2 {
		return "", arityError(v, 2)
	}
	sTyp, err := fc.checkExpr(v.Args[0], "")
	if err != nil {
		return "", err
	}
	elem, ok := setElemType(sTyp)
	if !ok {
		return "", fmt.Errorf("line %d: discard: unsupported type %s (must be a Set)", v.Line, sTyp)
	}
	if _, err := fc.checkExpr(v.Args[1], elem); err != nil {
		return "", err
	}
	v.ArgTypes = []string{sTyp, elem}
	return unitType, nil
}

// resolveSetBinaryOp is shared by union/intersect/difference — all
// `(a: Set[T], b: Set[T]) -> Set[T]`, requiring both arguments to be the
// exact same Set type.
func resolveSetBinaryOp(v *ast.CallExpr, fc *funcChecker, name string) (string, error) {
	if len(v.Args) != 2 {
		return "", arityError(v, 2)
	}
	aTyp, err := fc.checkExpr(v.Args[0], "")
	if err != nil {
		return "", err
	}
	if !isSetType(aTyp) {
		return "", fmt.Errorf("line %d: %s: unsupported type %s (must be a Set)", v.Line, name, aTyp)
	}
	if _, err := fc.checkExpr(v.Args[1], aTyp); err != nil {
		return "", err
	}
	v.ArgTypes = []string{aTyp, aTyp}
	return aTyp, nil
}

func resolveSetUnion(fc *funcChecker, v *ast.CallExpr) (string, error) {
	return resolveSetBinaryOp(v, fc, "union")
}
func resolveSetIntersect(fc *funcChecker, v *ast.CallExpr) (string, error) {
	return resolveSetBinaryOp(v, fc, "intersect")
}
func resolveSetDifference(fc *funcChecker, v *ast.CallExpr) (string, error) {
	return resolveSetBinaryOp(v, fc, "difference")
}

// resolveSetToList type-checks `toList(s) -> List[T]` (amifl-spec.md
// section 13.5) — element order is unspecified (17.2節#2's established
// "toList(_) |> sort" convention for anyone who needs a deterministic
// order).
func resolveSetToList(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 1 {
		return "", arityError(v, 1)
	}
	sTyp, err := fc.checkExpr(v.Args[0], "")
	if err != nil {
		return "", err
	}
	elem, ok := setElemType(sTyp)
	if !ok {
		return "", fmt.Errorf("line %d: toList: unsupported type %s (must be a Set)", v.Line, sTyp)
	}
	v.ArgTypes = []string{sTyp}
	return makeListType(elem), nil
}

// resolveMapKeys type-checks `keys(m) -> List[K]` (amifl-spec.md section
// 13.6).
func resolveMapKeys(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 1 {
		return "", arityError(v, 1)
	}
	mTyp, err := fc.checkExpr(v.Args[0], "")
	if err != nil {
		return "", err
	}
	key, _, ok := mapKeyValueTypes(mTyp)
	if !ok {
		return "", fmt.Errorf("line %d: keys: unsupported type %s (must be a Map)", v.Line, mTyp)
	}
	v.ArgTypes = []string{mTyp}
	return makeListType(key), nil
}

// resolveMapValues type-checks `values(m) -> List[V]` (amifl-spec.md
// section 13.6).
func resolveMapValues(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 1 {
		return "", arityError(v, 1)
	}
	mTyp, err := fc.checkExpr(v.Args[0], "")
	if err != nil {
		return "", err
	}
	_, val, ok := mapKeyValueTypes(mTyp)
	if !ok {
		return "", fmt.Errorf("line %d: values: unsupported type %s (must be a Map)", v.Line, mTyp)
	}
	v.ArgTypes = []string{mTyp}
	return makeListType(val), nil
}

// resolveMapEntries type-checks `entries(m) -> List[Tuple2[K,V]]`
// (amifl-spec.md section 13.6).
func resolveMapEntries(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 1 {
		return "", arityError(v, 1)
	}
	mTyp, err := fc.checkExpr(v.Args[0], "")
	if err != nil {
		return "", err
	}
	key, val, ok := mapKeyValueTypes(mTyp)
	if !ok {
		return "", fmt.Errorf("line %d: entries: unsupported type %s (must be a Map)", v.Line, mTyp)
	}
	v.ArgTypes = []string{mTyp}
	return makeListType(makeTupleType([]string{key, val})), nil
}

// resolveMapGet type-checks `get(m, k, default) -> V` (amifl-spec.md
// section 13.6) — default's type must match V exactly (no implicit
// conversion, principle 2).
func resolveMapGet(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 3 {
		return "", arityError(v, 3)
	}
	mTyp, err := fc.checkExpr(v.Args[0], "")
	if err != nil {
		return "", err
	}
	key, val, ok := mapKeyValueTypes(mTyp)
	if !ok {
		return "", fmt.Errorf("line %d: get: unsupported type %s (must be a Map)", v.Line, mTyp)
	}
	if _, err := fc.checkExpr(v.Args[1], key); err != nil {
		return "", err
	}
	if _, err := fc.checkExpr(v.Args[2], val); err != nil {
		return "", err
	}
	v.ArgTypes = []string{mTyp, key, val}
	return val, nil
}

// resolveMapSet type-checks `set(m, k, v) -> Unit` (amifl-spec.md section
// 13.6) — mutates m in place (MSET).
func resolveMapSet(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 3 {
		return "", arityError(v, 3)
	}
	mTyp, err := fc.checkExpr(v.Args[0], "")
	if err != nil {
		return "", err
	}
	key, val, ok := mapKeyValueTypes(mTyp)
	if !ok {
		return "", fmt.Errorf("line %d: set: unsupported type %s (must be a Map)", v.Line, mTyp)
	}
	if _, err := fc.checkExpr(v.Args[1], key); err != nil {
		return "", err
	}
	if _, err := fc.checkExpr(v.Args[2], val); err != nil {
		return "", err
	}
	v.ArgTypes = []string{mTyp, key, val}
	return unitType, nil
}

// resolveMapDelete type-checks `delete(m, k) -> Unit` (amifl-spec.md
// section 13.6).
func resolveMapDelete(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 2 {
		return "", arityError(v, 2)
	}
	mTyp, err := fc.checkExpr(v.Args[0], "")
	if err != nil {
		return "", err
	}
	key, _, ok := mapKeyValueTypes(mTyp)
	if !ok {
		return "", fmt.Errorf("line %d: delete: unsupported type %s (must be a Map)", v.Line, mTyp)
	}
	if _, err := fc.checkExpr(v.Args[1], key); err != nil {
		return "", err
	}
	v.ArgTypes = []string{mTyp, key}
	return unitType, nil
}
