package sema

import (
	"fmt"

	"github.com/amisonnet8/amifl/internal/ast"
)

// Check performs step 1's minimal semantic validation.
//
// This is intentionally narrow: AmiFL has no type checker yet (that lands
// in step 2), so Check only validates the handful of shapes step 1's
// bootstrap pipeline actually needs — a single parameter-less `fn main`
// returning Int, whose body is zero or more print(String literal) calls
// followed by an Int literal. Every rejection below is a step-1-specific
// placeholder standing in for the real type checker, not a permanent
// language rule; replace it as the real checks land (see CLAUDE.md's
// implementation step plan).
func Check(f *ast.File) error {
	main, err := findMain(f)
	if err != nil {
		return err
	}
	if main.ReturnType != "Int" {
		return fmt.Errorf("line %d: fn main must return Int, got %s", main.Line, main.ReturnType)
	}
	return checkMainBody(main)
}

func findMain(f *ast.File) (*ast.FuncDecl, error) {
	var main *ast.FuncDecl
	for _, fn := range f.Funcs {
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
	return main, nil
}

func checkMainBody(main *ast.FuncDecl) error {
	exprs := main.Body.Exprs
	if len(exprs) == 0 {
		return fmt.Errorf("line %d: fn main's body must not be empty", main.Line)
	}
	for i, e := range exprs {
		if i == len(exprs)-1 {
			if _, ok := e.(*ast.IntLit); !ok {
				return fmt.Errorf("fn main's last expression must be an Int literal (step 1 limitation)")
			}
			continue
		}
		if err := checkPrintCall(e); err != nil {
			return err
		}
	}
	return nil
}

func checkPrintCall(e ast.Expr) error {
	call, ok := e.(*ast.CallExpr)
	if !ok {
		return fmt.Errorf("non-final expressions in fn main must be Unit-typed calls (step 1 limitation: only print(...) exists so far)")
	}
	if call.Callee != "print" {
		return fmt.Errorf("line %d: step 1 only supports calling the built-in `print`, got %q", call.Line, call.Callee)
	}
	if len(call.Args) != 1 {
		return fmt.Errorf("line %d: print expects exactly 1 argument, got %d", call.Line, len(call.Args))
	}
	if _, ok := call.Args[0].(*ast.StringLit); !ok {
		return fmt.Errorf("line %d: step 1 only supports print(String literal)", call.Line)
	}
	return nil
}
