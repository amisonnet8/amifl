// builtins_data.go type-checks amifl-spec.md section 13.4's data-
// manipulation built-ins (step 11 phase 11b) — capability-based
// polymorphism (2.3節) resolved once per call site, exactly like
// builtins.go's isError/cast/parse: each resolver here inspects its first
// argument's already-resolved concrete type and picks the one capability
// group it belongs to, entirely at compile time (design issue 6's answer
// — no runtime dispatch, no AmiFL-visible generics).
package sema

import (
	"fmt"

	"github.com/amisonnet8/amifl/internal/ast"
)

func init() {
	for name, resolver := range map[string]builtinResolver{
		"len":        resolveLen,
		"slice":      resolveSlice,
		"at":         resolveAt,
		"setAt":      resolveSetAt,
		"contains":   resolveContains,
		"index":      resolveIndex,
		"split":      resolveSplit,
		"join":       resolveJoin,
		"replace":    resolveReplace,
		"trim":       resolveTrim,
		"upper":      resolveUpper,
		"lower":      resolveLower,
		"startsWith": resolveStartsWith,
		"endsWith":   resolveEndsWith,
		"map":        resolveMap,
		"filter":     resolveFilter,
		"reduce":     resolveReduce,
		"sort":       resolveSort,
		"sortBy":     resolveSortBy,
		"reverse":    resolveReverse,
		"unique":     resolveUnique,
		"flatten":    resolveFlatten,
		"zip":        resolveZip,
		"push":       resolvePush,
		"pop":        resolvePop,
		"insert":     resolveInsert,
		"removeAt":   resolveRemoveAt,
		"concat":     resolveConcat,
	} {
		builtinFuncs[name] = resolver
	}
}

// isEqualityComparableType reports whether t may be used as a List/Array
// element for contains/index/unique — broader than isComparableKeyType
// (Set/Map's own key restriction, amifl-spec.md section 2.2's explicit
// "数値・文字列・真偽値・タプル") since a struct is also Go-comparable
// (step 6: every struct field is itself scalar/struct/tuple, never a
// List/Map/Set/Func, so Go's own `==` is always valid) and 13.4 never
// states an equivalent restriction the way 2.2 does for Set/Map. An enum
// is deliberately excluded even though its own Go representation (a
// single tagged STTYPE, step 8) is technically comparable too — step 8
// already decided AmiFL-level `==`/`!=` don't apply to enum values at all
// ("switchのパターンマッチでのみ...アクセス"), and silently allowing it
// through contains()/index()/unique() while forbidding it via `==` would
// be an inconsistent back door.
func (fc *funcChecker) isEqualityComparableType(t string) bool {
	if isComparableKeyType(t) {
		return true
	}
	_, isStruct := fc.structs[t]
	return isStruct
}

func arityError(v *ast.CallExpr, want int) error {
	return fmt.Errorf("line %d: %s expects %d argument(s), got %d", v.Line, v.Callee, want, len(v.Args))
}

// resolveLen type-checks `len(x) -> Int` (amifl-spec.md section 13.4) —
// the Lenable capability: String, List, Array, Map, Set, Chan (step 12).
// Bytes needs no separate case: it canonicalizes straight to List(UInt8)
// (types.go's canonicalType), already covered by isListType.
func resolveLen(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 1 {
		return "", arityError(v, 1)
	}
	t, err := fc.checkExpr(v.Args[0], "")
	if err != nil {
		return "", err
	}
	_, isChan := chanElemType(t)
	if !(t == "String" || isListType(t) || isArrayType(t) || isMapType(t) || isSetType(t) || isChan) {
		return "", fmt.Errorf("line %d: len: unsupported type %s (must be String, List, Array, Map, Set, or Chan)", v.Line, t)
	}
	v.ArgTypes = []string{t}
	return "Int64", nil
}

