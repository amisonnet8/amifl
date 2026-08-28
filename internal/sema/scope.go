package sema

import (
	"fmt"

	"github.com/amisonnet8/amifl/internal/ast"
)

// binding is a name bound in some scope: either a runtime `let` variable
// or a compile-time `const`.
type binding struct {
	isConst bool
	typ     string
	// value holds the literal to inline at reference sites when isConst
	// is true (see ast.IdentExpr.ConstValue); unused for a `let`.
	value ast.Expr
}

// checker holds state shared across an entire file: the global
// (top-level) const bindings, visible to every function.
type checker struct {
	globals map[string]*binding
}

// funcChecker checks one function body. AmiFL has no nested block scopes
// yet (those arrive with if/while in step 4), so a single flat map
// suffices for now — revisit this (following Weave's "declared" set or
// Seed/Cascade's per-scope name mangling, per CLAUDE.md's "過去に踏まれた
// 地雷" #4) once blocks can nest.
type funcChecker struct {
	*checker
	locals map[string]*binding
}

func newFuncChecker(c *checker) *funcChecker {
	return &funcChecker{checker: c, locals: map[string]*binding{}}
}

// lookup finds a binding for name, checking the function-local scope
// before falling back to file-level globals — a local `let`/`const` may
// shadow a top-level `const` of the same name.
func (fc *funcChecker) lookup(name string) (*binding, bool) {
	if b, ok := fc.locals[name]; ok {
		return b, true
	}
	if b, ok := fc.globals[name]; ok {
		return b, true
	}
	return nil, false
}

// declare registers a new local binding, rejecting a duplicate name
// within the same (currently flat) function scope.
func (fc *funcChecker) declare(name string, b *binding) error {
	if _, exists := fc.locals[name]; exists {
		return fmt.Errorf("%q is already declared in this scope", name)
	}
	fc.locals[name] = b
	return nil
}
