package ast

// File is a parsed AmiFL source file: a sequence of top-level
// declarations.
type File struct {
	Decls []TopLevelDecl
}

// TopLevelDecl is a top-level declaration: *FuncDecl or *ConstDecl.
// AmiFL forbids top-level `let` (amifl-spec.md section 4, principle 5) —
// there is deliberately no *LetExpr case here, so the restriction is
// enforced by the grammar itself (internal/parser only recognizes `fn`
// and `const` at file scope) rather than a separate sema check.
type TopLevelDecl interface {
	topLevelDeclNode()
}

// FuncDecl is a top-level `fn` declaration.
//
// Step 1 only supports the parameter-less form `fn name() -> ReturnType {
// ... }` (amifl-spec.md section 14); parameters land in step 5.
type FuncDecl struct {
	Name       string
	ReturnType string
	Body       *Block
	Line       int
}

// ConstDecl is a `const` declaration (amifl-spec.md section 4): a
// compile-time-only binding that is inlined at every reference site
// rather than compiled to a runtime variable. It doubles as both a
// TopLevelDecl (top-level `const`, allowed unlike `let`) and an Expr
// (function-local `const`, usable anywhere a `let` can appear) — the two
// positions share identical rules, so one node type serves both.
type ConstDecl struct {
	Name string
	Type string // type annotation identifier, or "" if omitted (inferred)
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
	Type  string // type annotation identifier, or "" if omitted (inferred)
	Value Expr
	Line  int

	ResolvedType string // filled in by sema
	// InternalName is the Go variable name codegen emits for this
	// binding — Name suffixed with a function-wide unique counter (sema's
	// funcChecker.freshInternalName), never Name verbatim. See
	// CLAUDE.md's "確定した設計判断" for step 4: two `let`s named the same
	// (an outer one and a shadowing inner one, now that if/while bodies
	// get their own nested scope) would otherwise emit two Go variable
	// declarations with the *identical* generated name — legal Go
	// (genuine block shadowing), but it broke amivm's unused-variable
	// self-healing, which locates "the" declaration of a name assuming
	// there's only ever one per function.
	InternalName string
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

	InternalName string // filled in by sema; see LetExpr.InternalName
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
	// InternalName is set instead of ConstValue when Name resolves to a
	// `let` — see LetExpr.InternalName.
	InternalName string
}

// CallExpr is a function call `callee(args...)`.
type CallExpr struct {
	Callee string
	Args   []Expr
	Line   int
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

func (*FuncDecl) topLevelDeclNode()  {}
func (*ConstDecl) topLevelDeclNode() {}

func (*ConstDecl) exprNode()    {}
func (*LetExpr) exprNode()      {}
func (*AssignExpr) exprNode()   {}
func (*DiscardExpr) exprNode()  {}
func (*IdentExpr) exprNode()    {}
func (*CallExpr) exprNode()     {}
func (*StringLit) exprNode()    {}
func (*IntLit) exprNode()       {}
func (*FloatLit) exprNode()     {}
func (*BoolLit) exprNode()      {}
func (*BinaryExpr) exprNode()   {}
func (*UnaryExpr) exprNode()    {}
func (*IfExpr) exprNode()       {}
func (*WhileExpr) exprNode()    {}
func (*BreakExpr) exprNode()    {}
func (*ContinueExpr) exprNode() {}

func (n *ConstDecl) Pos() int    { return n.Line }
func (n *LetExpr) Pos() int      { return n.Line }
func (n *AssignExpr) Pos() int   { return n.Line }
func (n *DiscardExpr) Pos() int  { return n.Line }
func (n *IdentExpr) Pos() int    { return n.Line }
func (n *CallExpr) Pos() int     { return n.Line }
func (n *StringLit) Pos() int    { return n.Line }
func (n *IntLit) Pos() int       { return n.Line }
func (n *FloatLit) Pos() int     { return n.Line }
func (n *BoolLit) Pos() int      { return n.Line }
func (n *BinaryExpr) Pos() int   { return n.Line }
func (n *UnaryExpr) Pos() int    { return n.Line }
func (n *IfExpr) Pos() int       { return n.Line }
func (n *WhileExpr) Pos() int    { return n.Line }
func (n *BreakExpr) Pos() int    { return n.Line }
func (n *ContinueExpr) Pos() int { return n.Line }
