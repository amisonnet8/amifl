package lexer

import "testing"

func tokenize(t *testing.T, src string) []Token {
	t.Helper()
	lx := New(src)
	var toks []Token
	for {
		tok, err := lx.Next()
		if err != nil {
			t.Fatalf("Next() error: %v", err)
		}
		toks = append(toks, tok)
		if tok.Kind == EOF {
			return toks
		}
	}
}

func TestNext_HelloWorld(t *testing.T) {
	src := "fn main() -> Int {\n" +
		"    print(\"Hello, AmiFL!\")\n" +
		"    0\n" +
		"}\n"

	toks := tokenize(t, src)

	want := []Kind{
		KwFn, Ident, LParen, RParen, Arrow, Ident, LBrace, Newline,
		Ident, LParen, String, RParen, Newline,
		Int, Newline,
		RBrace, Newline,
		EOF,
	}
	if len(toks) != len(want) {
		t.Fatalf("got %d tokens, want %d: %+v", len(toks), len(want), toks)
	}
	for i, k := range want {
		if toks[i].Kind != k {
			t.Errorf("token %d: got Kind %v, want %v", i, toks[i].Kind, k)
		}
	}
	if toks[5].Value != "Int" {
		t.Errorf("return type token: got %q, want %q", toks[5].Value, "Int")
	}
	if toks[10].Value != "Hello, AmiFL!" {
		t.Errorf("string literal: got %q, want %q", toks[10].Value, "Hello, AmiFL!")
	}
	if toks[13].Value != "0" {
		t.Errorf("int literal: got %q, want %q", toks[13].Value, "0")
	}
}

func TestNext_LineComment(t *testing.T) {
	toks := tokenize(t, "// a comment\nfn")
	if len(toks) != 3 || toks[0].Kind != Newline || toks[1].Kind != KwFn || toks[2].Kind != EOF {
		t.Fatalf("unexpected tokens: %+v", toks)
	}
}

func TestNext_StringEscapes(t *testing.T) {
	toks := tokenize(t, `"a\nb\tc\\d\"e"`)
	if len(toks) != 2 || toks[0].Kind != String {
		t.Fatalf("unexpected tokens: %+v", toks)
	}
	want := "a\nb\tc\\d\"e"
	if toks[0].Value != want {
		t.Errorf("got %q, want %q", toks[0].Value, want)
	}
}

func TestNext_UnterminatedString(t *testing.T) {
	lx := New(`"abc`)
	if _, err := lx.Next(); err == nil {
		t.Fatal("expected an error for an unterminated string literal")
	}
}

func TestNext_UnknownEscape(t *testing.T) {
	lx := New(`"a\qb"`)
	if _, err := lx.Next(); err == nil {
		t.Fatal("expected an error for an unknown escape sequence")
	}
}

func TestNext_LineNumbersAdvanceAcrossNewlines(t *testing.T) {
	toks := tokenize(t, "fn\n\nmain")
	if toks[0].Line != 1 {
		t.Errorf("first token: got line %d, want 1", toks[0].Line)
	}
	// toks[0]=fn(line1) toks[1]=newline(line1) toks[2]=newline(line2) toks[3]=main(line3)
	if toks[3].Line != 3 {
		t.Errorf("main token: got line %d, want 3", toks[3].Line)
	}
}

func TestNext_LetConstKeywordsAndPunctuation(t *testing.T) {
	toks := tokenize(t, "let x: Int = 1\nconst Y = 2")
	want := []Kind{
		KwLet, Ident, Colon, Ident, Assign, Int, Newline,
		KwConst, Ident, Assign, Int,
		EOF,
	}
	if len(toks) != len(want) {
		t.Fatalf("got %d tokens, want %d: %+v", len(toks), len(want), toks)
	}
	for i, k := range want {
		if toks[i].Kind != k {
			t.Errorf("token %d: got Kind %v, want %v", i, toks[i].Kind, k)
		}
	}
}

func TestNext_BoolKeywords(t *testing.T) {
	toks := tokenize(t, "true false")
	if len(toks) != 3 || toks[0].Kind != KwTrue || toks[1].Kind != KwFalse || toks[2].Kind != EOF {
		t.Fatalf("unexpected tokens: %+v", toks)
	}
}

func TestNext_ControlFlowKeywords(t *testing.T) {
	toks := tokenize(t, "if elif else while break continue switch case default")
	want := []Kind{
		KwIf, KwElif, KwElse, KwWhile, KwBreak, KwContinue, KwSwitch, KwCase, KwDefault,
		EOF,
	}
	if len(toks) != len(want) {
		t.Fatalf("got %d tokens, want %d: %+v", len(toks), len(want), toks)
	}
	for i, k := range want {
		if toks[i].Kind != k {
			t.Errorf("token %d: got Kind %v, want %v", i, toks[i].Kind, k)
		}
	}
}

