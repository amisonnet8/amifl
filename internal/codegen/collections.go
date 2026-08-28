// collections.go compiles amifl-spec.md section 2.2's List[T]/Array[T;N],
// their shared `[...]` literal (ast.ListLit), postfix `[i]`/`[a:b]` access
// (ast.IndexExpr/IndexAssignExpr/SliceExpr), `for x in items { ... }`
// (ast.ForExpr) — step 7 — its `yield` form, `for x in items yield expr`
// (step 9, section 7), and, since step 10, `for` iterating a Set (the
// single-variable form) or a Map[K,V] (the two-variable `for k, v in m`
// form — genForMapStmt) via prepareForIteration's MPKEYS-based lowering.
// Set[T]/Map[K,V]'s own literal syntax and Go type are maps.go's job. See
// codegen.go's package doc for the surrounding step-by-step scope.
package codegen

import (
	"fmt"
	"strings"

	"github.com/amisonnet8/amifl/internal/ast"
)

// isListType/listElemType and isArrayType/arrayParts are codegen's own
// copies of sema's identical helpers (types.go's makeListType/
// makeArrayType and friends) — ast is codegen's and sema's only shared
// vocabulary (CLAUDE.md's リポジトリ構成), so these string conventions have
// to be independently understood here too, exactly like isTupleType above.
func isListType(t string) bool {
	return strings.HasPrefix(t, "List(") && strings.HasSuffix(t, ")")
}

func listElemType(t string) string {
	return strings.TrimSuffix(strings.TrimPrefix(t, "List("), ")")
}

func isArrayType(t string) bool {
	return strings.HasPrefix(t, "Array(") && strings.HasSuffix(t, ")")
}

// arrayParts splits "Array(Elem;Size)" back into its pieces, finding the
// *last* ";" (not the first) so a nested Array element (itself an
// "Array(...;...)" string, with its own inner ";") doesn't get split at
// the wrong point — see sema's types.go, whose arrayParts documents why
// this is unambiguous regardless of nesting depth.
func arrayParts(t string) (elem, size string) {
	inner := strings.TrimSuffix(strings.TrimPrefix(t, "Array("), ")")
	sep := strings.LastIndex(inner, ";")
	return inner[:sep], inner[sep+1:]
}

// listGoTypeName mints (or reuses) the synthesized Go/AMIVM slice type for
// one List[T] shape, keyed by its full canonical string.
func (p *program) listGoTypeName(canonical string) string {
	if name, ok := p.listTypes[canonical]; ok {
		return name
	}
	elemGoType := p.resolveGoType(listElemType(canonical))
	p.listSeq++
	name := fmt.Sprintf("AmiflList%d", p.listSeq)
	if p.listTypes == nil {
		p.listTypes = map[string]string{}
	}
	p.listTypes[canonical] = name

	fmt.Fprintf(&p.typeHeader, "SLTYPE\t^%s\t^%s\n", name, elemGoType)
	return name
}

// arrayGoTypeName mints (or reuses) the synthesized Go/AMIVM fixed-array
// type for one Array[T;N] dimension, keyed by its full canonical string.
// Always minted via ARTYPE, never AMIVM's inline `^[n]type1` type-token
// form — even though a single, non-nested dimension could use that form
// directly with no ARTYPE at all, this program always goes through a
// named type instead, uniformly with every other non-scalar type
// resolveGoType produces: several contexts (an outer Array's own element
// slot, an SLTYPE's element, an STTYPE FIELD) require a plain `^xxx_123`
// name and can't take an inline `^[n]xxx_123` composite token, and giving
// every Array a name up front means resolveGoType never has to know which
// context it's being asked for.
func (p *program) arrayGoTypeName(canonical string) string {
	if name, ok := p.arrayTypes[canonical]; ok {
		return name
	}
	elem, size := arrayParts(canonical)
	elemGoType := p.resolveGoType(elem)
	p.arraySeq++
	name := fmt.Sprintf("AmiflArray%d", p.arraySeq)
	if p.arrayTypes == nil {
		p.arrayTypes = map[string]string{}
	}
	p.arrayTypes[canonical] = name

	fmt.Fprintf(&p.typeHeader, "ARTYPE\t^%s\t^%s\t%s\n", name, elemGoType, size)
	return name
}

