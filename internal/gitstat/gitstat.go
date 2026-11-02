// Package gitstat reads the git state of projects.
package gitstat

import (
	"bufio"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// perRepoTimeout bounds a single repository inspection so one hung network
// filesystem cannot stall `codehop status`.
const perRepoTimeout = 10 * time.Second

// Status is the git state of one directory.
type Status struct {
	// IsRepo is false for directories that are not git work trees.
	IsRepo   bool
	Branch   string
	Detached bool
	// HasUpstream is false when the branch tracks nothing; Ahead/Behind are then 0.
	HasUpstream bool
	Ahead       int
	Behind      int
	// Changed counts staged, unstaged and conflicted entries.
	Changed int
	// Untracked counts untracked files.
	Untracked  int
	LastCommit *Commit
	// Err is set when git failed for a reason other than "not a repository".
	Err error
}

// Commit summarizes the HEAD commit.
type Commit struct {
	Hash    string
	Subject string
	Time    time.Time
}

// Dirty reports uncommitted or unpushed work.
func (s Status) Dirty() bool {
	return s.Changed > 0 || s.Untracked > 0 || s.Ahead > 0
}

// IsRepo reports whether dir looks like a git work tree (has a .git entry).
// Cheaper than running git, used to skip plain folders.
func IsRepo(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

// Inspect reads the git state of dir.
func Inspect(ctx context.Context, dir string) Status {
	if !IsRepo(dir) {
		return Status{}
	}
	ctx, cancel := context.WithTimeout(ctx, perRepoTimeout)
	defer cancel()

	safe := hardeningArgs(ctx, dir)
	out, err := git(ctx, dir, append(safe, "status", "--porcelain=v2", "--branch")...)
	if err != nil {
		return Status{IsRepo: true, Err: err}
	}
	st := parseStatus(out)
	st.IsRepo = true

	// An empty repository has no commit yet; that is not an error.
	if logOut, err := git(ctx, dir, append(safe, "log", "-1", "--format=%h%x00%ct%x00%s")...); err == nil {
		st.LastCommit = parseCommit(logOut)
	}
	return st
}

// hardeningArgs returns -c overrides that stop a repository's own config from
// running programs during these read-only commands. Without them, a project
// copied from elsewhere (zip, sync, shared drive) could run code merely by
// being previewed: core.fsmonitor and filter.<name>.clean run on `git status`,
// and log.showSignature runs gpg.program on `git log`. Command-line -c beats
// repository config. Filters defined in the repository are neutralized by
// name; filters from the user's own global config (e.g. git-lfs) still run.
func hardeningArgs(ctx context.Context, dir string) []string {
	args := []string{"-c", "core.fsmonitor=false", "-c", "log.showSignature=false"}
	// Reading config executes nothing.
	names, err := git(ctx, dir, "config", "--local", "--name-only", "--get-regexp", `^filter\.`)
	if err != nil {
		return args
	}
	seen := map[string]bool{}
	for _, key := range strings.Fields(names) {
		driver := strings.TrimPrefix(key, "filter.")
		if i := strings.LastIndexByte(driver, '.'); i >= 0 {
			driver = driver[:i]
		}
		if driver == "" || seen[driver] {
			continue
		}
		seen[driver] = true
		for _, setting := range []string{"clean=", "smudge=", "process=", "required=false"} {
			args = append(args, "-c", "filter."+driver+"."+setting)
		}
	}
	return args
}

// InspectAll inspects dirs concurrently and returns statuses in input order.
func InspectAll(ctx context.Context, dirs []string) []Status {
	out := make([]Status, len(dirs))
	jobs := make(chan int)
	var wg sync.WaitGroup
	workers := min(len(dirs), max(4, runtime.NumCPU()))
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				out[i] = Inspect(ctx, dirs[i])
			}
		}()
	}
	for i := range dirs {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	return out
}

func git(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	// Never prompt for credentials or open a pager from a status read.
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0")
	// The context only kills git itself; stop waiting for a child that keeps
	// the output pipe open (e.g. a filesystem monitor daemon).
	cmd.WaitDelay = time.Second
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			return "", errors.New(strings.TrimSpace(string(exitErr.Stderr)))
		}
		return "", err
	}
	return string(out), nil
}

// parseStatus parses `git status --porcelain=v2 --branch` output.
func parseStatus(out string) Status {
	var st Status
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "# branch.head "):
			st.Branch = strings.TrimPrefix(line, "# branch.head ")
			st.Detached = st.Branch == "(detached)"
		case strings.HasPrefix(line, "# branch.upstream "):
			st.HasUpstream = true
		case strings.HasPrefix(line, "# branch.ab "):
			for _, f := range strings.Fields(strings.TrimPrefix(line, "# branch.ab ")) {
				n, _ := strconv.Atoi(f[1:])
				if f[0] == '+' {
					st.Ahead = n
				} else {
					st.Behind = n
				}
			}
		case strings.HasPrefix(line, "? "):
			st.Untracked++
		case strings.HasPrefix(line, "1 "), strings.HasPrefix(line, "2 "), strings.HasPrefix(line, "u "):
			st.Changed++
		}
	}
	return st
}

func parseCommit(out string) *Commit {
	parts := strings.SplitN(strings.TrimSpace(out), "\x00", 3)
	if len(parts) != 3 {
		return nil
	}
	secs, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return nil
	}
	return &Commit{Hash: parts[0], Time: time.Unix(secs, 0), Subject: parts[2]}
}

// Age formats the time since t compactly: "5m", "3h", "2d", "6w", "1y".
func Age(t, now time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Hour:
		return strconv.Itoa(max(0, int(d.Minutes()))) + "m"
	case d < 24*time.Hour:
		return strconv.Itoa(int(d.Hours())) + "h"
	case d < 14*24*time.Hour:
		return strconv.Itoa(int(d.Hours()/24)) + "d"
	case d < 365*24*time.Hour:
		return strconv.Itoa(int(d.Hours()/24/7)) + "w"
	default:
		return strconv.Itoa(int(d.Hours()/24/365)) + "y"
	}
}
