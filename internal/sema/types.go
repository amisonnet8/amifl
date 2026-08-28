package sema

import (
	"math"
	"strconv"
	"strings"
)

// typeAliases maps AmiFL's convenience scalar-type aliases to their
// canonical name (amifl-spec.md section 2.1). Two type names denote the
// same type if and only if they canonicalize to the same string — e.g.
// "Int" and "Int64" are the same type, but "Int32" and "Int64" are not
// (principle 2: no implicit conversion between differently-sized or
// differently-signed numeric types).
var typeAliases = map[string]string{
	"Int":   "Int64",
	"UInt":  "UInt64",
	"Byte":  "UInt8",
	"Rune":  "Int32",
	"Float": "Float64",
}

// scalarTypes is the set of canonical scalar type names step 2 supports.
// Collections, struct/enum, Any, Error, and File (amifl-spec.md section
// 2.2) arrive in later steps.
var scalarTypes = map[string]bool{
	"Int8": true, "Int16": true, "Int32": true, "Int64": true,
	"UInt8": true, "UInt16": true, "UInt32": true, "UInt64": true,
	"Float32": true, "Float64": true,
	"Bool": true, "String": true,
	// Error (amifl-spec.md section 2.2, step 11) — see isErrorType.
	"Error": true,
}

// unitType is the implicit type of a `let`/`const`/assignment/discard
// expression, and of a block whose value is discarded (amifl-spec.md
// principle 1). It intentionally isn't in scalarTypes — Unit isn't a
// type a user can write in a type annotation (there is no AmiFL syntax
// that would even parse one there), only an internal bookkeeping type.
const unitType = "Unit"

// canonicalType resolves a type-annotation identifier to its canonical
// type name: a scalar (via typeAliases/scalarTypes) or, since step 6, a
// user-declared `struct` name (c.structs — always its own name verbatim,
// never aliased). This is a *checker* method, not a free function, exactly
// because struct names are per-file state collected while checking a
// particular file — step 2 through step 5 had no such state and could get
// away with a package-level function, but step 6's struct names can't.
func (c *checker) canonicalType(name string) (string, bool) {
	if alias, ok := typeAliases[name]; ok {
		name = alias
	}
	if scalarTypes[name] {
		return name, true
	}
	if _, ok := c.structs[name]; ok {
		return name, true
	}
	if _, ok := c.enums[name]; ok {
		return name, true
	}
	return "", false
}

// isEnumType reports whether t is a declared `enum` type's own name — an
// enum's canonical form is always just its own name verbatim, exactly
// like a struct's (canonicalType never aliases either), so a plain map
// lookup suffices.
func (c *checker) isEnumType(t string) bool {
	_, ok := c.enums[t]
	return ok
}

// canonicalReturnType is canonicalType plus one extra case usable only in
// a function's own return-type position (a top-level `fn`'s or a
// ClosureLit's): amifl-spec.md section 8.3 explicitly writes "戻り値無しは
// fn(T1, ...) -> Unit" — Unit as an explicit, user-writable return
// annotation meaning "no value" — even though Unit is everywhere else
// deliberately not a type a user can write (see unitType's doc comment;
// it's still not accepted for a `let`/const annotation or a parameter
// type, both of which go through plain canonicalType, unchanged).
func (c *checker) canonicalReturnType(name string) (string, bool) {
	if name == "Unit" {
		return unitType, true
	}
	return c.canonicalType(name)
}

// makeFuncType/funcTypeParts/isFuncType encode a closure's signature as
// "fn(P1,P2,...)->R" (Pi/R already-canonical scalar types) — a purely
// internal sema/codegen convention, never surfaced in source syntax. Step
// 5 gives AmiFL no `fn(...) -> R` type-annotation grammar at all (see
// ast.ClosureLit's doc comment), so, unlike scalar type names, this
// string is never parsed from user-written text — makeFuncType is the
// only producer (always fed already-canonical pieces) and funcTypeParts
// only ever decodes strings makeFuncType itself built, which is what
// keeps the decoding trivial (Pi/R can never themselves contain "," ")"
// or "->", since step 5's ClosureLit restricts them to plain scalars).
func makeFuncType(params []string, ret string) string {
	return "fn(" + strings.Join(params, ",") + ")->" + ret
}

func isFuncType(t string) bool {
	return strings.HasPrefix(t, "fn(")
}

func funcTypeParts(t string) (params []string, ret string, ok bool) {
	if !isFuncType(t) {
		return nil, "", false
	}
	sep := strings.Index(t, ")->")
	if sep < 0 {
		return nil, "", false
	}
	paramsRaw := t[len("fn("):sep]
	ret = t[sep+len(")->"):]
	if paramsRaw != "" {
		params = strings.Split(paramsRaw, ",")
	}
	return params, ret, true
}