// genListLitValue emits `[v1, v2, ...]` (amifl-spec.md sections 2.2/3.1).
// sema has already decided, via v.ResolvedType, whether this resolves to
// a List (declare + SLMAKE, then ASET per element) or an Array (declare
// only — VAR of a fixed-size type zero-initializes it — then ASET per
// element); ASET's own Go codegen (`single1[whole] = value1`) is
// identical either way, so only the declaration differs.
func (g *gen) genListLitValue(v *ast.ListLit) (string, error) {
	goType := g.prog.resolveGoType(v.ResolvedType)
	tmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", tmp, goType)
	if isListType(v.ResolvedType) {
		fmt.Fprintf(g.b, "\tSLMAKE\t%%%s\t^%s\t%d\n", tmp, goType, len(v.Elems))
	}
	for i, elem := range v.Elems {
		val, err := g.genValue(elem)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(g.b, "\tASET\t%%%s\t%d\t%s\n", tmp, i, val)
	}
	return "%" + tmp, nil
}

// genIndexValue emits `target[index]` (amifl-spec.md section 3.2) as AGET
// into a fresh temp of the element type — identical whether Target is a
// List (Go slice) or an Array (Go native array), since Go's own indexing
// syntax is the same either way (AGET's `variable[whole]` codegen).
func (g *gen) genIndexValue(v *ast.IndexExpr) (string, error) {
	targetVal, err := g.genValue(v.Target)
	if err != nil {
		return "", err
	}
	idxVal, err := g.genValue(v.Index)
	if err != nil {
		return "", err
	}
	goType := g.prog.resolveGoType(v.ResolvedType)
	tmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", tmp, goType)
	fmt.Fprintf(g.b, "\tAGET\t%%%s\t%s\t%s\n", tmp, targetVal, idxVal)
	return "%" + tmp, nil
}

// genIndexAssignStmt emits `target[index] = value` (amifl-spec.md section
// 3.2, "x[i] = v"). When target is itself an index expression (assigning
// into a nested Array/List — `matrix[i][j] = v`), a single ASET into
// genValue(target)'s result isn't always correct: genValue always
// produces a fresh copy when reading through a compound expression, and
// Go's native fixed-size Array is a genuine value type, so mutating that
// copy would silently never reach the original storage (this happens to
// work by accident for a List-of-Lists, since a slice header copy still
// aliases the same backing array — but not once an intermediate level is
// Array-typed, and this function doesn't get to assume which one it is —
// mirrors CLAUDE.md's "過去に踏まれた地雷" #8's identical FGET/FSET concern).
// sema's resolveIndexAssignExpr guarantees Target is either a plain
// identifier or a chain of IndexExprs bottoming out in one (never a
// struct field or other compound expression — a deliberate scope cut,
// see its doc comment), so the fix here only ever has to unwind through
// IndexExpr layers: read the innermost target into a temp, ASET into the
// temp, then recursively write the mutated temp back into *its* slot,
// unwinding outward until a plain variable is reached — correct for any
// nesting depth, with zero extra cost for the common single-level case
// (Target is already a bare identifier, so the recursion bottoms out
// immediately).
func (g *gen) genIndexAssignStmt(v *ast.IndexAssignExpr) error {
	idxVal, err := g.genValue(v.Index)
	if err != nil {
		return err
	}
	val, err := g.genValue(v.Value)
	if err != nil {
		return err
	}
	return g.emitIndexAssign(v.Target, idxVal, val)
}

