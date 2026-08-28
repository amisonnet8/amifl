package parser

import (
	"fmt"
	"strconv"

	"github.com/amisonnet8/amifl/internal/ast"
	"github.com/amisonnet8/amifl/internal/lexer"
)

// Parse tokenizes and parses a complete AmiFL source file.
func Parse(src string) (*ast.File, error) {
	p := &parser{lx: lexer.New(src)}
	if err := p.advance(); err != nil {
		return nil, err
	}
	return p.parseFile()
}

// parser is a hand-written recursive-descent parser using precedence
// climbing for amifl-spec.md section 6's binary/unary operators. It keeps
// one token of lookahead beyond cur (in ahead) so that, at statement
// position, an identifier can be told apart from a reassignment
// (`name = expr`) without having to commit to either parse before seeing
// the token after the name.
type parser struct {
	lx    *lexer.Lexer
	cur   lexer.Token
	ahead *lexer.Token
}

func (p *parser) advance() error {
	if p.ahead != nil {
		p.cur = *p.ahead
		p.ahead = nil
		return nil
	}
	tok, err := p.lx.Next()
	if err != nil {
		return err
	}
	p.cur = tok
	return nil
}

// peek returns the token after cur without consuming cur, caching it in
// ahead so the next advance() is free.
func (p *parser) peek() (lexer.Token, error) {
	if p.ahead == nil {
		tok, err := p.lx.Next()
		if err != nil {
			return lexer.Token{}, err
		}
		p.ahead = &tok
	}
	return *p.ahead, nil
}

func (p *parser) errorf(format string, args ...any) error {
	return fmt.Errorf("line %d: %s", p.cur.Line, fmt.Sprintf(format, args...))
}

func (p *parser) expect(k lexer.Kind) (lexer.Token, error) {
	if p.cur.Kind != k {
		return lexer.Token{}, p.errorf("expected %s, got %s", k, p.cur.Kind)
	}
	tok := p.cur
	if err := p.advance(); err != nil {
		return lexer.Token{}, err
	}
	return tok, nil
}

func (p *parser) skipNewlines() error {
	for p.cur.Kind == lexer.Newline {
		if err := p.advance(); err != nil {
			return err
		}
	}
	return nil
}

func (p *parser) parseFile() (*ast.File, error) {
	if err := p.skipNewlines(); err != nil {
		return nil, err
	}
	f := &ast.File{}
	for p.cur.Kind != lexer.EOF {
		decl, err := p.parseTopLevelDecl()
		if err != nil {
			return nil, err
		}
		f.Decls = append(f.Decls, decl)
		if err := p.skipNewlines(); err != nil {
			return nil, err
		}
	}
	return f, nil
}

// parseTopLevelDecl parses `fn` or `const` — the only two AmiFL
// declarations allowed at file scope. `let` is deliberately absent
// (amifl-spec.md principle 5); see ast.TopLevelDecl.
func (p *parser) parseTopLevelDecl() (ast.TopLevelDecl, error) {
	switch p.cur.Kind {
	case lexer.KwFn:
		return p.parseFuncDecl()
	case lexer.KwConst:
		return p.parseConstDecl()
	default:
		return nil, p.errorf("expected 'fn' or 'const' at top level, got %s", p.cur.Kind)
	}
}

func (p *parser) parseFuncDecl() (*ast.FuncDecl, error) {
	if _, err := p.expect(lexer.KwFn); err != nil {
		return nil, err
	}
	nameTok, err := p.expect(lexer.Ident)
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.LParen); err != nil {
		return nil, err
	}
	// Step 1 only supports parameter-less functions (amifl-spec.md section
	// 14's `fn main() -> Int { ... }` form); parameters land in step 5.
	if _, err := p.expect(lexer.RParen); err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.Arrow); err != nil {
		return nil, err
	}
	retTok, err := p.expect(lexer.Ident)
	if err != nil {
		return nil, err
	}
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	return &ast.FuncDecl{Name: nameTok.Value, ReturnType: retTok.Value, Body: body, Line: nameTok.Line}, nil
}

// parseConstDecl parses a `const name[: Type] = expr` declaration. The
// resulting node is used both as a TopLevelDecl and (when found inside a
// block by parseExpr) as an Expr — see ast.ConstDecl.
func (p *parser) parseConstDecl() (*ast.ConstDecl, error) {
	kwTok, err := p.expect(lexer.KwConst)
	if err != nil {
		return nil, err
	}
	nameTok, err := p.expect(lexer.Ident)
	if err != nil {
		return nil, err
	}
	typeName, err := p.parseOptionalTypeAnnotation()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.Assign); err != nil {
		return nil, err
	}
	value, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	return &ast.ConstDecl{Name: nameTok.Value, Type: typeName, Value: value, Line: kwTok.Line}, nil
}

