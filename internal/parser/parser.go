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
	// buf holds tokens already pulled from lx but not yet made current —
	// only ever populated by peekPastNewlines (ex8, below), which needs to
	// look past a run of Newline tokens without destructively consuming them
	// (advance() has no undo). Ordinary parsing drains it transparently:
	// advance() always prefers buf over lx when both have tokens waiting.
	buf []lexer.Token
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
	if len(p.buf) > 0 {
		p.cur = p.buf[0]
		p.buf = p.buf[1:]
		return nil
	}
	tok, err := p.lx.Next()
	if err != nil {
		return err
	}
	p.cur = tok
	return nil
}

// peekPastNewlines returns the first non-Newline token following p.cur
// without consuming anything — if p.cur itself isn't a Newline, that's just
// p.cur. Any extra tokens read from the lexer while looking are buffered
// (p.buf) rather than discarded, so a later advance() still sees them in
// their original order; the buffer only ever grows here and only ever
// drains through the ordinary advance() path. Used exclusively by
// parsePipeExpr (ex8) to decide whether a run of newlines is followed by a
// `|>` — a pipe chain continuing onto the next line — without destructively
// swallowing those newlines when it turns out not to be (they must remain
// intact for parseBlock's statement-separator logic to consume normally).
func (p *parser) peekPastNewlines() (lexer.Token, error) {
	if p.cur.Kind != lexer.Newline {
		return p.cur, nil
	}
	i := 0
	for {
		var tok lexer.Token
		if i < len(p.buf) {
			tok = p.buf[i]
		} else {
			t, err := p.lx.Next()
			if err != nil {
				return lexer.Token{}, err
			}
			p.buf = append(p.buf, t)
			tok = t
		}
		if tok.Kind != lexer.Newline {
			return tok, nil
		}
		i++
	}
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

// parseCommaList parses a bracket-delimited, comma-separated list of items
// (ex8, amifl-spec.md section 5): the opening delimiter must already be
// consumed, close is the (not-yet-consumed) closing delimiter's kind, and
// parseOne parses a single item. Newlines are tolerated before the first
// item, around every comma, and — since the loop condition is "cur has
// reached close" rather than "another item follows a comma" — right before
// close as a trailing comma. The caller still consumes close itself
// afterward (matching every other parse* helper's convention here).
func parseCommaList[T any](p *parser, close lexer.Kind, parseOne func() (T, error)) ([]T, error) {
	if err := p.skipNewlines(); err != nil {
		return nil, err
	}
	var items []T
	for p.cur.Kind != close {
		item, err := parseOne()
		if err != nil {
			return nil, err
		}
		items = append(items, item)
		if err := p.skipNewlines(); err != nil {
			return nil, err
		}
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
	return items, nil
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
	case lexer.KwExtern:
		return p.parseExternDecl()
	case lexer.KwImport:
		return p.parseImportDecl()
	default:
		return nil, p.errorf("expected 'fn', 'const', 'struct', 'enum', 'extern', or 'import' at top level, got %s", p.cur.Kind)
	}
}

// parseImportDecl parses `import alias "path"` (amifl-spec.md section
// 12.2) — step 14. Unlike parseExternDecl, there's no block body: a single
// alias and a single string-literal path, always resolved at compile time
// by internal/modloader (never by the parser itself, which has no
// filesystem access and no notion of "the current file's directory").
func (p *parser) parseImportDecl() (*ast.ImportDecl, error) {
	kwTok, err := p.expect(lexer.KwImport)
	if err != nil {
		return nil, err
	}
	aliasTok, err := p.expect(lexer.Ident)
	if err != nil {
		return nil, err
	}
	pathTok, err := p.expect(lexer.String)
	if err != nil {
		return nil, err
	}
	return &ast.ImportDecl{Alias: aliasTok.Value, Path: pathTok.Value, Line: kwTok.Line}, nil
}

// parseExternDecl parses `extern "path" as alias { type Name ... bind
// Name(params) -> Ret [as GoTarget] ... }` (amifl-spec.md section 15) —
// step 13. `type`/`bind` entries are newline-separated, one per line,
// mirroring parseEnumDecl's variant layout.
func (p *parser) parseExternDecl() (*ast.ExternDecl, error) {
	kwTok, err := p.expect(lexer.KwExtern)
	if err != nil {
		return nil, err
	}
	pathTok, err := p.expect(lexer.String)
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.KwAs); err != nil {
		return nil, err
	}
	aliasTok, err := p.expect(lexer.Ident)
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.LBrace); err != nil {
		return nil, err
	}
	if err := p.skipNewlines(); err != nil {
		return nil, err
	}
	var types []ast.ExternTypeDecl
	var binds []ast.ExternBindDecl
	for p.cur.Kind != lexer.RBrace {
		switch p.cur.Kind {
		case lexer.KwType:
			if err := p.advance(); err != nil {
				return nil, err
			}
			nameTok, err := p.expect(lexer.Ident)
			if err != nil {
				return nil, err
			}
			types = append(types, ast.ExternTypeDecl{Name: nameTok.Value, Line: nameTok.Line})
		case lexer.KwBind:
			bind, err := p.parseExternBind()
			if err != nil {
				return nil, err
			}
			binds = append(binds, bind)
		default:
			return nil, p.errorf("expected 'type' or 'bind' inside extern block, got %s", p.cur.Kind)
		}
		if p.cur.Kind == lexer.RBrace {
			break
		}
		if p.cur.Kind != lexer.Newline {
			return nil, p.errorf("expected newline after extern block entry, got %s", p.cur.Kind)
		}
		if err := p.skipNewlines(); err != nil {
			return nil, err
		}
	}
	if _, err := p.expect(lexer.RBrace); err != nil {
		return nil, err
	}
	return &ast.ExternDecl{Path: pathTok.Value, Alias: aliasTok.Value, Types: types, Binds: binds, Line: kwTok.Line}, nil
}

