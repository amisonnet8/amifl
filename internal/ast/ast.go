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

func (*NamedType) typeExprNode() {}
func (*ListType) typeExprNode()  {}
func (*ArrayType) typeExprNode() {}

func (n *NamedType) Pos() int { return n.Line }
func (n *ListType) Pos() int  { return n.Line }
func (n *ArrayType) Pos() int { return n.Line }

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
// parameter list (amifl-spec.md section 8.1). Step 5 restricts Type to a
// plain scalar type name — a parameter typed `fn(...) -> R` (a function
// value passed as an argument, i.e. a higher-order function) is not yet
// supported, a deliberate scope cut documented in CLAUDE.md's "確定した
// 設計判断" for step 5 (no surface syntax exists yet to write a Func-type
// annotation at all; see ClosureLit).
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
}

// CallExpr is a function call `callee(args...)` (amifl-spec.md section 8).
// Callee is always a bare name — never an arbitrary expression — resolved
// by sema to exactly one of: the built-in `print` (Callee == "print",
// handled as its own special case throughout, unchanged since step 1 —
// the general built-in function library arrives in step 11), a top-level
// `fn`, or a local closure-valued variable (CalleeToken set; a local
// binding shadows a same-named top-level `fn`, mirroring how a `let`
// already shadows a top-level `const` — see sema's resolveCallExpr).
// AmiFL has no syntax for calling the result of an arbitrary expression
// (`(fn(x: Int) -> Int { x })(5)` isn't reachable — parseIdentOrCall only
// ever produces a CallExpr from a bare identifier), so Callee never needs
// to generalize beyond a name.
type CallExpr struct {
	Callee string
	Args   []Expr
	Line   int

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
}

// ClosureLit is `fn(params) -> R { body }` used as an expression — a
// local, unnamed function value (amifl-spec.md section 8.1, "let square =
// fn(x: Int) -> Int { x * x }"). Unlike a top-level FuncDecl, a
// ClosureLit's own Params/ReturnType are always themselves plain scalar
// types (see Param) and, per step 5's scope, a ClosureLit is only legal
// as a `let`'s direct value — never a call argument, an if/while
// condition, a binary operand, or any other position (sema's
// resolveType's default *ast.ClosureLit case rejects it there with a
// clear message; resolveLetExpr is the sole place that recognizes and
// accepts one). This is what "no first-class function values beyond a
// `let`, no higher-order functions yet" amounts to concretely — a
// deliberate, documented step-5 scope cut (CLAUDE.md's "確定した設計判断"),
// not an oversight; revisit once actually needed. A `let` binding a
// ClosureLit may not carry its own type annotation either (the closure's
// signature is always fully explicit already, so an annotation would be
// redundant — and step 5 has no `fn(...) -> R` type-annotation grammar to
// write one in even if it wanted to).
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
// section 2.2/8.4). Every one of the struct's declared fields must be
// given exactly once; order doesn't matter (each is matched by name, not
// position) — sema's resolveStructLit checks both completeness and
// duplicates.
type StructLit struct {
	TypeName string
	Fields   []StructLitField
	Line     int

	ResolvedType string // filled by sema: always == TypeName (struct types aren't aliased)
}

// StructLitField is one `name: value` entry inside a StructLit.
type StructLitField struct {
	Name  string
	Value Expr
	Line  int
}

// FieldExpr is postfix `target.field` (amifl-spec.md section 3.2): tuple
// index sugar (`t.0`, `t.1`, ...) when Target's type is a Tuple, or
// ordinary struct field access when it's a struct — the same syntax and
// the same AST node either way, since both ultimately compile to one
// AMIVM instruction (FGET). Field is exactly the text the user wrote ("0",
// "1", ... or a struct field name); AmivmField is what codegen actually
// emits after FGET's `>` prefix — sema computes it once (a synthesized
// "F0"/"F1"/... for a tuple index, since Go struct fields can't be named
// with a bare digit, or Field verbatim for a struct) so codegen never has
// to re-derive tuple-vs-struct from ResolvedType (whose two encodings
// codegen has no vocabulary to tell apart — see types.go's doc comment on
// why that encoding is sema-internal). This mirrors LetExpr.Token/
// CallExpr.CalleeToken: sema resolves the AMIVM-facing detail once, onto
// the node, and codegen just reads it.
type FieldExpr struct {
	Target Expr
	Field  string
	Line   int

	ResolvedType string // filled by sema
	AmivmField   string // filled by sema
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

// ForExpr is `for x in items { ... }` (amifl-spec.md section 7): always
// Unit-typed, side-effect-only. The `yield` form (a `map` pipeline sugar,
// amifl-spec.md section 7) is step 9's job — this node has no field for
// it, since step 7 never produces one. Items must be a List[T] or
// Array[T;N] (the only iterable collection types that exist yet); break/
// continue inside Body act on this loop exactly like WhileExpr's (same
// loopDepth bookkeeping, never crossing a closure boundary).
type ForExpr struct {
	Var   string
	Items Expr
	Body  *Block
	Line  int

	// filled by sema:
	ElemType string // Items' element type — also Var's own type
	// VarToken is Var's AMIVM value token (e.g. "%x_7"), minted once via
	// freshInternalName exactly like LetExpr.Token — Var is a non-
	// reassignable binding (binding.reassignable stays false), mirroring
	// a function/closure parameter rather than a `let` (amifl-spec.md is
	// silent on whether a for-loop variable may be reassigned; step 5's
	// same silence about parameters was resolved the same conservative
	// way, for the same "明示性 > 簡潔さ" reasoning).
	VarToken string
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
type IntLit struct {
	Value uint64
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
func (*IndexExpr) exprNode()       {}
func (*IndexAssignExpr) exprNode() {}
func (*SliceExpr) exprNode()       {}
func (*ForExpr) exprNode()         {}

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
func (n *IndexExpr) Pos() int       { return n.Line }
func (n *IndexAssignExpr) Pos() int { return n.Line }
func (n *SliceExpr) Pos() int       { return n.Line }
func (n *ForExpr) Pos() int         { return n.Line }
