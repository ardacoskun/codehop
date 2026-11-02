package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadMissingFileUsesDefaults(t *testing.T) {
	t.Setenv("CODEHOP_ROOTS", "")
	t.Setenv("CODEHOP_EDITOR", "")
	cfg, err := LoadFile(filepath.Join(t.TempDir(), "nope.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Loaded || cfg.MaxDepth != DefaultMaxDepth || cfg.Editor != DefaultEditor || !*cfg.PreferWorkspace {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}

func TestLoadFileAndEnvOverrides(t *testing.T) {
	root := t.TempDir()
	p := writeConfig(t, `
roots = ["`+filepath.ToSlash(root)+`"]
max_depth = 2
editor = "cursor"
prefer_workspace = false
mystery = 1

[actions]
cc = "claude"
`)
	t.Setenv("CODEHOP_ROOTS", "")
	t.Setenv("CODEHOP_EDITOR", "zed")
	cfg, err := LoadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Loaded || cfg.MaxDepth != 2 || *cfg.PreferWorkspace {
		t.Fatalf("file values not applied: %+v", cfg)
	}
	if cfg.Editor != "zed" {
		t.Errorf("CODEHOP_EDITOR override: editor = %q", cfg.Editor)
	}
	if len(cfg.Roots) != 1 || cfg.Roots[0] != filepath.Clean(root) {
		t.Errorf("roots = %v", cfg.Roots)
	}
	if len(cfg.Warnings) != 1 || !strings.Contains(cfg.Warnings[0], "mystery") {
		t.Errorf("warnings = %v", cfg.Warnings)
	}
	if a := cfg.ActionList(); len(a) != 1 || a[0].Name != "cc" || a[0].Command != "claude" {
		t.Errorf("actions = %v", a)
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	for _, content := range []string{
		"max_depth = 99",
		"[actions]\n\"bad name\" = \"x\"",
		"[actions]\nempty = \"  \"",
		"roots = [",
	} {
		if _, err := LoadFile(writeConfig(t, content)); err == nil {
			t.Errorf("config %q accepted", content)
		}
	}
}

func TestRootsDeduplicated(t *testing.T) {
	root := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(root, link); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	got := expandAll([]string{root, link, root, filepath.Join(root, "missing"), filepath.Join(root, "missing")})
	if len(got) != 2 {
		t.Fatalf("expandAll = %v, want root and one missing path", got)
	}
}
