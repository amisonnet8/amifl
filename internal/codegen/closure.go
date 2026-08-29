// closure.go compiles amifl-spec.md section 8.1's local closure literals
// (`let f = fn(x: Int) -> Int { x * x }`) and, since ex3, every other
// producer of a Func-typed Go function type — see ast.ClosureLit's and
// ast.FuncType's own doc comments for the surrounding scope (a closure
// literal only ever appears as a `let`'s direct value; a Func-typed value
// more generally may now flow through a parameter, a return type, or a
// passed-by-name top-level `fn` reference).
package codegen

import (
	"fmt"
	"strings"

	"github.com/amisonnet8/amifl/internal/ast"
)

// program holds state shared across every function Generate compiles in
// one call — including the synthesized FNTYPE declarations every Func-
// typed Go function type needs (amivm_spec.md section 4.19: `CLOS`'s
// target must already be VAR-declared under a named function type, exactly
// like any other VAR/SET pair — there is no "declare and assign a closure
// in one step" instruction). closureSeq is the single counter behind every
// synthesized "AmiflFuncN" name, shared by two different mint paths:
// newFuncTypeDecl's own always-fresh, uncached use (chan.go's internal
// Stream relay closures — synthetic, with no canonical AmiFL Func type of
// their own to key a cache on) and funcGoTypeName's canonical-string-keyed
// cache below (every AmiFL-visible Func shape) — one shared counter keeps
// the two mint paths from ever emitting the same name twice.
type program struct {
	typeHeader strings.Builder
	closureSeq int

	// funcTypes backs funcGoTypeName — a Func shape's synthesized FNTYPE,
	// minted once per distinct canonical "fn(P1,...)->R" string and reused
	// program-wide (resolveGoType's own doc comment explains why this
	// sharing, unlike step 5's original per-literal-fresh minting, is now
	// load-bearing rather than optional).
	funcTypes map[string]string

	// tupleTypes/tupleSeq back structs.go's tupleGoTypeName — a tuple
	// shape's synthesized STTYPE, unlike a closure's FNTYPE, is minted
	// once per distinct canonical shape and reused (see resolveGoType's
	// doc comment for why).
	tupleTypes map[string]string
	tupleSeq   int

	// listTypes/listSeq and arrayTypes/arraySeq back collections.go's
	// listGoTypeName/arrayGoTypeName — step 7's List[T]/Array[T;N], minted
	// and reused the same deduplicated-per-shape way tuples are (and for
	// the same reason: a List[T]/Array[T;N] can recur as a struct field,
	// a function parameter/return type, or another List/Array's own
	// element type, and all of those should agree on one Go type per
	// shape).
	listTypes  map[string]string
	listSeq    int
	arrayTypes map[string]string
	arraySeq   int

	// setTypes/setSeq and mapTypes/mapSeq back maps.go's setGoTypeName/
	// mapGoTypeName — step 10's Set[T]/Map[K,V], minted and reused the
	// same deduplicated-per-shape way List/Array are, each via its own
	// MPTYPE (Set[T] and a structurally-identical Map[T,Bool] mint two
	// separate MPTYPE declarations rather than sharing one — see
	// setGoTypeName's doc comment for why that's fine).
	setTypes map[string]string
	setSeq   int
	mapTypes map[string]string
	mapSeq   int

	// chanTypes/chanSeq and streamTypes/streamSeq back chan.go's
	// chanGoTypeName/streamGoTypeName — step 12's Chan[T]/Stream[T], minted
	// and reused the same deduplicated-per-shape way Set/Map are, each via
	// its own CHTYPE (Chan[T] and a structurally-identical Stream[T] mint
	// two separate CHTYPE declarations rather than sharing one — see
	// streamGoTypeName's doc comment, mirroring setGoTypeName's step-10
	// precedent for why that's fine).
	chanTypes   map[string]string
	chanSeq     int
	streamTypes map[string]string
	streamSeq   int

	// rangeGoType backs structs.go's rangeGoTypeName — amifl-spec.md
	// section 3.1/7.3's `a..b`/`a..=b` Range value. Unlike every shape-
	// keyed map above (Tuple/List/Array/Set/Map/Chan/Stream, one Go type
	// per distinct element-type shape), Range has exactly one possible
	// shape at all — its two bounds are always Int64, with no type
	// parameter of its own (ex2's deliberate scope cut, mirroring how
	// Error/Unit/File are also fixed, unparameterized types) — so a
	// single cached string, minted at most once, is enough; "" means not
	// yet minted.
	rangeGoType string

	// externTypes backs resolveGoType's own step-13 case: every extern
	// `type Name` declaration maps its AmiFL name straight to "alias.Name"
	// (Generate populates this once, up front, from every ast.ExternDecl in
	// the file — see its own doc comment) — unlike every map above, this
	// one is never minted lazily on first use, since there's no synthesized
	// Go type to deduplicate here at all: the Go type already exists,
	// verbatim, in the target package.
	externTypes map[string]string

	// pkgPrefix is step 14's mechanical rename prefix (amifl-spec.md
	// section 12.4) for whichever package's own declarations are currently
	// being generated — "" for the root package, else that package's own
	// canonical alias plus "_" (see ast.ImportDecl's doc comment).
	// GenerateProgram sets this once per Unit, held constant across that
	// whole unit's own struct/enum declarations and function bodies, which
	// is always the correct context: a bare struct/enum type name or an
	// unqualified function call can only ever refer to a declaration in the
	// *same* package (step 14's deliberate scope cut — see CLAUDE.md's
	// "確定した設計判断" — never reaches another package's struct/enum, and
	// a cross-package function call goes through ast.FieldExpr.
	// QualifiedCallee instead, already fully resolved by sema with the
	// *target* package's own prefix baked in, never consulting this field
	// at all). Every synthesized shape-keyed type (tuple/list/array/set/
	// map/chan/stream/closure) deliberately ignores this — those are shared
	// program-wide by construction (resolveGoType's own doc comment), so a
	// List[Int] flowing across a qualified call still compiles to the same
	// Go type on both sides.
	pkgPrefix string
}

