package ast

// File is a parsed AmiFL source file.
type File struct {
	Funcs []*FuncDecl
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

// Block is a `{ ... }` body: a newline-separated sequence of expressions.
// AmiFL has no separate statement grammar (amifl-spec.md section 5) —
// a program is nothing but a sequence of expressions.
type Block struct {
	Exprs []Expr
}

// Expr is any AmiFL expression node.
type Expr interface {
	exprNode()
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

// IntLit is an integer literal.
type IntLit struct {
	Value int64
	Line  int
}

func (*CallExpr) exprNode()  {}
func (*StringLit) exprNode() {}
func (*IntLit) exprNode()    {}