// emitIndexAssign writes val into target[idx] (idx/val already-generated
// value tokens) — see genIndexAssignStmt's doc comment.
func (g *gen) emitIndexAssign(target ast.Expr, idx, val string) error {
	if inner, ok := target.(*ast.IndexExpr); ok {
		innerTargetVal, err := g.genValue(inner.Target)
		if err != nil {
			return err
		}
		innerIdxVal, err := g.genValue(inner.Index)
		if err != nil {
			return err
		}
		goType := g.prog.resolveGoType(inner.ResolvedType)
		tmp := g.newTemp()
		fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", tmp, goType)
		fmt.Fprintf(g.b, "\tAGET\t%%%s\t%s\t%s\n", tmp, innerTargetVal, innerIdxVal)
		fmt.Fprintf(g.b, "\tASET\t%%%s\t%s\t%s\n", tmp, idx, val)
		return g.emitIndexAssign(inner.Target, innerIdxVal, "%"+tmp)
	}
	targetVal, err := g.genValue(target)
	if err != nil {
		return err
	}
	fmt.Fprintf(g.b, "\tASET\t%s\t%s\t%s\n", targetVal, idx, val)
	return nil
}

// genSliceValue emits `target[from:to]` (amifl-spec.md section 3.2),
// always into a fresh List-typed temp (SliceExpr.ResolvedType, always a
// List[T] regardless of whether Target was a List or an Array — see
// ast.SliceExpr's doc comment). An omitted bound becomes AMIVM's `_`
// placeholder (the `from to` operand category, ignored/amivm/
// amivm_spec.md section 5) — computed here, not in the parser, since no
// AmiFL surface syntax ever produces a literal `_` token; only codegen,
// writing raw AMIVM text, needs one.
func (g *gen) genSliceValue(v *ast.SliceExpr) (string, error) {
	targetVal, err := g.genValue(v.Target)
	if err != nil {
		return "", err
	}
	from := "_"
	if v.From != nil {
		from, err = g.genValue(v.From)
		if err != nil {
			return "", err
		}
	}
	to := "_"
	if v.To != nil {
		to, err = g.genValue(v.To)
		if err != nil {
			return "", err
		}
	}
	goType := g.prog.resolveGoType(v.ResolvedType)
	tmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", tmp, goType)
	fmt.Fprintf(g.b, "\tSLICE\t%%%s\t%s\t%s\t%s\n", tmp, targetVal, from, to)
	return "%" + tmp, nil
}

// emitIndexLoopHeader emits the LOOP header shared by every step-7/9/10
// for-lowering (genForStmt, genForYieldValue): a fresh idx temp (plain Go
// `int`, never AmiFL-typed — the user's own code never sees the index
// itself) starts at -1, is incremented first thing inside LOOP, then
// bounds-checked against lenTmp and BREAKs once exhausted. The increment
// happens before the bounds check and before idx is used for anything, not
// after: CLAUDE.md's "過去に踏まれた地雷" #3 warns specifically against
// required per-iteration work placed *after* the body, since `continue`
// jumps straight back to LOOP's top and would skip it — starting at -1
// means the first increment lands on 0, and a `continue` inside the body
// simply re-enters the same increment-then-check sequence, correctly
// advancing to the next element every time, no LABEL/GOTO needed. Returns
// idxTmp so the caller can AGET/MGET whatever it needs with it before
// emitting the loop's own body.
func (g *gen) emitIndexLoopHeader(lenTmp string) string {
	idxTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^int\n", idxTmp)
	fmt.Fprintf(g.b, "\tSET\t%%%s\t-1\n", idxTmp)

	g.b.WriteString("\tLOOP\n")
	fmt.Fprintf(g.b, "\tADD\t%%%s\t%%%s\t1\n", idxTmp, idxTmp)
	doneTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^bool\n", doneTmp)
	fmt.Fprintf(g.b, "\tGTE\t%%%s\t%%%s\t%%%s\n", doneTmp, idxTmp, lenTmp)
	fmt.Fprintf(g.b, "\tIF\t%%%s\n", doneTmp)
	g.b.WriteString("\tBREAK\n")
	g.b.WriteString("\tENDIF\n")
	return idxTmp
}

