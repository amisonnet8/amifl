package lexer

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

var keywords = map[string]Kind{
	"fn":       KwFn,
	"let":      KwLet,
	"const":    KwConst,
	"true":     KwTrue,
	"false":    KwFalse,
	"if":       KwIf,
	"elif":     KwElif,
	"else":     KwElse,
	"while":    KwWhile,
	"break":    KwBreak,
	"continue": KwContinue,
	"switch":   KwSwitch,
	"case":     KwCase,
	"default":  KwDefault,
	"struct":   KwStruct,
	"enum":     KwEnum,
	"for":      KwFor,
	"in":       KwIn,
}

// Lexer tokenizes AmiFL source text one Token at a time via Next.
//
// AmiFL is expression-oriented with no statement terminator syntax
// (amifl-spec.md section 5), so unlike Go's automatic-semicolon-insertion
// approach, the lexer emits real Newline tokens and leaves it entirely to
// internal/parser to decide where they matter (block bodies) and where
// they don't (skipped everywhere else) — the same design Weave used
// (see CLAUDE.md's weave_implementation_notes.md reference).
type Lexer struct {
	src  string
	pos  int // byte offset of the next byte to read
	line int
}

// New returns a Lexer positioned at the start of src.
func New(src string) *Lexer {
	return &Lexer{src: src, line: 1}
}

// Next returns the next token and advances the lexer past it. Once the
// input is exhausted it keeps returning an EOF-kind token.
func (l *Lexer) Next() (Token, error) {
	l.skipSpacesAndComments()

	line := l.line
	if l.pos >= len(l.src) {
		return Token{Kind: EOF, Line: line}, nil
	}

	c := l.src[l.pos]
	switch {
	case c == '\n':
		l.pos++
		l.line++
		return Token{Kind: Newline, Line: line}, nil
	case c == '(':
		l.pos++
		return Token{Kind: LParen, Line: line}, nil
	case c == ')':
		l.pos++
		return Token{Kind: RParen, Line: line}, nil
	case c == '{':
		l.pos++
		return Token{Kind: LBrace, Line: line}, nil
	case c == '}':
		l.pos++
		return Token{Kind: RBrace, Line: line}, nil
	case c == '[':
		l.pos++
		return Token{Kind: LBracket, Line: line}, nil
	case c == ']':
		l.pos++
		return Token{Kind: RBracket, Line: line}, nil
	case c == ',':
		l.pos++
		return Token{Kind: Comma, Line: line}, nil
	case c == ';':
		l.pos++
		return Token{Kind: Semicolon, Line: line}, nil
	case c == ':':
		l.pos++
		return Token{Kind: Colon, Line: line}, nil
	case c == '.':
		// A leading '.' never reaches here for a float literal (lexNumber,
		// entered only when the *first* byte is a digit, consumes a '.'
		// itself as part of a number) — so this is always the field-access/
		// tuple-index operator (amifl-spec.md section 3.2, "t.0", step 6).
		l.pos++
		return Token{Kind: Dot, Line: line}, nil
	case c == '=' && l.byteAt(1) == '=':
		l.pos += 2
		return Token{Kind: EqEq, Line: line}, nil
	case c == '=':
		l.pos++
		return Token{Kind: Assign, Line: line}, nil
	case c == '-' && l.byteAt(1) == '>':
		l.pos += 2
		return Token{Kind: Arrow, Line: line}, nil
	case c == '-':
		l.pos++
		return Token{Kind: Minus, Line: line}, nil
	case c == '+':
		l.pos++
		return Token{Kind: Plus, Line: line}, nil
	case c == '*':
		l.pos++
		return Token{Kind: Star, Line: line}, nil
	case c == '/':
		l.pos++
		return Token{Kind: Slash, Line: line}, nil
	case c == '%':
		l.pos++
		return Token{Kind: Percent, Line: line}, nil
	case c == '&' && l.byteAt(1) == '&':
		l.pos += 2
		return Token{Kind: AndAnd, Line: line}, nil
	case c == '&' && l.byteAt(1) == '^':
		l.pos += 2
		return Token{Kind: AmpCaret, Line: line}, nil
	case c == '&':
		l.pos++
		return Token{Kind: Amp, Line: line}, nil
	case c == '|' && l.byteAt(1) == '|':
		l.pos += 2
		return Token{Kind: OrOr, Line: line}, nil
	case c == '|':
		l.pos++
		return Token{Kind: Pipe, Line: line}, nil
	case c == '^':
		l.pos++
		return Token{Kind: Caret, Line: line}, nil
	case c == '~':
		l.pos++
		return Token{Kind: Tilde, Line: line}, nil
	case c == '<' && l.byteAt(1) == '<':
		l.pos += 2
		return Token{Kind: Shl, Line: line}, nil
	case c == '<' && l.byteAt(1) == '=':
		l.pos += 2
		return Token{Kind: Lte, Line: line}, nil
	case c == '<':
		l.pos++
		return Token{Kind: Lt, Line: line}, nil
	case c == '>' && l.byteAt(1) == '>':
		l.pos += 2
		return Token{Kind: Shr, Line: line}, nil
	case c == '>' && l.byteAt(1) == '=':
		l.pos += 2
		return Token{Kind: Gte, Line: line}, nil
	case c == '>':
		l.pos++
		return Token{Kind: Gt, Line: line}, nil
	case c == '!' && l.byteAt(1) == '=':
		l.pos += 2
		return Token{Kind: NotEq, Line: line}, nil
	case c == '!':
		l.pos++
		return Token{Kind: Bang, Line: line}, nil
	case c == '"':
		return l.lexString(line)
	case isDigit(c):
		return l.lexNumber(line), nil
	case isIdentStart(c):
		return l.lexIdent(line), nil
	default:
		r, size := utf8.DecodeRuneInString(l.src[l.pos:])
		l.pos += size
		return Token{}, fmt.Errorf("line %d: unexpected character %q", line, r)
	}
}

