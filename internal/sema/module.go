// module.go implements step 14's cross-package reference support
// (amifl-spec.md section 12): the Exports a package's own CheckPackage run
// hands back for another package to import, and resolveFieldExpr's own
// qualified-reference branch (expr.go) that resolves `alias.Name` against
// an already-imported package's Exports. See sema.go's CheckPackage for
// the surrounding pass structure this plugs into.
package sema

import (
	"fmt"
	"strconv"
	"unicode"
	"unicode/utf8"

	"github.com/amisonnet8/amifl/internal/ast"
)

// ExportedFunc is one exported (capitalized-name) top-level `fn` or extern
// bind, as another package may call it via `alias.Name(args...)` (amifl-
// spec.md section 12.2).
type ExportedFunc struct {
	Params []string
	Ret    string
	// Token is the full AMIVM callname another package's own codegen must
	// use to call this declaration — already fully resolved: "!" plus this
	// package's own rename prefix plus the declared name for a plain `fn`
	// (ast.ImportDecl's doc comment on that prefix), or an extern bind's
	// own already-package-qualified Go callname ("?alias.GoName")
	// verbatim, since that one is never subject to AmiFL-level renaming at
	// all (it names a Go declaration directly, independent of which AmiFL
	// package the bind happens to be declared in). Either way, an importer
	// never needs to know or recompute which of the two shapes this is —
	// codegen's genQualifiedCallValue/genQualifiedCallStmt (module.go) just
	// use it verbatim as CALL's callname operand.
	Token string
}

// ExportedConst is one exported top-level `const`, as another package may
// reference it via `alias.NAME` — Value is this package's own already
// fully sema-resolved initializer expression, ready to inline at the
// reference site exactly the way a same-package reference already does
// (ast.IdentExpr.ConstValue's identical convention — amifl-spec.md section
// 4, "参照箇所へインライン展開される"; a const has no runtime storage
// either way, same-package or cross-package).
type ExportedConst struct {
	Typ   string
	Value ast.Expr
}

// ExportedStruct is one exported top-level `struct`, as another package may
// reference it via `alias.Name{...}` (construction) or `alias.Name` (a type
// annotation, ast.QualifiedType) — ex5. Fields mirrors structInfo.Fields
// exactly (scope.go) — CheckPackage's registerImportedTypes copies it
// straight into a *structInfo an importer's own c.structs gets keyed under
// the qualified canonical string (types.go's makeQualifiedType), so every
// existing struct-shaped consumer (resolveFieldExpr's `.field` access,
// resolveStructLit's completeness/duplicate checks, ...) handles a
// cross-package struct exactly like a local one with no changes of its
// own. GoName is this struct's *already fully package-prefixed* Go/AMIVM
// type name ("<this package's own canonical prefix>Name" — buildExports'
// own prefix parameter, identical to ExportedFunc.Token's "!"+prefix+name
// convention) — never just the bare declared name, since a same-named
// struct in two different packages would otherwise be indistinguishable to
// an importer (makeQualifiedType's own doc comment explains why the
// resulting canonical string wraps this rather than using it bare).
type ExportedStruct struct {
	Fields []fieldInfo
	GoName string
}

// ExportedEnum is ExportedStruct's exact enum counterpart — Variants
// mirrors enumInfo.Variants (scope.go), GoName the same already-prefixed
// Go/AMIVM STTYPE name genEnumDecl emits (enum.go), for the identical
// reason ExportedStruct.GoName needs one.
type ExportedEnum struct {
	Variants []variantInfo
	GoName   string
}

// Exports is one package's public surface — amifl-spec.md section 12.2's
// "他パッケージから参照できるのは、名前が大文字で始まるトップレベル宣言
// だけ...値の種類（fn／struct／enum／const）は問わない". Step 14 originally
// modeled only Funcs/Consts here (struct/enum export needed new
// construction syntax — `alias.Point{...}`/`alias.Status.Variant(...)` —
// that hadn't been designed yet); ex5 adds Structs/Enums once that syntax
// exists (parser's parsePostfixExpr/parseTypeExpr), completing the "値の種類
// は問わない" promise this section always made.
type Exports struct {
	Funcs   map[string]ExportedFunc
	Consts  map[string]ExportedConst
	Structs map[string]ExportedStruct
	Enums   map[string]ExportedEnum
}

