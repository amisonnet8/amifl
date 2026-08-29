package ast

// File is a parsed AmiFL source file: a sequence of top-level
// declarations.
type File struct {
	Decls []TopLevelDecl
}

// TypeExpr is a parsed type annotation — every position that used to hold
// a bare type-name string (amifl-spec.md sections 2.1/8.1/8.3) now holds
// one of these instead, as of step 7's List[T]/Array[T;N] (amifl-spec.md
// section 2.2), which need genuine recursive bracket syntax (`List[T]`,
// `Array[T;N]`, and their nesting — `List[List[Int]]`, `Array[Array[Int;
// N];M]`) that a single Ident token can no longer represent. NamedType
// alone covers every type-annotation step 1-6 ever wrote (a scalar or a
// struct name); ListType/ArrayType are new. This is parsed, unresolved
// syntax — sema's resolveTypeExpr (types.go) is what turns one into a
// canonical type string (the same kind of string every other part of sema
// already deals in), exactly the same division of labor Step 6 established
// between a struct/tuple literal's raw AST and its resolved type.
type TypeExpr interface {
	typeExprNode()
	Pos() int
}

// NamedType is a plain identifier used as a type: a scalar (amifl-spec.md
// section 2.1) or a struct name (section 2.2), or "Unit" in a return-type
// position (section 8.3) — everything step 1-6 supported, now wrapped in
// this node instead of a bare string.
type NamedType struct {
	Name string
	Line int
}

// ListType is `List[Elem]` (amifl-spec.md section 2.2) — always variable-
// length; Elem may itself be any TypeExpr (List[List[Int]] nests the same
// way tuples/structs already do).
type ListType struct {
	Elem TypeExpr
	Line int
}

// ArrayType is `Array[Elem;N]`, a single fixed dimension of compile-time
// size N. The parser is what desugars amifl-spec.md section 2.2's
// multi-dimension sugar (`Array[T;N1,N2,...]` ≡ `Array[Array[...
// Array[T;Nk]...];N2];N1]`) into nested ArrayType values at parse time —
// every later phase (sema, codegen) only ever sees a single Size per node,
// never a size list, the same "desugar once, early, so nobody downstream
// has to special-case the sugared form" approach step 4 used for `elif`
// and `switch`. Size is an Expr, not a bare integer, because it may
// reference a `const` (amifl-spec.md's compile-time-size requirement is
// exactly what AmiFL's `const` already models) — sema's resolveTypeExpr
// reduces it to a concrete value, since AMIVM's ARTYPE instruction takes
// a literal immediate, never an identifier or expression.
type ArrayType struct {
	Elem TypeExpr
	Size Expr
	Line int
}

// SetType is `Set[Elem]` (amifl-spec.md sections 2.2/13.5) — step 10.
// Elem is restricted to a comparable type (sema's isComparableKeyType:
// numeric, String, Bool, or Tuple — amifl-spec.md section 2.2's own
// wording, "Tは比較可能な型（数値・文字列・真偽値・タプル）のみ") since
// Set[T] compiles to a Go map[T]bool (CLAUDE.md's "確定した設計判断" for
// step 10), and Go itself would reject a non-comparable map key type.
type SetType struct {
	Elem TypeExpr
	Line int
}

// MapType is `Map[Key,Value]` (amifl-spec.md section 2.2) — step 10. Key
// carries the identical comparability restriction SetType.Elem does (the
// same Go map-key requirement); Value has none (amifl-spec.md never
// restricts it, and Go's own map imposes no comparability requirement on
// a map's value type).
type MapType struct {
	Key   TypeExpr
	Value TypeExpr
	Line  int
}

// TupleType is `Tuple2[T1,T2]` ... `Tuple8[T1,...,T8]` (amifl-spec.md
// section 2.2) — step 11, added once a user-defined function's return type
// needed to spell Tuple2[T,Error] (section 13.9's error-handling
// convention): step 6 deliberately left Tuple's own type-annotation syntax
// unimplemented (a tuple *literal* alone fully determines its type — see
// CLAUDE.md's "確定した設計判断" for step 6), but a bare function signature
// has no literal to infer from, so parse[T]/cast[T]-style built-ins
// returning Tuple2[T,Error] turned out to need this after all. The parser
// (parseTupleType) already validates len(Elems) against the digit in the
// type name itself ("Tuple2" through "Tuple8"), so sema's resolveTypeExpr
// never has to re-check arity here.
type TupleType struct {
	Elems []TypeExpr
	Line  int
}

// ChanType is `Chan[Elem]` (amifl-spec.md sections 2.2/11/13.8) — step 12.
// No comparability restriction on Elem (unlike SetType/MapType.Key) since a
// channel's element type imposes no such requirement in Go either.
type ChanType struct {
	Elem TypeExpr
	Line int
}

// StreamType is `Stream[Elem]` (amifl-spec.md sections 2.2/13.8) — step 12.
// Deliberately its own node rather than reusing ChanType even though both
// ultimately compile to the same Go channel shape (CLAUDE.md's "確定した
// 設計判断" for step 12): amifl-spec.md's own 17.2節#4 treats Stream[T] and
// Chan[T] as distinct types with no implicit conversion between them,
// mirroring Set[T]/Map[T,Bool]'s step-10 precedent of two distinct AmiFL
// types sharing one underlying Go representation.
type StreamType struct {
	Elem TypeExpr
	Line int
}

// FuncType is `fn(T1,T2,...) -> R` used as a *type annotation* (amifl-spec.md
// section 8.3, ex3) — a `let`, a `fn`/ClosureLit parameter, a `fn`/
// ClosureLit return type, or a struct field may now all name a function
// value's shape explicitly, lifting step 5's "no fn(...)->R grammar exists
// to write one in" limitation (this type's encoding — sema's makeFuncType —
// already existed since step 5 for a ClosureLit's own self-inferred type;
// only the surface syntax to *name* that encoding directly was missing).
// This is what makes two things possible that step 5 deliberately deferred
// (CLAUDE.md's "確定した設計判断" for that step): a top-level `fn` can now
// be referenced by name as a value (sema's resolveIdentExpr falls back to
// the top-level function table once an ordinary scope lookup fails), and a
// user-defined `fn`/closure can declare a parameter that itself accepts a
// function value — genuine higher-order functions, not just the built-in
// map/filter/reduce/sortBy's own hardcoded acceptance of one (sema/
// builtins_data.go).
//
// Distinct from ClosureLit (a *value*, carrying a body) — this node is
// purely a type. Params holds bare TypeExprs with no parameter *names*,
// mirroring every other bracketed type's element list (List[T],
// Tuple2[T1,T2], ...) rather than Param's `name: Type` pairs, which only
// make sense where an actual value binds to each parameter. sema's
// resolveTypeExpr resolves this into the exact same "fn(P1,...)->R"
// canonical string (types.go's makeFuncType) a ClosureLit's own
// ResolvedType already used before this type had any surface syntax at
// all — the two converge on one canonical string regardless of which one
// produced it, so nothing downstream (codegen's shared, deduplicated Go
// function-type minting included) needs to tell them apart.
//
// An inline ClosureLit literal still may only appear as a `let`'s direct
// value, plus one further carve-out since ex4 (a `|>` pipe's right-hand
// side, CallExpr.InlineClosure — see both doc comments); what's new here
// (ex3) is that a `let` binding one may now optionally carry a matching
// FuncType annotation (previously rejected unconditionally, since no
// grammar existed to write one), and that a Func-typed value — whether a
// `let`-bound closure or a bare top-level `fn` reference — can now flow
// through any position an ordinary value can: a call argument, a function
// parameter, a function's own return value.
type FuncType struct {
	Params []TypeExpr
	Ret    TypeExpr
	Line   int
}

