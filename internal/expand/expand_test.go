package expand

import (
	"errors"
	"strings"
	"testing"

	"github.com/ardacoskun/codehop/internal/discover"
)

type fakeResolver map[string][]discover.Project

func (f fakeResolver) Exact(name string) []discover.Project { return f[strings.ToLower(name)] }

func proj(key, path string) discover.Project {
	return discover.Project{Key: key, Name: key, Path: path}
}

var resolver = fakeResolver{
	"blog":          {proj("blog", "/home/me/projects/web/blog")},
	"color-shuffle": {proj("color-shuffle", "/home/me/projects/games/color-shuffle")},
	"types":         {proj("types", "/home/me/projects/lib/types")},
	"data.json":     {proj("data.json", "/home/me/projects/x/data.json")},
	"my app":        {proj("my app", "/home/me/projects/My App")},
	"spaced":        {proj("spaced", "/home/me/projects/My App")},
	"api":           {proj("web/api", "/home/me/projects/web/api"), proj("mobile/api", "/home/me/projects/mobile/api")},
	"out":           {proj("out", "/home/me/projects/out")},
	"2":             {proj("2", "/home/me/projects/2")},
	"splat":         {proj("splat", `C:\Users\me\projects\splat`)},
	"win":           {proj("win", `C:\Users\me\projects\win`)},
	"danger":        {proj("danger", "/home/me/projects/x$(touch pwned)")},
	"comma":         {proj("comma", `C:\p\a,b`)},
	"web/api":       {proj("web/api", "/home/me/projects/web/api")},
	"notes":         {proj("notes", "/home/me/projects/notes")},
	"my_app":        {proj("my_app", "/home/me/projects/my_app")},
	"lib":           {proj("lib", "/home/me/projects/lib")},
	"node":          {proj("node", "/home/me/projects/node")},
	"rel":           {proj("rel", "/home/me/projects/rel")},
}

func posix() Options {
	return Options{
		Dialect:  Posix,
		Prefix:   "@",
		Commands: []string{"cd", "ls", "open", "Set-Location"},
		Skip:     []string{"npm", "pnpm", "yarn", "curl"},
		Cwd:      "/work",
		Home:     "/home/me",
		Exists:   func(p string) bool { return p == "/work/blog" || p == "/work/@local" },
	}
}

