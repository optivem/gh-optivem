package generate

import (
	"fmt"
	"strings"
)

// Token kinds.
const (
	tokIdent    = "ident"
	tokNumber   = "number"
	tokString   = "string"
	tokTemplate = "template"
	tokPunct    = "punct"
)

// token is one lexeme. For strings and templates Text is the content without
// the quotes; for everything else it is the source text.
type token struct {
	Kind string
	Text string
	Line int
}

func (t token) is(kind, text string) bool { return t.Kind == kind && t.Text == text }

func (t token) punct(text string) bool { return t.is(tokPunct, text) }

func (t token) String() string {
	switch t.Kind {
	case tokString:
		return `"` + t.Text + `"`
	case tokTemplate:
		return "`" + t.Text + "`"
	}
	return t.Text
}

// lex tokenizes C-family source (Java, C#, TypeScript): comments are dropped,
// strings and template literals are single tokens, so parentheses and
// semicolons inside them never confuse the structural scan.
func lex(src string) ([]token, error) {
	src = strings.ReplaceAll(src, "\r\n", "\n")
	var toks []token
	line := 1
	i := 0
	for i < len(src) {
		c := src[i]
		switch {
		case c == '\n':
			line++
			i++
		case c == ' ' || c == '\t' || c == '\r' || c == '\f':
			i++
		case strings.HasPrefix(src[i:], "//"):
			for i < len(src) && src[i] != '\n' {
				i++
			}
		case strings.HasPrefix(src[i:], "/*"):
			end := strings.Index(src[i+2:], "*/")
			if end < 0 {
				return nil, fmt.Errorf("line %d: unterminated block comment", line)
			}
			line += strings.Count(src[i:i+2+end], "\n")
			i += end + 4
		case c == '"' || c == '\'':
			text, n, err := scanQuoted(src[i:], c, true)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", line, err)
			}
			toks = append(toks, token{tokString, text, line})
			i += n
		case c == '@' && i+1 < len(src) && src[i+1] == '"': // C# verbatim string
			text, n, err := scanQuoted(src[i+1:], '"', false)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", line, err)
			}
			toks = append(toks, token{tokString, text, line})
			line += strings.Count(src[i:i+1+n], "\n")
			i += 1 + n
		case c == '$' && i+1 < len(src) && src[i+1] == '"': // C# interpolated string
			text, n, err := scanQuoted(src[i+1:], '"', true)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", line, err)
			}
			toks = append(toks, token{tokTemplate, text, line})
			i += 1 + n
		case c == '`':
			text, n, err := scanTemplate(src[i:])
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", line, err)
			}
			toks = append(toks, token{tokTemplate, text, line})
			line += strings.Count(src[i:i+n], "\n")
			i += n
		case isDigit(c):
			j := i
			for j < len(src) && (isIdentByte(src[j]) || src[j] == '.' && j+1 < len(src) && isDigit(src[j+1])) {
				j++
			}
			toks = append(toks, token{tokNumber, src[i:j], line})
			i = j
		case isIdentStart(c):
			j := i
			for j < len(src) && isIdentByte(src[j]) {
				j++
			}
			toks = append(toks, token{tokIdent, src[i:j], line})
			i = j
		default:
			n := 1
			for _, p := range []string{"=>", "->", "::", "...", "?.", "??", "===", "!==", "==", "!="} {
				if strings.HasPrefix(src[i:], p) {
					n = len(p)
					break
				}
			}
			toks = append(toks, token{tokPunct, src[i : i+n], line})
			i += n
		}
	}
	return toks, nil
}

// scanQuoted reads a quoted literal starting at s[0] == q and returns its
// content and the number of bytes consumed. With escapes, a backslash escapes
// the next byte; without (C# verbatim), a doubled quote is a literal quote.
func scanQuoted(s string, q byte, escapes bool) (string, int, error) {
	var b strings.Builder
	for i := 1; i < len(s); i++ {
		c := s[i]
		switch {
		case escapes && c == '\\' && i+1 < len(s):
			b.WriteByte(c)
			b.WriteByte(s[i+1])
			i++
		case !escapes && c == q && i+1 < len(s) && s[i+1] == q:
			b.WriteByte(c)
			i++
		case c == q:
			return b.String(), i + 1, nil
		case escapes && c == '\n':
			return "", 0, fmt.Errorf("unterminated string literal")
		default:
			b.WriteByte(c)
		}
	}
	return "", 0, fmt.Errorf("unterminated string literal")
}

// scanTemplate reads a TS template literal, skipping over `${...}`
// interpolations so a backtick inside one does not end the literal.
func scanTemplate(s string) (string, int, error) {
	depth := 0
	for i := 1; i < len(s); i++ {
		switch {
		case s[i] == '\\':
			i++
		case depth == 0 && s[i] == '`':
			return s[1:i], i + 1, nil
		case strings.HasPrefix(s[i:], "${"):
			depth++
			i++
		case depth > 0 && s[i] == '}':
			depth--
		}
	}
	return "", 0, fmt.Errorf("unterminated template literal")
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }
func isIdentStart(c byte) bool {
	return c == '_' || c == '$' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}
func isIdentByte(c byte) bool { return isIdentStart(c) || isDigit(c) }

// matching returns the index of the bracket that closes toks[open]
// ("(", "[" or "{"), or -1.
func matching(toks []token, open int) int {
	depth := 0
	for i := open; i < len(toks); i++ {
		if toks[i].Kind != tokPunct {
			continue
		}
		switch toks[i].Text {
		case "(", "[", "{":
			depth++
		case ")", "]", "}":
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// splitTop splits toks on sep at bracket depth 0. Empty trailing parts (a
// trailing comma) are dropped.
func splitTop(toks []token, sep string) [][]token {
	var parts [][]token
	depth, start := 0, 0
	for i, t := range toks {
		if t.Kind != tokPunct {
			continue
		}
		switch t.Text {
		case "(", "[", "{":
			depth++
		case ")", "]", "}":
			depth--
		case sep:
			if depth == 0 {
				parts = append(parts, toks[start:i])
				start = i + 1
			}
		}
	}
	if start < len(toks) {
		parts = append(parts, toks[start:])
	}
	return parts
}

func joinTokens(toks []token) string {
	parts := make([]string, len(toks))
	for i, t := range toks {
		parts[i] = t.String()
	}
	return strings.Join(parts, " ")
}
