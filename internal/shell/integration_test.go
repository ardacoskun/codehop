package shell_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var (
	codehopBin string
	editorBin  string
)

// TestMain builds codehop and the fake editor once for all shell tests.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "codehop-shell-test")
	if err != nil {
		panic(err)
	}
	exe := ""
	if runtime.GOOS == "windows" {
		exe = ".exe"
	}
	codehopBin = filepath.Join(dir, "codehop"+exe)
	editorBin = filepath.Join(dir, "fakeeditor"+exe)
	for target, pkg := range map[string]string{codehopBin: "../../cmd/codehop", editorBin: "./testdata/fakeeditor"} {
		if out, err := exec.Command("go", "build", "-o", target, pkg).CombinedOutput(); err != nil {
			panic(string(out))
		}
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

type env struct {
	root, home, log string
	vars            []string
}

// newEnv creates a project tree and a config whose editor is the fake editor.
func newEnv(t *testing.T) env {
	t.Helper()
	base := t.TempDir()
	e := env{
		root: filepath.Join(base, "projects"),
		home: filepath.Join(base, "home"),
		log:  filepath.Join(base, "editor.log"),
	}
	dirs := []string{
		"games/color-shuffle/.git", "web/blog/.git", "web/murid-admin/.git", "web/murid-landing/.git",
		"misc/My App/.git", "misc/a;b/.git",
	}
	if runtime.GOOS != "windows" {
		// Terminal escape in a folder name: must never be listed or printed.
		dirs = append(dirs, "misc/evil\x1b[2Jname/.git")
	}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(e.root, filepath.FromSlash(d)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(e.home, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(base, "config.toml")
	content := "roots = [" + quoteTOML(e.root) + "]\neditor = " + quoteTOML(`"`+editorBin+`"`) + "\n[actions]\nrun = \"echo ACTION\"\n"
	if err := os.WriteFile(cfg, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	e.vars = append(os.Environ(),
		"CODEHOP_CONFIG="+cfg,
		"CODEHOP_DATA_DIR="+filepath.Join(base, "data"),
		"CODEHOP_CACHE_DIR="+filepath.Join(base, "cache"),
		"FAKE_EDITOR_LOG="+e.log,
		"PATH="+filepath.Dir(codehopBin)+string(os.PathListSeparator)+os.Getenv("PATH"),
		"HOME="+e.home,
	)
	return e
}

func quoteTOML(s string) string {
	return "'" + s + "'"
}

func (e env) editorCalls(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(e.log)
	if err != nil {
		return nil
	}
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

func (e env) run(t *testing.T, shell string, args ...string) string {
	t.Helper()
	cmd := exec.Command(shell, args...)
	cmd.Env = e.vars
	cmd.Dir = e.home
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s failed: %v\n%s", shell, err, out)
	}
	return string(out)
}

func requireShell(t *testing.T, name string) string {
	t.Helper()
	path, err := exec.LookPath(name)
	if err != nil {
		t.Skipf("%s not installed", name)
	}
	return path
}

// posixScript exercises code, cdp and actions; the same body works in zsh and
// bash. A pre-existing alias must not break loading the integration.
const posixPrelude = `alias code=false
alias run=false
`

const posixScript = `
code color-shuffle
code shuffle
code .
code -n notes.txt
code -u NONE f.txt
cdp blog && pwd
run murid-admin extra-arg
run blog
echo RUN_DONE
codehop list --names
`

func checkPosixResults(t *testing.T, e env, out string) {
	t.Helper()
	calls := e.editorCalls(t)
	want := []string{
		filepath.Join(e.root, "games", "color-shuffle"),
		filepath.Join(e.root, "games", "color-shuffle"),
		".",
		"-n notes.txt",
		"-u NONE f.txt",
	}
	if len(calls) != len(want) {
		t.Fatalf("editor calls = %q, want %q\noutput:\n%s", calls, want, out)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Errorf("editor call %d = %q, want %q", i, calls[i], want[i])
		}
	}
	if !strings.Contains(out, filepath.Join("web", "blog")) {
		t.Errorf("cdp did not change directory:\n%s", out)
	}
	// PowerShell's echo prints one argument per line, so compare words.
	words := strings.Join(strings.Fields(out), " ")
	if !strings.Contains(words, "ACTION extra-arg") {
		t.Errorf("action did not run with its extra argument:\n%s", out)
	}
	if !strings.Contains(words, "ACTION RUN_DONE") {
		t.Errorf("action received the project name as an argument:\n%s", out)
	}
	if strings.Contains(out, "evil") || strings.Contains(out, "\x1b[2J") {
		t.Errorf("project with a control character in its name was listed:\n%q", out)
	}
	if !strings.Contains(out, "My App") || !strings.Contains(out, "a;b") {
		t.Errorf("projects with spaces or ';' missing from list:\n%s", out)
	}
}

func TestZsh(t *testing.T) {
	zsh := requireShell(t, "zsh")
	e := newEnv(t)
	// err_exit: the integration must load cleanly in strict shells too.
	out := e.run(t, zsh, "-f", "-c", `setopt err_exit
autoload -Uz compinit && compinit -u -d "$HOME/.zcompdump"
`+posixPrelude+`eval "$(codehop init zsh --cmd code)"`+posixScript+`
_zsh_autosuggest_strategy_codehop "code colo"; echo "SUGGEST:$suggestion"
_zsh_autosuggest_strategy_codehop "ls colo"; echo "OTHER:[$suggestion]"`)
	checkPosixResults(t, e, out)
	if !strings.Contains(out, "SUGGEST:code color-shuffle") || !strings.Contains(out, "OTHER:[]") {
		t.Errorf("zsh-autosuggestions strategy wrong:\n%s", out)
	}
}

func TestBash(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash on Windows is covered by the Git Bash CI job")
	}
	bash := requireShell(t, "bash")
	e := newEnv(t)
	out := e.run(t, bash, "--norc", "--noprofile", "-c", `set -e
shopt -s expand_aliases
`+posixPrelude+`eval "$(codehop init bash --cmd code)"`+posixScript+`
__codehop_set_words() { COMP_WORDS=("$@"); COMP_CWORD=$(( $# - 1 )); }
__codehop_set_words code muri; __codehop_complete; echo "COMPLETE:${COMPREPLY[*]}"
__codehop_set_words code shuf; __codehop_complete; echo "FUZZY:${COMPREPLY[*]}"
__codehop_set_words code My; __codehop_complete; echo "SPACE:${COMPREPLY[*]}"
__codehop_set_words code 'a'; __codehop_complete; echo "SEMI:${COMPREPLY[*]}"`)
	checkPosixResults(t, e, out)
	if !strings.Contains(out, "COMPLETE:murid-admin murid-landing") {
		t.Errorf("prefix completion wrong:\n%s", out)
	}
	if !strings.Contains(out, "FUZZY:color-shuffle") {
		t.Errorf("fuzzy completion wrong:\n%s", out)
	}
	if !strings.Contains(out, `SPACE:My\ App`) || !strings.Contains(out, `SEMI:a\;b`) {
		t.Errorf("completion did not shell-quote names:\n%s", out)
	}
}

func TestFish(t *testing.T) {
	fish := requireShell(t, "fish")
	e := newEnv(t)
	out := e.run(t, fish, "--no-config", "-c", `alias code=false
codehop init fish --cmd code | source
code color-shuffle
code shuffle
code .
code -n notes.txt
code -u NONE f.txt
cdp blog; and pwd
run murid-admin extra-arg
run blog
echo RUN_DONE
codehop list --names
complete -C 'code muri'`)
	checkPosixResults(t, e, out)
	if !strings.Contains(out, "murid-admin") || !strings.Contains(out, "murid-landing") {
		t.Errorf("fish completion missing projects:\n%s", out)
	}
}

// TestPowerShell runs under PowerShell 7 (pwsh) and, on Windows, also under
// Windows PowerShell 5.1, whose native-argument handling differs.
func TestPowerShell(t *testing.T) {
	editions := []string{"pwsh"}
	if runtime.GOOS == "windows" {
		editions = append(editions, "powershell")
	}
	for _, edition := range editions {
		t.Run(edition, func(t *testing.T) {
			testPowerShell(t, requireShell(t, edition))
		})
	}
}

func testPowerShell(t *testing.T, ps string) {
	e := newEnv(t)
	out := e.run(t, ps, "-NoProfile", "-NonInteractive", "-Command", `
$ErrorActionPreference = 'Stop'
Set-Alias code Get-Date
Invoke-Expression (& { (codehop init powershell --cmd code | Out-String) })
code color-shuffle
code shuffle
code .
code -n notes.txt
code -u NONE f.txt
cdp blog; (Get-Location).Path
run murid-admin extra-arg
run blog
'RUN_DONE'
codehop list --names
function complete($line) {
    $c = TabExpansion2 -inputScript $line -cursorColumn $line.Length
    ($c.CompletionMatches | ForEach-Object CompletionText) -join ' '
}
"COMPLETE:" + (complete 'code muri')
"EMPTY:" + (complete 'code ')
"SPACE:" + (complete 'code My')
"SEMI:" + (complete 'code a')`)
	checkPosixResults(t, e, out)
	for _, want := range []string{"COMPLETE:murid-admin murid-landing", "SPACE:'My App'", "SEMI:'a;b'"} {
		if !strings.Contains(out, want) {
			t.Errorf("PowerShell completion: missing %q in\n%s", want, out)
		}
	}
	if !strings.Contains(out, "EMPTY:") || strings.Contains(out, "EMPTY:\n") {
		t.Errorf("PowerShell completion of an empty word returned nothing:\n%s", out)
	}
}
