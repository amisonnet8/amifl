// maps.go compiles amifl-spec.md section 2.2's Set[T]/Map[K,V], their
// shared bare-brace literal (ast.SetOrMapLit) — step 10. See codegen.go's
// package doc for the surrounding step-by-step scope, and collections.go's
// prepareForIteration for how `for` iterates a Set/Map (MPKEYS-based,
// unlike List/Array's direct AGET).
package codegen

import (
	"fmt"
	"strings"

	"github.com/amisonnet8/amifl/internal/ast"
)

// isSetType/setElemType and isMapType/mapKeyValueTypes are codegen's own
// copies of sema's identical helpers (types.go's makeSetType/makeMapType
// and friends) — ast is codegen's and sema's only shared vocabulary
// (CLAUDE.md's リポジトリ構成), so these string conventions have to be
// independently understood here too, exactly like isTupleType/isListType
// above. mapKeyValueTypes needs the identical depth-aware split sema's own
// copy does (a Map's key/value may themselves be a nested Tuple/List/
// Array/Set/Map, each containing "(" ")" "," or ";" — see sema's
// types.go for why a naive comma-split doesn't work here).
func isSetType(t string) bool {
	return strings.HasPrefix(t, "Set(") && strings.HasSuffix(t, ")")
}

func setElemType(t string) string {
	return strings.TrimSuffix(strings.TrimPrefix(t, "Set("), ")")
}

func isMapType(t string) bool {
	return strings.HasPrefix(t, "Map(") && strings.HasSuffix(t, ")")
}

func mapKeyValueTypes(t string) (key, val string) {
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
				return inner[:i], inner[i+1:]
			}
		}
	}
	return inner, ""
}

// makeListType is codegen's own copy of sema's identical helper — needed
// here (collections.go's prepareForIteration) to name the plain List a
// Set/Map's keys get collected into via MPKEYS before a `for` loop can
// iterate them (see prepareForIteration's doc comment).
func makeListType(elem string) string {
	return "List(" + elem + ")"
}

// setGoTypeName mints (or reuses) the synthesized Go/AMIVM map type
// backing one Set[T] shape, keyed by its full canonical string. Set[T]
// compiles to Go's native map[T]bool (CLAUDE.md's "確定した設計判断" for
// step 10: Set membership is exactly what a bool-valued map already
// models, and repeated MSET already gives Set literal construction
// genuine dedup for free — no separate Go type or AMIVM instruction
// category is needed). This mints its own MPTYPE separately from
// mapGoTypeName below, keyed under Set[T]'s own canonical string rather
// than Map[T,Bool]'s — even though the two would produce a structurally
// identical Go type (map[T]bool), sharing one MPTYPE across them would mean
// resolveGoType's caller-visible contract ("one Go type per distinct
// canonical AmiFL type string", the same convention tupleGoTypeName/
// listGoTypeName/arrayGoTypeName already follow) stops being uniform for
// this one case — a harmless amount of duplicate MPTYPE output (at most
// one extra line) is a small price for not special-casing that.
func (p *program) setGoTypeName(canonical string) string {
	if name, ok := p.setTypes[canonical]; ok {
		return name
	}
	elemGoType := p.resolveGoType(setElemType(canonical))
	p.setSeq++
	name := fmt.Sprintf("AmiflSet%d", p.setSeq)
	if p.setTypes == nil {
		p.setTypes = map[string]string{}
	}
	p.setTypes[canonical] = name

	fmt.Fprintf(&p.typeHeader, "MPTYPE\t^%s\t^%s\t^bool\n", name, elemGoType)
	return name
}

// mapGoTypeName mints (or reuses) the synthesized Go/AMIVM map type
// backing one Map[K,V] shape, keyed by its full canonical string —
// mirrors setGoTypeName above, minus the hardcoded bool value type.
func (p *program) mapGoTypeName(canonical string) string {
	if name, ok := p.mapTypes[canonical]; ok {
		return name
	}
	key, val := mapKeyValueTypes(canonical)
	keyGoType := p.resolveGoType(key)
	valGoType := p.resolveGoType(val)
	p.mapSeq++
	name := fmt.Sprintf("AmiflMap%d", p.mapSeq)
	if p.mapTypes == nil {
		p.mapTypes = map[string]string{}
	}
	p.mapTypes[canonical] = name

	fmt.Fprintf(&p.typeHeader, "MPTYPE\t^%s\t^%s\t^%s\n", name, keyGoType, valGoType)
	return name
}

// genSetOrMapLitValue emits `{v1, v2, ...}` (Set[T]) or `{k1: v1, ...}`
// (Map[K,V]) (amifl-spec.md sections 2.2/3.1) — a fresh temp of the
// literal's own (deduplicated) Go map type, MPMAKE'd empty and then MSET
// once per element/entry. The Set form's repeated MSET(elem, true) is what
// gives a Set literal genuine dedup (amifl-spec.md's "重複無し") for free —
// a duplicate element just overwrites the same key with the same value —
// exactly like a duplicate key in the Map form naturally overwrites with
// whichever value came last, standard map-literal semantics that need no
// special-casing here.
func (g *gen) genSetOrMapLitValue(v *ast.SetOrMapLit) (string, error) {
	goType := g.prog.resolveGoType(v.ResolvedType)
	tmp := g.newTemp()
	fmt.Fprintf(g.b, "\tVAR\t%%%s\t^%s\n", tmp, goType)
	fmt.Fprintf(g.b, "\tMPMAKE\t%%%s\t^%s\n", tmp, goType)

	if isSetType(v.ResolvedType) {
		for _, el := range v.Elems {
			val, err := g.genValue(el)
			if err != nil {
				return "", err
			}
			fmt.Fprintf(g.b, "\tMSET\t%%%s\t%s\ttrue\n", tmp, val)
		}
		return "%" + tmp, nil
	}
	for _, entry := range v.Entries {
		keyVal, err := g.genValue(entry.Key)
		if err != nil {
			return "", err
		}
		valVal, err := g.genValue(entry.Value)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(g.b, "\tMSET\t%%%s\t%s\t%s\n", tmp, keyVal, valVal)
	}
	return "%" + tmp, nil
}
