package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ardacoskun/codehop/internal/clone"
	"github.com/ardacoskun/codehop/internal/config"
	"github.com/ardacoskun/codehop/internal/discover"
	"github.com/ardacoskun/codehop/internal/editor"
	"github.com/ardacoskun/codehop/internal/match"
	"github.com/ardacoskun/codehop/internal/paths"
	"github.com/ardacoskun/codehop/internal/picker"
)

func newOpenCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "open [project]",
		Short: "Open a project in the editor (picker when no name is given)",
		Long: `Open a project in the configured editor.

The name is matched exactly, then as a prefix, a word or initials, a substring
and finally fuzzily. A single match opens directly; several open the picker.
Remote references (gh:owner/repo, git URLs) are cloned first if missing.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return a.openQuery(firstArg(args), false)
		},
		ValidArgsFunction: a.completeProjects,
	}
}

func newQueryCmd(a *app) *cobra.Command {
	var record, exact bool
	cmd := &cobra.Command{
		Use:   "query [project]",
		Short: "Print the path of a project (picker when ambiguous)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if exact {
				return a.queryExact(firstArg(args))
			}
			p, _, err := a.resolveProject(firstArg(args))
			if err != nil {
				return err
			}
			if record {
				a.recordVisit(p.Path)
			}
			fmt.Fprintln(a.stdout, p.Path)
			return nil
		},
		ValidArgsFunction: a.completeProjects,
	}
	cmd.Flags().BoolVar(&record, "record", false, "count this as a visit for frecency ranking")
	cmd.Flags().BoolVar(&exact, "exact", false, "only an exact name match; never ask (for shell completion)")
	return cmd
}

// queryExact prints the path of the one project named exactly name, quickly
// and without a picker; shell completion uses it inside "@project/".
func (a *app) queryExact(name string) error {
	r, err := a.exactResolver(completionBudget)
	if err != nil {
		return err
	}
	matches := r.Exact(name)
	if len(matches) != 1 {
		return exitCodeError{code: 1}
	}
	fmt.Fprintln(a.stdout, matches[0].Path)
	return nil
}

// newWrapCmd is what the shell's `code` function calls. It keeps the
// editor's own behavior for paths and flags and resolves anything else as
// a project name.
func newWrapCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:                "wrap [args...]",
		Short:              "Editor wrapper used by the shell integration",
		Hidden:             true,
		DisableFlagParsing: true,
		RunE: func(_ *cobra.Command, args []string) error {
			if isPassthrough(args) {
				return a.passthrough(args)
			}
			return a.openQuery(firstArg(args), true)
		},
	}
}

// isPassthrough reports whether the wrapper should hand args to the editor
// untouched: several arguments, any flag, or an existing path ("code .").
func isPassthrough(args []string) bool {
	if len(args) == 0 {
		return false
	}
	if len(args) > 1 {
		return true
	}
	if strings.HasPrefix(args[0], "-") {
		return true
	}
	_, err := os.Stat(args[0])
	return err == nil
}

// looksLikePath reports whether q was clearly meant as a file path, so a
// missing file is created by the editor instead of searched as a project.
func looksLikePath(q string) bool {
	return strings.HasPrefix(q, ".") || strings.HasPrefix(q, "~") || filepath.IsAbs(q) ||
		strings.HasPrefix(q, "/") || strings.HasPrefix(q, `\`)
}

// passthrough runs the editor with the user's own arguments. A broken config
// must not break plain editor use ("code ."), so it falls back to
// CODEHOP_EDITOR or the default editor with a warning.
func (a *app) passthrough(args []string) error {
	ed, err := a.editor()
	if err != nil {
		a.warnf("%v; using the default editor", err)
		spec := os.Getenv("CODEHOP_EDITOR")
		if spec == "" {
			spec = config.DefaultEditor
		}
		if ed, err = editor.Parse(spec); err != nil {
			return err
		}
	}
	return ed.Passthrough(args)
}

// openQuery resolves q (cloning remotes first) and opens it. fromWrapper
// sends path-like input ("./notes.md", ".env", "~/x") to the editor, as the
// `code` shell function must never turn a file into a project.
func (a *app) openQuery(q string, fromWrapper bool) error {
	if fromWrapper && q != "" && looksLikePath(q) {
		return a.passthrough([]string{q})
	}
	if remote, ok := clone.Parse(q); ok {
		return a.cloneAndOpen(remote)
	}
	p, picked, err := a.resolveProject(q)
	if err != nil {
		return err
	}
	if picked {
		// The user chose explicitly, so no "query → project" hint.
		q = ""
	}
	return a.open(p, q)
}

// open opens p in the editor and records the visit. A non-empty query that
// differs from the project's name is echoed so fuzzy resolutions are visible.
func (a *app) open(p discover.Project, query string) error {
	ed, err := a.editor()
	if err != nil {
		return err
	}
	cfg, err := a.config()
	if err != nil {
		return err
	}
	if query != "" && !strings.EqualFold(query, p.Key) && !strings.EqualFold(query, p.Name) {
		a.warnf("%s → %s", query, p.Key)
	}
	target, note := ed.Target(p.Path, *cfg.PreferWorkspace)
	if note != "" {
		a.warnf("%s", note)
	}
	if err := ed.Open(target); err != nil {
		return err
	}
	a.recordVisit(p.Path)
	return nil
}

// errNoUniqueMatch signals that q needs the picker (zero or several matches).
type errNoUniqueMatch struct{ candidates []match.Candidate }

func (e errNoUniqueMatch) Error() string { return "no unique match" }

// findUnique returns the single project q resolves to, rescanning once if
// nothing matches (a project may have been created since the last scan).
func (a *app) findUnique(q string) (discover.Project, error) {
	projects, err := a.projects()
	if err != nil {
		return discover.Project{}, err
	}
	res := match.Rank(q, projects, a.frecency())
	if len(res.Candidates) == 0 {
		rescanned, err := a.rescan(commandBudget)
		if err != nil {
			return discover.Project{}, err
		}
		res = match.Rank(q, rescanned, a.frecency())
	}
	if p, ok := res.Unique(); ok {
		return p, nil
	}
	return discover.Project{}, errNoUniqueMatch{candidates: res.Candidates}
}

// resolveProject turns q into one project, asking the user when q is empty,
// ambiguous or matches nothing. picked reports that the user chose in the
// picker rather than q resolving on its own.
func (a *app) resolveProject(q string) (p discover.Project, picked bool, err error) {
	cfg, err := a.config()
	if err != nil {
		return p, false, err
	}
	if q != "" {
		found, err := a.findUnique(q)
		if err == nil {
			return found, false, nil
		}
		var noUnique errNoUniqueMatch
		if !errors.As(err, &noUnique) {
			return p, false, err
		}
		if len(noUnique.candidates) > 0 {
			choices := pickerChoices(noUnique.candidates)
			if !picker.Interactive() {
				return p, false, fmt.Errorf("%q is not a unique match; candidates: %s", q, describeChoices(choices))
			}
			p, err = a.pick(choices, q, cfg.FzfOpts)
			return p, true, err
		}
		if !picker.Interactive() {
			return p, false, fmt.Errorf("no project matches %q", q)
		}
		a.warnf("no project matches %q", q)
	}

	// findUnique already rescanned when nothing matched, so this is cached.
	projects, err := a.projects()
	if err != nil {
		return p, false, err
	}
	if len(projects) == 0 {
		return p, false, fmt.Errorf("no projects found under %s; set roots in %s",
			describeRoots(cfg.Roots), paths.Pretty(cfg.Path))
	}
	p, err = a.pick(match.SortByFrecency(projects, a.frecency()), q, cfg.FzfOpts)
	return p, true, err
}

func (a *app) pick(choices []discover.Project, query, fzfOpts string) (discover.Project, error) {
	return picker.Pick(choices, picker.Options{
		Query:          query,
		PreviewCommand: previewCommand(),
		FzfOpts:        fzfOpts,
	})
}

// pickerChoices keeps the ranked candidates, dropping fuzzy-only matches
// when better ones exist so the picker is not flooded with noise.
func pickerChoices(candidates []match.Candidate) []discover.Project {
	keepFuzzy := candidates[0].Tier == match.Fuzzy
	out := make([]discover.Project, 0, len(candidates))
	for _, c := range candidates {
		if c.Tier > match.Fuzzy || keepFuzzy {
			out = append(out, c.Project)
		}
	}
	return out
}

func (a *app) cloneAndOpen(remote clone.Remote) error {
	cfg, err := a.config()
	if err != nil {
		return err
	}
	root, err := cfg.CloneDestinationRoot()
	if err != nil {
		return err
	}
	dest, err := remote.Destination(root)
	if err != nil {
		return err
	}
	if _, err := os.Stat(dest); err == nil {
		if !clone.IsCloneOf(dest, remote) {
			return fmt.Errorf("%s already exists and is not a clone of %s", paths.Pretty(dest), remote.Display())
		}
	} else {
		a.warnf("cloning %s into %s", remote.Display(), paths.Pretty(dest))
		if err := clone.Clone(remote, dest); err != nil {
			return err
		}
	}
	_, _ = a.rescan(commandBudget)
	rel, _ := filepath.Rel(root, dest)
	return a.open(discover.Project{Key: remote.Name, Name: remote.Name, Path: dest, Root: root, Rel: filepath.ToSlash(rel)}, "")
}

// previewCommand is the shell command fzf runs to preview a project.
func previewCommand() string {
	exe, err := os.Executable()
	if err != nil {
		exe = "codehop"
	}
	return quoteForShell(exe) + " preview {}"
}

// quoteForShell quotes s for the shell fzf runs commands with: sh on
// Unix-likes, cmd.exe on Windows.
func quoteForShell(s string) string {
	if runtime.GOOS == "windows" {
		return `"` + s + `"`
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func describeRoots(roots []string) string {
	if len(roots) == 0 {
		return "(no roots: none of the default folders exist)"
	}
	pretty := make([]string, len(roots))
	for i, r := range roots {
		pretty[i] = paths.Pretty(r)
	}
	return strings.Join(pretty, ", ")
}

func describeChoices(projects []discover.Project) string {
	const maxShown = 8
	keys := make([]string, 0, maxShown)
	for i, p := range projects {
		if i == maxShown {
			keys = append(keys, "…")
			break
		}
		keys = append(keys, p.Key)
	}
	return strings.Join(keys, ", ")
}

func firstArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}