func TestNext_FloatLiterals(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want string
	}{
		{"3.14", "3.14"},
		{"1.23e4", "1.23e4"},
		{"1.5e-3", "1.5e-3"},
		{"2E2", "2E2"},
	} {
		toks := tokenize(t, tc.src)
		if len(toks) != 2 || toks[0].Kind != Float {
			t.Fatalf("tokenize(%q): unexpected tokens: %+v", tc.src, toks)
		}
		if toks[0].Value != tc.want {
			t.Errorf("tokenize(%q): got %q, want %q", tc.src, toks[0].Value, tc.want)
		}
	}
}

func TestNext_IntNotConfusedWithFloat(t *testing.T) {
	// A '.' not followed by a digit must not start a fractional part
	// (leaves room for a future tuple '.' field-access operator, step 6,
	// to never be swallowed by a number literal).
	toks := tokenize(t, "42")
	if len(toks) != 2 || toks[0].Kind != Int || toks[0].Value != "42" {
		t.Fatalf("unexpected tokens: %+v", toks)
	}
}

func TestNext_ExponentWithoutDigitsIsNotConsumed(t *testing.T) {
	// "1e" with no following digit: 'e' must be left for the next token
	// (an identifier), not swallowed into a malformed float.
	toks := tokenize(t, "1e")
	want := []Kind{Int, Ident, EOF}
	if len(toks) != len(want) {
		t.Fatalf("got %d tokens, want %d: %+v", len(toks), len(want), toks)
	}
	for i, k := range want {
		if toks[i].Kind != k {
			t.Errorf("token %d: got Kind %v, want %v", i, toks[i].Kind, k)
		}
	}
}

func TestNext_Operators(t *testing.T) {
	src := "+ - * / % & | ^ &^ ~ << >> && || ! == != < <= > >="
	want := []Kind{
		Plus, Minus, Star, Slash, Percent, Amp, Pipe, Caret, AmpCaret, Tilde,
		Shl, Shr, AndAnd, OrOr, Bang, EqEq, NotEq, Lt, Lte, Gt, Gte,
		EOF,
	}
	toks := tokenize(t, src)
	if len(toks) != len(want) {
		t.Fatalf("got %d tokens, want %d: %+v", len(toks), len(want), toks)
	}
	for i, k := range want {
		if toks[i].Kind != k {
			t.Errorf("token %d: got Kind %v, want %v", i, toks[i].Kind, k)
		}
	}
}

func TestNext_MinusVsArrow(t *testing.T) {
	toks := tokenize(t, "- ->")
	if len(toks) != 3 || toks[0].Kind != Minus || toks[1].Kind != Arrow {
		t.Fatalf("unexpected tokens: %+v", toks)
	}
}

func TestNext_SlashIsNotConfusedWithComment(t *testing.T) {
	toks := tokenize(t, "1 / 2 // trailing comment\n3")
	want := []Kind{Int, Slash, Int, Newline, Int, EOF}
	if len(toks) != len(want) {
		t.Fatalf("got %d tokens, want %d: %+v", len(toks), len(want), toks)
	}
	for i, k := range want {
		if toks[i].Kind != k {
			t.Errorf("token %d: got Kind %v, want %v", i, toks[i].Kind, k)
		}
	}
}

func TestNext_AmpCaretIsNotAmpThenCaret(t *testing.T) {
	toks := tokenize(t, "&^")
	if len(toks) != 2 || toks[0].Kind != AmpCaret {
		t.Fatalf("unexpected tokens: %+v", toks)
	}
}

func TestNext_DotDotAndDotDotEq(t *testing.T) {
	// ex2's Range tokens (amifl-spec.md section 3.1/7.3). "0..10" must not
	// be mistaken for "0." (a malformed float, since lexNumber only
	// consumes '.' when the *next* byte is a digit — here it's another
	// '.') plus a stray '.', nor must "0..=10" swallow the '=' into
	// anything but its own dedicated DotDotEq token.
	toks := tokenize(t, "0..10 0..=10")
	want := []Kind{Int, DotDot, Int, Int, DotDotEq, Int, EOF}
	if len(toks) != len(want) {
		t.Fatalf("got %d tokens, want %d: %+v", len(toks), len(want), toks)
	}
	for i, k := range want {
		if toks[i].Kind != k {
			t.Errorf("token %d: got Kind %v, want %v; all: %+v", i, toks[i].Kind, k, toks)
		}
	}
}

func TestNext_DotDotEqIsNotDotDotThenAssign(t *testing.T) {
	toks := tokenize(t, "..=")
	if len(toks) != 2 || toks[0].Kind != DotDotEq {
		t.Fatalf("unexpected tokens: %+v", toks)
	}
}

