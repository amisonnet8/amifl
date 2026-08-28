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
// climbing for amifl-spec.md section 6's binary/unary operators. Single-
// token lookahead (cur only) suffices everywhere, including reassignment
// (`name = expr`, `x[i] = v`) — see parseExpr's doc comment for why that
// no longer needs a peek-ahead buffer (step 7 removed the one earlier
// steps used).
type parser struct {
	lx  *lexer.Lexer
	cur lexer.Token
	// noCompositeLit suppresses treating `Ident '{'` as the start of a
	// struct literal (ast.StructLit) — set only while parsing an if/elif/
	// while condition, the one position a bare `{` is genuinely ambiguous
	// between "start of a struct literal" and "start of the following
	// block" (Go has the identical ambiguity and resolves it the same way:
	// disallow an unparenthesized composite literal in a condition, still
	// allow one wrapped in parens or nested inside any other delimiter —
	// call args, a struct literal's own field values, a tuple literal's
	// elements, since all of those already have an unambiguous closing
	// token of their own). Every place that enters such an unambiguous
	// context (the LParen branch of parsePrimaryExpr, parseCallArgs,
	// parseStructLit) saves and clears this around its own parsing, so
	// nesting composes correctly with no stack needed beyond the call
	// stack itself.
	noCompositeLit bool
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
	case lexer.KwStruct:
		return p.parseStructDecl()
	case lexer.KwEnum:
		return p.parseEnumDecl()
	default:
		return nil, p.errorf("expected 'fn', 'const', 'struct', or 'enum' at top level, got %s", p.cur.Kind)
	}
}

// parseEnumDecl parses `enum Name { Variant1 [(field1: Type1, ...)] ... }`
// (amifl-spec.md section 2.2) — step 8. Variants are newline-separated
// (never comma-separated), one per line, mirroring parseStructDecl's field
// layout and the spec's own formatting. Each variant reuses
// parseParamList for its optional field list — identical grammar to a
// `fn`'s or a struct's own field list (`name: Type` pairs).
func (p *parser) parseEnumDecl() (*ast.EnumDecl, error) {
	kwTok, err := p.expect(lexer.KwEnum)
	if err != nil {
		return nil, err
	}
	nameTok, err := p.expect(lexer.Ident)
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.LBrace); err != nil {
		return nil, err
	}
	if err := p.skipNewlines(); err != nil {
		return nil, err
	}
	var variants []ast.EnumVariant
	for p.cur.Kind != lexer.RBrace {
		variantTok, err := p.expect(lexer.Ident)
		if err != nil {
			return nil, err
		}
		var fields []ast.Param
		if p.cur.Kind == lexer.LParen {
			if err := p.advance(); err != nil {
				return nil, err
			}
			fields, err = p.parseParamList()
			if err != nil {
				return nil, err
			}
		}
		variants = append(variants, ast.EnumVariant{Name: variantTok.Value, Fields: fields, Line: variantTok.Line})
		if p.cur.Kind == lexer.RBrace {
			break
		}
		if p.cur.Kind != lexer.Newline {
			return nil, p.errorf("expected newline after enum variant, got %s", p.cur.Kind)
		}
		if err := p.skipNewlines(); err != nil {
			return nil, err
		}
	}
	if _, err := p.expect(lexer.RBrace); err != nil {
		return nil, err
	}
	if len(variants) == 0 {
		return nil, fmt.Errorf("line %d: enum %q must declare at least one variant", kwTok.Line, nameTok.Value)
	}
	return &ast.EnumDecl{Name: nameTok.Value, Variants: variants, Line: kwTok.Line}, nil
}

// parseStructDecl parses `struct Name { field1: Type1, field2: Type2, ... }`
// (amifl-spec.md section 2.2) — a field list with the exact same grammar
// as parseParamList (comma-separated `name: Type`, single-line, no field
// defaults), reused as ast.Param entries since a struct field and a
// function parameter share every attribute that matters here.
func (p *parser) parseStructDecl() (*ast.StructDecl, error) {
	kwTok, err := p.expect(lexer.KwStruct)
	if err != nil {
		return nil, err
	}
	nameTok, err := p.expect(lexer.Ident)
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.LBrace); err != nil {
		return nil, err
	}
	fields, err := p.parseFieldTypeList(lexer.RBrace)
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.RBrace); err != nil {
		return nil, err
	}
	return &ast.StructDecl{Name: nameTok.Value, Fields: fields, Line: kwTok.Line}, nil
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
	params, err := p.parseParamList()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.Arrow); err != nil {
		return nil, err
	}
	retType, err := p.parseTypeExpr()
	if err != nil {
		return nil, err
	}
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	return &ast.FuncDecl{Name: nameTok.Value, Params: params, ReturnType: retType, Body: body, Line: nameTok.Line}, nil
}

