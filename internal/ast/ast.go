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

// IntLit is an integer literal. Value is uint64 rather than int64 because
// step 2 has no unary minus yet (arithmetic operators land in step 3), so
// every literal is non-negative — but representing UInt64's full range
// needs the extra bit int64 doesn't have.
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

func (*FuncDecl) topLevelDeclNode()  {}
func (*ConstDecl) topLevelDeclNode() {}

func (*ConstDecl) exprNode()   {}
func (*LetExpr) exprNode()     {}
func (*AssignExpr) exprNode()  {}
func (*DiscardExpr) exprNode() {}
func (*IdentExpr) exprNode()   {}
func (*CallExpr) exprNode()    {}
func (*StringLit) exprNode()   {}
func (*IntLit) exprNode()      {}
func (*FloatLit) exprNode()    {}
func (*BoolLit) exprNode()     {}

func (n *ConstDecl) Pos() int   { return n.Line }
func (n *LetExpr) Pos() int     { return n.Line }
func (n *AssignExpr) Pos() int  { return n.Line }
func (n *DiscardExpr) Pos() int { return n.Line }
func (n *IdentExpr) Pos() int   { return n.Line }
func (n *CallExpr) Pos() int    { return n.Line }
func (n *StringLit) Pos() int   { return n.Line }
func (n *IntLit) Pos() int      { return n.Line }
func (n *FloatLit) Pos() int    { return n.Line }
func (n *BoolLit) Pos() int     { return n.Line }
