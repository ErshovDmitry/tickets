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

Environment variables (`TICKETS_DIR`, `TICKET_WHO`, `TICKET_LANG`) and the global `-C` / `--tickets-dir` flag: [configuration.md](configuration.md).

## List output

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

## Full command reference

Verbatim output of `ticket help` (`TICKET_LANG=en`; the first line shows the binary version, `dev` if built without `-ldflags`):

```
ticket version dev
ticket — project tickets (T-NNNN-<status>.md files in <project>/tickets/).

  ticket init
      create the tickets structure in the current directory: tickets/ and tickets/archive/ (idempotent)
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

Global flag (before the command, every command except init):
  -C <path>, --tickets-dir <path>
      path to the tickets directory itself (not the project root); works from any current directory

Tickets dir discovery (first match wins): -C/--tickets-dir → $TICKETS_DIR → upward scan for tickets/ from the current directory.

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
- i18n: the file format follows `TICKET_LANG` at creation time. An English locale produces plain labels (`## Summary`, `- Status: open`); a Russian locale produces bilingual labels (`## Summary (Кратко)`, `- Status (Статус): open`). Old files with Russian-only headers are not supported by new versions — see the migration one-liner in the `ticket help` output above.
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
