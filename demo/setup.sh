#!/usr/bin/env bash
# Builds a throwaway demo world for demo/demo.tape: sample projects with git
# history, a config whose "editor" just prints what would open, and a minimal
# zsh setup. Usage: demo/setup.sh <dir>
set -euo pipefail
dir="$1"
rm -rf "$dir"
mkdir -p "$dir/bin" "$dir/home/projects"

go build -o "$dir/bin/codehop" ./cmd/codehop
cat >"$dir/bin/open-in-editor" <<'SH'
#!/usr/bin/env bash
printf '\033[32m✓\033[0m opened \033[1m%s\033[0m in VS Code\n' "${1/#$HOME/~}"
SH
chmod +x "$dir/bin/open-in-editor"
# Shadow the real editor CLI so a misconfigured recording can never open it.
ln -sf open-in-editor "$dir/bin/code"

export HOME="$dir/home" GIT_AUTHOR_NAME=demo GIT_AUTHOR_EMAIL=demo@example.com \
  GIT_COMMITTER_NAME=demo GIT_COMMITTER_EMAIL=demo@example.com
# make_project <rel> <title> <commit message> <days ago> [dirty|ahead|behind]
make_project() {
  local p="$HOME/projects/$1" date
  date="$(($(date +%s) - $4 * 86400)) +0000"
  mkdir -p "$p"
  printf '# %s\n\n%s\n' "$2" "A sample project for the codehop demo." >"$p/README.md"
  git -C "$p" init -q -b main
  git -C "$p" add .
  GIT_AUTHOR_DATE="$date" GIT_COMMITTER_DATE="$date" git -C "$p" commit -q -m "$3"
  case "${5:-}" in
    dirty) echo change >>"$p/README.md"; touch "$p/todo.txt" ;;
    ahead | behind)
      git clone -q --bare "$p" "$dir/remotes/$1.git"
      git -C "$p" remote add origin "$dir/remotes/$1.git"
      git -C "$p" fetch -q origin && git -C "$p" branch -q -u origin/main
      echo more >>"$p/README.md" && git -C "$p" commit -q -am "wip: next step"
      if [ "$5" = behind ]; then git -C "$p" push -q origin main && git -C "$p" reset -q --hard HEAD~1; fi ;;
  esac
}
make_project web/design-system "Design System" "feat: add color tokens" 2 dirty
make_project web/website "Website" "fix: footer links" 12
make_project web/website-v2 "Website v2" "feat: new landing page" 3 ahead
make_project games/color-shuffle "Color Shuffle" "feat: daily puzzle" 1
make_project games/high-tide "High Tide" "chore: bump engine" 40
make_project services/api-server "API Server" "feat: rate limiting" 5 behind
make_project services/auth-service "Auth Service" "fix: token refresh" 9
make_project tools/dotfiles "Dotfiles" "chore: tidy zshrc" 70

mkdir -p "$HOME/.config/codehop"
cat >"$HOME/.config/codehop/config.toml" <<TOML
roots = ["~/projects"]
editor = "open-in-editor"
TOML
cat >"$HOME/.zshrc" <<'ZSH'
export PATH="$HOME/../bin:$PATH"
PROMPT='%F{magenta}❯%f '
autoload -Uz compinit && compinit -u -d "$HOME/.zcompdump"
zstyle ':completion:*' menu select
zstyle ':completion:*:descriptions' format '%F{8}%d%f'
eval "$(codehop init zsh --cmd code)"
ZSH
echo "demo ready in $dir"