// parseOptionalTypeAnnotation parses a leading `: TypeName`, or returns ""
// if there is none.
func (p *parser) parseOptionalTypeAnnotation() (string, error) {
	if p.cur.Kind != lexer.Colon {
		return "", nil
	}
	if err := p.advance(); err != nil {
		return "", err
	}
	tok, err := p.expect(lexer.Ident)
	if err != nil {
		return "", err
	}
	return tok.Value, nil
}

func (p *parser) parseBlock() (*ast.Block, error) {
	if _, err := p.expect(lexer.LBrace); err != nil {
		return nil, err
	}
	if err := p.skipNewlines(); err != nil {
		return nil, err
	}
	block := &ast.Block{}
	for p.cur.Kind != lexer.RBrace {
		if p.cur.Kind == lexer.EOF {
			return nil, p.errorf("unexpected end of file, expected '}'")
		}
		expr, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		block.Exprs = append(block.Exprs, expr)
		if p.cur.Kind == lexer.RBrace {
			break
		}
		if p.cur.Kind != lexer.Newline {
			return nil, p.errorf("expected newline after expression, got %s", p.cur.Kind)
		}
		if err := p.skipNewlines(); err != nil {
			return nil, err
		}
	}
	if _, err := p.expect(lexer.RBrace); err != nil {
		return nil, err
	}
	return block, nil
}

// parseExpr parses one expression in statement position: a block's
// top-level `let`/`const`/`_ = ...`/reassignment/value-expression entries
// (amifl-spec.md section 5). Reassignment (`name = expr`) is deliberately
// not reachable from within parseOrExpr's operator chain — it isn't listed
// among amifl-spec.md section 6's operators, and (like let/const/discard)
// it's Unit-typed, so nesting it inside a larger expression would only
// ever be legal in a position that itself has to be Unit-typed.
func (p *parser) parseExpr() (ast.Expr, error) {
	switch p.cur.Kind {
	case lexer.KwLet:
		return p.parseLetExpr()
	case lexer.KwConst:
		cd, err := p.parseConstDecl()
		if err != nil {
			return nil, err
		}
		return cd, nil
	case lexer.Ident:
		if p.cur.Value == "_" {
			return p.parseDiscardExpr()
		}
		nxt, err := p.peek()
		if err != nil {
			return nil, err
		}
		if nxt.Kind == lexer.Assign {
			return p.parseAssignExpr()
		}
	}
	return p.parseOrExpr()
}

func (p *parser) parseAssignExpr() (ast.Expr, error) {
	nameTok, err := p.expect(lexer.Ident)
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.Assign); err != nil {
		return nil, err
	}
	value, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	return &ast.AssignExpr{Name: nameTok.Value, Value: value, Line: nameTok.Line}, nil
}

// binaryLevel parses left-associative left (op right)* where op is any
// token found in ops, mapped to its AST operator text.
func (p *parser) binaryLevel(next func() (ast.Expr, error), ops map[lexer.Kind]string) (ast.Expr, error) {
	left, err := next()
	if err != nil {
		return nil, err
	}
	for {
		op, ok := ops[p.cur.Kind]
		if !ok {
			return left, nil
		}
		line := p.cur.Line
		if err := p.advance(); err != nil {
			return nil, err
		}
		right, err := next()
		if err != nil {
			return nil, err
		}
		left = &ast.BinaryExpr{Op: op, Left: left, Right: right, Line: line}
	}
}

// The chain implements amifl-spec.md section 6's precedence table
// (high to low): unary `! - ~` -> `* / % << >> & &^` -> `+ - | ^` ->
// `< <= > >=` -> `== !=` -> `&&` -> `||`. `|>` (lowest, step 9) and
// postfix `. [] ?` (highest, later steps) aren't reachable yet.
func (p *parser) parseOrExpr() (ast.Expr, error) {
	return p.binaryLevel(p.parseAndExpr, map[lexer.Kind]string{lexer.OrOr: "||"})
}

func (p *parser) parseAndExpr() (ast.Expr, error) {
	return p.binaryLevel(p.parseEqualityExpr, map[lexer.Kind]string{lexer.AndAnd: "&&"})
}

func (p *parser) parseEqualityExpr() (ast.Expr, error) {
	return p.binaryLevel(p.parseComparisonExpr, map[lexer.Kind]string{
		lexer.EqEq: "==", lexer.NotEq: "!=",
	})
}

func (p *parser) parseComparisonExpr() (ast.Expr, error) {
	return p.binaryLevel(p.parseAdditiveExpr, map[lexer.Kind]string{
		lexer.Lt: "<", lexer.Lte: "<=", lexer.Gt: ">", lexer.Gte: ">=",
	})
}

func (p *parser) parseAdditiveExpr() (ast.Expr, error) {
	return p.binaryLevel(p.parseMultiplicativeExpr, map[lexer.Kind]string{
		lexer.Plus: "+", lexer.Minus: "-", lexer.Pipe: "|", lexer.Caret: "^",
	})
}