// newFuncTypeDecl emits one FNTYPE line for a function value shaped by
// paramGoTypes/retGoType (retGoType == "" for a Unit-returning function —
// FNTYPE's own return-type segment is then left empty, matching FUNC's
// same "no result list at all" treatment for Unit — see genFuncDecl),
// unconditionally minting a fresh "AmiflFuncN" name every call — the
// low-level primitive behind two different callers with two different
// needs: chan.go's internal Stream relay closures call this directly,
// uncached, since those have no canonical AmiFL Func type to key a cache
// on at all; funcGoTypeName below wraps this with exactly that caching for
// every AmiFL-visible Func shape. Both draw from the same p.closureSeq
// counter, so the two mint paths never collide on a name.
func (p *program) newFuncTypeDecl(paramGoTypes []string, retGoType string) string {
	p.closureSeq++
	name := fmt.Sprintf("AmiflFunc%d", p.closureSeq)
	fmt.Fprintf(&p.typeHeader, "FNTYPE\t^%s", name)
	for _, t := range paramGoTypes {
		fmt.Fprintf(&p.typeHeader, "\t^%s", t)
	}
	p.typeHeader.WriteString("\t:")
	if retGoType != "" {
		fmt.Fprintf(&p.typeHeader, "\t^%s", retGoType)
	}
	p.typeHeader.WriteString("\n")
	return name
}

// funcGoTypeName mints (or reuses) the shared Go/AMIVM function type for
// one Func canonical shape ("fn(P1,...)->R" — sema's makeFuncType/
// funcTypeParts), keyed by that full string exactly like tupleGoTypeName
// keys a tuple shape — see resolveGoType's own doc comment for why this
// sharing is load-bearing here, not just an optimization: Go requires two
// *named* function types to be identical, not just structurally alike,
// for one to be assignable to the other, so every closure literal,
// passed-by-name top-level `fn` reference, and Func-typed parameter/
// return/let-annotation of the same shape must resolve to this exact same
// name or a perfectly valid AmiFL program simply won't compile as Go.
func (p *program) funcGoTypeName(canonical string) string {
	if name, ok := p.funcTypes[canonical]; ok {
		return name
	}
	params, ret, _ := funcTypeParts(canonical)
	// Resolved before newFuncTypeDecl writes this FNTYPE's own header line
	// — see tupleGoTypeName's identical fix/doc comment for why
	// interleaving a nested type's own header mid-declaration would be
	// wrong (CLAUDE.md's step-13 "STTYPE/ENDSTTYPE内側でネストした型宣言を
	// 発行してはいけない" lesson, equally true of FNTYPE's own single-line
	// form — resolveGoType for a param/ret can still mint an entirely new
	// nested SLTYPE/MPTYPE/etc mid-resolution).
	paramGoTypes := make([]string, len(params))
	for i, pt := range params {
		paramGoTypes[i] = p.resolveGoType(pt)
	}
	var retGoType string
	if ret != unitType {
		retGoType = p.resolveGoType(ret)
	}
	name := p.newFuncTypeDecl(paramGoTypes, retGoType)
	if p.funcTypes == nil {
		p.funcTypes = map[string]string{}
	}
	p.funcTypes[canonical] = name
	return name
}

