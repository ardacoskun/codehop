// Package argv splits user-written command lines into arguments.
package argv

import (
	"errors"
	"strings"
)

// Split splits s on whitespace, honoring single and double quotes so values
// with spaces can be written as "C:\Program Files\x". Backslashes are
// literal, which keeps Windows paths intact.
func Split(s string) ([]string, error) {
	var (
		fields  []string
		cur     strings.Builder
		quote   rune
		inField bool
	)
	for _, r := range s {
		switch {
		case quote != 0 && r == quote:
			quote = 0
		case quote != 0:
			cur.WriteRune(r)
		case r == '"' || r == '\'':
			quote, inField = r, true
		case r == ' ' || r == '\t' || r == '\n':
			if inField {
				fields = append(fields, cur.String())
				cur.Reset()
				inField = false
			}
		default:
			cur.WriteRune(r)
			inField = true
		}
	}
	if quote != 0 {
		return nil, errors.New("unterminated quote")
	}
	if inField {
		fields = append(fields, cur.String())
	}
	return fields, nil
}
