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
// unitType above. tupleTypeParts uses splitTopLevelCommas, not a plain
// strings.Split — a tuple element may itself be a compound type (List/
// Map/Set/Array/struct) containing a "," of its own; see sema/types.go's
// identical fix for the full explanation.
func isTupleType(t string) bool {
	return strings.HasPrefix(t, "Tuple(")
}

func tupleTypeParts(t string) []string {
	inner := strings.TrimSuffix(strings.TrimPrefix(t, "Tuple("), ")")
	if inner == "" {
		return nil
	}
	return splitTopLevelCommas(inner)
}

// splitTopLevelCommas is codegen's own copy of sema's identical helper
// (types.go) — splits s at every comma sitting at paren-nesting depth 0,
// so a nested compound type's own commas (inside its own parens) never
// get mistaken for a separator between sibling type strings.
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

// resolveGoType returns the Go/AMIVM type name amifl type t compiles to: a
// scalar's fixed name (goTypeNames), a tuple/func/list/array/set/map's
// deduplicated synthesized type name (tupleGoTypeName/funcGoTypeName
// (closure.go, ex3)/listGoTypeName/arrayGoTypeName/setGoTypeName/
// mapGoTypeName (maps.go, step 10), each minted on first use across the
// whole program and reused for every later reference to the same shape —
// load-bearing for Func specifically, not just an optimization the way it
// is for the others: Go requires two *named* function types to be
// identical, not just structurally alike, for one to be assignable to the
// other, so a closure literal, a passed-by-name top-level `fn`, and a
// Func-typed parameter/return/let-annotation of the same shape all have to
// resolve to the exact same Go type or they simply won't compile — see
// funcGoTypeName's own doc comment), or a struct's own declared name
// verbatim (its STTYPE's Go type is always exactly the struct's AmiFL
// name — already a valid Go identifier, since every AmiFL identifier is
// one, so no mangling is needed the way a tuple/array's positional shape
// needs one).
func (p *program) resolveGoType(t string) string {
	if isTupleType(t) {
		return p.tupleGoTypeName(t)
	}
	if isFuncType(t) {
		return p.funcGoTypeName(t)
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
	if isChanType(t) {
		return p.chanGoTypeName(t)
	}
	if isStreamType(t) {
		return p.streamGoTypeName(t)
	}
	if t == "Range" {
		return p.rangeGoTypeName()
	}
	if goName, ok := p.externTypes[t]; ok {
		return goName
	}
	if goType, ok := goTypeNames[t]; ok {
		return goType
	}
	// Only a user struct/enum's own bare AmiFL name ever reaches this
	// fallback (every other shape is handled by one of the branches above)
	// — step 14's pkgPrefix (program's own doc comment) is what keeps two
	// different packages' same-named structs/enums from colliding as Go
	// identifiers once GenerateProgram compiles them into one combined
	// output; "" for the root package leaves this exactly as it always was.
	return p.pkgPrefix + t
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
	// Resolved before this STTYPE's own header line is written — see
	// genStructDecl's identical fix/doc comment for why interleaving would
	// be wrong (a nested SLTYPE/etc. minted mid-block).
	goTypes := make([]string, len(elems))
	for i, e := range elems {
		goTypes[i] = p.resolveGoType(e)
	}
	p.tupleSeq++
	name := fmt.Sprintf("AmiflTuple%d", p.tupleSeq)
	if p.tupleTypes == nil {
		p.tupleTypes = map[string]string{}
	}
	p.tupleTypes[canonical] = name

	fmt.Fprintf(&p.typeHeader, "STTYPE\t^%s\n", name)
	for i, goType := range goTypes {
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
// rangeGoTypeName mints (once) the single compiler-synthesized STTYPE
// behind every Range value (amifl-spec.md section 3.1/7.3, ex2) —
// `{From, To int64}`, always exactly this one shape (program.rangeGoType's
// doc comment explains why no canonical-string-keyed map is needed the
// way tuple/list/etc need). Field names are `From`/`To`, not `F0`/`F1` —
// unlike Tuple's positional fields (Step 6's ".0"/".1" sugar demands
// Go-identifier-safe names), Range has no AmiFL-visible field-access
// syntax at all (ex2's scope cut: not even `.From`/`.To`), so the names
// exist purely for codegen's own genRangeValue/genForRangeStmt to read
// and write by.
func (p *program) rangeGoTypeName() string {
	if p.rangeGoType != "" {
		return p.rangeGoType
	}
	p.rangeGoType = "AmiflRange"
	p.typeHeader.WriteString("STTYPE\t^AmiflRange\n")
	p.typeHeader.WriteString("\tFIELD\t>From\t^int64\n")
	p.typeHeader.WriteString("\tFIELD\t>To\t^int64\n")
	p.typeHeader.WriteString("ENDSTTYPE\n")
	return p.rangeGoType
}

// genRangeValue emits `a..b` / `a..=b` (ex2, ast.RangeExpr): a fresh
// AmiflRange temp, FSET field by field — To is bumped by one first when
// Inclusive is set, so the runtime representation is always a half-open
// [From,To) pair (ast.RangeExpr's doc comment: Inclusive never survives
// past this one call, every consumer downstream — genForRangeStmt,
// genForRangeYieldValue — only ever sees the normalized form).
func (g *gen) genRangeValue(v *ast.RangeExpr) (string, error) {
	fromVal, err := g.genValue(v.From)
	if err != nil {
		return "", err
	}
	toVal, err := g.genValue(v.To)
	if err != nil {
		return "", err
	}
	if v.Inclusive {
		bumpedTmp := g.newTemp()
		fmt.Fprintf(g.b, "\tVAR\t%%%s\t^int64\n", bumpedTmp)
		fmt.Fprintf(g.b, "\tADD\t%%%s\t%s\t1\n", bumpedTmp, toVal)
		toVal = "%" + bumpedTmp
	}
	goType := g.prog.resolveGoType("Range")
	tmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", tmp, goType)
	fmt.Fprintf(g.b, "\tFSET\t%%%s\t>From\t%s\n", tmp, fromVal)
	fmt.Fprintf(g.b, "\tFSET\t%%%s\t>To\t%s\n", tmp, toVal)
	return "%" + tmp, nil
}

func genStructDecl(prog *program, d *ast.StructDecl) {
	// Every field's Go type is resolved *before* this STTYPE's own header
	// line is written, not interleaved field-by-field — resolveGoType can
	// itself mint and emit a brand-new nested type declaration (a List/
	// Array/Set/Map/Tuple/Chan/Stream field's first-ever use anywhere in
	// the program) directly into this same prog.typeHeader builder, and
	// amivm's IR parser rejects anything but FIELD lines appearing between
	// an STTYPE and its ENDSTTYPE — interleaving would splice that nested
	// declaration into the middle of *this* still-open block. (Found via
	// step 13's extern.aml actually triggering it for a first-use
	// Tuple2[Bytes,Error] — see tupleGoTypeName's identical fix and
	// CLAUDE.md's step 13 notes.)
	goTypes := make([]string, len(d.Fields))
	for i, f := range d.Fields {
		goTypes[i] = prog.resolveGoType(f.ResolvedType)
	}
	fmt.Fprintf(&prog.typeHeader, "STTYPE\t^%s%s\n", prog.pkgPrefix, d.Name)
	for i, f := range d.Fields {
		fmt.Fprintf(&prog.typeHeader, "\tFIELD\t>%s\t^%s\n", f.Name, goTypes[i])
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
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", tmp, g.prog.resolveGoType(v.ResolvedType))
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
	// Step 14's two qualified-reference cases (module.go's own doc comment)
	// — a const reference inlines exactly like ast.IdentExpr.ConstValue
	// already does (genValue's own *ast.IdentExpr case), and a qualified
	// call gets its own dedicated path since its callee token is already
	// fully resolved (no v.Target to FGET from at all — Target only ever
	// names the import alias, a compile-time-only reference, never a
	// runtime value).
	if v.QualifiedConstValue != nil {
		return g.genValue(v.QualifiedConstValue)
	}
	if v.IsQualifiedCall {
		return g.genQualifiedCallValue(v)
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
