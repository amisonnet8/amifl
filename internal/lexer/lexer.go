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
	"return":   KwReturn,
	"switch":   KwSwitch,
	"case":     KwCase,
	"default":  KwDefault,
	"struct":   KwStruct,
	"enum":     KwEnum,
	"for":      KwFor,
	"in":       KwIn,
	"yield":    KwYield,
	"extern":   KwExtern,
	"as":       KwAs,
	"bind":     KwBind,
	"type":     KwType,
	"import":   KwImport,
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
	case c == '.' && l.byteAt(1) == '.' && l.byteAt(2) == '=':
		// `a..=b`, ex2's inclusive Range bound (amifl-spec.md section 3.1/
		// 7.3) — checked before the plain ".." case below since both share
		// their first two bytes.
		l.pos += 3
		return Token{Kind: DotDotEq, Line: line}, nil
	case c == '.' && l.byteAt(1) == '.':
		// `a..b`, ex2's half-open Range (amifl-spec.md section 3.1/7.3).
		// Never mistaken for two adjacent float literals' fractional dots
		// (lexNumber only ever consumes a single '.' immediately following
		// a digit run) or for a trailing '.' after an integer (lexNumber's
		// own isDigit(byteAt(1)) guard leaves a bare "0.." with its second
		// '.' unconsumed, landing here rather than misparsing as "0." plus
		// a stray '.').
		l.pos += 2
		return Token{Kind: DotDot, Line: line}, nil
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
	case c == '|' && l.byteAt(1) == '>':
		l.pos += 2
		return Token{Kind: PipeArrow, Line: line}, nil
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
	case c == '?':
		l.pos++
		return Token{Kind: Question, Line: line}, nil
	case c == '"':
		return l.lexString(line)
	case isDigit(c):
		return l.lexNumber(line)
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
// section 3.1: "42 ... 3.14 1.23e4 0x1A 0o17 0b101 1_000_000", ex7). A '.'
// only starts a fractional part when followed by a digit — this both
// rejects a bare trailing '.' and leaves room for the '.' field-access
// operator (tuples, step 6) to never be swallowed by a number literal.
//
// ex7 (ignored/amivm/amivm_spec.md section 6, upgraded alongside this
// change) added hex (`0x1A`)/octal (`0o17`)/binary (`0b101`) integer forms
// and a digit-separator `_` — the parser hands the raw token text straight
// to strconv.ParseUint(text, 0, 64)/ParseFloat(text, 64) (base 0 so the
// prefix picks the base), which already implements amivm's exact
// underscore-placement rules (no leading/trailing/doubled `_`, matching
// Go's own numeric-literal grammar) and reports a clear "invalid syntax"
// error for a malformed one — so this lexer deliberately does *not*
// re-validate underscore placement or a base's own digit set itself
// (`0o18`/`0b12` lex as one token same as a well-formed one, then fail at
// strconv.ParseUint) — it only needs to find the token's *end*.
func (l *Lexer) lexNumber(line int) (Token, error) {
	start := l.pos

	// Prefixed literal: 0x/0X (hex) 0o/0O (octal) 0b/0B (binary). None of
	// these has a floating-point form (no hex/octal/binary float syntax in
	// AmiFL, matching amivm's own grammar), so the token ends the moment
	// the digit-or-underscore run does — no '.'/exponent check needed.
	if l.src[l.pos] == '0' {
		var isValidDigit func(byte) bool
		switch l.byteAt(1) {
		case 'x', 'X':
			isValidDigit = isHexDigit
		case 'o', 'O', 'b', 'B':
			isValidDigit = isDigit
		}
		if isValidDigit != nil {
			l.pos += 2
			l.consumeDigitsAndUnderscores(isValidDigit)
			return Token{Kind: Int, Value: l.src[start:l.pos], Line: line}, nil
		}
	}

	l.consumeDigitsAndUnderscores(isDigit)

	isFloat := false
	if l.pos < len(l.src) && l.src[l.pos] == '.' && isDigit(l.byteAt(1)) {
		isFloat = true
		l.pos++
		l.consumeDigitsAndUnderscores(isDigit)
	}

	if l.pos < len(l.src) && (l.src[l.pos] == 'e' || l.src[l.pos] == 'E') {
		if end, ok := l.exponentEnd(l.pos + 1); ok {
			isFloat = true
			l.pos = end
		}
	}

	text := l.src[start:l.pos]
	if isFloat {
		return Token{Kind: Float, Value: text, Line: line}, nil
	}
	// amivm's decimal-integer grammar only allows a lone "0", or a run
	// starting 1-9 (no other leading zero) — anything else starting with
	// '0' (e.g. "017", "0_5") would otherwise silently reach the parser's
	// strconv.ParseUint(text, 0, 64) and reparse as *legacy octal* (Go's
	// base-0 rule: a bare "0" prefix means octal) instead of the decimal
	// value it looks like — a correctness trap caught here, at lex time,
	// rather than producing a silently wrong Value.
	if len(text) > 1 && text[0] == '0' {
		return Token{}, fmt.Errorf("line %d: a decimal integer literal can't start with a leading zero (%q) — use 0o for octal, or drop the leading zero", line, text)
	}
	return Token{Kind: Int, Value: text, Line: line}, nil
}

// consumeDigitsAndUnderscores advances l.pos past a maximal run of bytes
// each satisfying isValidDigit or equal to '_' — see lexNumber's doc
// comment for why underscore placement isn't validated here.
func (l *Lexer) consumeDigitsAndUnderscores(isValidDigit func(byte) bool) {
	for l.pos < len(l.src) && (isValidDigit(l.src[l.pos]) || l.src[l.pos] == '_') {
		l.pos++
	}
}

func isHexDigit(c byte) bool {
	return isDigit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// exponentEnd checks for a well-formed exponent suffix ([+-]?digit+,
// digits allowed '_'-separated same as everywhere else, ex7) starting at
// pos (just past the 'e'/'E') and, if found, returns the offset just past
// its last digit.
func (l *Lexer) exponentEnd(pos int) (int, bool) {
	if pos < len(l.src) && (l.src[pos] == '+' || l.src[pos] == '-') {
		pos++
	}
	if pos >= len(l.src) || !isDigit(l.src[pos]) {
		return 0, false
	}
	for pos < len(l.src) && (isDigit(l.src[pos]) || l.src[pos] == '_') {
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
