#!/usr/bin/env bash
# End-to-end check of the bash integration with a fake editor. CI runs it in
# Git Bash on Windows; it works on any system with bash and Go.
set -euo pipefail

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
exe=""
case "$(uname -s)" in MINGW* | MSYS* | CYGWIN*) exe=".exe" ;; esac

go build -o "$work/bin/codehop$exe" ./cmd/codehop
go build -o "$work/bin/fakeeditor$exe" ./internal/shell/testdata/fakeeditor
mkdir -p "$work/projects/games/color-shuffle/.git" "$work/projects/web/blog/.git"

editor="$work/bin/fakeeditor$exe"
projects="$work/projects"
if command -v cygpath >/dev/null; then
  editor="$(cygpath -m "$editor")"
  projects="$(cygpath -m "$projects")"
fi
printf "roots = ['%s']\neditor = '\"%s\"'\n" "$projects" "$editor" >"$work/config.toml"

export PATH="$work/bin:$PATH"
export CODEHOP_CONFIG="$work/config.toml" CODEHOP_DATA_DIR="$work/data" CODEHOP_CACHE_DIR="$work/cache"
export FAKE_EDITOR_LOG="$work/editor.log"

eval "$(codehop init bash --cmd code)"
code shuffle
cdp blog
case "$PWD" in */web/blog) ;; *) echo "cdp failed: $PWD" >&2; exit 1 ;; esac
grep -q "color-shuffle" "$FAKE_EDITOR_LOG" || { echo "editor not called:" >&2; cat "$FAKE_EDITOR_LOG" >&2; exit 1; }
echo "smoke test passed"