// prepareForIteration returns the slice-shaped value a for-loop should
// AGET over (iterVal) and a temp holding its length (lenTmp), for every
// shape `for` accepts (step 10 adds Set/Map to step 7/9's List/Array): a
// List/Array is already index-addressable, so itemsVal is used directly
// and its length just comes from `?len` (Go's own builtin, called through
// AMIVM's raw-Go-function-name CALL form — a codegen-internal detail, not
// the user-facing `len` builtin step 11 adds); a Set or a Map isn't
// index-addressable at all, so its keys are first collected into a plain
// List[K] via MPKEYS (v.ElemType is already the Set's element type, or
// the Map's key type for the two-variable form — resolveForExpr's job,
// not this function's), and the rest of the loop then treats that exactly
// like an ordinary List. The two-variable Map form (genForMapStmt) calls
// this too, purely for the keys list and its length — it separately
// MGETs each iteration's value out of the original Map (itemsVal) itself,
// which this function has no reason to know about.
func (g *gen) prepareForIteration(v *ast.ForExpr, itemsVal string) (iterVal, lenTmp string) {
	lenTmp = g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^int\n", lenTmp)

	if isSetType(v.ItemsType) || isMapType(v.ItemsType) {
		keysGoType := g.prog.resolveGoType(makeListType(v.ElemType))
		keysTmp := g.newTemp()
		fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", keysTmp, keysGoType)
		fmt.Fprintf(g.b, "\tMPKEYS\t%%%s\t%s\n", keysTmp, itemsVal)
		g.writeCall("%"+lenTmp, "?len", []string{"%" + keysTmp})
		return "%" + keysTmp, lenTmp
	}
	g.writeCall("%"+lenTmp, "?len", []string{itemsVal})
	return itemsVal, lenTmp
}

// genForStmt lowers `for x in items { ... }` (amifl-spec.md section 7,
// List/Array/Set — step 10 adds Set) or `for k, v in m { ... }` (step 10,
// Map[K,V] — ast.ForExpr.Var2 set) into an index-based LOOP — AMIVM has no
// native for-each instruction. items itself is evaluated exactly once,
// matching how `while`'s condition, not the thing it tests, is what's
// re-evaluated per iteration.
func (g *gen) genForStmt(v *ast.ForExpr) error {
	itemsVal, err := g.genValue(v.Items)
	if err != nil {
		return err
	}

	if v.Var2 != "" {
		return g.genForMapStmt(v, itemsVal)
	}

	iterVal, lenTmp := g.prepareForIteration(v, itemsVal)
	idxTmp := g.emitIndexLoopHeader(lenTmp)

	elemGoType := g.prog.resolveGoType(v.ElemType)
	fmt.Fprintf(g.b, "\tVAR\t%s\t^%s\n", v.VarToken, elemGoType)
	fmt.Fprintf(g.b, "\tAGET\t%s\t%s\t%%%s\n", v.VarToken, iterVal, idxTmp)

	if err := g.genStmtBlock(v.Body.Exprs); err != nil {
		return err
	}
	g.b.WriteString("\tENDLOOP\n")
	return nil
}

