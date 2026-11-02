package discover

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// mkTree creates dirs (slash-separated, relative to root); a path ending in a
// marker file name creates that file.
func mkTree(t *testing.T, root string, entries ...string) {
	t.Helper()
	for _, e := range entries {
		p := filepath.Join(root, filepath.FromSlash(e))
		if filepath.Ext(p) != "" || filepath.Base(p) == ".git" || filepath.Base(p) == "Gemfile" {
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, nil, 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func keys(projects []Project) []string {
	out := make([]string, len(projects))
	for i, p := range projects {
		out[i] = p.Key
	}
	return out
}

func defaultOpts(roots ...string) Options {
	return Options{
		Roots:    roots,
		MaxDepth: 3,
		Markers:  []string{".git", "package.json", "*.sln"},
		Ignore:   []string{"node_modules"},
	}
}

func TestScanFindsProjectsByMarker(t *testing.T) {
	root := t.TempDir()
	mkTree(t, root,
		"web/blog/package.json",
		"web/blog/packages/inner/package.json", // inside a project: not listed
		"games/shuffle/.git",
		"dotnet/App/App.sln",
		"empty-category",
		"node_modules/dep/package.json", // ignored dir
		".hidden/secret/.git",           // hidden dir
		"a/b/c/too-deep/.git",           // depth 4
	)
	got := keys(Scan(defaultOpts(root)).Projects)
	want := []string{"App", "blog", "shuffle"}
	if len(got) != len(want) {
		t.Fatalf("keys = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("keys = %v, want %v", got, want)
		}
	}
}

func TestScanDisambiguatesDuplicateNames(t *testing.T) {
	root := t.TempDir()
	mkTree(t, root, "web/api/.git", "mobile/api/.git", "solo/.git")
	got := keys(Scan(defaultOpts(root)).Projects)
	want := map[string]bool{"mobile/api": true, "web/api": true, "solo": true}
	if len(got) != 3 {
		t.Fatalf("keys = %v", got)
	}
	for _, k := range got {
		if !want[k] {
			t.Fatalf("unexpected key %q in %v", k, got)
		}
	}
}

func TestScanRelAndCategory(t *testing.T) {
	root := t.TempDir()
	mkTree(t, root, "games/shuffle/.git", "top/.git")
	for _, p := range Scan(defaultOpts(root)).Projects {
		switch p.Key {
		case "shuffle":
			if p.Rel != "games/shuffle" || p.Category() != "games" {
				t.Errorf("shuffle: rel=%q category=%q", p.Rel, p.Category())
			}
		case "top":
			if p.Rel != "top" || p.Category() != "" {
				t.Errorf("top: rel=%q category=%q", p.Rel, p.Category())
			}
		}
	}
}

func TestCacheNoticesNewProjects(t *testing.T) {
	root := t.TempDir()
	mkTree(t, root, "web/blog/.git")
	cache := NewCache(t.TempDir(), defaultOpts(root))
	if got := keys(cache.Projects()); len(got) != 1 {
		t.Fatalf("first scan = %v", got)
	}

	// Directory mtimes can have coarse resolution; make the change visible.
	time.Sleep(20 * time.Millisecond)
	mkTree(t, root, "web/shop/.git")
	future := time.Now().Add(time.Second)
	if err := os.Chtimes(filepath.Join(root, "web"), future, future); err != nil {
		t.Fatal(err)
	}
	if got := keys(cache.Projects()); len(got) != 2 {
		t.Fatalf("after adding a project = %v, want 2 projects", got)
	}
}

func TestCacheNoticesRemovedProjects(t *testing.T) {
	root := t.TempDir()
	mkTree(t, root, "web/blog/.git", "web/shop/.git")
	cache := NewCache(t.TempDir(), defaultOpts(root))
	if got := keys(cache.Projects()); len(got) != 2 {
		t.Fatalf("first scan = %v", got)
	}
	if err := os.RemoveAll(filepath.Join(root, "web", "shop")); err != nil {
		t.Fatal(err)
	}
	if got := keys(cache.Projects()); len(got) != 1 {
		t.Fatalf("after removal = %v, want 1 project", got)
	}
}

func TestCacheInvalidatedByOptions(t *testing.T) {
	root := t.TempDir()
	mkTree(t, root, "a/.git", "b/package.json")
	dir := t.TempDir()
	opts := defaultOpts(root)
	if got := keys(NewCache(dir, opts).Projects()); len(got) != 2 {
		t.Fatalf("got %v", got)
	}
	opts.Markers = []string{".git"}
	if got := keys(NewCache(dir, opts).Projects()); len(got) != 1 {
		t.Fatalf("after marker change got %v, want 1", got)
	}
}

func TestScanSkipsUnsafeNames(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows forbids control characters in file names")
	}
	root := t.TempDir()
	mkTree(t, root, "ok/.git", "evil\x1b]52;c;x\a/.git", "x\n3\tforged/.git", "--flag/.git")
	if got := keys(Scan(defaultOpts(root)).Projects); len(got) != 1 || got[0] != "ok" {
		t.Fatalf("keys = %q, want only ok", got)
	}
}

func TestCacheNoticesRemovedMarker(t *testing.T) {
	root := t.TempDir()
	mkTree(t, root, "web/blog/.git", "web/shop/.git")
	cache := NewCache(t.TempDir(), defaultOpts(root))
	if got := keys(cache.Projects()); len(got) != 2 {
		t.Fatalf("first scan = %v", got)
	}
	if err := os.Remove(filepath.Join(root, "web", "shop", ".git")); err != nil {
		t.Fatal(err)
	}
	if got := keys(cache.Projects()); len(got) != 1 {
		t.Fatalf("after removing the marker = %v, want 1 project", got)
	}
}

func TestCacheNoticesRootCreatedLater(t *testing.T) {
	root := filepath.Join(t.TempDir(), "later")
	cache := NewCache(t.TempDir(), defaultOpts(root))
	if got := cache.Projects(); len(got) != 0 {
		t.Fatalf("missing root gave %v", got)
	}
	mkTree(t, root, "app/.git")
	if got := keys(cache.Projects()); len(got) != 1 {
		t.Fatalf("after creating the root = %v, want 1 project", got)
	}
}

func TestKeysUniqueAcrossSameNamedRoots(t *testing.T) {
	a := filepath.Join(t.TempDir(), "code")
	b := filepath.Join(t.TempDir(), "code")
	mkTree(t, a, "api/.git")
	mkTree(t, b, "api/.git")
	got := keys(Scan(defaultOpts(a, b)).Projects)
	if len(got) != 2 || got[0] == got[1] {
		t.Fatalf("keys = %q, want two distinct keys", got)
	}
}

func TestProjectsWithinFallsBackWhenDiskHangs(t *testing.T) {
	root := t.TempDir()
	mkTree(t, root, "web/blog/.git")
	cache := NewCache(t.TempDir(), defaultOpts(root))
	if got, fresh := cache.ProjectsWithin(time.Second); !fresh || len(got) != 1 {
		t.Fatalf("first scan = %v, fresh=%v", got, fresh)
	}

	// Invalidate the cache, then make scanning hang like a dead network mount.
	mkTree(t, root, "web/shop/.git")
	future := time.Now().Add(time.Second)
	if err := os.Chtimes(filepath.Join(root, "web"), future, future); err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	cache.scan = func(Options) Result {
		<-release
		return Result{}
	}

	start := time.Now()
	got, fresh := cache.ProjectsWithin(100 * time.Millisecond)
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("ProjectsWithin blocked for %v", elapsed)
	}
	if fresh || len(got) != 1 || got[0].Key != "blog" {
		t.Fatalf("got %v fresh=%v, want the last known list (blog) marked stale", keys(got), fresh)
	}
}
