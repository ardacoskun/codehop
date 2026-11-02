package match

import (
	"testing"

	"github.com/ardacoskun/codehop/internal/discover"
)

func project(rel string) discover.Project {
	name := rel
	for i := len(rel) - 1; i >= 0; i-- {
		if rel[i] == '/' {
			name = rel[i+1:]
			break
		}
	}
	return discover.Project{Key: name, Name: name, Path: "/root/" + rel, Root: "/root", Rel: rel}
}

var fixture = []discover.Project{
	project("games/color-shuffle"),
	project("games/cs-develop"),
	project("games/cs-release"),
	project("web/creative-engine"),
	project("web/PromptToStream"),
	project("rn/lamekan"),
	project("rn/lamekan-v1"),
	project("web/murid-admin"),
	project("web/murid-landing"),
}

func TestRankUnique(t *testing.T) {
	tests := []struct {
		query string
		want  string // empty: no unique match
	}{
		{"color-shuffle", "color-shuffle"},
		{"COLOR_SHUFFLE", "color-shuffle"},
		{"color shuffle", "color-shuffle"},
		{"color", "color-shuffle"},
		{"shuffle", "color-shuffle"},
		{"ce", "creative-engine"},
		{"pts", "PromptToStream"},
		{"stream", "PromptToStream"},
		{"lamekan", "lamekan"},
		{"web/murid-admin", "murid-admin"},
		{"games/color", "color-shuffle"},
		{"cs", ""},     // several prefix matches
		{"murid", ""},  // two prefix matches
		{"clrshf", ""}, // fuzzy-only never auto-opens
		{"zzz", ""},    // nothing
		{"", ""},       // empty query
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			p, ok := Rank(tt.query, fixture, nil).Unique()
			if tt.want == "" {
				if ok {
					t.Fatalf("Rank(%q) unique = %s, want none", tt.query, p.Key)
				}
				return
			}
			if !ok || p.Key != tt.want {
				t.Fatalf("Rank(%q) unique = %q (ok=%v), want %q", tt.query, p.Key, ok, tt.want)
			}
		})
	}
}

func TestRankTiers(t *testing.T) {
	tests := []struct {
		query string
		key   string
		want  Tier
	}{
		{"lamekan", "lamekan", Exact},
		{"lamekan", "lamekan-v1", Prefix},
		{"shuffle", "color-shuffle", Word},
		{"cs", "color-shuffle", Word},
		{"ffle", "color-shuffle", Substring},
		{"clrshf", "color-shuffle", Fuzzy},
	}
	for _, tt := range tests {
		got := None
		for _, c := range Rank(tt.query, fixture, nil).Candidates {
			if c.Project.Key == tt.key {
				got = c.Tier
			}
		}
		if got != tt.want {
			t.Errorf("tier(%q, %s) = %d, want %d", tt.query, tt.key, got, tt.want)
		}
	}
}

func TestRankFrecencyBreaksTies(t *testing.T) {
	frecency := func(path string) float64 {
		if path == "/root/games/cs-release" {
			return 10
		}
		return 0
	}
	res := Rank("cs-", fixture, frecency)
	if len(res.Candidates) < 2 || res.Candidates[0].Project.Key != "cs-release" {
		t.Fatalf("first candidate = %v, want cs-release", res.Candidates)
	}
}

func TestSortByFrecency(t *testing.T) {
	frecency := func(path string) float64 {
		if path == "/root/rn/lamekan-v1" {
			return 1
		}
		return 0
	}
	got := SortByFrecency(fixture, frecency)
	if got[0].Key != "lamekan-v1" || got[1].Key != "color-shuffle" {
		t.Fatalf("order = %s, %s; want lamekan-v1, color-shuffle", got[0].Key, got[1].Key)
	}
}

func TestSplitWords(t *testing.T) {
	tests := map[string][]string{
		"PromptToStream": {"prompt", "to", "stream"},
		"color-shuffle":  {"color", "shuffle"},
		"my_app.v2":      {"my", "app", "v2"},
		"HTTPServer":     {"http", "server"},
	}
	for in, want := range tests {
		got := splitWords(in)
		if len(got) != len(want) {
			t.Fatalf("splitWords(%q) = %v, want %v", in, got, want)
		}
		for i := range got {
			if got[i] != want[i] {
				t.Fatalf("splitWords(%q) = %v, want %v", in, got, want)
			}
		}
	}
}

func TestRankUnicodeNormalization(t *testing.T) {
	decomposed := project("web/cafe\u0301-site") // "café" as e + combining accent (NFD)
	p, ok := Rank("caf\u00e9", []discover.Project{decomposed}, nil).Unique()
	if !ok || p.Key != decomposed.Key {
		t.Fatalf("NFC query did not match NFD folder name")
	}
}
