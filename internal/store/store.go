// Package store keeps per-project visit statistics and turns them into a
// frecency score (frequency weighted by recency), in the style of zoxide.
package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/ardacoskun/codehop/internal/paths"
)

const (
	dbVersion = 1
	// maxTotalRank bounds the sum of all ranks; above it every rank is scaled
	// down and entries that fall below minRank are forgotten.
	maxTotalRank = 1000.0
	agingFactor  = 0.9
	minRank      = 1.0

	hour = int64(time.Hour / time.Second)
	day  = 24 * hour
	week = 7 * day
)

// Entry is the visit record of one project path.
type Entry struct {
	Path       string  `json:"path"`
	Rank       float64 `json:"rank"`
	LastAccess int64   `json:"last_access"`
}

type dbFile struct {
	Version int     `json:"version"`
	Entries []Entry `json:"entries"`
}

// DB is the frecency database. It is not safe for concurrent use; concurrent
// processes may lose a visit (last writer wins), which only nudges ordering.
type DB struct {
	path    string
	entries map[string]*Entry
}

// Open reads the database at <dataDir>/db.json. A missing or corrupt file
// yields an empty database rather than an error, so ordering degrades
// gracefully instead of breaking the shell.
func Open(dataDir string) *DB {
	db := &DB{path: filepath.Join(dataDir, "db.json"), entries: map[string]*Entry{}}
	data, err := os.ReadFile(db.path)
	if err != nil {
		return db
	}
	var f dbFile
	if json.Unmarshal(data, &f) != nil || f.Version != dbVersion {
		return db
	}
	for i := range f.Entries {
		e := f.Entries[i]
		db.entries[e.Path] = &e
	}
	return db
}

// Path returns the database file path.
func (db *DB) Path() string { return db.path }

// Visit records one visit to path at time now.
func (db *DB) Visit(path string, now time.Time) {
	e, ok := db.entries[path]
	if !ok {
		e = &Entry{Path: path}
		db.entries[path] = e
	}
	e.Rank++
	e.LastAccess = now.Unix()
	db.age(path)
}

// Score returns the frecency of path at time now (0 if never visited).
func (db *DB) Score(path string, now time.Time) float64 {
	e, ok := db.entries[path]
	if !ok {
		return 0
	}
	elapsed := now.Unix() - e.LastAccess
	switch {
	case elapsed < hour:
		return e.Rank * 4
	case elapsed < day:
		return e.Rank * 2
	case elapsed < week:
		return e.Rank / 2
	default:
		return e.Rank / 4
	}
}

// Prune removes entries whose directory no longer exists.
func (db *DB) Prune() {
	for path := range db.entries {
		if _, err := os.Stat(path); err != nil {
			delete(db.entries, path)
		}
	}
}

// Save writes the database atomically.
func (db *DB) Save() error {
	f := dbFile{Version: dbVersion, Entries: make([]Entry, 0, len(db.entries))}
	for _, e := range db.entries {
		f.Entries = append(f.Entries, *e)
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return paths.WriteFileAtomic(db.path, data)
}

// age scales every rank down once the total passes maxTotalRank and forgets
// entries that fall below minRank, except keep (the path just visited).
func (db *DB) age(keep string) {
	total := 0.0
	for _, e := range db.entries {
		total += e.Rank
	}
	if total <= maxTotalRank {
		return
	}
	scale := agingFactor * maxTotalRank / total
	for path, e := range db.entries {
		e.Rank *= scale
		if e.Rank < minRank && path != keep {
			delete(db.entries, path)
		}
	}
}