func TestLine(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		// What it is for.
		{"prefixed path in any command", "mkdir @color-shuffle/assets/new", "mkdir ~/projects/games/color-shuffle/assets/new"},
		{"prefixed project alone", "git -C @color-shuffle status", "git -C ~/projects/games/color-shuffle status"},
		{"several prefixed words", "cp @blog/a.png @color-shuffle/", "cp ~/projects/web/blog/a.png ~/projects/games/color-shuffle/"},
		{"bare name after cd", "cd color-shuffle", "cd ~/projects/games/color-shuffle"},
		{"bare name with subpath", "ls color-shuffle/assets", "ls ~/projects/games/color-shuffle/assets"},
		{"case-insensitive command", "CD color-shuffle", "CD ~/projects/games/color-shuffle"},
		{"after sudo and env assignment", "FOO=1 sudo ls color-shuffle", "FOO=1 sudo ls ~/projects/games/color-shuffle"},
		{"prefixed names in every command of a chain", "cd @color-shuffle && ls @blog | wc -l", "cd ~/projects/games/color-shuffle && ls ~/projects/web/blog | wc -l"},
		{"bare name in a chain keeps the shell's own failure", "cd color-shuffle && rm -rf *", "cd color-shuffle && rm -rf *"},
		{"bare name before a pipe", "ls color-shuffle | wc -l", "ls color-shuffle | wc -l"},
		{"bare name only in the first command", "mkdir color-shuffle && cd color-shuffle", "mkdir color-shuffle && cd color-shuffle"},
		{"prefixed name in a later command", "mkdir x && cp @blog/a x", "mkdir x && cp ~/projects/web/blog/a x"},
		{"prefixed redirect target", "echo hi > @blog/notes.txt", "echo hi > ~/projects/web/blog/notes.txt"},
		{"path needing quotes", "ls @spaced/docs", "ls '/home/me/projects/My App/docs'"},
		{"command given by path", "/bin/ls color-shuffle", "/bin/ls ~/projects/games/color-shuffle"},

		// What it must never touch.
		{"npm scoped package", "npm i @types/node", "npm i @types/node"},
		{"pnpm scoped package", "pnpm add -D @types/node", "pnpm add -D @types/node"},
		{"npm via full path", "/usr/local/bin/npm i @types/node", "/usr/local/bin/npm i @types/node"},
		{"curl file upload", "curl -d @data.json https://x", "curl -d @data.json https://x"},
		{"single quotes", "echo '@blog'", "echo '@blog'"},
		{"double quotes", `ls "@blog"`, `ls "@blog"`},
		{"escaped", `ls \@blog`, `ls \@blog`},
		{"variable", "ls $HOME/@blog", "ls $HOME/@blog"},
		{"command substitution", "echo $(ls @blog)", "echo $(ls @blog)"},
		{"subshell", "(cd @blog && ls)", "(cd @blog && ls)"},
		{"backticks", "echo `ls @blog`", "echo `ls @blog`"},
		{"heredoc", "cat <<EOF > @blog/x", "cat <<EOF > @blog/x"},
		{"substitution inside double quotes", `echo "$(echo "a @blog b")"`, `echo "$(echo "a @blog b")"`},
		{"ansi-c string", `echo $'a\' @blog \'b'`, `echo $'a\' @blog \'b'`},
		{"parameter expansion", "echo ${X} @blog", "echo ${X} @blog"},
		{"project path that is unsafe to write", "ls @danger", "ls @danger"},
		{"unbalanced quote", `echo "@blog`, `echo "@blog`},
		{"comment", "ls # @blog", "ls # @blog"},
		{"unknown project", "mkdir @nothing/x", "mkdir @nothing/x"},
		{"bare name after non-listed command", "mkdir blog2 color-shuffle", "mkdir blog2 color-shuffle"},
		{"local folder wins over project", "cd blog", "cd blog"},
		{"literal @file that exists", "cat @local", "cat @local"},
		{"prefix alone", "git checkout @", "git checkout @"},
		{"git reflog syntax", "git show @{1}", "git show @{1}"},
		{"email-like word", "echo me@blog", "echo me@blog"},
		{"flag", "ls -la", "ls -la"},
		{"dot path", "cd ./color-shuffle", "cd ./color-shuffle"},
		{"home path", "cd ~/color-shuffle", "cd ~/color-shuffle"},
		{"redirect target is a file, not a project", "ls color-shuffle > out", "ls ~/projects/games/color-shuffle > out"},
		{"fd number before redirect", "ls 2>/dev/null", "ls 2>/dev/null"},
		{"prefixed word as the command", "@blog foo", "@blog foo"},
		{"sudo with flags is not guessed", "sudo -u root ls @blog", "sudo -u root ls @blog"},
		{"glob", "ls @blog*", "ls @blog*"},
		{"url", "open https://blog", "open https://blog"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Line(tt.in, posix(), resolver)
			if err != nil {
				t.Fatalf("Line(%q) error: %v", tt.in, err)
			}
			if got.Line != tt.want {
				t.Errorf("Line(%q)\n got %q\nwant %q", tt.in, got.Line, tt.want)
			}
			if got.Changed != (tt.in != tt.want) {
				t.Errorf("Changed = %v", got.Changed)
			}
		})
	}
}

func TestLineAmbiguous(t *testing.T) {
	_, err := Line("ls @api/src", posix(), resolver)
	var amb *AmbiguousError
	if !errors.As(err, &amb) || !strings.Contains(amb.Error(), "web/api") || !strings.Contains(amb.Error(), "mobile/api") {
		t.Fatalf("err = %v, want AmbiguousError listing both keys", err)
	}
	// A bare ambiguous name is left for the command to handle.
	got, err := Line("cd api", posix(), resolver)
	if err != nil || got.Changed {
		t.Fatalf("bare ambiguous name: %+v, %v", got, err)
	}
}

func TestLineRecordsProjects(t *testing.T) {
	got, err := Line("cp @blog/a @color-shuffle/b", posix(), resolver)
	if err != nil || len(got.Projects) != 2 || got.Projects[0].Key != "blog" {
		t.Fatalf("Projects = %+v, %v", got.Projects, err)
	}
}

