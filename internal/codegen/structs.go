// structs.go compiles amifl-spec.md section 2.2's `struct` declarations and
// `Tuple2`~`Tuple8` literals (step 6), plus the `.field`/`.N` postfix
// access shared by both (ast.FieldExpr) — see codegen.go's package doc for
// the surrounding step-by-step scope.
package codegen

import (
	"fmt"
	"strings"

	"github.com/amisonnet8/amifl/internal/ast"
)

// isTupleType/tupleTypeParts are codegen's own copies of sema's identical
// helpers (types.go's makeTupleType/tupleTypeParts) — ast is codegen's and
// sema's only shared vocabulary (CLAUDE.md's リポジトリ構成), so a string
// convention sema invents for its own bookkeeping (here, "Tuple(T1,T2,...)")
// has to be independently understood on the codegen side too, exactly like
// unitType above. Kept minimal: codegen only ever needs to split it back
// into element types to mint a Tuple's own synthesized STTYPE once.
func isTupleType(t string) bool {
	return strings.HasPrefix(t, "Tuple(")
}

func tupleTypeParts(t string) []string {
	inner := strings.TrimSuffix(strings.TrimPrefix(t, "Tuple("), ")")
	if inner == "" {
		return nil
	}
	return strings.Split(inner, ",")
}

// resolveGoType returns the Go/AMIVM type name amifl type t compiles to: a
// scalar's fixed name (goTypeNames), a tuple/list/array/set/map's
// deduplicated synthesized type name (tupleGoTypeName/listGoTypeName/
// arrayGoTypeName/setGoTypeName/mapGoTypeName (maps.go, step 10), each
// minted on first use across the whole program — unlike a closure's
// FNTYPE, which mints fresh every time, these benefit from sharing one Go
// type per shape so a type can flow through a function signature, a
// struct field, or another collection's own element type meaningfully),
// or a struct's own declared name verbatim (its STTYPE's Go type is
// always exactly the struct's AmiFL name — already a valid Go identifier,
// since every AmiFL identifier is one, so no mangling is needed the way a
// tuple/array's positional shape needs one).
func (p *program) resolveGoType(t string) string {
	if isTupleType(t) {
		return p.tupleGoTypeName(t)
	}
	if isListType(t) {
		return p.listGoTypeName(t)
	}
	if isArrayType(t) {
		return p.arrayGoTypeName(t)
	}
	if isSetType(t) {
		return p.setGoTypeName(t)
	}
	if isMapType(t) {
		return p.mapGoTypeName(t)
	}
	if goType, ok := goTypeNames[t]; ok {
		return goType
	}
	return t
}

// tupleGoTypeName mints (or reuses) the synthesized Go/AMIVM struct type
// for one tuple shape, keyed by its full canonical string so two tuple
// literals of the same element types always share one STTYPE. Fields are
// named "F0", "F1", ... (amivm_spec.md's `>` field-name prefix takes a Go
// identifier, and Go struct fields can't be named with a bare digit —
// ast.FieldExpr's doc comment) rather than encoding the element types into
// the name (unlike closures' FNTYPE, which doesn't need a stable, lookup-
// friendly name at all — this one does, since resolveFieldExpr already
// settled on "F"+index at sema time and codegen must reproduce exactly the
// same field names here for FGET/FSET to agree).
func (p *program) tupleGoTypeName(canonical string) string {
	if name, ok := p.tupleTypes[canonical]; ok {
		return name
	}
	elems := tupleTypeParts(canonical)
	p.tupleSeq++
	name := fmt.Sprintf("AmiflTuple%d", p.tupleSeq)
	if p.tupleTypes == nil {
		p.tupleTypes = map[string]string{}
	}
	p.tupleTypes[canonical] = name

	p.typeHeader.WriteString("STTYPE\t^" + name + "\n")
	for i, e := range elems {
		goType := p.resolveGoType(e)
		fmt.Fprintf(&p.typeHeader, "\tFIELD\t>F%d\t^%s\n", i, goType)
	}
	p.typeHeader.WriteString("ENDSTTYPE\n")
	return name
}

// genStructDecl emits one user `struct` declaration's STTYPE block
// directly into prog.typeHeader — called for every ast.StructDecl before
// any function body is generated (Generate), so a struct referenced by a
// function parameter/return type or another struct's own field is always
// already declared in the emitted IR by the time it's used (Go's package-
// level type declarations don't actually require this, but nothing
// guarantees amivm's own IR parsing doesn't — see Generate's existing
// comment on the identical concern for FNTYPE).
func genStructDecl(prog *program, d *ast.StructDecl) {
	prog.typeHeader.WriteString("STTYPE\t^" + d.Name + "\n")
	for _, f := range d.Fields {
		goType := prog.resolveGoType(f.ResolvedType)
		fmt.Fprintf(&prog.typeHeader, "\tFIELD\t>%s\t^%s\n", f.Name, goType)
	}
	prog.typeHeader.WriteString("ENDSTTYPE\n")
}

// genTupleLitValue emits `(v1, v2, ...)`: a fresh temp of the tuple's own
// (deduplicated) Go struct type, FSET field by field in position order.
func (g *gen) genTupleLitValue(v *ast.TupleLit) (string, error) {
	goType := g.prog.resolveGoType(v.ResolvedType)
	tmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", tmp, goType)
	for i, elem := range v.Elems {
		val, err := g.genValue(elem)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(g.b, "\tFSET\t%%%s\t>F%d\t%s\n", tmp, i, val)
	}
	return "%" + tmp, nil
}

// genStructLitValue emits `TypeName{field: v, ...}`: a fresh temp of the
// struct's own (unmangled) Go type, FSET field by field in the order the
// literal happened to list them (sema already checked every field is
// present exactly once — order doesn't matter to Go's own struct
// assignment semantics).
func (g *gen) genStructLitValue(v *ast.StructLit) (string, error) {
	tmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", tmp, v.ResolvedType)
	for _, f := range v.Fields {
		val, err := g.genValue(f.Value)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(g.b, "\tFSET\t%%%s\t>%s\t%s\n", tmp, f.Name, val)
	}
	return "%" + tmp, nil
}

// genFieldValue emits `target.field` (a tuple index, a struct field, or —
// step 8 — enum variant construction, amifl-spec.md sections 3.2/2.2) as
// FGET into a fresh temp of the field's own type. Enum construction is
// dispatched separately (genEnumVariantValue, enum.go) since it doesn't
// read v.Target at all (Target names a *type* there, not a value — see
// ast.FieldExpr's doc comment). Every other case generates v.Target
// through the normal genValue path, which always reduces a compound
// sub-expression to a single flat variable/temp token first — exactly the
// "bare identifier, no multi-level path" shape FGET's `variable` operand
// requires (CLAUDE.md's "過去に踏まれた地雷" #8), satisfied here
// automatically with no special-casing needed.
func (g *gen) genFieldValue(v *ast.FieldExpr) (string, error) {
	if v.IsEnumVariant {
		return g.genEnumVariantValue(v)
	}
	targetVal, err := g.genValue(v.Target)
	if err != nil {
		return "", err
	}
	goType := g.prog.resolveGoType(v.ResolvedType)
	tmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", tmp, goType)
	fmt.Fprintf(g.b, "\tFGET\t%%%s\t%s\t>%s\n", tmp, targetVal, v.AmivmField)
	return "%" + tmp, nil
}
