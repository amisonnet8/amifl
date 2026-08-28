// builtins_set_map.go compiles amifl-spec.md sections 13.5 (Set[T]) and
// 13.6 (Map[K,V])'s built-ins (step 11 phase 11c) — the codegen half of
// sema/builtins_set_map.go's dispatch.
package codegen

import (
	"fmt"

	"github.com/amisonnet8/amifl/internal/ast"
)

// genSetAddStmt emits `add(s, v) -> Unit` (amifl-spec.md section 13.5) —
// a plain MSET(s, v, true), mutating s in place (a Set is a Go
// map[T]bool, step 10).
func (g *gen) genSetAddStmt(c *ast.CallExpr) error {
	sVal, err := g.genValue(c.Args[0])
	if err != nil {
		return err
	}
	vVal, err := g.genValue(c.Args[1])
	if err != nil {
		return err
	}
	fmt.Fprintf(g.b, "\tMSET\t%s\t%s\ttrue\n", sVal, vVal)
	return nil
}

// genSetDiscardStmt emits `discard(s, v) -> Unit` (amifl-spec.md section
// 13.5) via Go's own builtin `delete` (the same `CALL ?builtin args...`
// pattern `len` already uses) — no amiflrt helper needed.
func (g *gen) genSetDiscardStmt(c *ast.CallExpr) error {
	sVal, err := g.genValue(c.Args[0])
	if err != nil {
		return err
	}
	vVal, err := g.genValue(c.Args[1])
	if err != nil {
		return err
	}
	g.writeCall("", "?delete", []string{sVal, vVal})
	return nil
}

// genSetBinaryOpValue emits union/intersect/difference(a, b) -> Set[T]
// (amifl-spec.md section 13.5) via the matching amiflrt set-algebra
// helper.
func (g *gen) genSetBinaryOpValue(c *ast.CallExpr, amiflrtFn string) (string, error) {
	aVal, err := g.genValue(c.Args[0])
	if err != nil {
		return "", err
	}
	bVal, err := g.genValue(c.Args[1])
	if err != nil {
		return "", err
	}
	elem := setElemType(c.ArgTypes[0])
	elemGoType := g.prog.resolveGoType(elem)
	goType := g.prog.resolveGoType(c.ResolvedType)
	tmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", tmp, goType)
	g.writeGenericCall([]string{"%" + tmp}, amiflrtFn, []string{elemGoType}, []string{aVal, bVal})
	return "%" + tmp, nil
}

// genSetToListValue emits `toList(s) -> List[T]` (amifl-spec.md section
// 13.5) via MPKEYS directly — identical to how a Set's own single-variable
// `for` iteration already collects its keys (collections.go's
// prepareForIteration), just surfaced here as its own value instead of
// feeding a loop.
func (g *gen) genSetToListValue(c *ast.CallExpr) (string, error) {
	sVal, err := g.genValue(c.Args[0])
	if err != nil {
		return "", err
	}
	goType := g.prog.resolveGoType(c.ResolvedType)
	tmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", tmp, goType)
	fmt.Fprintf(g.b, "\tMPKEYS\t%%%s\t%s\n", tmp, sVal)
	return "%" + tmp, nil
}

// genMapKeysValue emits `keys(m) -> List[K]` (amifl-spec.md section 13.6)
// via MPKEYS.
func (g *gen) genMapKeysValue(c *ast.CallExpr) (string, error) {
	return g.genSetToListValue(c) // identical: MPKEYS doesn't care whether the map's values are bool (Set) or something else
}

// genMapValuesValue emits `values(m) -> List[V]` (amifl-spec.md section
// 13.6) via amiflrt.MapValues[K,V] — there's no native instruction
// equivalent to MPKEYS for values.
func (g *gen) genMapValuesValue(c *ast.CallExpr) (string, error) {
	mVal, err := g.genValue(c.Args[0])
	if err != nil {
		return "", err
	}
	key, val := mapKeyValueTypes(c.ArgTypes[0])
	keyGoType := g.prog.resolveGoType(key)
	valGoType := g.prog.resolveGoType(val)
	goType := g.prog.resolveGoType(c.ResolvedType)
	tmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", tmp, goType)
	g.writeGenericCall([]string{"%" + tmp}, "?amiflrt.MapValues", []string{keyGoType, valGoType}, []string{mVal})
	return "%" + tmp, nil
}

