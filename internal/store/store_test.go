package store

import (
	"testing"
	"time"
)

func TestScoreDecaysWithAge(t *testing.T) {
	db := Open(t.TempDir())
	now := time.Unix(1_700_000_000, 0)
	db.Visit("/p", now)

	tests := []struct {
		after time.Duration
		want  float64
	}{
		{time.Minute, 4},
		{2 * time.Hour, 2},
		{3 * 24 * time.Hour, 0.5},
		{30 * 24 * time.Hour, 0.25},
	}
	for _, tt := range tests {
		if got := db.Score("/p", now.Add(tt.after)); got != tt.want {
			t.Errorf("score after %v = %v, want %v", tt.after, got, tt.want)
		}
	}
	if got := db.Score("/unknown", now); got != 0 {
		t.Errorf("unknown score = %v", got)
	}
}

func TestSaveAndReopen(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	db := Open(dir)
	db.Visit("/a", now)
	db.Visit("/a", now)
	db.Visit("/b", now)
	if err := db.Save(); err != nil {
		t.Fatal(err)
	}
	re := Open(dir)
	if re.Score("/a", now) <= re.Score("/b", now) {
		t.Fatalf("a=%v b=%v, want a > b", re.Score("/a", now), re.Score("/b", now))
	}
}

func TestAgingBoundsTotalRank(t *testing.T) {
	db := Open(t.TempDir())
	now := time.Now()
	for i := 0; i < 3000; i++ {
		db.Visit("/hot", now)
	}
	total := 0.0
	for _, e := range db.entries {
		total += e.Rank
	}
	if total > maxTotalRank {
		t.Fatalf("total rank %v exceeds %v", total, maxTotalRank)
	}
}

func TestPruneDropsMissingPaths(t *testing.T) {
	dir := t.TempDir()
	db := Open(dir)
	db.Visit(dir, time.Now())
	db.Visit("/definitely/not/here", time.Now())
	db.Prune()
	if len(db.entries) != 1 {
		t.Fatalf("entries after prune = %d, want 1", len(db.entries))
	}
}

func TestNewVisitSurvivesAging(t *testing.T) {
	db := Open(t.TempDir())
	now := time.Now()
	for i := 0; i < 2000; i++ {
		db.Visit("/hot", now)
	}
	db.Visit("/new", now)
	if db.Score("/new", now) == 0 {
		t.Fatal("the project just visited was forgotten by aging")
	}
}
