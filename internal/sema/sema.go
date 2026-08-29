package sema

import (
	"fmt"

	"github.com/amisonnet8/amifl/internal/ast"
)

// reservedMainName must match codegen's entryFunc constant — both are
// deliberately independent copies of the same string, not a shared
// symbol (ast is sema's and codegen's only shared vocabulary; see
// CLAUDE.md's リポジトリ構成). A user-declared `fn`/`const` named this
// would collide with the internal name codegen compiles the user's own
// `fn main` under (CLAUDE.md's "確定した設計判断" for step 1's main/
// amifl_main bridge) — Cascade's CLAUDE.md records the identical
// reservation for its own `cascade_main`.
const reservedMainName = "amifl_main"

// Check performs semantic validation on a single-file, single-package
// program — steps 1-13's entry point, kept exactly as every existing
// caller (and the whole sema_test.go suite) already uses it: a single file
// is just CheckPackage's one-file, root-package (prefix "", no imports)
// case.
func Check(f *ast.File) error {
	_, err := CheckPackage([]*ast.File{f}, "", nil)
	return err
}

// CheckPackage is Check generalized to step 14's multi-file package
// (amifl-spec.md section 12.1) and cross-package import (12.2) support:
// scalar type checking, let/const scope resolution (with const inlining),
// operators (step 3), if/elif/else/while/switch and their lexical scoping
// (step 4), top-level `fn` declarations (any number, any parameter list,
// callable in any order — forward references and mutual/self recursion all
// just work, since every signature is registered in one pass before any
// body is checked) and local closures with their own `Func` type (step 5),
// the expression-oriented "every non-final expression in a block must be
// Unit-typed" rule (amifl-spec.md principle 1), and every later step's own
// type-system growth (structs, enums, collections, capability resolution,
// extern binds, ...).
//
// files are every .aml file making up one package, sharing one flat
// namespace exactly like Check's own single file always did — no import
// needed between files in the same package (12.1's "同じディレクトリに
// 置かれた.amlファイル群は…1つの共有スコープにあるかのようにコンパイル
// される"). prefix is "" for the root package, whose own declarations are
// never mangled and whose `fn main` is validated as the program's entry
// point (12.3 — a non-root package's own `main`, if it declares one, is
// just an ordinary function, not validated as an entry point at all); any
// other package's own canonical rename prefix otherwise (ast.ImportDecl's
// doc comment on how that's chosen). imports maps each `import alias
// "..."` this package's own files declare to that alias's already-computed
// Exports — internal/modloader.Load processes the package DAG leaves-
// first, so every dependency's Exports are ready by the time its importer
// is checked here.
func CheckPackage(files []*ast.File, prefix string, imports map[string]Exports) (Exports, error) {
	c := &checker{
		globals:       map[string]*binding{},
		funcs:         map[string]funcSig{},
		structs:       map[string]*structInfo{},
		enums:         map[string]*enumInfo{},
		externTypes:   map[string]bool{},
		externAliases: map[string]string{},
		imports:       imports,
	}
	// ex5: every imported package's own exported structs/enums are
	// registered into c.structs/c.enums here, before anything else — see
	// registerImportedTypes' own doc comment. This has to run before Pass 0
	// below (not folded into it) since Pass 0's own registerStructName/
	// registerEnumName check c.structs/c.enums for a name collision, and a
	// same-package declaration should never collide with an imported
	// qualified entry (it structurally can't — see makeQualifiedType's doc
	// comment — but running this first keeps the invariant "c.structs/
	// c.enums are fully populated with every name resolveFieldExpr/
	// resolveTypeExpr might ever need to look up" true from the very start
	// of type-checking, matching how imports themselves are always fully
	// resolved before CheckPackage even begins, amifl-spec.md's own
	// dependency-order guarantee).
	c.registerImportedTypes()

	var consts []*ast.ConstDecl
	var funcs []*ast.FuncDecl
	var structs []*ast.StructDecl
	var enums []*ast.EnumDecl
	var externs []*ast.ExternDecl
	for _, f := range files {
		for _, decl := range f.Decls {
			switch d := decl.(type) {
			case *ast.ConstDecl:
				consts = append(consts, d)
			case *ast.FuncDecl:
				funcs = append(funcs, d)
			case *ast.StructDecl:
				structs = append(structs, d)
			case *ast.EnumDecl:
				enums = append(enums, d)
			case *ast.ExternDecl:
				externs = append(externs, d)
			case *ast.ImportDecl:
				// Resolved entirely by internal/modloader before
				// CheckPackage ever runs (path resolution, cycle
				// detection, the global alias-uniqueness rule) — nothing
				// left to check here beyond what fc.imports lookups
				// already validate at each qualified-reference use site
				// (resolveFieldExpr's resolveQualifiedReference).
			default:
				return Exports{}, fmt.Errorf("sema: unknown top-level declaration %T", decl)
			}
		}
	}

	// Pass 0: register every extern block's own alias plus its `type`
	// entries, then every struct's and enum's name (0a), and then structs'/
	// enums' fields/variants (0b), before anything else — a `fn`'s param/
	// return type, a `const`'s annotation/initializer, a struct field, or
	// an enum variant's own field can all name a struct, enum, or extern
	// type regardless of where in the file (or which of the three
	// declaration kinds) it's declared. registerStructName/registerEnumName
	// each check c.structs, c.enums, *and* c.externTypes for a name
	// collision (registerExternTypes running first here is what makes that
	// last check meaningful), so a struct/enum/extern-type sharing one name
	// is caught regardless of which of the three comes first in the file.
	for _, ext := range externs {
		if err := c.registerExternTypes(ext); err != nil {
			return Exports{}, err
		}
	}
	for _, st := range structs {
		if err := c.registerStructName(st); err != nil {
			return Exports{}, err
		}
	}
	for _, en := range enums {
		if err := c.registerEnumName(en); err != nil {
			return Exports{}, err
		}
	}
	for _, st := range structs {
		if err := c.registerStructFields(st); err != nil {
			return Exports{}, err
		}
	}
	for _, en := range enums {
		if err := c.registerEnumVariants(en); err != nil {
			return Exports{}, err
		}
	}

	// Consts are checked here, now that struct names/fields are known —
	// unchanged from step 2-5 otherwise (still in file order, still no
	// forward references between consts themselves), except a const's
	// initializer may now itself be a struct/tuple literal referencing a
	// struct type declared anywhere in the file.
	for _, d := range consts {
		if err := c.checkTopLevelConst(d); err != nil {
			return Exports{}, err
		}
	}

	// Pass 1: register every function's signature (and validate its own
	// parameter list) before checking any body, so a call can reference a
	// function declared later in the file, or itself, or another function
	// that in turn calls back into it. Every extern bind's own signature is
	// registered here too (into the same c.funcs table a `fn` uses — see
	// registerExternBind), so a bind can likewise be called before or after
	// its own declaration, and a `fn` can call into a bind exactly as it
	// would call another `fn`.
	for _, fn := range funcs {
		if err := c.registerFuncSig(fn); err != nil {
			return Exports{}, err
		}
	}
	for _, ext := range externs {
		for bi := range ext.Binds {
			if err := c.registerExternBind(ext.Alias, &ext.Binds[bi]); err != nil {
				return Exports{}, err
			}
		}
	}

	// Only the root package's own `main` is ever validated as the
	// program's entry point (amifl-spec.md section 12.3) — a non-root
	// package's own `main`, if it declares one, is checked as an ordinary
	// function like any other (Pass 2 below still type-checks its body
	// regardless), just never singled out or required.
	if prefix == "" {
		if _, err := findAndValidateMain(funcs); err != nil {
			return Exports{}, err
		}
	}

	// Pass 2: check every body, now that every signature (including a
	// forward or mutually-recursive reference) is already known.
	for _, fn := range funcs {
		if err := c.checkFunc(fn); err != nil {
			return Exports{}, err
		}
	}
	return c.buildExports(prefix), nil
}

