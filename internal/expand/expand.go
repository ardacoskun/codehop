// Package expand rewrites a command line so project references become paths:
// "@blog/src" in any command, and a bare project name after commands such as
// cd or ls. It is deliberately conservative: anything it does not fully
// understand (quotes, substitutions, heredocs, unknown command prefixes) is
// left exactly as typed.
package expand

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/ardacoskun/codehop/internal/discover"
	"github.com/ardacoskun/codehop/internal/match"
	"github.com/ardacoskun/codehop/internal/safetext"
)

// Dialect is the shell syntax family of a command line.
type Dialect int

const (
	// Posix covers zsh and bash.
	Posix Dialect = iota
	// Fish is the fish shell.
	Fish
	// PowerShell covers Windows PowerShell 5.1 and PowerShell 7.
	PowerShell
)

// ParseDialect maps a shell name to its dialect.
func ParseDialect(shell string) (Dialect, error) {
	switch strings.ToLower(shell) {
	case "zsh", "bash", "sh":
		return Posix, nil
	case "fish":
		return Fish, nil
	case "powershell", "pwsh":
		return PowerShell, nil
	}
	return 0, fmt.Errorf("unsupported shell %q", shell)
}

// Resolver finds the projects whose name or key exactly matches name.
type Resolver interface {
	Exact(name string) []discover.Project
}

// Options configures a rewrite.
type Options struct {
	Dialect Dialect
	// Prefix marks a project path in any command ("@"); "" disables it.
	Prefix string
	// Commands accept a bare project name as a path argument.
	Commands []string
	// Skip lists commands whose arguments are never rewritten.
	Skip []string
	// Keep lists names whose prefixed form must stay as typed (PowerShell
	// variables, where "@name" splats the variable).
	Keep []string
	// Cwd resolves relative paths for the "a file of that name exists" check.
	Cwd string
	// Home enables "~/..." output in Posix and fish lines; "" disables it.
	Home string
	// Exists reports whether a path exists; nil uses the file system.
	Exists func(path string) bool
	// LocalNames lists the entries of Cwd; nil reads the directory. Bare
	// names that match one of them (ignoring case and separator style) stay.
	LocalNames func() []string
	// CDPath holds the directories cd searches ($CDPATH).
	CDPath []string
}

// Result is a rewritten line.
type Result struct {
	Line    string
	Changed bool
	// Projects are the projects that were expanded, in line order.
	Projects []discover.Project
	// Edits are the replacements made, in line order.
	Edits []Edit
}

// Edit replaces line[Start:End] with Text.
type Edit struct {
	Start, End int
	Text       string
}

// AmbiguousError reports a prefixed name that matches several projects.
type AmbiguousError struct {
	Word string
	Keys []string
}

func (e *AmbiguousError) Error() string {
	return safetext.Sanitize(fmt.Sprintf("%s matches several projects (%s); write the longer name, "+
		"or quote the word to keep it as typed", e.Word, strings.Join(e.Keys, ", ")))
}

// errUnsupported marks syntax the rewriter leaves alone on purpose.
var errUnsupported = errors.New("unsupported syntax")

// prefixWords run another command; the command after them is the real one.
var prefixWords = map[string]bool{
	"sudo": true, "doas": true, "env": true, "command": true, "builtin": true,
	"exec": true, "time": true, "nohup": true, "nice": true, "!": true,
	"if": true, "then": true, "else": true, "elif": true, "do": true,
	"while": true, "until": true, "and": true, "or": true, "not": true, "begin": true,
}

