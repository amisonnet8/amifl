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
	KwIf
	KwElif
	KwElse
	KwWhile
	KwBreak
	KwContinue
	KwReturn
	KwSwitch
	KwCase
	KwDefault
	KwStruct
	KwEnum
	KwFor
	KwIn
	KwYield
	KwExtern
	KwAs
	KwBind
	KwType
	KwImport
	LParen
	RParen
	LBrace
	RBrace
	LBracket
	RBracket
	Arrow
	Comma
	Colon
	Semicolon
	Dot
	DotDot
	DotDotEq
	Assign

	// Operators (amifl-spec.md section 6).
	Plus      // +
	Minus     // -
	Star      // *
	Slash     // /
	Percent   // %
	Amp       // &
	Pipe      // |
	Caret     // ^
	AmpCaret  // &^
	Tilde     // ~
	Shl       // <<
	Shr       // >>
	AndAnd    // &&
	OrOr      // ||
	Bang      // !
	EqEq      // ==
	NotEq     // !=
	Lt        // <
	Lte       // <=
	Gt        // >
	Gte       // >=
	PipeArrow // |>
	Question  // ?
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
	case KwIf:
		return "'if'"
	case KwElif:
		return "'elif'"
	case KwElse:
		return "'else'"
	case KwWhile:
		return "'while'"
	case KwBreak:
		return "'break'"
	case KwContinue:
		return "'continue'"
	case KwReturn:
		return "'return'"
	case KwSwitch:
		return "'switch'"
	case KwCase:
		return "'case'"
	case KwDefault:
		return "'default'"
	case KwStruct:
		return "'struct'"
	case KwEnum:
		return "'enum'"
	case KwFor:
		return "'for'"
	case KwIn:
		return "'in'"
	case KwYield:
		return "'yield'"
	case KwExtern:
		return "'extern'"
	case KwAs:
		return "'as'"
	case KwBind:
		return "'bind'"
	case KwType:
		return "'type'"
	case KwImport:
		return "'import'"
	case LParen:
		return "'('"
	case RParen:
		return "')'"
	case LBrace:
		return "'{'"
	case RBrace:
		return "'}'"
	case LBracket:
		return "'['"
	case RBracket:
		return "']'"
	case Arrow:
		return "'->'"
	case Comma:
		return "','"
	case Colon:
		return "':'"
	case Semicolon:
		return "';'"
	case Dot:
		return "'.'"
	case DotDot:
		return "'..'"
	case DotDotEq:
		return "'..='"
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
	case PipeArrow:
		return "'|>'"
	case Question:
		return "'?'"
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