// resolveSlice type-checks `slice(x, from, to) -> String|List[T]`
// (amifl-spec.md section 13.4) — the Sliceable capability restricted here
// to String/List/Array (Bytes/Stream aren't implemented yet, step 12/13).
// Unlike the `x[a:b]` sugar (ast.SliceExpr), this named-function form
// takes all 3 arguments unconditionally — AmiFL has no way to spell the
// sugar's `_`-omitted bound as a plain value expression a user could pass
// here (principle 7, "可変長引数無し・名前付き引数無し"; the sugar's own
// `_` is a parser-level token, never a real AmiFL value — ast.SliceExpr's
// doc comment), so a user wanting an open-ended bound uses `x[a:]`/`x[:b]`
// instead of this function. Stream[T] (step 12) is included, unlike the
// x[a:b] sugar (ast.SliceExpr stays List/Array/String-only, never Stream —
// AMIVM's own SLICE instruction only ever generates Go's native `x[from:
// to]` syntax, which a channel can't use at all): codegen's
// genSliceBuiltinValue instead composes it from skip+take (chan.go), which
// this named-function form's "all 3 bounds always given" contract makes
// straightforward and the sugar's `_`-omittable-bound one doesn't.
func resolveSlice(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 3 {
		return "", arityError(v, 3)
	}
	xTyp, err := fc.checkExpr(v.Args[0], "")
	if err != nil {
		return "", err
	}
	if _, err := fc.checkExpr(v.Args[1], "Int64"); err != nil {
		return "", err
	}
	if _, err := fc.checkExpr(v.Args[2], "Int64"); err != nil {
		return "", err
	}
	v.ArgTypes = []string{xTyp, "Int64", "Int64"}
	if xTyp == "String" {
		return "String", nil
	}
	if elem, ok := elementType(xTyp); ok {
		return makeListType(elem), nil
	}
	if elem, ok := streamElemType(xTyp); ok {
		return makeStreamType(elem), nil
	}
	return "", fmt.Errorf("line %d: slice: unsupported type %s (must be String, List, Array, or Stream)", v.Line, xTyp)
}

// resolveAt type-checks `at(x, i) -> T` (amifl-spec.md section 13.4) —
// List/Array only, exactly like the `x[i]` sugar it's an alternate way to
// spell (ast.IndexExpr).
func resolveAt(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 2 {
		return "", arityError(v, 2)
	}
	xTyp, err := fc.checkExpr(v.Args[0], "")
	if err != nil {
		return "", err
	}
	elem, ok := elementType(xTyp)
	if !ok {
		return "", fmt.Errorf("line %d: at: unsupported type %s (must be a List or Array)", v.Line, xTyp)
	}
	if _, err := fc.checkExpr(v.Args[1], "Int64"); err != nil {
		return "", err
	}
	v.ArgTypes = []string{xTyp, "Int64"}
	return elem, nil
}

// resolveSetAt type-checks `setAt(x, i, v) -> Unit` (amifl-spec.md section
// 13.4) — List only ("setAtはListのみ。固定長はx[i]=vのみ": a fixed-size
// Array can only be mutated through the `x[i]=v` sugar, never this named
// function).
func resolveSetAt(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 3 {
		return "", arityError(v, 3)
	}
	xTyp, err := fc.checkExpr(v.Args[0], "")
	if err != nil {
		return "", err
	}
	elem, ok := listElemType(xTyp)
	if !ok {
		return "", fmt.Errorf("line %d: setAt: unsupported type %s (must be a List)", v.Line, xTyp)
	}
	if _, err := fc.checkExpr(v.Args[1], "Int64"); err != nil {
		return "", err
	}
	if _, err := fc.checkExpr(v.Args[2], elem); err != nil {
		return "", err
	}
	v.ArgTypes = []string{xTyp, "Int64", elem}
	return unitType, nil
}

// resolveContains type-checks `contains(x, target) -> Bool` (amifl-spec.md
// section 13.4) — the Containable capability: String (substring), List/
// Array (element membership), Map (key membership), Set (element
// membership).
func resolveContains(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 2 {
		return "", arityError(v, 2)
	}
	xTyp, err := fc.checkExpr(v.Args[0], "")
	if err != nil {
		return "", err
	}
	switch {
	case xTyp == "String":
		if _, err := fc.checkExpr(v.Args[1], "String"); err != nil {
			return "", err
		}
		v.ArgTypes = []string{xTyp, "String"}
	case isListType(xTyp) || isArrayType(xTyp):
		elem, _ := elementType(xTyp)
		if !fc.isEqualityComparableType(elem) {
			return "", fmt.Errorf("line %d: contains: element type %s isn't comparable", v.Line, elem)
		}
		if _, err := fc.checkExpr(v.Args[1], elem); err != nil {
			return "", err
		}
		v.ArgTypes = []string{xTyp, elem}
	case isMapType(xTyp):
		key, _, _ := mapKeyValueTypes(xTyp)
		if _, err := fc.checkExpr(v.Args[1], key); err != nil {
			return "", err
		}
		v.ArgTypes = []string{xTyp, key}
	case isSetType(xTyp):
		elem, _ := setElemType(xTyp)
		if _, err := fc.checkExpr(v.Args[1], elem); err != nil {
			return "", err
		}
		v.ArgTypes = []string{xTyp, elem}
	default:
		return "", fmt.Errorf("line %d: contains: unsupported type %s (must be String, List, Array, Map, or Set)", v.Line, xTyp)
	}
	return "Bool", nil
}

