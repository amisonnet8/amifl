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
	return "", false
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
// `for`, slicing).
func elementType(t string) (string, bool) {
	if e, ok := listElemType(t); ok {
		return e, true
	}
	if e, _, ok := arrayParts(t); ok {
		return e, true
	}
	return "", false
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