// genMapEntriesValue emits `entries(m) -> List[Tuple2[K,V]]` (amifl-
// spec.md section 13.6) via a hand-written loop — MPKEYS collects m's
// keys into a List[K], then each iteration MGETs the matching value and
// FSETs a Tuple2[K,V] (the same "amiflrt/native multi-value in, codegen
// assembles the Tuple2 STTYPE" convention as parse[T]/pop/zip, since a
// generic Go struct amiflrt could return wouldn't be assignable to
// AmiFL's own synthesized STTYPE).
func (g *gen) genMapEntriesValue(c *ast.CallExpr) (string, error) {
	mVal, err := g.genValue(c.Args[0])
	if err != nil {
		return "", err
	}
	key, val := mapKeyValueTypes(c.ArgTypes[0])
	keyGoType := g.prog.resolveGoType(key)
	valGoType := g.prog.resolveGoType(val)

	keysGoType := g.prog.resolveGoType(elemToListType(key))
	keysTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", keysTmp, keysGoType)
	fmt.Fprintf(g.b, "\tMPKEYS\t%%%s\t%s\n", keysTmp, mVal)

	lenTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^int\n", lenTmp)
	g.writeCall("%"+lenTmp, "?len", []string{"%" + keysTmp})

	resultGoType := g.prog.resolveGoType(c.ResolvedType)
	resultTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", resultTmp, resultGoType)
	fmt.Fprintf(g.b, "\tSLMAKE\t%%%s\t^%s\t%%%s\n", resultTmp, resultGoType, lenTmp)

	pairGoType := g.prog.resolveGoType(elemTypeOfList(c.ResolvedType))
	idxTmp := g.emitIndexLoopHeader(lenTmp)
	kTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", kTmp, keyGoType)
	fmt.Fprintf(g.b, "\tAGET\t%%%s\t%%%s\t%%%s\n", kTmp, keysTmp, idxTmp)
	vTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", vTmp, valGoType)
	fmt.Fprintf(g.b, "\tMGET\t%%%s\t%s\t%%%s\n", vTmp, mVal, kTmp)
	pairTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", pairTmp, pairGoType)
	fmt.Fprintf(g.b, "\tFSET\t%%%s\t>F0\t%%%s\n", pairTmp, kTmp)
	fmt.Fprintf(g.b, "\tFSET\t%%%s\t>F1\t%%%s\n", pairTmp, vTmp)
	fmt.Fprintf(g.b, "\tASET\t%%%s\t%%%s\t%%%s\n", resultTmp, idxTmp, pairTmp)
	g.b.WriteString("\tENDLOOP\n")

	return "%" + resultTmp, nil
}

// genMapGetValue emits `get(m, k, default) -> V` (amifl-spec.md section
// 13.6) via MGET's `ok`-receiving form plus an IF/ELSE picking default
// when the key isn't present.
func (g *gen) genMapGetValue(c *ast.CallExpr) (string, error) {
	mVal, err := g.genValue(c.Args[0])
	if err != nil {
		return "", err
	}
	kVal, err := g.genValue(c.Args[1])
	if err != nil {
		return "", err
	}
	defaultVal, err := g.genValue(c.Args[2])
	if err != nil {
		return "", err
	}
	goType := g.prog.resolveGoType(c.ResolvedType)

	foundTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", foundTmp, goType)
	okTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^bool\n", okTmp)
	fmt.Fprintf(g.b, "\tMGET\t%%%s\t%%%s\t%s\t%s\n", foundTmp, okTmp, mVal, kVal)

	resultTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", resultTmp, goType)
	fmt.Fprintf(g.b, "\tIF\t%%%s\n", okTmp)
	fmt.Fprintf(g.b, "\tSET\t%%%s\t%%%s\n", resultTmp, foundTmp)
	g.b.WriteString("\tELSE\n")
	fmt.Fprintf(g.b, "\tSET\t%%%s\t%s\n", resultTmp, defaultVal)
	g.b.WriteString("\tENDIF\n")
	return "%" + resultTmp, nil
}

// genMapSetStmt emits `set(m, k, v) -> Unit` (amifl-spec.md section 13.6)
// — a plain MSET, mutating m in place.
func (g *gen) genMapSetStmt(c *ast.CallExpr) error {
	mVal, err := g.genValue(c.Args[0])
	if err != nil {
		return err
	}
	kVal, err := g.genValue(c.Args[1])
	if err != nil {
		return err
	}
	vVal, err := g.genValue(c.Args[2])
	if err != nil {
		return err
	}
	fmt.Fprintf(g.b, "\tMSET\t%s\t%s\t%s\n", mVal, kVal, vVal)
	return nil
}

// genMapDeleteStmt emits `delete(m, k) -> Unit` (amifl-spec.md section
// 13.6) via Go's own builtin `delete`.
func (g *gen) genMapDeleteStmt(c *ast.CallExpr) error {
	mVal, err := g.genValue(c.Args[0])
	if err != nil {
		return err
	}
	kVal, err := g.genValue(c.Args[1])
	if err != nil {
		return err
	}
	g.writeCall("", "?delete", []string{mVal, kVal})
	return nil
}