// findAndValidateMain locates `fn main` among funcs (whose signatures
// registerFuncSig must have already resolved) and enforces amifl-spec.md
// section 14's entry-point shape: exactly one `fn main`, returning `Int`,
// taking either no parameters or a single `args: List[String]` parameter
// (codegen.go's GenerateProgram is what actually supplies argv — see
// amiflrt.Args — when the latter form is declared).
func findAndValidateMain(funcs []*ast.FuncDecl) (*ast.FuncDecl, error) {
	var main *ast.FuncDecl
	for _, fn := range funcs {
		if fn.Name != "main" {
			continue
		}
		if main != nil {
			return nil, fmt.Errorf("line %d: duplicate `fn main` (first declared at line %d)", fn.Line, main.Line)
		}
		main = fn
	}
	if main == nil {
		return nil, fmt.Errorf("missing entry point: no `fn main` declared (amifl-spec.md section 14)")
	}
	switch len(main.Params) {
	case 0:
		// fn main() -> Int — the argument-less form.
	case 1:
		if main.Params[0].ResolvedType != "List(String)" {
			return nil, fmt.Errorf("line %d: fn main's single parameter must be List[String], got %s", main.Line, main.Params[0].ResolvedType)
		}
	default:
		return nil, fmt.Errorf("line %d: fn main must take no parameters or a single args: List[String] parameter, got %d", main.Line, len(main.Params))
	}
	if main.ResolvedReturnType != "Int64" {
		return nil, fmt.Errorf("line %d: fn main must return Int, got %s", main.Line, main.ReturnType)
	}
	return main, nil
}

