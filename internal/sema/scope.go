package sema

import (
	"fmt"

	"github.com/amisonnet8/amifl/internal/ast"
)

// binding is a name bound in some scope: a runtime `let` variable, a
// compile-time `const`, or (step 5) a runtime, non-reassignable function
// parameter or closure parameter.
type binding struct {
	isConst bool
	// reassignable is true only for a `let` binding (amifl-spec.md
	// section 4, "再代入可"). A const is rejected for its own reason
	// (isConst, "cannot assign to a const"); a function/closure parameter
	// is also not reassignable — the spec grants "再代入可" explicitly and
	// only to `let`, never to a parameter, and amifl-spec.md's principle
	// of explicitness ("明示性 > 簡潔さ") argues against silently
	// extending that to parameters absent a concrete need. If a caller
	// needs a mutable copy, `let x2 = x` remains available.
	reassignable bool
	typ          string
	// value holds the literal/operator expression to inline at reference
	// sites when isConst is true (see ast.IdentExpr.ConstValue); unused
	// otherwise.
	value ast.Expr
	// token is the full AMIVM value token this binding reads as — "%x_3"
	// for a `let`, "$N" for a top-level fn parameter, "&L-N" for a
	// closure parameter (see ast.LetExpr.Token) — computed once at
	// declaration time and reused verbatim by every reference, at any
	// nesting depth (CLAUDE.md's 過去に踏まれた地雷 #9: a closure body is
	// just a child scope, so a captured outer token needs no special
	// handling to keep working unchanged inside it). Unused for a const.
	token string
}

// funcSig is a top-level `fn`'s signature (amifl-spec.md section 8),
// resolved once per file — before any function body is checked — so that
// calls may appear in any order: forward references, mutual recursion,
// and self-recursion (amifl-spec.md section 8.6) all just work, the way
// they would for any statically-typed language with whole-file overload-
// free function declarations.
type funcSig struct {
	params []string
	ret    string
	// externCallee/externMethod are set only for a signature registered
	// from an extern `bind` (registerExternBind, step 13) — at most one is
	// ever non-empty. externCallee is the full AMIVM callname ("?alias.
	// GoName") for a plain package-level function bind, copied verbatim
	// onto CallExpr.CalleeToken at each call site exactly like a closure
	// call already does (codegen's calleeToken treats any non-empty
	// CalleeToken as "already resolved, use as-is" regardless of source).
	// externMethod is the bare Go method name for a method-style bind
	// (amifl-spec.md section 15.2) — copied onto CallExpr.ExternMethod
	// instead, since there's no fixed callname to precompute (see that
	// field's own doc comment).
	externCallee string
	externMethod string
}

// structInfo is a top-level `struct`'s resolved shape (amifl-spec.md
// section 2.2), registered once per file — before any function body or
// other struct's own fields are checked, the same forward-reference-
// friendly two-pass approach funcSig uses for `fn` (registerStructName
// records the name alone; registerStructFields fills Fields once every
// struct name in the file is already known, so one struct's field may
// reference another declared later, or even earlier, in the file).
type structInfo struct {
	Name   string
	Fields []fieldInfo
}

type fieldInfo struct {
	Name string
	Typ  string
}

// fieldType looks up one field's canonical type by name.
func (s *structInfo) fieldType(name string) (string, bool) {
	for _, f := range s.Fields {
		if f.Name == name {
			return f.Typ, true
		}
	}
	return "", false
}

// enumInfo is a top-level `enum`'s resolved shape (amifl-spec.md section
// 2.2) — step 8, registered the same forward-reference-friendly two-pass
// way structInfo is (registerEnumName records the name alone;
// registerEnumVariants fills Variants once every struct/enum name in the
// file is known, so a variant field may reference a struct or enum
// declared anywhere in the file).
type enumInfo struct {
	Name     string
	Variants []variantInfo
}

// variantIndex looks up a variant's position in Variants by name — this
// position is also its Tag value (codegen's genEnumDecl/genEnumVariantValue).
func (e *enumInfo) variantIndex(name string) (int, bool) {
	for i, v := range e.Variants {
		if v.Name == name {
			return i, true
		}
	}
	return -1, false
}

