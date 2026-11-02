package cli

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/spf13/cobra"

	"github.com/ardacoskun/codehop/internal/paths"
)

type checkLevel int

const (
	checkOK checkLevel = iota
	checkWarn
	checkFail
)

// rootResponseBudget is how long doctor waits for each root to answer.
const rootResponseBudget = 2 * time.Second

// statWithin stats path but gives up after budget; responded is false when
// the file system did not answer in time (the stat keeps running detached).
func statWithin(path string, budget time.Duration) (info os.FileInfo, responded bool, err error) {
	type result struct {
		info os.FileInfo
		err  error
	}
	done := make(chan result, 1)
	go func() {
		info, err := os.Stat(path)
		done <- result{info, err}
	}()
	select {
	case r := <-done:
		return r.info, true, r.err
	case <-time.After(budget):
		return nil, false, nil
	}
}

func newDoctorCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check the config, roots, editor and optional tools",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			if failed := a.runDoctor(); failed > 0 {
				return exitCodeError{code: 1}
			}
			return nil
		},
	}
}

// runDoctor prints one line per check and returns the number of failures.
func (a *app) runDoctor() int {
	color := colorEnabled(a.stdout)
	failed := 0
	report := func(level checkLevel, format string, args ...any) {
		mark := map[checkLevel]cell{
			checkOK:   {"✓", styleGreen},
			checkWarn: {"!", styleYellow},
			checkFail: {"✗", styleRed},
		}[level]
		if level == checkFail {
			failed++
		}
		t := newTable(a.stdout, color)
		t.row(mark, cell{text: fmt.Sprintf(format, args...)})
		t.flush()
	}

	cfg, err := a.config()
	if err != nil {
		report(checkFail, "config: %v", err)
		return failed
	}
	if cfg.Loaded {
		report(checkOK, "config: %s", paths.Pretty(cfg.Path))
	} else {
		report(checkOK, "config: none, using defaults (create one with `codehop config init`)")
	}
	for _, w := range cfg.Warnings {
		report(checkWarn, "config: %s", w)
	}

	if len(cfg.Roots) == 0 {
		report(checkFail, "roots: none configured and no default folder exists; set roots in the config")
	}
	for _, r := range cfg.Roots {
		info, responded, err := statWithin(r, rootResponseBudget)
		switch {
		case !responded:
			report(checkFail, "root %s: not responding after %s (unreachable network drive?)", paths.Pretty(r), rootResponseBudget)
		case err != nil || !info.IsDir():
			report(checkFail, "root %s: not a directory", paths.Pretty(r))
		default:
			report(checkOK, "root %s", paths.Pretty(r))
		}
	}

	if cache, err := a.projectCache(); err == nil {
		start := time.Now()
		projects, fresh := cache.RefreshWithin(diagnosticBudget)
		switch {
		case !fresh:
			report(checkFail, "projects: scan did not finish within %s; a folder under a root is not responding", diagnosticBudget)
		case len(projects) == 0:
			report(checkWarn, "projects: none found (max_depth %d)", cfg.MaxDepth)
		default:
			report(checkOK, "projects: %d found in %s (max_depth %d)", len(projects),
				time.Since(start).Round(time.Millisecond), cfg.MaxDepth)
		}
	}

	if ed, err := a.editor(); err != nil {
		report(checkFail, "editor: %v", err)
	} else if bin, err := ed.Lookup(); err != nil {
		report(checkFail, "editor: %v", err)
	} else {
		report(checkOK, "editor: %s (%s)", ed.Name, bin)
	}

	for _, tool := range []struct{ name, why string }{
		{"fzf", "the picker falls back to a numbered list"},
		{"git", "status, previews and cloning are unavailable"},
	} {
		if bin, err := exec.LookPath(tool.name); err == nil {
			report(checkOK, "%s: %s", tool.name, bin)
		} else {
			report(checkWarn, "%s: not found, %s", tool.name, tool.why)
		}
	}

	report(checkOK, "data: %s", paths.Pretty(paths.DataDir()))
	report(checkOK, "cache: %s", paths.Pretty(paths.CacheDir()))
	return failed
}
