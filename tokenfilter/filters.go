package tokenfilter

import (
	"fmt"
	"iter"
	"slices"

	"github.com/cloudspannerecosystem/memefish/token"
)

// StripHints strip token sequences of hints.
// It preserve comments as best effort basis.
func StripHints(seq iter.Seq2[token.Token, error]) iter.Seq2[token.Token, error] {
	return func(yield func(token.Token, error) bool) {
		// Temporary preserved "@" token, it will be released immediately on next token.
		var undeterminedAt *token.Token

		// inHint state is true, @{ <here> }.
		var inHint bool

		// comments of skipped tokens
		var savedComments []token.TokenComment

		for tok, err := range seq {
			// inHint logic is prioritized
			if inHint {
				savedComments = append(savedComments, tok.Comments...)

				if err != nil {
					tok.Comments = savedComments
					_ = yield(tok, fmt.Errorf("unclosed hint with error: %w", err))
					return
				}

				switch tok.Kind {
				case token.TokenEOF:
					tok.Comments = savedComments
					_ = yield(tok, fmt.Errorf("unclosed hint"))
					return
				case "}":
					inHint = false
				}
				continue
			}

			// Saved comments are considered as prefixes of current token comments.
			if len(savedComments) > 0 {
				tok.Comments = append(savedComments, tok.Comments...)
				savedComments = nil
			}

			if undeterminedAt != nil {
				// Both of control flows reset undeterminedAt.

				// Turn inHint true only when @{.
				if tok.Kind == "{" {
					inHint = true

					// discard undeterminedAt and "{", but save comments.
					savedComments = append(slices.Clone(undeterminedAt.Comments), tok.Comments...)
					undeterminedAt = nil
					continue
				}

				// Flush and clear undeterminedAt
				if !yield(*undeterminedAt, nil) {
					return
				}

				undeterminedAt = nil
				// fallthrough
			}

			// returns
			switch {
			case err != nil:
				_ = yield(tok, err)
				return
			case tok.Kind == token.TokenEOF:
				_ = yield(tok, nil)
				return
			default:
				// no action
			}

			switch {
			case tok.Kind == "@":
				undeterminedAt = &tok
				continue
			default:
				if !yield(tok, nil) {
					return
				}
			}
		}
	}
}
