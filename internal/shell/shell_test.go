package shell

import (
	"strings"
	"testing"

	"github.com/ardacoskun/codehop/internal/config"
)

func TestRenderAllShells(t *testing.T) {
	opts := Options{Cmd: "code", CdCmd: "cdp", Actions: []config.Action{{Name: "cc", Command: "claude"}}}
	for _, sh := range Supported {
		out, err := Render(sh, opts)
		if err != nil {
			t.Fatalf("%s: %v", sh, err)
		}
		for _, want := range []string{" wrap ", "query --record", "claude", "cdp", "cc"} {
			if !strings.Contains(out, want) {
				t.Errorf("%s output lacks %q", sh, want)
			}
		}
	}
}

func TestRenderSkipsDisabledCommands(t *testing.T) {
	out, err := Render("zsh", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "codehop wrap") || strings.Contains(out, "query --record") || strings.Contains(out, "compdef __codehop_complete_first") {
		t.Errorf("disabled commands still rendered:\n%s", out)
	}
}

func TestRenderRejectsBadInput(t *testing.T) {
	if _, err := Render("tcsh", Options{}); err == nil {
		t.Error("unsupported shell accepted")
	}
	if _, err := Render("zsh", Options{Cmd: "rm -rf"}); err == nil {
		t.Error("invalid command name accepted")
	}
	if _, err := Render("zsh", Options{Cmd: "code", CdCmd: "code"}); err == nil {
		t.Error("duplicate command name accepted")
	}
}

func TestNormalizeAliases(t *testing.T) {
	if got, err := Normalize("PWSH"); err != nil || got != "powershell" {
		t.Errorf("Normalize(PWSH) = %q, %v", got, err)
	}
}
