package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpand(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	t.Setenv("CODEHOP_TEST_DIR", "/opt/work")
	tests := map[string]string{
		"":                      "",
		"~":                     home,
		"~/projects":            filepath.Join(home, "projects"),
		"$CODEHOP_TEST_DIR/x":   filepath.Clean("/opt/work/x"),
		"${CODEHOP_TEST_DIR}/y": filepath.Clean("/opt/work/y"),
		"/a/b/../c":             filepath.Clean("/a/c"),
	}
	for in, want := range tests {
		if got := Expand(in); got != want {
			t.Errorf("Expand(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestExpandPercentVars(t *testing.T) {
	t.Setenv("CODEHOP_TEST_DIR", `C:\work`)
	if got := expandPercentVars(`%CODEHOP_TEST_DIR%\x %UNKNOWN_VAR_X% 100%`); got != `C:\work\x %UNKNOWN_VAR_X% 100%` {
		t.Errorf("expandPercentVars = %q", got)
	}
}

func TestPretty(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	if got := Pretty(filepath.Join(home, "projects", "x")); got != filepath.Join("~", "projects", "x") {
		t.Errorf("Pretty = %q", got)
	}
	if got := Pretty("/elsewhere"); got != "/elsewhere" {
		t.Errorf("Pretty = %q", got)
	}
}

func TestWriteFileAtomic(t *testing.T) {
	p := filepath.Join(t.TempDir(), "nested", "file.json")
	if err := WriteFileAtomic(p, []byte("one")); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(p, []byte("two")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(p)
	if err != nil || string(data) != "two" {
		t.Fatalf("content = %q, %v", data, err)
	}
	entries, _ := os.ReadDir(filepath.Dir(p))
	if len(entries) != 1 {
		t.Errorf("temp files left behind: %d entries", len(entries))
	}
}
