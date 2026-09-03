**English** | [Русский](README.ru.md)

# ticket

Cross-platform (Windows / Linux / macOS) CLI ticket system in Go.

## What is it

- Tickets are plain Markdown files `T-NNNN-<status>.md` in the project's `tickets/` directory. Files are the single source of truth: readable by a human without any tools (`cat`, editor, `git diff`), no database required.
- One static binary `ticket` (`ticket.exe` on Windows), built with `CGO_ENABLED=0`, zero runtime dependencies.
- Cross-platform counterpart of the bash version [init-tickets.md](https://gitlab.com/ai-dmitry/promts/-/blob/main/init-tickets.md).

## Installation

The ready binary goes into `<project>/tickets/bin/` — from there it locates the tickets directory itself and works from any current directory. Resolution order (first match wins):

1. `$TICKETS_DIR` — explicit override;
2. upward scan from the current directory for a child directory named `tickets` (git-style);
3. exe-relative: `<dir-of-exe>/..` — the standard layout `tickets/bin/ticket`.

### Configuration (env)

| Variable | Purpose |
|----------|---------|
| `TICKETS_DIR` | explicit tickets directory override |
| `TICKET_WHO` | ticket author; chain `TICKET_WHO → USER → USERNAME → agent` |
| `TICKET_LANG` | language of `ticket help` and of files created by `ticket new`; chain `TICKET_LANG → LC_ALL → LANG`, default **RU** (EN is opt-in: `TICKET_LANG=en`) |

## Build from source

Requires Go >= 1.26 (the only external dependency is `golang.org/x/sys`, needed for file locking on Windows).

The binary version comes from the git tag (fallback `dev`); set the variable once before building:

```bash
VER=$(git describe --tags --always 2>/dev/null | sed 's/^v//'); VER=${VER:-dev}
```

Current platform:

```bash
CGO_ENABLED=0 go build -ldflags "-X ticket/internal/cli.version=$VER" -o tickets/bin/ticket ./cmd/ticket
```

Cross-compilation (all with `CGO_ENABLED=0`, format `GOOS=... GOARCH=... go build -o dist/<name> ./cmd/ticket`):

| Target | Command |
|--------|---------|
| linux/amd64 | `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-X ticket/internal/cli.version=$VER" -o dist/ticket-linux-amd64 ./cmd/ticket` |
| linux/arm64 | `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags "-X ticket/internal/cli.version=$VER" -o dist/ticket-linux-arm64 ./cmd/ticket` |
| windows/amd64 | `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags "-X ticket/internal/cli.version=$VER" -o dist/ticket.exe ./cmd/ticket` |
| windows/arm64 | `CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build -ldflags "-X ticket/internal/cli.version=$VER" -o dist/ticket-windows-arm64.exe ./cmd/ticket` |
| darwin/amd64 | `CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags "-X ticket/internal/cli.version=$VER" -o dist/ticket-darwin-amd64 ./cmd/ticket` |
| darwin/arm64 | `CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags "-X ticket/internal/cli.version=$VER" -o dist/ticket-darwin-arm64 ./cmd/ticket` |

> ⚠️ On filesystems without hardlink support (FAT/exFAT on removable media) the `set` and `archive` commands fail with a `link` error — this is fail-safe, no data is lost; keep `tickets/` on NTFS or a POSIX filesystem, see [DEPLOY.md](DEPLOY.md) for details.

If the project is under git — decide whether to commit `tickets/` together with the project or add it to `.gitignore`.

## Usage

Quick start:

```bash
# create a ticket (status open), prints the file path
ticket new "Export breaks" -t BUG -p high -d "Fails on large files" -w ivan

# -P overrides the project (default = basename of tickets/ parent);
# a warning is printed to stderr when a project name is seen for the first time
ticket new "Cross-project entry" -P otherproj

# list tickets: default is active (= open + wip)
ticket list
ticket list all        # including done/closed
ticket list archive    # archive only
ticket list all -P otherproj   # filter by project (exact match, case-sensitive)

# show a ticket (searches the archive too)
ticket show 7

# change status: open → wip → done / closed
ticket set 7 wip "took the ticket"
ticket set 7 done "fixed, tests green"

# move closed tickets (done/closed) into tickets/archive/
ticket archive         # all closed
ticket archive 7       # one specific

# binary version (dev if built without -ldflags)
ticket version
```

Example `ticket list all` output — the project column appears only when projects differ in the output:

```
T-0001  tmp         open     BUG: Export fails on large files
T-0002  tmp         open     ENH: Add JSON output to list
T-0003  otherproj   open     BUG: Cross-project entry
```

With a `-P` filter the project column is dropped (single project):

```
T-0003  open     BUG: Cross-project entry
```

Full command reference — verbatim output of `ticket help` (`TICKET_LANG=en`; the first line shows the binary version, `dev` if built without `-ldflags`):

```
ticket version dev
ticket — project tickets (T-NNNN-<status>.md files in <project>/tickets/).

  ticket new "<brief>" [-t BUG|OPS|TD|ENH] [-p low|normal|high] [-d "<details>"] [-w who] [-P <project>]
      create a ticket (status open), prints the file path;
      -P overrides the project (default = basename of tickets/ parent);
      warning to stderr if the project is seen for the first time
  ticket list [active|open|wip|done|closed|archive|all] [-P <project>]
      list tickets; default active (= open + wip);
      archive — closed tickets moved into the archive;
      -P filters by project (exact match, case-sensitive);
      project column shown when output tickets have differing projects
  ticket show <number|filename>
      show a ticket (the archive is searched too)
  ticket set <number> <status> ["comment"]
      change the status (renames the file itself and appends a line to the ticket journal);
      re-setting the same status with a comment appends a journal entry
      (status and file name unchanged), without a comment it is an error;
      moving an archived ticket back to open/wip restores it from the archive into work
  ticket archive [<number>]
      move closed tickets (done/closed) into archive/;
      without a number — all closed ones, with a number — only the given one

Statuses: open — new; wip — in progress; done — fixed; closed — rejected/duplicate.
Types: BUG — breakage; OPS — incident/maintenance; TD — tech debt; ENH — enhancement.
Secrets (passwords, tokens, keys) must NOT be written into tickets.

Migrating old tickets: the old ticket format with Russian headers is not supported by new versions; run once over tickets/*.md:

sed -i -E \
 -e 's/^## Кратко$/## Summary (Кратко)/' \
 -e 's/^## Подробности$/## Details (Подробности)/' \
 -e 's/^## Комментарии от пользователя$/## User comments (Комментарии от пользователя)/' \
 -e 's/^## Комментарии$/## Comments (Комментарии)/' \
 -e 's/^## Журнал$/## Journal (Журнал)/' \
 -e 's/^- Статус:/- Status (Статус):/' \
 -e 's/^- Приоритет:/- Priority (Приоритет):/' \
 -e 's/^- Создан: (.*) · кем: (.*)$/- Created (Создан): \1 · by (кем): \2/' \
 -e 's/^- Проект:/- Project (Проект):/' tickets/*.md
```

> Note: the migration command is written for GNU sed (Linux); on macOS use `sed -i ''` instead, on Windows run it in Git Bash or WSL.

## Ticket file format

- File name: `T-NNNN-<status>.md`; numbers are issued atomically via OS locks and are never reused.
- Status lives in the file name. Never rename files manually: `ticket set` performs the status change (it renames the file itself and appends the ticket journal).
- Lifecycle: `open` (new) → `wip` (in progress) → `done` (fixed) / `closed` (rejected/duplicate).
- Types in the header: `BUG` — breakage; `OPS` — incident/maintenance; `TD` — tech debt; `ENH` — enhancement.
- i18n: the file format follows `TICKET_LANG` at creation time. An English locale produces plain labels (`## Summary`, `- Status: open`); a Russian locale produces bilingual labels (`## Summary (Кратко)`, `- Status (Статус): open`). Old files with Russian-only headers are not supported by new versions — see the migration one-liner in the `ticket help` output.
- `ticket new` without `-d` inserts an HTML comment hint into `Details` (what was found, where — file:line, logs, how to reproduce, fix proposal).
- Archive: `ticket archive` moves closed tickets into `tickets/archive/`; `list archive` shows the archive; `show` and `set` work with archived tickets — moving one back to `open`/`wip` returns it into work.
- Secrets (passwords, tokens, keys) must NOT be written into tickets.

Example ticket (`TICKET_LANG=en ticket new "Broken export" -t ENH -p low`):

```markdown
# T-0002 · ENH: Broken export

- Status: open
- Priority: low
- Created: 2026-09-04 05:54 · by: erdmitry
- Project: tmp

## Summary
Broken export

## Details
<!-- what was found, where (file:line), logs/output, how to reproduce, fix proposal -->

## User comments
_User remarks: write here — the agent reads this section before working on the ticket. The agent does not write here._

## Comments

## Journal
- 2026-09-04 05:54 — ticket created (erdmitry).
```

Sections: `Summary` — brief description; `Details` — specifics (file:line, logs, reproduction); `User comments` — written by the user, read by the agent before working on the ticket, the agent never writes there; `Comments` — the agent's working notes; `Journal` — append-only log written automatically by `new`/`set`.

## Integration into a project AGENTS.md

Paste the block below into the `AGENTS.md` of the project that uses `tickets/` (create the file in the project root if it does not exist; paths are relative to the project root; the binary finds the tickets directory regardless of the current directory):

```markdown
## Tickets (`tickets/`)

- At session start: `./tickets/bin/ticket list` — if there are open tickets, briefly remind the user about them in your first reply.
- Found a problem, bug, or anything suspicious while working — create a ticket IMMEDIATELY: `./tickets/bin/ticket new "<brief>" -t BUG|OPS|TD|ENH -p low|normal|high -d "<details>"`. Do not stay silent, even if you worked around or fixed it on the spot. Put file:line into the details.
- Status changes — only via `./tickets/bin/ticket set <number> <status> "<comment>"` (the binary renames the file itself and appends the ticket journal); never rename ticket files manually.
- Before working on a ticket, read its `User comments` section — the user leaves remarks there; the agent does not write there. Free-form working notes go to `Comments`.
- Tests/smoke runs of `ticket` — only in a sandbox (a temp directory via `$TICKETS_DIR`), NEVER in the live `tickets/`.
- Many closed tickets (`done`/`closed`) — move them into the archive: `./tickets/bin/ticket archive`. Archived tickets are not lost: `ticket list archive`, `show`/`set` work with them, reopening returns a ticket into work.
- Do NOT write secrets (passwords, tokens, keys) into tickets.
```

## Development

```bash
gofmt -l . && go vet ./...      # must be clean before commit
go test ./...                   # must be green before marking done
```

Local build — see [Build from source](#build-from-source). For Windows: cross-compile `dist/ticket.exe` (windows/amd64, see the table above) and run `scripts/smoke-windows.ps1` on a Windows host (manually over SSH). The smoke script verifies `new`/`list`/`show`/`set`, exe-relative resolution from a foreign working directory, the `TICKETS_DIR` override, and parallel `new` ×5 (five unique sequential numbers via OS lock) — entirely inside a temp sandbox, the repo and user data are untouched.

## Links

- bash original: [init-tickets.md](https://gitlab.com/ai-dmitry/promts/-/blob/main/init-tickets.md)
- `AGENTS_ARCHITECTURE.md` — architecture (local file, not published to the repository)
- `DEPLOY.md` — build, filesystem requirements

## Status

**v1 done.**