// parseExternBind parses one `bind Name(params) -> Ret [as GoTarget]`
// entry — GoTarget (see ExternBindDecl's doc comment) is either a bare
// identifier or a `Type.Method` pair, both just an Ident optionally
// followed by `.` Ident, so one shared tail handles both shapes.
func (p *parser) parseExternBind() (ast.ExternBindDecl, error) {
	if err := p.advance(); err != nil { // consume 'bind'
		return ast.ExternBindDecl{}, err
	}
	nameTok, err := p.expect(lexer.Ident)
	if err != nil {
		return ast.ExternBindDecl{}, err
	}
	if _, err := p.expect(lexer.LParen); err != nil {
		return ast.ExternBindDecl{}, err
	}
	params, err := p.parseParamList()
	if err != nil {
		return ast.ExternBindDecl{}, err
	}
	if _, err := p.expect(lexer.Arrow); err != nil {
		return ast.ExternBindDecl{}, err
	}
	retType, err := p.parseTypeExpr()
	if err != nil {
		return ast.ExternBindDecl{}, err
	}
	goTarget := ""
	if p.cur.Kind == lexer.KwAs {
		if err := p.advance(); err != nil {
			return ast.ExternBindDecl{}, err
		}
		t1Tok, err := p.expect(lexer.Ident)
		if err != nil {
			return ast.ExternBindDecl{}, err
		}
		goTarget = t1Tok.Value
		if p.cur.Kind == lexer.Dot {
			if err := p.advance(); err != nil {
				return ast.ExternBindDecl{}, err
			}
			t2Tok, err := p.expect(lexer.Ident)
			if err != nil {
				return ast.ExternBindDecl{}, err
			}
			goTarget += "." + t2Tok.Value
		}
	}
	return ast.ExternBindDecl{Name: nameTok.Value, GoTarget: goTarget, Params: params, ReturnType: retType, Line: nameTok.Line}, nil
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
// struct type, amifl-spec.md sections 2.1/2.2), one of step 7's two
// bracket-generic collection types, `List[Elem]` and `Array[Elem;N1,N2,
// ...]` (section 2.2), (ex3) `fn(T1,T2,...) -> R` — the one case that
// isn't a leading identifier at all (`fn` is lexer.KwFn, a keyword, not an
// Ident), so it's checked first, before the p.expect(lexer.Ident) every
// other case still shares — or (ex5) `alias.Name`, a cross-package struct/
// enum reference (amifl-spec.md section 12.2), checked right after the
// leading identifier and before the reserved bracket-generic names, since a
// package alias could in principle collide textually with one of those.
// "List" and "Array" are recognized structurally, by comparing the leading
// identifier's text, rather than being reserved keywords — exactly like
// "Unit" is only special in a return-type position (sema's
// canonicalReturnType) without being a keyword anywhere else. A variable
// can still be named "List" or "Array" without conflict, since this
// function is only ever reached from a type-annotation position.
func (p *parser) parseTypeExpr() (ast.TypeExpr, error) {
	if p.cur.Kind == lexer.KwFn {
		return p.parseFuncType()
	}
	nameTok, err := p.expect(lexer.Ident)
	if err != nil {
		return nil, err
	}
	// ex5: `alias.Name` (amifl-spec.md section 12.2) — a cross-package
	// struct/enum type annotation. Checked before the switch below since a
	// package alias could in principle collide textually with one of the
	// six reserved bracket-generic names ("List" etc.) — the '.' takes
	// priority, matching how the qualified-struct-literal check in
	// parsePostfixExpr similarly runs before any other interpretation of a
	// leading identifier gets a chance.
	if p.cur.Kind == lexer.Dot {
		if err := p.advance(); err != nil {
			return nil, err
		}
		typeNameTok, err := p.expect(lexer.Ident)
		if err != nil {
			return nil, err
		}
		return &ast.QualifiedType{Alias: nameTok.Value, Name: typeNameTok.Value, Line: nameTok.Line}, nil
	}
	switch nameTok.Value {
	case "List":
		return p.parseListType(nameTok)
	case "Array":
		return p.parseArrayType(nameTok)
	case "Set":
		return p.parseSetType(nameTok)
	case "Map":
		return p.parseMapType(nameTok)
	case "Tuple2", "Tuple3", "Tuple4", "Tuple5", "Tuple6", "Tuple7", "Tuple8":
		return p.parseTupleType(nameTok)
	case "Chan":
		return p.parseChanType(nameTok)
	case "Stream":
		return p.parseStreamType(nameTok)
	default:
		return &ast.NamedType{Name: nameTok.Value, Line: nameTok.Line}, nil
	}
}

// parseTupleType parses `Tuple2[T1,T2]` ... `Tuple8[T1,...,T8]`
// (amifl-spec.md section 2.2) — step 11, see ast.TupleType's doc comment
// for why this arrived later than List/Array/Set/Map's own bracket syntax
// (step 7/10). nameTok is already known to be one of "Tuple2".."Tuple8"
// (the switch above), so the digit right after "Tuple" is always exactly
// one ASCII byte — wantN below reads it directly rather than pulling in
// strconv/strings for a single-character parse.
func (p *parser) parseTupleType(nameTok lexer.Token) (ast.TypeExpr, error) {
	wantN := int(nameTok.Value[len("Tuple")] - '0')
	if _, err := p.expect(lexer.LBracket); err != nil {
		return nil, err
	}
	elems, err := parseCommaList(p, lexer.RBracket, p.parseTypeExpr)
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.RBracket); err != nil {
		return nil, err
	}
	if len(elems) != wantN {
		return nil, p.errorf("line %d: %s expects %d type argument(s), got %d", nameTok.Line, nameTok.Value, wantN, len(elems))
	}
	return &ast.TupleType{Elems: elems, Line: nameTok.Line}, nil
}

// parseSetType parses `Set[Elem]` (amifl-spec.md section 2.2) — step 10.
func (p *parser) parseSetType(nameTok lexer.Token) (ast.TypeExpr, error) {
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
	return &ast.SetType{Elem: elem, Line: nameTok.Line}, nil
}

// parseMapType parses `Map[Key,Value]` (amifl-spec.md section 2.2) — step 10.
func (p *parser) parseMapType(nameTok lexer.Token) (ast.TypeExpr, error) {
	if _, err := p.expect(lexer.LBracket); err != nil {
		return nil, err
	}
	key, err := p.parseTypeExpr()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.Comma); err != nil {
		return nil, err
	}
	val, err := p.parseTypeExpr()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.RBracket); err != nil {
		return nil, err
	}
	return &ast.MapType{Key: key, Value: val, Line: nameTok.Line}, nil
}