// QualifiedType is `alias.Name` used as a type annotation (amifl-spec.md
// section 12.2, ex5) — a cross-package `struct`/`enum` referenced by its
// declaring package's own name, exactly as NamedType would name a
// same-package one. Parsed by parseTypeExpr the moment it sees `Ident '.'
// Ident` where a plain NamedType would otherwise have ended (every other
// bracketed type — List[T], Tuple2[T1,T2], ... — starts its own bracket
// right after the leading name instead, so there's no ambiguity between
// this and any of those). sema's resolveTypeExpr is the only place that
// interprets Alias/Name — looking Alias up in the current package's own
// imports, then Name in that import's Exports.Structs/Exports.Enums —
// resolving to the same "Qualified(GoName)" canonical string
// (types.go's makeQualifiedType) a qualified StructLit/enum-variant
// FieldExpr resolves to, so a value built one way and a parameter/`let`
// annotated the other agree on being the same type. Unlike FuncType (ex3),
// there is no separate "value" grammar this shadows — a struct/enum type
// name was never independently a value in this language (only
// `TypeName{...}`/`EnumType.Variant(...)` construction was), so there's no
// analogue of ClosureLit's own scope-cut to worry about here.
type QualifiedType struct {
	Alias string
	Name  string
	Line  int
}

func (*NamedType) typeExprNode()     {}
func (*ListType) typeExprNode()      {}
func (*ArrayType) typeExprNode()     {}
func (*SetType) typeExprNode()       {}
func (*MapType) typeExprNode()       {}
func (*TupleType) typeExprNode()     {}
func (*ChanType) typeExprNode()      {}
func (*StreamType) typeExprNode()    {}
func (*FuncType) typeExprNode()      {}
func (*QualifiedType) typeExprNode() {}

func (n *NamedType) Pos() int     { return n.Line }
func (n *ListType) Pos() int      { return n.Line }
func (n *ArrayType) Pos() int     { return n.Line }
func (n *SetType) Pos() int       { return n.Line }
func (n *MapType) Pos() int       { return n.Line }
func (n *TupleType) Pos() int     { return n.Line }
func (n *ChanType) Pos() int      { return n.Line }
func (n *StreamType) Pos() int    { return n.Line }
func (n *FuncType) Pos() int      { return n.Line }
func (n *QualifiedType) Pos() int { return n.Line }

// TopLevelDecl is a top-level declaration: *FuncDecl or *ConstDecl.
// AmiFL forbids top-level `let` (amifl-spec.md section 4, principle 5) —
// there is deliberately no *LetExpr case here, so the restriction is
// enforced by the grammar itself (internal/parser only recognizes `fn`
// and `const` at file scope) rather than a separate sema check.
type TopLevelDecl interface {
	topLevelDeclNode()
}

// FuncDecl is a top-level `fn` declaration (amifl-spec.md section 8).
// Step 5 lifts step 1-4's "only a parameter-less `fn main`" restriction:
// any number of top-level functions may be declared, each with any number
// of parameters, may call each other regardless of declaration order
// (including mutual/self recursion — sema collects every signature in one
// pass before checking any body), and `main` itself is just the one
// function sema additionally requires to exist, take no parameters (the
// `List[String] args` form is deferred to step 7, once `List` exists),
// and return `Int`.
type FuncDecl struct {
	Name       string
	Params     []Param
	ReturnType TypeExpr
	Body       *Block
	Line       int

	// ResolvedReturnType is filled in by sema: canonicalType(ReturnType),
	// or the Unit sentinel type for a `-> Unit` declaration (amifl-spec.md
	// section 8.3, "戻り値無しはfn(T1, ...) -> Unit") — the one place a
	// user-written "Unit" is accepted; see sema's canonicalReturnType.
	ResolvedReturnType string
}

// Param is one `name: Type` entry in a FuncDecl's or ClosureLit's
// parameter list (amifl-spec.md section 8.1). Type may be any TypeExpr —
// a plain scalar/struct/enum name, a collection (List/Array/Set/Map/Tuple/
// Chan/Stream), or, since ex3, FuncType (`fn(...) -> R`, a function value
// passed as a parameter — genuine higher-order functions; see FuncType's
// own doc comment for what this newly enables).
type Param struct {
	Name string
	Type TypeExpr
	Line int

	ResolvedType string // filled in by sema
}

// StructDecl is a top-level `struct Name { field1: Type1, ... }` declaration
// (amifl-spec.md section 2.2). Fields only, no methods (principle 4). Like
// FuncDecl, sema resolves every field's type in a dedicated pass before any
// function body is checked, so one struct's field may reference another
// struct declared later in the file (or, symmetrically, be referenced by
// one declared earlier) — order-independent, the same reasoning that
// motivates step 5's two-pass function signature collection.
type StructDecl struct {
	Name   string
	Fields []Param // reuses Param{Name,Type,Line,ResolvedType}; a field has no "reassignable" concept of its own
	Line   int
}

// EnumDecl is a top-level `enum Name { Variant1 [(field1: Type1, ...)] ...
// }` declaration (amifl-spec.md section 2.2) — step 8. Each variant is
// listed on its own line (mirroring StructDecl's field layout, and the
// spec's own formatting of the construct), never comma-separated. See
// CLAUDE.md's "確定した設計判断" for step 8's chosen runtime
// representation (a single STTYPE per enum, holding a `Tag` plus every
// variant's fields unioned together, each qualified `Variant_field` to
// avoid same-named fields across variants colliding as Go struct fields).
type EnumDecl struct {
	Name     string
	Variants []EnumVariant
	Line     int
}

// EnumVariant is one variant in an EnumDecl: a name and zero or more named
// fields (amifl-spec.md section 2.2, "各バリアントは0個以上の名前付き
// フィールドを持てる") — `Ok` (Fields empty) or `Retry(delay: Int)`.
// Reuses Param{Name,Type,Line,ResolvedType} exactly like StructDecl.Fields.
type EnumVariant struct {
	Name   string
	Fields []Param
	Line   int
}

// ExternDecl is a top-level `extern "path" as alias { type Name ... bind
// Name(params) -> Ret [as GoTarget] ... }` declaration (amifl-spec.md
// section 15) — step 13's mechanism for binding Go package assets. Alias
// is used two ways: as the amivm `-i alias=path` import mapping codegen
// requests (cmd/amifl/build.go's ExternImportMappings) so `?alias.Xxx`
// resolves deterministically regardless of whether alias happens to match
// the package's own name, and as the literal prefix codegen emits for
// every plain-function bind's callname. There is deliberately no AmiFL-
// level `alias.Name(...)` call syntax (amifl-spec.md section 15.2 rules
// this out explicitly: "AmiFLに`.`を使ったメソッド呼び出し構文は存在しな
// い") — every bind (and, symmetrically, every extern type) becomes an
// ordinary bare top-level name in scope, sharing FuncDecl's/StructDecl's
// no-overloading namespace (sema's registerExternBind/registerExternTypes
// check collisions against c.funcs/c.structs/c.enums/c.globals exactly
// like a `fn`/`struct` declaration would).
type ExternDecl struct {
	Path  string
	Alias string
	Types []ExternTypeDecl
	Binds []ExternBindDecl
	Line  int
}

// ExternTypeDecl is one `type Name` entry inside an ExternDecl — an opaque
// Go type (amifl-spec.md section 2.2's "Anyとは異なり、具体的な1つのGo型
// を指す不透明な型"-style handle, the same treatment File already gets)
// with no AmiFL-visible fields or operators. codegen maps it straight to
// `alias.Name` (no separate Go-side rename knob, unlike ExternBindDecl's
// GoTarget — a deliberate step-13 scope cut: every example needing this
// so far names the AmiFL type identically to the Go type it wraps).
type ExternTypeDecl struct {
	Name string
	Line int
}