// makeTupleType/tupleTypeParts/isTupleType encode a TupleLit's shape as
// "Tuple(T1,T2,...)" (Ti already-canonical scalar or struct type names) —
// an internal sema/codegen convention exactly mirroring makeFuncType above,
// with the identical reason a naive comma-split decoding stays safe:
// sema's resolveTupleLit rejects any element whose own type isTupleType or
// isFuncType (ast.TupleLit's doc comment), so no Ti can itself contain a
// "," or ")" that would need bracket-depth-aware parsing to undo.
func makeTupleType(elems []string) string {
	return "Tuple(" + strings.Join(elems, ",") + ")"
}

func isTupleType(t string) bool {
	return strings.HasPrefix(t, "Tuple(")
}

func tupleTypeParts(t string) (elems []string, ok bool) {
	if !isTupleType(t) {
		return nil, false
	}
	inner := strings.TrimSuffix(strings.TrimPrefix(t, "Tuple("), ")")
	if inner == "" {
		return nil, true
	}
	return strings.Split(inner, ","), true
}

// makeListType/listElemType/isListType encode List[T] (amifl-spec.md
// section 2.2) as "List(T)" — T already-canonical, mirroring Tuple/Func's
// internal string-encoding convention. Unlike Tuple/Func, this string
// *is* parsed from real user-written surface syntax (`List[T]`) — but
// only by the parser, into ast.ListType; sema's resolveTypeExpr is what
// turns that into this canonical string, and nothing downstream ever goes
// back the other way (codegen only ever decodes a string makeListType
// itself built, exactly like every other type-encoding here).
func makeListType(elem string) string {
	return "List(" + elem + ")"
}

func isListType(t string) bool {
	return strings.HasPrefix(t, "List(") && strings.HasSuffix(t, ")")
}

func listElemType(t string) (string, bool) {
	if !isListType(t) {
		return "", false
	}
	return strings.TrimSuffix(strings.TrimPrefix(t, "List("), ")"), true
}

// makeArrayType/arrayParts/isArrayType encode one dimension of Array[T;N]
// (amifl-spec.md section 2.2) as "Array(T;N)" — T already-canonical, N a
// decimal literal sema has already reduced Array[T;N]'s size expression
// to (evalConstArraySize). A multi-dimensional Array[T;N1,N2,...] is
// nested ArrayType values by the time sema ever sees one (the parser
// desugars it — ast.ArrayType's doc comment), so this string encoding
// never has to represent more than a single dimension either.
//
// arrayParts finds the *last* ";" to split elem from size, not the
// first: elem may itself be another "Array(...;...)" string (nested
// arrays), which has its own inner ";" earlier in the string — but
// makeArrayType always appends the outer ";size" last, so the rightmost
// ";" in the full string is always the outer one, regardless of nesting
// depth. (Tuple/struct/scalar element types can never contain ";" at
// all, so this is unambiguous for every element kind step 7 allows.)
func makeArrayType(elem, size string) string {
	return "Array(" + elem + ";" + size + ")"
}

func isArrayType(t string) bool {
	return strings.HasPrefix(t, "Array(") && strings.HasSuffix(t, ")")
}

func arrayParts(t string) (elem string, size uint64, ok bool) {
	if !isArrayType(t) {
		return "", 0, false
	}
	inner := strings.TrimSuffix(strings.TrimPrefix(t, "Array("), ")")
	sep := strings.LastIndex(inner, ";")
	if sep < 0 {
		return "", 0, false
	}
	elem = inner[:sep]
	n, err := strconv.ParseUint(inner[sep+1:], 10, 64)
	if err != nil {
		return "", 0, false
	}
	return elem, n, true
}

// elementType returns t's element type if t is a List or an Array —
// shared by every consumer that only cares "does this hold a T, and what
// T" regardless of which of the two container kinds it is (indexing,
// `for`, slicing). Deliberately excludes Set (step 10) — `x[i]`/`x[a:b]`
// are List/Array only (amifl-spec.md section 13.4's at/setAt/slice row),
// while `for` iteration also accepts Set — see forIterableElemType below,
// which is why that's a separate function rather than an extra case added
// here.
func elementType(t string) (string, bool) {
	if e, ok := listElemType(t); ok {
		return e, true
	}
	if e, _, ok := arrayParts(t); ok {
		return e, true
	}
	return "", false
}

// makeSetType/isSetType/setElemType encode Set[T] (amifl-spec.md sections
// 2.2/13.5) as "Set(T)" — T already-canonical, mirroring List's identical
// encoding convention (makeListType).
func makeSetType(elem string) string {
	return "Set(" + elem + ")"
}

func isSetType(t string) bool {
	return strings.HasPrefix(t, "Set(") && strings.HasSuffix(t, ")")
}

func setElemType(t string) (string, bool) {
	if !isSetType(t) {
		return "", false
	}
	return strings.TrimSuffix(strings.TrimPrefix(t, "Set("), ")"), true
}

