# Usage

Day-to-day commands of the `ticket` CLI and the on-disk format of the ticket files it produces.

## Commands

Quick start:

```bash
# create a ticket (status open), prints the file path
ticket new "Export breaks" -t BUG -p high -d "Fails on large files" -w ivan

# long details: from a file (@path) or from stdin (-); the content is copied byte for byte
ticket new "Export breaks" -d @details.md
ticket new "Export breaks" -d -

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

# resolve the pair left by an interrupted status change
# (the number reports exit 2: T-0007-open.md + T-0007-done.md)
ticket recover 7 T-0007-done.md

# binary version (dev if built without -ldflags)
ticket version
```

Parse warnings from `list` are printed to stderr; the command returns exit code 0 and shows the tickets it parsed successfully.

Command shapes at a glance:

- `ticket new "<brief>" [-t BUG|OPS|TD|ENH] [-p low|normal|high] [-d "<details>"] [-w who] [-P <project>]`
- `ticket list [active|open|wip|done|closed|archive|all] [-P <project>] [--json]`
- `ticket show <number|filename>`
- `ticket set <number> <status> ["comment"]`
- `ticket archive [<number>]`
- `ticket recover <number> <new-file> [--yes]`
- `ticket version`

Environment variables (`TICKETS_DIR`, `TICKET_WHO`, `TICKET_LANG`, `TICKET_LOCK_TIMEOUT`, `TICKET_LOG`) and the global `-C` / `--tickets-dir` flag: [configuration.md](configuration.md).

### Details input (`-d`)

- Plain text — used literally.
- `@<file>` — the content of that file; the content is copied byte for byte (no trimming, no newline conversion, no size limit — more than 64 KiB is accepted).
- `@@name` — the file named `@name`: exactly one `@` marker is stripped, `@@` is not an escape.
- `@` alone — an error: the path after `@` is required.
- `-` — the content of stdin.
- An empty file or empty stdin — an empty `Details` value; the command succeeds and the created ticket shows the Details format block (the same as with `-d ""`).
- A missing file or a failed stdin read — an error: the message is localized (the file error names the path) and no ticket is created.

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
      -d takes the details: plain text — literally, @<file> — file content, @@name — file named "@name", - — stdin;
      @ without a path is an error: no ticket is created, the message is localized;
      the content is copied byte for byte, an empty file or stdin gives an empty Details value — the ticket then shows the Details format block (the same as -d ""); there is no size limit;
      Details format — a numbered list, one item per line: N. <what> — <file:line>; <fix>
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
  ticket recover <number> <new-file> [--yes]
      recover a ticket after an interrupted status change that left two files for one number;
      <new-file> is the basename to keep (for example T-0001-done.md), never a path;
      without --yes it prints the kept and the deleted file and asks yes/no;
      without --yes and without a terminal it refuses and changes nothing;
      only the stale old file is removed, the kept file is never rewritten;
      a repeat after a successful recovery, and a single file already named <new-file>, are a conservative no-op success;
      recover only checks that the wanted file is present: it cannot prove that it is the previously saved file and not a lone stale one,
      so exit 0 means "the wanted name is already on disk", not "a recovery happened"
      exit 2 when the number is not cleanly recoverable (three or more files, diverging contents, equal statuses in the file names);
      keeping the stale file, naming a file outside the pair or a single file with another name is exit 1 and changes nothing

Global flag (before the command, every command except init):
  -C <path>, --tickets-dir <path>
      path to the tickets directory itself (not the project root); works from any current directory
  --verbose
      debug log to stderr; TICKET_LOG=<path> also enables logging and sends the records to that file (the file wins over the flag);
      accepted only BEFORE the command name; after help/version it is ignored; the glued form --verbose=x is not recognised;
      stdout is never used for log records

Tickets dir discovery (first match wins): -C/--tickets-dir → $TICKETS_DIR → upward scan for tickets/ from the current directory.

Lock timeout: TICKET_LOCK_TIMEOUT sets how long ticket waits to acquire .lock before returning an error; the value is a Go duration with a mandatory unit (30s, 2m, 500ms); an empty or invalid value silently falls back to the default; 0 or less means a single attempt with no wait; it applies to new, set and archive.

Exit codes: 0 — success; 1 — error (bad arguments, ticket not found, collision, input or store error); 2 — ambiguous ticket number (commands show, set, archive, new, recover): one number belongs to several files — the number and the conflicting files are printed to stderr. The list command prints the same warning for duplicates but exits with code 0.