// parseTypeExpr parses a type annotation: a plain name (a scalar or
// struct type, amifl-spec.md sections 2.1/2.2), or one of step 7's two
// bracket-generic collection types, `List[Elem]` and `Array[Elem;N1,N2,
// ...]` (section 2.2). "List" and "Array" are recognized structurally
// here, by comparing the leading identifier's text, rather than being
// reserved keywords — exactly like "Unit" is only special in a return-type
// position (sema's canonicalReturnType) without being a keyword anywhere
// else. A variable can still be named "List" or "Array" without conflict,
// since this function is only ever reached from a type-annotation
// position.
func (p *parser) parseTypeExpr() (ast.TypeExpr, error) {
	nameTok, err := p.expect(lexer.Ident)
	if err != nil {
		return nil, err
	}
	switch nameTok.Value {
	case "List":
		return p.parseListType(nameTok)
	case "Array":
		return p.parseArrayType(nameTok)
	default:
		return &ast.NamedType{Name: nameTok.Value, Line: nameTok.Line}, nil
	}
}

func (p *parser) parseListType(nameTok lexer.Token) (ast.TypeExpr, error) {
	if _, err := p.expect(lexer.LBracket); err != nil {
		return nil, err
	}
	elem, err := p.parseTypeExpr()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.RBracket); err != nil {
		return nil, err
	}
	return &ast.ListType{Elem: elem, Line: nameTok.Line}, nil
}

// parseArrayType parses `Array[Elem;N1,N2,...]`, desugaring the
// multi-dimension size list into nested ast.ArrayType values at parse
// time (amifl-spec.md section 2.2's own stated equivalence) — see
// ast.ArrayType's doc comment for why every later phase only ever
// handles a single dimension.
func (p *parser) parseArrayType(nameTok lexer.Token) (ast.TypeExpr, error) {
	if _, err := p.expect(lexer.LBracket); err != nil {
		return nil, err
	}
	elem, err := p.parseTypeExpr()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.Semicolon); err != nil {
		return nil, err
	}
	var sizes []ast.Expr
	for {
		size, err := p.parseOrExpr()
		if err != nil {
			return nil, err
		}
		sizes = append(sizes, size)
		if p.cur.Kind != lexer.Comma {
			break
		}
		if err := p.advance(); err != nil {
			return nil, err
		}
	}
	if _, err := p.expect(lexer.RBracket); err != nil {
		return nil, err
	}
	// Array[T;N1,N2,...] ≡ Array[Array[...Array[T;Nk]...];N2];N1] — build
	// from the innermost (last) size outward so the outermost dimension
	// (N1) ends up as the returned node's own Size.
	result := elem
	for i := len(sizes) - 1; i >= 0; i-- {
		result = &ast.ArrayType{Elem: result, Size: sizes[i], Line: nameTok.Line}
	}
	return result, nil
}

// parseParamList parses a `fn`/closure-literal parameter list's contents
// (everything between an already-consumed `(` and the terminating `)`,
// which this also consumes): zero or more comma-separated `name: Type`
// entries. Shared verbatim between parseFuncDecl and parseClosureLit —
// amifl-spec.md section 8.1's grammar for the two is identical. Step 5
// restricts Type to a plain scalar identifier (see ast.Param's doc
// comment) — a `fn(...) -> R` type here (a function-valued parameter)
// isn't supported yet, so this never recurses into fn-type parsing the
// way a hypothetical parseType would.
func (p *parser) parseParamList() ([]ast.Param, error) {
	fields, err := p.parseFieldTypeList(lexer.RParen)
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.RParen); err != nil {
		return nil, err
	}
	return fields, nil
}

// parseFieldTypeList parses zero or more comma-separated `name: Type`
// entries up to (but not consuming) end — the shared grammar behind both
// a parameter list (parseParamList, terminated by `)`) and a `struct`
// field list (parseStructDecl, terminated by `}`). Newlines are skipped
// after the opening delimiter, after each comma, and before end, so a
// struct's fields (unlike a `fn`'s, which are normally short enough to fit
// one line) can be laid out one per line — the natural style once a
// struct has more than a couple of fields, and how amifl-spec.md itself
// formats `enum`'s analogous variant list (section 2.2).
func (p *parser) parseFieldTypeList(end lexer.Kind) ([]ast.Param, error) {
	if err := p.skipNewlines(); err != nil {
		return nil, err
	}
	var fields []ast.Param
	if p.cur.Kind != end {
		for {
			nameTok, err := p.expect(lexer.Ident)
			if err != nil {
				return nil, err
			}
			if _, err := p.expect(lexer.Colon); err != nil {
				return nil, err
			}
			typ, err := p.parseTypeExpr()
			if err != nil {
				return nil, err
			}
			fields = append(fields, ast.Param{Name: nameTok.Value, Type: typ, Line: nameTok.Line})
			if p.cur.Kind != lexer.Comma {
				break
			}
			if err := p.advance(); err != nil {
				return nil, err
			}
			if err := p.skipNewlines(); err != nil {
				return nil, err
			}
		}
		if err := p.skipNewlines(); err != nil {
			return nil, err
		}
	}
	return fields, nil
}

