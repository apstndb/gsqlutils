package gsqlutils

import (
	"fmt"
	"github.com/apstndb/gsqlutils/tokenfilter"
	"iter"
	"slices"
	"strings"

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
	s, err := tryUnlexTokenSeq(tokenfilter.StripHints(NewLexerSeq(filepath, s)))
	if err != nil {
		return s, fmt.Errorf("error on SimpleSkipHints, err: %w", err)
	}
	return s, nil
}

// tryUnlexTokenSeqSimple convert seq to string, it ignores whitespaces and comments.
// Token are separated with a single whitespace, except when two tokens are consecutive with no whitespaces in between.
func tryUnlexTokenSeq(seq iter.Seq2[token.Token, error]) (string, error) {
	var b strings.Builder
	prevEnd := token.InvalidPos
	for tok, err := range seq {
		if err != nil {
			return b.String(), err
		}

		if tok.Kind == token.TokenEOF {
			break
		}

		if b.Len() > 0 && tok.Pos != prevEnd {
			b.WriteRune(' ')
		}

		b.WriteString(tok.Raw)
		prevEnd = tok.End
	}
	return b.String(), nil
}

// tryUnlexTokenSeqSimple convert seq to string, it ignores whitespaces and comments.
// Token are separated with a single whitespace, except when two tokens are consecutive with no whitespaces in between.
func tryUnlexTokenSeqWithComments(seq iter.Seq2[token.Token, error]) (string, error) {
	var b strings.Builder
	prevEnd := token.InvalidPos
	for tok, err := range seq {
		if err != nil {
			return b.String(), err
		}

		if len(tok.Comments) > 0 {

		}

		if tok.Kind == token.TokenEOF {
			break
		}

		if b.Len() > 0 && tok.Pos != prevEnd {
			b.WriteRune(' ')
		}

		b.WriteString(tok.Raw)
		prevEnd = tok.End
	}
	return b.String(), nil
}

// SimpleStripComments strips comments in an input string without parsing.
// It don't preserve whitespaces. All tokens are separated with a single whitespace.
// filepath can be empty, it is only used in error message.
//
// [terminating semicolons]: https://cloud.google.com/spanner/docs/reference/standard-sql/lexical#terminating_semicolons
func SimpleStripComments(filepath, s string) (string, error) {
	s, err := tryUnlexTokenSeq(NewLexerSeq(filepath, s))
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
