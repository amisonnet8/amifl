// enum.go compiles amifl-spec.md section 2.2's `enum` declarations, their
// `EnumType.Variant(...)` construction syntax (ast.FieldExpr, when
// IsEnumVariant — see structs.go's genFieldValue), and section 10's
// subject-carrying `switch` pattern matching (ast.SwitchExpr) — step 8.
// See codegen.go's package doc for the surrounding step-by-step scope, and
// CLAUDE.md's "確定した設計判断" for step 8's chosen runtime
// representation.
package codegen

import (
	"fmt"

	"github.com/amisonnet8/amifl/internal/ast"
)

// genEnumDecl emits one user `enum` declaration's STTYPE block directly
// into prog.typeHeader — called for every ast.EnumDecl before any function
// body is generated (Generate), mirroring genStructDecl exactly.
//
// The representation is a single struct per enum: a `Tag` field (the
// variant's position in Name's own declared list, i.e. what
// genEnumVariantValue writes and genSwitchChain's EQ tests against) plus
// every variant's own fields, unioned together into that same struct and
// qualified `Variant_field` so that two different variants' same-named
// fields (or a coincidental clash with "Tag" itself — impossible, since
// every data field carries a variant prefix) never collide as Go struct
// fields. A field belonging to whichever variant *isn't* the one actually
// constructed is simply left at its Go zero value and never read — a
// switch case only ever FGETs a field once its own case's Tag comparison
// has already matched, so which variant is "really" live is always known
// by construction, not by inspecting the otherwise-unused fields.
func genEnumDecl(prog *program, d *ast.EnumDecl) {
	prog.typeHeader.WriteString("STTYPE\t^" + d.Name + "\n")
	prog.typeHeader.WriteString("\tFIELD\t>Tag\t^int\n")
	for _, variant := range d.Variants {
		for _, f := range variant.Fields {
			goType := prog.resolveGoType(f.ResolvedType)
			fmt.Fprintf(&prog.typeHeader, "\tFIELD\t>%s_%s\t^%s\n", variant.Name, f.Name, goType)
		}
	}
	prog.typeHeader.WriteString("ENDSTTYPE\n")
}

// genEnumVariantValue emits `EnumType.Variant` / `EnumType.Variant(field:
// v, ...)` (amifl-spec.md section 2.2): a fresh temp of the enum's own
// STTYPE (genEnumDecl), Tag FSET to the variant's declared position, then
// each argument FSET into its variant-qualified field name. v.Target is
// never read — unlike every other ast.FieldExpr use, Target here names a
// *type*, not a value (see FieldExpr's own doc comment on the three-way
// ambiguity sema resolves).
func (g *gen) genEnumVariantValue(v *ast.FieldExpr) (string, error) {
	tmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", tmp, v.ResolvedType)
	fmt.Fprintf(g.b, "\tFSET\t%%%s\t>Tag\t%d\n", tmp, v.VariantIndex)
	for _, a := range v.Args {
		val, err := g.genValue(a.Value)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(g.b, "\tFSET\t%%%s\t>%s_%s\t%s\n", tmp, v.Field, a.Name, val)
	}
	return "%" + tmp, nil
}

// genSwitchStmt/genSwitchValue lower a SwitchExpr (amifl-spec.md section
// 10) into a chain of nested IF/ELSE testing the subject's `Tag` field —
// AMIVM has no native pattern-matching instruction, so this mirrors
// genIfBranch/genIfValue's ELIF-as-nested-ELSE technique (CLAUDE.md's
// "過去に踏まれた地雷" #2) exactly, generalized from a Bool condition to an
// EQ-against-Tag test repeated once per case. The subject is evaluated
// exactly once, before the chain — matching WhileExpr's Cond/ForExpr's
// Items precedent of never re-running a side-effecting expression once
// per branch.
func (g *gen) genSwitchStmt(v *ast.SwitchExpr) error {
	subjectVal, err := g.genValue(v.Subject)
	if err != nil {
		return err
	}
	return g.genSwitchChain(v.Cases, 0, subjectVal, v.Default, nil)
}

func (g *gen) genSwitchValue(v *ast.SwitchExpr) (string, error) {
	subjectVal, err := g.genValue(v.Subject)
	if err != nil {
		return "", err
	}
	goType := g.prog.resolveGoType(v.ResolvedType)
	dest := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", dest, goType)
	if err := g.genSwitchChain(v.Cases, 0, subjectVal, v.Default, &dest); err != nil {
		return "", err
	}
	return "%" + dest, nil
}

// genSwitchChain emits cases[i:] (and, once cases is exhausted, def) as
// nested IF/ELSE, testing subjectVal's Tag field against each case's
// VariantIndex in turn. dest == nil runs every branch purely for effect
// (genStmtBlock, a Unit-typed switch); dest != nil writes each branch's
// value into the pre-declared temp *dest (genValueBlock) — exactly
// genIfBranch/genIfValueBranch's split, generalized from 2 branches to N.
// Reaching the end of cases with def == nil (an exhaustive switch, sema-
// guaranteed — resolveSwitchExpr requires either a default or full
// variant coverage) emits nothing further, the same way genElseStmt/
// genIfValueBranch's nil-else case does for an if with no else: sema
// having already required every possible Tag to be covered is what makes
// leaving no final ELSE safe (Go's own zero-initialized dest would
// otherwise silently surface if this were ever reached, but it can't be,
// by construction).
func (g *gen) genSwitchChain(cases []ast.SwitchCase, i int, subjectVal string, def *ast.Block, dest *string) error {
	if i == len(cases) {
		if def == nil {
			return nil
		}
		if dest != nil {
			return g.genValueBlock(def.Exprs, *dest)
		}
		return g.genStmtBlock(def.Exprs)
	}

	c := cases[i]
	tagTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^int\n", tagTmp)
	fmt.Fprintf(g.b, "\tFGET\t%%%s\t%s\t>Tag\n", tagTmp, subjectVal)
	condTmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^bool\n", condTmp)
	fmt.Fprintf(g.b, "\tEQ\t%%%s\t%%%s\t%d\n", condTmp, tagTmp, c.VariantIndex)
	fmt.Fprintf(g.b, "\tIF\t%%%s\n", condTmp)

	for bi, token := range c.BindingTokens {
		fieldName := c.Variant + "_" + c.Bindings[bi]
		goType := g.prog.resolveGoType(c.BindingTypes[bi])
		fmt.Fprintf(g.b, "\tVAR\t%s\t^%s\n", token, goType)
		fmt.Fprintf(g.b, "\tFGET\t%s\t%s\t>%s\n", token, subjectVal, fieldName)
	}
	var bodyErr error
	if dest != nil {
		bodyErr = g.genValueBlock(c.Body.Exprs, *dest)
	} else {
		bodyErr = g.genStmtBlock(c.Body.Exprs)
	}
	if bodyErr != nil {
		return bodyErr
	}

	if i+1 < len(cases) || def != nil {
		g.b.WriteString("\tELSE\n")
		if err := g.genSwitchChain(cases, i+1, subjectVal, def, dest); err != nil {
			return err
		}
	}
	g.b.WriteString("\tENDIF\n")
	return nil
}
