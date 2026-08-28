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