// ExternBindDecl is one `bind Name(params) -> Ret [as GoTarget]` entry
// (amifl-spec.md section 15.1/15.2). GoTarget is empty when the `as`
// clause is omitted, meaning "call the package-level Go function named
// Name verbatim" (15.1's common case — `bind Marshal(...) -> ...` calls
// `alias.Marshal`). Written explicitly, GoTarget takes one of two shapes,
// told apart by whether it contains a `.`:
//
//   - a bare identifier: still a plain package-level function, just under
//     a different Go name than Name (renaming to dodge a collision with
//     another bind already using Name, or simply for AmiFL-side clarity);
//   - `Type.Method`: amifl-spec.md section 15.2's method-as-function
//     convention. Params[0]'s declared type must equal Type exactly (sema's
//     registerExternBind checks this) — it supplies the receiver value,
//     dispatched at codegen time via AMIVM's METHVAL (`local :=
//     receiver.method`) rather than a plain CALL, since AMIVM's callname
//     grammar has no way to spell a package-qualified method-expression
//     name directly (only one `.` is ever allowed in a callname token).
//     Every call site's sema resolution copies just the bare method name
//     onto that CallExpr's own ExternMethod field (never GoTarget itself,
//     nor the Type prefix — codegen only ever needs the method name, since
//     the receiver comes from evaluating Args[0], not from a fixed token).
type ExternBindDecl struct {
	Name       string
	GoTarget   string
	Params     []Param
	ReturnType TypeExpr
	Line       int

	ResolvedReturnType string // filled by sema, mirrors FuncDecl's own field
}

// ImportDecl is a top-level `import alias "path"` declaration (amifl-spec.md
// section 12.2) — step 14's cross-package reference mechanism. Path is a
// relative path (from the *importing file's own* directory — amifl-spec.md's
// own wording, "パスは参照元ファイル自身のディレクトリからの相対パス";
// files sharing one package directory therefore always resolve identically)
// resolved by internal/modloader at load time, never by sema/codegen
// themselves (mirroring ExternDecl.Path, resolved by amivm's own `-i`
// mapping rather than sema/codegen). Alias plays the identical dual role
// ExternDecl.Alias does: the lookup key sema uses to resolve `alias.Name`
// (FieldExpr's qualified-reference branch, resolveFieldExpr) and the
// mechanical rename prefix (`<alias>_<Name>`, amifl-spec.md section 12.4)
// codegen gives every one of the imported package's own top-level
// declarations — chosen by whichever import first brings a given package
// into the program (modloader's alias registry), since amifl-spec.md 12.4
// requires one alias to map to at most one package program-wide (assigning
// the same alias to two *different* packages is a compile error; two
// different files independently importing the *same* package need not
// agree on an alias — the first one seen wins as that package's canonical
// rename prefix, and every subsequent importer's own alias remains valid
// purely as its own file-local lookup key).
type ImportDecl struct {
	Alias string
	Path  string
	Line  int
}

// ConstDecl is a `const` declaration (amifl-spec.md section 4): a
// compile-time-only binding that is inlined at every reference site
// rather than compiled to a runtime variable. It doubles as both a
// TopLevelDecl (top-level `const`, allowed unlike `let`) and an Expr
// (function-local `const`, usable anywhere a `let` can appear) — the two
// positions share identical rules, so one node type serves both.
type ConstDecl struct {
	Name string
	Type TypeExpr // type annotation, or nil if omitted (inferred)
	// Value must resolve to a literal, directly or (recursively) through
	// references to earlier consts — step 2's only means of combining
	// values; full constant folding of const-to-const arithmetic
	// ("...または const どうしの演算") arrives with operators in step 3.
	Value Expr
	Line  int

	ResolvedType string // filled in by sema
}

// Block is a `{ ... }` body: a newline-separated sequence of expressions.
// AmiFL has no separate statement grammar (amifl-spec.md section 5) — a
// program is nothing but a sequence of expressions, and a block's own
// type is its last expression's type (every non-last expression must be
// Unit-typed, amifl-spec.md's principle 1).
type Block struct {
	Exprs []Expr
}

// Expr is any AmiFL expression node.
type Expr interface {
	exprNode()
	// Pos returns the 1-based source line the expression starts on, for
	// error messages.
	Pos() int
}

// LetExpr declares a new mutable local binding (amifl-spec.md section 4).
// Only usable inside a Block — there is no top-level counterpart, which
// is how the language's ban on top-level `let` (principle 5) is enforced
// structurally (see TopLevelDecl) rather than by a sema check.
type LetExpr struct {
	Name  string
	Type  TypeExpr // type annotation, or nil if omitted (inferred)
	Value Expr
	Line  int

	ResolvedType string // filled in by sema
	// Token is the full AMIVM value token codegen emits for this binding
	// (e.g. "%x_3") — Name suffixed with a function-wide unique counter
	// (sema's funcChecker.freshInternalName) and prefixed with "%", never
	// Name verbatim. See CLAUDE.md's "確定した設計判断" for step 4: two
	// `let`s named the same (an outer one and a shadowing inner one, now
	// that if/while bodies get their own nested scope) would otherwise
	// emit two Go variable declarations with the *identical* generated
	// name — legal Go (genuine block shadowing), but it broke amivm's
	// unused-variable self-healing, which locates "the" declaration of a
	// name assuming there's only ever one per function.
	//
	// Step 5 generalizes this field from a bare name (always manually
	// prefixed with "%" at every use site) to the complete token, because
	// a `let` bound to a closure literal's own parameter or a top-level
	// `fn`'s parameter needs this same field to carry a "$N" or "&L-N"
	// token instead — see IdentExpr.Token.
	Token string
}

// AssignExpr reassigns an existing `let`-bound local (amifl-spec.md
// section 4, "再代入可"). Whether Name actually names a reassignable
// `let` (as opposed to a `const` or an undeclared identifier) can only be
// known once scope is resolved, so — unlike LetExpr's top-level ban —
// this is checked by sema, not the grammar.
type AssignExpr struct {
	Name  string
	Value Expr
	Line  int

	Token string // filled in by sema; see LetExpr.Token
}

// DiscardExpr explicitly discards a non-Unit-typed expression's value
// (`_ = expr`; amifl-spec.md section 5, "捨てたいときは明示的に `_ = 式`").
type DiscardExpr struct {
	Value Expr
	Line  int
}

// IdentExpr reads a variable or constant by name.
type IdentExpr struct {
	Name string
	Line int

	// filled in by sema:
	ResolvedType string
	// ConstValue is non-nil when Name resolves to a const, holding the
	// literal to inline in its place — AmiFL constants have no runtime
	// storage (amifl-spec.md section 4, "参照箇所へインライン展開される").
	ConstValue Expr
	// Token is set instead of ConstValue when Name resolves to a runtime
	// binding — a `let` ("%x_3"), a top-level fn parameter ("$N"), or a
	// closure parameter ("&L-N"). See LetExpr.Token.
	Token string
	// IsFuncRef is set instead of ConstValue/Token when Name resolves to
	// neither a scope-bound binding nor a const, but a *top-level* `fn` or
	// extern plain-callee bind's own name, referenced here as a value
	// rather than called directly (ex3: passing a top-level function by
	// name, e.g. `let f = add` or `apply(add, 5)` — step 5's originally
	// deferred "トップレベル関数を名前で値として渡す" scope cut, lifted
	// now that FuncType gives this a type to have). There is no runtime
	// binding to read a token from in this case — codegen's genValue
	// instead emits an AMIVM FUNCVAL instruction on the spot
	// (genFuncRefValue) to materialize the reference into a fresh value.
	// FuncRefCallee is the already-resolved AMIVM callname ("?alias.
	// GoName") for the extern-bind case, mirroring CallExpr.CalleeToken's
	// identical convention; left "" for a plain top-level `fn`, in which
	// case codegen derives "!"+pkgPrefix+Name itself (mirroring
	// calleeToken()), since sema has no pkgPrefix of its own to bake in —
	// see program.pkgPrefix's doc comment. A method-style extern bind
	// (ExternMethod-only — no fixed callname without a receiver in scope)
	// can't be referenced this way at all; resolveIdentExpr rejects it
	// with a clear error rather than setting IsFuncRef.
	IsFuncRef     bool
	FuncRefCallee string
}