// Line rewrites line. It returns the line unchanged when nothing applies and
// an *AmbiguousError when a prefixed name matches several projects.
func Line(line string, opts Options, r Resolver) (Result, error) {
	unchanged := Result{Line: line}
	tokens, err := tokenize(line, opts.Dialect)
	if err != nil {
		return unchanged, nil
	}
	exists := opts.Exists
	if exists == nil {
		exists = func(p string) bool {
			_, err := os.Lstat(p)
			return err == nil
		}
	}
	commands := lowerSet(opts.Commands)
	skip := lowerSet(opts.Skip)
	keep := map[string]bool{}
	for _, k := range opts.Keep {
		keep[strings.ToLower(k)] = true
	}

	var (
		edits    []Edit
		projects []discover.Project
	)
	isLocal := localChecker(opts)
	segs := segments(tokens)
	for _, seg := range segs {
		cmdIdx := commandIndex(seg)
		if cmdIdx < 0 {
			continue
		}
		cmd := commandName(seg[cmdIdx].text)
		// A skipped tool run through a wrapper ("xargs npm i @types/node",
		// "ssh host npm …") means "@" may be its own syntax. Commands that
		// take project names (cd, ls) only run themselves, so "cd node"
		// still works.
		if skip[cmd] || (!commands[cmd] && mentionsAny(seg, skip, opts.Prefix)) {
			continue
		}
		// Bare names only when the line is this one command. In a chain the
		// shell's own checks matter: "cd docs && rm -rf *" must keep failing
		// when ./docs is missing, and "mkdir blog && cd blog" means the new
		// folder.
		bareAllowed := commands[cmd] && len(segs) == 1
		takesValue := valueFlags(cmd, opts.Dialect)
		for i := cmdIdx + 1; i < len(seg); i++ {
			t := seg[i]
			if t.kind != wordToken || !t.plain {
				continue
			}
			prev := seg[i-1]
			afterRedirect := prev.kind == redirectToken
			afterValueFlag := i-1 > cmdIdx && prev.kind == wordToken && takesValue(prev.text)
			prefixed := opts.Prefix != "" && strings.HasPrefix(t.text, opts.Prefix) && len(t.text) > len(opts.Prefix)
			var body string
			switch {
			case prefixed:
				body = t.text[len(opts.Prefix):]
				first := firstSegment(body, opts.Dialect)
				if keep[strings.ToLower(first)] || exists(join(opts.Cwd, t.text)) {
					continue
				}
			case bareAllowed && !afterRedirect && !afterValueFlag && bareCandidate(t.text, opts):
				body = t.text
				first := firstSegment(body, opts.Dialect)
				if exists(join(opts.Cwd, t.text)) || isLocal(first) || inCDPath(cmd, first, opts, exists) {
					continue
				}
			default:
				continue
			}
			rest, matches := resolveLongest(body, opts.Dialect, r)
			switch {
			case len(matches) == 1:
				text, ok := render(matches[0].Path, rest, opts)
				if !ok {
					continue
				}
				edits = append(edits, Edit{t.start, t.end, text})
				projects = append(projects, matches[0])
			case len(matches) > 1 && prefixed:
				keys := make([]string, len(matches))
				for k, m := range matches {
					keys[k] = m.Key
				}
				return unchanged, &AmbiguousError{Word: t.text, Keys: keys}
			}
		}
	}
	if len(edits) == 0 {
		return unchanged, nil
	}
	var b strings.Builder
	last := 0
	for _, e := range edits {
		b.WriteString(line[last:e.Start])
		b.WriteString(e.Text)
		last = e.End
	}
	b.WriteString(line[last:])
	return Result{Line: b.String(), Changed: true, Projects: projects, Edits: edits}, nil
}

func lowerSet(names []string) map[string]bool {
	set := make(map[string]bool, len(names))
	for _, n := range names {
		set[strings.ToLower(n)] = true
	}
	return set
}

// segments splits tokens into simple commands at ; & && || | and newlines.
func segments(tokens []token) [][]token {
	var (
		out [][]token
		cur []token
	)
	for _, t := range tokens {
		if t.kind == separatorToken {
			if len(cur) > 0 {
				out = append(out, cur)
			}
			cur = nil
			continue
		}
		cur = append(cur, t)
	}
	if len(cur) > 0 {
		out = append(out, cur)
	}
	return out
}

// commandIndex returns the index of the command word in a simple command,
// skipping variable assignments and wrappers like sudo. It returns -1 when
// the command cannot be determined safely (e.g. "sudo -u root ls").
func commandIndex(seg []token) int {
	for i, t := range seg {
		if t.kind != wordToken {
			return -1
		}
		if isAssignment(t.text) {
			continue
		}
		if prefixWords[strings.ToLower(t.text)] {
			if i+1 < len(seg) && strings.HasPrefix(seg[i+1].text, "-") {
				return -1
			}
			continue
		}
		if !t.plain {
			return -1
		}
		return i
	}
	return -1
}

func isAssignment(word string) bool {
	eq := strings.IndexByte(word, '=')
	if eq <= 0 {
		return false
	}
	for i, r := range word[:eq] {
		if r != '_' && !unicode.IsLetter(r) && (i == 0 || !unicode.IsDigit(r)) {
			return false
		}
	}
	return true
}