// isExported reports whether name is visible from another package —
// amifl-spec.md section 12.2's "名前が大文字で始まる" rule, deliberately
// matching Go's own visibility convention (the spec's own stated reason:
// "実行基盤がGoである以上、生成物がそのまま正規のGoパッケージ群になるため、
// Goのツールチェーンとの親和性を優先した").
func isExported(name string) bool {
	r, _ := utf8.DecodeRuneInString(name)
	return unicode.IsUpper(r)
}

// buildExports gathers c's own exported funcs/consts/structs/enums once
// CheckPackage has finished checking every declaration — called exactly
// once, at the very end of CheckPackage, so every signature/const value/
// field type it reads is already fully resolved. Every canonical type
// string handed onto an Exports field passes through exportTypeString
// first — see that function's own doc comment for why a bare "Point"
// (correct from *this* package's own perspective) would otherwise reach an
// importer as an unrecognizable string.
func (c *checker) buildExports(prefix string) Exports {
	ex := Exports{
		Funcs:   map[string]ExportedFunc{},
		Consts:  map[string]ExportedConst{},
		Structs: map[string]ExportedStruct{},
		Enums:   map[string]ExportedEnum{},
	}
	for name, sig := range c.funcs {
		if !isExported(name) {
			continue
		}
		// A method-style extern bind (sig.externMethod != "") has no fixed
		// callname token at all — METHVAL dispatch needs the receiver
		// *value* at each call site (CallExpr.ExternMethod's own doc
		// comment), which a qualified reference has no way to supply
		// (Target only ever names the import alias, never a receiver
		// expression). Rather than exporting a token that would silently
		// misresolve, such a bind is simply omitted from Exports — a
		// cross-package reference to it reports "no exported name", the
		// same clear error an unexported name would get.
		if sig.externMethod != "" {
			continue
		}
		token := sig.externCallee
		if token == "" {
			token = "!" + prefix + name
		}
		params := make([]string, len(sig.params))
		for i, p := range sig.params {
			params[i] = c.exportTypeString(p, prefix)
		}
		ex.Funcs[name] = ExportedFunc{Params: params, Ret: c.exportTypeString(sig.ret, prefix), Token: token}
	}
	for name, b := range c.globals {
		if !b.isConst || !isExported(name) {
			continue
		}
		ex.Consts[name] = ExportedConst{Typ: c.exportTypeString(b.typ, prefix), Value: b.value}
	}
	// c.structs/c.enums hold both this package's own locally-declared names
	// (plain identifiers) *and*, since ex5's registerImportedTypes runs
	// before any of c's own declarations are registered, every transitively
	// imported struct/enum this package itself imports — keyed by the
	// "Qualified(...)" envelope makeQualifiedType produces, never a plain
	// identifier. isQualifiedType filters those back out here: exporting
	// them onward under that garbled compound key would silently leak a
	// transitive dependency's types under a name no importer could ever
	// spell, rather than the clean re-export amifl-spec.md doesn't actually
	// ask for (12.2 only promises a package's *own* declarations).
	for name, info := range c.structs {
		if isQualifiedType(name) || !isExported(name) {
			continue
		}
		ex.Structs[name] = ExportedStruct{Fields: c.exportFieldInfos(info.Fields, prefix), GoName: prefix + name}
	}
	for name, info := range c.enums {
		if isQualifiedType(name) || !isExported(name) {
			continue
		}
		variants := make([]variantInfo, len(info.Variants))
		for i, va := range info.Variants {
			variants[i] = variantInfo{Name: va.Name, Fields: c.exportFieldInfos(va.Fields, prefix)}
		}
		ex.Enums[name] = ExportedEnum{Variants: variants, GoName: prefix + name}
	}
	return ex
}

// exportFieldInfos rewrites every field's Typ through exportTypeString —
// shared by buildExports' struct-field and enum-variant-field loops (both
// are exactly a []fieldInfo).
func (c *checker) exportFieldInfos(fields []fieldInfo, prefix string) []fieldInfo {
	out := make([]fieldInfo, len(fields))
	for i, f := range fields {
		out[i] = fieldInfo{Name: f.Name, Typ: c.exportTypeString(f.Typ, prefix)}
	}
	return out
}