// CallExpr is a function call `callee(args...)` (amifl-spec.md section 8).
// Callee is a bare name — never an arbitrary expression — resolved by sema
// to exactly one of: the built-in `print` (Callee == "print", handled as
// its own special case throughout, unchanged since step 1 — the general
// built-in function library arrives in step 11), a top-level `fn`, or a
// local closure-valued variable (CalleeToken set; a local binding shadows a
// same-named top-level `fn`, mirroring how a `let` already shadows a
// top-level `const` — see sema's resolveCallExpr). AmiFL still has no
// syntax for calling the result of an arbitrary *general* expression
// (`(someExpr)(5)` isn't reachable — parseIdentOrCall only ever produces a
// CallExpr from a bare identifier), so Callee itself never needs to
// generalize beyond a name. Step 9's `|>` (amifl-spec.md section 9) is the
// one other producer of this node: `a |> f`/`a |> f(_, b)` desugar at parse
// time straight into a CallExpr exactly as if the user had written
// `f(a)`/`f(a, b)` by hand (parser's parsePipeRHS) — sema and codegen
// never know a pipe was involved at all, requiring no code of their own
// for the common case (CLAUDE.md's design-issue-7 prediction).
//
// InlineClosure is `|>`'s *other* producer (ex4, amifl-spec.md section 9:
// `data |> fn(x) -> R {...}`) — the one shape where Callee genuinely isn't
// a name at all: parsePipeRHS sets InlineClosure instead of Callee when the
// pipe's right-hand side is a bare `fn(...)->R{...}` literal rather than an
// existing name, and leaves Callee as the display-only placeholder
// "<closure>" (pipeChainLabel's own convention for a labelless stage,
// reused here since the real callee has no name to show). This is
// deliberately narrower than "a ClosureLit anywhere a value can go" —
// ClosureLit's own doc comment's restriction to a `let`'s direct value is
// otherwise unchanged; only this one additional position (pipe RHS) is
// carved out, reachable only through this field, never by relaxing
// resolveType's general ClosureLit rejection. sema's resolveCallExpr
// resolves InlineClosure via the same resolveClosureLit used for a `let`
// (checkCallArgs then validates Args — always exactly [lhs] — against the
// closure's own inferred parameter types, giving the same stage-numbered
// pipeline diagnostic, amifl-spec.md section 9.1, as any other pipe stage
// for free); codegen's calleeToken mints the closure into a fresh temp via
// genClosureLitInto and calls through that token, exactly as if the user
// had first `let`-bound it and then piped into the name.
type CallExpr struct {
	Callee        string
	InlineClosure *ClosureLit
	Args          []Expr
	Line          int
	// TypeArg is the bracketed type argument for the four reserved generic
	// builtins `cast[T]`/`parse[T]`/`unwrap[T]`/`okOr[T]` (amifl-spec.md
	// sections 13.3/13.9) — nil for every other call, including a call to a
	// Builtin that doesn't take one. This is surface syntax the parser
	// recognizes only for those four reserved names (parseIdentOrCall/
	// parsePipeRHS), never a general user-facing generics grammar
	// (principle 4, "ユーザー拡張ポイントを絞る" — no user generics). Always
	// exactly one type (none of the four ever takes more), unlike AMIVM's
	// own CALL instruction which allows several — a single field rather
	// than a slice keeps that arity fixed at the type level.
	TypeArg TypeExpr
	// ResolvedTypeArg holds TypeArg after sema resolves it to its canonical
	// type string (see sema/builtins.go) — "" when TypeArg is nil.
	ResolvedTypeArg string

	// filled in by sema:
	ResolvedType string
	// CalleeToken is the AMIVM callname operand for a closure call — a
	// variable token ("%f_3"/"$1"/"&1-1", whatever Callee resolved to)
	// copied from the resolved binding. Left empty for "print" (its own
	// hardcoded codegen path never reads this) and for a top-level `fn`
	// call, which codegen derives directly from Callee instead (`"!" +
	// Callee`, substituting codegen's internal entry-point name for
	// `"main"` — see codegen's calleeToken) so that sema never needs to
	// know that internal-naming detail (ast is sema's and codegen's only
	// shared vocabulary; neither package depends on the other).
	CalleeToken string
	// Builtin is the canonical name of the step-11 built-in function this
	// call resolved to (e.g. "len", "contains", "cast") — empty for
	// "print" (still its own step-1 special case), a user `fn` call, or a
	// closure call. See sema/builtins.go's resolveBuiltinCall and
	// codegen/builtins.go's genBuiltinValue/genBuiltinStmt for the two ends
	// of this dispatch.
	Builtin string
	// ArgTypes is Args' own resolved canonical types, parallel to Args —
	// filled by sema for every Builtin call so codegen's per-capability
	// dispatch never has to re-derive an argument's type from the AST
	// itself (mirroring how every other Resolved* field here exists so
	// codegen only ever reads what sema already computed).
	ArgTypes []string
	// ExternMethod is set (step 13) exactly when Callee resolves to a
	// method-style extern bind (amifl-spec.md section 15.2 — see
	// ExternBindDecl's doc comment) — the bare Go method name to invoke via
	// AMIVM's METHVAL, with Args[0] (already type-checked as the receiver)
	// supplying the receiver value and Args[1:] the method's own arguments.
	// CalleeToken is left empty in this case: there's no fixed callname to
	// derive ahead of time, since the callable value only exists once
	// METHVAL extracts it from the receiver at codegen time.
	ExternMethod string
	// ExternParamTypes is set (step 13) exactly when Callee resolves to any
	// extern bind (plain or method-style) — the bind's own *declared*
	// parameter types, parallel to Args (unlike ArgTypes above, which
	// records an argument's own resolved type and only for a Builtin call).
	// codegen/extern.go's externCallee reads this to find any position
	// declared "Any" and, only there, guards against a subtle Go gotcha: a
	// bare untyped integer-literal argument boxed directly into an `any`
	// parameter defaults to Go's own "int", not the "int64" AmiFL's
	// literal-defaulting (sema's resolveIntLit) actually gave it — see
	// that function's own doc comment for the full explanation.
	ExternParamTypes []string

	// Pipe-chain metadata (amifl-spec.md section 9.1, step 15's
	// stage-numbered type-mismatch diagnostic) — filled in by the parser's
	// parsePipeExpr/parsePipeRHS only for a CallExpr synthesized from a
	// `|>` step; PipeStage stays 0 (its zero value) for every ordinary
	// call, which callers use as the "not part of a pipe" test. PipeStage
	// is this call's 1-based position among the chain's `|>` steps (the
	// first RHS after the initial value is stage 1). PipeArgIndex is which
	// element of Args received the piped-in value (0 unless an explicit
	// `_` placeholder chose otherwise). PipeChainLabels holds one short
	// display label per position — index 0 the initial left-hand value,
	// index i (i>=1) that stage's own callee name — identical across every
	// CallExpr in the same chain, so a mismatch at any one stage can still
	// render the whole chain.
	PipeStage       int
	PipeArgIndex    int
	PipeChainLabels []string
}

