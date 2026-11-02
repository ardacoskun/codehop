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
		"web/api/.git", "mobile/api/.git", // same name: "@api" is ambiguous
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
setopt nounset
`+posixPrelude+`eval "$(codehop init zsh --cmd code)"`+posixScript+`
_zsh_autosuggest_strategy_codehop "code colo"; echo "SUGGEST:$suggestion"
_zsh_autosuggest_strategy_codehop "mkdir colo"; echo "OTHER:[$suggestion]"
_zsh_autosuggest_strategy_codehop "ls colo"; echo "LISTED:$suggestion"`)
	checkPosixResults(t, e, out)
	if !strings.Contains(out, "SUGGEST:code color-shuffle") || !strings.Contains(out, "OTHER:[]") ||
		!strings.Contains(out, "LISTED:ls color-shuffle") {
		t.Errorf("zsh-autosuggestions strategy wrong:\n%s", out)
	}
}

func TestBash(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash on Windows is covered by the Git Bash CI job")
	}
	bash := requireShell(t, "bash")
	e := newEnv(t)
	out := e.run(t, bash, "--norc", "--noprofile", "-c", `set -euo pipefail
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

// expandChecks are the expansions every shell must agree on.
func expandChecks(t *testing.T, e env, out string) {
	t.Helper()
	shuffle := filepath.Join(e.root, "games", "color-shuffle")
	for _, want := range []string{
		"EXP1:mkdir " + shuffle + "/x",         // @project/path in any command
		"EXP2:cd " + shuffle,                   // bare name after cd
		"EXP3:npm i @color-shuffle",            // npm is never touched
		"EXP4:mkdir blog2 && cd color-shuffle", // bare names only in the first command
		"EXP5:echo '@color-shuffle'",           // quoted text is never touched
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	if !strings.Contains(out, "BLOCKED:yes") || !strings.Contains(out, "mobile/api") {
		t.Errorf("ambiguous @api did not block the line:\n%s", out)
	}
}

const expandLines = `mkdir @color-shuffle/x
cd color-shuffle
npm i @color-shuffle
mkdir blog2 && cd color-shuffle
echo '@color-shuffle'`

func TestExpandZsh(t *testing.T) {
	zsh := requireShell(t, "zsh")
	e := newEnv(t)
	e.vars = append(e.vars, "EXPAND_LINES="+expandLines)
	out := e.run(t, zsh, "-f", "-c", `setopt err_exit
autoload -Uz compinit && compinit -u -d "$HOME/.zcompdump"
setopt nounset
eval "$(codehop init zsh --cmd code)"
eval "$(codehop init zsh --cmd code)"   # a second load (source ~/.zshrc) must not chain to itself
print -r -- "PREV:[${__codehop_prev_first-}]"
# zle is only active at an interactive prompt; record what would be shown.
zle() { print -r -- "ZLE:$*"; }
CONTEXT=start PREBUFFER=   # what zle sets for a fresh command at the prompt
n=0
for line in "${(@f)$(print -r -- "$EXPAND_LINES")}"; do
  n=$((n + 1)); BUFFER=$line
  __codehop_expand_buffer
  print -r -- "EXP$n:$BUFFER"
done
BUFFER='ls @api/src'
if __codehop_expand_buffer; then print BLOCKED:no; else print -r -- "BLOCKED:yes"; fi
CONTEXT=cont BUFFER='cd color-shuffle'   # a continuation line, e.g. a heredoc body
__codehop_expand_buffer; print -r -- "CONT:$BUFFER"`)
	expandChecks(t, e, out)
	if !strings.Contains(out, "CONT:cd color-shuffle") {
		t.Errorf("continuation line was rewritten:\n%s", out)
	}
	if strings.Contains(out, "PREV:[__codehop") {
		t.Errorf("second load made the -first- completer call itself:\n%s", out)
	}
}

func TestExpandBash(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash on Windows is covered by the Git Bash CI job")
	}
	bash := requireShell(t, "bash")
	e := newEnv(t)
	e.vars = append(e.vars, "EXPAND_LINES="+expandLines)
	// -i: the Enter integration is only installed in interactive shells.
	out := e.run(t, bash, "--norc", "--noprofile", "-i", "-c", `
if (( BASH_VERSINFO[0] < 4 )); then echo SKIP-OLD-BASH; exit 0; fi
eval "$(codehop init bash --cmd code)" 2>/dev/null
eval "$(codehop init bash --cmd code)" 2>/dev/null   # a second load must not chain to itself
echo "PREVDEFAULT:[${__codehop_prev_default-}]"
n=0
while IFS= read -r line; do
  n=$((n + 1)); READLINE_LINE=$line
  __codehop_expand_readline 2>/dev/null
  printf '%s\n' "EXP$n:$READLINE_LINE"
done <<< "$EXPAND_LINES"
READLINE_LINE='ls @api/src'
__codehop_expand_readline 2>&1 | grep -q mobile/api && echo "BLOCKED:yes mobile/api"
COMP_LINE='ls @col' COMP_POINT=7 COMP_WORDS=(ls @col) COMP_CWORD=1 COMPREPLY=()
__codehop_complete_expand ls @col ls
echo "CEXP:${COMPREPLY[*]}"
COMP_LINE='mytool @col' COMP_POINT=11 COMP_WORDS=(mytool @col) COMP_CWORD=1 COMPREPLY=()
__codehop_complete_default mytool @col mytool
echo "CDEFAULT:${COMPREPLY[*]}"
COMP_LINE='curl -d @col' COMP_POINT=12 COMP_WORDS=(curl -d @col) COMP_CWORD=2 COMPREPLY=()
if __codehop_complete_expand curl @col -d; then echo "CSKIP:taken"; else echo "CSKIP:left"; fi`)
	if strings.Contains(out, "SKIP-OLD-BASH") {
		t.Skip("bash < 4 has no READLINE_LINE")
	}
	expandChecks(t, e, out)
	for _, want := range []string{"CEXP:@color-shuffle/", "CDEFAULT:@color-shuffle/", "CSKIP:left", "PREVDEFAULT:[]"} {
		if !strings.Contains(out, want) {
			t.Errorf("bash completion: missing %q in\n%s", want, out)
		}
	}
}