// parseChanType parses `Chan[Elem]` (amifl-spec.md sections 2.2/11/13.8) —
// step 12.
func (p *parser) parseChanType(nameTok lexer.Token) (ast.TypeExpr, error) {
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
	return &ast.ChanType{Elem: elem, Line: nameTok.Line}, nil
}

// parseStreamType parses `Stream[Elem]` (amifl-spec.md section 2.2/13.8) —
// step 12.
func (p *parser) parseStreamType(nameTok lexer.Token) (ast.TypeExpr, error) {
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
	return &ast.StreamType{Elem: elem, Line: nameTok.Line}, nil
}

// parseFuncType parses `fn(T1,T2,...) -> R` as a *type annotation*
// (amifl-spec.md section 8.3's Func type, ex3) — distinct from
// parseClosureLit's `fn(params) -> R { body }` *value* syntax (reached
// only from parsePrimaryExpr, never here, exactly the way parseTopLevelDecl's
// statement-position `fn` and parseClosureLit's expression-position `fn`
// already share KwFn without ambiguity — the two are told apart purely by
// which parse function is on the call stack when KwFn is seen). The two
// share an outwardly similar params/arrow/return-type shape, but a type
// position never has a body, and this one's param list holds bare
// TypeExprs (no parameter *names* — see ast.FuncType's own doc comment for
// why), so it can't just delegate to parseParamList the way parseClosureLit
// does.
func (p *parser) parseFuncType() (ast.TypeExpr, error) {
	kwTok, err := p.expect(lexer.KwFn)
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.LParen); err != nil {
		return nil, err
	}
	params, err := parseCommaList(p, lexer.RParen, p.parseTypeExpr)
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.RParen); err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.Arrow); err != nil {
		return nil, err
	}
	ret, err := p.parseTypeExpr()
	if err != nil {
		return nil, err
	}
	return &ast.FuncType{Params: params, Ret: ret, Line: kwTok.Line}, nil
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
	sizes, err := parseCommaList(p, lexer.RBracket, p.parsePipeExpr)
	if err != nil {
		return nil, err
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
// amifl-spec.md section 8.1's grammar for the two is identical. Type is
// parsed by the ordinary parseTypeExpr (via parseFieldTypeList below), so
// a parameter may since ex3 be Func-typed (`f: fn(Int) -> Int`) exactly
// like any other type-annotation position — see ast.Param's doc comment.
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
// field list (parseStructDecl, terminated by `}`). Delegates to
// parseCommaList (ex8), so a struct's fields (unlike a `fn`'s, which are
// normally short enough to fit one line) can be laid out one per line — the
// natural style once a struct has more than a couple of fields, and how
// amifl-spec.md itself formats `enum`'s analogous variant list (section
// 2.2) — with an optional trailing comma before end.
func (p *parser) parseFieldTypeList(end lexer.Kind) ([]ast.Param, error) {
	return parseCommaList(p, end, func() (ast.Param, error) {
		nameTok, err := p.expect(lexer.Ident)
		if err != nil {
			return ast.Param{}, err
		}
		if _, err := p.expect(lexer.Colon); err != nil {
			return ast.Param{}, err
		}
		typ, err := p.parseTypeExpr()
		if err != nil {
			return ast.Param{}, err
		}
		return ast.Param{Name: nameTok.Value, Type: typ, Line: nameTok.Line}, nil
	})
}

// parseClosureLit parses `fn(params) -> R { body }` as an expression
// (amifl-spec.md section 8.1) — reachable from parsePrimaryExpr (an
// ordinary value position — sema still restricts what a bare ClosureLit
// value may resolve *from* there to just a `let`'s direct value, its own
// doc comment) and, since ex4, from parsePipeRHS (a `|>` chain's right-hand
// side, wrapped into InlineClosure rather than left as a bare value). Never
// reachable from parseTopLevelDecl's statement-position `fn`, so there's no
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
// top-level `let`/`const`/`_ = ...`/reassignment/`break`/`continue`/
// `return`/value-expression entries (amifl-spec.md section 5). Reassignment
// (`name = expr`, `x[i] = v`) is deliberately not reachable from within
// parseOrExpr's operator chain — it isn't listed among amifl-spec.md
// section 6's operators, and (like let/const/discard) it's Unit-typed, so
// nesting it inside a larger expression would only ever be legal in a
// position that itself has to be Unit-typed. Detecting it no longer needs a
// peek past the target (step 2's original approach, sufficient when the
// only assignable target was a bare name): step 7 adds `x[i] = v`, a target
// that isn't just one token, so this instead parses the target as an
// ordinary expression first and reclassifies it if `=` follows — the same
// technique Go itself uses for its own assignment statements.
//
// `break`/`continue`/`return` (ex11 moved the first two here from
// parsePrimaryExpr — see ast.ReturnExpr's doc comment) are statement-
// position only for a structural reason, not just a style choice: each one
// lowers to a single AMIVM control-flow instruction (BREAK/CONTINUE/RET)
// that has to stand on its own in the generated instruction stream — none
// of the three can be embedded as a value inside another instruction's
// argument list (there's no way to "return the value of a jump" at the
// AMIVM-IR level). Restricting them to statement position (which, since a
// block's last statement is still just a statement, already includes a
// block's own tail — enough to write `if done { return 5 } else { 10 }`)
// means codegen never has to handle one turning up nested inside an
// arbitrary expression (a call argument, a binary operand, a list
// element, ...).
func (p *parser) parseExpr() (ast.Expr, error) {
	switch p.cur.Kind {
	case lexer.KwLet:
		return p.parseLetExpr()
	case lexer.KwConst:
		return p.parseConstDecl()
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
	case lexer.KwReturn:
		return p.parseReturnExpr()
	case lexer.Ident:
		if p.cur.Value == "_" {
			return p.parseDiscardExpr()
		}
	}
	expr, err := p.parsePipeExpr()
	if err != nil {
		return nil, err
	}
	if p.cur.Kind != lexer.Assign {
		return expr, nil
	}
	return p.finishAssignExpr(expr)
}

// finishAssignExpr consumes the `=` parseExpr just found after target and
// builds the appropriate assignment node. A bare identifier
// (ast.AssignExpr), an index expression (ast.IndexAssignExpr), or — since
// ex10 — a plain `.field` access with no trailing call (ast.FieldAssignExpr)
// are valid targets; anything else (a call, an enum-variant construction, a
// literal, a binary expression, ...) is simply not assignable. A FieldExpr
// with Args != nil (`p.Field(...)` — enum construction or a qualified call)
// is rejected the same way, since assigning to a call's result makes no
// sense.
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
	case *ast.FieldExpr:
		if t.Args != nil {
			return nil, fmt.Errorf("line %d: invalid assignment target (cannot assign to a call)", eqLine)
		}
		return &ast.FieldAssignExpr{Target: t.Target, Field: t.Field, Value: value, Line: t.Line}, nil
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

// parseRangeExpr parses an optional `a..b` / `a..=b` numeric range
// (amifl-spec.md section 3.1/7.3, ex2) — sits directly below parsePipeExpr
// (so `0..10 |> f` parses as `(0..10) |> f`, never `0..(10 |> f)`: see
// parsePipeExpr's own call site below) and directly above the full
// binary-operator chain (so `for i in 0..n-1` fully parses the bound
// `n-1` via parseOrExpr before `..` ever sees it — the same "parse the
// widest expression first, then look at what's left over" shape
// parseExpr's assignment detection and step-14's parseFieldCallArgs both
// already use). Non-chaining by construction: parseOrExpr's own result
// is used directly as each bound, so `a..b..c` simply leaves a second
// `..`/`..=` sitting unconsumed for whatever parses next to reject, the
// same way any other unexpected token would.
func (p *parser) parseRangeExpr() (ast.Expr, error) {
	left, err := p.parseOrExpr()
	if err != nil {
		return nil, err
	}
	inclusive := false
	switch p.cur.Kind {
	case lexer.DotDotEq:
		inclusive = true
	case lexer.DotDot:
		// inclusive stays false
	default:
		return left, nil
	}
	line := p.cur.Line
	if err := p.advance(); err != nil {
		return nil, err
	}
	right, err := p.parseOrExpr()
	if err != nil {
		return nil, err
	}
	return &ast.RangeExpr{From: left, To: right, Inclusive: inclusive, Line: line}, nil
}

// parsePipeExpr is the top of the value-expression grammar — every
// "start of an expression" entry point outside the operator-precedence
// chain itself (list/tuple/struct/enum literal elements, call args via
// parseExpr, index/slice bounds, array-size expressions, if/while/for
// headers, switch case conditions/bodies, for-yield's own expression, ...)
// parses through here rather than calling parseOrExpr/parseRangeExpr
// directly, so that `|>` (amifl-spec.md section 9, lower precedence than
// even `||` — see parseOrExpr's doc comment) and `..`/`..=` (ex2's Range,
// one level tighter than `|>` — parseRangeExpr's own doc comment) are
// both reachable everywhere a value expression already was, the same way
// the whole operator chain below them already is. Left-associative: `a |>
// f |> g` parses as `(a |> f) |> g`, matching every other binary-shaped
// operator here.
func (p *parser) parsePipeExpr() (ast.Expr, error) {
	left, err := p.parseRangeExpr()
	if err != nil {
		return nil, err
	}
	continues, err := p.pipeContinues()
	if err != nil {
		return nil, err
	}
	if !continues {
		return left, nil
	}

	// A real chain: collect every stage's CallExpr as we go (each already
	// carries its own PipeArgIndex, set by parsePipeRHS below) so that once
	// the chain ends we know its full length and can back-fill PipeStage/
	// PipeChainLabels onto all of them at once — amifl-spec.md section 9.1's
	// diagnostic needs the whole chain's shape, not just one stage.
	labels := []string{pipeChainLabel(left)}
	var stages []*ast.CallExpr
	for continues {
		if err := p.advance(); err != nil { // consume '|>'
			return nil, err
		}
		next, err := p.parsePipeRHS(left)
		if err != nil {
			return nil, err
		}
		call := next.(*ast.CallExpr) // parsePipeRHS always returns *ast.CallExpr
		stages = append(stages, call)
		labels = append(labels, call.Callee)
		left = next
		continues, err = p.pipeContinues()
		if err != nil {
			return nil, err
		}
	}
	for i, call := range stages {
		call.PipeStage = i + 1
		call.PipeChainLabels = labels
	}
	return left, nil
}

// pipeContinues reports whether the token stream, from p.cur, continues an
// in-progress pipe chain (ex8) — either p.cur is already `|>`, or p.cur is
// one or more Newlines followed (once skipped) by `|>`, i.e. a chain
// continuing onto the next line: `data\n  |> f\n  |> g`. In that second
// case the newlines are consumed, landing p.cur on `|>` itself, exactly as
// if no newline had been there. In every other case (plain non-newline,
// non-`|>` token, or newlines followed by anything but `|>`) nothing is
// consumed — a genuine statement-separator newline is left untouched for
// parseBlock's own newline handling to see.
func (p *parser) pipeContinues() (bool, error) {
	if p.cur.Kind == lexer.PipeArrow {
		return true, nil
	}
	if p.cur.Kind != lexer.Newline {
		return false, nil
	}
	next, err := p.peekPastNewlines()
	if err != nil {
		return false, err
	}
	if next.Kind != lexer.PipeArrow {
		return false, nil
	}
	if err := p.skipNewlines(); err != nil {
		return false, err
	}
	return true, nil
}

// pipeChainLabel produces a short display label for a pipeline's initial
// left-hand value (amifl-spec.md section 9.1's diagnostic, e.g. "data" in
// `data |> parse |> ...`) — only IdentExpr/CallExpr get a precise label
// (their own name), since anything else (a literal, a binary expression, a
// field access, ...) has no single short name; those fall back to a generic
// placeholder rather than attempting to unparse arbitrary source, which
// nothing else in this codebase does either.
func pipeChainLabel(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.IdentExpr:
		return v.Name
	case *ast.CallExpr:
		return v.Callee
	default:
		return "<value>"
	}
}

