// closure.go compiles amifl-spec.md section 8.1's local closure literals
// (`let f = fn(x: Int) -> Int { x * x }`) — see codegen.go's package doc
// and ast.ClosureLit's doc comment for the step-5 scope this operates
// under (a closure literal only ever appears as a `let`'s direct value).
package codegen

import (
	"fmt"
	"strings"

	"github.com/amisonnet8/amifl/internal/ast"
)

// program holds state shared across every function Generate compiles in
// one call — currently just the synthesized FNTYPE declarations every
// closure literal's own Go function type needs (amivm_spec.md section
// 4.19: `CLOS`'s target must already be VAR-declared under a named
// function type, exactly like any other VAR/SET pair — there is no
// "declare and assign a closure in one step" instruction). Unlike
// goTypeNames' scalar lookups, a closure's Go type has no natural shared
// name to reuse, so each closure literal simply mints its own ("AmiflFuncN"
// in first-encountered order) rather than de-duplicating identically-
// shaped closures — step 5 never needs two closures to share a Go type
// (no Func-typed parameters/return types exist yet to require matching an
// external annotation — see ast.ClosureLit's doc comment), so
// deduplication would add bookkeeping with no payoff yet.
type program struct {
	typeHeader strings.Builder
	closureSeq int

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

// newFuncTypeDecl emits one FNTYPE line for a closure shaped by
// paramGoTypes/retGoType (retGoType == "" for a Unit-returning closure —
// FNTYPE's own return-type segment is then left empty, matching FUNC's
// same "no result list at all" treatment for Unit — see genFuncDecl) and
// returns the synthesized Go type name to declare the closure's VAR
// under.
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
	// A ClosureLit's own params/return are always plain scalars when it's
	// written as a `let`'s direct value (step 5's scope), but the *same*
	// let-bound closure is routinely passed to map/filter/reduce/sortBy
	// (step 11) as the List/Array element's own type — which can be a
	// compound type (Tuple/List/Map/Set/struct, whatever the collection
	// holds). A direct goTypeNames[...] lookup here only ever has scalar
	// entries, so it must go through resolveGoType (the same dispatcher
	// every other type-to-Go-type site uses) rather than duplicate its
	// scalar-only fallback — found via a Tuple2-typed reduce accumulator
	// in examples/run_length_encode.aml (step 15's examples expansion).
	var paramGoTypes []string
	for _, p := range lit.Params {
		paramGoTypes = append(paramGoTypes, g.prog.resolveGoType(p.ResolvedType))
	}
	var retGoType string
	if lit.ResolvedReturnType != unitType {
		retGoType = g.prog.resolveGoType(lit.ResolvedReturnType)
	}

	typeName := g.prog.newFuncTypeDecl(paramGoTypes, retGoType)
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