func TestLineDisabledPrefix(t *testing.T) {
	opts := posix()
	opts.Prefix = ""
	got, _ := Line("ls @blog", opts, resolver)
	if got.Changed {
		t.Fatalf("prefix disabled but line changed: %q", got.Line)
	}
	got, _ = Line("cd color-shuffle", opts, resolver)
	if !got.Changed {
		t.Fatal("bare commands should still work with the prefix disabled")
	}
}

func TestLineOtherPrefix(t *testing.T) {
	opts := posix()
	opts.Prefix = ","
	got, _ := Line("ls ,blog @blog", opts, resolver)
	if got.Line != "ls ~/projects/web/blog @blog" {
		t.Fatalf("got %q", got.Line)
	}
}

func TestLineFish(t *testing.T) {
	opts := posix()
	opts.Dialect = Fish
	tests := map[string]string{
		"mkdir @blog/x":                     "mkdir ~/projects/web/blog/x",
		"cd @color-shuffle; and ls @blog":   "cd ~/projects/games/color-shuffle; and ls ~/projects/web/blog",
		"cd color-shuffle":                  "cd ~/projects/games/color-shuffle",
		"cd color-shuffle; and rm -r x":     "cd color-shuffle; and rm -r x",
		"mkdir blog2; and cd color-shuffle": "mkdir blog2; and cd color-shuffle",
		"echo (ls @blog)":                   "echo (ls @blog)",
		`echo '@blog\'s'`:                   `echo '@blog\'s'`,
		"ls @spaced":                        `ls '/home/me/projects/My App'`,
	}
	for in, want := range tests {
		got, err := Line(in, opts, resolver)
		if err != nil || got.Line != want {
			t.Errorf("fish Line(%q) = %q, %v; want %q", in, got.Line, err, want)
		}
	}
}

func TestLinePowerShell(t *testing.T) {
	opts := Options{
		Dialect:  PowerShell,
		Prefix:   "@",
		Commands: []string{"cd", "Set-Location", "ls"},
		Skip:     []string{"npm"},
		Cwd:      `C:\work`,
		Keep:     []string{"splat"},
		Exists:   func(string) bool { return false },
	}
	tests := map[string]string{
		`code @win\src`:                    `code C:\Users\me\projects\win\src`,
		`Set-Location win`:                 `Set-Location C:\Users\me\projects\win`,
		`ls @spaced`:                       `ls '/home/me/projects/My App'`,
		`Invoke-Thing @splat`:              `Invoke-Thing @splat`,
		`ls | % { $_.Name } ; ls @win`:     `ls | % { $_.Name } ; ls @win`,
		"echo `@win":                       "echo `@win",
		`echo 'it''s @win'`:                `echo 'it''s @win'`,
		`npm i @types/node`:                `npm i @types/node`,
		"echo \u2018hi @win/x there\u2019": "echo \u2018hi @win/x there\u2019",
		"echo \u201chi @win/x\u201d":       "echo \u201chi @win/x\u201d",
		`echo @"`:                          `echo @"`,
		`ls @comma`:                        `ls 'C:\p\a,b'`,
	}
	for in, want := range tests {
		got, err := Line(in, opts, resolver)
		if err != nil || got.Line != want {
			t.Errorf("powershell Line(%q) = %q, %v; want %q", in, got.Line, err, want)
		}
	}
}

func TestRender(t *testing.T) {
	tests := []struct {
		d          Dialect
		path, want string
		ok         bool
	}{
		{Posix, "/p/My App", "'/p/My App'", true},
		{Posix, "/p/a^b", "'/p/a^b'", true},
		{Posix, "/p/çalışma", "/p/çalışma", true},
		{Fish, "/p/x y", "'/p/x y'", true},
		{PowerShell, `C:\Users\John Smith\p`, `'C:\Users\John Smith\p'`, true},
		{PowerShell, `C:\p\a,b`, `'C:\p\a,b'`, true},
		// Never written: these could close a surrounding quote or run code.
		{Posix, "/p/it's", "", false},
		{Posix, "/p/$x", "", false},
		{Posix, "/p/a`b", "", false},
		{Posix, `/p/a\b`, "", false},
		{Posix, `/p/a"b`, "", false},
		{Fish, `/p/a\b`, "", false},
		{PowerShell, `C:\p\it's`, "", false},
		{PowerShell, "C:\\p\\x\u2019y", "", false},
		{PowerShell, `C:\p\$x`, "", false},
	}
	for _, tt := range tests {
		got, ok := render(tt.path, "", Options{Dialect: tt.d})
		if got != tt.want || ok != tt.ok {
			t.Errorf("render(%q, %d) = %q, %v; want %q, %v", tt.path, tt.d, got, ok, tt.want, tt.ok)
		}
	}
}