// genClosureLitInto compiles lit directly into token — the `let`'s own
// pre-computed value token (sema's freshInternalName), which VAR
// declares here before CLOS assigns into it. Closure parameters and
// captured outer bindings need no codegen-level handling at all beyond
// this: sema already baked every reference's full token ("&L-N" for a
// closure parameter, whatever the outer binding's own token is for a
// capture) directly onto each ast.IdentExpr.Token, and genValue's
// IdentExpr case just returns it — so a captured `%x_3`/`$1`/`&1-1` "just
// works" here exactly as it would anywhere else, by construction, with no
// depth bookkeeping in codegen at all (contrast sema's funcChecker.
// closureDepth, needed only to *compute* those tokens once at declaration
// time).
//
// The body recurses through the *same* gen (see gen's own doc comment in
// codegen.go for why sharing tmpSeq/prog is safe and sufficient) — CLOS,
// like IF/LOOP, is a genuinely nested Go block (here, a nested Go func
// literal), so its instructions are simply written inline into g.b at
// the point genLetStmt encounters the closure, precisely mirroring
// genIfBranch/genWhileStmt's existing approach (step 4) rather than
// needing any goto/VAR-hoisting machinery of its own.
func (g *gen) genClosureLitInto(token string, lit *ast.ClosureLit) error {
	// lit.ResolvedType is already the canonical "fn(P1,...)->R" string
	// sema computed (checkClosureBody) — funcGoTypeName decodes it back
	// into paramGoTypes/retGoType itself (closureGoTypes below does the
	// identical decoding for CLOS's own operand list), so this routes
	// through the same shared, deduplicated cache every other Func-typed
	// position now uses (resolveGoType's own doc comment) instead of
	// step 5's original always-fresh newFuncTypeDecl call — load-bearing
	// since ex3, not merely tidier: a closure passed into a Func-typed
	// parameter of the same shape, or reassigned into another Func-typed
	// variable, needs the identical Go type to type-check at all.
	paramGoTypes, retGoType := g.closureGoTypes(lit.ResolvedType)
	typeName := g.prog.funcGoTypeName(lit.ResolvedType)
	fmt.Fprintf(g.b, "\tVAR\t%s\t^%s\n", token, typeName)

	fmt.Fprintf(g.b, "\tCLOS\t%s", token)
	for _, t := range paramGoTypes {
		fmt.Fprintf(g.b, "\t^%s", t)
	}
	g.b.WriteString("\t:")
	if retGoType != "" {
		fmt.Fprintf(g.b, "\t^%s", retGoType)
	}
	g.b.WriteString("\n")

	savedRetType := g.retType
	g.retType = lit.ResolvedReturnType
	var bodyErr error
	if lit.ResolvedReturnType == unitType {
		if bodyErr = g.genStmtBlock(lit.Body.Exprs); bodyErr == nil {
			g.b.WriteString("\tRET\n")
		}
	} else {
		bodyErr = g.genBlock(lit.Body.Exprs)
	}
	g.retType = savedRetType
	if bodyErr != nil {
		return bodyErr
	}
	g.b.WriteString("\tENDCLOS\n")
	return nil
}

// genFuncRefValue materializes v — a bare reference to a *top-level* `fn`
// or extern plain-callee bind, resolved as a value rather than called
// directly (ex3, sema's resolveIdentExpr sets v.IsFuncRef) — via AMIVM's
// FUNCVAL instruction (`local := callname`, amivm_spec.md section 4.20).
// Unlike every other IdentExpr, there is no pre-existing runtime binding/
// token to just hand back here, so this is the one genValue IdentExpr case
// that actually emits an instruction. FUNCVAL's own local operand is never
// VAR-predeclared (amivm_spec.md's own note on this, mirrored by METHVAL's
// identical rule — extern.go's externCallee already relies on it), so,
// unlike genLetStmt's ordinary VAR-then-SET pattern, this one line is the
// entire instruction.
func (g *gen) genFuncRefValue(v *ast.IdentExpr) (string, error) {
	callname := v.FuncRefCallee
	if callname == "" {
		// A plain top-level `fn`, mirroring calleeToken()'s identical
		// derivation for an ordinary call — sema has no pkgPrefix of its
		// own to bake in (program.pkgPrefix's doc comment), so codegen
		// derives it here instead, substituting the internal entry-point
		// name for "main" exactly like calleeToken does.
		name := g.prog.pkgPrefix + v.Name
		if g.prog.pkgPrefix == "" && v.Name == "main" {
			name = entryFunc
		}
		callname = "!" + name
	}
	tmp := g.newTemp()
	fmt.Fprintf(g.b, "\tFUNCVAL\t%%%s\t%s\n", tmp, callname)
	return "%" + tmp, nil
}
