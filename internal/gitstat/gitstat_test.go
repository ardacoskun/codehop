package gitstat

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestParseStatus(t *testing.T) {
	out := `# branch.oid abc
# branch.head main
# branch.upstream origin/main
# branch.ab +2 -1
1 .M N... 100644 100644 100644 a b file.go
2 R. N... 100644 100644 100644 a b R100 new.go	old.go
u UU N... 100644 100644 100644 100644 a b c conflict.go
? notes.txt
? tmp/
`
	st := parseStatus(out)
	if st.Branch != "main" || !st.HasUpstream || st.Ahead != 2 || st.Behind != 1 || st.Changed != 3 || st.Untracked != 2 {
		t.Fatalf("parseStatus = %+v", st)
	}
	if !st.Dirty() {
		t.Error("should be dirty")
	}
}

func TestParseStatusDetachedNoUpstream(t *testing.T) {
	st := parseStatus("# branch.oid abc\n# branch.head (detached)\n")
	if !st.Detached || st.HasUpstream || st.Dirty() {
		t.Fatalf("parseStatus = %+v", st)
	}
}

func TestAge(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	tests := map[time.Duration]string{
		5 * time.Minute:      "5m",
		3 * time.Hour:        "3h",
		2 * 24 * time.Hour:   "2d",
		30 * 24 * time.Hour:  "4w",
		800 * 24 * time.Hour: "2y",
	}
	for d, want := range tests {
		if got := Age(now.Add(-d), now); got != want {
			t.Errorf("Age(-%v) = %q, want %q", d, got, want)
		}
	}
}

func TestInspectRealRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	if st := Inspect(context.Background(), dir); !st.IsRepo || st.LastCommit != nil {
		t.Fatalf("empty repo: %+v", st)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-q", "-m", "first")
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	st := Inspect(context.Background(), dir)
	if st.Branch != "main" || st.Untracked != 1 || st.LastCommit == nil || st.LastCommit.Subject != "first" {
		t.Fatalf("Inspect = %+v", st)
	}
	if plain := Inspect(context.Background(), t.TempDir()); plain.IsRepo {
		t.Error("plain folder reported as repo")
	}
}

// TestInspectDoesNotRunRepositoryCommands guards against a copied repository
// running code through its own .git/config when previewed or listed.
func TestInspectDoesNotRunRepositoryCommands(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh to detect execution")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	marker := filepath.Join(t.TempDir(), "executed")
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, ".gitattributes"), []byte("* filter=evil\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(file, []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-q", "-m", "init")
	touch := "touch '" + marker + "'"
	run("config", "core.fsmonitor", touch+"; false")
	run("config", "filter.evil.clean", "sh -c \""+touch+"; cat\"")
	// Make the file racily modified so status must run the clean filter.
	if err := os.WriteFile(file, []byte("b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(file, old, old); err != nil {
		t.Fatal(err)
	}

	st := Inspect(context.Background(), dir)
	if st.Err != nil {
		t.Fatalf("Inspect: %v", st.Err)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("repository config ran a command during Inspect")
	}
	if st.Changed != 1 {
		t.Errorf("Changed = %d, want 1", st.Changed)
	}
}