// resolveIndex type-checks `index(x, target) -> Tuple2[Int, Bool]`
// (amifl-spec.md section 13.4) — String, List, Array only (unlike
// contains, Map/Set aren't in this row of the table: a Set/Map's own
// iteration order is unspecified, so a "position" wouldn't be meaningful).
func resolveIndex(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 2 {
		return "", arityError(v, 2)
	}
	xTyp, err := fc.checkExpr(v.Args[0], "")
	if err != nil {
		return "", err
	}
	var elem string
	if xTyp == "String" {
		elem = "String"
		if _, err := fc.checkExpr(v.Args[1], "String"); err != nil {
			return "", err
		}
	} else if e, ok := elementType(xTyp); ok {
		elem = e
		if !fc.isEqualityComparableType(elem) {
			return "", fmt.Errorf("line %d: index: element type %s isn't comparable", v.Line, elem)
		}
		if _, err := fc.checkExpr(v.Args[1], elem); err != nil {
			return "", err
		}
	} else {
		return "", fmt.Errorf("line %d: index: unsupported type %s (must be String, List, or Array)", v.Line, xTyp)
	}
	v.ArgTypes = []string{xTyp, elem}
	return makeTupleType([]string{"Int64", "Bool"}), nil
}

// resolveStringUnary is shared by trim/upper/lower — all `(s: String) ->
// String`.
func resolveStringUnary(v *ast.CallExpr, fc *funcChecker) (string, error) {
	if len(v.Args) != 1 {
		return "", arityError(v, 1)
	}
	if _, err := fc.checkExpr(v.Args[0], "String"); err != nil {
		return "", err
	}
	v.ArgTypes = []string{"String"}
	return "String", nil
}

func resolveTrim(fc *funcChecker, v *ast.CallExpr) (string, error)  { return resolveStringUnary(v, fc) }
func resolveUpper(fc *funcChecker, v *ast.CallExpr) (string, error) { return resolveStringUnary(v, fc) }
func resolveLower(fc *funcChecker, v *ast.CallExpr) (string, error) { return resolveStringUnary(v, fc) }

// resolveSplit type-checks `split(s, sep) -> List[String]` (amifl-spec.md
// section 13.4).
func resolveSplit(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 2 {
		return "", arityError(v, 2)
	}
	if _, err := fc.checkExpr(v.Args[0], "String"); err != nil {
		return "", err
	}
	if _, err := fc.checkExpr(v.Args[1], "String"); err != nil {
		return "", err
	}
	v.ArgTypes = []string{"String", "String"}
	return makeListType("String"), nil
}

// resolveJoin type-checks `join(xs, sep) -> String` (amifl-spec.md section
// 13.4) — xs must be exactly List[String] (not Array[String;N]; joining a
// fixed-size collection of strings has no concrete need yet, and 13.4's
// table lists join alongside split/trim/... under the plain "String" row,
// not the Sliceable/Lenable capability rows — kept narrow rather than
// guessed wide).
func resolveJoin(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 2 {
		return "", arityError(v, 2)
	}
	if _, err := fc.checkExpr(v.Args[0], makeListType("String")); err != nil {
		return "", err
	}
	if _, err := fc.checkExpr(v.Args[1], "String"); err != nil {
		return "", err
	}
	v.ArgTypes = []string{makeListType("String"), "String"}
	return "String", nil
}

// resolveReplace type-checks `replace(s, old, new) -> String` (amifl-
// spec.md section 13.4).
func resolveReplace(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 3 {
		return "", arityError(v, 3)
	}
	for _, a := range v.Args {
		if _, err := fc.checkExpr(a, "String"); err != nil {
			return "", err
		}
	}
	v.ArgTypes = []string{"String", "String", "String"}
	return "String", nil
}