func (c *checker) checkTopLevelConst(d *ast.ConstDecl) error {
	if d.Name == reservedMainName {
		return fmt.Errorf("line %d: %q is a reserved name (used internally to compile `fn main`)", d.Line, d.Name)
	}
	if _, exists := c.globals[d.Name]; exists {
		return fmt.Errorf("line %d: %q is already declared", d.Line, d.Name)
	}
	fc := newFuncChecker(c)
	typ, lit, err := resolveConstDecl(fc, d)
	if err != nil {
		return err
	}
	d.ResolvedType = typ
	c.globals[d.Name] = &binding{isConst: true, typ: typ, value: lit}
	return nil
}

// registerStructName reserves d's name (Check's pass 0a) — split from
// registerStructFields (0b) so every struct name in the file is known
// before any struct's fields (which may reference another struct, in
// either declaration order) are resolved.
func (c *checker) registerStructName(d *ast.StructDecl) error {
	if d.Name == reservedMainName {
		return fmt.Errorf("line %d: %q is a reserved name (used internally to compile `fn main`)", d.Line, d.Name)
	}
	if _, exists := c.structs[d.Name]; exists {
		return fmt.Errorf("line %d: duplicate struct %q", d.Line, d.Name)
	}
	if _, exists := c.enums[d.Name]; exists {
		return fmt.Errorf("line %d: %q is already declared as an enum", d.Line, d.Name)
	}
	if _, exists := c.externTypes[d.Name]; exists {
		return fmt.Errorf("line %d: %q is already declared as an extern type", d.Line, d.Name)
	}
	c.structs[d.Name] = &structInfo{Name: d.Name}
	return nil
}

// registerEnumName reserves d's name (Check's pass 0a) — mirrors
// registerStructName exactly (see its own doc comment on the two-pass
// split and the cross-check with c.structs).
func (c *checker) registerEnumName(d *ast.EnumDecl) error {
	if d.Name == reservedMainName {
		return fmt.Errorf("line %d: %q is a reserved name (used internally to compile `fn main`)", d.Line, d.Name)
	}
	if _, exists := c.enums[d.Name]; exists {
		return fmt.Errorf("line %d: duplicate enum %q", d.Line, d.Name)
	}
	if _, exists := c.structs[d.Name]; exists {
		return fmt.Errorf("line %d: %q is already declared as a struct", d.Line, d.Name)
	}
	if _, exists := c.externTypes[d.Name]; exists {
		return fmt.Errorf("line %d: %q is already declared as an extern type", d.Line, d.Name)
	}
	c.enums[d.Name] = &enumInfo{Name: d.Name}
	return nil
}

// registerEnumVariants resolves d's variants and their field types (Check's
// pass 0b) — mirrors registerStructFields, plus its own two duplicate
// checks amifl-spec.md section 2.2 requires that structs don't (a struct
// has no variants to be distinct from each other): variant names must be
// unique within the enum, and field names unique within each variant
// (structs already had that latter check; here it's per-variant instead
// of per-declaration).
func (c *checker) registerEnumVariants(d *ast.EnumDecl) error {
	fc := newFuncChecker(c)
	seenVariant := map[string]bool{}
	var variants []variantInfo
	for vi := range d.Variants {
		v := &d.Variants[vi]
		if seenVariant[v.Name] {
			return fmt.Errorf("line %d: duplicate variant %q in enum %q", v.Line, v.Name, d.Name)
		}
		seenVariant[v.Name] = true

		seenField := map[string]bool{}
		var fields []fieldInfo
		for fi := range v.Fields {
			f := &v.Fields[fi]
			if seenField[f.Name] {
				return fmt.Errorf("line %d: duplicate field %q in variant %s.%s", f.Line, f.Name, d.Name, v.Name)
			}
			seenField[f.Name] = true
			ft, err := fc.resolveTypeExpr(f.Type)
			if err != nil {
				return err
			}
			f.ResolvedType = ft
			fields = append(fields, fieldInfo{Name: f.Name, Typ: ft})
		}
		variants = append(variants, variantInfo{Name: v.Name, Fields: fields})
	}
	c.enums[d.Name].Variants = variants
	return nil
}

