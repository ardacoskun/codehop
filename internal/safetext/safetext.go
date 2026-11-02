// Package safetext keeps untrusted text (folder names, README lines, commit
// subjects, git errors) from controlling the terminal or forging output.
package safetext

import "strings"

// isUnsafe reports runes that can move the cursor, rewrite the screen, set
// the clipboard (C0/C1 controls, DEL, ESC sequences) or visually reorder
// text (bidirectional overrides and isolates).
func isUnsafe(r rune) bool {
	switch {
	case r < 0x20, r == 0x7f, r >= 0x80 && r <= 0x9f:
		return true
	case r >= 0x202a && r <= 0x202e, r >= 0x2066 && r <= 0x2069:
		return true
	}
	return false
}

// HasUnsafe reports whether s contains any rune Sanitize would replace.
func HasUnsafe(s string) bool {
	return strings.IndexFunc(s, isUnsafe) >= 0
}

// Sanitize makes s safe to print: tabs become spaces and every other unsafe
// rune becomes "?".
func Sanitize(s string) string {
	if !HasUnsafe(s) {
		return s
	}
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\t':
			return ' '
		case isUnsafe(r):
			return '?'
		}
		return r
	}, s)
}