// resolveStringPredicate is shared by startsWith/endsWith — both
// `(s: String, x: String) -> Bool`.
func resolveStringPredicate(v *ast.CallExpr, fc *funcChecker) (string, error) {
	if len(v.Args) != 2 {
		return "", arityError(v, 2)
	}
	if _, err := fc.checkExpr(v.Args[0], "String"); err != nil {
		return "", err
	}
	if _, err := fc.checkExpr(v.Args[1], "String"); err != nil {
		return "", err
	}
	v.ArgTypes = []string{"String", "String"}
	return "Bool", nil
}

func resolveStartsWith(fc *funcChecker, v *ast.CallExpr) (string, error) {
	return resolveStringPredicate(v, fc)
}
func resolveEndsWith(fc *funcChecker, v *ast.CallExpr) (string, error) {
	return resolveStringPredicate(v, fc)
}

// requireListOrArrayElem checks arg0's type is List/Array and returns its
// element type — shared by map/filter/reduce/sort/sortBy/reverse/unique/
// flatten/push/pop/insert/removeAt/concat's List-only or List/Array-only
// domain checks below.
func requireListOrArrayElem(fc *funcChecker, v *ast.CallExpr, name string) (xTyp, elem string, err error) {
	xTyp, err = fc.checkExpr(v.Args[0], "")
	if err != nil {
		return "", "", err
	}
	e, ok := elementType(xTyp)
	if !ok {
		return "", "", fmt.Errorf("line %d: %s: unsupported type %s (must be a List or Array)", v.Line, name, xTyp)
	}
	return xTyp, e, nil
}

func requireListElem(fc *funcChecker, v *ast.CallExpr, name string) (xTyp, elem string, err error) {
	xTyp, err = fc.checkExpr(v.Args[0], "")
	if err != nil {
		return "", "", err
	}
	e, ok := listElemType(xTyp)
	if !ok {
		return "", "", fmt.Errorf("line %d: %s: unsupported type %s (must be a List)", v.Line, name, xTyp)
	}
	return xTyp, e, nil
}

// resolveMap type-checks `map(xs, f) -> List[U]` (amifl-spec.md section
// 13.4) — List/Array only (Stream/Chan arrive in step 12). f's own type is
// resolved with no expected type (an already-`let`-bound closure value —
// step 5 only allows a ClosureLit literal directly as a `let`'s value, so
// f is always a reference to one, never an inline literal here) and its
// param/return types are read back out via funcTypeParts rather than
// independently guessed, so U is whatever f itself already says it is.
func resolveMap(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 2 {
		return "", arityError(v, 2)
	}
	xTyp, elem, err := requireListOrArrayElem(fc, v, "map")
	if err != nil {
		return "", err
	}
	fTyp, err := fc.checkExpr(v.Args[1], "")
	if err != nil {
		return "", err
	}
	params, ret, ok := funcTypeParts(fTyp)
	if !ok || len(params) != 1 || params[0] != elem {
		return "", fmt.Errorf("line %d: map: f must be fn(%s) -> U, got %s", v.Line, elem, fTyp)
	}
	v.ArgTypes = []string{xTyp, fTyp}
	return makeListType(ret), nil
}

// resolveFilter type-checks `filter(xs, f) -> List[T]` (amifl-spec.md
// section 13.4).
func resolveFilter(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 2 {
		return "", arityError(v, 2)
	}
	xTyp, elem, err := requireListOrArrayElem(fc, v, "filter")
	if err != nil {
		return "", err
	}
	fTyp, err := fc.checkExpr(v.Args[1], "")
	if err != nil {
		return "", err
	}
	params, ret, ok := funcTypeParts(fTyp)
	if !ok || len(params) != 1 || params[0] != elem || ret != "Bool" {
		return "", fmt.Errorf("line %d: filter: f must be fn(%s) -> Bool, got %s", v.Line, elem, fTyp)
	}
	v.ArgTypes = []string{xTyp, fTyp}
	return makeListType(elem), nil
}