// parseClosureLit parses `fn(params) -> R { body }` as an expression
// (amifl-spec.md section 8.1) — reachable only from parsePrimaryExpr,
// never from parseTopLevelDecl's statement-position `fn`, so there's no
// ambiguity between the two despite sharing the KwFn keyword.
func (p *parser) parseClosureLit() (ast.Expr, error) {
	kwTok, err := p.expect(lexer.KwFn)
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.LParen); err != nil {
		return nil, err
	}
	params, err := p.parseParamList()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.Arrow); err != nil {
		return nil, err
	}
	retType, err := p.parseTypeExpr()
	if err != nil {
		return nil, err
	}
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	return &ast.ClosureLit{Params: params, ReturnType: retType, Body: body, Line: kwTok.Line}, nil
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

// parseOptionalTypeAnnotation parses a leading `: Type`, or returns nil if
// there is none.
func (p *parser) parseOptionalTypeAnnotation() (ast.TypeExpr, error) {
	if p.cur.Kind != lexer.Colon {
		return nil, nil
	}
	if err := p.advance(); err != nil {
		return nil, err
	}
	return p.parseTypeExpr()
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
// (amifl-spec.md section 5). Reassignment (`name = expr`, `x[i] = v`) is
// deliberately not reachable from within parseOrExpr's operator chain —
// it isn't listed among amifl-spec.md section 6's operators, and (like
// let/const/discard) it's Unit-typed, so nesting it inside a larger
// expression would only ever be legal in a position that itself has to be
// Unit-typed. Detecting it no longer needs a peek past the target (step
// 2's original approach, sufficient when the only assignable target was a
// bare name): step 7 adds `x[i] = v`, a target that isn't just one token,
// so this instead parses the target as an ordinary expression first and
// reclassifies it if `=` follows — the same technique Go itself uses for
// its own assignment statements.
func (p *parser) parseExpr() (ast.Expr, error) {
	switch p.cur.Kind {
	case lexer.KwLet:
		return p.parseLetExpr()
	case lexer.KwConst:
		return p.parseConstDecl()
	case lexer.Ident:
		if p.cur.Value == "_" {
			return p.parseDiscardExpr()
		}
	}
	expr, err := p.parseOrExpr()
	if err != nil {
		return nil, err
	}
	if p.cur.Kind != lexer.Assign {
		return expr, nil
	}
	return p.finishAssignExpr(expr)
}

// finishAssignExpr consumes the `=` parseExpr just found after target and
// builds the appropriate assignment node. Only a bare identifier
// (ast.AssignExpr) or an index expression (ast.IndexAssignExpr) are valid
// targets — a field (`t.x = v`) remains unsupported (step 6's deliberate
// scope cut, ast.FieldExpr's doc comment), and anything else (a call, a
// literal, a binary expression, ...) is simply not assignable.
func (p *parser) finishAssignExpr(target ast.Expr) (ast.Expr, error) {
	eqLine := p.cur.Line
	if err := p.advance(); err != nil { // consume '='
		return nil, err
	}
	value, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	switch t := target.(type) {
	case *ast.IdentExpr:
		return &ast.AssignExpr{Name: t.Name, Value: value, Line: t.Line}, nil
	case *ast.IndexExpr:
		return &ast.IndexAssignExpr{Target: t.Target, Index: t.Index, Value: value, Line: t.Line}, nil
	default:
		return nil, fmt.Errorf("line %d: invalid assignment target", eqLine)
	}
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
		return p.parsePostfixExpr()
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

// parsePostfixExpr parses a primary expression followed by zero or more
// `.field` accesses (amifl-spec.md section 3.2: tuple index sugar `t.0`,
// `t.1`, ... and ordinary struct field access, both the same ast.FieldExpr
// node — see its doc comment) and/or `[...]` index/slice accesses (section
// 3.2's `x[i]`/`x[a:b]`, step 7 — see parseIndexOrSlice) — the highest-
// precedence level in section 6's table (`() . [] 関数呼び出し 後置?`),
// above unary. Call parsing (`f(...)`) already happens one level down
// inside parsePrimaryExpr (parseIdentOrCall), so a chain like
// `f(x).0[i].y` composes for free: each `.field`/`[...]` simply wraps
// whatever came before it.
func (p *parser) parsePostfixExpr() (ast.Expr, error) {
	expr, err := p.parsePrimaryExpr()
	if err != nil {
		return nil, err
	}
	for {
		switch p.cur.Kind {
		case lexer.Dot:
			dotLine := p.cur.Line
			if err := p.advance(); err != nil {
				return nil, err
			}
			var field string
			switch p.cur.Kind {
			case lexer.Int, lexer.Ident:
				field = p.cur.Value
			default:
				return nil, p.errorf("expected a field name or tuple index after '.', got %s", p.cur.Kind)
			}
			if err := p.advance(); err != nil {
				return nil, err
			}
			var args []ast.StructLitField
			if p.cur.Kind == lexer.LParen {
				args, err = p.parseEnumVariantArgs()
				if err != nil {
					return nil, err
				}
			}
			expr = &ast.FieldExpr{Target: expr, Field: field, Args: args, Line: dotLine}
		case lexer.LBracket:
			expr, err = p.parseIndexOrSlice(expr)
			if err != nil {
				return nil, err
			}
		default:
			return expr, nil
		}
	}
}

// parseEnumVariantArgs parses `(field1: v1, field2: v2, ...)` right after a
// postfix `.Variant` (amifl-spec.md section 2.2, "Status.Retry(delay: 5)")
// — the same named-field convention parseStructLit uses, reusing
// ast.StructLitField for each entry. Always returns a non-nil slice (empty
// for `()`), which is what tells FieldExpr.Args apart from a plain
// `.field` access with no trailing call at all (nil) — see FieldExpr's doc
// comment.
func (p *parser) parseEnumVariantArgs() ([]ast.StructLitField, error) {
	if _, err := p.expect(lexer.LParen); err != nil {
		return nil, err
	}
	saved := p.noCompositeLit
	p.noCompositeLit = false
	defer func() { p.noCompositeLit = saved }()

	args := []ast.StructLitField{}
	if p.cur.Kind != lexer.RParen {
		for {
			fieldNameTok, err := p.expect(lexer.Ident)
			if err != nil {
				return nil, err
			}
			if _, err := p.expect(lexer.Colon); err != nil {
				return nil, err
			}
			val, err := p.parseOrExpr()
			if err != nil {
				return nil, err
			}
			args = append(args, ast.StructLitField{Name: fieldNameTok.Value, Value: val, Line: fieldNameTok.Line})
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
	return args, nil
}

// parseIndexOrSlice parses `[...]` following an already-parsed target
// (amifl-spec.md section 3.2): `target[i]` (ast.IndexExpr), or
// `target[a:b]` / `target[a:]` / `target[:b]` / `target[:]`
// (ast.SliceExpr, From/To nil when omitted) — told apart by whether a `:`
// shows up before the closing `]`.
func (p *parser) parseIndexOrSlice(target ast.Expr) (ast.Expr, error) {
	openTok, err := p.expect(lexer.LBracket)
	if err != nil {
		return nil, err
	}
	saved := p.noCompositeLit
	p.noCompositeLit = false
	defer func() { p.noCompositeLit = saved }()

	var from ast.Expr
	if p.cur.Kind != lexer.Colon {
		from, err = p.parseOrExpr()
		if err != nil {
			return nil, err
		}
	}
	if p.cur.Kind == lexer.Colon {
		if err := p.advance(); err != nil {
			return nil, err
		}
		to, err := p.parseOptionalSliceBound()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(lexer.RBracket); err != nil {
			return nil, err
		}
		return &ast.SliceExpr{Target: target, From: from, To: to, Line: openTok.Line}, nil
	}
	if _, err := p.expect(lexer.RBracket); err != nil {
		return nil, err
	}
	return &ast.IndexExpr{Target: target, Index: from, Line: openTok.Line}, nil
}

// parseOptionalSliceBound parses the (possibly absent) expression right
// before a slice's closing `]` — absent exactly when `]` comes next.
func (p *parser) parseOptionalSliceBound() (ast.Expr, error) {
	if p.cur.Kind == lexer.RBracket {
		return nil, nil
	}
	return p.parseOrExpr()
}

// parseListLit parses `[v1, v2, ...]` (amifl-spec.md sections 2.2/3.1) —
// the literal syntax shared by both List[T] and Array[T;N]; which one it
// resolves to is entirely sema's job (see ast.ListLit's doc comment).
func (p *parser) parseListLit() (ast.Expr, error) {
	openTok, err := p.expect(lexer.LBracket)
	if err != nil {
		return nil, err
	}
	saved := p.noCompositeLit
	p.noCompositeLit = false
	defer func() { p.noCompositeLit = saved }()

	var elems []ast.Expr
	if p.cur.Kind != lexer.RBracket {
		for {
			elem, err := p.parseOrExpr()
			if err != nil {
				return nil, err
			}
			elems = append(elems, elem)
			if p.cur.Kind != lexer.Comma {
				break
			}
			if err := p.advance(); err != nil {
				return nil, err
			}
		}
	}
	if _, err := p.expect(lexer.RBracket); err != nil {
		return nil, err
	}
	return &ast.ListLit{Elems: elems, Line: openTok.Line}, nil
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
		return p.parseParenOrTupleExpr()
	case lexer.LBracket:
		return p.parseListLit()
	case lexer.KwIf:
		return p.parseIfExpr()
	case lexer.KwSwitch:
		return p.parseSwitchExpr()
	case lexer.KwWhile:
		return p.parseWhileExpr()
	case lexer.KwFor:
		return p.parseForExpr()
	case lexer.KwBreak:
		tok := p.cur
		if err := p.advance(); err != nil {
			return nil, err
		}
		return &ast.BreakExpr{Line: tok.Line}, nil
	case lexer.KwContinue:
		tok := p.cur
		if err := p.advance(); err != nil {
			return nil, err
		}
		return &ast.ContinueExpr{Line: tok.Line}, nil
	case lexer.KwFn:
		return p.parseClosureLit()
	default:
		return nil, p.errorf("unexpected %s, expected an expression", p.cur.Kind)
	}
}

// parseHeaderExpr parses the "header expression" of an if/elif/while
// condition or a `for`'s `items` (amifl-spec.md section 7): parseOrExpr
// (never parseExpr — a header deliberately stays at "any operator
// expression" and never reaches statement-only forms: let/const/assign/
// discard), with noCompositeLit set so a bare `Ident '{'` right at the end
// is left for the following block to consume rather than being swallowed
// as a struct literal — see noCompositeLit's doc comment on the parser
// struct for the ambiguity this resolves (identical to Go's own "no
// composite literal in an if/for header" rule, and for the same reason —
// `for x in items { ... }` has the exact same "bare identifier directly
// followed by the body's `{`" shape an if/while condition does).
func (p *parser) parseHeaderExpr() (ast.Expr, error) {
	saved := p.noCompositeLit
	p.noCompositeLit = true
	defer func() { p.noCompositeLit = saved }()
	return p.parseOrExpr()
}

// parseIfExpr parses `if cond { ... } [elif cond { ... }]* [else { ... }]?`
// (amifl-spec.md section 7).
//
// `elif`/`else` must directly follow the previous branch's closing `}` on
// the same line — parseOptionalElse never skips a Newline to look for one
// (CLAUDE.md's "確定した設計判断": elif/else must be "cuddled", mirroring
// how the example in amifl-spec.md section 5 writes the whole chain on one
// line). Writing `elif`/`else` on its own line leaves it as a dangling
// token that statement-position parsing rejects with a clear "unexpected
// 'elif'" error, rather than silently accepting two different layouts.
func (p *parser) parseIfExpr() (ast.Expr, error) {
	kwTok, err := p.expect(lexer.KwIf)
	if err != nil {
		return nil, err
	}
	cond, err := p.parseHeaderExpr()
	if err != nil {
		return nil, err
	}
	then, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	elseBody, err := p.parseOptionalElse()
	if err != nil {
		return nil, err
	}
	return &ast.IfExpr{Cond: cond, Then: then, Else: elseBody, Line: kwTok.Line}, nil
}

func (p *parser) parseOptionalElse() (ast.ElseBody, error) {
	switch p.cur.Kind {
	case lexer.KwElif:
		elifTok := p.cur
		if err := p.advance(); err != nil {
			return nil, err
		}
		cond, err := p.parseHeaderExpr()
		if err != nil {
			return nil, err
		}
		then, err := p.parseBlock()
		if err != nil {
			return nil, err
		}
		nested, err := p.parseOptionalElse()
		if err != nil {
			return nil, err
		}
		return &ast.IfExpr{Cond: cond, Then: then, Else: nested, Line: elifTok.Line}, nil
	case lexer.KwElse:
		if err := p.advance(); err != nil {
			return nil, err
		}
		block, err := p.parseBlock()
		if err != nil {
			return nil, err
		}
		return block, nil
	default:
		return nil, nil
	}
}

func (p *parser) parseWhileExpr() (ast.Expr, error) {
	kwTok, err := p.expect(lexer.KwWhile)
	if err != nil {
		return nil, err
	}
	cond, err := p.parseHeaderExpr()
	if err != nil {
		return nil, err
	}
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	return &ast.WhileExpr{Cond: cond, Body: body, Line: kwTok.Line}, nil
}

// parseForExpr parses `for x in items { ... }` (amifl-spec.md section 7).
// The `yield` form (a `map` pipeline sugar) is step 9's job — not
// reachable here yet, so Items always goes through the same Unit-typed,
// side-effect-only body every other `for` in step 7's scope does.
func (p *parser) parseForExpr() (ast.Expr, error) {
	kwTok, err := p.expect(lexer.KwFor)
	if err != nil {
		return nil, err
	}
	varTok, err := p.expect(lexer.Ident)
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.KwIn); err != nil {
		return nil, err
	}
	items, err := p.parseHeaderExpr()
	if err != nil {
		return nil, err
	}
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	return &ast.ForExpr{Var: varTok.Value, Items: items, Body: body, Line: kwTok.Line}, nil
}

// switchCase is one `case <bool-expr>: <value-expr>` or
// `default: <value-expr>` clause, collected before desugaring (see
// parseSwitchExpr).
type switchCase struct {
	cond ast.Expr // nil for `default`
	body ast.Expr
	line int
}

// parseSwitchExpr parses `switch` (amifl-spec.md section 10): either the
// step-4 subject-less form (every case a plain Bool expression — an elif
// chain with different keywords, principle 3: "1つの仕組みで足りるものを2つ
// 用意しない", desugared straight into an *ast.IfExpr with no AST node or
// sema/codegen support of its own) when `{` comes right after `switch`, or
// (step 8) the subject-carrying enum-pattern form otherwise — a real
// *ast.SwitchExpr, needed now that enum values (and their variant
// patterns) exist to match against. Telling the two apart needs only one
// token of lookahead (is `{` next, or a subject expression's own first
// token) — the subject-less form has never allowed anything between
// `switch` and `{`, so this is unambiguous.
func (p *parser) parseSwitchExpr() (ast.Expr, error) {
	kwTok, err := p.expect(lexer.KwSwitch)
	if err != nil {
		return nil, err
	}
	if p.cur.Kind == lexer.LBrace {
		return p.parseBoolSwitchExpr(kwTok)
	}
	subject, err := p.parseHeaderExpr()
	if err != nil {
		return nil, err
	}
	return p.parseEnumSwitchExpr(kwTok, subject)
}

// parseBoolSwitchExpr parses the step-4 subject-less `switch` form — see
// parseSwitchExpr's doc comment. kwTok is the already-consumed `switch`
// keyword token.
func (p *parser) parseBoolSwitchExpr(kwTok lexer.Token) (ast.Expr, error) {
	if _, err := p.expect(lexer.LBrace); err != nil {
		return nil, err
	}
	if err := p.skipNewlines(); err != nil {
		return nil, err
	}

	var cases []switchCase
	haveDefault := false
	for p.cur.Kind != lexer.RBrace {
		var c switchCase
		switch p.cur.Kind {
		case lexer.KwCase:
			caseTok := p.cur
			if err := p.advance(); err != nil {
				return nil, err
			}
			cond, err := p.parseOrExpr()
			if err != nil {
				return nil, err
			}
			if _, err := p.expect(lexer.Colon); err != nil {
				return nil, err
			}
			body, err := p.parseOrExpr()
			if err != nil {
				return nil, err
			}
			c = switchCase{cond: cond, body: body, line: caseTok.Line}
		case lexer.KwDefault:
			defTok := p.cur
			if haveDefault {
				return nil, p.errorf("duplicate 'default' in switch")
			}
			if err := p.advance(); err != nil {
				return nil, err
			}
			if _, err := p.expect(lexer.Colon); err != nil {
				return nil, err
			}
			body, err := p.parseOrExpr()
			if err != nil {
				return nil, err
			}
			haveDefault = true
			c = switchCase{cond: nil, body: body, line: defTok.Line}
		case lexer.EOF:
			return nil, p.errorf("unexpected end of file, expected '}'")
		default:
			return nil, p.errorf("expected 'case' or 'default' in switch, got %s", p.cur.Kind)
		}
		cases = append(cases, c)
		if p.cur.Kind == lexer.RBrace {
			break
		}
		if p.cur.Kind != lexer.Newline {
			return nil, p.errorf("expected newline after switch case, got %s", p.cur.Kind)
		}
		if err := p.skipNewlines(); err != nil {
			return nil, err
		}
	}
	if _, err := p.expect(lexer.RBrace); err != nil {
		return nil, err
	}
	if len(cases) == 0 || (len(cases) == 1 && cases[0].cond == nil) {
		return nil, p.errorf("switch must have at least one 'case'")
	}

	var elseBody ast.ElseBody
	if haveDefault && cases[len(cases)-1].cond == nil {
		last := cases[len(cases)-1]
		elseBody = &ast.Block{Exprs: []ast.Expr{last.body}}
		cases = cases[:len(cases)-1]
	}
	var result *ast.IfExpr
	for i := len(cases) - 1; i >= 0; i-- {
		c := cases[i]
		if c.cond == nil {
			return nil, fmt.Errorf("line %d: 'default' must be the last clause in a switch", c.line)
		}
		result = &ast.IfExpr{
			Cond: c.cond,
			Then: &ast.Block{Exprs: []ast.Expr{c.body}},
			Else: elseBody,
			Line: c.line,
		}
		elseBody = result
	}
	return result, nil
}

// parseEnumSwitchExpr parses step 8's subject-carrying `switch` form's body
// (amifl-spec.md section 10): `{` case+ [default] `}`, where every case
// pattern is `EnumType.Variant[(binding, ...)]` (parseSwitchCasePattern) —
// `is Type`/`in [...]` aren't recognized here (out of step 8's scope, see
// ast.SwitchExpr's doc comment). Unlike parseBoolSwitchExpr, this produces
// a real *ast.SwitchExpr rather than desugaring, since sema needs Subject
// and each case's own field bindings to type-check against.
func (p *parser) parseEnumSwitchExpr(kwTok lexer.Token, subject ast.Expr) (ast.Expr, error) {
	if _, err := p.expect(lexer.LBrace); err != nil {
		return nil, err
	}
	if err := p.skipNewlines(); err != nil {
		return nil, err
	}

	var cases []ast.SwitchCase
	var defaultBlock *ast.Block
	for p.cur.Kind != lexer.RBrace {
		switch p.cur.Kind {
		case lexer.KwCase:
			caseTok := p.cur
			if err := p.advance(); err != nil {
				return nil, err
			}
			enumName, variant, bindings, err := p.parseSwitchCasePattern()
			if err != nil {
				return nil, err
			}
			if _, err := p.expect(lexer.Colon); err != nil {
				return nil, err
			}
			body, err := p.parseOrExpr()
			if err != nil {
				return nil, err
			}
			cases = append(cases, ast.SwitchCase{
				EnumName: enumName,
				Variant:  variant,
				Bindings: bindings,
				Body:     &ast.Block{Exprs: []ast.Expr{body}},
				Line:     caseTok.Line,
			})
		case lexer.KwDefault:
			if defaultBlock != nil {
				return nil, p.errorf("duplicate 'default' in switch")
			}
			if err := p.advance(); err != nil {
				return nil, err
			}
			if _, err := p.expect(lexer.Colon); err != nil {
				return nil, err
			}
			body, err := p.parseOrExpr()
			if err != nil {
				return nil, err
			}
			defaultBlock = &ast.Block{Exprs: []ast.Expr{body}}
		case lexer.EOF:
			return nil, p.errorf("unexpected end of file, expected '}'")
		default:
			return nil, p.errorf("expected 'case' or 'default' in switch, got %s", p.cur.Kind)
		}
		if p.cur.Kind == lexer.RBrace {
			break
		}
		if p.cur.Kind != lexer.Newline {
			return nil, p.errorf("expected newline after switch case, got %s", p.cur.Kind)
		}
		if err := p.skipNewlines(); err != nil {
			return nil, err
		}
	}
	if _, err := p.expect(lexer.RBrace); err != nil {
		return nil, err
	}
	if len(cases) == 0 {
		return nil, p.errorf("switch must have at least one 'case'")
	}
	return &ast.SwitchExpr{Subject: subject, Cases: cases, Default: defaultBlock, Line: kwTok.Line}, nil
}

// parseSwitchCasePattern parses `EnumType.Variant` or
// `EnumType.Variant(binding1, binding2, ...)` (amifl-spec.md section 10) —
// a case pattern's own dedicated grammar, distinct from the general
// expression grammar's enum-variant *construction* syntax
// (parseEnumVariantArgs): a pattern's bindings are bare identifiers (not
// `name: value` pairs), positionally naming which local each field binds
// to — sema requires each one to equal the corresponding field's own
// declared name (ast.SwitchCase's doc comment).
func (p *parser) parseSwitchCasePattern() (enumName, variant string, bindings []string, err error) {
	enumTok, err := p.expect(lexer.Ident)
	if err != nil {
		return "", "", nil, err
	}
	if _, err := p.expect(lexer.Dot); err != nil {
		return "", "", nil, err
	}
	variantTok, err := p.expect(lexer.Ident)
	if err != nil {
		return "", "", nil, err
	}
	if p.cur.Kind != lexer.LParen {
		return enumTok.Value, variantTok.Value, nil, nil
	}
	if err := p.advance(); err != nil { // consume '('
		return "", "", nil, err
	}
	var bindingList []string
	if p.cur.Kind != lexer.RParen {
		for {
			bindTok, err := p.expect(lexer.Ident)
			if err != nil {
				return "", "", nil, err
			}
			bindingList = append(bindingList, bindTok.Value)
			if p.cur.Kind != lexer.Comma {
				break
			}
			if err := p.advance(); err != nil {
				return "", "", nil, err
			}
		}
	}
	if _, err := p.expect(lexer.RParen); err != nil {
		return "", "", nil, err
	}
	return enumTok.Value, variantTok.Value, bindingList, nil
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
// value: a call (`name(...)`), a struct literal (`Name{...}` — amifl-spec.md
// section 2.2, suppressed by noCompositeLit inside an if/elif/while
// condition, see its doc comment), or a bare variable read. Reassignment
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
	if p.cur.Kind == lexer.LBrace && !p.noCompositeLit {
		return p.parseStructLit(nameTok)
	}
	return &ast.IdentExpr{Name: nameTok.Value, Line: nameTok.Line}, nil
}

func (p *parser) parseCallArgs(nameTok lexer.Token) (ast.Expr, error) {
	if _, err := p.expect(lexer.LParen); err != nil {
		return nil, err
	}
	saved := p.noCompositeLit
	p.noCompositeLit = false
	defer func() { p.noCompositeLit = saved }()

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

// parseParenOrTupleExpr parses `(` already having been seen: either a
// parenthesized grouping `(expr)` (no comma at all — returned as just the
// inner expression, exactly as before step 6) or a tuple literal `(v1, v2,
// ...)` (amifl-spec.md section 2.2), told apart by whether a comma ever
// follows the first element. A trailing comma right after the first (and
// only) element — `(x,)` — produces a 1-element ast.TupleLit; sema rejects
// that arity (see TupleLit's doc comment for why the parser doesn't).
func (p *parser) parseParenOrTupleExpr() (ast.Expr, error) {
	openLine := p.cur.Line
	if err := p.advance(); err != nil { // consume '('
		return nil, err
	}
	saved := p.noCompositeLit
	p.noCompositeLit = false
	defer func() { p.noCompositeLit = saved }()

	first, err := p.parseOrExpr()
	if err != nil {
		return nil, err
	}
	elems := []ast.Expr{first}
	sawComma := false
	for p.cur.Kind == lexer.Comma {
		sawComma = true
		if err := p.advance(); err != nil {
			return nil, err
		}
		if p.cur.Kind == lexer.RParen {
			break
		}
		elem, err := p.parseOrExpr()
		if err != nil {
			return nil, err
		}
		elems = append(elems, elem)
	}
	if _, err := p.expect(lexer.RParen); err != nil {
		return nil, err
	}
	if !sawComma {
		return first, nil
	}
	return &ast.TupleLit{Elems: elems, Line: openLine}, nil
}

// parseStructLit parses `Name{field1: v1, field2: v2, ...}` — nameTok is
// the already-consumed type name, `{` is next.
func (p *parser) parseStructLit(nameTok lexer.Token) (ast.Expr, error) {
	if _, err := p.expect(lexer.LBrace); err != nil {
		return nil, err
	}
	saved := p.noCompositeLit
	p.noCompositeLit = false
	defer func() { p.noCompositeLit = saved }()

	var fields []ast.StructLitField
	if p.cur.Kind != lexer.RBrace {
		for {
			fieldNameTok, err := p.expect(lexer.Ident)
			if err != nil {
				return nil, err
			}
			if _, err := p.expect(lexer.Colon); err != nil {
				return nil, err
			}
			val, err := p.parseOrExpr()
			if err != nil {
				return nil, err
			}
			fields = append(fields, ast.StructLitField{Name: fieldNameTok.Value, Value: val, Line: fieldNameTok.Line})
			if p.cur.Kind != lexer.Comma {
				break
			}
			if err := p.advance(); err != nil {
				return nil, err
			}
		}
	}
	if _, err := p.expect(lexer.RBrace); err != nil {
		return nil, err
	}
	return &ast.StructLit{TypeName: nameTok.Value, Fields: fields, Line: nameTok.Line}, nil
}