// commandName normalizes "/usr/bin/LS", "npm.cmd" or "ls" to "ls"/"npm".
func commandName(word string) string {
	if i := strings.LastIndexAny(word, `/\`); i >= 0 {
		word = word[i+1:]
	}
	word = strings.ToLower(word)
	for _, ext := range []string{".exe", ".cmd", ".bat", ".ps1"} {
		word = strings.TrimSuffix(word, ext)
	}
	return word
}

// resolveLongest finds the project body refers to, trying the longest
// leading path first so keys like "web/api" (given to projects that share a
// folder name) work: "web/api/src" → project web/api, rest "/src".
func resolveLongest(body string, d Dialect, r Resolver) (rest string, matches []discover.Project) {
	seps := "/"
	if d == PowerShell {
		seps = `/\`
	}
	for end := len(body); end > 0; {
		if name := body[:end]; !strings.ContainsAny(name[len(name)-1:], seps) {
			if m := r.Exact(name); len(m) > 0 {
				return body[end:], m
			}
		}
		end = strings.LastIndexAny(body[:end], seps)
	}
	return "", nil
}

// mentionsAny reports whether a word of seg could run one of the commands:
// a bare name ("npm") or a command path ("/usr/local/bin/npm",
// "./node_modules/.bin/tsx"), but not a project reference ("@api/http") or a
// relative path argument ("src/node").
func mentionsAny(seg []token, names map[string]bool, prefix string) bool {
	for _, t := range seg {
		if t.kind != wordToken {
			continue
		}
		w := t.text
		if prefix != "" && strings.HasPrefix(w, prefix) {
			continue
		}
		if strings.ContainsAny(w, `/\`) && !strings.HasPrefix(w, "/") && !strings.HasPrefix(w, ".") &&
			!strings.HasPrefix(w, "~") && !filepath.IsAbs(w) {
			continue
		}
		if names[commandName(w)] {
			return true
		}
	}
	return false
}

// localChecker returns a test for "the current directory has an entry with
// this name", ignoring case and separator style like project matching does,
// so "ls my_app" with ./my-app present stays local.
func localChecker(opts Options) func(name string) bool {
	var local map[string]bool
	return func(name string) bool {
		if local == nil {
			local = map[string]bool{}
			list := opts.LocalNames
			if list == nil {
				list = func() []string { return readDirNames(opts.Cwd) }
			}
			for _, n := range list() {
				local[match.Normalize(n)] = true
			}
		}
		return local[match.Normalize(name)]
	}
}

func readDirNames(dir string) []string {
	if dir == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	return names
}

// inCDPath reports whether cd would find name through $CDPATH.
func inCDPath(cmd, name string, opts Options, exists func(string) bool) bool {
	if cmd != "cd" && cmd != "pushd" {
		return false
	}
	for _, dir := range opts.CDPath {
		if dir == "" {
			continue
		}
		// Rooted entries ("/x", or "\x" on Windows) are not relative to cwd.
		if !filepath.IsAbs(dir) && !strings.HasPrefix(dir, "/") && !strings.HasPrefix(dir, `\`) {
			dir = join(opts.Cwd, dir)
		}
		if exists(filepath.Join(dir, name)) {
			return true
		}
	}
	return false
}

// posixValueFlags and powershellValueFlags are options of the default
// [expand] commands whose next word is a value, not a path: "open -a Notes",
// "ls -I blog", "Get-ChildItem -Filter blog".
var posixValueFlags = map[string][]string{
	"open": {"-a", "-b", "--env", "--stdin", "--stdout", "--stderr"},
	"ls": {"-I", "--ignore", "--hide", "-w", "--width", "-T", "--tabsize", "--format", "--sort",
		"--time-style", "--block-size", "--quoting-style", "--indicator-style", "--color"},
	"tree": {"-I", "-P", "-L", "-o", "-H", "-T", "--filelimit", "--charset", "--timefmt", "--sort"},
}

var powershellValueFlags = []string{
	"-filter", "-include", "-exclude", "-depth", "-attributes", "-credential", "-stackname",
}

func valueFlags(cmd string, d Dialect) func(word string) bool {
	if d == PowerShell {
		return func(w string) bool {
			for _, f := range powershellValueFlags {
				if strings.EqualFold(w, f) {
					return true
				}
			}
			return false
		}
	}
	flags := posixValueFlags[cmd]
	return func(w string) bool {
		// A cluster of short options ("-lI") takes a value when its last
		// option does.
		if len(w) > 2 && w[0] == '-' && w[1] != '-' && !strings.Contains(w, "=") {
			w = "-" + w[len(w)-1:]
		}
		for _, f := range flags {
			if w == f {
				return true
			}
		}
		return false
	}
}

// firstSegment returns the part of s before the first path separator.
func firstSegment(s string, d Dialect) string {
	seps := "/"
	if d == PowerShell {
		seps = `/\`
	}
	if i := strings.IndexAny(s, seps); i >= 0 {
		return s[:i]
	}
	return s
}

// bareCandidate reports whether word could be a bare project name rather than
// a flag, a path or something else the command means literally.
func bareCandidate(word string, opts Options) bool {
	if word == "" {
		return false
	}
	switch word[0] {
	case '-', '+', '.', '~', '/', '\\':
		return false
	}
	if opts.Prefix != "" && strings.HasPrefix(word, opts.Prefix) {
		return false
	}
	return !strings.Contains(word, ":") // drive letters, URLs, scp targets
}

func join(cwd, p string) string {
	if cwd == "" || filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(cwd, p)
}
