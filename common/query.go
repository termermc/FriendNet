package common

import "bytes"

// EscapeQueryString escapes the provided query string to ensure it can be used with SQLite FTS5 queries.
// It removes most syntax by replacing them with spaces.
// It removes all tag queries, except for "tags:".
// The last unclosed quote is removed, if present.
func EscapeQueryString(query string) string {
	str := []byte(query)
	strLen := len(str)
	quoteNum := 0
	lastQuoteIdx := bytes.LastIndexByte(str, '"')

	for i, c := range str {
		switch c {
		case '+':
			fallthrough
		case '^':
			fallthrough
		case '*':
			fallthrough
		case '(':
			fallthrough
		case ')':
			fallthrough
		case '{':
			fallthrough
		case '}':
			fallthrough
		case '[':
			fallthrough
		case ']':
			fallthrough
		case '-':
			fallthrough
		case ',':
			fallthrough
		case '.':
			fallthrough
		case '/':
			fallthrough
		case '\\':
			fallthrough
		case '!':
			fallthrough
		case '?':
			str[i] = ' '
		case '"':
			quoteNum++

			// Replace if this is the last quote and it is unclosed.
			if i == lastQuoteIdx && quoteNum%2 != 0 {
				str[i] = ' '
			}
		case ':':
			str[i] = ' '

			// Check for "NEAR", "AND", "OR", "NOT".
			// Only the first letter needs to be made lowercase to make it plaintext and not a keyword.
		case 'N':
			// NEAR, NOT
			if (i+3 <= strLen && str[i+1] == 'E' && str[i+2] == 'A' && str[i+3] == 'R') ||
				(i+2 <= strLen && str[i+1] == 'O' && str[i+2] == 'T') {
				str[i] = 'n'
			}
		case 'A':
			// AND
			if i+2 <= strLen && str[i+1] == 'N' && str[i+2] == 'D' {
				str[i] = 'a'
			}
		case 'O':
			// OR
			if i+1 <= strLen && str[i+1] == 'R' {
				str[i] = 'o'
			}
		}
	}

	return string(str)
}
