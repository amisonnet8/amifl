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
	// File (amifl-spec.md section 2.2, step 12) — an opaque handle, never
	// constructed except via 13.10's built-ins (open/stdin/stdout/stderr).
	// codegen's goTypeNames maps it straight to *amiflrt.FileHandle.
	"File": true,
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
	// Bytes (amifl-spec.md section 2.1: "可変長バイト列、List[Byte]と同じ
	// 内部表現") isn't a scalar at all — it canonicalizes straight to
	// List[UInt8] (makeListType below), so every capability List[T] already
	// has (Lenable/Sliceable/Concatenable, len/slice/concat/+, and step 7's
	// x[i]/x[i]=v/x[a:b] sugar) "just works" for Bytes too, with zero extra
	// codegen — unlike typeAliases' scalar-to-scalar aliasing (Int -> Int64
	// etc.), this alias target is itself a composite canonical string, so it
	// can't live in that map and is special-cased here instead.
	if name == "Bytes" {
		return makeListType("UInt8"), true
	}
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
	if c.externTypes[name] {
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

// makeFuncType/funcTypeParts/isFuncType encode a Func value's signature as
// "fn(P1,P2,...)->R" (Pi/R already-canonical types) — an internal sema/
// codegen convention, unchanged since step 5, when it was still never
// surfaced in source syntax at all (a ClosureLit's own self-inferred type
// was its only producer). Ex3 adds ast.FuncType, a real `fn(...) -> R`
// type-annotation grammar — resolveTypeExpr's *ast.FuncType case is now a
// second producer, always converging on this identical encoding, so every
// existing consumer (funcTypeParts, codegen's own copy, map/filter/reduce/
// sortBy's resolvers) needed no changes at all: a canonical Func string
// means the same thing and decodes the same way regardless of which
// syntax wrote it.
//
// Pi/R were assumed scalar-only when step 5 wrote this (a ClosureLit's
// own params/return are indeed always scalar), but step 11's map/filter/
// reduce/sortBy pass a *List/Array element's* type as a closure param —
// which can itself be a Tuple/List/Array/Set/Map/struct, i.e. contain a
// "," of its own. funcTypeParts splitting paramsRaw with a plain
// strings.Split (found via a compound-typed reduce accumulator in
// examples/run_length_encode.aml, step 15's examples expansion) silently
// over-split a single "Tuple(Int64,Int64)" param into three pieces —
// exactly the class of bug mapKeyValueTypes's depth-aware split already
// exists to avoid for Map[K,V]'s own key/value. splitTopLevelCommas below
// applies that same technique generally (Map only ever needs 2 pieces at
// the first depth-0 comma; params needs all of them).
func makeFuncType(params []string, ret string) string {
	return "fn(" + strings.Join(params, ",") + ")->" + ret
}

func isFuncType(t string) bool {
	return strings.HasPrefix(t, "fn(")
}

// funcTypeParts's own ")->" params/return separator needs the identical
// depth-aware treatment splitTopLevelCommas already gives commas — found
// the hard way (examples/higher_order_functions.aml's `compose`, ex3's
// examples expansion, actually run through the full amivm -> go build ->
// execute pipeline, not caught by inspection): once a param can itself be
// a Func type (`fn(g: fn(Int)->Int, x: Int) -> Int`, ex3's whole point),
// the canonical string becomes "fn(fn(Int64)->Int64,Int64)->Int64" — a
// naive strings.Index(t, ")->") finds the *inner* Func type's own ")->"
// first (right after its single param's closing paren), silently
// truncating paramsRaw to just that inner Func type and handing back a
// bogus ret starting mid-string. The fix walks the string tracking paren
// depth (exactly splitTopLevelCommas's own technique) and only accepts a
// ")->" match once depth has returned all the way to 0 — the point that's
// always exactly the outermost "fn(...)"'s own closing paren, however deep
// a nested Func-typed param's own parens go.
func funcTypeParts(t string) (params []string, ret string, ok bool) {
	if !isFuncType(t) {
		return nil, "", false
	}
	sep := -1
	depth := 0
	for i := 0; i < len(t); i++ {
		switch t[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 && i+2 < len(t) && t[i+1] == '-' && t[i+2] == '>' {
				sep = i
			}
		}
		if sep >= 0 {
			break
		}
	}
	if sep < 0 {
		return nil, "", false
	}
	paramsRaw := t[len("fn("):sep]
	ret = t[sep+len(")->"):]
	if paramsRaw != "" {
		params = splitTopLevelCommas(paramsRaw)
	}
	return params, ret, true
}

// splitTopLevelCommas splits s at every comma that sits at paren-nesting
// depth 0 — s itself carries no surrounding parens (mapKeyValueTypes's
// "inner", funcTypeParts' "paramsRaw", tupleTypeParts' "inner" are all
// already-stripped interiors), so a "(" bumps depth and a matching ")"
// drops it back, and only a depth-0 comma is a real separator between
// sibling type strings rather than one buried inside a nested Tuple/List/
// Map/.../'s own parens.
func splitTopLevelCommas(s string) []string {
	var parts []string
	depth := 0
	start := 0
	for i, r := range s {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, s[start:])
	return parts
}

// makeTupleType/tupleTypeParts/isTupleType encode a TupleLit's shape as
// "Tuple(T1,T2,...)" — an internal sema/codegen convention exactly
// mirroring makeFuncType above. resolveTupleLit only rejects a Tuple/Func-
// typed element (ast.TupleLit's doc comment); it does *not* restrict
// elements to scalars, so a Ti can legitimately be a List/Map/Set/Array/
// struct and contain its own "," — tupleTypeParts needs the same
// splitTopLevelCommas depth-aware split funcTypeParts does, for the
// identical reason (see that function's doc comment).
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
	return splitTopLevelCommas(inner), true
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

// makeChanType/isChanType/chanElemType encode Chan[T] (amifl-spec.md
// sections 2.2/11/13.8) as "Chan(T)" — T already-canonical, mirroring
// List's identical single-element encoding convention (makeListType). No
// comparability restriction, unlike Set[T]/Map[K,_] (isComparableKeyType) —
// a channel's element type carries no such requirement in Go either.
func makeChanType(elem string) string {
	return "Chan(" + elem + ")"
}

func isChanType(t string) bool {
	return strings.HasPrefix(t, "Chan(") && strings.HasSuffix(t, ")")
}

func chanElemType(t string) (string, bool) {
	if !isChanType(t) {
		return "", false
	}
	return strings.TrimSuffix(strings.TrimPrefix(t, "Chan("), ")"), true
}

// makeStreamType/isStreamType/streamElemType encode Stream[T] (amifl-spec.md
// sections 2.2/13.8) as "Stream(T)" — deliberately its own encoding rather
// than reusing Chan(T) even though both ultimately compile to the same Go
// channel shape (CLAUDE.md's "確定した設計判断" for step 12): amifl-spec.md
// section 17.2#4 treats Stream[T]/Chan[T] as distinct types with no implicit
// conversion between them, mirroring Set[T]/Map[T,Bool]'s step-10 precedent
// (setGoTypeName's doc comment) of two AmiFL types sharing one Go
// representation but never sharing one canonical string or CHTYPE.
func makeStreamType(elem string) string {
	return "Stream(" + elem + ")"
}

func isStreamType(t string) bool {
	return strings.HasPrefix(t, "Stream(") && strings.HasSuffix(t, ")")
}

func streamElemType(t string) (string, bool) {
	if !isStreamType(t) {
		return "", false
	}
	return strings.TrimSuffix(strings.TrimPrefix(t, "Stream("), ")"), true
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
