package gsqlutils

import (
	"fmt"
	"iter"
	"slices"
	"strings"

	"github.com/apstndb/gsqlutils/internal"
	"github.com/apstndb/gsqlutils/tokenfilter"

	"github.com/cloudspannerecosystem/memefish"
	"github.com/cloudspannerecosystem/memefish/token"
	"github.com/samber/lo"
	"spheric.cloud/xiter"
)

type RawStatement struct {
	Pos, End   token.Pos
	Statement  string
	Terminator string
}

func NewLexerSeq(filename, s string) iter.Seq2[token.Token, error] {
	return LexerSeq(newLexer(filename, s))
}

// LexerSeq converts memefish.Lexer to iter.Seq2 with error.
// If it reaches to EOF, it stops without error.
func LexerSeq(lexer *memefish.Lexer) iter.Seq2[token.Token, error] {
	return func(yield func(token.Token, error) bool) {
		for {
			if err := lexer.NextToken(); err != nil {
				_ = yield(lexer.Token, err)
				return
			}

			if lexer.Token.Kind == token.TokenEOF {
				_ = yield(lexer.Token, nil)
				return
			}

			if !yield(lexer.Token, nil) {
				return
			}
		}
	}
}

func (stmt *RawStatement) StripComments() (RawStatement, error) {
	result, err := StripComments("", stmt.Statement)

	// It can assume InputStatement.Statement doesn't have any terminating characters.
	return RawStatement{
		Statement:  result,
		Terminator: stmt.Terminator,
	}, err
}

type ErrLexerStatus struct {
	WaitingString string
}

func (e *ErrLexerStatus) Error() string {
	return fmt.Sprintf("lexer error with waiting: %v", e.WaitingString)
}

func SeparateInputPreserveCommentsWithStatus(filepath, s string) ([]RawStatement, error) {
	lexer := newLexer(filepath, s)

	var results []RawStatement
	var pos token.Pos
outer:
	for tok, err := range LexerSeq(lexer) {
		if err != nil {
			if err, ok := lo.ErrorsAs[*memefish.Error](err); ok {
				results = append(results, RawStatement{Pos: pos, End: err.Position.End, Statement: lexer.Buffer[pos:err.Position.End]})
				return results, toErrLexerStatus(err, lexer.Buffer[tok.Pos:])
			}
			return results, err
		}

		// renew pos to first comment or first token of a statement.
		if pos.Invalid() {
			tokenComment, ok := lo.First(tok.Comments)
			pos = lo.Ternary(ok, tokenComment.Pos, tok.Pos)
		}

		switch tok.Kind {
		case token.TokenEOF:
			// If pos:tok.Pos is not empty, add remaining part of buffer to result.
			if pos != tok.Pos {
				results = append(results, RawStatement{Statement: s[pos:tok.Pos], Pos: pos, End: tok.Pos})
			}
			// no need to continue
			break outer
		case ";":
			results = append(results, RawStatement{Statement: s[pos:tok.Pos], Pos: pos, End: tok.End, Terminator: ";"})
			pos = token.InvalidPos
		default:
		}
	}
	return results, nil
}

const errMessageUnclosedTripleQuotedStringLiteral = `unclosed triple-quoted string literal`
const errMessageUnclosedComment = `unclosed comment`

// NOTE: memefish.Error.Message can be changed.
func toErrLexerStatus(err *memefish.Error, head string) error {
	switch {
	case err.Message == errMessageUnclosedTripleQuotedStringLiteral && strings.HasPrefix(head, `"""`):
		return &ErrLexerStatus{WaitingString: `"""`}
	case err.Message == errMessageUnclosedTripleQuotedStringLiteral:
		return &ErrLexerStatus{WaitingString: `'''`}
	case err.Message == errMessageUnclosedComment:
		return &ErrLexerStatus{WaitingString: `*/`}
	default:
		return err
	}
}