// variantInfo is one enum variant's resolved shape: a name and its own
// field list (possibly empty).
type variantInfo struct {
	Name   string
	Fields []fieldInfo
}

func (v *variantInfo) fieldType(name string) (string, bool) {
	for _, f := range v.Fields {
		if f.Name == name {
			return f.Typ, true
		}
	}
	return "", false
}

// checker holds state shared across an entire file: the global
// (top-level) const bindings, the top-level `fn` signature table, and the
// top-level `struct`/`enum` shape tables, all visible to every function.
type checker struct {
	globals map[string]*binding
	funcs   map[string]funcSig
	structs map[string]*structInfo
	enums   map[string]*enumInfo
	// externTypes is the set of `type Name` names declared inside any
	// extern block (step 13) — just existence, unlike structs/enums, since
	// an extern type has no fields/variants of its own for canonicalType to
	// need; codegen independently derives the actual Go type string
	// ("alias.Name") by walking ast.ExternDecl directly (ast is sema's and
	// codegen's only shared vocabulary — see CLAUDE.md's リポジトリ構成).
	externTypes map[string]bool
	// externAliases tracks every extern block's own `as alias` (alias ->
	// path) purely to reject a second block reusing an alias already
	// claimed in this file — see reservedExternAliases's doc comment for
	// why alias collisions matter beyond ordinary name-shadowing.
	externAliases map[string]string
	// imports maps each `import alias "path"` this package's own files
	// declare (amifl-spec.md section 12.2, step 14) to that alias's
	// already-computed Exports — nil for a package with no imports at all
	// (Check's own single-file, prefix-less call always passes nil).
	// resolveFieldExpr's qualified-reference branch is the sole reader.
	imports map[string]Exports
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

// funcChecker checks one function (or, step 5, closure) body: its current
// (innermost) lexical scope, how many `while` bodies currently enclose
// whatever expression is being checked (loopDepth — used to reject a
// stray break/continue found outside of any loop), how many CLOS levels
// deep the expression currently being checked is nested inside its own
// enclosing FUNC (closureDepth — 0 directly inside the function body, 1
// inside its outermost closure literal, 2 inside a closure nested in
// that, ... — used only to compute a closure parameter's "&L-N" token at
// declaration time), and a function-wide counter used to mint every
// `let`'s Token.
//
// A closure literal reuses this *same* funcChecker (not a fresh one) for
// its body — resolveClosureLit only pushes a child scope and saves/
// resets loopDepth/closureDepth around the call — so that its `let`s draw
// from the identical declSeq counter as the enclosing function's. This is
// deliberate, not an oversight: it's what guarantees every "%xxx" token
// anywhere in one compiled Go function (including inside any nested CLOS,
// itself just a nested Go func literal) stays globally unique, which is
// exactly the invariant CLAUDE.md's "確定した設計判断" for step 4 (the
// amivm unused-variable self-healing bug) already established is load-
// bearing — a closure body is just another place that invariant must
// keep holding.
type funcChecker struct {
	*checker
	scope        *scope
	loopDepth    int
	closureDepth int
	declSeq      int
	// retType is the declared return type of the function/closure whose
	// body is currently being checked — step 11's postfix `?`
	// (resolveTryExpr) needs it to enforce amifl-spec.md section 3.3's
	// "自分を囲む関数の戻り値がTuple2[U,Error]（またはError単体）の場合に
	// のみ使用可" restriction. Set once in Check's checkFunc for a
	// top-level `fn`; saved/restored around checkClosureBody exactly like
	// loopDepth (a closure's own declared return type governs `?` inside
	// its body, not the enclosing function's — a `?` can't reach past a
	// closure boundary any more than break/continue can, amifl-spec.md
	// section 8's closures being ordinary functions in this respect).
	retType string
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

// freshInternalName mints name_N, N unique within this function — the bare
// Go variable name a `let` actually compiles to (its Token is "%" + this).
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