// ClosureLit is `fn(params) -> R { body }` used as an expression — a
// local, unnamed function value (amifl-spec.md section 8.1, "let square =
// fn(x: Int) -> Int { x * x }"). Params/ReturnType are ordinary TypeExprs
// (Param's own doc comment) — since step 7+ these could already be List/
// Array/Set/Map/Tuple/struct types (map/filter/reduce/sortBy routinely
// pass a collection's element type through), and since ex3 a param/return
// may itself be FuncType, letting a closure accept or produce another
// function value. A ClosureLit is still only legal as a `let`'s direct
// value — never a call argument, an if/while condition, a binary operand,
// or any other position (sema's resolveType's default *ast.ClosureLit
// case rejects it there with a clear message; resolveLetExpr is the sole
// place that recognizes and accepts one) — with exactly one further
// carve-out since ex4: a `|>` pipe's right-hand side (CallExpr.
// InlineClosure's own doc comment), reached through a dedicated field
// rather than by relaxing this general rule. Passing an inline closure
// literal directly as an ordinary call argument remains future work,
// tracked separately from both ex3 and ex4. A `let` binding a ClosureLit
// may now optionally
// carry a matching FuncType annotation (FuncType's own doc comment) —
// sema checks it against the closure's self-inferred signature rather
// than rejecting it outright, since ex3 gives AmiFL a real grammar to
// write one in; omitting it, as before, is just as valid (the signature
// is always fully self-determined from the literal itself either way).
type ClosureLit struct {
	Params     []Param
	ReturnType TypeExpr
	Body       *Block
	Line       int

	// filled in by sema:
	ResolvedReturnType string
	// ResolvedType is this closure's own function type, encoded as
	// "fn(P1,P2,...)->R" (sema's makeFuncType/funcTypeParts) — purely an
	// internal sema/codegen convention with no user-facing surface syntax
	// in step 5 (see the type's doc comment above), used to type a `let`
	// binding a closure and to validate/resolve calls through it.
	ResolvedType string
}

// TupleLit is `(v1, v2, ...)` (amifl-spec.md section 2.2), always 2-8
// elements — the parser also produces this node for a syntactically
// ambiguous 1-element form `(x,)` (the trailing comma that disambiguates a
// tuple from a parenthesized grouping `(x)`, per the spec's own note on
// this), leaving sema to reject that arity (Tuple2~Tuple8 is the whole
// range the type system actually has; grouping already covers "wrap one
// expression in parens", so there is nothing a real Tuple1 would add).
//
// Step 6 restricts every element's own type to a scalar or a struct type —
// never itself a Tuple or a Func (sema's resolveTupleLit enforces this) —
// so ResolvedType's encoding (types.go's makeTupleType/tupleTypeParts,
// mirroring step 5's makeFuncType) never has to parse a nested, comma-
// containing element type back out of the flat comma-joined string it
// builds. Revisit only once a concrete need for nested tuples appears
// (mirrors step 5's identical scope cut for nested Func types).
type TupleLit struct {
	Elems []Expr
	Line  int

	ResolvedType string // filled by sema: types.go's makeTupleType(elemTypes)
}

// StructLit is `TypeName{field1: v1, field2: v2, ...}` (amifl-spec.md
// section 2.2/8.4), or, since ex5, `alias.TypeName{...}` — a cross-package
// struct construction (Qualifier holds the import alias, "" for the plain,
// same-package form). The parser only ever produces the qualified form from
// inside parsePostfixExpr's own `.field` loop (never parseIdentOrCall,
// which only ever sees a *bare* leading identifier) — right after building
// what would otherwise become a plain `alias.TypeName` FieldExpr with no
// call args, a following `{` (outside a noCompositeLit context, exactly the
// same disambiguation parseIdentOrCall's own unqualified check already
// uses) is reinterpreted as this node instead, with the FieldExpr's
// would-be Target's Name becoming Qualifier and its would-be Field becoming
// TypeName. Every one of the struct's declared fields must be given exactly
// once; order doesn't matter (each is matched by name, not position) —
// sema's resolveStructLit checks both completeness and duplicates, for
// either form identically (only the very first step — finding the right
// *structInfo to check fields against — differs by Qualifier).
type StructLit struct {
	Qualifier string
	TypeName  string
	Fields    []StructLitField
	Line      int

	// ResolvedType is TypeName verbatim for the same-package form (struct
	// types aren't aliased), or, for a qualified literal, the synthesized
	// "Qualified(GoName)" canonical string (types.go's makeQualifiedType) —
	// see that function's own doc comment for why a cross-package struct/
	// enum can't just reuse its own bare declared name as its canonical
	// type here the way a same-package one does.
	ResolvedType string
}

// StructLitField is one `name: value` entry inside a StructLit.
type StructLitField struct {
	Name  string
	Value Expr
	Line  int
}

// FieldExpr is postfix `target.field` (amifl-spec.md section 3.2): tuple
// index sugar (`t.0`, `t.1`, ...) when Target's type is a Tuple, ordinary
// struct field access when it's a struct, or (step 8) enum variant
// construction (`型名.バリアント名(...)`, section 2.2) when Target is a bare
// identifier naming a declared enum type — three different meanings
// sharing the same postfix-dot syntax, told apart by sema (resolveFieldExpr)
// rather than the parser, exactly the way tuple-vs-struct access already
// were before step 8. The enum case is a genuine third thing (Target names
// a *type*, not a value — nothing to FGET at all), reusing this node
// anyway because amifl-spec.md itself writes it with the identical
// `X.Y(...)` shape, structs have no methods (principle 4) to make
// `value.field(...)` ambiguous with, and CallExpr's own grammar never
// calls an arbitrary expression (only a bare name) — so a trailing `(...)`
// right after a `.field` could never mean anything else.
//
// Field is exactly the text the user wrote ("0", "1", ... a struct field
// name, or a variant name). AmivmField is what codegen emits after FGET's
// `>` prefix for the tuple/struct cases only — sema computes it once (a
// synthesized "F0"/"F1"/... for a tuple index, since Go struct fields
// can't be named with a bare digit, or Field verbatim for a struct) so
// codegen never has to re-derive tuple-vs-struct from ResolvedType (whose
// two encodings codegen has no vocabulary to tell apart — see types.go's
// doc comment on why that encoding is sema-internal). This mirrors
// LetExpr.Token/CallExpr.CalleeToken: sema resolves the AMIVM-facing
// detail once, onto the node, and codegen just reads it.
type FieldExpr struct {
	Target Expr
	Field  string
	// Args is non-nil (a zero-length slice counts) exactly when the parser
	// saw a trailing `(...)` after Field — reuses StructLitField{Name,
	// Value,Line} for each `field: value` entry, the same named-field
	// convention amifl-spec.md's struct literals already use (section 8.4)
	// and enum variant construction deliberately mirrors (section 2.2's
	// own example, "Status.Retry(delay: 5)"). nil for a plain `.field`/`.N`
	// access with no trailing call at all.
	Args []StructLitField
	Line int

	ResolvedType string // filled by sema
	AmivmField   string // filled by sema; meaningful only when IsEnumVariant is false

	// IsEnumVariant and VariantIndex are filled by sema exactly when this
	// FieldExpr resolves to enum variant construction — either
	// `EnumType.Variant` (Args == nil, a zero-field variant) or
	// `EnumType.Variant(field: v, ...)` (Args != nil). VariantIndex is the
	// variant's position in its enum's declared variant list, i.e. the
	// `Tag` value codegen writes (see EnumDecl's doc comment).
	IsEnumVariant bool
	VariantIndex  int

	// IsQualifiedCall, QualifiedCallee, and QualifiedArgTypes are filled by
	// sema exactly when this FieldExpr resolves to step 14's cross-package
	// function call `alias.Name(args...)` (amifl-spec.md section 12.2) —
	// Target a bare identifier naming a known `import` alias (never an enum
	// — that check runs first in resolveFieldExpr) and Field naming an
	// exported (capitalized) function or extern-bind in that package. Every
	// Args entry's Name must be "" (positional-only, principle 7 — "名前付
	// き引数無し"; resolveFieldExpr rejects a mix with enum-style named
	// entries as a clear error rather than silently misreading one for the
	// other). QualifiedCallee is the full AMIVM callname token the imported
	// package's own codegen pass minted for that declaration (e.g.
	// "!mathutil_Clamp" — see ImportDecl's doc comment on the rename
	// scheme), already resolved by the time the *importing* package is
	// checked (modloader processes packages in dependency order, leaves
	// first). QualifiedArgTypes mirrors CallExpr.ArgTypes's role for a
	// builtin call — each Args[i].Value's own resolved type, parallel to
	// Args — purely so codegen never has to re-derive an argument's type
	// from the AST itself.
	IsQualifiedCall   bool
	QualifiedCallee   string
	QualifiedArgTypes []string

	// QualifiedConstValue is filled by sema exactly when this FieldExpr
	// resolves to step 14's cross-package const reference `alias.NAME`
	// (Args == nil, Target a known import alias, Field naming an exported
	// const) — the exporting package's own already-resolved ConstValue
	// expression (ast.IdentExpr.ConstValue's identical inlining convention,
	// amifl-spec.md section 4: a const has no runtime storage, so every
	// reference — same-package or cross-package — is replaced by the
	// literal it stands for). ResolvedType is still set to the const's own
	// type, exactly like every other FieldExpr case.
	QualifiedConstValue Expr
}