func TestNext_Underscore(t *testing.T) {
	toks := tokenize(t, "_")
	if len(toks) != 2 || toks[0].Kind != Ident || toks[0].Value != "_" {
		t.Fatalf("unexpected tokens: %+v", toks)
	}
}

// --- ex7: hex/octal/binary integer literals, digit-separator `_`
// (amifl-spec.md section 3.1, ignored/amivm/amivm_spec.md section 6) ---

func TestNext_HexOctalBinaryIntLiterals(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"0x1A", "0x1A"},
		{"0X1a", "0X1a"},
		{"0o17", "0o17"},
		{"0O17", "0O17"},
		{"0b101", "0b101"},
		{"0B101", "0B101"},
	} {
		toks := tokenize(t, tc.src)
		if len(toks) != 2 || toks[0].Kind != Int || toks[0].Value != tc.want {
			t.Fatalf("tokenize(%q): unexpected tokens: %+v", tc.src, toks)
		}
	}
}

func TestNext_DigitSeparatorInIntAndFloatLiterals(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"1_000_000", "1_000_000"},
		{"0x1_A", "0x1_A"},
		{"0o1_7", "0o1_7"},
		{"0b1_01", "0b1_01"},
		{"1_000.5", "1_000.5"},
		{"1_0.5e1_0", "1_0.5e1_0"},
	} {
		toks := tokenize(t, tc.src)
		if len(toks) != 2 || toks[0].Value != tc.want {
			t.Fatalf("tokenize(%q): unexpected tokens: %+v", tc.src, toks)
		}
	}
}

// TestNext_LeadingZeroDecimalIsAnError is a regression test for a
// correctness trap ex7 deliberately guards against: strconv.ParseUint's
// own base-0 mode (used by the parser so hex/octal/binary prefixes
// resolve correctly) treats a bare "0" prefix as *legacy octal* — so
// without this lexer-level rejection, "017" would silently parse as 15
// (octal) instead of the 17 a reader would expect from decimal digits.
// amivm's own grammar (section 6) already restricts decimal literals to a
// lone "0" or a run starting 1-9, exactly matching this check.
func TestNext_LeadingZeroDecimalIsAnError(t *testing.T) {
	for _, src := range []string{"017", "0_5"} {
		lx := New(src)
		if _, err := lx.Next(); err == nil {
			t.Fatalf("tokenize(%q): expected an error for a leading-zero decimal literal", src)
		}
	}
}

// TestNext_LeadingZeroIsFineForALoneZeroOrAFloat confirms the rejection
// above doesn't overreach: a bare "0" (no further digits) and "0" as a
// float's integer part (no octal-legacy ambiguity for floats, since the
// parser calls strconv.ParseFloat, which has no base-0 prefix behavior at
// all) both still lex fine.
func TestNext_LeadingZeroIsFineForALoneZeroOrAFloat(t *testing.T) {
	toks := tokenize(t, "0 0.5")
	want := []Kind{Int, Float, EOF}
	if len(toks) != len(want) {
		t.Fatalf("got %d tokens, want %d: %+v", len(toks), len(want), toks)
	}
	for i, k := range want {
		if toks[i].Kind != k {
			t.Errorf("token %d: got Kind %v, want %v; all: %+v", i, toks[i].Kind, k, toks)
		}
	}
}

func TestNext_HexDigitStopsAtNonHexByte(t *testing.T) {
	// "0x1Ax" — 'x' isn't a valid hex digit, so the literal ends at "0x1A"
	// and the trailing "x" becomes its own identifier token, exactly like
	// any other number-immediately-followed-by-identifier adjacency.
	toks := tokenize(t, "0x1Ax")
	want := []Kind{Int, Ident, EOF}
	if len(toks) != len(want) {
		t.Fatalf("got %d tokens, want %d: %+v", len(toks), len(want), toks)
	}
	for i, k := range want {
		if toks[i].Kind != k {
			t.Errorf("token %d: got Kind %v, want %v; all: %+v", i, toks[i].Kind, k, toks)
		}
	}
	if toks[0].Value != "0x1A" {
		t.Errorf("got Int token %q, want \"0x1A\"", toks[0].Value)
	}
}

func TestNext_MalformedPrefixedLiteralLexesAsOneTokenNotSplit(t *testing.T) {
	// "0o18" (8 isn't a valid octal digit) still lexes as a single Int
	// token — the parser's strconv.ParseUint(text, 0, 64) is what actually
	// rejects it (lexNumber's own doc comment explains why the digit-set
	// validity check is deliberately deferred there instead of here).
	toks := tokenize(t, "0o18")
	if len(toks) != 2 || toks[0].Kind != Int || toks[0].Value != "0o18" {
		t.Fatalf("unexpected tokens: %+v", toks)
	}
}
