// builtins_data.go compiles amifl-spec.md section 13.4's data-manipulation
// built-ins (step 11 phase 11b) — the codegen half of sema/
// builtins_data.go's dispatch. Every resolver there already picked the one
// capability group (2.3節) a call belongs to and recorded the concrete
// types codegen needs on c.ArgTypes/c.ResolvedType; this file only ever
// reads those back, the same discipline builtins.go's isError/cast/parse
// already established.
package codegen

import (
	"fmt"
	"strings"

	"github.com/amisonnet8/amifl/internal/ast"
)

// funcTypeParts is codegen's own copy of sema's identical decoder
// (types.go's "fn(P1,P2,...)->R" convention) — needed here because map/
// filter/reduce/sortBy's closure argument type has to be split back into
// its parameter/return types to mint the right Go type arguments for
// amiflrt's generic helpers. paramsRaw is split with splitTopLevelCommas
// (structs.go), not a plain strings.Split — a param can itself be a
// compound type (a List/Array element passed through map/filter/reduce/
// sortBy is under no scalar-only restriction) and so may contain a "," of
// its own; see sema/types.go's identical fix for the full explanation.
func funcTypeParts(t string) (params []string, ret string, ok bool) {
	if !strings.HasPrefix(t, "fn(") {
		return nil, "", false
	}
	sep := strings.Index(t, ")->")
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

// genLenValue emits `len(x)` (amifl-spec.md section 13.4) — Go's own
// builtin `len` already works natively on every Lenable type (String,
// slice, array by value, map — the same `?len` call the `for` loop's own
// length-probing codegen already uses, step 7's prepareForIteration), so
// no amiflrt helper is needed; only the platform-`int` -> `Int64` cast
// step-1's main/amifl_main bridge already established.
func (g *gen) genLenValue(c *ast.CallExpr) (string, error) {
	argVal, err := g.genValue(c.Args[0])
	if err != nil {
		return "", err
	}
	rawTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^int\n", rawTmp)
	g.writeCall("%"+rawTmp, "?len", []string{argVal})
	tmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^int64\n", tmp)
	g.writeCall("%"+tmp, "?int64", []string{"%" + rawTmp})
	return "%" + tmp, nil
}

// genSliceBuiltinValue emits `slice(x, from, to)` (amifl-spec.md section
// 13.4) — AMIVM's SLICE instruction already generates plain Go slice
// syntax (`x[from:to]`), which works identically for a string (yielding a
// substring) and a slice (List/Array, both step 7 established), so no
// per-domain branching is needed beyond the two already-generated bound
// values.
func (g *gen) genSliceBuiltinValue(c *ast.CallExpr) (string, error) {
	targetVal, err := g.genValue(c.Args[0])
	if err != nil {
		return "", err
	}
	fromVal, err := g.genValue(c.Args[1])
	if err != nil {
		return "", err
	}
	toVal, err := g.genValue(c.Args[2])
	if err != nil {
		return "", err
	}

	// Stream[T] (step 12) can't use SLICE at all — AMIVM's own SLICE
	// instruction always generates Go's native `x[from:to]` syntax, which a
	// channel can't be subjected to. slice(s, from, to) is composed instead
	// as skip(s, from) |> take(_, to-from) (chan.go's genSkipStream/
	// genTakeStream), reusing exactly the codegen the named `skip`/`take`
	// built-ins themselves use.
	if isStreamType(c.ArgTypes[0]) {
		skipped, err := g.genSkipStream(targetVal, fromVal, c.ArgTypes[0], c.ArgTypes[0])
		if err != nil {
			return "", err
		}
		nTmp := g.newTemp()
		fmt.Fprintf(g.b, "\tVAR\t%%%s\t^int64\n", nTmp)
		fmt.Fprintf(g.b, "\tSUB\t%%%s\t%s\t%s\n", nTmp, toVal, fromVal)
		return g.genTakeStream(skipped, "%"+nTmp, c.ArgTypes[0], c.ResolvedType)
	}

	goType := g.prog.resolveGoType(c.ResolvedType)
	tmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", tmp, goType)
	fmt.Fprintf(g.b, "\tSLICE\t%%%s\t%s\t%s\t%s\n", tmp, targetVal, fromVal, toVal)
	return "%" + tmp, nil
}

// genAtValue emits `at(x, i)` (amifl-spec.md section 13.4) — a plain AGET,
// exactly like the `x[i]` sugar it's an alternate spelling of
// (collections.go's genIndexValue).
func (g *gen) genAtValue(c *ast.CallExpr) (string, error) {
	targetVal, err := g.genValue(c.Args[0])
	if err != nil {
		return "", err
	}
	idxVal, err := g.genValue(c.Args[1])
	if err != nil {
		return "", err
	}
	goType := g.prog.resolveGoType(c.ResolvedType)
	tmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", tmp, goType)
	fmt.Fprintf(g.b, "\tAGET\t%%%s\t%s\t%s\n", tmp, targetVal, idxVal)
	return "%" + tmp, nil
}

// genSetAtStmt emits `setAt(x, i, v)` (amifl-spec.md section 13.4) — a
// plain ASET. Unlike the `x[i]=v` sugar (ast.IndexAssignExpr), x here is
// an ordinary call argument rather than a restricted assignable path
// (isAssignableIndexTarget) — but that restriction exists only to make the
// *nested* write-back case (`matrix[i][j]=v`) unambiguous; setAt has no
// such nesting; x is evaluated once via genValue exactly like any other
// argument, and ASET's `variable` operand only needs *some* bare token
// (which genValue always produces), not specifically x's original
// identifier — since List is a Go slice (a reference type), mutating
// through that temp copy of the slice header still writes into the same
// backing array the caller's own List value shares (CLAUDE.md's 過去に
// 踏まれた地雷 #7).
func (g *gen) genSetAtStmt(c *ast.CallExpr) error {
	targetVal, err := g.genValue(c.Args[0])
	if err != nil {
		return err
	}
	idxVal, err := g.genValue(c.Args[1])
	if err != nil {
		return err
	}
	val, err := g.genValue(c.Args[2])
	if err != nil {
		return err
	}
	fmt.Fprintf(g.b, "\tASET\t%s\t%s\t%s\n", targetVal, idxVal, val)
	return nil
}

// genContainsValue emits `contains(x, target)` (amifl-spec.md section
// 13.4), dispatching on c.ArgTypes[0]: String uses strings.Contains
// directly; List/Array uses amiflrt.Contains[T]; Map/Set both use MGET's
// `ok`-receiving form (amivm_spec.md section 4.10.6 — the same instruction
// step-10's Map already relies on, "MGETのokを受け取る形") since a Set is
// itself a `map[T]bool` (step 10's established representation) — no
// amiflrt helper needed for either.
func (g *gen) genContainsValue(c *ast.CallExpr) (string, error) {
	xTyp := c.ArgTypes[0]
	xVal, err := g.genValue(c.Args[0])
	if err != nil {
		return "", err
	}
	targetVal, err := g.genValue(c.Args[1])
	if err != nil {
		return "", err
	}
	switch {
	case xTyp == "String":
		tmp := g.newTemp()
		fmt.Fprintf(g.b, "\tVAR\t%%%s\t^bool\n", tmp)
		g.writeCall("%"+tmp, "?strings.Contains", []string{xVal, targetVal})
		return "%" + tmp, nil
	case isListType(xTyp) || isArrayType(xTyp):
		elem, _ := elementType(xTyp)
		elemGoType := g.prog.resolveGoType(elem)
		tmp := g.newTemp()
		fmt.Fprintf(g.b, "\tVAR\t%%%s\t^bool\n", tmp)
		g.writeGenericCall([]string{"%" + tmp}, "?amiflrt.Contains", []string{elemGoType}, []string{xVal, targetVal})
		return "%" + tmp, nil
	default: // Map or Set — both a Go map under the hood (step 10)
		valGoType := mapOrSetValueGoType(g.prog, xTyp)
		discardTmp := g.newTemp()
		fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", discardTmp, valGoType)
		okTmp := g.newTemp()
		fmt.Fprintf(g.b, "\tVAR\t%%%s\t^bool\n", okTmp)
		fmt.Fprintf(g.b, "\tMGET\t%%%s\t%%%s\t%s\t%s\n", discardTmp, okTmp, xVal, targetVal)
		return "%" + okTmp, nil
	}
}

// mapOrSetValueGoType returns the Go type MGET's discarded first result
// should be declared as for a Map[K,V] or Set[T] (whose value type is
// always Go `bool`, step 10) — shared by genContainsValue here and, in
// phase 11c, Map/Set's own get/keys-adjacent built-ins.
func mapOrSetValueGoType(p *program, t string) string {
	if isSetType(t) {
		return "bool"
	}
	_, val := mapKeyValueTypes(t)
	return p.resolveGoType(val)
}

// genIndexBuiltinValue emits `index(x, target) -> Tuple2[Int,Bool]`
// (amifl-spec.md section 13.4) — String uses amiflrt.StringIndex; List/
// Array uses amiflrt.IndexOf[T]. Both return a native (value, ok) pair
// (CLAUDE.md's established "amiflrt returns a Go multi-value, codegen
// assembles the Tuple2 STTYPE" convention, first used by parse[T]).
func (g *gen) genIndexBuiltinValue(c *ast.CallExpr) (string, error) {
	xTyp := c.ArgTypes[0]
	xVal, err := g.genValue(c.Args[0])
	if err != nil {
		return "", err
	}
	targetVal, err := g.genValue(c.Args[1])
	if err != nil {
		return "", err
	}

	idxTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^int64\n", idxTmp)
	okTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^bool\n", okTmp)
	if xTyp == "String" {
		g.writeCallMulti([]string{"%" + idxTmp, "%" + okTmp}, "?amiflrt.StringIndex", []string{xVal, targetVal})
	} else {
		elem, _ := elementType(xTyp)
		elemGoType := g.prog.resolveGoType(elem)
		g.writeGenericCall([]string{"%" + idxTmp, "%" + okTmp}, "?amiflrt.IndexOf", []string{elemGoType}, []string{xVal, targetVal})
	}

	tupleGoType := g.prog.resolveGoType(c.ResolvedType)
	tupleTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", tupleTmp, tupleGoType)
	fmt.Fprintf(g.b, "\tFSET\t%%%s\t>F0\t%%%s\n", tupleTmp, idxTmp)
	fmt.Fprintf(g.b, "\tFSET\t%%%s\t>F1\t%%%s\n", tupleTmp, okTmp)
	return "%" + tupleTmp, nil
}

// genStringUnaryValue emits a `(s: String) -> String` built-in (trim/
// upper/lower) as a direct call to the matching strings.* function.
func (g *gen) genStringUnaryValue(c *ast.CallExpr, goFn string) (string, error) {
	argVal, err := g.genValue(c.Args[0])
	if err != nil {
		return "", err
	}
	tmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^string\n", tmp)
	g.writeCall("%"+tmp, goFn, []string{argVal})
	return "%" + tmp, nil
}

// genStringPredicateValue emits a `(s: String, x: String) -> Bool`
// built-in (startsWith/endsWith) as a direct call to the matching
// strings.* function.
func (g *gen) genStringPredicateValue(c *ast.CallExpr, goFn string) (string, error) {
	argVals, err := g.genArgValues(c.Args)
	if err != nil {
		return "", err
	}
	tmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^bool\n", tmp)
	g.writeCall("%"+tmp, goFn, argVals)
	return "%" + tmp, nil
}

// genReplaceValue emits `replace(s, old, new) -> String` (amifl-spec.md
// section 13.4) via strings.ReplaceAll.
func (g *gen) genReplaceValue(c *ast.CallExpr) (string, error) {
	argVals, err := g.genArgValues(c.Args)
	if err != nil {
		return "", err
	}
	tmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^string\n", tmp)
	g.writeCall("%"+tmp, "?strings.ReplaceAll", argVals)
	return "%" + tmp, nil
}

// genSplitValue emits `split(s, sep) -> List[String]` (amifl-spec.md
// section 13.4) via strings.Split — the result's unnamed `[]string` is
// directly assignable to our named List(String) Go type (CLAUDE.md's
// established assignability argument for SLTYPE's defined-not-aliased
// declaration), so no amiflrt wrapper or explicit conversion is needed.
func (g *gen) genSplitValue(c *ast.CallExpr) (string, error) {
	argVals, err := g.genArgValues(c.Args)
	if err != nil {
		return "", err
	}
	goType := g.prog.resolveGoType(c.ResolvedType)
	tmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", tmp, goType)
	g.writeCall("%"+tmp, "?strings.Split", argVals)
	return "%" + tmp, nil
}

// genJoinValue emits `join(xs, sep) -> String` (amifl-spec.md section
// 13.4) via strings.Join — our named List(String) argument is directly
// assignable to strings.Join's unnamed `[]string` parameter, the mirror
// image of genSplitValue's assignability argument.
func (g *gen) genJoinValue(c *ast.CallExpr) (string, error) {
	argVals, err := g.genArgValues(c.Args)
	if err != nil {
		return "", err
	}
	tmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^string\n", tmp)
	g.writeCall("%"+tmp, "?strings.Join", argVals)
	return "%" + tmp, nil
}

// closureGoTypes splits a Func-typed argument's already-resolved type
// string (funcTypeParts) into the Go type names its parameter(s) and
// return type compile to — shared by map/filter/reduce/sortBy below to
// mint amiflrt's explicit `<<T,U>>` type arguments.
func (g *gen) closureGoTypes(fTyp string) (paramGoTypes []string, retGoType string) {
	params, ret, _ := funcTypeParts(fTyp)
	for _, p := range params {
		paramGoTypes = append(paramGoTypes, g.prog.resolveGoType(p))
	}
	return paramGoTypes, g.prog.resolveGoType(ret)
}

// genMapValue emits `map(xs, f) -> List[U]` (amifl-spec.md section 13.4)
// via amiflrt.MapSlice[T,U]. An Array argument is sliced (`arr[:]`) first
// — Go generics need a slice, not a fixed-size array (arrays of different
// N are different Go types, incompatible with a single type parameter).
func (g *gen) genMapValue(c *ast.CallExpr) (string, error) {
	xsVal, err := g.arrayAwareSliceValue(c.Args[0], c.ArgTypes[0])
	if err != nil {
		return "", err
	}
	fVal, err := g.genValue(c.Args[1])
	if err != nil {
		return "", err
	}
	paramTypes, retGoType := g.closureGoTypes(c.ArgTypes[1])
	goType := g.prog.resolveGoType(c.ResolvedType)
	tmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", tmp, goType)
	g.writeGenericCall([]string{"%" + tmp}, "?amiflrt.MapSlice", []string{paramTypes[0], retGoType}, []string{xsVal, fVal})
	return "%" + tmp, nil
}

// genFilterValue emits `filter(xs, f) -> List[T]` (amifl-spec.md section
// 13.4) via amiflrt.FilterSlice[T].
func (g *gen) genFilterValue(c *ast.CallExpr) (string, error) {
	xsVal, err := g.arrayAwareSliceValue(c.Args[0], c.ArgTypes[0])
	if err != nil {
		return "", err
	}
	fVal, err := g.genValue(c.Args[1])
	if err != nil {
		return "", err
	}
	paramTypes, _ := g.closureGoTypes(c.ArgTypes[1])
	goType := g.prog.resolveGoType(c.ResolvedType)
	tmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", tmp, goType)
	g.writeGenericCall([]string{"%" + tmp}, "?amiflrt.FilterSlice", []string{paramTypes[0]}, []string{xsVal, fVal})
	return "%" + tmp, nil
}

// genReduceValue emits `reduce(xs, init, f) -> U` (amifl-spec.md section
// 13.4) via amiflrt.Reduce[T,U].
func (g *gen) genReduceValue(c *ast.CallExpr) (string, error) {
	xsVal, err := g.arrayAwareSliceValue(c.Args[0], c.ArgTypes[0])
	if err != nil {
		return "", err
	}
	initVal, err := g.genValue(c.Args[1])
	if err != nil {
		return "", err
	}
	fVal, err := g.genValue(c.Args[2])
	if err != nil {
		return "", err
	}
	elem, _ := elementType(c.ArgTypes[0])
	elemGoType := g.prog.resolveGoType(elem)
	uGoType := g.prog.resolveGoType(c.ResolvedType)
	tmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", tmp, uGoType)
	g.writeGenericCall([]string{"%" + tmp}, "?amiflrt.Reduce", []string{elemGoType, uGoType}, []string{xsVal, initVal, fVal})
	return "%" + tmp, nil
}

// genSortValue emits `sort(xs) -> List[T]` (amifl-spec.md section 13.4)
// via amiflrt.SortSlice[T] (List only — sema's resolveSort already
// rejected Array).
func (g *gen) genSortValue(c *ast.CallExpr) (string, error) {
	xsVal, err := g.genValue(c.Args[0])
	if err != nil {
		return "", err
	}
	elem, _ := elementType(c.ArgTypes[0])
	elemGoType := g.prog.resolveGoType(elem)
	goType := g.prog.resolveGoType(c.ResolvedType)
	tmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", tmp, goType)
	g.writeGenericCall([]string{"%" + tmp}, "?amiflrt.SortSlice", []string{elemGoType}, []string{xsVal})
	return "%" + tmp, nil
}

// genSortByValue emits `sortBy(xs, opt) -> List[T]` (amifl-spec.md section
// 13.4) via amiflrt.SortBySlice[T,K].
func (g *gen) genSortByValue(c *ast.CallExpr) (string, error) {
	xsVal, err := g.genValue(c.Args[0])
	if err != nil {
		return "", err
	}
	optVal, err := g.genValue(c.Args[1])
	if err != nil {
		return "", err
	}
	elem, _ := elementType(c.ArgTypes[0])
	elemGoType := g.prog.resolveGoType(elem)
	_, keyGoType := g.closureGoTypes(c.ArgTypes[1])
	goType := g.prog.resolveGoType(c.ResolvedType)
	tmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", tmp, goType)
	g.writeGenericCall([]string{"%" + tmp}, "?amiflrt.SortBySlice", []string{elemGoType, keyGoType}, []string{xsVal, optVal})
	return "%" + tmp, nil
}

// genReverseValue emits `reverse(xs) -> same shape` (amifl-spec.md section
// 13.4) — String via amiflrt.ReverseString, List via amiflrt.ReverseSlice
// [T], Array via a dedicated fixed-size in-place-shape-preserving loop
// (genReverseArrayValue) since Go's array value semantics don't fit the
// generic slice-returning helpers.
func (g *gen) genReverseValue(c *ast.CallExpr) (string, error) {
	xTyp := c.ArgTypes[0]
	xVal, err := g.genValue(c.Args[0])
	if err != nil {
		return "", err
	}
	switch {
	case xTyp == "String":
		tmp := g.newTemp()
		fmt.Fprintf(g.b, "\tVAR\t%%%s\t^string\n", tmp)
		g.writeCall("%"+tmp, "?amiflrt.ReverseString", []string{xVal})
		return "%" + tmp, nil
	case isListType(xTyp):
		elem, _ := elementType(xTyp)
		elemGoType := g.prog.resolveGoType(elem)
		goType := g.prog.resolveGoType(c.ResolvedType)
		tmp := g.newTemp()
		fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", tmp, goType)
		g.writeGenericCall([]string{"%" + tmp}, "?amiflrt.ReverseSlice", []string{elemGoType}, []string{xVal})
		return "%" + tmp, nil
	default: // Array[T;N]
		return g.genReverseArrayValue(c, xVal)
	}
}

// genReverseArrayValue builds a new Array[T;N] with xVal's elements in
// reverse order via a plain index loop (AGET reading from the high end,
// ASET writing from the low end) — reversal never changes an Array's
// length, so (unlike slice/filter) the fixed size is preserved, matching
// the input's own shape rather than falling back to a List the way
// map/filter/reduce do.
func (g *gen) genReverseArrayValue(c *ast.CallExpr, xVal string) (string, error) {
	elem, size := arrayParts(c.ArgTypes[0])
	elemGoType := g.prog.resolveGoType(elem)
	goType := g.prog.resolveGoType(c.ResolvedType)

	resultTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", resultTmp, goType)

	lenTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^int\n", lenTmp)
	fmt.Fprintf(g.b, "\tSET\t%%%s\t%s\n", lenTmp, size)

	idxTmp := g.emitIndexLoopHeader(lenTmp)
	srcIdxTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^int\n", srcIdxTmp)
	fmt.Fprintf(g.b, "\tSUB\t%%%s\t%%%s\t%%%s\n", srcIdxTmp, lenTmp, idxTmp)
	fmt.Fprintf(g.b, "\tSUB\t%%%s\t%%%s\t1\n", srcIdxTmp, srcIdxTmp)
	elemTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", elemTmp, elemGoType)
	fmt.Fprintf(g.b, "\tAGET\t%%%s\t%s\t%%%s\n", elemTmp, xVal, srcIdxTmp)
	fmt.Fprintf(g.b, "\tASET\t%%%s\t%%%s\t%%%s\n", resultTmp, idxTmp, elemTmp)
	g.b.WriteString("\tENDLOOP\n")

	return "%" + resultTmp, nil
}

// genUniqueValue emits `unique(xs) -> List[T]` (amifl-spec.md section
// 13.4) via amiflrt.Unique[T].
func (g *gen) genUniqueValue(c *ast.CallExpr) (string, error) {
	xsVal, err := g.genValue(c.Args[0])
	if err != nil {
		return "", err
	}
	elem, _ := elementType(c.ArgTypes[0])
	elemGoType := g.prog.resolveGoType(elem)
	goType := g.prog.resolveGoType(c.ResolvedType)
	tmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", tmp, goType)
	g.writeGenericCall([]string{"%" + tmp}, "?amiflrt.Unique", []string{elemGoType}, []string{xsVal})
	return "%" + tmp, nil
}

// genFlattenValue emits `flatten(xs) -> List[T]` (amifl-spec.md section
// 13.4) via amiflrt.Flatten[S,E] — S is the *outer* list's own element
// type (AmiFL's synthesized, named List[T] Go type), E the innermost
// element type; see Flatten's own doc comment (amiflrt/collections.go) for
// why a single type parameter isn't enough here, unlike every other
// generic helper in this file.
func (g *gen) genFlattenValue(c *ast.CallExpr) (string, error) {
	xsVal, err := g.genValue(c.Args[0])
	if err != nil {
		return "", err
	}
	outer, _ := elementType(c.ArgTypes[0])
	inner, _ := elementType(outer)
	outerGoType := g.prog.resolveGoType(outer)
	innerGoType := g.prog.resolveGoType(inner)
	goType := g.prog.resolveGoType(c.ResolvedType)
	tmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", tmp, goType)
	g.writeGenericCall([]string{"%" + tmp}, "?amiflrt.Flatten", []string{outerGoType, innerGoType}, []string{xsVal})
	return "%" + tmp, nil
}

// genZipValue emits `zip(xs, ys) -> List[Tuple2[A,B]]` (amifl-spec.md
// section 13.4) via a hand-written index loop (prepareForIteration/
// emitIndexLoopHeader's established shape) rather than a generic amiflrt
// helper — a Tuple2 is codegen's own synthesized STTYPE (structs.go), and
// a generic Go struct amiflrt could return wouldn't be assignable to it
// (two distinct named struct types, even with identical fields, need an
// explicit conversion in Go) — the same reasoning CLAUDE.md's "確定した
// 設計判断" already applied to parse[T]/pop's Tuple2 assembly, just with a
// loop body instead of a single FSET pair. The result length is
// min(len(xs), len(ys)), computed once up front.
func (g *gen) genZipValue(c *ast.CallExpr) (string, error) {
	xsVal, err := g.genValue(c.Args[0])
	if err != nil {
		return "", err
	}
	ysVal, err := g.genValue(c.Args[1])
	if err != nil {
		return "", err
	}
	aElem, _ := elementType(c.ArgTypes[0])
	bElem, _ := elementType(c.ArgTypes[1])
	aGoType := g.prog.resolveGoType(aElem)
	bGoType := g.prog.resolveGoType(bElem)

	xLenTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^int\n", xLenTmp)
	g.writeCall("%"+xLenTmp, "?len", []string{xsVal})
	yLenTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^int\n", yLenTmp)
	g.writeCall("%"+yLenTmp, "?len", []string{ysVal})

	lenTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^int\n", lenTmp)
	condTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^bool\n", condTmp)
	fmt.Fprintf(g.b, "\tLT\t%%%s\t%%%s\t%%%s\n", condTmp, xLenTmp, yLenTmp)
	fmt.Fprintf(g.b, "\tIF\t%%%s\n", condTmp)
	fmt.Fprintf(g.b, "\tSET\t%%%s\t%%%s\n", lenTmp, xLenTmp)
	g.b.WriteString("\tELSE\n")
	fmt.Fprintf(g.b, "\tSET\t%%%s\t%%%s\n", lenTmp, yLenTmp)
	g.b.WriteString("\tENDIF\n")

	resultGoType := g.prog.resolveGoType(c.ResolvedType)
	resultTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", resultTmp, resultGoType)
	fmt.Fprintf(g.b, "\tSLMAKE\t%%%s\t^%s\t%%%s\n", resultTmp, resultGoType, lenTmp)

	pairGoType := g.prog.resolveGoType(elemTypeOfList(c.ResolvedType))
	idxTmp := g.emitIndexLoopHeader(lenTmp)
	aTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", aTmp, aGoType)
	fmt.Fprintf(g.b, "\tAGET\t%%%s\t%s\t%%%s\n", aTmp, xsVal, idxTmp)
	bTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", bTmp, bGoType)
	fmt.Fprintf(g.b, "\tAGET\t%%%s\t%s\t%%%s\n", bTmp, ysVal, idxTmp)
	pairTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", pairTmp, pairGoType)
	fmt.Fprintf(g.b, "\tFSET\t%%%s\t>F0\t%%%s\n", pairTmp, aTmp)
	fmt.Fprintf(g.b, "\tFSET\t%%%s\t>F1\t%%%s\n", pairTmp, bTmp)
	fmt.Fprintf(g.b, "\tASET\t%%%s\t%%%s\t%%%s\n", resultTmp, idxTmp, pairTmp)
	g.b.WriteString("\tENDLOOP\n")

	return "%" + resultTmp, nil
}

// elemTypeOfList strips List(...)'s own wrapper, used only by genZipValue
// to recover the Tuple2[A,B] canonical string from c.ResolvedType
// (List(Tuple(A,B))) so resolveGoType mints/reuses the right STTYPE.
func elemTypeOfList(t string) string {
	e, _ := elementType(t)
	return e
}

// genPushValue emits `push(xs, v) -> List[T]` (amifl-spec.md section 13.4)
// via amiflrt.Push[T].
func (g *gen) genPushValue(c *ast.CallExpr) (string, error) {
	xsVal, err := g.genValue(c.Args[0])
	if err != nil {
		return "", err
	}
	vVal, err := g.genValue(c.Args[1])
	if err != nil {
		return "", err
	}
	elem, _ := elementType(c.ArgTypes[0])
	elemGoType := g.prog.resolveGoType(elem)
	goType := g.prog.resolveGoType(c.ResolvedType)
	tmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", tmp, goType)
	g.writeGenericCall([]string{"%" + tmp}, "?amiflrt.Push", []string{elemGoType}, []string{xsVal, vVal})
	return "%" + tmp, nil
}

// genPopValue emits `pop(xs) -> Tuple2[List[T], T]` (amifl-spec.md section
// 13.4) via amiflrt.Pop[T] — a native (list, elem) pair codegen assembles
// into the Tuple2 STTYPE, exactly like genIndexBuiltinValue above.
func (g *gen) genPopValue(c *ast.CallExpr) (string, error) {
	xsVal, err := g.genValue(c.Args[0])
	if err != nil {
		return "", err
	}
	elem, _ := elementType(c.ArgTypes[0])
	elemGoType := g.prog.resolveGoType(elem)

	restGoType := g.prog.resolveGoType(c.ArgTypes[0])
	restTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", restTmp, restGoType)
	lastTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", lastTmp, elemGoType)
	g.writeGenericCall([]string{"%" + restTmp, "%" + lastTmp}, "?amiflrt.Pop", []string{elemGoType}, []string{xsVal})

	tupleGoType := g.prog.resolveGoType(c.ResolvedType)
	tupleTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", tupleTmp, tupleGoType)
	fmt.Fprintf(g.b, "\tFSET\t%%%s\t>F0\t%%%s\n", tupleTmp, restTmp)
	fmt.Fprintf(g.b, "\tFSET\t%%%s\t>F1\t%%%s\n", tupleTmp, lastTmp)
	return "%" + tupleTmp, nil
}

// genInsertValue emits `insert(xs, i, v) -> List[T]` (amifl-spec.md
// section 13.4) via amiflrt.Insert[T].
func (g *gen) genInsertValue(c *ast.CallExpr) (string, error) {
	argVals, err := g.genArgValues(c.Args)
	if err != nil {
		return "", err
	}
	elem, _ := elementType(c.ArgTypes[0])
	elemGoType := g.prog.resolveGoType(elem)
	goType := g.prog.resolveGoType(c.ResolvedType)
	tmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", tmp, goType)
	g.writeGenericCall([]string{"%" + tmp}, "?amiflrt.Insert", []string{elemGoType}, argVals)
	return "%" + tmp, nil
}

// genRemoveAtValue emits `removeAt(xs, i) -> List[T]` (amifl-spec.md
// section 13.4) via amiflrt.RemoveAt[T].
func (g *gen) genRemoveAtValue(c *ast.CallExpr) (string, error) {
	argVals, err := g.genArgValues(c.Args)
	if err != nil {
		return "", err
	}
	elem, _ := elementType(c.ArgTypes[0])
	elemGoType := g.prog.resolveGoType(elem)
	goType := g.prog.resolveGoType(c.ResolvedType)
	tmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", tmp, goType)
	g.writeGenericCall([]string{"%" + tmp}, "?amiflrt.RemoveAt", []string{elemGoType}, argVals)
	return "%" + tmp, nil
}

// genConcatValue emits `concat(a, b) -> same type` (amifl-spec.md section
// 13.4) — String via AMIVM's own CONCAT instruction (Concatenable since
// step 3, already used for `+` on Strings), List via amiflrt.ConcatSlice
// [T].
func (g *gen) genConcatValue(c *ast.CallExpr) (string, error) {
	aVal, err := g.genValue(c.Args[0])
	if err != nil {
		return "", err
	}
	bVal, err := g.genValue(c.Args[1])
	if err != nil {
		return "", err
	}
	if c.ArgTypes[0] == "String" {
		tmp := g.newTemp()
		fmt.Fprintf(g.b, "\tVAR\t%%%s\t^string\n", tmp)
		fmt.Fprintf(g.b, "\tCONCAT\t%%%s\t%s\t%s\n", tmp, aVal, bVal)
		return "%" + tmp, nil
	}
	elem, _ := elementType(c.ArgTypes[0])
	elemGoType := g.prog.resolveGoType(elem)
	goType := g.prog.resolveGoType(c.ResolvedType)
	tmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", tmp, goType)
	g.writeGenericCall([]string{"%" + tmp}, "?amiflrt.ConcatSlice", []string{elemGoType}, []string{aVal, bVal})
	return "%" + tmp, nil
}

// arrayAwareSliceValue evaluates e (already known to have canonical type
// t) and, if t is an Array, slices it into a plain Go slice (`arr[:]`,
// valid since a VAR-declared array is addressable) before returning —
// map/filter/reduce's amiflrt helpers are generic over a *slice* type, and
// a fixed-size Go array `[N]T` doesn't unify with `[]T` the way a List
// already does (arrays of different N are different Go types entirely).
func (g *gen) arrayAwareSliceValue(e ast.Expr, t string) (string, error) {
	val, err := g.genValue(e)
	if err != nil {
		return "", err
	}
	if !isArrayType(t) {
		return val, nil
	}
	elem, _ := elementType(t)
	sliceGoType := g.prog.resolveGoType(elemToListType(elem))
	tmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", tmp, sliceGoType)
	fmt.Fprintf(g.b, "\tSLICE\t%%%s\t%s\t_\t_\n", tmp, val)
	return "%" + tmp, nil
}

// elemToListType wraps elem back into a List(...) canonical string —
// arrayAwareSliceValue's own tiny counterpart to sema's makeListType,
// needed only to name the Go slice type an Array-to-slice conversion
// declares its temp under.
func elemToListType(elem string) string {
	return "List(" + elem + ")"
}