Statuses: open — new; wip — in progress; done — fixed; closed — rejected/duplicate.
Types: BUG — breakage; OPS — incident/maintenance; TD — tech debt; ENH — enhancement.
Secrets (passwords, tokens, keys) must NOT be written into tickets.
```

## Global flags

Two global flags are recognised, and both are accepted only BEFORE the command name: `-C <dir>` / `--tickets-dir <dir>` (also `--tickets-dir=<dir>` and the glued `-C<dir>`) picks the tickets directory for one invocation, and `--verbose` turns on the debug log.

`--verbose` writes the log to stderr; if `TICKET_LOG` is set to a path, records go to that file instead (the file wins over the flag). stdout is never used for log records. A token that looks like a global flag but appears AFTER the command name is a subcommand argument, not a global flag — `ticket list --verbose` and `ticket new "T" -d x --verbose` are errors, not silent no-ops. `--verbose` after `help`/`version` is ignored, and the glued form `--verbose=x` is not recognised.

```bash
ticket --verbose list                 # debug log to stderr
ticket --verbose show 7 2>/dev/null   # log still goes to stderr, ticket text stays on stdout
```

The record format, the destination rules, the failure behaviour and the complete event list live in [configuration.md](configuration.md) — they are not duplicated here.

## Recovering after an interrupted status change (`recover`)

A status change is a multi-step commit: the new file is created by link, then the tmp and the old file are removed. If the process is killed or hits a timeout between the steps, both files survive — for example `T-0007-open.md` and `T-0007-done.md`. The number becomes ambiguous: `show`, `set` and `archive` print both conflicting ABSOLUTE paths to stderr and exit with code 2. For exactly that pair there is a command:

```bash
ticket recover <number> <new-file> [--yes]
```

- `<new-file>` — the BASENAME from the ambiguity diagnostic (for example `T-0007-done.md`); a directory must not be given: the basename is matched against entries in both `tickets/` and `tickets/archive/` (a pair left by a `reopen` is recovered too). An identical basename in both directories is a refusal: the path is never guessed.
- Without `--yes` the command prints what is kept and what will be discarded, and asks yes/no. Without a terminal (non-TTY) and without `--yes` — exit 1 and no action at all: a pipeline cannot delete a file by accident, automation MUST pass `--yes`.
- The signature is matched strictly: the new file must equal the old file plus EXACTLY one trailing journal entry `From=old status → To=new status`, and the old file must LACK that trailing entry. The comparison runs after parse/render (every field, unknown/RawTemplates included): any other difference, an extra or a foreign journal entry is a refusal.
- The selected name must be the new file. Naming the old one (keep old) is a refusal with a hint: `recover` the new file first, then `ticket set` back to the previous status if needed.
- ONLY the old file is deleted; the new file is never opened for writing, its content and journal stay byte-identical, no `tmp/` and no recovery journal entry are created.
- A repeat is a conservative no-op: when only the new file remains, the command exits 0 and changes nothing (the file is not reopened for writing). The same exit 0 covers a single file that is already named `<new-file>` — the postcondition is "the wanted name is on disk", never proof that this was the previously saved file.
- Rolling the status back is an ordinary follow-up `ticket set <number> <old-status> "reason"`: it appends the journal and renames the file. `recover` has no rollback flag.
- Any error leaves the pair in place. Path reporting depends on the diagnostic: the ambiguity report (more than two files, and the same report from `show`/`set`/`archive`) lists every conflicting ABSOLUTE path, while a `RecoveryState` refusal names only the conflicting BASENAMES — the same-status refusal names none. Do not count on paths appearing in every error: retry with the basename after fixing the cause.

Exit codes: `0` — success, including a conservative no-op (a repeat after a successful recovery, or a single file already named `<new-file>`) — `0` means the wanted file name is on disk and does NOT prove that a recovery happened; `1` — a usage error, a refused confirmation, a missing file, an I/O error (archive scan included), the keep-old choice and a selection that is not the crash pair; `2` — the number cannot be recovered automatically: more than two files, diverging content or equal statuses. For automation, exit code `2` means "do not touch it": the pair needs a manual review.

```bash
ticket set 7 done "fixed"                 # interrupted midway: exit 2, both files on stderr
ticket recover 7 T-0007-done.md           # interactive: keep/discard + yes/no
ticket recover 7 T-0007-done.md --yes     # the same without the question, for automation
```

🔴 `recover` deletes a file, so test it only in a sandbox: `-C /tmp/…` or `TICKETS_DIR`, never in the live `tickets/`.

## Ticket file format

- File name: `T-NNNN-<status>.md`; numbers are issued atomically via OS locks and are never reused.
- Status lives in the file name. Never rename files manually: `ticket set` performs the status change (it renames the file itself and appends the ticket journal).
- Lifecycle: `open` (new) → `wip` (in progress) → `done` (fixed) / `closed` (rejected/duplicate).
- Types in the header: `BUG` — breakage; `OPS` — incident/maintenance; `TD` — tech debt; `ENH` — enhancement.
- i18n: the file format follows `TICKET_LANG` at creation time. An English locale produces plain labels (`## Summary`, `- Status: open`); a Russian locale produces bilingual labels (`## Summary (Кратко)`, `- Status (Статус): open`). Tickets with Russian-only headers (`## Кратко`, `- Статус:`) are not recognised by the parser: those fields stay empty and the next `set` rewrites the file in the current format without them — fix such files by hand. Files in the pre-MD022 format (current format, but without the blank line after the heading) are read and normalised on the next `set`.
- `ticket new` without `-d` (or with an empty `Details` value, e.g. from an empty file or stdin) inserts a format block into `Details` — the numbered-list rule: one item per line, `N. <what> — <file:line>; <fix>`.
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
<!-- Details = numbered list:
1. <what> — <file:line>; <fix>
-->

## User comments

## Comments

## Journal
- 2026-09-04 05:54 — ticket created (erdmitry).
```

## Sections

- `Summary` — brief description.
- `Details` — specifics as a numbered list, one item per line: `N. <what> — <file:line>; <fix>` (file:line, logs, reproduction).
- `User comments` — written by the user, read by the agent before working on the ticket; the agent never writes there.
- `Comments` — the agent's working notes.
- `Journal` — append-only log written automatically by `new` / `set`.

Agent integration, cross-project work, and the archive workflow in context: [scenarios.md](scenarios.md).
