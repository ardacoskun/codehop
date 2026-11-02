# Contributing to codehop

Thanks for helping! Bug reports, ideas and pull requests are all welcome.

## Development

You need Go (see `go.mod` for the version). Optional: zsh, fish and PowerShell
to run every shell integration test locally (missing shells are skipped).

```sh
go build ./cmd/codehop       # build
go test ./...                # unit + shell integration tests
golangci-lint run            # lint (https://golangci-lint.run)
scripts/smoke-test.sh        # bash end-to-end check
```

Try your build without touching your real history or cache:

```sh
go build -o /tmp/codehop ./cmd/codehop
CODEHOP_DATA_DIR=/tmp/ch-data CODEHOP_CACHE_DIR=/tmp/ch-cache /tmp/codehop list
eval "$(/tmp/codehop init zsh)"   # in a throwaway shell
```

## Layout

| Path | What it does |
|---|---|
| `cmd/codehop` | entry point |
| `internal/cli` | commands (cobra) |
| `internal/discover` | finds projects under the roots, mtime-validated cache |
| `internal/match` | ranks projects for a query (exact → prefix → word → substring → fuzzy) |
| `internal/store` | frecency database |
| `internal/editor` | editor presets and launching |
| `internal/picker` | fzf picker with numbered fallback |
| `internal/gitstat`, `internal/preview` | `codehop status` and picker previews |
| `internal/clone` | `gh:owner/repo` and git URL cloning |
| `internal/shell` | `codehop init` templates and their integration tests |

The shell templates stay thin: anything that can live in Go (matching,
deciding whether `code x` is a path or a project) does, so all shells behave
the same.

## Pull requests

- Keep changes focused; add or update tests.
- Use [Conventional Commits](https://www.conventionalcommits.org/) for PR titles
  (`feat: ...`, `fix: ...`). Releases and the changelog are generated from them.
- New user-facing behavior needs README and `--help` updates.

## Going public (one-time checklist)

The repository starts private. When making it public:

1. Create the public repositories `ardacoskun/homebrew-tap` and `ardacoskun/scoop-bucket`.
2. Create a fine-grained token with contents write access to those two repositories only.
3. Create a `release` environment (Settings → Environments) with that token as
   `TAP_GITHUB_TOKEN`, and add `environment: release` to the goreleaser job in
   `.github/workflows/release.yml`.
4. Release as usual. Homebrew, Scoop and build attestations switch on
   automatically for public repositories.

## Releasing (maintainers)

Merging to `main` keeps a release PR open (release-please). Merging that PR tags
the version; GoReleaser then publishes binaries, the Homebrew cask and the Scoop
manifest. The `TAP_GITHUB_TOKEN` secret must be a fine-grained token with
contents write access to `homebrew-tap` and `scoop-bucket`.
