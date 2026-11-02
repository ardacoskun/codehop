package expand

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

type tokenKind int

const (
	wordToken tokenKind = iota
	// separatorToken ends a simple command: ; & && || | |& newline.
	separatorToken
	// redirectToken is < > >> &> and similar; the next word is a file.
	redirectToken
)

type token struct {
	kind       tokenKind
	start, end int
	text       string
	// plain words contain no quotes, escapes, variables or globs, so they
	// mean exactly what they say and can be replaced safely.
	plain bool
}

// tokenize splits a command line into words and operators. It returns
// errUnsupported for syntax whose words it cannot place reliably: subshells
// and substitutions ( ) $( ) ` `, PowerShell blocks { }, heredocs and
// unbalanced quotes. Callers then leave the line alone.
func tokenize(line string, d Dialect) ([]token, error) {
	if hasUnsupported(line, d) {
		return nil, errUnsupported
	}
	var tokens []token
	i := 0
	for i < len(line) {
		c := line[i]
		switch {
		case c == ' ' || c == '\t' || c == '\r':
			i++
		case c == '\n':
			tokens = append(tokens, token{kind: separatorToken, start: i, end: i + 1, text: "\n"})
			i++
		case c == '#':
			// A comment runs to the end of the line.
			for i < len(line) && line[i] != '\n' {
				i++
			}
		case c == '(' || c == ')' || (d == PowerShell && (c == '{' || c == '}')) || (d == Posix && c == '`'):
			return nil, errUnsupported
		case c == ';' || c == '|' || c == '&' || c == '<' || c == '>':
			t, err := operator(line, i, d)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, t)
			i = t.end
		default:
			t, err := word(line, i, d)
			if err != nil {
				return nil, err
			}
			// "2>file": a number glued to a redirection is a file descriptor.
			if t.end < len(line) && (line[t.end] == '<' || line[t.end] == '>') {
				t.plain = false
			}
			tokens = append(tokens, t)
			i = t.end
		}
	}
	return tokens, nil
}

// unsupportedAnywhere are constructs that can hide quotes or code from the
// tokenizer (substitutions inside double quotes, ANSI-C strings, PowerShell
// here-strings and typographic quotes). Lines containing them are left alone.
var unsupportedAnywhere = map[Dialect][]string{
	Posix:      {"$(", "${", "`", "$'", "$\""},
	Fish:       {"$(", "${"},
	PowerShell: {"$(", "${", "@(", "@{", "@'", "@\"", "\u2018", "\u2019", "\u201a", "\u201b", "\u201c", "\u201d", "\u201e"},
}

func hasUnsupported(line string, d Dialect) bool {
	// PowerShell's stop-parsing token passes the rest of the line verbatim.
	if d == PowerShell && strings.Contains(line, "--%") {
		return true
	}
	for _, s := range unsupportedAnywhere[d] {
		if strings.Contains(line, s) {
			return true
		}
	}
	return false
}

func operator(line string, i int, d Dialect) (token, error) {
	two := ""
	if i+1 < len(line) {
		two = line[i : i+2]
	}
	switch {
	case two == "<<" && d != PowerShell:
		return token{}, errUnsupported // heredoc or here-string
	case two == "&&" || two == "||" || two == "|&" || two == ";;":
		return token{kind: separatorToken, start: i, end: i + 2, text: two}, nil
	case two == ">>" || two == "&>" || two == ">&" || two == "<>" || two == ">|":
		return token{kind: redirectToken, start: i, end: i + 2, text: two}, nil
	case line[i] == '<' || line[i] == '>':
		return token{kind: redirectToken, start: i, end: i + 1, text: line[i : i+1]}, nil
	}
	return token{kind: separatorToken, start: i, end: i + 1, text: line[i : i+1]}, nil
}

// word reads one word starting at i, honoring the dialect's quotes and
// escapes, and decides whether it is plain.
func word(line string, i int, d Dialect) (token, error) {
	start := i
	plain := true
	for i < len(line) {
		c := line[i]
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n' ||
			c == ';' || c == '|' || c == '&' || c == '<' || c == '>' || c == '(' || c == ')':
			return token{kind: wordToken, start: start, end: i, text: line[start:i], plain: plain}, nil
		case d == PowerShell && (c == '{' || c == '}'):
			return token{}, errUnsupported
		case c == '\'' || c == '"':
			end, ok := closingQuote(line, i, d)
			if !ok {
				return token{}, errUnsupported
			}
			plain = false
			i = end + 1
		case escapeChar(c, d):
			plain = false
			i += 2
		case c == '`' && d == Posix:
			return token{}, errUnsupported
		default:
			r, size := utf8.DecodeRuneInString(line[i:])
			if !plainRune(r, d) {
				plain = false
			}
			i += size
		}
	}
	return token{kind: wordToken, start: start, end: len(line), text: line[start:], plain: plain}, nil
}

func escapeChar(c byte, d Dialect) bool {
	if d == PowerShell {
		return c == '`'
	}
	return c == '\\'
}

// closingQuote finds the quote closing the one at i.
func closingQuote(line string, i int, d Dialect) (int, bool) {
	q := line[i]
	for j := i + 1; j < len(line); j++ {
		c := line[j]
		switch {
		case c == q && d == PowerShell && j+1 < len(line) && line[j+1] == q:
			j++ // '' or "" inside a PowerShell string
		case c == q:
			return j, true
		case q == '"' && escapeChar(c, d), q == '\'' && d == Fish && c == '\\':
			j++
		}
	}
	return 0, false
}

// plainRune reports characters that are literal in every position of a word.
func plainRune(r rune, d Dialect) bool {
	if unicode.IsLetter(r) || unicode.IsDigit(r) {
		return true
	}
	switch r {
	case '.', '_', '-', '+', '/', '@', ',', ':', '%', '^', '=':
		return true
	case '\\':
		return d == PowerShell
	}
	return false
}

// unsafePathChars can change meaning when a quoted path ends up inside a
// context the shell parses differently (a heredoc, a pasted typographic
// quote). Paths containing them are never written into a command line.
const unsafePathChars = "$`'\"\n\r\u2018\u2019\u201a\u201b\u201c\u201d\u201e"

// render writes path+rest as a single word of the dialect: unquoted when
// every character is literal (using ~ for the home directory where the shell
// expands it), single-quoted otherwise. ok is false for paths that are not
// safe to write at all.
func render(path, rest string, opts Options) (text string, ok bool) {
	full := path + rest
	if strings.ContainsAny(full, unsafePathChars) || (opts.Dialect != PowerShell && strings.Contains(full, `\`)) {
		return "", false
	}
	if opts.Dialect != PowerShell && opts.Home != "" {
		if rel, found := strings.CutPrefix(full, opts.Home+"/"); found && literal("x/"+rel, opts.Dialect) {
			return "~/" + rel, true
		}
	}
	if literal(full, opts.Dialect) {
		return full, true
	}
	// No quote characters remain (see unsafePathChars), so plain single
	// quotes are enough in every dialect.
	return "'" + full + "'", true
}

func literal(s string, d Dialect) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		// ^ is a glob operator in zsh with EXTENDED_GLOB and , builds an array
		// in PowerShell, so output quotes them.
		if !plainRune(r, d) || (r == '^' && d != PowerShell) || (r == ',' && d == PowerShell) {
			return false
		}
	}
	// A leading prefix character would be read as a project reference again.
	return !strings.ContainsRune("@,%^+:=", rune(s[0]))
}
