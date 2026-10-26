# Development

Building `ticket` from source, cross-compiling for all supported targets, and the checks that must pass before a commit.

## Requirements

- Go >= 1.26.
- The only external dependency is `golang.org/x/sys`, needed for file locking on Windows.
- Build with `CGO_ENABLED=0` — the result is a static binary with zero runtime dependencies.

## Build from source

The binary version comes from the git tag (fallback `dev`). Set the variable once before building:

```bash
VER=$(git describe --tags --always 2>/dev/null | sed 's/^v//'); VER=${VER:-dev}
```

Current platform:

```bash
CGO_ENABLED=0 go build -ldflags "-X ticket/internal/cli.version=$VER" -o ~/bin/ticket ./cmd/ticket
```

Installing to `PATH` and per-project setup: [installation.md](installation.md).

### Cross-compilation

All targets use `CGO_ENABLED=0`, format `GOOS=... GOARCH=... go build -o dist/<name> ./cmd/ticket`:

| Target | Command |
|--------|---------|
| linux/amd64 | `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-X ticket/internal/cli.version=$VER" -o dist/ticket-linux-amd64 ./cmd/ticket` |
| linux/arm64 | `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags "-X ticket/internal/cli.version=$VER" -o dist/ticket-linux-arm64 ./cmd/ticket` |
| windows/amd64 | `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags "-X ticket/internal/cli.version=$VER" -o dist/ticket.exe ./cmd/ticket` |
| windows/arm64 | `CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build -ldflags "-X ticket/internal/cli.version=$VER" -o dist/ticket-windows-arm64.exe ./cmd/ticket` |
| darwin/amd64 | `CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags "-X ticket/internal/cli.version=$VER" -o dist/ticket-darwin-amd64 ./cmd/ticket` |
| darwin/arm64 | `CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags "-X ticket/internal/cli.version=$VER" -o dist/ticket-darwin-arm64 ./cmd/ticket` |

> ⚠️ On filesystems without hardlink support (FAT/exFAT, common on Windows removable media) the `set` and `archive` commands fail with a `link` error. This is fail-safe: no data is lost. Keep `tickets/` on NTFS or a POSIX filesystem with hardlink support.

## Hygiene

Must be clean before a commit, green before marking work done:

```bash
gofmt -l . && go vet ./...      # must be clean before commit
go test ./...                   # must be green before marking done
```

## Markdown lint

Documentation is linted with [markdownlint-cli2](https://github.com/DavidAnson/markdownlint-cli2); the config lives in `.markdownlint-cli2.jsonc`.

What it checks: Markdown structure and hygiene — blank lines around headings (MD022) and code blocks (MD031), lists surrounded by blank lines (MD032), no inline HTML where a Markdown construct fits (MD033), no bare URLs (MD034), and a language tag on every fenced code block (MD040).

Disabled rules, and why:

- **MD013** (line length) — prose in `docs/` is written as long single lines (up to 420 characters); wrapping it would be a mass reflow of every document, outside the scope of this change.
- **MD060** (table column style) — 36 violations across 7 tables that mix compact and padded column styles; unifying table formatting is a separate task.

Scope: `tickets/archive/**` is excluded by user decision (2026-10-26, item 2b); active `tickets/*.md` are linted. Also excluded: `docs_api/**`, `AGENTS.md`, `AGENTS_ARCHITECTURE.md`, `DEPLOY.md`, `node_modules/**`, `dist/**`.

Local run — the version used in CI is pinned (0.23.3):

```bash
markdownlint-cli2 --no-globs $(git ls-files '*.md')
```

## Windows smoke

Cross-compile `dist/ticket.exe` (windows/amd64, see the table above) and run `scripts/smoke-windows.ps1` on a Windows host (manually over SSH):

```powershell
powershell -ExecutionPolicy Bypass -File scripts\smoke-windows.ps1
```

The script runs entirely inside a temp sandbox — the repo and user data are untouched. Exit code 0 means all steps passed, 1 means at least one failed. Seven steps:

1. `new` — creates the ticket and prints its path;
2. `list` — shows the ticket;
3. `show` — prints the ticket body;
4. `set` — renames the file, updates the status line and journal;
5. global `-C` / `--tickets-dir` flag from a foreign working directory;
6. `TICKETS_DIR` environment override;
7. parallel `new` ×5 — five unique sequential numbers via OS lock.