// resolveReduce type-checks `reduce(xs, init, f) -> U` (amifl-spec.md
// section 13.4) — f is fn(U, T) -> U (accumulator first, fold-left
// convention; the spec doesn't pin down the parameter order, so this is a
// documented step-11 design choice — CLAUDE.md's "確定した設計判断").
func resolveReduce(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 3 {
		return "", arityError(v, 3)
	}
	xTyp, elem, err := requireListOrArrayElem(fc, v, "reduce")
	if err != nil {
		return "", err
	}
	initTyp, err := fc.checkExpr(v.Args[1], "")
	if err != nil {
		return "", err
	}
	fTyp, err := fc.checkExpr(v.Args[2], "")
	if err != nil {
		return "", err
	}
	params, ret, ok := funcTypeParts(fTyp)
	if !ok || len(params) != 2 || params[0] != initTyp || params[1] != elem || ret != initTyp {
		return "", fmt.Errorf("line %d: reduce: f must be fn(%s, %s) -> %s, got %s", v.Line, initTyp, elem, initTyp, fTyp)
	}
	v.ArgTypes = []string{xTyp, initTyp, fTyp}
	return initTyp, nil
}

// resolveSort type-checks `sort(xs) -> List[T]` (amifl-spec.md section
// 13.4) — List only, T restricted to the Ordered capability.
func resolveSort(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 1 {
		return "", arityError(v, 1)
	}
	xTyp, elem, err := requireListElem(fc, v, "sort")
	if err != nil {
		return "", err
	}
	if !isOrderedType(elem) {
		return "", fmt.Errorf("line %d: sort: element type %s isn't Ordered", v.Line, elem)
	}
	v.ArgTypes = []string{xTyp}
	return xTyp, nil
}

// resolveSortBy type-checks `sortBy(xs, opt) -> List[T]` (amifl-spec.md
// section 13.4) — opt is a key-extraction closure `fn(T) -> K`, K itself
// Ordered (a documented step-11 design choice for what `opt` means —
// CLAUDE.md's "確定した設計判断").
func resolveSortBy(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 2 {
		return "", arityError(v, 2)
	}
	xTyp, elem, err := requireListElem(fc, v, "sortBy")
	if err != nil {
		return "", err
	}
	optTyp, err := fc.checkExpr(v.Args[1], "")
	if err != nil {
		return "", err
	}
	params, ret, ok := funcTypeParts(optTyp)
	if !ok || len(params) != 1 || params[0] != elem || !isOrderedType(ret) {
		return "", fmt.Errorf("line %d: sortBy: opt must be fn(%s) -> K with K Ordered, got %s", v.Line, elem, optTyp)
	}
	v.ArgTypes = []string{xTyp, optTyp}
	return xTyp, nil
}

// resolveReverse type-checks `reverse(xs) -> same shape` (amifl-spec.md
// section 13.4) — List[T]->List[T], Array[T;N]->Array[T;N] (size
// preserved — reversing never changes length, unlike slice/filter),
// String->String (rune-aware, ReverseString).
func resolveReverse(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 1 {
		return "", arityError(v, 1)
	}
	xTyp, err := fc.checkExpr(v.Args[0], "")
	if err != nil {
		return "", err
	}
	if xTyp != "String" && !isListType(xTyp) && !isArrayType(xTyp) {
		return "", fmt.Errorf("line %d: reverse: unsupported type %s (must be String, List, or Array)", v.Line, xTyp)
	}
	v.ArgTypes = []string{xTyp}
	return xTyp, nil
}

// resolveUnique type-checks `unique(xs) -> List[T]` (amifl-spec.md section
// 13.4) — List only, T restricted to isEqualityComparableType (the same
// restriction contains/index apply to a List/Array element).
func resolveUnique(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 1 {
		return "", arityError(v, 1)
	}
	xTyp, elem, err := requireListElem(fc, v, "unique")
	if err != nil {
		return "", err
	}
	if !fc.isEqualityComparableType(elem) {
		return "", fmt.Errorf("line %d: unique: element type %s isn't comparable", v.Line, elem)
	}
	v.ArgTypes = []string{xTyp}
	return xTyp, nil
}

// resolveFlatten type-checks `flatten(xs) -> List[T]` (amifl-spec.md
// section 13.4) — xs must be List[List[T]].
func resolveFlatten(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 1 {
		return "", arityError(v, 1)
	}
	xTyp, outer, err := requireListElem(fc, v, "flatten")
	if err != nil {
		return "", err
	}
	inner, ok := listElemType(outer)
	if !ok {
		return "", fmt.Errorf("line %d: flatten: expected List[List[T]], got %s", v.Line, xTyp)
	}
	v.ArgTypes = []string{xTyp}
	return makeListType(inner), nil
}