func (p *parser) parseMultiplicativeExpr() (ast.Expr, error) {
	return p.binaryLevel(p.parseUnaryExpr, map[lexer.Kind]string{
		lexer.Star: "*", lexer.Slash: "/", lexer.Percent: "%",
		lexer.Shl: "<<", lexer.Shr: ">>", lexer.Amp: "&", lexer.AmpCaret: "&^",
	})
}

func (p *parser) parseUnaryExpr() (ast.Expr, error) {
	var op string
	switch p.cur.Kind {
	case lexer.Bang:
		op = "!"
	case lexer.Minus:
		op = "-"
	case lexer.Tilde:
		op = "~"
	default:
		return p.parsePrimaryExpr()
	}
	tok := p.cur
	if err := p.advance(); err != nil {
		return nil, err
	}
	operand, err := p.parseUnaryExpr()
	if err != nil {
		return nil, err
	}
	return &ast.UnaryExpr{Op: op, Operand: operand, Line: tok.Line}, nil
}

func (p *parser) parsePrimaryExpr() (ast.Expr, error) {
	switch p.cur.Kind {
	case lexer.KwTrue, lexer.KwFalse:
		tok := p.cur
		if err := p.advance(); err != nil {
			return nil, err
		}
		return &ast.BoolLit{Value: tok.Kind == lexer.KwTrue, Line: tok.Line}, nil
	case lexer.String:
		tok := p.cur
		if err := p.advance(); err != nil {
			return nil, err
		}
		return &ast.StringLit{Value: tok.Value, Line: tok.Line}, nil
	case lexer.Int:
		tok := p.cur
		n, err := strconv.ParseUint(tok.Value, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("line %d: invalid integer literal %q: %w", tok.Line, tok.Value, err)
		}
		if err := p.advance(); err != nil {
			return nil, err
		}
		return &ast.IntLit{Value: n, Line: tok.Line}, nil
	case lexer.Float:
		tok := p.cur
		fv, err := strconv.ParseFloat(tok.Value, 64)
		if err != nil {
			return nil, fmt.Errorf("line %d: invalid float literal %q: %w", tok.Line, tok.Value, err)
		}
		if err := p.advance(); err != nil {
			return nil, err
		}
		return &ast.FloatLit{Value: fv, Line: tok.Line}, nil
	case lexer.Ident:
		return p.parseIdentOrCall()
	case lexer.LParen:
		if err := p.advance(); err != nil {
			return nil, err
		}
		inner, err := p.parseOrExpr()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(lexer.RParen); err != nil {
			return nil, err
		}
		return inner, nil
	default:
		return nil, p.errorf("unexpected %s, expected an expression", p.cur.Kind)
	}
}

func (p *parser) parseLetExpr() (ast.Expr, error) {
	kwTok, err := p.expect(lexer.KwLet)
	if err != nil {
		return nil, err
	}
	nameTok, err := p.expect(lexer.Ident)
	if err != nil {
		return nil, err
	}
	typeName, err := p.parseOptionalTypeAnnotation()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.Assign); err != nil {
		return nil, err
	}
	value, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	return &ast.LetExpr{Name: nameTok.Value, Type: typeName, Value: value, Line: kwTok.Line}, nil
}

// parseDiscardExpr parses `_ = expr` (amifl-spec.md section 5). The `_`
// has already been confirmed to be the current token's text by the
// caller; it lexes as a plain Ident (isIdentStart accepts '_'), so there
// is no dedicated lexer token for it.
func (p *parser) parseDiscardExpr() (ast.Expr, error) {
	tok, err := p.expect(lexer.Ident) // consumes "_"
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.Assign); err != nil {
		return nil, err
	}
	value, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	return &ast.DiscardExpr{Value: value, Line: tok.Line}, nil
}

// parseIdentOrCall parses whatever follows a plain identifier used as a
// value: a call (`name(...)`) or a bare variable read. Reassignment
// (`name = expr`) is recognized earlier, at statement position in
// parseExpr, and never reaches here (see parseExpr's doc comment).
func (p *parser) parseIdentOrCall() (ast.Expr, error) {
	nameTok, err := p.expect(lexer.Ident)
	if err != nil {
		return nil, err
	}
	if p.cur.Kind == lexer.LParen {
		return p.parseCallArgs(nameTok)
	}
	return &ast.IdentExpr{Name: nameTok.Value, Line: nameTok.Line}, nil
}

func (p *parser) parseCallArgs(nameTok lexer.Token) (ast.Expr, error) {
	if _, err := p.expect(lexer.LParen); err != nil {
		return nil, err
	}
	var args []ast.Expr
	if p.cur.Kind != lexer.RParen {
		for {
			arg, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			args = append(args, arg)
			if p.cur.Kind != lexer.Comma {
				break
			}
			if err := p.advance(); err != nil {
				return nil, err
			}
		}
	}
	if _, err := p.expect(lexer.RParen); err != nil {
		return nil, err
	}
	return &ast.CallExpr{Callee: nameTok.Value, Args: args, Line: nameTok.Line}, nil
}
