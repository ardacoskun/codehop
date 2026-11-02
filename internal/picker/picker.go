// Package picker lets the user choose a project interactively, with fzf when
// it is installed and a numbered prompt otherwise.
package picker

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/term"

	"github.com/ardacoskun/codehop/internal/argv"
	"github.com/ardacoskun/codehop/internal/discover"
	"github.com/ardacoskun/codehop/internal/paths"
)

// ErrCanceled is returned when the user leaves the picker without choosing.
var ErrCanceled = errors.New("canceled")

// ErrNotInteractive is returned when there is no terminal to ask on.
var ErrNotInteractive = errors.New("no terminal available for the picker")

// Options configures a pick.
type Options struct {
	// Query pre-fills the search field.
	Query string
	// PreviewCommand is the shell command fzf runs for the preview; "{}" is
	// replaced by the project path. Empty disables the preview.
	PreviewCommand string
	// FzfOpts are extra options appended to the fzf command line.
	FzfOpts string
	// Prompt is shown before the search field.
	Prompt string
}

// Pick asks the user to choose one of projects.
func Pick(projects []discover.Project, opts Options) (discover.Project, error) {
	if len(projects) == 0 {
		return discover.Project{}, errors.New("no projects to choose from")
	}
	if !Interactive() {
		return discover.Project{}, ErrNotInteractive
	}
	if fzf, err := exec.LookPath("fzf"); err == nil {
		return pickFzf(fzf, projects, opts)
	}
	return pickNumbered(projects, os.Stdin, os.Stderr)
}

func pickFzf(fzf string, projects []discover.Project, opts Options) (discover.Project, error) {
	width := 0
	for _, p := range projects {
		width = max(width, utf8.RuneCountInString(p.Key))
	}
	var input bytes.Buffer
	for i, p := range projects {
		// Fields: hidden index (maps the selection back), padded display, path.
		// Padding with spaces, not tabs, keeps the category column aligned.
		pad := strings.Repeat(" ", width-utf8.RuneCountInString(p.Key)+2)
		fmt.Fprintf(&input, "%d\t%s%s\x1b[2m%s\x1b[0m\t%s\n", i, p.Key, pad, p.Category(), p.Path)
	}

	prompt := opts.Prompt
	if prompt == "" {
		prompt = "project> "
	}
	args := []string{
		"--ansi", "--delimiter=\t", "--with-nth=2",
		"--height=50%", "--min-height=12", "--layout=reverse", "--info=inline",
		"--tiebreak=index", "--prompt=" + prompt, "--query=" + opts.Query,
	}
	if opts.PreviewCommand != "" {
		args = append(args, "--preview="+strings.ReplaceAll(opts.PreviewCommand, "{}", "{3}"),
			"--preview-window=right,55%,wrap")
	}
	if extra, err := argv.Split(opts.FzfOpts); err == nil {
		args = append(args, extra...)
	}

	cmd := exec.Command(fzf, args...)
	cmd.Stdin = &input
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		// fzf exits 1 for "no match" and 130 for Esc/Ctrl-C.
		if errors.As(err, &exitErr) && (exitErr.ExitCode() == 1 || exitErr.ExitCode() == 130) {
			return discover.Project{}, ErrCanceled
		}
		return discover.Project{}, fmt.Errorf("fzf: %w", err)
	}
	idxField, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\t")
	idx, err := strconv.Atoi(idxField)
	if err != nil || idx < 0 || idx >= len(projects) {
		return discover.Project{}, fmt.Errorf("unexpected fzf output %q", out)
	}
	return projects[idx], nil
}

func pickNumbered(projects []discover.Project, in io.Reader, out io.Writer) (discover.Project, error) {
	width := len(strconv.Itoa(len(projects)))
	for i, p := range projects {
		fmt.Fprintf(out, "%*d) %s  %s\n", width, i+1, p.Key, paths.Pretty(p.Path))
	}
	fmt.Fprint(out, "Choose a project (number, empty to cancel): ")
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && line == "" {
		return discover.Project{}, ErrCanceled
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return discover.Project{}, ErrCanceled
	}
	n, err := strconv.Atoi(line)
	if err != nil || n < 1 || n > len(projects) {
		return discover.Project{}, fmt.Errorf("invalid choice %q", line)
	}
	return projects[n-1], nil
}

// Interactive reports whether the user can be asked. Stdout is usually
// captured ("$(codehop query)"), so the terminal is judged by stdin and stderr.
func Interactive() bool {
	return IsTerminal(os.Stdin) && IsTerminal(os.Stderr)
}

// IsTerminal reports whether f is attached to a terminal (not merely a
// character device such as /dev/null).
func IsTerminal(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}