// ListLit is `[v1, v2, ...]` (amifl-spec.md sections 2.2/3.1) — the one
// literal syntax shared by both List[T] and Array[T;N] ("既定のリテラル
// `[1,2,3]`はList[T]。型注釈で明示したときのみArray[T;N]になる"): which one
// it resolves to is decided purely by the surrounding type context
// (sema's resolveListLit), the same untyped-literal-adapts-to-`expected`
// pattern step 2 established for IntLit/FloatLit — no separate ArrayLit
// node exists. An empty `[]` needs an `expected` type to resolve at all
// (nothing else could tell it its element type).
type ListLit struct {
	Elems []Expr
	Line  int

	ResolvedType string // filled by sema: makeListType(elem) or makeArrayType(elem, n)
}

// SetOrMapLit is `{v1, v2, ...}` (Set[T]) or `{k1: v1, k2: v2, ...}`
// (Map[K,V]) (amifl-spec.md sections 2.2/3.1) — step 10. Both forms share
// one bare-brace literal syntax; which one a non-empty literal is gets
// decided by the parser with one token of lookahead right after its first
// entry (a `:` immediately after it means Map, anything else means Set —
// parser's parseBraceLit), so Elems (Set form) and Entries (Map form) are
// mutually exclusive and each non-nil slice always has at least one
// element. A bare `{}` sets neither (both nil) — exactly like an empty
// `[]` (ast.ListLit's doc comment), its actual kind can't be told apart
// from syntax alone, so sema's resolveSetOrMapLit falls back to `expected`
// to decide, erroring if there's no type annotation to consult either.
type SetOrMapLit struct {
	Elems   []Expr        // Set form; nil for the Map form or an empty `{}`
	Entries []MapLitEntry // Map form; nil for the Set form or an empty `{}`
	Line    int

	ResolvedType string // filled by sema: makeSetType(elem) or makeMapType(key,val)
}

// MapLitEntry is one `key: value` entry inside a SetOrMapLit's Map form.
type MapLitEntry struct {
	Key   Expr
	Value Expr
	Line  int
}

// IndexExpr is `target[index]` (amifl-spec.md section 3.2, "x[i]" — the
// spec describes this as sugar for a builtin `at(x,i)` call, but step 7
// compiles it directly to AGET rather than routing through a named
// function: the general capability-dispatched builtin-function machinery
// `at`/`setAt`/`slice` would eventually live behind (2.3/13.4節) doesn't
// exist until step 11, and step 7's Target is always statically known to
// be a List or an Array, so there is nothing left for a generic dispatch
// to resolve. Only List[T]/Array[T;N] are supported as Target in step 7
// (String/Map indexing arrive with their own types, later steps).
type IndexExpr struct {
	Target Expr
	Index  Expr
	Line   int

	ResolvedType string // filled by sema: Target's element type
}

// IndexAssignExpr is `target[index] = value` (amifl-spec.md section 3.2,
// "x[i] = v"). Always Unit-typed. Compiles directly to ASET, for the same
// reason IndexExpr compiles directly to AGET rather than a named `setAt`
// call — see IndexExpr's doc comment.
type IndexAssignExpr struct {
	Target Expr
	Index  Expr
	Value  Expr
	Line   int
}

// FieldAssignExpr is `target.field = value` (amifl-spec.md section 3.2,
// "p.x = 5", ex10). Always Unit-typed. Compiles directly to FSET, the same
// way IndexAssignExpr compiles directly to ASET — see IndexAssignExpr's and
// IndexExpr's doc comments; codegen's write-back (collections.go's
// readAssignableContainer) treats an Index/Field chain uniformly, so a
// target may freely mix both (`p.points[i].y = v`, `xs[i].total = v`, ...).
type FieldAssignExpr struct {
	Target Expr
	Field  string
	Value  Expr
	Line   int

	AmivmField string // filled by sema; the field's own bare Go name (see FieldExpr.AmivmField)
}

// SliceExpr is `target[from:to]` / `target[from:]` / `target[:to]` /
// `target[:]` (amifl-spec.md section 3.2) — From/To are nil when omitted
// (never a literal placeholder token; the spec's own "省略時は`_`を渡す"
// description of the equivalent `slice(x, from, to)` call is about that
// named function's own signature, not AmiFL surface syntax the parser
// needs to produce — codegen is what turns a nil bound into AMIVM's `_`
// placeholder, once, right where SLICE is emitted). Always resolves to a
// List[T] of Target's own element type, regardless of whether Target
// itself was a List or an Array (slicing a fixed-size array can't
// preserve a fixed size in the general case, since from/to may be runtime
// values — matching Go's own array-slicing semantics exactly, see
// CLAUDE.md's "確定した設計判断" for step 7).
type SliceExpr struct {
	Target Expr
	From   Expr // nil if omitted
	To     Expr // nil if omitted
	Line   int

	ResolvedType string // filled by sema: always makeListType(elemType)
}

// TryExpr is the postfix `?` operator (amifl-spec.md section 3.3): "戻り値
// がTuple2[T, Error]である呼び出し式の直後にのみ書ける後置演算子" — Value
// must resolve to Tuple2[U,Error] (IsBareError false, ElemType == U) or to
// bare Error itself (IsBareError true — a call like `close(f)?` once step
// 12 adds functions returning a bare Error; nothing produces one yet in
// step 11, but sema's check is written to accept it uniformly rather than
// special-casing "not yet reachable" away). Compiles to an early `RET` out
// of the *enclosing function* (not the enclosing block) when the error is
// non-nil, propagating a zero value in every earlier return slot alongside
// it (amifl-spec.md: "エラーがあればゼロ値＋エラーで即returnする糖衣") —
// which is why sema restricts this to a function whose own return type is
// itself Tuple2[_,Error] or Error (checked against the enclosing
// funcChecker's declared return type, not Value's), never generalizing to
// a 3rd type the way 17.2節#1 explicitly rules out.
type TryExpr struct {
	Value Expr
	Line  int

	// filled by sema:
	IsBareError bool   // true if Value's type is bare Error, not Tuple2[_,Error]
	ElemType    string // Value's Tuple2[ElemType,Error] payload type; "" if IsBareError
}

