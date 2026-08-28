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
	// value holds the literal/operator expression to inline at reference
	// sites when isConst is true (see ast.IdentExpr.ConstValue); unused
	// for a `let`.
	value ast.Expr
	// internalName is the Go variable name a `let` binding compiles to
	// (see ast.LetExpr.InternalName); unused for a `const` (which has no
	// runtime storage at all).
	internalName string
}

// checker holds state shared across an entire file: the global
// (top-level) const bindings, visible to every function.
type checker struct {
	globals map[string]*binding
}

// scope is one lexical block's bindings, chained to its enclosing scope
// (nil for a function's own top-level scope). Step 4 introduces real
// nesting — if/elif/else and while bodies each get a child scope via
// funcChecker.pushScope/popScope, so a `let` inside can shadow an outer
// one of the same name without disturbing it once the block ends.
type scope struct {
	parent *scope
	names  map[string]*binding
}

func newScope(parent *scope) *scope {
	return &scope{parent: parent, names: map[string]*binding{}}
}

func (s *scope) declare(name string, b *binding) error {
	if _, exists := s.names[name]; exists {
		return fmt.Errorf("%q is already declared in this scope", name)
	}
	s.names[name] = b
	return nil
}

func (s *scope) lookup(name string) (*binding, bool) {
	for cur := s; cur != nil; cur = cur.parent {
		if b, ok := cur.names[name]; ok {
			return b, true
		}
	}
	return nil, false
}

// funcChecker checks one function body: its current (innermost) lexical
// scope, how many `while` bodies currently enclose whatever expression is
// being checked (loopDepth — used to reject a stray break/continue found
// outside of any loop), and a function-wide counter used to mint every
// `let`'s InternalName.
type funcChecker struct {
	*checker
	scope     *scope
	loopDepth int
	declSeq   int
}

func newFuncChecker(c *checker) *funcChecker {
	return &funcChecker{checker: c, scope: newScope(nil)}
}

// lookup finds a binding for name, checking the current scope chain
// (innermost to outermost) before falling back to file-level globals — a
// local `let`/`const` may shadow a top-level `const` of the same name.
func (fc *funcChecker) lookup(name string) (*binding, bool) {
	if b, ok := fc.scope.lookup(name); ok {
		return b, true
	}
	if b, ok := fc.globals[name]; ok {
		return b, true
	}
	return nil, false
}

// declare registers a new binding in the current (innermost) scope,
// rejecting a duplicate name within that same scope — shadowing an outer
// scope's binding is fine, redeclaring within one block is not.
func (fc *funcChecker) declare(name string, b *binding) error {
	return fc.scope.declare(name, b)
}

// pushScope/popScope bracket checking one nested block (an if/elif/else
// branch, a while body). Every call site pushes right before checking the
// block and pops right after, so declarations inside never leak to a
// sibling branch or the enclosing scope.
func (fc *funcChecker) pushScope() {
	fc.scope = newScope(fc.scope)
}

func (fc *funcChecker) popScope() {
	fc.scope = fc.scope.parent
}

// freshInternalName mints name_N, N unique within this function — the Go
// variable name a `let` actually compiles to (ast.LetExpr.InternalName).
//
// This exists because of a real bug found only by running step 4's
// nested-scope support through the full amivm -> go build pipeline
// (CLAUDE.md's "実地検証必須" precedent): with if/while bodies getting
// their own child scope, an inner `let x` shadowing an outer `let x` is
// perfectly legal AmiFL, and — since AMIVM's IF/LOOP are genuinely nested
// Go blocks — perfectly legal *Go* too, if both compiled to the bare name
// `%x`. But amivm's own unused-variable self-healing (CLAUDE.md's amivm
// reference, "未使用変数の救済方法") locates "the" VAR declaration of a
// name by a name-only search and patches the first match it finds,
// assuming one declaration per name per function; asked to fix an unused
// *inner* `x` while an *outer*, already-used `x` shares the identical
// name, it kept patching the wrong (outer) declaration and never
// converged, hitting its retry cap. Minting every `let` a unique
// underlying name — the same fix Seed/Cascade already use for the
// general shadowing problem (CLAUDE.md's "過去に踏まれた地雷" #4) — sidesteps
// this by construction: no two `let`s ever share a Go name to begin with,
// whether or not they're allowed to shadow each other logically.
//
// One counter shared by every `let` in the function (rather than one per
// name) means a name never needs to be reserved in advance — declaring
// `x` twice in sibling scopes (not nested, so not really "shadowing" at
// all) still gets two distinct internal names for free, and the counter
// can't collide with codegen's own unrelated "amifl_tmpN" temp names
// (different prefix pattern entirely — see internal/codegen's newTemp).
func (fc *funcChecker) freshInternalName(name string) string {
	fc.declSeq++
	return fmt.Sprintf("%s_%d", name, fc.declSeq)
}