// makeMapType/isMapType/mapKeyValueTypes encode Map[K,V] (amifl-spec.md
// section 2.2) as "Map(K,V)" — K/V already-canonical. Unlike every other
// type-encoding here (Tuple/Func/List/Array), V is *not* restricted to a
// flat, comma-free shape: a Map's value may itself be another List/Array/
// Set/Map/Tuple (there is no reason to forbid `Map[String, List[Int]]`
// the way step 6 forbids a nested Tuple), and even K may be a Tuple (one
// of the four comparable kinds Set/Map keys allow — isComparableKeyType).
// Either can therefore itself contain "(" ")" "," or ";" internally, so a
// naive first/last-comma split (the trick makeArrayType's arrayParts uses,
// relying on ";" never appearing in a nested element) doesn't work here —
// mapKeyValueTypes instead walks the string tracking paren depth and
// splits at the first depth-0 comma, which is always exactly the one
// makeMapType itself inserted between K and V (every canonical type
// string has balanced parens by construction).
func makeMapType(key, val string) string {
	return "Map(" + key + "," + val + ")"
}

func isMapType(t string) bool {
	return strings.HasPrefix(t, "Map(") && strings.HasSuffix(t, ")")
}

func mapKeyValueTypes(t string) (key, val string, ok bool) {
	if !isMapType(t) {
		return "", "", false
	}
	inner := strings.TrimSuffix(strings.TrimPrefix(t, "Map("), ")")
	depth := 0
	for i, r := range inner {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				return inner[:i], inner[i+1:], true
			}
		}
	}
	return "", "", false
}

// forIterableElemType is elementType (List/Array) plus Set (step 10) —
// `for x in items { ... }`'s single-variable form accepts all three
// (amifl-spec.md section 7 doesn't restrict `for` to List/Array once a
// Set exists to iterate; Map instead needs the two-variable `for k, v in
// m` form — resolveForExpr handles that separately via mapKeyValueTypes,
// never through this function). Kept apart from elementType itself since
// that one backs indexing/slicing too, which Set never supports (see
// elementType's own doc comment).
func forIterableElemType(t string) (string, bool) {
	if e, ok := elementType(t); ok {
		return e, true
	}
	if e, ok := setElemType(t); ok {
		return e, true
	}
	return "", false
}

// isComparableKeyType reports whether t may be used as a Set[T] element or
// a Map[K,_] key (amifl-spec.md section 2.2, "Tは比較可能な型（数値・文字列・
// 真偽値・タプル）のみ" — stated for Set but applied here to Map's key too,
// since both ultimately compile to a Go map (CLAUDE.md's "確定した設計判断"
// for step 10) whose key type Go itself requires to be comparable). A
// struct type is deliberately excluded even though step 6 made structs
// Go-comparable too (its own `==` is allowed) — amifl-spec.md's own wording
// enumerates exactly four kinds and doesn't mention struct; a documented,
// conservative reading rather than an oversight, revisit if a concrete need
// for struct-keyed Set/Map appears.
func isComparableKeyType(t string) bool {
	return isIntType(t) || isFloatType(t) || t == "Bool" || t == "String" || isTupleType(t)
}

// isErrorType reports whether t is the built-in Error type (amifl-spec.md
// section 2.2) — a Go `error` value under the hood (codegen's goTypeNames).
func isErrorType(t string) bool {
	return t == "Error"
}

// tuple2ErrorPayload reports whether t is exactly Tuple2[U,Error] for some
// U, returning U — amifl-spec.md's "戻り値は常に単数、複数値はTuple2[T,
// Error]のような統一形で包む" convention (principle 6), used by the `?`
// operator (resolveTryExpr) and by every built-in function returning one
// (sema/builtins.go).
func tuple2ErrorPayload(t string) (payload string, ok bool) {
	elems, ok := tupleTypeParts(t)
	if !ok || len(elems) != 2 || elems[1] != "Error" {
		return "", false
	}
	return elems[0], true
}

func isIntType(name string) bool {
	switch name {
	case "Int8", "Int16", "Int32", "Int64", "UInt8", "UInt16", "UInt32", "UInt64":
		return true
	}
	return false
}

func isFloatType(name string) bool {
	return name == "Float32" || name == "Float64"
}

func isUIntType(name string) bool {
	switch name {
	case "UInt8", "UInt16", "UInt32", "UInt64":
		return true
	}
	return false
}

func isSignedIntType(name string) bool {
	switch name {
	case "Int8", "Int16", "Int32", "Int64":
		return true
	}
	return false
}

// isOrderedType reports whether name has the Ordered capability
// (amifl-spec.md section 2.3: `< <= > >=`) — every numeric type plus
// String (Go's native string comparison is already lexicographic, so no
// special codegen is needed beyond the plain LT/LTE/GT/GTE instructions).
func isOrderedType(name string) bool {
	return isIntType(name) || isFloatType(name) || name == "String"
}

// intLitMax is the largest value a bare (non-negated) IntLit can hold for
// each integer type. A literal directly under unary `-` gets one extra bit
// of headroom for signed types (resolveNegatedIntLit) since e.g. -128 is a
// valid Int8 even though the literal "128" alone isn't.
var intLitMax = map[string]uint64{
	"Int8":   math.MaxInt8,
	"Int16":  math.MaxInt16,
	"Int32":  math.MaxInt32,
	"Int64":  math.MaxInt64,
	"UInt8":  math.MaxUint8,
	"UInt16": math.MaxUint16,
	"UInt32": math.MaxUint32,
	"UInt64": math.MaxUint64,
}