func TestLineReviewFindings(t *testing.T) {
	opts := posix()
	opts.Commands = append(opts.Commands, "tree")
	opts.LocalNames = func() []string { return []string{"my-app", "blog"} }
	opts.CDPath = []string{"/cdpath", "sub"}
	opts.Skip = append(opts.Skip, "node", "http", "tsx")
	opts.Exists = func(p string) bool {
		return p == "/work/blog" || p == "/cdpath/lib" || p == "/work/sub/rel"
	}
	tests := []struct{ name, in, want string }{
		{"longer key with a slash", "ls @web/api/src", "ls ~/projects/web/api/src"},
		{"bare longer key", "cd web/api", "cd ~/projects/web/api"},
		{"skipped tool behind xargs", "xargs npm i @types/node", "xargs npm i @types/node"},
		{"skipped tool over ssh", "ssh host npm i @blog", "ssh host npm i @blog"},
		{"value of open -a", "open -a notes", "open -a notes"},
		{"value of ls -I", "ls -I notes", "ls -I notes"},
		{"value of tree -I", "tree -I notes", "tree -I notes"},
		{"boolean flag before a name", "ls -la notes", "ls -la ~/projects/notes"},
		{"local name in another spelling", "ls my_app", "ls my_app"},
		{"local folder with a missing subpath", "cd blog/docs", "cd blog/docs"},
		{"reachable through CDPATH", "cd lib", "cd lib"},
		{"CDPATH only matters to cd", "ls lib", "ls ~/projects/lib"},
		{"noclobber redirect", "echo hi >| @blog/n", "echo hi >| ~/projects/web/blog/n"},
		{"cd to a project named like a skipped tool", "cd node", "cd ~/projects/node"},
		{"path ending like a skipped tool", "ls @blog/http", "ls ~/projects/web/blog/http"},
		{"skipped tool given by path behind a wrapper", "xargs /usr/local/bin/npm i @types/node", "xargs /usr/local/bin/npm i @types/node"},
		{"skipped tool from node_modules", "time ./node_modules/.bin/tsx @blog", "time ./node_modules/.bin/tsx @blog"},
		{"value flag in a cluster", "ls -lI notes", "ls -lI notes"},
		{"relative CDPATH entry", "cd rel", "cd rel"},
	}
	for _, tt := range tests {
		got, err := Line(tt.in, opts, resolver)
		if err != nil || got.Line != tt.want {
			t.Errorf("%s: Line(%q) = %q, %v; want %q", tt.name, tt.in, got.Line, err, tt.want)
		}
	}
}

func TestLinePowerShellReviewFindings(t *testing.T) {
	opts := Options{
		Dialect:    PowerShell,
		Prefix:     "@",
		Commands:   []string{"gci", "cd"},
		Exists:     func(string) bool { return false },
		LocalNames: func() []string { return nil },
	}
	tests := map[string]string{
		"gci -Filter notes": "gci -Filter notes",
		"gci -filter notes": "gci -filter notes",
		"cmd --% @win/x":    "cmd --% @win/x",
		"gci notes":         "gci /home/me/projects/notes",
	}
	for in, want := range tests {
		got, err := Line(in, opts, resolver)
		if err != nil || got.Line != want {
			t.Errorf("powershell Line(%q) = %q, %v; want %q", in, got.Line, err, want)
		}
	}
}

func TestAmbiguousMessageExplainsWayOut(t *testing.T) {
	_, err := Line("ls @api", posix(), resolver)
	if err == nil || !strings.Contains(err.Error(), "quote") || !strings.Contains(err.Error(), "longer name") {
		t.Fatalf("message = %v", err)
	}
}
