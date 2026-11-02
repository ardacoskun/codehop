// Package match ranks projects against a user query.
package match

import (
	"sort"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/ardacoskun/codehop/internal/discover"
)

// Tier is the strength of a match; higher is better.
type Tier int

const (
	// None means the query does not match.
	None Tier = iota
	// Fuzzy means the query's characters appear in order ("clrshf" → color-shuffle).
	Fuzzy
	// Substring means the query appears anywhere in the relative path.
	Substring
	// Word means a word of the name starts with the query ("shuffle" → color-shuffle),
	// or the query is a prefix of the name's initials ("cs" → color-shuffle).
	Word
	// Prefix means the name (or relative path, for queries with "/") starts with the query.
	Prefix
	// Exact means the query equals the key, name or relative path.
	Exact
)

// Candidate is a project that matched, with its tier and frecency.
type Candidate struct {
	Project  discover.Project
	Tier     Tier
	Frecency float64
}

// Result is a ranked match outcome.
type Result struct {
	// Candidates holds every match, best first.
	Candidates []Candidate
}

// Unique returns the single best candidate when exactly one project matches
// at the best tier. Fuzzy-only matches never count as unique: a typo should
// lead to a picker, not silently open an unrelated project.
func (r Result) Unique() (discover.Project, bool) {
	if len(r.Candidates) == 0 {
		return discover.Project{}, false
	}
	best := r.Candidates[0].Tier
	if best < Substring || (len(r.Candidates) > 1 && r.Candidates[1].Tier == best) {
		return discover.Project{}, false
	}
	return r.Candidates[0].Project, true
}

// Rank matches query against projects. frecency may be nil.
// Candidates are ordered by tier, then frecency, then key.
func Rank(query string, projects []discover.Project, frecency func(path string) float64) Result {
	q := normalize(query)
	var res Result
	if q == "" {
		return res
	}
	for _, p := range projects {
		tier := tierFor(q, p)
		if tier == None {
			continue
		}
		c := Candidate{Project: p, Tier: tier}
		if frecency != nil {
			c.Frecency = frecency(p.Path)
		}
		res.Candidates = append(res.Candidates, c)
	}
	sort.SliceStable(res.Candidates, func(i, j int) bool {
		a, b := res.Candidates[i], res.Candidates[j]
		if a.Tier != b.Tier {
			return a.Tier > b.Tier
		}
		if a.Frecency != b.Frecency {
			return a.Frecency > b.Frecency
		}
		return strings.ToLower(a.Project.Key) < strings.ToLower(b.Project.Key)
	})
	return res
}

// SortByFrecency orders projects by frecency (highest first), then key.
func SortByFrecency(projects []discover.Project, frecency func(path string) float64) []discover.Project {
	out := append([]discover.Project(nil), projects...)
	scores := make(map[string]float64, len(out))
	for _, p := range out {
		scores[p.Path] = frecency(p.Path)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if scores[out[i].Path] != scores[out[j].Path] {
			return scores[out[i].Path] > scores[out[j].Path]
		}
		return strings.ToLower(out[i].Key) < strings.ToLower(out[j].Key)
	})
	return out
}

func tierFor(q string, p discover.Project) Tier {
	key, name, rel := normalize(p.Key), normalize(p.Name), normalize(p.Rel)
	if q == key || q == name || q == rel {
		return Exact
	}
	target := name
	if strings.Contains(q, "/") {
		target = rel
	}
	if strings.HasPrefix(target, q) || strings.HasPrefix(key, q) {
		return Prefix
	}
	if !strings.Contains(q, "/") && matchesWord(q, p.Name) {
		return Word
	}
	if strings.Contains(rel, q) {
		return Substring
	}
	if isSubsequence(stripSeparators(q), stripSeparators(rel)) {
		return Fuzzy
	}
	return None
}

func matchesWord(q, name string) bool {
	words := splitWords(norm.NFC.String(name))
	initials := make([]rune, 0, len(words))
	for _, w := range words {
		if strings.HasPrefix(w, q) {
			return true
		}
		initials = append(initials, []rune(w)[0])
	}
	return len([]rune(q)) >= 2 && strings.HasPrefix(string(initials), q)
}

// splitWords splits on separators and camelCase boundaries, lowercased:
// "PromptToStream" → [prompt to stream], "color-shuffle" → [color shuffle].
func splitWords(s string) []string {
	var words []string
	var cur []rune
	runes := []rune(s)
	flush := func() {
		if len(cur) > 0 {
			words = append(words, strings.ToLower(string(cur)))
			cur = cur[:0]
		}
	}
	for i, r := range runes {
		switch {
		case isSeparator(r):
			flush()
		case unicode.IsUpper(r) && i > 0 && (unicode.IsLower(runes[i-1]) ||
			(i+1 < len(runes) && unicode.IsLower(runes[i+1]) && unicode.IsUpper(runes[i-1]))):
			flush()
			cur = append(cur, r)
		default:
			cur = append(cur, r)
		}
	}
	flush()
	return words
}

// Normalize lowercases and unifies separators the way project matching does,
// so "Color_Shuffle", "color shuffle" and "color-shuffle" compare equal.
func Normalize(s string) string { return normalize(s) }

// normalize lowercases and unifies separators so "Color_Shuffle", "color shuffle"
// and "color-shuffle" compare equal. Path separators become "/".
// Names are NFC-normalized first: macOS often stores "é" decomposed (NFD)
// while typed queries are composed (NFC).
func normalize(s string) string {
	s = strings.ToLower(strings.TrimSpace(norm.NFC.String(s)))
	s = strings.ReplaceAll(s, `\`, "/")
	return strings.Map(func(r rune) rune {
		if r == '_' || r == ' ' || r == '.' {
			return '-'
		}
		return r
	}, s)
}

func stripSeparators(s string) string {
	return strings.Map(func(r rune) rune {
		if isSeparator(r) || r == '/' {
			return -1
		}
		return r
	}, s)
}

func isSeparator(r rune) bool {
	return r == '-' || r == '_' || r == ' ' || r == '.'
}

func isSubsequence(needle, haystack string) bool {
	if needle == "" {
		return false
	}
	h := []rune(haystack)
	i := 0
	for _, r := range needle {
		for i < len(h) && h[i] != r {
			i++
		}
		if i == len(h) {
			return false
		}
		i++
	}
	return true
}