// exportTypeString rewrites t — a canonical type string as resolved from
// *this* package's own perspective — into the form an importing package's
// checker needs to recognize any of this package's own struct/enum types t
// might mention, anywhere inside it. A bare name matching one of c's own
// locally-declared structs/enums (never one already registered from one of
// c's *own* imports — isQualifiedType's guard, mirroring buildExports' own
// struct/enum loops) becomes its qualified canonical string
// (makeQualifiedType(prefix+name)); every compound shape (Tuple/List/Array/
// Set/Map/Chan/Stream/Func) recurses into each of its own component types,
// since a local struct/enum can appear nested arbitrarily deep inside one
// (a struct field typed List[Point], a function returning Tuple2[Point,
// Error], ...); anything else (a scalar, Error, Any, File, Range, an
// extern type, or a type already wrapped in "Qualified(...)" — inherited
// transitively from one of c's *own* imports, and therefore already
// correct for any further importer too, GoName being globally stable
// regardless of who references it) passes through unchanged.
//
// Without this, buildExports would hand an importer the literal bare
// string "Point" for e.g. a function parameter — indistinguishable from
// anything the importer's own package might independently declare under
// that same name, and not a key its own c.structs ever contains (only
// "Qualified(<this package's prefix>Point)" is ever registered there,
// registerImportedTypes' own convention). Found the hard way: ex5's own
// first worked example (a function in the struct's own declaring package,
// called with a value built via the qualified struct literal) rejected a
// perfectly well-typed value with "expected Point, got
// Qualified(geo_Point)" before this existed.
func (c *checker) exportTypeString(t, prefix string) string {
	if isQualifiedType(t) {
		return t
	}
	if _, ok := c.structs[t]; ok {
		return makeQualifiedType(prefix + t)
	}
	if _, ok := c.enums[t]; ok {
		return makeQualifiedType(prefix + t)
	}
	if elems, ok := tupleTypeParts(t); ok {
		rewritten := make([]string, len(elems))
		for i, e := range elems {
			rewritten[i] = c.exportTypeString(e, prefix)
		}
		return makeTupleType(rewritten)
	}
	if e, ok := listElemType(t); ok {
		return makeListType(c.exportTypeString(e, prefix))
	}
	if e, n, ok := arrayParts(t); ok {
		return makeArrayType(c.exportTypeString(e, prefix), strconv.FormatUint(n, 10))
	}
	if e, ok := setElemType(t); ok {
		return makeSetType(c.exportTypeString(e, prefix))
	}
	if k, v, ok := mapKeyValueTypes(t); ok {
		return makeMapType(c.exportTypeString(k, prefix), c.exportTypeString(v, prefix))
	}
	if e, ok := chanElemType(t); ok {
		return makeChanType(c.exportTypeString(e, prefix))
	}
	if e, ok := streamElemType(t); ok {
		return makeStreamType(c.exportTypeString(e, prefix))
	}
	if params, ret, ok := funcTypeParts(t); ok {
		rewrittenParams := make([]string, len(params))
		for i, p := range params {
			rewrittenParams[i] = c.exportTypeString(p, prefix)
		}
		return makeFuncType(rewrittenParams, c.exportTypeString(ret, prefix))
	}
	return t
}

// registerImportedTypes (ex5) copies every imported package's own exported
// structs/enums into c.structs/c.enums, keyed by their qualified canonical
// string (types.go's makeQualifiedType — never the bare declared name,
// which would collide with a same-named struct/enum in a *different*
// imported package, or in c itself). This is what lets most existing
// struct/enum-shaped sema machinery (resolveFieldExpr's `.field` access,
// resolveStructLit's field-completeness check, checkExpr's general type-
// agreement checking, ...) handle a cross-package type exactly like a local
// one, with no changes of their own — resolveSwitchExpr's subject lookup is
// the one deliberate exception, explicitly rejecting a qualified subject
// rather than silently accepting a type no case pattern could ever
// actually name (that function's own isQualifiedType check). Only the few
// places that *produce* a qualified canonical string in the first place
// (resolveTypeExpr's *ast.QualifiedType case, resolveStructLit's Qualifier
// branch, resolveFieldExpr's alias.EnumType.Variant branch) need to know
// this envelope exists at all. Run once, up front (sema.go's CheckPackage,
// before Pass 0), so c.structs/c.enums are fully populated before any of
// c's own declarations — whose field/param/return types may themselves
// name one of these — are even registered.
func (c *checker) registerImportedTypes() {
	for _, pkg := range c.imports {
		for _, es := range pkg.Structs {
			canon := makeQualifiedType(es.GoName)
			c.structs[canon] = &structInfo{Name: canon, Fields: es.Fields}
		}
		for _, ee := range pkg.Enums {
			canon := makeQualifiedType(ee.GoName)
			c.enums[canon] = &enumInfo{Name: canon, Variants: ee.Variants}
		}
	}
}

