package gsqlutils

import (
	"github.com/cloudspannerecosystem/memefish"
	"github.com/cloudspannerecosystem/memefish/token"
	"github.com/k0kubun/pp"
	"strings"
)

type RawStatement struct {
	Pos, End  token.Pos
	Statement string
}

// StripComments strips comments in an input string without parsing.
// This function won't panic but return error if lexer become error state.
// filepath can be empty, it is only used in error message.
//
// [terminating semicolons]: https://cloud.google.com/spanner/docs/reference/standard-sql/lexical#terminating_semicolons
func StripComments(filepath, s string) (string, error) {
	lex := &memefish.Lexer{
		File: &token.File{
			FilePath: filepath,
			Buffer:   s,
		},
	}

	var b strings.Builder
	var firstPos token.Pos
	for {
		_ = firstPos
		if len(lex.Token.Comments) > 0 {
			// flush
			b.WriteString(s[firstPos:lex.Token.Comments[0].Pos])
			if lex.Token.Kind == token.TokenEOF {
				// no need to continue
				break
			}
			var commentStrBuilder strings.Builder
			var hasNewline bool
			for _, comment := range lex.Token.Comments {
				commentStrBuilder.WriteString(comment.Space)
				if strings.ContainsAny(comment.Raw, "\n") {
					hasNewline = true
				}
			}
			commentStr := strings.TrimSpace(commentStrBuilder.String())
			if commentStr != "" {
				b.WriteString(commentStr)
			} else {
				if hasNewline {
					b.WriteString("\n")
				} else {
					b.WriteString(" ")
				}
			}
			b.WriteString(lex.Token.Raw)
			firstPos = lex.Token.End
		}

		if lex.Token.Kind == token.TokenEOF {
			b.WriteString(s[firstPos:lex.Token.Pos])
			break
		}

		err := lex.NextToken()
		if err != nil {
			return "", err
		}
		pp.Println(lex.Token)
		/*

			if lex.Token.Kind == token.TokenEOF {
				if lex.Token.End != firstPos {
					b.WriteString(s[firstPos:lex.Token.Pos])
				}
				break
			}
		*/
	}
	return b.String(), nil
}
