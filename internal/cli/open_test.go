package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ardacoskun/codehop/internal/discover"
	"github.com/ardacoskun/codehop/internal/match"
)

func TestIsPassthrough(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		args []string
		want bool
	}{
		{nil, false},
		{[]string{"color-shuffle"}, false},
		{[]string{"."}, true},
		{[]string{file}, true},
		{[]string{dir}, true},
		{[]string{"-n"}, true},
		{[]string{"--diff"}, true},
		{[]string{"-"}, true},
		{[]string{"a", "b"}, true},
	}
	for _, tt := range tests {
		if got := isPassthrough(tt.args); got != tt.want {
			t.Errorf("isPassthrough(%q) = %v, want %v", tt.args, got, tt.want)
		}
	}
}

func TestLooksLikePath(t *testing.T) {
	for q, want := range map[string]bool{
		"./new.md": true, "../x": true, "~/notes": true, "/tmp/x": true, ".env": true,
		"notes.md": false, "color-shuffle": false, "web/blog": false,
	} {
		if got := looksLikePath(q); got != want {
			t.Errorf("looksLikePath(%q) = %v, want %v", q, got, want)
		}
	}
}

func TestPickerChoicesDropsFuzzyNoise(t *testing.T) {
	p := func(k string) discover.Project { return discover.Project{Key: k} }
	mixed := []match.Candidate{{Project: p("cs-a"), Tier: match.Prefix}, {Project: p("x"), Tier: match.Fuzzy}}
	if got := pickerChoices(mixed); len(got) != 1 || got[0].Key != "cs-a" {
		t.Errorf("mixed = %v", got)
	}
	fuzzy := []match.Candidate{{Project: p("a"), Tier: match.Fuzzy}, {Project: p("b"), Tier: match.Fuzzy}}
	if got := pickerChoices(fuzzy); len(got) != 2 {
		t.Errorf("fuzzy-only = %v", got)
	}
}
