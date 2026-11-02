// Package cli wires codehop's commands together.
package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/spf13/cobra"

	"github.com/ardacoskun/codehop/internal/config"
	"github.com/ardacoskun/codehop/internal/discover"
	"github.com/ardacoskun/codehop/internal/editor"
	"github.com/ardacoskun/codehop/internal/paths"
	"github.com/ardacoskun/codehop/internal/picker"
	"github.com/ardacoskun/codehop/internal/safetext"
	"github.com/ardacoskun/codehop/internal/store"
)

// exitCodeError carries a specific process exit code (e.g. the editor's).
type exitCodeError struct{ code int }

func (e exitCodeError) Error() string { return fmt.Sprintf("exit status %d", e.code) }

// app holds what commands share. Config and database load lazily so cheap
// commands such as `init` stay fast.
type app struct {
	stdout io.Writer
	stderr io.Writer
	now    func() time.Time

	cfg   *config.Config
	db    *store.DB
	cache *discover.Cache
}

func (a *app) config() (*config.Config, error) {
	if a.cfg == nil {
		cfg, err := config.Load()
		if err != nil {
			return nil, err
		}
		a.cfg = cfg
	}
	return a.cfg, nil
}

func (a *app) database() *store.DB {
	if a.db == nil {
		a.db = store.Open(paths.DataDir())
	}
	return a.db
}

func (a *app) projectCache() (*discover.Cache, error) {
	if a.cache == nil {
		cfg, err := a.config()
		if err != nil {
			return nil, err
		}
		a.cache = discover.NewCache(paths.CacheDir(), discover.Options{
			Roots: cfg.Roots, MaxDepth: cfg.MaxDepth, Markers: cfg.Markers, Ignore: cfg.Ignore,
		})
	}
	return a.cache, nil
}

// Time limits for finding projects. A root on an unreachable network drive
// can block file system calls indefinitely; past the limit codehop uses the
// last known project list instead of hanging the shell.
const (
	// completionBudget keeps Tab responsive.
	completionBudget = 400 * time.Millisecond
	// commandBudget bounds the wait before opening or jumping.
	commandBudget = 3 * time.Second
	// diagnosticBudget is how long doctor and refresh wait for a full scan.
	diagnosticBudget = 30 * time.Second
)

// projects returns the projects within commandBudget.
func (a *app) projects() ([]discover.Project, error) {
	return a.projectsWithin(commandBudget)
}

func (a *app) projectsWithin(budget time.Duration) ([]discover.Project, error) {
	c, err := a.projectCache()
	if err != nil {
		return nil, err
	}
	projects, fresh := c.ProjectsWithin(budget)
	if !fresh {
		a.warnSlowDisk()
	}
	return projects, nil
}

// rescan rescans within budget, falling back like projectsWithin.
func (a *app) rescan(budget time.Duration) ([]discover.Project, error) {
	c, err := a.projectCache()
	if err != nil {
		return nil, err
	}
	projects, fresh := c.RefreshWithin(budget)
	if !fresh {
		a.warnSlowDisk()
	}
	return projects, nil
}

func (a *app) warnSlowDisk() {
	a.warnf("a folder under your roots is not responding (network drive?); using the last known project list. Run `codehop doctor` for details")
}

func (a *app) editor() (editor.Editor, error) {
	cfg, err := a.config()
	if err != nil {
		return editor.Editor{}, err
	}
	return editor.Parse(cfg.Editor)
}

func (a *app) frecency() func(string) float64 {
	db, now := a.database(), a.now()
	return func(path string) float64 { return db.Score(path, now) }
}

// recordVisit bumps path in the frecency database. The database is re-read
// first so visits other shells saved meanwhile (e.g. while a terminal editor
// was open) are kept. Failures only cost ordering, so they are reported but
// never fail the command.
func (a *app) recordVisit(path string) {
	db := store.Open(paths.DataDir())
	a.db = db
	db.Visit(path, a.now())
	db.Prune()
	if err := db.Save(); err != nil {
		fmt.Fprintf(a.stderr, "codehop: could not save history: %v\n", err)
	}
}

// warnf prints a message to stderr. Messages can quote folder names and git
// output, so they are sanitized against terminal escape sequences.
func (a *app) warnf(format string, args ...any) {
	fmt.Fprintln(a.stderr, "codehop: "+safetext.Sanitize(fmt.Sprintf(format, args...)))
}

// Execute runs the CLI and returns the process exit code.
func Execute(version string) int {
	a := &app{stdout: os.Stdout, stderr: os.Stderr, now: time.Now}
	root := newRootCmd(a, version)
	err := root.Execute()
	if err == nil {
		return 0
	}
	var exitErr exitCodeError
	var cmdExit *exec.ExitError
	switch {
	case errors.Is(err, picker.ErrCanceled):
		return 1
	case errors.As(err, &exitErr):
		return exitErr.code
	case errors.As(err, &cmdExit):
		return cmdExit.ExitCode()
	}
	fmt.Fprintln(os.Stderr, "codehop: "+safetext.Sanitize(err.Error()))
	return 1
}

func newRootCmd(a *app, version string) *cobra.Command {
	root := &cobra.Command{
		Use:   "codehop",
		Short: "Open any project by name, from anywhere",
		Long: `codehop finds the projects on your disk and opens them in your editor by name,
with tab completion, fuzzy matching and frecency ranking.

Set up your shell (see "codehop init --help"), then:
  code <project>     open a project (Tab completes names)
  code               pick a project interactively
  cdp <project>      cd into a project`,
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(a.stdout)
	root.SetErr(a.stderr)
	root.AddCommand(
		newOpenCmd(a),
		newQueryCmd(a),
		newWrapCmd(a),
		newExpandLineCmd(a),
		newExpandWordCmd(a),
		newListCmd(a),
		newStatusCmd(a),
		newPreviewCmd(a),
		newInitCmd(a),
		newRefreshCmd(a),
		newDoctorCmd(a),
		newConfigCmd(a),
	)
	return root
}