// StripComments strips comments in an input string without parsing but preserving whitespaces.
// This function won't panic but return error if lexer become error state.
// filepath can be empty, it is only used in error message.
//
// [terminating semicolons]: https://cloud.google.com/spanner/docs/reference/standard-sql/lexical#terminating_semicolons
func StripComments(filepath, s string) (string, error) {
	// TODO: refactor
	var b strings.Builder
	var prevEnd token.Pos
	var stmtFirstPos token.Pos
	for tok, err := range NewLexerSeq(filepath, s) {
		if err != nil {
			return "", err
		}

		if tok.Kind == ";" {
			stmtFirstPos = tok.End
		}

		if comment, ok := lo.First(tok.Comments); ok {
			// flush all string before comments
			b.WriteString(s[prevEnd:comment.Pos])
			if tok.Kind == token.TokenEOF {
				// no need to continue
				break
			}

			it := xiter.Filter(slices.Values(tok.Comments), func(comment token.TokenComment) bool {
				raw := comment.Raw
				switch {
				case stmtFirstPos != comment.Pos:
					return true
				case comment.Space != "":
					return true
				// rest are ignorable.
				case strings.HasPrefix(raw, "--") || strings.HasPrefix(raw, "#"):
					return false
				default:
					return false
				}
			})

			hasNewline := xiter.Any(it, func(comment token.TokenComment) bool {
				return strings.ContainsAny(comment.Raw, "\n")
			})
			if stmtFirstPos != comment.Pos {
				// Unless the comment is placed at the head of statement, comments will be a whitespace.
				if hasNewline {
					b.WriteString("\n")
				} else {
					b.WriteString(" ")
				}
			}

			b.WriteString(tok.Raw)
			prevEnd = tok.End
		}

		// flush EOF
		if tok.Kind == token.TokenEOF {
			b.WriteString(s[prevEnd:tok.Pos])
			break
		}

	}
	return b.String(), nil
}

// FirstNonHintToken returns the first non-hint token.
// filepath can be empty, it is only used in error message.
// Note: Currently, this function doesn't take care about nested curly brace in a hint.
func FirstNonHintToken(filepath, s string) (token.Token, error) {
	lexer := newLexer(filepath, s)

	next, stop := iter.Pull2(tokenfilter.StripHints(LexerSeq(lexer)))
	defer stop()

	tok, err, _ := next()
	if err != nil {
		return tok, fmt.Errorf("can't get first token, err: %w, position: %v", err, lexer.Position(tok.Pos, tok.End))
	}

	return tok, nil
}

// SimpleSkipHints strips hints in an input string without parsing.
// It don't preserve any hints and comments and whitespaces. All tokens are separated with a single whitespace.
// filepath can be empty, it is only used in error message.
func SimpleSkipHints(filepath, s string) (string, error) {
	s, err := tryUnlexTokenSeq(true, tokenfilter.StripHints(NewLexerSeq(filepath, s)))
	if err != nil {
		return s, fmt.Errorf("error on SimpleSkipHints, err: %w", err)
	}
	return s, nil
}

var invalidToken = token.Token{Kind: token.TokenBad, Pos: token.InvalidPos, End: token.InvalidPos}

func nthToken[N internal.Integer](tokens []token.Token, nth N) token.Token {
	return internal.NthOr(tokens, nth, invalidToken)
}

func prevKind[N internal.Integer](tokens []token.Token, prevNth N) token.TokenKind {
	return nthToken(tokens, -prevNth).Kind
}

type tokenList []token.Token

func (t tokenList) prevKind(prevNth int) token.TokenKind {
	return prevKind(t, prevNth)
}