// resolveQualifiedReference type-checks step 14's `alias.Name` (amifl-
// spec.md section 12.2) once resolveFieldExpr has already determined
// ident.Name is a known import alias — a function call
// (`alias.Name(args...)`, v.Args != nil) or a const reference
// (`alias.NAME`, v.Args == nil), told apart exactly the way
// resolveFieldExpr's other branches already are, by presence of a trailing
// `(...)`. Every function-call argument must be positional (Name == "") —
// principle 7, "名前付き引数無し" — parseFieldCallArgs's shared grammar
// with enum variant construction (which *requires* every argument named)
// means a mixed or fully-named argument list here is a genuine user
// mistake, not an enum construction that happened to target the wrong
// name, so it's reported as its own clear error rather than falling
// through to a confusing "wrong argument count"/"no such field" message.
func (fc *funcChecker) resolveQualifiedReference(v *ast.FieldExpr, alias string, pkg Exports) (string, error) {
	if fn, ok := pkg.Funcs[v.Field]; ok {
		if v.Args == nil {
			return "", fmt.Errorf("line %d: %s.%s is a function; call it with (...)", v.Line, alias, v.Field)
		}
		if len(v.Args) != len(fn.Params) {
			return "", fmt.Errorf("line %d: %s.%s expects %d argument(s), got %d", v.Line, alias, v.Field, len(fn.Params), len(v.Args))
		}
		argTypes := make([]string, len(v.Args))
		for i := range v.Args {
			a := &v.Args[i]
			if a.Name != "" {
				return "", fmt.Errorf("line %d: qualified function calls take positional arguments only, got named argument %q", a.Line, a.Name)
			}
			t, err := fc.checkExpr(a.Value, fn.Params[i])
			if err != nil {
				return "", err
			}
			argTypes[i] = t
		}
		v.IsQualifiedCall = true
		v.QualifiedCallee = fn.Token
		v.QualifiedArgTypes = argTypes
		v.ResolvedType = fn.Ret
		return fn.Ret, nil
	}
	if c, ok := pkg.Consts[v.Field]; ok {
		if v.Args != nil {
			return "", fmt.Errorf("line %d: %s.%s is a const, not a function", v.Line, alias, v.Field)
		}
		v.QualifiedConstValue = c.Value
		v.ResolvedType = c.Typ
		return c.Typ, nil
	}
	// A struct/enum type name reaches this generic fallback only when it
	// isn't being used the way resolveFieldExpr's own dedicated branches
	// recognize (a bare `alias.Point` with nothing further, e.g. `let x =
	// mathutil.Point` with no `{...}` — not a valid value in this language
	// any more than a same-package bare struct/enum name is) — ex5 gives
	// this its own clearer message rather than the generic "no exported
	// name" a genuinely unknown Field still gets below.
	if _, ok := pkg.Structs[v.Field]; ok {
		return "", fmt.Errorf("line %d: %s.%s is a struct type, not a value; construct one with %s.%s{...} or name it in a type annotation", v.Line, alias, v.Field, alias, v.Field)
	}
	if _, ok := pkg.Enums[v.Field]; ok {
		return "", fmt.Errorf("line %d: %s.%s is an enum type, not a value; construct a variant with %s.%s.Variant(...) or name it in a type annotation", v.Line, alias, v.Field, alias, v.Field)
	}
	return "", fmt.Errorf("line %d: package %q has no exported name %q", v.Line, alias, v.Field)
}