// registerStructFields resolves d's field types (Check's pass 0b) — every
// struct name in the file, including d's own, is already registered by
// registerStructName by the time this runs, so a field naming any struct
// (declared earlier or later in the file) resolves fine. A throwaway
// funcChecker (mirroring checkTopLevelConst's identical need) is what lets
// a field's Array[T;N] size reference a top-level const — resolveTypeExpr
// is a funcChecker method because of that same possibility, even though a
// struct field itself never has runtime scope of its own.
func (c *checker) registerStructFields(d *ast.StructDecl) error {
	fc := newFuncChecker(c)
	seen := map[string]bool{}
	var fields []fieldInfo
	for i := range d.Fields {
		f := &d.Fields[i]
		if seen[f.Name] {
			return fmt.Errorf("line %d: duplicate field %q in struct %q", f.Line, f.Name, d.Name)
		}
		seen[f.Name] = true
		ft, err := fc.resolveTypeExpr(f.Type)
		if err != nil {
			return err
		}
		f.ResolvedType = ft
		fields = append(fields, fieldInfo{Name: f.Name, Typ: ft})
	}
	c.structs[d.Name].Fields = fields
	return nil
}

// registerFuncSig resolves and records fn's signature (amifl-spec.md
// section 8.7 forbids overloading, so one entry per name suffices) —
// Check's pass 1, run for every top-level function before any body is
// checked.
func (c *checker) registerFuncSig(fn *ast.FuncDecl) error {
	if fn.Name == reservedMainName {
		return fmt.Errorf("line %d: %q is a reserved name (used internally to compile `fn main`)", fn.Line, fn.Name)
	}
	if _, exists := c.funcs[fn.Name]; exists {
		return fmt.Errorf("line %d: duplicate function %q", fn.Line, fn.Name)
	}
	if _, exists := c.globals[fn.Name]; exists {
		return fmt.Errorf("line %d: %q is already declared as a const", fn.Line, fn.Name)
	}

	fc := newFuncChecker(c)
	seen := map[string]bool{}
	var params []string
	for i := range fn.Params {
		p := &fn.Params[i]
		if seen[p.Name] {
			return fmt.Errorf("line %d: duplicate parameter %q", p.Line, p.Name)
		}
		seen[p.Name] = true
		pt, err := fc.resolveTypeExpr(p.Type)
		if err != nil {
			return err
		}
		p.ResolvedType = pt
		params = append(params, pt)
	}

	retType, err := fc.resolveReturnTypeExpr(fn.ReturnType)
	if err != nil {
		return err
	}
	fn.ResolvedReturnType = retType
	c.funcs[fn.Name] = funcSig{params: params, ret: retType}
	return nil
}

// checkFunc type-checks fn's body against its already-registered
// signature (registerFuncSig) — Check's pass 2. Parameters are declared
// as non-reassignable bindings (binding.reassignable stays false; see its
// doc comment) holding "$N" tokens, 1-indexed and unqualified by name
// (amivm_spec.md section 3: "$Nの意味は「関数引数」...関数名による修飾は
// 無い" — position alone identifies a FUNC's own parameter).
func (c *checker) checkFunc(fn *ast.FuncDecl) error {
	sig := c.funcs[fn.Name]
	fc := newFuncChecker(c)
	fc.retType = sig.ret
	for i, p := range fn.Params {
		token := fmt.Sprintf("$%d", i+1)
		if err := fc.declare(p.Name, &binding{typ: sig.params[i], token: token}); err != nil {
			return fmt.Errorf("line %d: %s", p.Line, err)
		}
	}
	_, err := fc.checkBlock(fn.Body, sig.ret)
	return err
}

// checkBlock type-checks a block's expressions against the
// expression-oriented rule that every non-final expression must be
// Unit-typed (amifl-spec.md principle 1), and returns the block's own
// type: the last expression's type, checked against expected ("" for no
// context). Reused as-is for nested blocks (if/elif/else/while bodies,
// step 4) — callers wrap it with fc.pushScope/popScope so a nested
// block's own declarations don't leak out.
func (fc *funcChecker) checkBlock(b *ast.Block, expected string) (string, error) {
	if len(b.Exprs) == 0 {
		if expected != "" && expected != unitType {
			return "", fmt.Errorf("empty block has type Unit, expected %s", expected)
		}
		return unitType, nil
	}
	for i, e := range b.Exprs {
		if i < len(b.Exprs)-1 {
			t, err := fc.checkExpr(e, "")
			if err != nil {
				return "", err
			}
			// neverType (return/break/continue, ex11) is fine here too — a
			// diverging statement never produces a value that would need
			// discarding in the first place.
			if t != unitType && t != neverType {
				return "", fmt.Errorf("line %d: non-final expression in a block must be Unit-typed, got %s (discard it explicitly with `_ = ...` if this is intentional)", e.Pos(), t)
			}
			continue
		}
		return fc.checkExpr(e, expected)
	}
	panic("unreachable")
}
