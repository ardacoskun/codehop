// Package shell renders the per-shell integration printed by `codehop init`.
package shell

import (
	"bytes"
	"embed"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"text/template"

	"github.com/ardacoskun/codehop/internal/config"
)

//go:embed templates/*
var templates embed.FS

// Supported lists the shells `codehop init` accepts.
var Supported = []string{"bash", "fish", "powershell", "zsh"}

var aliases = map[string]string{"pwsh": "powershell"}

var commandNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)

// Options are the user's choices for the generated integration.
type Options struct {
	// Cmd is the name of the open/wrapper command (e.g. "code"); empty skips it.
	Cmd string
	// CdCmd is the name of the jump command (e.g. "cdp"); empty skips it.
	CdCmd string
	// Actions become commands that cd into a project and run a command.
	Actions []config.Action
}

type templateData struct {
	Options
	// CompleteNames are all commands that complete project names as their
	// first argument (CdCmd and actions).
	CompleteNames []string
	// AllNames are every command the integration defines.
	AllNames []string
}

// Normalize maps aliases such as "pwsh" to a supported shell name.
func Normalize(name string) (string, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if a, ok := aliases[name]; ok {
		name = a
	}
	i := sort.SearchStrings(Supported, name)
	if i == len(Supported) || Supported[i] != name {
		return "", fmt.Errorf("unsupported shell %q (supported: %s)", name, strings.Join(Supported, ", "))
	}
	return name, nil
}

// Render returns the integration script for shellName.
func Render(shellName string, opts Options) (string, error) {
	name, err := Normalize(shellName)
	if err != nil {
		return "", err
	}
	if err := validate(opts); err != nil {
		return "", err
	}
	tmpl, err := template.New(name).Funcs(template.FuncMap{
		"join": strings.Join,
	}).ParseFS(templates, "templates/"+name+".tmpl")
	if err != nil {
		return "", err
	}
	data := templateData{Options: opts}
	if opts.CdCmd != "" {
		data.CompleteNames = append(data.CompleteNames, opts.CdCmd)
	}
	for _, a := range opts.Actions {
		data.CompleteNames = append(data.CompleteNames, a.Name)
	}
	if opts.Cmd != "" {
		data.AllNames = append(data.AllNames, opts.Cmd)
	}
	data.AllNames = append(data.AllNames, data.CompleteNames...)
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, name+".tmpl", data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func validate(opts Options) error {
	seen := map[string]bool{}
	names := []string{opts.Cmd, opts.CdCmd}
	for _, a := range opts.Actions {
		names = append(names, a.Name)
	}
	for _, n := range names {
		if n == "" {
			continue
		}
		if !commandNamePattern.MatchString(n) {
			return fmt.Errorf("invalid command name %q", n)
		}
		if seen[n] {
			return fmt.Errorf("command name %q is used twice (check --cmd, --cd-cmd and [actions])", n)
		}
		seen[n] = true
	}
	return nil
}
