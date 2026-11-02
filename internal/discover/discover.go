// Package discover finds project directories under the configured roots and
// caches the result.
package discover

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ardacoskun/codehop/internal/safetext"
)

// Project is a directory recognized as a project.
type Project struct {
	// Key is the unique name used on the command line: the folder name, or
	// "parent/name" when several projects share a folder name.
	Key string `json:"key"`
	// Name is the folder name.
	Name string `json:"name"`
	// Path is the absolute path.
	Path string `json:"path"`
	// Root is the configured root the project was found under.
	Root string `json:"root"`
	// Rel is the slash-separated path relative to Root, e.g. "web/blog".
	Rel string `json:"rel"`
	// Marker is the entry that made the folder a project (e.g. ".git").
	Marker string `json:"marker"`
}

// Category returns the slash-separated parent of Rel ("web" for "web/blog"),
// or "" for projects directly under a root.
func (p Project) Category() string {
	if i := strings.LastIndexByte(p.Rel, '/'); i >= 0 {
		return p.Rel[:i]
	}
	return ""
}

// Options controls a scan.
type Options struct {
	Roots    []string
	MaxDepth int
	Markers  []string
	Ignore   []string
}

// Result is the outcome of a scan.
type Result struct {
	Projects []Project
	// Containers maps every non-project directory that was read to its
	// modification time (UnixNano). A changed mtime means a child was added
	// or removed, so the cache must be rebuilt. Roots that do not exist yet
	// are recorded as missingRoot so creating them invalidates the cache.
	Containers map[string]int64
}

type markerSet struct {
	exact map[string]bool
	globs []string
}

func newMarkerSet(markers []string) markerSet {
	ms := markerSet{exact: make(map[string]bool, len(markers))}
	for _, m := range markers {
		if strings.ContainsAny(m, "*?[") {
			ms.globs = append(ms.globs, m)
		} else {
			ms.exact[m] = true
		}
	}
	return ms
}

func (ms markerSet) matches(name string) bool {
	if ms.exact[name] {
		return true
	}
	for _, g := range ms.globs {
		if ok, _ := filepath.Match(g, name); ok {
			return true
		}
	}
	return false
}

// Scan walks each root up to MaxDepth levels. A directory containing a marker
// is a project and is not descended into. Hidden and ignored directories are
// skipped. Unreadable directories are skipped silently.
func Scan(opts Options) Result {
	markers := newMarkerSet(opts.Markers)
	ignore := make(map[string]bool, len(opts.Ignore))
	for _, name := range opts.Ignore {
		ignore[name] = true
	}

	res := Result{Containers: map[string]int64{}}
	seen := map[string]bool{}
	for _, root := range opts.Roots {
		walk(root, root, 0, opts.MaxDepth, markers, ignore, seen, &res)
	}
	assignKeys(res.Projects)
	sort.Slice(res.Projects, func(i, j int) bool { return res.Projects[i].Key < res.Projects[j].Key })
	return res
}

// missingRoot is the Containers value of a root that does not exist.
const missingRoot = 0

func walk(root, dir string, depth, maxDepth int, markers markerSet, ignore, seen map[string]bool, res *Result) {
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		if depth == 0 {
			res.Containers[dir] = missingRoot
		}
		return
	}
	if seen[resolved] {
		return
	}
	seen[resolved] = true

	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	for _, e := range entries {
		if markers.matches(e.Name()) {
			rel, _ := filepath.Rel(root, dir)
			res.Projects = append(res.Projects, Project{
				Name:   filepath.Base(dir),
				Path:   dir,
				Root:   root,
				Rel:    filepath.ToSlash(rel),
				Marker: e.Name(),
			})
			return
		}
	}

	res.Containers[dir] = info.ModTime().UnixNano()
	if depth >= maxDepth {
		return
	}
	for _, e := range entries {
		name := e.Name()
		// Names with control characters could forge terminal output or the
		// picker's line protocol, so such folders are never listed.
		// A leading "-" would read as a flag once completed onto a command line.
		if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "-") || ignore[name] || safetext.HasUnsafe(name) {
			continue
		}
		if !e.IsDir() && e.Type()&os.ModeSymlink == 0 {
			continue
		}
		walk(root, filepath.Join(dir, name), depth+1, maxDepth, markers, ignore, seen, res)
	}
}

// assignKeys gives every project a unique Key: its folder name when unique,
// otherwise the shortest path suffix that tells the duplicates apart.
func assignKeys(projects []Project) {
	byName := map[string][]int{}
	for i, p := range projects {
		byName[strings.ToLower(p.Name)] = append(byName[strings.ToLower(p.Name)], i)
	}
	for _, idx := range byName {
		if len(idx) == 1 {
			p := &projects[idx[0]]
			p.Key = p.Name
			continue
		}
		disambiguate(projects, idx)
	}
}

func disambiguate(projects []Project, idx []int) {
	full := make([][]string, len(idx))
	maxLen := 0
	for n, i := range idx {
		// Include the root's own name so identical Rel paths in two roots differ.
		segs := append([]string{filepath.Base(projects[i].Root)}, strings.Split(projects[i].Rel, "/")...)
		full[n] = segs
		maxLen = max(maxLen, len(segs))
	}
	for take := 2; take <= maxLen; take++ {
		keys := make([]string, len(idx))
		unique := map[string]bool{}
		for n, segs := range full {
			keys[n] = strings.Join(segs[max(0, len(segs)-take):], "/")
			unique[strings.ToLower(keys[n])] = true
		}
		if len(unique) == len(idx) {
			for n, i := range idx {
				projects[i].Key = keys[n]
			}
			return
		}
	}
	// Identical relative paths under roots with the same folder name
	// (~/work/code/x and ~/home/code/x): only the full path is unique.
	for _, i := range idx {
		projects[i].Key = filepath.ToSlash(projects[i].Path)
	}
}