// byteAt returns the byte offset bytes ahead of l.pos, or 0 if that is
// past the end of the source (0 never collides with a real token-starting
// byte we compare against here).
func (l *Lexer) byteAt(offset int) byte {
	if l.pos+offset >= len(l.src) {
		return 0
	}
	return l.src[l.pos+offset]
}

func (l *Lexer) skipSpacesAndComments() {
	for l.pos < len(l.src) {
		switch c := l.src[l.pos]; {
		case c == ' ' || c == '\t' || c == '\r':
			l.pos++
		case c == '/' && l.byteAt(1) == '/':
			for l.pos < len(l.src) && l.src[l.pos] != '\n' {
				l.pos++
			}
		default:
			return
		}
	}
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isIdentCont(c byte) bool {
	return isIdentStart(c) || isDigit(c)
}

func (l *Lexer) lexIdent(line int) Token {
	start := l.pos
	for l.pos < len(l.src) && isIdentCont(l.src[l.pos]) {
		l.pos++
	}
	text := l.src[start:l.pos]
	if kw, ok := keywords[text]; ok {
		return Token{Kind: kw, Value: text, Line: line}
	}
	return Token{Kind: Ident, Value: text, Line: line}
}

// lexNumber reads an integer or floating-point literal (amifl-spec.md
// section 3.1: "42 ... 3.14 1.23e4"). A '.' only starts a fractional part
// when followed by a digit — this both rejects a bare trailing '.' and
// leaves room for a future '.' field-access operator (tuples, step 6) to
// never be swallowed by a number literal.
func (l *Lexer) lexNumber(line int) Token {
	start := l.pos
	for l.pos < len(l.src) && isDigit(l.src[l.pos]) {
		l.pos++
	}

	isFloat := false
	if l.pos < len(l.src) && l.src[l.pos] == '.' && isDigit(l.byteAt(1)) {
		isFloat = true
		l.pos++
		for l.pos < len(l.src) && isDigit(l.src[l.pos]) {
			l.pos++
		}
	}

	if l.pos < len(l.src) && (l.src[l.pos] == 'e' || l.src[l.pos] == 'E') {
		if end, ok := l.exponentEnd(l.pos + 1); ok {
			isFloat = true
			l.pos = end
		}
	}

	text := l.src[start:l.pos]
	if isFloat {
		return Token{Kind: Float, Value: text, Line: line}
	}
	return Token{Kind: Int, Value: text, Line: line}
}

// exponentEnd checks for a well-formed exponent suffix ([+-]?digit+)
// starting at pos (just past the 'e'/'E') and, if found, returns the
// offset just past its last digit.
func (l *Lexer) exponentEnd(pos int) (int, bool) {
	if pos < len(l.src) && (l.src[pos] == '+' || l.src[pos] == '-') {
		pos++
	}
	if pos >= len(l.src) || !isDigit(l.src[pos]) {
		return 0, false
	}
	for pos < len(l.src) && isDigit(l.src[pos]) {
		pos++
	}
	return pos, true
}

// lexString reads a "..." literal, decoding the small set of escapes step
// 1 needs. Non-escape bytes are copied verbatim (including UTF-8
// continuation bytes, which are always >= 0x80 and so can never be
// mistaken for the ASCII '\\' or '"' bytes checked below) so multi-byte
// characters in string content pass through untouched without needing
// rune-level decoding here.
func (l *Lexer) lexString(line int) (Token, error) {
	l.pos++ // consume opening quote
	var sb strings.Builder
	for {
		if l.pos >= len(l.src) || l.src[l.pos] == '\n' {
			return Token{}, fmt.Errorf("line %d: unterminated string literal", line)
		}
		c := l.src[l.pos]
		if c == '"' {
			l.pos++
			return Token{Kind: String, Value: sb.String(), Line: line}, nil
		}
		if c == '\\' {
			l.pos++
			if l.pos >= len(l.src) {
				return Token{}, fmt.Errorf("line %d: unterminated string literal", line)
			}
			esc := l.src[l.pos]
			switch esc {
			case 'n':
				sb.WriteByte('\n')
			case 't':
				sb.WriteByte('\t')
			case '\\':
				sb.WriteByte('\\')
			case '"':
				sb.WriteByte('"')
			default:
				return Token{}, fmt.Errorf("line %d: unknown escape sequence \\%c", line, esc)
			}
			l.pos++
			continue
		}
		sb.WriteByte(c)
		l.pos++
	}
}
