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

// parser is a hand-written recursive-descent parser (amifl-spec.md's
// operator-precedence table lands with expressions in step 3; step 1's
// grammar is small enough that parseExpr doesn't need it yet).
type parser struct {
	lx  *lexer.Lexer
	cur lexer.Token
}

func (p *parser) advance() error {
	tok, err := p.lx.Next()
	if err != nil {
		return err
	}
	p.cur = tok
	return nil
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
		fn, err := p.parseFuncDecl()
		if err != nil {
			return nil, err
		}
		f.Funcs = append(f.Funcs, fn)
		if err := p.skipNewlines(); err != nil {
			return nil, err
		}
	}
	return f, nil
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

func (p *parser) parseExpr() (ast.Expr, error) {
	switch p.cur.Kind {
	case lexer.String:
		tok := p.cur
		if err := p.advance(); err != nil {
			return nil, err
		}
		return &ast.StringLit{Value: tok.Value, Line: tok.Line}, nil
	case lexer.Int:
		tok := p.cur
		n, err := strconv.ParseInt(tok.Value, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("line %d: invalid integer literal %q: %w", tok.Line, tok.Value, err)
		}
		if err := p.advance(); err != nil {
			return nil, err
		}
		return &ast.IntLit{Value: n, Line: tok.Line}, nil
	case lexer.Ident:
		return p.parseCallExpr()
	default:
		return nil, p.errorf("unexpected %s, expected an expression", p.cur.Kind)
	}
}

func (p *parser) parseCallExpr() (ast.Expr, error) {
	nameTok, err := p.expect(lexer.Ident)
	if err != nil {
		return nil, err
	}
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
