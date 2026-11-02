package argv

import (
	"reflect"
	"testing"
)

func TestSplit(t *testing.T) {
	tests := map[string][]string{
		"":                         nil,
		"code":                     {"code"},
		"  code   -n  ":            {"code", "-n"},
		`"C:\Program Files\x" a`:   {`C:\Program Files\x`, "a"},
		`--height='50%' --x "a b"`: {"--height=50%", "--x", "a b"},
		`''`:                       {""},
	}
	for in, want := range tests {
		got, err := Split(in)
		if err != nil {
			t.Fatalf("Split(%q): %v", in, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("Split(%q) = %q, want %q", in, got, want)
		}
	}
	if _, err := Split(`"open`); err == nil {
		t.Error("unterminated quote should fail")
	}
}
