// Package config loads codehop's TOML configuration and applies environment
// overrides on top of the built-in defaults.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/ardacoskun/codehop/internal/paths"
)

const (
	// DefaultMaxDepth is how many directory levels below a root are searched.
	DefaultMaxDepth = 3
	// MaxAllowedDepth caps max_depth so a bad value cannot walk the whole disk.
	MaxAllowedDepth = 8
	// DefaultEditor is used when neither the config nor CODEHOP_EDITOR names one.
	DefaultEditor = "code"
)

// DefaultRootCandidates are tried, in order, when no roots are configured;
// only the ones that exist are used.
var DefaultRootCandidates = []string{
	"~/projects", "~/Projects", "~/code", "~/Code", "~/src", "~/dev",
	"~/Developer", "~/repos", "~/workspace", "~/git",
}

// DefaultMarkers are file or directory names (globs allowed) whose presence
// makes a directory a project.
var DefaultMarkers = []string{
	".git", ".codehop", "package.json", "go.mod", "Cargo.toml", "pyproject.toml",
	"pubspec.yaml", "composer.json", "Gemfile", "pom.xml", "build.gradle",
	"build.gradle.kts", "Package.swift", "deno.json", "mix.exs", "*.sln",
	"*.code-workspace",
}

// DefaultIgnore are directory names never descended into.
var DefaultIgnore = []string{
	"node_modules", "vendor", "dist", "build", "target", "Pods", "DerivedData",
	"venv", ".venv", "__pycache__",
}

var actionNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)

// Config is the effective configuration after defaults, file and environment.
type Config struct {
	Roots           []string          `toml:"roots"`
	MaxDepth        int               `toml:"max_depth"`
	Markers         []string          `toml:"markers"`
	Ignore          []string          `toml:"ignore"`
	Editor          string            `toml:"editor"`
	PreferWorkspace *bool             `toml:"prefer_workspace"`
	CloneRoot       string            `toml:"clone_root"`
	FzfOpts         string            `toml:"fzf_opts"`
	Actions         map[string]string `toml:"actions"`

	// Path is the config file that was read, or would be read if it existed.
	Path string `toml:"-"`
	// Loaded reports whether the file at Path existed and was parsed.
	Loaded bool `toml:"-"`
	// Warnings collects non-fatal problems such as unknown keys.
	Warnings []string `toml:"-"`
}

// Action is a named shell command run inside a project after cd'ing into it.
type Action struct {
	Name    string
	Command string
}

// Load reads the config file (if any), applies CODEHOP_* environment
// overrides and fills defaults.
func Load() (*Config, error) {
	return LoadFile(paths.ConfigFile())
}

// LoadFile is Load with an explicit config path.
func LoadFile(path string) (*Config, error) {
	cfg := &Config{Path: path}
	meta, err := toml.DecodeFile(path, cfg)
	switch {
	case err == nil:
		cfg.Loaded = true
		for _, key := range meta.Undecoded() {
			cfg.Warnings = append(cfg.Warnings, fmt.Sprintf("unknown config key %q", key.String()))
		}
	case errors.Is(err, fs.ErrNotExist):
		// No config file: defaults only.
	default:
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	cfg.applyEnv()
	if err := cfg.normalize(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

func (c *Config) applyEnv() {
	if v := os.Getenv("CODEHOP_ROOTS"); v != "" {
		c.Roots = filepath.SplitList(v)
	}
	if v := os.Getenv("CODEHOP_EDITOR"); v != "" {
		c.Editor = v
	}
	if v := os.Getenv("CODEHOP_FZF_OPTS"); v != "" {
		c.FzfOpts = v
	}
}

func (c *Config) normalize() error {
	if len(c.Roots) == 0 {
		for _, candidate := range DefaultRootCandidates {
			if isDir(paths.Expand(candidate)) {
				c.Roots = append(c.Roots, candidate)
			}
		}
	}
	c.Roots = expandAll(c.Roots)

	switch {
	case c.MaxDepth == 0:
		c.MaxDepth = DefaultMaxDepth
	case c.MaxDepth < 0 || c.MaxDepth > MaxAllowedDepth:
		return fmt.Errorf("max_depth must be between 1 and %d, got %d", MaxAllowedDepth, c.MaxDepth)
	}

	if c.Markers == nil {
		c.Markers = DefaultMarkers
	}
	if c.Ignore == nil {
		c.Ignore = DefaultIgnore
	}
	c.Editor = strings.TrimSpace(c.Editor)
	if c.Editor == "" {
		c.Editor = DefaultEditor
	}
	if c.PreferWorkspace == nil {
		preferWorkspace := true
		c.PreferWorkspace = &preferWorkspace
	}
	if c.CloneRoot != "" {
		c.CloneRoot = paths.Expand(c.CloneRoot)
	}

	for name := range c.Actions {
		if !actionNamePattern.MatchString(name) {
			return fmt.Errorf("action name %q must start with a letter or _ and contain only letters, digits, _ and -", name)
		}
		if strings.TrimSpace(c.Actions[name]) == "" {
			return fmt.Errorf("action %q has an empty command", name)
		}
	}
	return nil
}

// ActionList returns the configured actions sorted by name.
func (c *Config) ActionList() []Action {
	names := make([]string, 0, len(c.Actions))
	for name := range c.Actions {
		names = append(names, name)
	}
	sort.Strings(names)
	actions := make([]Action, 0, len(names))
	for _, name := range names {
		actions = append(actions, Action{Name: name, Command: c.Actions[name]})
	}
	return actions
}

// CloneDestinationRoot is where clone-and-open puts new repositories:
// clone_root if set, otherwise the first root.
func (c *Config) CloneDestinationRoot() (string, error) {
	if c.CloneRoot != "" {
		return c.CloneRoot, nil
	}
	if len(c.Roots) > 0 {
		return c.Roots[0], nil
	}
	return "", errors.New("no roots configured; set roots or clone_root in the config")
}

// expandAll expands roots and drops duplicates, including the same folder
// spelled differently (~/projects vs ~/Projects on case-insensitive disks,
// or a symlink to another root).
func expandAll(in []string) []string {
	out := make([]string, 0, len(in))
	var infos []os.FileInfo
	for _, p := range in {
		p = paths.Expand(p)
		if p == "" {
			continue
		}
		info, err := os.Stat(p)
		if err == nil && containsSameFile(infos, info) {
			continue
		}
		if err == nil {
			infos = append(infos, info)
		} else if slices.Contains(out, p) {
			continue
		}
		out = append(out, p)
	}
	return out
}

func containsSameFile(infos []os.FileInfo, info os.FileInfo) bool {
	for _, seen := range infos {
		if os.SameFile(seen, info) {
			return true
		}
	}
	return false
}

func isDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}
