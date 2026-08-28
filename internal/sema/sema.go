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

// Check performs semantic validation: scalar type checking, let/const
// scope resolution (with const inlining), operators (step 3), if/elif/
// else/while/switch and their lexical scoping (step 4), top-level `fn`
// declarations (any number, any parameter list, callable in any order —
// forward references and mutual/self recursion all just work, since every
// signature is registered in one pass before any body is checked) and
// local closures with their own `Func` type (step 5), and the
// expression-oriented "every non-final expression in a block must be
// Unit-typed" rule (amifl-spec.md principle 1). AmiFL's full type system
// (structs, enums, collections, capability resolution, ...) grows across
// later steps — see CLAUDE.md's implementation step plan.
func Check(f *ast.File) error {
	c := &checker{globals: map[string]*binding{}, funcs: map[string]funcSig{}, structs: map[string]*structInfo{}}

	var consts []*ast.ConstDecl
	var funcs []*ast.FuncDecl
	var structs []*ast.StructDecl
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.ConstDecl:
			consts = append(consts, d)
		case *ast.FuncDecl:
			funcs = append(funcs, d)
		case *ast.StructDecl:
			structs = append(structs, d)
		default:
			return fmt.Errorf("sema: unknown top-level declaration %T", decl)
		}
	}

	// Pass 0: register every struct's name (0a) and then its fields (0b),
	// before anything else — a `fn`'s param/return type, a `const`'s
	// annotation/initializer, and another struct's own field can all name
	// a struct type regardless of where in the file it's declared.
	for _, st := range structs {
		if err := c.registerStructName(st); err != nil {
			return err
		}
	}
	for _, st := range structs {
		if err := c.registerStructFields(st); err != nil {
			return err
		}
	}

	// Consts are checked here, now that struct names/fields are known —
	// unchanged from step 2-5 otherwise (still in file order, still no
	// forward references between consts themselves), except a const's
	// initializer may now itself be a struct/tuple literal referencing a
	// struct type declared anywhere in the file.
	for _, d := range consts {
		if err := c.checkTopLevelConst(d); err != nil {
			return err
		}
	}

	// Pass 1: register every function's signature (and validate its own
	// parameter list) before checking any body, so a call can reference a
	// function declared later in the file, or itself, or another function
	// that in turn calls back into it.
	for _, fn := range funcs {
		if err := c.registerFuncSig(fn); err != nil {
			return err
		}
	}

	if _, err := findAndValidateMain(funcs); err != nil {
		return err
	}

	// Pass 2: check every body, now that every signature (including a
	// forward or mutually-recursive reference) is already known.
	for _, fn := range funcs {
		if err := c.checkFunc(fn); err != nil {
			return err
		}
	}
	return nil
}

// findAndValidateMain locates `fn main` among funcs (whose signatures
// registerFuncSig must have already resolved) and enforces amifl-spec.md
// section 14's entry-point shape: exactly one `fn main`, taking no
// parameters (the `List[String] args` form is deferred to step 7, once
// `List` exists) and returning `Int`.
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
	if len(main.Params) != 0 {
		return nil, fmt.Errorf("line %d: fn main must take no parameters in step 5 (List[String] args land in step 7)", main.Line)
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
	c.structs[d.Name] = &structInfo{Name: d.Name}
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
			if t != unitType {
				return "", fmt.Errorf("line %d: non-final expression in a block must be Unit-typed, got %s (discard it explicitly with `_ = ...` if this is intentional)", e.Pos(), t)
			}
			continue
		}
		return fc.checkExpr(e, expected)
	}
	panic("unreachable")
}