// parsePipeRHS parses the right-hand side of one `|>` step, given the
// already-parsed left-hand value lhs, and desugars the whole step directly
// into an *ast.CallExpr (amifl-spec.md section 9) — see CallExpr's doc
// comment. Most of the time the right-hand side is `name` or
// `name(args...)` (CallExpr.Callee is always a bare name in this case,
// never an arbitrary expression — the same restriction parseIdentOrCall
// already enforces for an ordinary call, so this doesn't introduce a new
// capability, just a new way to reach the existing one).
//
// Since ex4, the right-hand side may instead be a bare inline closure
// literal (`data |> fn(x) -> R {...}`) — step 9's original scope cut here
// (CLAUDE.md's "確定した設計判断" for that step) is lifted now that ex3's
// FuncType gives a closure's signature a name to check against, and AMIVM's
// FUNCVAL/CLOS instructions already give codegen everything needed to mint
// one on the spot. This is recognized by a leading `fn` keyword — no other
// RHS form starts with one — and produces a CallExpr with InlineClosure set
// instead of Callee (that field's own doc comment) rather than trying to
// force a closure literal through the name-based CallExpr shape below;
// like the plain `a |> f` case, lhs becomes the closure's sole argument
// (amifl-spec.md section 9 doesn't offer `_`/explicit-args syntax for this
// form — the closure's own parameter list already says everything an
// argument list would).
func (p *parser) parsePipeRHS(lhs ast.Expr) (ast.Expr, error) {
	if p.cur.Kind == lexer.KwFn {
		closExpr, err := p.parseClosureLit()
		if err != nil {
			return nil, err
		}
		lit := closExpr.(*ast.ClosureLit)
		return &ast.CallExpr{Callee: "<closure>", InlineClosure: lit, Args: []ast.Expr{lhs}, Line: lit.Line, PipeArgIndex: 0}, nil
	}
	nameTok, err := p.expect(lexer.Ident)
	if err != nil {
		return nil, err
	}
	typeArg, err := p.parseGenericTypeArgBracket(nameTok)
	if err != nil {
		return nil, err
	}
	if p.cur.Kind != lexer.LParen {
		// `a |> f` — f takes lhs as its sole argument (amifl-spec.md
		// section 9, "省略時は第1引数へ左辺値を注入する", the degenerate
		// no-other-args case). Also covers `a |> unwrap[T]`/`a |> cast[T]`
		// (parseGenericTypeArgBracket already consumed the bracket above).
		return &ast.CallExpr{Callee: nameTok.Value, Args: []ast.Expr{lhs}, Line: nameTok.Line, TypeArg: typeArg, PipeArgIndex: 0}, nil
	}
	if err := p.advance(); err != nil { // consume '('
		return nil, err
	}
	saved := p.noCompositeLit
	p.noCompositeLit = false
	defer func() { p.noCompositeLit = saved }()

	// `a |> f(...)`: lhs is injected at an explicit `_` placeholder's
	// position if one appears, or prepended as the first argument
	// otherwise (amifl-spec.md section 9). `_` is recognized here as its
	// own token, never routed through parsePipeExpr/parseExpr — unlike
	// `_ = expr`'s discard statement (the only other place `_` means
	// anything today), a bare `_` here isn't followed by `=`, so it can't
	// reuse parseDiscardExpr's grammar; this is a new, narrower meaning
	// specific to a pipe call's argument list.
	var args []ast.Expr
	placeholderIdx := -1
	if err := p.skipNewlines(); err != nil {
		return nil, err
	}
	for p.cur.Kind != lexer.RParen {
		if p.cur.Kind == lexer.Ident && p.cur.Value == "_" {
			if placeholderIdx >= 0 {
				return nil, p.errorf("a pipe's '_' placeholder may appear at most once per call")
			}
			placeholderIdx = len(args)
			args = append(args, nil) // filled in below, once lhs is known to go here
			if err := p.advance(); err != nil {
				return nil, err
			}
		} else {
			arg, err := p.parsePipeExpr()
			if err != nil {
				return nil, err
			}
			args = append(args, arg)
		}
		if err := p.skipNewlines(); err != nil {
			return nil, err
		}
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
	if _, err := p.expect(lexer.RParen); err != nil {
		return nil, err
	}

	argIdx := 0
	if placeholderIdx >= 0 {
		args[placeholderIdx] = lhs
		argIdx = placeholderIdx
	} else {
		args = append([]ast.Expr{lhs}, args...)
	}
	return &ast.CallExpr{Callee: nameTok.Value, Args: args, Line: nameTok.Line, TypeArg: typeArg, PipeArgIndex: argIdx}, nil
}

// The chain implements amifl-spec.md section 6's precedence table
// (high to low): unary `! - ~` -> `* / % << >> & &^` -> `+ - | ^` ->
// `< <= > >=` -> `== !=` -> `&&` -> `||` -> `|>` (lowest, step 9,
// parsePipeExpr above). postfix `. [] ?` (highest) sits below unary,
// inside parsePostfixExpr.
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
			fieldLine := p.cur.Line
			switch p.cur.Kind {
			case lexer.Int, lexer.Ident:
				field = p.cur.Value
			default:
				return nil, p.errorf("expected a field name or tuple index after '.', got %s", p.cur.Kind)
			}
			if err := p.advance(); err != nil {
				return nil, err
			}
			// ex5: `alias.Name{...}` — a cross-package struct literal
			// (amifl-spec.md section 12.2). Only reachable right off a bare
			// leading identifier (expr hasn't been reassigned away from
			// *ast.IdentExpr yet, i.e. this is the chain's first '.'), and
			// only outside a noCompositeLit context — the identical
			// disambiguation parseIdentOrCall's own unqualified `Ident '{'`
			// check already uses (e.g. `if p.field { ... }`'s body-opening
			// `{` must never be swallowed this way). sema is what actually
			// verifies "alias" names a real import and "Name" a real
			// exported struct — nothing here checks that, exactly like the
			// unqualified form's own StructLit doesn't verify TypeName names
			// a real struct at parse time either.
			if alias, ok := expr.(*ast.IdentExpr); ok && p.cur.Kind == lexer.LBrace && !p.noCompositeLit {
				lit, err := p.parseStructLit(lexer.Token{Kind: lexer.Ident, Value: field, Line: fieldLine})
				if err != nil {
					return nil, err
				}
				sl := lit.(*ast.StructLit)
				sl.Qualifier = alias.Name
				expr = sl
				continue
			}
			var args []ast.StructLitField
			if p.cur.Kind == lexer.LParen {
				args, err = p.parseFieldCallArgs()
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
		case lexer.Question:
			// Postfix `?` (amifl-spec.md section 3.3) — error-short-circuit
			// on a Tuple2[T,Error]/Error-returning expression.
			questionLine := p.cur.Line
			if err := p.advance(); err != nil {
				return nil, err
			}
			expr = &ast.TryExpr{Value: expr, Line: questionLine}
		default:
			return expr, nil
		}
	}
}

