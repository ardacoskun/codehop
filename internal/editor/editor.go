// Package editor turns the configured editor into a command line and runs it.
package editor

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ardacoskun/codehop/internal/argv"
)

// Preset describes a known editor.
type Preset struct {
	// Bins are executable names tried in order.
	Bins []string
	// Args are passed before the path.
	Args []string
	// Terminal editors run in the foreground and take over the terminal.
	Terminal bool
	// Workspace editors understand *.code-workspace files.
	Workspace bool
}

// Presets maps preset names to editors.
var Presets = map[string]Preset{
	"code":          {Bins: []string{"code"}, Workspace: true},
	"code-insiders": {Bins: []string{"code-insiders"}, Workspace: true},
	"cursor":        {Bins: []string{"cursor"}, Workspace: true},
	"windsurf":      {Bins: []string{"windsurf"}, Workspace: true},
	"codium":        {Bins: []string{"codium", "vscodium"}, Workspace: true},
	"zed":           {Bins: []string{"zed", "zeditor"}},
	"subl":          {Bins: []string{"subl"}},
	"idea":          {Bins: []string{"idea", "idea64"}},
	"webstorm":      {Bins: []string{"webstorm", "webstorm64"}},
	"goland":        {Bins: []string{"goland", "goland64"}},
	"pycharm":       {Bins: []string{"pycharm", "pycharm64", "charm"}},
	"nvim":          {Bins: []string{"nvim"}, Terminal: true},
	"vim":           {Bins: []string{"vim"}, Terminal: true},
	"hx":            {Bins: []string{"hx", "helix"}, Terminal: true},
}

// pathPlaceholder marks where the project path goes in a custom command.
const pathPlaceholder = "{path}"

// Editor is a resolved editor command.
type Editor struct {
	// Name is the preset name, or the executable name for custom commands.
	Name string
	// Bin is the executable as written in the config (not yet looked up).
	Bin string
	// Args are the arguments, possibly containing {path}.
	Args      []string
	Terminal  bool
	Workspace bool
	// candidates are the executable names tried by Lookup.
	candidates []string
}

// Parse resolves a config value: a preset name or a custom command line.
func Parse(spec string) (Editor, error) {
	fields, err := argv.Split(spec)
	if err != nil {
		return Editor{}, fmt.Errorf("editor %q: %w", spec, err)
	}
	if len(fields) == 0 {
		return Editor{}, errors.New("editor is empty")
	}
	if p, ok := Presets[fields[0]]; ok {
		return Editor{
			Name:       fields[0],
			Bin:        p.Bins[0],
			Args:       append(append([]string(nil), p.Args...), fields[1:]...),
			Terminal:   p.Terminal,
			Workspace:  p.Workspace,
			candidates: p.Bins,
		}, nil
	}
	return Editor{Name: commandBase(fields[0]), Bin: fields[0], Args: fields[1:], candidates: fields[:1]}, nil
}

// CommandName is the name a shell wrapper is installed under by default:
// the preset's (or command's) executable name, e.g. "code" or "cursor".
func (e Editor) CommandName() string {
	return commandBase(e.Bin)
}

// commandBase returns the executable name without directory or extension,
// accepting both / and \ so Windows paths in a shared config work anywhere.
func commandBase(bin string) string {
	if i := strings.LastIndexAny(bin, `/\`); i >= 0 {
		bin = bin[i+1:]
	}
	return strings.TrimSuffix(bin, filepath.Ext(bin))
}

// Lookup finds the editor executable on PATH.
func (e Editor) Lookup() (string, error) {
	for _, bin := range e.candidates {
		if path, err := exec.LookPath(bin); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("editor %q not found on PATH (tried %s)", e.Name, strings.Join(e.candidates, ", "))
}

// OpenArgs returns the arguments that open target, honoring {path}.
func (e Editor) OpenArgs(target string) []string {
	args := make([]string, 0, len(e.Args)+1)
	placed := false
	for _, a := range e.Args {
		if strings.Contains(a, pathPlaceholder) {
			a = strings.ReplaceAll(a, pathPlaceholder, target)
			placed = true
		}
		args = append(args, a)
	}
	if !placed {
		args = append(args, target)
	}
	return args
}

// Open opens target (a directory or workspace file) in the editor.
func (e Editor) Open(target string) error {
	return e.run(e.OpenArgs(target))
}

// Passthrough runs the editor with the user's raw arguments, as if the
// wrapper did not exist (e.g. "code .", "code -n file.txt").
func (e Editor) Passthrough(args []string) error {
	return e.run(append(e.PassthroughArgs(), args...))
}

// PassthroughArgs are the configured arguments without the {path} slot:
// "--folder {path}" and "--folder={path}" are dropped, other flags are kept.
func (e Editor) PassthroughArgs() []string {
	var out []string
	for i, a := range e.Args {
		if !strings.Contains(a, pathPlaceholder) {
			out = append(out, a)
			continue
		}
		prev := i - 1
		if a == pathPlaceholder && prev >= 0 && strings.HasPrefix(e.Args[prev], "-") &&
			len(out) > 0 && out[len(out)-1] == e.Args[prev] {
			out = out[:len(out)-1]
		}
	}
	return out
}

func (e Editor) run(args []string) error {
	bin, err := e.Lookup()
	if err != nil {
		return err
	}
	if err := checkBatchArgs(runtime.GOOS, bin, args); err != nil {
		return err
	}
	cmd := exec.Command(bin, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

// batchMetachars are interpreted by cmd.exe even inside quotes.
const batchMetachars = "&|<>^%!\"\r\n"

// checkBatchArgs refuses arguments that cmd.exe could interpret when the
// editor is a .cmd/.bat script on Windows (VS Code's `code` is code.cmd).
// Windows runs such scripts through cmd.exe, and Go cannot quote arguments
// safely for it, so a folder named "x&calc" would otherwise run calc.
func checkBatchArgs(goos, bin string, args []string) error {
	if goos != "windows" {
		return nil
	}
	ext := strings.ToLower(filepath.Ext(bin))
	if ext != ".cmd" && ext != ".bat" {
		return nil
	}
	for _, a := range args {
		if strings.ContainsAny(a, batchMetachars) {
			return fmt.Errorf("refusing to pass %q to %s: it contains characters cmd.exe would interpret "+
				"(%s); rename the folder or set editor to the editor's .exe", a, filepath.Base(bin), "& | < > ^ % ! \"")
		}
	}
	return nil
}

// Target returns what to open for a project directory: its single
// *.code-workspace file when preferWorkspace is set, the editor supports it
// and the file only opens folders inside the project; otherwise the
// directory itself. note explains why a workspace file was skipped.
func (e Editor) Target(dir string, preferWorkspace bool) (target, note string) {
	if !preferWorkspace || !e.Workspace {
		return dir, ""
	}
	matches, err := filepath.Glob(filepath.Join(dir, "*.code-workspace"))
	if err != nil || len(matches) != 1 {
		return dir, ""
	}
	if err := checkWorkspace(matches[0], dir); err != nil {
		return dir, fmt.Sprintf("%v; opening the folder instead", err)
	}
	return matches[0], ""
}
