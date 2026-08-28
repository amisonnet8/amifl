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
	p.typeHeader.WriteString("FNTYPE\t^" + name)
	for _, t := range paramGoTypes {
		p.typeHeader.WriteString("\t^" + t)
	}
	p.typeHeader.WriteString("\t:")
	if retGoType != "" {
		p.typeHeader.WriteString("\t^" + retGoType)
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
	var paramGoTypes []string
	for _, p := range lit.Params {
		t, ok := goTypeNames[p.ResolvedType]
		if !ok {
			return fmt.Errorf("codegen: unknown type %q (sema should have rejected this)", p.ResolvedType)
		}
		paramGoTypes = append(paramGoTypes, t)
	}
	var retGoType string
	if lit.ResolvedReturnType != unitType {
		t, ok := goTypeNames[lit.ResolvedReturnType]
		if !ok {
			return fmt.Errorf("codegen: unknown type %q (sema should have rejected this)", lit.ResolvedReturnType)
		}
		retGoType = t
	}

	typeName := g.prog.newFuncTypeDecl(paramGoTypes, retGoType)
	fmt.Fprintf(g.b, "\tVAR\t%s\t^%s\n", token, typeName)

	g.b.WriteString("\tCLOS\t" + token)
	for _, t := range paramGoTypes {
		g.b.WriteString("\t^" + t)
	}
	g.b.WriteString("\t:")
	if retGoType != "" {
		g.b.WriteString("\t^" + retGoType)
	}
	g.b.WriteString("\n")

	if lit.ResolvedReturnType == unitType {
		if err := g.genStmtBlock(lit.Body.Exprs); err != nil {
			return err
		}
		g.b.WriteString("\tRET\n")
	} else if err := g.genBlock(lit.Body.Exprs); err != nil {
		return err
	}
	g.b.WriteString("\tENDCLOS\n")
	return nil
}