// parseFieldCallArgs parses `(arg1, arg2, ...)` right after a postfix
// `.Field` — the single grammar that serves both of the two meanings this
// shape can have (sema's resolveFieldExpr is what tells them apart, not the
// parser): enum variant construction (amifl-spec.md section 2.2,
// "Status.Retry(delay: 5)"), where every argument is a `name: value` pair,
// or (step 14) a cross-package qualified function call (amifl-spec.md
// section 12.2, "mathutil.clamp(15, 0, 10)"), where every argument is a
// plain positional value. Each argument decides its own shape independently
// with no lookahead beyond the single token parsePostfixExpr already uses
// elsewhere in this parser: parse it as an ordinary expression first (a
// bare field-name label like `delay` parses trivially as *ast.IdentExpr,
// since nothing in AmiFL's expression grammar ever continues past a bare
// identifier with a `:` — no ternary, no slice bounds outside `[...]`, no
// map-literal colon outside `{...}`) and, only when that expression turned
// out to be a bare identifier immediately followed by `:`, reinterpret it
// as a name label and parse the real value after the colon. This is safe
// precisely because AmiFL's grammar never lets `:` legally follow a
// positional argument's own bare-identifier value (mirrors how step 7
// removed its old assignment-detection peek buffer by relying on the
// identical "parse as an expression, inspect what's left over" trick).
//
// Always returns a non-nil slice (empty for `()`), which is what tells
// FieldExpr.Args apart from a plain `.field` access with no trailing call
// at all (nil) — see FieldExpr's doc comment.
func (p *parser) parseFieldCallArgs() ([]ast.StructLitField, error) {
	if _, err := p.expect(lexer.LParen); err != nil {
		return nil, err
	}
	saved := p.noCompositeLit
	p.noCompositeLit = false
	defer func() { p.noCompositeLit = saved }()

	args, err := parseCommaList(p, lexer.RParen, func() (ast.StructLitField, error) {
		argLine := p.cur.Line
		first, err := p.parsePipeExpr()
		if err != nil {
			return ast.StructLitField{}, err
		}
		var name string
		value := first
		if ident, ok := first.(*ast.IdentExpr); ok && p.cur.Kind == lexer.Colon {
			if err := p.advance(); err != nil {
				return ast.StructLitField{}, err
			}
			name = ident.Name
			value, err = p.parsePipeExpr()
			if err != nil {
				return ast.StructLitField{}, err
			}
		}
		return ast.StructLitField{Name: name, Value: value, Line: argLine}, nil
	})
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.RParen); err != nil {
		return nil, err
	}
	if args == nil {
		// Always non-nil (even for `()`) — this is what tells a called
		// field apart from a plain `.field` access with no call at all
		// (FieldExpr.Args's doc comment).
		args = []ast.StructLitField{}
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

	if err := p.skipNewlines(); err != nil {
		return nil, err
	}
	var from ast.Expr
	if p.cur.Kind != lexer.Colon {
		from, err = p.parsePipeExpr()
		if err != nil {
			return nil, err
		}
		if err := p.skipNewlines(); err != nil {
			return nil, err
		}
	}
	if p.cur.Kind == lexer.Colon {
		if err := p.advance(); err != nil {
			return nil, err
		}
		if err := p.skipNewlines(); err != nil {
			return nil, err
		}
		to, err := p.parseOptionalSliceBound()
		if err != nil {
			return nil, err
		}
		if err := p.skipNewlines(); err != nil {
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
	return p.parsePipeExpr()
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

	elems, err := parseCommaList(p, lexer.RBracket, p.parsePipeExpr)
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.RBracket); err != nil {
		return nil, err
	}
	return &ast.ListLit{Elems: elems, Line: openTok.Line}, nil
}

// parseBraceLit parses a bare `{...}` value expression: `{v1, v2, ...}`
// (Set[T]) or `{k1: v1, k2: v2, ...}` (Map[K,V]) (amifl-spec.md sections
// 2.2/3.1) — step 10. Unlike a struct literal (`Name{...}`), a bare `{`
// never needs noCompositeLit's disambiguation: its own matching `}`
// unambiguously delimits the whole literal wherever it starts (including
// right at the start of an if/while/for header — noCompositeLit's
// disambiguation problem is specifically about a shared opening token
// between "start of a composite literal" and "start of the following
// block", which a struct literal has via its leading `Ident` and a bare
// `{` simply doesn't).
//
// Set vs Map is told apart with one token of lookahead (skipping newlines,
// ex8) right after the first entry: a `:` means Map (parse `key: value`
// pairs from here on), anything else means Set (parse plain value
// expressions). A bare `{}` can't be told apart at all (ast.SetOrMapLit's
// doc comment) — Elems and Entries are both left nil for sema's
// resolveSetOrMapLit to sort out via `expected`. Newlines are tolerated
// throughout (before the first entry, around each comma including a
// trailing one, and around `:`) the same way parseCommaList handles every
// other bracketed list — kept as a hand-written loop rather than delegating
// to it since Set/Map's first-entry-decides-the-shape branching doesn't fit
// parseCommaList's one-item-at-a-time shape.
func (p *parser) parseBraceLit() (ast.Expr, error) {
	openTok, err := p.expect(lexer.LBrace)
	if err != nil {
		return nil, err
	}
	saved := p.noCompositeLit
	p.noCompositeLit = false
	defer func() { p.noCompositeLit = saved }()

	if err := p.skipNewlines(); err != nil {
		return nil, err
	}
	if p.cur.Kind == lexer.RBrace {
		if err := p.advance(); err != nil {
			return nil, err
		}
		return &ast.SetOrMapLit{Line: openTok.Line}, nil
	}

	first, err := p.parsePipeExpr()
	if err != nil {
		return nil, err
	}
	if err := p.skipNewlines(); err != nil {
		return nil, err
	}

	if p.cur.Kind == lexer.Colon {
		if err := p.advance(); err != nil {
			return nil, err
		}
		if err := p.skipNewlines(); err != nil {
			return nil, err
		}
		firstVal, err := p.parsePipeExpr()
		if err != nil {
			return nil, err
		}
		entries := []ast.MapLitEntry{{Key: first, Value: firstVal, Line: openTok.Line}}
		if err := p.skipNewlines(); err != nil {
			return nil, err
		}
		for p.cur.Kind == lexer.Comma {
			if err := p.advance(); err != nil {
				return nil, err
			}
			if err := p.skipNewlines(); err != nil {
				return nil, err
			}
			if p.cur.Kind == lexer.RBrace {
				break // trailing comma
			}
			keyTok := p.cur.Line
			key, err := p.parsePipeExpr()
			if err != nil {
				return nil, err
			}
			if _, err := p.expect(lexer.Colon); err != nil {
				return nil, err
			}
			if err := p.skipNewlines(); err != nil {
				return nil, err
			}
			val, err := p.parsePipeExpr()
			if err != nil {
				return nil, err
			}
			entries = append(entries, ast.MapLitEntry{Key: key, Value: val, Line: keyTok})
			if err := p.skipNewlines(); err != nil {
				return nil, err
			}
		}
		if _, err := p.expect(lexer.RBrace); err != nil {
			return nil, err
		}
		return &ast.SetOrMapLit{Entries: entries, Line: openTok.Line}, nil
	}

	elems := []ast.Expr{first}
	for p.cur.Kind == lexer.Comma {
		if err := p.advance(); err != nil {
			return nil, err
		}
		if err := p.skipNewlines(); err != nil {
			return nil, err
		}
		if p.cur.Kind == lexer.RBrace {
			break // trailing comma
		}
		elem, err := p.parsePipeExpr()
		if err != nil {
			return nil, err
		}
		elems = append(elems, elem)
		if err := p.skipNewlines(); err != nil {
			return nil, err
		}
	}
	if _, err := p.expect(lexer.RBrace); err != nil {
		return nil, err
	}
	return &ast.SetOrMapLit{Elems: elems, Line: openTok.Line}, nil
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
		// Base 0: infers the base from tok.Value's own prefix (0x/0o/0b,
		// or plain decimal otherwise) and accepts '_' digit separators —
		// exactly amivm's own upgraded literal grammar (ex7,
		// ignored/amivm/amivm_spec.md section 6). Safe against Go's
		// legacy "bare 0 prefix means octal" base-0 quirk because the
		// lexer already rejected any multi-digit token starting with an
		// unprefixed '0' (lexer.go's lexNumber).
		n, err := strconv.ParseUint(tok.Value, 0, 64)
		if err != nil {
			return nil, fmt.Errorf("line %d: invalid integer literal %q: %w", tok.Line, tok.Value, err)
		}
		if err := p.advance(); err != nil {
			return nil, err
		}
		return &ast.IntLit{Value: n, Token: tok.Value, Line: tok.Line}, nil
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
	case lexer.LBrace:
		return p.parseBraceLit()
	case lexer.KwIf:
		return p.parseIfExpr()
	case lexer.KwSwitch:
		return p.parseSwitchExpr()
	case lexer.KwWhile:
		return p.parseWhileExpr()
	case lexer.KwFor:
		return p.parseForExpr()
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
	return p.parsePipeExpr()
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

// parseForExpr parses `for x in items { ... }` (amifl-spec.md section 7,
// Unit-typed, side-effect-only), step 9's `for x in items yield expr` form
// (a single trailing expression, parsePipeExpr, not a `{ }` block: the spec
// gives no block form for `yield`, and every case-body-shaped position in
// this parser already keeps to a single expression the same way — see
// e.g. parseBoolSwitchExpr/parseEnumSwitchExpr), or step 10's two-variable
// `for k, v in m { ... }` form (Map[K,V] iteration — ast.ForExpr.Var2's
// doc comment). Which of the first two forms this is can't be told apart
// until `items` has already been fully parsed, so both share this one
// function rather than being split the way parseSwitchExpr splits on
// `switch`'s very next token; the two-variable form is detected earlier,
// right after the first loop variable, via a comma one token of lookahead
// can already resolve. `for k, v in m yield ...` is rejected here as a
// plain parse error — step 10's deliberate scope cut (Var2's doc comment)
// — rather than left for sema, since the combination can never mean
// anything valid regardless of m's type.
func (p *parser) parseForExpr() (ast.Expr, error) {
	kwTok, err := p.expect(lexer.KwFor)
	if err != nil {
		return nil, err
	}
	varTok, err := p.expect(lexer.Ident)
	if err != nil {
		return nil, err
	}
	var var2Tok lexer.Token
	hasVar2 := false
	if p.cur.Kind == lexer.Comma {
		if err := p.advance(); err != nil {
			return nil, err
		}
		var2Tok, err = p.expect(lexer.Ident)
		if err != nil {
			return nil, err
		}
		hasVar2 = true
	}
	if _, err := p.expect(lexer.KwIn); err != nil {
		return nil, err
	}
	items, err := p.parseHeaderExpr()
	if err != nil {
		return nil, err
	}
	if p.cur.Kind == lexer.KwYield {
		if hasVar2 {
			return nil, p.errorf("`for %s, %s in ... yield ...` isn't supported (Map iteration with `yield` isn't supported yet — use the single-variable form)", varTok.Value, var2Tok.Value)
		}
		if err := p.advance(); err != nil {
			return nil, err
		}
		yield, err := p.parsePipeExpr()
		if err != nil {
			return nil, err
		}
		return &ast.ForExpr{Var: varTok.Value, Items: items, Yield: yield, Line: kwTok.Line}, nil
	}
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	fe := &ast.ForExpr{Var: varTok.Value, Items: items, Body: body, Line: kwTok.Line}
	if hasVar2 {
		fe.Var2 = var2Tok.Value
	}
	return fe, nil
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
			cond, err := p.parsePipeExpr()
			if err != nil {
				return nil, err
			}
			if _, err := p.expect(lexer.Colon); err != nil {
				return nil, err
			}
			body, err := p.parsePipeExpr()
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
			body, err := p.parsePipeExpr()
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
			body, err := p.parsePipeExpr()
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
			body, err := p.parsePipeExpr()
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
	bindingList, err := parseCommaList(p, lexer.RParen, func() (string, error) {
		bindTok, err := p.expect(lexer.Ident)
		return bindTok.Value, err
	})
	if err != nil {
		return "", "", nil, err
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

// parseReturnExpr parses `return`/`return expr` (amifl-spec.md section 5,
// ex11) — reached only from parseExpr's statement position (see its own
// doc comment for why). Bare `return` (Value left nil) is recognized
// exactly when the token right after `return` can't start a value here at
// all: a Newline (the block's own statement separator) or the block's
// closing `}` — every other token is parsed as the return value via
// parsePipeExpr (never parseExpr — a `return`'s own value is an ordinary
// value expression, not itself another statement).
func (p *parser) parseReturnExpr() (ast.Expr, error) {
	tok, err := p.expect(lexer.KwReturn)
	if err != nil {
		return nil, err
	}
	if p.cur.Kind == lexer.Newline || p.cur.Kind == lexer.RBrace || p.cur.Kind == lexer.EOF {
		return &ast.ReturnExpr{Line: tok.Line}, nil
	}
	val, err := p.parsePipeExpr()
	if err != nil {
		return nil, err
	}
	return &ast.ReturnExpr{Value: val, Line: tok.Line}, nil
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
	typeArg, err := p.parseGenericTypeArgBracket(nameTok)
	if err != nil {
		return nil, err
	}
	if p.cur.Kind == lexer.LParen {
		call, err := p.parseCallArgs(nameTok)
		if err != nil {
			return nil, err
		}
		call.(*ast.CallExpr).TypeArg = typeArg
		return call, nil
	}
	if typeArg != nil {
		return nil, p.errorf("line %d: expected '(' after %s[...]", p.cur.Line, nameTok.Value)
	}
	if p.cur.Kind == lexer.LBrace && !p.noCompositeLit {
		return p.parseStructLit(nameTok)
	}
	return &ast.IdentExpr{Name: nameTok.Value, Line: nameTok.Line}, nil
}

// genericBuiltinNames is the fixed set of reserved built-in names that take
// a bracketed type argument (amifl-spec.md sections 13.3/13.8/13.9):
// `cast[T]`/`parse[T]`/`unwrap[T]`/`okOr[T]`/`chan[T]`. Recognized only for
// these five reserved names — see ast.CallExpr.TypeArg's doc comment for why
// this isn't a general user-facing generics grammar. Any other identifier
// followed by `[` is ordinary indexing (parsePostfixExpr), untouched.
// `chan[T]` needs the bracket (unlike, say, `collect`/`take`/`skip`/
// `parallel`) because its only argument is a plain Int buffer size — T has
// no argument to infer from, exactly like cast[T]/parse[T].
var genericBuiltinNames = map[string]bool{
	"cast": true, "parse": true, "unwrap": true, "okOr": true, "chan": true,
}

// parseGenericTypeArgBracket consumes an optional `[Type]` immediately
// following nameTok, returning the parsed type (nil if nameTok isn't one of
// genericBuiltinNames or isn't followed by `[`). Shared by parseIdentOrCall
// (ordinary call position) and parsePipeRHS (pipe RHS position) so both
// `cast[Int](v)` and `v |> cast[Int]`/`v |> okOr[Int](0)` parse the same
// bracket syntax identically.
func (p *parser) parseGenericTypeArgBracket(nameTok lexer.Token) (ast.TypeExpr, error) {
	if !genericBuiltinNames[nameTok.Value] || p.cur.Kind != lexer.LBracket {
		return nil, nil
	}
	if err := p.advance(); err != nil { // consume '['
		return nil, err
	}
	typeArg, err := p.parseTypeExpr()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.RBracket); err != nil {
		return nil, err
	}
	return typeArg, nil
}

// parseCallArgs parses `(arg1, arg2, ...)` for an ordinary call
// (`name(...)`). Each argument goes through parsePipeExpr, not parseExpr —
// call-argument position must never reach any of parseExpr's
// statement-only forms (ex11's `return`/`break`/`continue` in particular:
// none of the three can be embedded inside a CALL instruction's argument
// list at the AMIVM-IR level, ast.ReturnExpr's doc comment — but the same
// reasoning already applied, more quietly, to `let`/`const`/`_ = expr`/
// reassignment, all Unit-typed and so never a legal argument value anyway;
// ex11 just made the restriction load-bearing instead of merely academic).
func (p *parser) parseCallArgs(nameTok lexer.Token) (ast.Expr, error) {
	if _, err := p.expect(lexer.LParen); err != nil {
		return nil, err
	}
	saved := p.noCompositeLit
	p.noCompositeLit = false
	defer func() { p.noCompositeLit = saved }()

	args, err := parseCommaList(p, lexer.RParen, p.parsePipeExpr)
	if err != nil {
		return nil, err
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

	if err := p.skipNewlines(); err != nil {
		return nil, err
	}
	first, err := p.parsePipeExpr()
	if err != nil {
		return nil, err
	}
	if err := p.skipNewlines(); err != nil {
		return nil, err
	}
	elems := []ast.Expr{first}
	sawComma := false
	for p.cur.Kind == lexer.Comma {
		sawComma = true
		if err := p.advance(); err != nil {
			return nil, err
		}
		if err := p.skipNewlines(); err != nil {
			return nil, err
		}
		if p.cur.Kind == lexer.RParen {
			break
		}
		elem, err := p.parsePipeExpr()
		if err != nil {
			return nil, err
		}
		elems = append(elems, elem)
		if err := p.skipNewlines(); err != nil {
			return nil, err
		}
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

	fields, err := parseCommaList(p, lexer.RBrace, func() (ast.StructLitField, error) {
		fieldNameTok, err := p.expect(lexer.Ident)
		if err != nil {
			return ast.StructLitField{}, err
		}
		if _, err := p.expect(lexer.Colon); err != nil {
			return ast.StructLitField{}, err
		}
		val, err := p.parsePipeExpr()
		if err != nil {
			return ast.StructLitField{}, err
		}
		return ast.StructLitField{Name: fieldNameTok.Value, Value: val, Line: fieldNameTok.Line}, nil
	})
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.RBrace); err != nil {
		return nil, err
	}
	return &ast.StructLit{TypeName: nameTok.Value, Fields: fields, Line: nameTok.Line}, nil
}
