package sema

import (
	"fmt"

	"github.com/amisonnet8/amifl/internal/ast"
)

// Check performs semantic validation: scalar type checking, let/const
// scope resolution (with const inlining), operators (step 3), if/elif/
// else/while/switch and their lexical scoping (step 4), and the
// expression-oriented "every non-final expression in a block must be
// Unit-typed" rule (amifl-spec.md principle 1). AmiFL's full type system
// (structs, enums, collections, capability resolution, general function
// calls, ...) grows across later steps — see CLAUDE.md's implementation
// step plan. Only a single `fn main` is supported so far; declaring and
// calling other functions arrives in step 5.
func Check(f *ast.File) error {
	c := &checker{globals: map[string]*binding{}}

	var funcs []*ast.FuncDecl
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.ConstDecl:
			if err := c.checkTopLevelConst(d); err != nil {
				return err
			}
		case *ast.FuncDecl:
			funcs = append(funcs, d)
		default:
			return fmt.Errorf("sema: unknown top-level declaration %T", decl)
		}
	}

	// Consts are resolved (in file order, so a const may only reference
	// an *earlier* const — CLAUDE.md's "確定した設計判断") in the loop
	// above before any function is checked here, so every function sees
	// every top-level const regardless of where in the file it sits.
	main, err := findAndValidateMain(funcs)
	if err != nil {
		return err
	}
	return c.checkFunc(main)
}

func findAndValidateMain(funcs []*ast.FuncDecl) (*ast.FuncDecl, error) {
	var main *ast.FuncDecl
	for _, fn := range funcs {
		if fn.Name != "main" {
			return nil, fmt.Errorf("line %d: step 2 only supports a single `fn main`; general function declarations arrive in step 5", fn.Line)
		}
		if main != nil {
			return nil, fmt.Errorf("line %d: duplicate `fn main` (first declared at line %d)", fn.Line, main.Line)
		}
		main = fn
	}
	if main == nil {
		return nil, fmt.Errorf("missing entry point: no `fn main` declared (amifl-spec.md section 14)")
	}
	retType, ok := canonicalType(main.ReturnType)
	if !ok {
		return nil, fmt.Errorf("line %d: unknown type %q", main.Line, main.ReturnType)
	}
	if retType != "Int64" {
		return nil, fmt.Errorf("line %d: fn main must return Int, got %s", main.Line, main.ReturnType)
	}
	return main, nil
}

func (c *checker) checkTopLevelConst(d *ast.ConstDecl) error {
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

func (c *checker) checkFunc(fn *ast.FuncDecl) error {
	retType, ok := canonicalType(fn.ReturnType)
	if !ok {
		return fmt.Errorf("line %d: unknown type %q", fn.Line, fn.ReturnType)
	}
	fc := newFuncChecker(c)
	_, err := fc.checkBlock(fn.Body, retType)
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
