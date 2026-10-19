//go:build linux

package steam

import "strings"

// vdfTokens returns the quoted strings of a Valve KeyValues (VDF) file, in
// order. Steam's .vdf and .acf files are a sequence of "key" "value" pairs
// inside braces; reading just the quoted tokens is enough to pick out the keys
// and values this package needs, without a full parser. Backslash escapes
// inside a quoted string are honoured.
func vdfTokens(s string) []string {
	var out []string
	var b strings.Builder
	inQuote := false
	escaped := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inQuote {
			switch {
			case escaped:
				b.WriteByte(c)
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				out = append(out, b.String())
				b.Reset()
				inQuote = false
			default:
				b.WriteByte(c)
			}
			continue
		}
		if c == '"' {
			inQuote = true
		}
	}
	return out
}

// vdfValue returns the value that follows the first occurrence of key in
// toks, or "" when the key is absent.
func vdfValue(toks []string, key string) string {
	for i := 0; i+1 < len(toks); i++ {
		if toks[i] == key {
			return toks[i+1]
		}
	}
	return ""
}
