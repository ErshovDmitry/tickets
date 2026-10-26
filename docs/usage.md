# Usage

Day-to-day commands of the `ticket` CLI and the on-disk format of the ticket files it produces.

## Commands

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

Parse warnings from `list` are printed to stderr; the command returns exit code 0 and shows the tickets it parsed successfully.

Command shapes at a glance:

- `ticket new "<brief>" [-t BUG|OPS|TD|ENH] [-p low|normal|high] [-d "<details>"] [-w who] [-P <project>]`
- `ticket list [active|open|wip|done|closed|archive|all] [-P <project>]`
- `ticket show <number|filename>`
- `ticket set <number> <status> ["comment"]`
- `ticket archive [<number>]`
- `ticket version`

Environment variables (`TICKETS_DIR`, `TICKET_WHO`, `TICKET_LANG`, `TICKET_LOCK_TIMEOUT`) and the global `-C` / `--tickets-dir` flag: [configuration.md](configuration.md).

## List output

Example `ticket list all` output — the project column appears only when projects differ in the output:

```text
T-0001  tmp         open     BUG: Export fails on large files
T-0002  tmp         open     ENH: Add JSON output to list
T-0003  otherproj   open     BUG: Cross-project entry
```

With a `-P` filter the project column is dropped (single project):

```text
T-0003  open     BUG: Cross-project entry
```

## Full command reference

Help text (`TICKET_LANG=en`), byte-identical to the embedded template `internal/cli/templates/usage.en.txt`. The binary prepends a version line (`ticket version <ver>`, `dev` when built without `-ldflags`) that is not part of the template:

```text
ticket — project tickets (T-NNNN-<status>.md files in <project>/tickets/).

  ticket init
      create the tickets structure in the current directory: tickets/ and tickets/archive/ (idempotent)
  ticket new "<brief>" [-t BUG|OPS|TD|ENH] [-p low|normal|high] [-d "<details>"] [-w who] [-P <project>]
      create a ticket (status open), prints the file path;
      -P overrides the project (default = basename of tickets/ parent);
      warning to stderr if the project is seen for the first time
  ticket list [active|open|wip|done|closed|archive|all] [-P <project>] [--json]
      list tickets; default active (= open + wip);
      archive — closed tickets moved into the archive;
      -P filters by project (exact match, case-sensitive);
      project column shown when output tickets have differing projects;
      --json outputs a JSON array (6 fields: number, status, type, priority, title, created); empty = []
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

Global flag (before the command, every command except init):
  -C <path>, --tickets-dir <path>
      path to the tickets directory itself (not the project root); works from any current directory

Tickets dir discovery (first match wins): -C/--tickets-dir → $TICKETS_DIR → upward scan for tickets/ from the current directory.

Lock timeout: TICKET_LOCK_TIMEOUT sets how long ticket waits to acquire .lock before returning an error; the value is a Go duration with a mandatory unit (30s, 2m, 500ms); an empty or invalid value silently falls back to the default; 0 or less means a single attempt with no wait; it applies to new, set and archive.

Statuses: open — new; wip — in progress; done — fixed; closed — rejected/duplicate.
Types: BUG — breakage; OPS — incident/maintenance; TD — tech debt; ENH — enhancement.
Secrets (passwords, tokens, keys) must NOT be written into tickets.
```

## Ticket file format

- File name: `T-NNNN-<status>.md`; numbers are issued atomically via OS locks and are never reused.
- Status lives in the file name. Never rename files manually: `ticket set` performs the status change (it renames the file itself and appends the ticket journal).
- Lifecycle: `open` (new) → `wip` (in progress) → `done` (fixed) / `closed` (rejected/duplicate).
- Types in the header: `BUG` — breakage; `OPS` — incident/maintenance; `TD` — tech debt; `ENH` — enhancement.
- i18n: the file format follows `TICKET_LANG` at creation time. An English locale produces plain labels (`## Summary`, `- Status: open`); a Russian locale produces bilingual labels (`## Summary (Кратко)`, `- Status (Статус): open`). Tickets with Russian-only headers (`## Кратко`, `- Статус:`) are not recognised by the parser: those fields stay empty and the next `set` rewrites the file in the current format without them — fix such files by hand. Files in the pre-MD022 format (current format, but without the blank line after the heading) are read and normalised on the next `set`.
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

## Comments

## Journal
- 2026-09-04 05:54 — ticket created (erdmitry).
```

## Sections

- `Summary` — brief description.
- `Details` — specifics (file:line, logs, reproduction).
- `User comments` — written by the user, read by the agent before working on the ticket; the agent never writes there.
- `Comments` — the agent's working notes.
- `Journal` — append-only log written automatically by `new` / `set`.

Agent integration, cross-project work, and the archive workflow in context: [scenarios.md](scenarios.md).
