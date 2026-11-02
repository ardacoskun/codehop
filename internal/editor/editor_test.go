package editor

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestParsePresetAndCustom(t *testing.T) {
	tests := []struct {
		spec      string
		name      string
		cmdName   string
		openArgs  []string
		workspace bool
	}{
		{"code", "code", "code", []string{"/p"}, true},
		{"cursor --new-window", "cursor", "cursor", []string{"--new-window", "/p"}, true},
		{"zed", "zed", "zed", []string{"/p"}, false},
		{`"/opt/My Editor/bin/edit" --folder {path} --x`, "edit", "edit", []string{"--folder", "/p", "--x"}, false},
		{`C:\Tools\edit.exe`, "edit", "edit", []string{"/p"}, false},
	}
	for _, tt := range tests {
		e, err := Parse(tt.spec)
		if err != nil {
			t.Fatalf("Parse(%q): %v", tt.spec, err)
		}
		if e.Name != tt.name || e.CommandName() != tt.cmdName || e.Workspace != tt.workspace {
			t.Errorf("Parse(%q) = name %q cmd %q ws %v", tt.spec, e.Name, e.CommandName(), e.Workspace)
		}
		if got := e.OpenArgs("/p"); !reflect.DeepEqual(got, tt.openArgs) {
			t.Errorf("Parse(%q).OpenArgs = %v, want %v", tt.spec, got, tt.openArgs)
		}
	}
	if _, err := Parse(`"unterminated`); err == nil {
		t.Error("unterminated quote should fail")
	}
	if _, err := Parse("   "); err == nil {
		t.Error("empty editor should fail")
	}
}

func TestTargetPrefersSingleWorkspace(t *testing.T) {
	code, _ := Parse("code")
	zed, _ := Parse("zed")

	target := func(e Editor, dir string, prefer bool) string {
		got, _ := e.Target(dir, prefer)
		return got
	}
	dir := t.TempDir()
	if got := target(code, dir, true); got != dir {
		t.Fatalf("no workspace: got %s", got)
	}
	ws := filepath.Join(dir, "app.code-workspace")
	if err := os.WriteFile(ws, []byte(`{"folders": [{"path": "."}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := target(code, dir, true); got != ws {
		t.Errorf("one workspace: got %s, want %s", got, ws)
	}
	if got := target(code, dir, false); got != dir {
		t.Errorf("prefer_workspace=false: got %s", got)
	}
	if got := target(zed, dir, true); got != dir {
		t.Errorf("non-workspace editor: got %s", got)
	}
	if err := os.WriteFile(filepath.Join(dir, "other.code-workspace"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := target(code, dir, true); got != dir {
		t.Errorf("two workspaces: got %s, want the folder", got)
	}
}

func TestPassthroughArgsDropPathSlot(t *testing.T) {
	tests := map[string][]string{
		"code":                     nil,
		"cursor --new-window":      {"--new-window"},
		"edit --folder {path} --x": {"--x"},
		"edit --folder={path} --x": {"--x"},
		"edit --wait {path}":       nil,
		"edit -r --wait":           {"-r", "--wait"},
	}
	for spec, want := range tests {
		e, err := Parse(spec)
		if err != nil {
			t.Fatal(err)
		}
		if got := e.PassthroughArgs(); len(got) != len(want) || (len(want) > 0 && !reflect.DeepEqual(got, want)) {
			t.Errorf("Parse(%q).PassthroughArgs() = %q, want %q", spec, got, want)
		}
	}
}

func TestCheckBatchArgs(t *testing.T) {
	safe := []string{`C:\Users\me\projects\color-shuffle`, "-n", "R and D"}
	if err := checkBatchArgs("windows", `C:\VSCode\bin\code.cmd`, safe); err != nil {
		t.Errorf("safe args rejected: %v", err)
	}
	for _, bad := range []string{`C:\p\x&calc`, "a|b", "%PATH%", `say"hi`, "a^b", "x!y!"} {
		if err := checkBatchArgs("windows", `C:\VSCode\bin\code.CMD`, []string{bad}); err == nil {
			t.Errorf("%q accepted for a .cmd editor", bad)
		}
		if err := checkBatchArgs("windows", `C:\VSCode\Code.exe`, []string{bad}); err != nil {
			t.Errorf("%q rejected for an .exe editor: %v", bad, err)
		}
		if err := checkBatchArgs("darwin", "/usr/local/bin/code", []string{bad}); err != nil {
			t.Errorf("%q rejected on macOS: %v", bad, err)
		}
	}
}

func TestCheckWorkspace(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "packages", "web"), 0o755); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, content string
		safe          bool
	}{
		{"self", `{"folders": [{"path": "."}]}`, true},
		{"subfolders with JSONC", `{
			// VS Code allows comments and trailing commas
			"folders": [
				{"path": "packages/web"}, /* the app */
				{"path": "./packages"},
			],
			"settings": {"url": "http://example.com//not-a-comment"},
		}`, true},
		{"parent", `{"folders": [{"path": ".."}]}`, false},
		{"escape via subpath", `{"folders": [{"path": "packages/../../x"}]}`, false},
		{"absolute outside", `{"folders": [{"path": "/"}]}`, false},
		{"rooted backslash", `{"folders": [{"path": "\\\\Windows"}]}`, false},
		// "C:x" is drive-relative on Windows but a plain file name elsewhere.
		{"drive relative", `{"folders": [{"path": "C:x"}]}`, runtime.GOOS != "windows"},
		{"not created yet", `{"folders": [{"path": "packages/later"}]}`, true},
		{"remote uri", `{"folders": [{"uri": "vscode-remote://ssh-remote+evil/home"}]}`, false},
		{"no folders", `{"settings": {}}`, false},
		{"broken json", `{"folders": [`, false},
	}
	for _, tt := range tests {
		file := filepath.Join(dir, "app.code-workspace")
		if err := os.WriteFile(file, []byte(tt.content), 0o644); err != nil {
			t.Fatal(err)
		}
		err := checkWorkspace(file, dir)
		if (err == nil) != tt.safe {
			t.Errorf("%s: checkWorkspace error = %v, want safe=%v", tt.name, err, tt.safe)
		}
	}
}
