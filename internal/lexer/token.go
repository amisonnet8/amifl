package lexer

// Kind identifies the lexical category of a Token.
type Kind int

const (
	EOF Kind = iota
	Newline
	Ident
	Int
	Float
	String
	KwFn
	KwLet
	KwConst
	KwTrue
	KwFalse
	LParen
	RParen
	LBrace
	RBrace
	Arrow
	Comma
	Colon
	Assign

	// Operators (amifl-spec.md section 6).
	Plus     // +
	Minus    // -
	Star     // *
	Slash    // /
	Percent  // %
	Amp      // &
	Pipe     // |
	Caret    // ^
	AmpCaret // &^
	Tilde    // ~
	Shl      // <<
	Shr      // >>
	AndAnd   // &&
	OrOr     // ||
	Bang     // !
	EqEq     // ==
	NotEq    // !=
	Lt       // <
	Lte      // <=
	Gt       // >
	Gte      // >=
)

func (k Kind) String() string {
	switch k {
	case EOF:
		return "end of file"
	case Newline:
		return "newline"
	case Ident:
		return "identifier"
	case Int:
		return "integer literal"
	case Float:
		return "float literal"
	case String:
		return "string literal"
	case KwFn:
		return "'fn'"
	case KwLet:
		return "'let'"
	case KwConst:
		return "'const'"
	case KwTrue:
		return "'true'"
	case KwFalse:
		return "'false'"
	case LParen:
		return "'('"
	case RParen:
		return "')'"
	case LBrace:
		return "'{'"
	case RBrace:
		return "'}'"
	case Arrow:
		return "'->'"
	case Comma:
		return "','"
	case Colon:
		return "':'"
	case Assign:
		return "'='"
	case Plus:
		return "'+'"
	case Minus:
		return "'-'"
	case Star:
		return "'*'"
	case Slash:
		return "'/'"
	case Percent:
		return "'%'"
	case Amp:
		return "'&'"
	case Pipe:
		return "'|'"
	case Caret:
		return "'^'"
	case AmpCaret:
		return "'&^'"
	case Tilde:
		return "'~'"
	case Shl:
		return "'<<'"
	case Shr:
		return "'>>'"
	case AndAnd:
		return "'&&'"
	case OrOr:
		return "'||'"
	case Bang:
		return "'!'"
	case EqEq:
		return "'=='"
	case NotEq:
		return "'!='"
	case Lt:
		return "'<'"
	case Lte:
		return "'<='"
	case Gt:
		return "'>'"
	case Gte:
		return "'>='"
	default:
		return "unknown token"
	}
}

// Token is a single lexical token, together with the 1-based source line
// it starts on and, for Ident/Int/Float/String, its text (already
// unescaped for String).
type Token struct {
	Kind  Kind
	Value string
	Line  int
}
