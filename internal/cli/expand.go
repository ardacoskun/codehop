package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ardacoskun/codehop/internal/discover"
	"github.com/ardacoskun/codehop/internal/expand"
	"github.com/ardacoskun/codehop/internal/match"
)

// Replies of expand-line. The shell reads the first line and acts on it:
// OK is followed by the line to run, SAME means run the line as typed, and
// BLOCK is followed by the reason the line must not run.
const (
	expandOK    = "OK"
	expandSame  = "SAME"
	expandBlock = "BLOCK"
)

const (
	// maxLineBytes bounds the command line read on stdin; longer lines are
	// left alone rather than cut.
	maxLineBytes = 1 << 20
	// enterBudget bounds the whole rewrite so Enter never hangs on a slow
	// disk; past it the line runs as typed.
	enterBudget = 1500 * time.Millisecond
)

var errLineTooLong = errors.New("command line too long to rewrite")

// readLine reads the command line from stdin.
func readLine(r io.Reader) (string, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxLineBytes+1))
	if err != nil {
		return "", err
	}
	if len(data) > maxLineBytes {
		return "", errLineTooLong
	}
	return string(data), nil
}

type expandFlags struct {
	shell  string
	cwd    string
	cdpath string
	keep   []string
}

func (f *expandFlags) register(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.shell, "shell", "", "shell syntax of the line: zsh, bash, fish or powershell")
	cmd.Flags().StringVar(&f.cwd, "cwd", "", "directory relative paths are checked against (default: current)")
	cmd.Flags().StringSliceVar(&f.keep, "keep", nil, "names whose prefixed form stays as typed (PowerShell variables)")
	cmd.Flags().StringVar(&f.cdpath, "cdpath", "", "the shell's cd search path, list-separated (default: $CDPATH)")
	_ = cmd.MarkFlagRequired("shell")
}

// newExpandLineCmd is called by the shell integration when Enter is pressed.
// It reads the command line on stdin and prints "OK" and the line to run, or
// "BLOCK" and a message when the line must not run as typed.
func newExpandLineCmd(a *app) *cobra.Command {
	var f expandFlags
	cmd := &cobra.Command{
		Use:    "expand-line",
		Short:  "Rewrite project references in a command line (shell integration)",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			line, err := readLine(cmd.InOrStdin())
			if err != nil {
				return err
			}
			if f.shell == "powershell" || f.shell == "pwsh" {
				// PowerShell ends text piped to a program with a newline
				// (CRLF on Windows); it is not part of the command line.
				line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
			}
			res, err := a.expandWithin(enterBudget, line, f, true)
			var amb *expand.AmbiguousError
			switch {
			case errors.As(err, &amb):
				fmt.Fprintf(a.stdout, "%s\ncodehop: %s", expandBlock, amb.Error())
				return nil
			case err != nil:
				return err
			case !res.Changed:
				fmt.Fprint(a.stdout, expandSame)
				return nil
			}
			fmt.Fprintf(a.stdout, "%s\n%s", expandOK, res.Line)
			return nil
		},
	}
	f.register(cmd)
	return cmd
}

// newExpandWordCmd expands the last word of the command line read on stdin
// (fish abbreviations run when a word is finished). It prints the word's
// replacement, or exits 1 when the word stays as typed. The whole line is
// needed so the same rules apply as for Enter (e.g. bare names only in the
// first command).
func newExpandWordCmd(a *app) *cobra.Command {
	var f expandFlags
	cmd := &cobra.Command{
		Use:    "expand-word",
		Short:  "Rewrite the last word of a command line (shell integration)",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			line, err := readLine(cmd.InOrStdin())
			if err != nil {
				return exitCodeError{code: 1}
			}
			// fish has already inserted the space that finished the word.
			line = strings.TrimRight(line, " \t\r\n")
			// The line is still being typed, so no visit is recorded.
			res, err := a.expandWithin(enterBudget, line, f, false)
			if err != nil || !res.Changed {
				return exitCodeError{code: 1}
			}
			last := res.Edits[len(res.Edits)-1]
			if last.End != len(line) {
				return exitCodeError{code: 1}
			}
			fmt.Fprint(a.stdout, last.Text)
			return nil
		},
	}
	f.register(cmd)
	return cmd
}

// errExpandTimeout means the rewrite took longer than its budget.
var errExpandTimeout = errors.New("rewrite timed out")

// expandWithin runs expandLine but gives up after budget, so the shell runs
// the line as typed instead of waiting on a slow disk.
func (a *app) expandWithin(budget time.Duration, line string, f expandFlags, record bool) (expand.Result, error) {
	type outcome struct {
		res expand.Result
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		res, err := a.expandLine(line, f, record)
		done <- outcome{res, err}
	}()
	select {
	case o := <-done:
		return o.res, o.err
	case <-time.After(budget):
		return expand.Result{Line: line}, errExpandTimeout
	}
}

// expandLine rewrites line according to the [expand] config. With record,
// it counts a visit for every project it expanded.
func (a *app) expandLine(line string, f expandFlags, record bool) (expand.Result, error) {
	unchanged := expand.Result{Line: line}
	cfg, err := a.config()
	if err != nil {
		return unchanged, err
	}
	if !*cfg.Expand.Enabled {
		return unchanged, nil
	}
	dialect, err := expand.ParseDialect(f.shell)
	if err != nil {
		return unchanged, err
	}
	cwd := f.cwd
	if cwd == "" {
		if cwd, err = os.Getwd(); err != nil {
			return unchanged, err
		}
	}
	home, _ := os.UserHomeDir()
	r, err := a.exactResolver(completionBudget)
	if err != nil {
		return unchanged, err
	}
	res, err := expand.Line(line, expand.Options{
		Dialect:  dialect,
		Prefix:   *cfg.Expand.Prefix,
		Commands: cfg.Expand.Commands,
		Skip:     cfg.Expand.Skip,
		Keep:     f.keep,
		Cwd:      cwd,
		Home:     home,
		CDPath:   cdpath(f.cdpath),
	}, r)
	if err != nil || !res.Changed || !record {
		return res, err
	}
	seen := map[string]bool{}
	for _, p := range res.Projects {
		if !seen[p.Path] {
			seen[p.Path] = true
			a.recordVisit(p.Path)
		}
	}
	return res, nil
}

// cdpath returns the cd search path the shell passed, or $CDPATH.
func cdpath(flag string) []string {
	if flag == "" {
		flag = os.Getenv("CDPATH")
	}
	return filepath.SplitList(flag)
}

// exactResolver resolves names against the project list loaded within budget.
func (a *app) exactResolver(budget time.Duration) (expand.Resolver, error) {
	projects, err := a.projectsWithin(budget)
	if err != nil {
		return nil, err
	}
	return exactResolver(projects), nil
}

type exactResolver []discover.Project

// Exact returns the projects whose key or name equals name (ignoring case
// and separator style).
func (r exactResolver) Exact(name string) []discover.Project {
	var out []discover.Project
	for _, c := range match.Rank(name, r, nil).Candidates {
		if c.Tier == match.Exact {
			out = append(out, c.Project)
		}
	}
	return out
}