// genForMapStmt lowers `for k, v in m { ... }` (step 10, ast.ForExpr.Var2
// set — always the Body form, never Yield, see Var2's doc comment):
// prepareForIteration collects m's keys into a plain List[K] via MPKEYS
// exactly as it would for a Set, and the loop AGETs each key from that
// list and then MGETs the matching value straight out of m (itemsVal)
// itself — safe with the single-result form of MGET (no `ok` needed) since
// a key freshly read from m's own MPKEYS is guaranteed present in m.
func (g *gen) genForMapStmt(v *ast.ForExpr, itemsVal string) error {
	iterVal, lenTmp := g.prepareForIteration(v, itemsVal)
	idxTmp := g.emitIndexLoopHeader(lenTmp)

	keyGoType := g.prog.resolveGoType(v.ElemType)
	fmt.Fprintf(g.b, "\tVAR\t%s\t^%s\n", v.VarToken, keyGoType)
	fmt.Fprintf(g.b, "\tAGET\t%s\t%s\t%%%s\n", v.VarToken, iterVal, idxTmp)

	valGoType := g.prog.resolveGoType(v.Var2Type)
	fmt.Fprintf(g.b, "\tVAR\t%s\t^%s\n", v.Var2Token, valGoType)
	fmt.Fprintf(g.b, "\tMGET\t%s\t%s\t%s\n", v.Var2Token, itemsVal, v.VarToken)

	if err := g.genStmtBlock(v.Body.Exprs); err != nil {
		return err
	}
	g.b.WriteString("\tENDLOOP\n")
	return nil
}

// genForExprStmt is genStmt's ast.ForExpr case: the Body form (Unit-typed,
// side-effect-only) runs genForStmt directly; the Yield form (step 9,
// always List(T)-typed) only ever reaches statement position via
// DiscardExpr's recursion (sema never lets a non-Unit expression appear
// undiscarded in statement position) — genForYieldValue still runs in
// full (Yield may itself contain side effects), its resulting List token
// simply discarded, exactly like every other "value expression that may
// contain an effectful sub-expression" genStmt already discards this way
// (TupleLit, StructLit, ListLit, ...).
func (g *gen) genForExprStmt(v *ast.ForExpr) error {
	if v.Yield != nil {
		_, err := g.genForYieldValue(v)
		return err
	}
	return g.genForStmt(v)
}

// genForYieldValue lowers `for x in items yield expr` (amifl-spec.md
// section 7, step 9; List/Array/Set — step 10 adds Set, via the same
// prepareForIteration MPKEYS treatment genForStmt uses, never a Map: Var2
// is never set here, the parser rejects `for k, v in m yield ...`
// outright — ast.ForExpr.Var2's doc comment) into a length-preallocated
// List built by a single loop — SLMAKE up front (unlike genListLitValue's
// List path, whose size is known from a literal's own element count, here
// it's prepareForIteration's own lenTmp, a runtime value), then the exact
// same increment-first LOOP structure genForStmt uses (emitIndexLoopHeader
// — see its doc comment for why the increment comes first), ASETting each
// iteration's Yield value into the preallocated slot instead of running
// Body purely for effect. This is a direct, single-loop compilation — not
// a literal call to a builtin named `map` (see ast.ForExpr's doc comment
// for why: capability-dispatched builtins don't exist until step 11).
func (g *gen) genForYieldValue(v *ast.ForExpr) (string, error) {
	itemsVal, err := g.genValue(v.Items)
	if err != nil {
		return "", err
	}

	iterVal, lenTmp := g.prepareForIteration(v, itemsVal)

	resultGoType := g.prog.resolveGoType(v.ResolvedType)
	resultTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", resultTmp, resultGoType)
	fmt.Fprintf(g.b, "\tSLMAKE\t%%%s\t^%s\t%%%s\n", resultTmp, resultGoType, lenTmp)

	idxTmp := g.emitIndexLoopHeader(lenTmp)

	elemGoType := g.prog.resolveGoType(v.ElemType)
	fmt.Fprintf(g.b, "\tVAR\t%s\t^%s\n", v.VarToken, elemGoType)
	fmt.Fprintf(g.b, "\tAGET\t%s\t%s\t%%%s\n", v.VarToken, iterVal, idxTmp)

	yieldVal, err := g.genValue(v.Yield)
	if err != nil {
		return "", err
	}
	fmt.Fprintf(g.b, "\tASET\t%%%s\t%%%s\t%s\n", resultTmp, idxTmp, yieldVal)

	g.b.WriteString("\tENDLOOP\n")
	return "%" + resultTmp, nil
}