// ForExpr is `for x in items { ... }` (amifl-spec.md section 7, Body set,
// Yield nil) — always Unit-typed, side-effect-only — or (step 9)
// `for x in items yield expr` (Yield set, Body nil): amifl-spec.md's own
// stated equivalence to `items |> map(x => expr)`, but codegen compiles it
// directly into a length-preallocated List built by a single loop
// (genForYieldValue) rather than literally generating a call to a builtin
// named `map` — the general capability-dispatched builtin-function
// machinery `map`/`filter`/etc. (2.3/13.4節) doesn't exist until step 11,
// mirroring step 7's identical reasoning for why `x[i]` compiles directly
// to AGET instead of routing through a named `at` function. Body and
// Yield are mutually exclusive; exactly one is always set. Items must be
// a List[T], Array[T;N], or (step 10) Set[T] — Var2 empty — or a Map[K,V]
// with Var2 set (see Var2's own doc comment); a bare `{}` (Set/Map's own
// ambiguous empty form) can never actually appear as Items in a way that
// resolves either, since a for-loop's Items has no `expected` type of its
// own for sema to disambiguate it with (amifl-spec.md never restricts
// this — it just falls out of resolveForExpr calling checkExpr(Items, "")).
//
// break/continue inside Body act on this loop exactly like WhileExpr's
// (same loopDepth bookkeeping, never crossing a closure boundary) — but
// amifl-spec.md section 7 explicitly restricts break/continue to the
// non-yield form ("breakcontinueはyield無し形のみで使用可"), so
// resolveForExpr suppresses loopDepth (resets it to 0, exactly like a
// closure body does for the identical "can't reach an enclosing loop"
// reason) while checking Yield — a break/continue written inside a yield
// expression is rejected with the ordinary "outside of a loop" error,
// regardless of whether this ForExpr itself happens to be lexically
// nested inside some other loop.
type ForExpr struct {
	Var   string
	Items Expr
	Body  *Block // set iff Yield == nil
	Yield Expr   // set iff Body == nil (step 9)
	Line  int

	// Var2 is step 10's `for k, v in m { ... }` second loop variable — set
	// (non-empty) only for Map[K,V] iteration, which is the only iterable
	// collection type step 10 adds that can't be walked with a single
	// per-element value (a Map entry is inherently a key *and* a value).
	// The parser rejects Var2 combined with Yield outright (`for k, v in m
	// yield ...` never parses) rather than letting sema reject it — step
	// 10's deliberate scope cut, mirroring how the parser (not sema)
	// already keeps Body and Yield themselves mutually exclusive by
	// construction; a two-variable Map yield form is a plausible future
	// generalization, revisit once an actual need appears.
	Var2 string

	// filled by sema:
	// ItemsType is Items' own resolved canonical type (List/Array/Set/Map)
	// — codegen needs this (unlike step 7/9, which never had to tell
	// collection *kinds* apart at this point) to choose how to lower the
	// loop: List/Array are already index-addressable and iterate directly;
	// Set/Map are not, so codegen first collects their keys into a plain
	// List via MPKEYS and iterates that instead (collections.go's
	// prepareForIteration).
	ItemsType string
	// ElemType is Var's own type: the single-variable form's element type
	// (List/Array/Set), or the two-variable form's *key* type (Var2Type
	// holds the value type in that case).
	ElemType string
	// VarToken is Var's AMIVM value token (e.g. "%x_7"), minted once via
	// freshInternalName exactly like LetExpr.Token — Var is a non-
	// reassignable binding (binding.reassignable stays false), mirroring
	// a function/closure parameter rather than a `let` (amifl-spec.md is
	// silent on whether a for-loop variable may be reassigned; step 5's
	// same silence about parameters was resolved the same conservative
	// way, for the same "明示性 > 簡潔さ" reasoning).
	VarToken string
	// Var2Type/Var2Token mirror ElemType/VarToken for Var2 (the Map value
	// type) — unused unless Var2 != "".
	Var2Type  string
	Var2Token string
	// ResolvedType is set only for the Yield form: always
	// makeListType(Yield's own resolved type) — unused (implicitly Unit)
	// for the Body form, exactly like WhileExpr never bothers storing its
	// own always-Unit type.
	ResolvedType string
}

// RangeExpr is `a..b` (half-open) / `a..=b` (closed) — amifl-spec.md
// section 3.1/7.3, ex2 of the post-15-step roadmap ("Range型の追加(for i
// in 0..10のような数値範囲反復)"). From/To are ordinary expressions (not
// restricted to literals), always Int64 — no other integer width, no
// Float, and deliberately no user-writable surface type-annotation syntax
// at all (a RangeExpr's own resolved type is always the fixed string
// "Range", which canonicalType never recognizes as a nameable type —
// mirroring step 5's identical scope cut for Func/ClosureLit: a value
// exists and can be `let`-bound and consumed, but can never be spelled
// out in a `: Type`/`-> Type` annotation, a struct field, or a
// List/Array/Set/Map element-type position). A Range value is consumed
// only by `for x in range { ... }` / `for x in range yield expr`
// (ForExpr.ItemsType == "Range") — no `len`/`contains`/other 13.4 builtin
// wiring, no `.From`/`.To` field access, no descending/stepped ranges;
// From >= To (half-open) or From > To (closed) is a valid empty range at
// runtime, never a compile-time error. Codegen represents it as a single
// compiler-synthesized `{From, To int64}` struct, always normalized to a
// half-open [From,To) pair at construction time — Inclusive only ever
// affects how genRangeValue builds that struct (bumping To by one), never
// surviving into the runtime representation itself, so no separate
// "inclusive" flag needs to travel any further than that one codegen
// call (internal/codegen/structs.go's rangeGoTypeName).
type RangeExpr struct {
	From, To  Expr
	Inclusive bool
	Line      int
}

// SwitchExpr is `switch subject { case Type.Variant(binding, ...): body ...
// [default: body] }` (amifl-spec.md section 10) — step 8. This node exists
// only for the subject-carrying form; the subject-less, Bool-only case
// list (step 4's scope, still fully supported) keeps desugaring straight
// into IfExpr at parse time instead (parser's parseBoolSwitchExpr), since
// that form is exactly an elif chain with no bindings and no reason to
// gain one now — see IfExpr's doc comment. Step 8's scope further
// restricts Subject to a static enum type: `is Type`/`in [...]` (spec's
// other two case-pattern forms) need Any/collection-capability machinery
// that doesn't exist until later steps (extern's Any boundary is step 13;
// capability-dispatched Containable is step 11) — attempting either here
// is a plain parse error (parseSwitchCasePattern only ever recognizes
// `EnumType.Variant[...]`).
type SwitchExpr struct {
	Subject Expr
	Cases   []SwitchCase
	Default *Block // nil unless written; only legal to omit once Cases exhaustively covers every one of Subject's enum's variants exactly once (amifl-spec.md section 10, "全バリアントを1回ずつ網羅していればdefault省略可")
	Line    int

	ResolvedType string // filled by sema
	EnumName     string // filled by sema: Subject's resolved enum type name
}

// SwitchCase is one `case EnumType.Variant[(binding1, binding2, ...)]:
// body` clause of a SwitchExpr. Bindings are bare identifiers, positionally
// matching the variant's own declared field list — amifl-spec.md section
// 10's own example ("Status.Retry(delay)と書くとフィールドdelayが...束縛
// される") requires each one to literally equal its corresponding field's
// declared name (sema's resolveSwitchExpr enforces this; want a different
// local name instead, `let` it inside Body). nil for a variant with no
// declared fields.
type SwitchCase struct {
	EnumName string // the enum type name written in the pattern (e.g. "Status" in "Status.Retry(delay)") — sema checks this matches the SwitchExpr's own Subject enum, catching a copy-pasted or mistyped enum name with a clear error instead of an opaque "no such variant"
	Variant  string
	Bindings []string
	Body     *Block
	Line     int

	// filled by sema:
	VariantIndex  int      // this variant's position in the enum's declared variant list — codegen's Tag value
	BindingTokens []string // AMIVM value tokens minted for each binding, parallel to Bindings
	BindingTypes  []string // each binding's own canonical field type, parallel to Bindings
}

// StringLit is a string literal.
type StringLit struct {
	Value string
	Line  int
}

