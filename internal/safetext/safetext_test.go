package safetext

import "testing"

func TestSanitize(t *testing.T) {
	tests := map[string]string{
		"color-shuffle":        "color-shuffle",
		"ünïcødé proje":        "ünïcødé proje",
		"a\tb":                 "a b",
		"x\n3\tlegit":          "x?3 legit",
		"\x1b]52;c;ZXZpbA==\a": "?]52;c;ZXZpbA==?",
		"\x1b[2J":              "?[2J",
		"del\x7f c1\u009b":     "del? c1?",
		"bidi\u202egnp.exe":    "bidi?gnp.exe",
		"isolate\u2066x\u2069": "isolate?x?",
	}
	for in, want := range tests {
		if got := Sanitize(in); got != want {
			t.Errorf("Sanitize(%q) = %q, want %q", in, got, want)
		}
		if HasUnsafe(in) != (in != want) {
			t.Errorf("HasUnsafe(%q) = %v", in, HasUnsafe(in))
		}
	}
}