// tryUnlexTokenSeqSimple convert seq to string, it ignores whitespaces and comments.
// Token are separated with a single whitespace, except when two tokens are consecutive with no whitespaces in between.
func tryUnlexTokenSeq(newlineOnSemicolon bool, seq iter.Seq2[token.Token, error]) (string, error) {
	tokens := tokenList(nil)

	var b strings.Builder
	prev := token.Token{Pos: token.InvalidPos, End: token.InvalidPos}

	// Count "{" level in hint
	inHintLevel := 0

	// Count "<" level in compound type
	compoundTypeLevel := 0
	for tok, err := range seq {
		if err != nil {
			return b.String(), err
		}

		if tok.Kind == token.TokenEOF {
			break
		}

		if (compoundTypeLevel > 0 || prev.Kind == "ARRAY" || prev.Kind == "STRUCT") && tok.Kind == "<" {
			compoundTypeLevel++
		}

		if (prev.Kind == "@" || inHintLevel > 0) && tok.Kind == "{" {
			inHintLevel++
		}

		// TODO in-hint
		if b.Len() > 0 {
			if prev.Kind == ";" {
				b.WriteRune(lo.Ternary(newlineOnSemicolon, '\n', ' '))

				// after open or dot
			} else if !internal.OneOf(prev.Kind, "(", "{", "[", ".") &&
				// before close or dot, comma, colon
				!internal.OneOf(tok.Kind, ")", "}", "]", ".", ",", ":", ";") &&

				// hint
				!(tok.Kind == "@" && internal.OneOf(prev.Kind, ")", token.TokenIdent)) &&
				!(prev.Kind == "@" && tok.Kind == "{") &&

				// '=' in hint
				!(internal.OneOf(prev.Kind, "@") && internal.OneOf(tok.Kind, "{")) &&
				!(inHintLevel > 0 && prev.Kind == "=") &&
				!(inHintLevel > 0 && tok.Kind == "=") &&

				// '<' & '>' in compound types
				!(internal.OneOf(prev.Kind, "STRUCT", "ARRAY") && internal.OneOf(tok.Kind, "<", "<>")) &&
				!(compoundTypeLevel > 0 && prev.Kind == "<") &&
				!(compoundTypeLevel > 0 && internal.OneOf(tok.Kind, ">", ">>")) &&

				// UNNEST, WITH expression
				!(tok.Kind == "(" && internal.OneOf(prev.Kind, "UNNEST", "WITH", "STRUCT", "ARRAY", "CAST")) &&

				// subscript expression
				!(tok.Kind == "[") &&

				// unary minus
				!(prev.Kind == "-" && internal.OneOf(tokens.prevKind(-2), "[", "{", "<", "(", ",", token.TokenBad)) &&

				// "identifier(" can be function calls, it is natural not to be separated by whitespace, preserve original.
				// Note: STORING () should be separated by whitespaces
				!(tok.Kind == "(" && prev.Kind == token.TokenIdent &&
					!prev.IsKeywordLike("STORING") &&
					tok.Pos == prev.End) {
				b.WriteRune(' ')
			}
		}

		if compoundTypeLevel > 0 {
			switch {
			case tok.Kind == ">":
				compoundTypeLevel -= 1
			case tok.Kind == ">>":
				compoundTypeLevel -= 2
			}
		}

		if inHintLevel > 0 && tok.Kind == "}" {
			inHintLevel--
		}

		b.WriteString(tok.Raw)

		prev = tok

		if tok.Kind == ";" {
			tokens = tokenList(nil)
		}
		tokens = append(tokens, tok)
	}
	return b.String(), nil
}

// tryUnlexTokenSeqSimple convert seq to string, it ignores whitespaces and comments.
// Token are separated with a single whitespace, except when two tokens are consecutive with no whitespaces in between.
func tryUnlexTokenSeqWithComments(seq iter.Seq2[token.Token, error]) (string, error) {
	var b strings.Builder
	prev := token.Token{End: token.InvalidPos}
	for tok, err := range seq {
		if err != nil {
			return b.String(), err
		}

		if tok.Kind == token.TokenEOF {
			break
		}

		if b.Len() > 0 && (slices.Contains([]token.TokenKind{}, prev.Kind)) && tok.Pos != prev.End {
			b.WriteRune(' ')
		}

		b.WriteString(tok.Raw)

		prev = tok
	}
	return b.String(), nil
}

// SimpleStripComments strips comments in an input string without parsing.
// It don't preserve whitespaces. All tokens are separated with a single whitespace.
// filepath can be empty, it is only used in error message.
//
// [terminating semicolons]: https://cloud.google.com/spanner/docs/reference/standard-sql/lexical#terminating_semicolons
func SimpleStripComments(filepath, s string) (string, error) {
	s, err := tryUnlexTokenSeq(true, NewLexerSeq(filepath, s))
	if err != nil {
		return s, fmt.Errorf("error on SimpleStripComments, err: %w", err)
	}
	return s, nil
}

func newLexer(filepath string, s string) *memefish.Lexer {
	return &memefish.Lexer{
		File: &token.File{
			FilePath: filepath,
			Buffer:   s,
		},
	}
}