// IntLit is an integer literal. Value is uint64, not int64, so it can
// represent UInt64's full range; a literal is always written without a
// sign (amifl-spec.md section 3.1), so it's never itself negative — a
// negative value is a UnaryExpr{Op: "-"} wrapping one of these instead.
//
// Token is the exact source text as lexed (base prefix and '_' digit
// separators included verbatim, e.g. "0x1_A", ex7) — codegen emits this
// directly as the AMIVM literal token rather than re-deriving decimal text
// from Value, since amivm's own upgraded literal grammar
// (ignored/amivm/amivm_spec.md section 6) accepts these forms "そのまま"
// (as-is) and its documented negative-literal examples (`-0x1A` etc.) are
// exactly "-" prepended to a raw token like this one — codegen.go's
// literalToken already prepends "-" this same way for a decimal Token, so
// no new negation logic was needed. Value still carries the fully-parsed
// magnitude regardless of base, and is what every semantic check (range/
// overflow, capability dispatch, ...) operates on — Token only ever
// matters to codegen's literal rendering.
type IntLit struct {
	Value uint64
	Token string
	Line  int
}

// FloatLit is a floating-point literal.
type FloatLit struct {
	Value float64
	Line  int
}

// BoolLit is a `true`/`false` literal.
type BoolLit struct {
	Value bool
	Line  int
}

// BinaryExpr is a binary operator expression (amifl-spec.md section 6):
// arithmetic (+ - * / %), bitwise (& | ^ &^), shift (<< >>), comparison
// (== != < <= > >=), or logical (&& ||). Op holds the operator's surface
// text ("+", "==", "&&", ...).
type BinaryExpr struct {
	Op    string
	Left  Expr
	Right Expr
	Line  int

	// ResolvedType is filled in by sema: for arithmetic/bitwise/shift/
	// concatenation operators it's both operands' (equal, per principle 2)
	// type and the expression's own type; for comparison/logical operators
	// (whose own type is always Bool) it's still the operands' shared
	// type, which codegen needs to declare the correct Go type for
	// whichever operand is a sub-expression requiring its own temp.
	ResolvedType string
}

// UnaryExpr is a prefix operator expression (amifl-spec.md section 6): `!`
// (logical not, Bool only), `-` (arithmetic negate, Numeric), or `~`
// (bitwise not, integer types only).
type UnaryExpr struct {
	Op      string
	Operand Expr
	Line    int

	ResolvedType string // filled in by sema
}

// ElseBody is an IfExpr's second-and-later branch: nil (no else, which
// forces the whole if-expression to be Unit-typed — amifl-spec.md section
// 7, "else省略時はUnit型限定"), an *IfExpr (continuing an elif chain), or
// a *Block (the final else's body).
type ElseBody interface {
	elseBodyNode()
}

func (*IfExpr) elseBodyNode() {}
func (*Block) elseBodyNode()  {}

// IfExpr is `if cond { ... } [elif cond { ... }]* [else { ... }]?`
// (amifl-spec.md section 7) — a full expression, not a statement. `elif`
// desugars at parse time into a nested else-branch IfExpr rather than
// getting its own field or AMIVM instruction (CLAUDE.md's "過去に踏まれた
// 地雷" #2: codegen emits ELSE + a nested IF, never AMIVM's ELIF, since
// ELIF's condition operand can't itself span multiple instructions).
// `switch`'s Bool-only case form (step 4's scope; `is Type`/`in [...]`/
// enum patterns are later steps) desugars into this same node at parse
// time too — with no subject and only plain Bool conditions, a switch
// case list *is* an elif chain (amifl-spec.md principle 3: "1つの仕組みで
// 足りるものを2つ用意しない").
type IfExpr struct {
	Cond Expr
	Then *Block
	Else ElseBody // nil | *IfExpr | *Block
	Line int

	ResolvedType string // filled in by sema; "Unit" when Else doesn't end in a *Block
}

// WhileExpr is `while cond { ... }` (amifl-spec.md section 7): always
// Unit-typed. break/continue inside Body act on this loop only, never
// crossing a closure boundary (enforced by sema, not representable in the
// AST).
type WhileExpr struct {
	Cond Expr
	Body *Block
	Line int
}

// BreakExpr and ContinueExpr are `break`/`continue` (amifl-spec.md section
// 7): always Unit-typed, only legal inside a WhileExpr's Body (sema
// rejects one found outside any loop).
type BreakExpr struct{ Line int }
type ContinueExpr struct{ Line int }

func (*FuncDecl) topLevelDeclNode()   {}
func (*ConstDecl) topLevelDeclNode()  {}
func (*StructDecl) topLevelDeclNode() {}
func (*EnumDecl) topLevelDeclNode()   {}
func (*ExternDecl) topLevelDeclNode() {}
func (*ImportDecl) topLevelDeclNode() {}

func (*ConstDecl) exprNode()       {}
func (*LetExpr) exprNode()         {}
func (*AssignExpr) exprNode()      {}
func (*DiscardExpr) exprNode()     {}
func (*IdentExpr) exprNode()       {}
func (*CallExpr) exprNode()        {}
func (*StringLit) exprNode()       {}
func (*IntLit) exprNode()          {}
func (*FloatLit) exprNode()        {}
func (*BoolLit) exprNode()         {}
func (*BinaryExpr) exprNode()      {}
func (*UnaryExpr) exprNode()       {}
func (*IfExpr) exprNode()          {}
func (*WhileExpr) exprNode()       {}
func (*BreakExpr) exprNode()       {}
func (*ContinueExpr) exprNode()    {}
func (*ClosureLit) exprNode()      {}
func (*TupleLit) exprNode()        {}
func (*StructLit) exprNode()       {}
func (*FieldExpr) exprNode()       {}
func (*ListLit) exprNode()         {}
func (*SetOrMapLit) exprNode()     {}
func (*IndexExpr) exprNode()       {}
func (*IndexAssignExpr) exprNode() {}
func (*FieldAssignExpr) exprNode() {}
func (*SliceExpr) exprNode()       {}
func (*ForExpr) exprNode()         {}
func (*RangeExpr) exprNode()       {}
func (*SwitchExpr) exprNode()      {}
func (*TryExpr) exprNode()         {}

func (n *ConstDecl) Pos() int       { return n.Line }
func (n *LetExpr) Pos() int         { return n.Line }
func (n *AssignExpr) Pos() int      { return n.Line }
func (n *DiscardExpr) Pos() int     { return n.Line }
func (n *IdentExpr) Pos() int       { return n.Line }
func (n *CallExpr) Pos() int        { return n.Line }
func (n *StringLit) Pos() int       { return n.Line }
func (n *IntLit) Pos() int          { return n.Line }
func (n *FloatLit) Pos() int        { return n.Line }
func (n *BoolLit) Pos() int         { return n.Line }
func (n *BinaryExpr) Pos() int      { return n.Line }
func (n *UnaryExpr) Pos() int       { return n.Line }
func (n *IfExpr) Pos() int          { return n.Line }
func (n *WhileExpr) Pos() int       { return n.Line }
func (n *BreakExpr) Pos() int       { return n.Line }
func (n *ContinueExpr) Pos() int    { return n.Line }
func (n *ClosureLit) Pos() int      { return n.Line }
func (n *TupleLit) Pos() int        { return n.Line }
func (n *StructLit) Pos() int       { return n.Line }
func (n *FieldExpr) Pos() int       { return n.Line }
func (n *ListLit) Pos() int         { return n.Line }
func (n *SetOrMapLit) Pos() int     { return n.Line }
func (n *IndexExpr) Pos() int       { return n.Line }
func (n *IndexAssignExpr) Pos() int { return n.Line }
func (n *FieldAssignExpr) Pos() int { return n.Line }
func (n *SliceExpr) Pos() int       { return n.Line }
func (n *ForExpr) Pos() int         { return n.Line }
func (n *RangeExpr) Pos() int       { return n.Line }
func (n *SwitchExpr) Pos() int      { return n.Line }
func (n *TryExpr) Pos() int         { return n.Line }
