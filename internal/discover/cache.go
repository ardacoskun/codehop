package discover

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ardacoskun/codehop/internal/paths"
)

const cacheVersion = 2

type cacheFile struct {
	Version     int              `json:"version"`
	Fingerprint string           `json:"fingerprint"`
	Containers  map[string]int64 `json:"containers"`
	Projects    []Project        `json:"projects"`
}

// Cache stores scan results on disk and reuses them while every container
// directory keeps its modification time, so new or removed projects show up
// immediately without rescanning on every call.
type Cache struct {
	Path string
	Opts Options
	// scan is Scan, replaceable in tests to simulate an unresponsive disk.
	scan func(Options) Result
}

// NewCache returns a cache at <cacheDir>/projects.json for the given options.
func NewCache(cacheDir string, opts Options) *Cache {
	return &Cache{Path: filepath.Join(cacheDir, "projects.json"), Opts: opts, scan: Scan}
}

// ProjectsWithin is Projects with a time limit. Checking or rescanning can
// block indefinitely on an unreachable network drive under a root; after
// budget it gives up and returns the last known list instead. fresh is false
// when that happened. The abandoned check keeps running in the background
// and still updates the cache file if it ever finishes.
func (c *Cache) ProjectsWithin(budget time.Duration) (projects []Project, fresh bool) {
	return c.within(budget, c.Projects)
}

// RefreshWithin is Refresh with the same time limit as ProjectsWithin.
func (c *Cache) RefreshWithin(budget time.Duration) (projects []Project, fresh bool) {
	return c.within(budget, c.Refresh)
}

func (c *Cache) within(budget time.Duration, fn func() []Project) ([]Project, bool) {
	done := make(chan []Project, 1)
	go func() { done <- fn() }()
	timer := time.NewTimer(budget)
	defer timer.Stop()
	select {
	case projects := <-done:
		return projects, true
	case <-timer.C:
		return c.lastKnown(), false
	}
}

// lastKnown returns the cached list without touching the scanned folders.
func (c *Cache) lastKnown() []Project {
	f, ok := c.read()
	if !ok {
		return nil
	}
	return f.Projects
}

func (c *Cache) read() (cacheFile, bool) {
	var f cacheFile
	data, err := os.ReadFile(c.Path)
	if err != nil {
		return f, false
	}
	if json.Unmarshal(data, &f) != nil || f.Version != cacheVersion || f.Fingerprint != c.fingerprint() {
		return f, false
	}
	return f, true
}

// Projects returns cached projects when the cache is still valid, otherwise
// rescans and rewrites the cache. Cache write errors are ignored: the result
// is still correct, only slower next time.
func (c *Cache) Projects() []Project {
	if projects, ok := c.load(); ok {
		return projects
	}
	return c.Refresh()
}

// Refresh rescans unconditionally and rewrites the cache.
func (c *Cache) Refresh() []Project {
	res := c.scan(c.Opts)
	data, err := json.Marshal(cacheFile{
		Version:     cacheVersion,
		Fingerprint: c.fingerprint(),
		Containers:  res.Containers,
		Projects:    res.Projects,
	})
	if err == nil {
		_ = paths.WriteFileAtomic(c.Path, data)
	}
	return res.Projects
}

func (c *Cache) load() ([]Project, bool) {
	f, ok := c.read()
	if !ok {
		return nil, false
	}
	for dir, mtime := range f.Containers {
		info, err := os.Stat(dir)
		switch {
		case err != nil && mtime == missingRoot && errors.Is(err, fs.ErrNotExist):
			// Still missing: nothing changed.
		case err != nil, mtime == missingRoot, info.ModTime().UnixNano() != mtime:
			return nil, false
		}
	}
	// A project stops being one when its marker goes (e.g. .git deleted);
	// that does not touch any container's mtime.
	for _, p := range f.Projects {
		if _, err := os.Lstat(filepath.Join(p.Path, p.Marker)); err != nil {
			return nil, false
		}
	}
	return f.Projects, true
}

// fingerprint identifies the scan options; a config change invalidates the cache.
func (c *Cache) fingerprint() string {
	h := sha256.New()
	for _, part := range [][]string{c.Opts.Roots, c.Opts.Markers, c.Opts.Ignore, {strconv.Itoa(c.Opts.MaxDepth)}} {
		h.Write([]byte(strings.Join(part, "\x00")))
		h.Write([]byte{0xff})
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}
