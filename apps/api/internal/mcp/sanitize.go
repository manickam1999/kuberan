package mcp

import (
	"strconv"
	"strings"
	"unicode"
)

// maxUntrustedRunes caps how much of one user-controlled string a tool echoes
// back to the model.
const maxUntrustedRunes = 200

// untrusted renders a user-controlled string (a description, name,
// counterparty or note) for tool output. Tool results are read by a language
// model, so text a third party can influence (a merchant name on an imported
// statement, a shared expense's counterparty) must not be able to pose as tool
// output or as instructions. untrusted:
//
//   - replaces control and format characters (newlines, tabs, ANSI escapes,
//     bidi overrides, zero-width characters) with spaces, so a value cannot
//     start a new line of output;
//   - collapses runs of whitespace and trims the ends;
//   - caps the result at maxUntrustedRunes runes, marking a cut with "...";
//   - wraps it in double quotes (escaping any inside), so the value is visibly
//     delimited data.
//
// Every tool that prints a user-controlled string uses it; IDs, enums, dates
// and amounts come from the server and are printed as-is.
func untrusted(s string) string {
	return strconv.Quote(cleanText(s, maxUntrustedRunes))
}

// cleanText strips control and format characters, collapses whitespace and
// caps s at limit runes (adding "..." when cut). Invalid UTF-8 is replaced.
func cleanText(s string, limit int) string {
	var sb strings.Builder
	pendingSpace := false
	n := 0
	truncated := false
	for _, r := range strings.ToValidUTF8(s, "\uFFFD") {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || unicode.IsSpace(r) ||
			r == '\u2028' || r == '\u2029' {
			pendingSpace = true
			continue
		}
		need := 1
		if pendingSpace && n > 0 {
			need = 2
		}
		if n+need > limit {
			truncated = true
			break
		}
		if need == 2 {
			sb.WriteByte(' ')
		}
		sb.WriteRune(r)
		n += need
		pendingSpace = false
	}
	if truncated {
		return sb.String() + "..."
	}
	return sb.String()
}
