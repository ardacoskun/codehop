<div align="center">

# codehop

**Open any project by name, from anywhere in your terminal.**

`code color<Tab>` → `code color-shuffle` → your editor opens it.

[![CI](https://github.com/ardacoskun/codehop/actions/workflows/ci.yml/badge.svg)](https://github.com/ardacoskun/codehop/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/ardacoskun/codehop?sort=semver)](https://github.com/ardacoskun/codehop/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/ardacoskun/codehop)](https://goreportcard.com/report/github.com/ardacoskun/codehop)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

<img src="assets/demo.gif" alt="codehop demo: tab-completing project names, fuzzy opening and the fzf picker" width="760">

</div>

codehop finds the projects on your disk (any folder with `.git`, `package.json`,
`go.mod`, `Cargo.toml`, …) and lets you open them by name in VS Code, Cursor, Zed,
JetBrains IDEs or any other editor. It works in zsh, bash, fish and PowerShell on
macOS, Linux and Windows, and it is a single ~5 MB binary that answers in a few
milliseconds.

- **Tab completion from disk.** Every project shows up, not just the folders you visited.
- **Fuzzy matching.** `code shuffle`, `code cs` (initials) and `code clrshf` all find `color-shuffle`.
- **Frecency.** The projects you open most and most recently come first.
- **Picker.** Plain `code` opens an [fzf](https://github.com/junegunn/fzf) picker with git status and the README in the preview.
- **Your editor keeps working.** `code .`, `code file.ts` and `code -n` behave exactly as before.
- **Projects in every command.** `mkdir @blog/assets`, `cp logo.png @blog/public/` and `cd blog` work anywhere: project names become paths when you press Enter.
- **More than opening.** Run `cdp <project>` to jump there, add your own actions (`cc <project>` → `claude`), check `git` state across every project at once, and clone with `code gh:owner/repo`.

## Getting started

```sh
brew install ardacoskun/tap/codehop      # or see Installation below
echo 'eval "$(codehop init zsh)"' >> ~/.zshrc
exec zsh

code <Tab>             # list your projects
code color-shuffle     # open one
code                   # pick interactively
cd color-shuffle       # jump there from anywhere
ls @color-shuffle/src  # use a project path in any command
```

Run `codehop doctor` if nothing shows up. It prints where codehop is looking and which editor it found.

## Installation

| Platform | Command |
|---|---|
| macOS (Homebrew) | `brew install ardacoskun/tap/codehop` |
| Windows (Scoop) | `scoop bucket add ardacoskun https://github.com/ardacoskun/scoop-bucket` then `scoop install codehop` |
| macOS / Linux (script) | `curl -fsSL https://raw.githubusercontent.com/ardacoskun/codehop/main/scripts/install.sh \| sh` |
| Windows (script) | `irm https://raw.githubusercontent.com/ardacoskun/codehop/main/scripts/install.ps1 \| iex` |
| Go 1.26+ | `go install github.com/ardacoskun/codehop/cmd/codehop@latest` |
| Manual | Download an archive from [Releases](https://github.com/ardacoskun/codehop/releases) and put `codehop` on your `PATH` |

[fzf](https://github.com/junegunn/fzf) is optional but recommended for the picker. Without it, codehop shows a numbered list instead.

### Shell setup

Add one line to your shell's startup file:

| Shell | File | Line |
|---|---|---|
| zsh | `~/.zshrc` (after `compinit`) | `eval "$(codehop init zsh)"` |
| bash | `~/.bashrc` | `eval "$(codehop init bash)"` |
| fish | `~/.config/fish/config.fish` | `codehop init fish \| source` |
| PowerShell | `$PROFILE` | `Invoke-Expression (& { (codehop init powershell \| Out-String) })` |

Git Bash on Windows uses the bash line.

On Windows PowerShell 5.1 with codehop 0.2.0 or earlier, if loading the profile fails with an error about `-Chord`, update PSReadLine and reopen the terminal:

```powershell
Install-Module PSReadLine -Scope CurrentUser -Force -SkipPublisherCheck
```

`init` defines these commands:

| Command | What it does |
|---|---|
| `code [project]` | Opens a project. With no argument it opens the picker. The name follows your editor: `cursor`, `zed`, … |
| `cdp [project]` | `cd`s into a project. |
| `<action> [project] [args]` | Defined by each entry in `[actions]` (see [Actions](#actions)). |

Use `--cmd <name>` to wrap a different command, `--no-cmd` to leave your editor
command untouched, and `--cd-cmd <name>` to rename `cdp` (`--cd-cmd ""` skips it).

## Usage

```sh
code design-system      # exact name
code desi               # prefix
code system             # a word inside the name
code ds                 # initials of the words
code web/blog           # category/name, when names repeat
code cs                 # several matches: the picker opens with them
code                    # pick from every project

code .                  # unchanged: open the current folder
code -n notes.md        # unchanged: flags and multiple arguments go straight to the editor
code ./new-file.md      # unchanged: paths that start with ./ ../ ~/ or / are files

cdp color<Tab>          # jump into a project
codehop status          # git state of every project
code gh:owner/repo      # clone into your first root if missing, then open
```

### How a name is resolved

`code <word>` checks the following in order:

1. Is `<word>` an existing file or folder? Then it goes to your editor unchanged.
2. Is `<word>` a remote (`gh:owner/repo`, `gl:…`, `bb:…`, `cb:…`, a git URL)? Then codehop clones it if it is missing and opens it.
3. Otherwise codehop ranks the projects:

| Tier | Example query → project |
|---|---|
| Exact name or `category/name` | `website` → `website` (not `website-v2`) |
| Prefix | `desi` → `design-system` |
| Word or initials | `shuffle`, `cs` → `color-shuffle` |
| Substring of the path | `ffle` → `color-shuffle` |
| Fuzzy (letters in order) | `clrshf` → `color-shuffle` |

One match in the best tier opens directly. A tie, or a fuzzy-only match, opens the
picker, so a typo never opens the wrong project. Within a tier, frecency decides the
order. If nothing matches, codehop says so and opens the picker with your query.

### Tab completion

As you type, a greyed-out suggestion completes the name: `code col` shows
`code color-shuffle`, and → accepts it. In zsh this needs
[zsh-autosuggestions](https://github.com/zsh-users/zsh-autosuggestions) (codehop
adds itself as its first strategy). fish does it on its own.

Completion offers prefix matches first. When nothing starts with what you typed,
it offers fuzzy matches and replaces the word: `code shuf<Tab>` becomes
`code color-shuffle`. In zsh it works with
[fzf-tab](https://github.com/Aloxaf/fzf-tab) and shows each project's category
next to its name.

On Windows, PowerShell's Tab cycles through completions in place instead of
listing them. Press Ctrl+Space to see the list, or add
`Set-PSReadLineKeyHandler -Key Tab -Function MenuComplete` to `$PROFILE` to make
Tab show it. PowerShell's greyed-out suggestions come from your history, not from
codehop, and need PSReadLine 2.1+ (built into PowerShell 7; Windows PowerShell 5.1
ships 2.0).

### Project paths in any command

Write `@` in front of a project name and it becomes that project's path in
any command. Commands such as `cd` and `ls` also take a plain project name:

```sh
mkdir @color-shuffle/assets/new   # → mkdir ~/projects/games/color-shuffle/assets/new
cp logo.png @blog/public/         # → cp logo.png ~/projects/web/blog/public/
git -C @design-system status
cd color-shuffle                  # → cd ~/projects/games/color-shuffle
ls blog                           # → ls ~/projects/web/blog
```

The rewrite happens when you press Enter, so the line on screen and in your
history is the real command. Tab completes `@col` to `@color-shuffle/` and
then the files inside, and a grey suggestion shows the name as you type
(zsh with zsh-autosuggestions).

codehop leaves a line alone rather than guess:

- **Exact names only on Enter.** `@colo` is completed with Tab, but it is not
  expanded on Enter.
- **Local files win.** `cd blog` goes to `./blog` if it exists, even if its
  name is spelled `Blog` or `blog_` differently from the project, and if
  `$CDPATH` finds it.
- **Plain names only work when the line is that single command.** In a
  chain such as `cd docs && rm -rf *`, a missing `./docs` must still stop the
  line, so codehop leaves it alone. Use `@docs` when you mean the project.
- **Some commands are never touched.** By default these are npm, pnpm, yarn,
  bun, deno, expo, nx and the other JavaScript tools, node, curl and HTTPie.
  In those tools `@` means something else, as in `npm i @types/node` and
  `curl -d @file.json`. The rule holds anywhere on the line, so
  `xargs npm i @types/node` is safe too.
- **Option values stay values.** `open -a Notes`, `ls -I build` and
  `Get-ChildItem -Filter blog` are left alone.
- **Some text is never touched.** This covers quoted text, `$(...)`, subshells,
  heredocs, continuation lines and variables. A project whose path contains
  quotes, `$` or backticks is never written into a command. In PowerShell it also covers `@name` when `$name` is
  a variable, since that is a splat.
- **Ambiguous names stop the line.** If a name matches two projects (`web/api`
  and `mobile/api`), the line is not run and both are shown.

| Shell | Tab | Expanded |
|---|---|---|
| zsh | ✓ | on Enter |
| bash 4+, Git Bash | ✓ | on Enter |
| fish 3.6+ | — | when the word is finished (space or Enter) |
| PowerShell (PSReadLine) | ✓ | on Enter |

Change the marker, the command list or the skip list under `[expand]` in the
config. Set `enabled = false` to turn the feature off.

### Actions

Actions are your own commands that jump into a project and run something:

```toml
[actions]
cc = "claude"        # cc color-shuffle            → cd there, run claude
t  = "npm test"      # t blog --watch              → cd there, run npm test --watch
lg = "lazygit"
```

Extra arguments after the project name go to the command. The command is
inserted into your shell as written, so use syntax your shell understands.

### Git status across projects

```console
$ codehop status --dirty
PROJECT        BRANCH        CHANGES                 SYNC         LAST COMMIT
api-server     main          1 changed               up to date   9d ago
design-system  feat/tokens   3 changed, 1 untracked  ↑2           2h ago
website-v2     redesign      1 untracked             ↑4 ↓1        3w ago
```

`codehop status [filter]` shows every project. `--dirty` keeps only those with
uncommitted, unpushed or unpulled work, and `--json` is for scripts.

## Configuration

codehop works without a config file. `codehop config init` writes a commented
one, and `codehop config edit` opens it.

| OS | Config file |
|---|---|
| macOS, Linux | `$XDG_CONFIG_HOME/codehop/config.toml` (default `~/.config/codehop/config.toml`) |
| Windows | `%APPDATA%\codehop\config.toml` |

```toml
# Folders searched for projects. Default: whichever of ~/projects, ~/code, ~/src,
# ~/dev, ~/Developer, ~/repos, ~/workspace, ~/git exist.
roots = ["~/projects", "~/work"]

max_depth = 3                  # levels below a root; a project's own subfolders are never searched
markers = [".git", "package.json", "go.mod", "Cargo.toml", "*.sln"]   # globs allowed
ignore = ["node_modules", "vendor", "dist", "build", "target"]
editor = "code"                # preset or any command; {path} places the path
prefer_workspace = true        # open a project's single *.code-workspace if it stays inside the project
clone_root = "~/projects"      # where gh:owner/repo goes; default: the first root
fzf_opts = "--height=60%"

[actions]
cc = "claude"

[expand]
prefix = "@"                   # @ , + or : — "" turns @-paths off
commands = ["cd", "ls", "open", "tree"]          # take a plain project name
skip = ["npm", "pnpm", "yarn", "bun", "curl"]    # never rewritten
```

Hidden folders are skipped. To make any folder a project, drop an empty `.codehop` file into it.

### Editors

`editor` accepts a preset name or a full command:

| Preset | Runs | Notes |
|---|---|---|
| `code` (default), `code-insiders`, `cursor`, `windsurf`, `codium` | the editor's CLI | opens `*.code-workspace` files |
| `zed`, `subl` | the editor's CLI | |
| `idea`, `webstorm`, `goland`, `pycharm` | JetBrains launcher scripts | enable them in JetBrains Toolbox |
| `nvim`, `vim`, `hx` | in the terminal | |
| anything else | e.g. `"code --profile Work"` or `'"C:\Tools\My Editor\edit.exe" --folder {path}'` | |

### Environment variables

| Variable | Overrides |
|---|---|
| `CODEHOP_CONFIG` | config file path |
| `CODEHOP_ROOTS` | `roots` (separated by `:`, or by `;` on Windows) |
| `CODEHOP_EDITOR` | `editor` |
| `CODEHOP_FZF_OPTS` | `fzf_opts` |
| `CODEHOP_DATA_DIR` | frecency database folder (default `~/.local/share/codehop`, `%LOCALAPPDATA%\codehop`) |
| `CODEHOP_CACHE_DIR` | project cache folder (default `~/.cache/codehop`, `%LOCALAPPDATA%\codehop\cache`) |
| `NO_COLOR` | disables colors |

## Commands

| Command | Description |
|---|---|
| `codehop open [project]` | Open a project. It never falls back to opening a file. |
| `codehop query [project]` | Print a project's path (`--record` counts it as a visit). |
| `codehop list` | List projects, most frecent first (`--names`, `--paths`, `--json`, `--query`). |
| `codehop status [filter]` | Git state of every project (`--dirty`, `--json`). |
| `codehop init <shell>` | Print the shell integration. |
| `codehop doctor` | Check the config, roots, editor, fzf and git. |
| `codehop refresh` | Rescan now. Rarely needed, because the cache notices changes by itself. |
| `codehop config path\|init\|edit` | Manage the config file. |
| `codehop completion <shell>` | Completion for `codehop`'s own subcommands. |

## How it compares

| | codehop | [zoxide](https://github.com/ajeetdsouza/zoxide) | oh-my-zsh [`pj`](https://github.com/ohmyzsh/ohmyzsh/tree/master/plugins/pj) | [ghq](https://github.com/x-motemen/ghq) |
|---|---|---|---|---|
| Knows projects you never visited | ✓ (scans disk) | ✗ (learns from `cd`) | ✓ (one level) | only ones it cloned |
| Opens in your editor by name | ✓ | via glue scripts | ✓ (`pjo`) | via glue scripts |
| Fuzzy matching + frecency | ✓ | ✓ | ✗ | ✗ |
| zsh, bash, fish, PowerShell | ✓ | ✓ | zsh only | ✓ |

codehop and zoxide work well together: zoxide for any directory, codehop for projects.

## FAQ

**Will it break `code .` or opening files?** No. Existing paths, flags and multiple
arguments go straight to the real editor. Only a single word that isn't a path is
treated as a project name.

**I have a file and a project with the same name.** The file wins in the current
folder. Use `codehop open <name>` to force the project.

**How do I create a new file called `notes.md`?** Write it as a path: `code ./notes.md`.
A bare word that matches nothing opens the picker instead of creating an empty
file, which protects you from typos.

**Is it fast?** Completion reads a small cache that is checked against folder
modification times. A run takes about 5 ms, and new or deleted projects appear
immediately.

**Windows says it refuses to pass an argument to `code.cmd`.** See
[Security](#security): rename the folder, or point `editor` at `Code.exe`.

**Two projects have the same folder name.** Both get a longer, unique name such as
`web/api` and `mobile/api`.

**How do I uninstall?** Remove the `init` line from your shell profile, uninstall
the binary, and delete the config, data and cache folders listed by `codehop doctor`.

## Security

codehop is built to be safe to run on folders you did not create:

- **Nothing from a project is executed.** Git is run with overrides
  (`core.fsmonitor=false`, the repository's own `filter.*` drivers disabled,
  `log.showSignature=false`), so a copied repository cannot run code when it is
  previewed or listed by `codehop status`.
- **Nothing untrusted reaches your terminal raw.** Folders whose names contain
  control characters are ignored. README lines, commit subjects and git errors
  are stripped of escape sequences. The picker preview only reads a README that
  is a regular file inside the project.
- **Shell completion quotes every name**, so a folder named `a;rm -rf ~` is
  completed as one harmless argument.
- **Workspace files are checked before opening.** A project's
  `*.code-workspace` is only opened if every folder it lists is inside the
  project. Otherwise codehop opens the folder and says why.
- **An unreachable network drive cannot freeze your shell.** Tab waits at most
  0.4 s and `code`/`cdp` at most 3 s, then fall back to the last known project
  list. `codehop doctor` shows which root is not responding.
- **Command lines are only rewritten when the meaning is clear.** A command
  line is changed only for exact project names, never inside quotes or
  substitutions, never for skipped commands, and always visibly before it runs.
- **Clones stay inside your clone root.** If a folder with that name already
  exists, it is only opened when it is a clone of the same repository.
- **On Windows**, editors that are `.cmd`/`.bat` scripts (VS Code's `code` is
  `code.cmd`) run through `cmd.exe`. codehop refuses to pass them arguments
  containing `& | < > ^ % ! "`. Rename the folder, or set `editor` to the
  editor's `.exe`.
- **`[actions]` and `editor` come from your own config** and run as written,
  like the rest of your shell profile. codehop never reads configuration from
  inside a project.

Release archives come with a `checksums.txt` and GitHub build provenance. The
install scripts verify the checksum. To check provenance, run
`gh attestation verify <file> --repo ardacoskun/codehop`. The Homebrew cask
removes the macOS quarantine flag because the binary is not notarized yet.

Report vulnerabilities privately; see [SECURITY.md](SECURITY.md).

## Contributing

Issues and pull requests are welcome. See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

[MIT](LICENSE)