// resolveZip type-checks `zip(xs, ys) -> List[Tuple2[A,B]]` (amifl-spec.md
// section 13.4) — List only (per 13.4's table).
func resolveZip(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 2 {
		return "", arityError(v, 2)
	}
	xTyp, err := fc.checkExpr(v.Args[0], "")
	if err != nil {
		return "", err
	}
	a, ok := listElemType(xTyp)
	if !ok {
		return "", fmt.Errorf("line %d: zip: unsupported type %s for the first argument (must be a List)", v.Line, xTyp)
	}
	yTyp, err := fc.checkExpr(v.Args[1], "")
	if err != nil {
		return "", err
	}
	b, ok := listElemType(yTyp)
	if !ok {
		return "", fmt.Errorf("line %d: zip: unsupported type %s for the second argument (must be a List)", v.Line, yTyp)
	}
	v.ArgTypes = []string{xTyp, yTyp}
	return makeListType(makeTupleType([]string{a, b})), nil
}

// resolvePush type-checks `push(xs, v) -> List[T]` (amifl-spec.md section
// 13.4) — List only, non-destructive (returns a new list — CLAUDE.md's
// "確定した設計判断" for step 11's ambiguous-signature resolutions).
func resolvePush(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 2 {
		return "", arityError(v, 2)
	}
	xTyp, elem, err := requireListElem(fc, v, "push")
	if err != nil {
		return "", err
	}
	if _, err := fc.checkExpr(v.Args[1], elem); err != nil {
		return "", err
	}
	v.ArgTypes = []string{xTyp, elem}
	return xTyp, nil
}

// resolvePop type-checks `pop(xs) -> Tuple2[List[T], T]` (amifl-spec.md
// section 13.4) — List only.
func resolvePop(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 1 {
		return "", arityError(v, 1)
	}
	xTyp, elem, err := requireListElem(fc, v, "pop")
	if err != nil {
		return "", err
	}
	v.ArgTypes = []string{xTyp}
	return makeTupleType([]string{xTyp, elem}), nil
}

// resolveInsert type-checks `insert(xs, i, v) -> List[T]` (amifl-spec.md
// section 13.4) — List only, non-destructive.
func resolveInsert(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 3 {
		return "", arityError(v, 3)
	}
	xTyp, elem, err := requireListElem(fc, v, "insert")
	if err != nil {
		return "", err
	}
	if _, err := fc.checkExpr(v.Args[1], "Int64"); err != nil {
		return "", err
	}
	if _, err := fc.checkExpr(v.Args[2], elem); err != nil {
		return "", err
	}
	v.ArgTypes = []string{xTyp, "Int64", elem}
	return xTyp, nil
}

// resolveRemoveAt type-checks `removeAt(xs, i) -> List[T]` (amifl-spec.md
// section 13.4) — List only, non-destructive.
func resolveRemoveAt(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 2 {
		return "", arityError(v, 2)
	}
	xTyp, _, err := requireListElem(fc, v, "removeAt")
	if err != nil {
		return "", err
	}
	if _, err := fc.checkExpr(v.Args[1], "Int64"); err != nil {
		return "", err
	}
	v.ArgTypes = []string{xTyp, "Int64"}
	return xTyp, nil
}

// resolveConcat type-checks `concat(a, b) -> same type` (amifl-spec.md
// section 13.4) — String (AMIVM's own CONCAT instruction, Concatenable
// since step 3) or List (amiflrt.ConcatSlice); Bytes isn't implemented yet
// (step 12).
func resolveConcat(fc *funcChecker, v *ast.CallExpr) (string, error) {
	if len(v.Args) != 2 {
		return "", arityError(v, 2)
	}
	aTyp, err := fc.checkExpr(v.Args[0], "")
	if err != nil {
		return "", err
	}
	if _, err := fc.checkExpr(v.Args[1], aTyp); err != nil {
		return "", err
	}
	if aTyp != "String" && !isListType(aTyp) {
		return "", fmt.Errorf("line %d: concat: unsupported type %s (must be String or List)", v.Line, aTyp)
	}
	v.ArgTypes = []string{aTyp, aTyp}
	return aTyp, nil
}