func TestExpandFish(t *testing.T) {
	fish := requireShell(t, "fish")
	e := newEnv(t)
	e.vars = append(e.vars, "EXPAND_LINES="+expandLines)
	// Abbreviations only run at an interactive prompt, so check that the
	// integration registers one and that the word rewrite it calls works.
	out := e.run(t, fish, "--no-config", "-c", `codehop init fish --cmd code | source
abbr --list | string match -q __codehop_expand; and echo ABBR:yes
printf 'mkdir @color-shuffle/x' | codehop expand-word --shell fish; echo
printf 'mkdir blog2; and cd color-shuffle' | codehop expand-word --shell fish; or echo SECOND:untouched
printf 'npm i @color-shuffle' | codehop expand-word --shell fish; or echo NPM:untouched
set -l n 0
for line in (string split \n -- $EXPAND_LINES)
    set n (math $n + 1)
    set -l parts (string split -m1 \n -- (__codehop_expand_reply $line))
    if test "$parts[1]" = OK; echo "EXP$n:$parts[2]"; else; echo "EXP$n:$line"; end
end
set -l parts (string split -m1 \n -- (__codehop_expand_reply 'ls @api/src'))
test "$parts[1]" = BLOCK; and echo "BLOCKED:yes $parts[2]"`)
	shuffle := filepath.Join(e.root, "games", "color-shuffle")
	for _, want := range []string{"ABBR:yes", shuffle + "/x", "SECOND:untouched", "NPM:untouched"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	// The Enter binding's whole-line rewrite.
	expandChecks(t, e, out)
}

func TestExpandPowerShell(t *testing.T) {
	editions := []string{"pwsh"}
	if runtime.GOOS == "windows" {
		editions = append(editions, "powershell")
	}
	for _, edition := range editions {
		t.Run(edition, func(t *testing.T) {
			ps := requireShell(t, edition)
			e := newEnv(t)
			e.vars = append(e.vars, "EXPAND_LINES="+expandLines)
			out := e.run(t, ps, "-NoProfile", "-NonInteractive", "-Command", `
$ErrorActionPreference = 'Stop'
Invoke-Expression (& { (codehop init powershell --cmd code | Out-String) })
$n = 0
foreach ($line in ($env:EXPAND_LINES -split "`+"`"+`n")) {
    $n++
    $r = __codehop_expand_line $line
    $text = if ($r -and $r[0] -eq 'OK') { $r[1] } else { $line }
    "EXP${n}:$text"
}
$r = __codehop_expand_line 'ls @api/src'
if ($r -and $r[0] -eq 'BLOCK') { "BLOCKED:yes " + $r[1] }
function complete($line) {
    $c = TabExpansion2 -inputScript $line -cursorColumn $line.Length
    ($c.CompletionMatches | ForEach-Object CompletionText) -join ' '
}
"CEXP:" + (complete 'ls @col')
"CBARE:" + (complete 'cd color')
"CNONE:" + (complete 'Get-ChildItem -Pa')
"CSKIP:" + (complete 'curl -d @col')
Invoke-Expression (& { (codehop init powershell --cmd code | Out-String) })
"CRELOAD:" + (complete 'ls @col')`)
			// PowerShell writes Windows paths with backslashes and never uses ~.
			shuffle := filepath.Join(e.root, "games", "color-shuffle")
			out = strings.ReplaceAll(out, shuffle+`\x`, shuffle+"/x")
			expandChecks(t, e, out)
			if strings.Contains(out, "CSKIP:@color-shuffle") {
				t.Errorf("PowerShell took over completion for a skipped tool:\n%s", out)
			}
			for _, want := range []string{"CEXP:@color-shuffle/", "CBARE:color-shuffle/", "CNONE:-Path", "CRELOAD:@color-shuffle/"} {
				if !strings.Contains(out, want) {
					t.Errorf("PowerShell completion: missing %q in\n%s", want, out)
				}
			}
		})
	}
}
