---
name: ticket-cli
version: "1.10"
created: 2026-09-04
modified: 2026-11-11
type: guide
tags: [ticket, tickets, t-nnnn, cli, task-tracker, dogfooding]
description: "Workflow for the `ticket` CLI — file-based task tracker (tickets are plain `T-NNNN-<status>.md` files, no DB). Keywords: ticket, tickets, T-NNNN, CLI task tracker, dogfooding, ticket new/list/show/set/archive/recover. PROACTIVELY activate for: (1) any ticket CRUD via the `ticket` binary, (2) working on a ticket T-NNNN, (3) closing/archiving tickets, (4) resolving a crash pair (two files, ambiguous number) with `ticket recover`."
---

# ticket-cli

Workflow for operating the `ticket` CLI. Tickets are plain Markdown files — the files are the single source of truth; there is NO database. Commands below are imperative: MUST / NEVER / ONLY.

## Context

- Tickets are plain files `tickets/T-NNNN-<status>.md`; files are the single source of truth; there is NO database.
- The CLI is `ticket` on your `PATH` (`~/bin/ticket`; `%USERPROFILE%\bin\ticket.exe` on Windows); `-P <project>` flag: see README.
- Numbers are issued atomically via OS locks and are never reused. Status lives in the file name — NEVER rename ticket files manually.
- Ticket directory discovery, first match wins: `-C`/`--tickets-dir` → `$TICKETS_DIR` → upward scan from cwd for `tickets/` (see README Installation).
- i18n: language chain `TICKET_LANG → LC_ALL → LANG`, default **RU**; `TICKET_LANG=en` opts into English help and English-label ticket files.
- 🔴 Ticket CONTENT language: write titles, Details, comments and journal entries in **Russian** (project rule since 2026-11-06). The `-t`/`-p` flag values and file names stay English (`T-NNNN-<status>.md`); the i18n setting above only governs help text and section labels, not what you write.

## Commands

### Global flags

Two flags are global, and both are accepted ONLY before the command name: a flag-like token after the command name is a subcommand argument and is rejected (e.g. `ticket list --verbose` fails).

`-C <path>` / `-C<path>` / `--tickets-dir <path>` / `--tickets-dir=<path>` — the value is the path to the tickets directory itself (not a parent). Overrides `TICKETS_DIR`. Works with every command except `init` (which creates, never resolves).

`--verbose` enables the debug log; `TICKET_LOG=<path>` sends the records to that file instead — the file wins over the flag. Log records NEVER reach stdout: stdout stays pure command output.

Record format, destinations, failure behaviour and the complete event list are owned by `docs/configuration.md` (`## Debug log (--verbose / TICKET_LOG)`) — this skill cross-references them and does not repeat them.

### `init`

```text
ticket init
```

Creates `tickets/` and `tickets/archive/` in the current directory. Idempotent: repeat runs do nothing. Exit 1 if `tickets` exists as a non-directory (file/FIFO/broken link) — never overwrites. It does NOT install the binary.

### `new`

```text
ticket new "<brief>" [-t BUG|OPS|TD|ENH] [-p low|normal|high] [-d "<details>"] [-w who] [-P <project>]
```

Creates a ticket with status `open` and prints the file path. `-w` sets the author. `-P` overrides the project (default: basename of the tickets-dir parent); a stderr warning is printed when a project name is seen for the first time.

Details input (`-d`): plain text is used literally; `@<file>` reads that file; `@@name` reads the file named `@name` (exactly one `@` is stripped — `@@` is NOT an escape); `-` reads stdin; a bare `@` (no path) is an error. The content is copied byte for byte — no trimming, no newline conversion, no size limit (more than 64 KiB is accepted); an empty file or empty stdin gives an empty `Details` value — the created ticket then shows the Details format block (the same as `-d ""`). A failed read prints a localized message and creates NO ticket.

Details format: a numbered list, one item per line — `N. <what> — <file:line>; <fix>` (RU: `N. <что> — <файл:строка>; <решение>`). A ticket created without `-d` — or with an empty `Details` value, e.g. from an empty file or stdin — gets the same rule as a format block in `Details`.

### `list`

```text
ticket list [active|open|wip|done|closed|archive|all] [-P <project>] [--json]
```

Default is `active` (= open + wip). `archive` lists tickets already moved into `tickets/archive/` (done/closed only). `-P` filters by project (exact, case-sensitive); the project column appears when projects in the output differ. `--json` (since v1.2.3, T-0081) prints a JSON array with 6 fields (`number, status, type, priority, title, created`); empty result = `[]`.

### `show`

```text
ticket show <number|filename>
```

Shows a ticket; searches the archive too.

### `set`

```text
ticket set <number> <status> ["comment"]
```

Renames the file and appends a journal line. Re-setting the SAME status WITH a comment appends a journal entry only (no rename); the same status WITHOUT a comment is an error. Moving an archived ticket to `open`/`wip` returns it from the archive into work.

### `recover`

```text
ticket recover <number> <new-file> [--yes]
```

Repairs the pair an interrupted status change leaves behind (`T-NNNN-old.md` + `T-NNNN-new.md`, e.g. `T-0007-open.md` + `T-0007-done.md`): `show`, `set` and `archive` report the number as ambiguous (exit 2) and print both conflicting ABSOLUTE paths. `<new-file>` is the basename from that diagnostic (e.g. `T-0007-done.md`) — it selects the canonical file, and the path is NEVER guessed: the basename is matched against `tickets/` and `tickets/archive/` (an identical basename in both dirs is a refusal).

Only the exact crash signature is accepted: the new file must equal the old one plus exactly one trailing journal entry `From=old status → To=new status`, and the old file must LACK that entry. The comparison runs after parse/render (all fields, unknown/RawTemplates included), so any other difference is rejected and the pair is left byte-identical.

What it does NOT do: it deletes ONLY the old file — the new file is never opened for writing, its content and journal stay byte-identical, no `tmp/` and no recovery journal entry are created. Repeating the command when only the new file remains is a no-op with exit 0. The keep-old choice is refused; roll the status back afterwards with a normal `ticket set <number> <old-status> "reason"`.

Confirmation: without `--yes` it prints keep/discard and asks yes/no; a non-TTY run without `--yes` exits 1 having done nothing, so automation MUST pass `--yes`. Any error keeps the pair for a retry, but path reporting differs: the ambiguity report (more than two files, also from `show`/`set`/`archive`) prints every conflicting ABSOLUTE path, while a `RecoveryState` refusal prints only the conflicting BASENAMES (same-status names none). Never assume paths appear in every error — pass the bare basename.

🔴 Run `recover` only in a sandbox (`-C /tmp/…` or `TICKETS_DIR`): it deletes a file — NEVER in the live `tickets/`.

### `archive`

```text
ticket archive [<number>]
```

Moves done/closed tickets into `tickets/archive/`; no number = all closed, with a number = only the given one.

## Exit codes

- `0` — success.
- `1` — error (bad arguments, ticket not found, collision, input or store error).
- `2` — ambiguous ticket number: the number belongs to several ticket files; the number and the conflicting files are printed to stderr.

`ticket recover` reuses these codes: `0` also covers an idempotent repeat; `1` covers a usage error, a refused confirmation, a missing file, an I/O error (archive scan included), the keep-old choice and a selection that is not the crash pair; `2` means the number cannot be recovered automatically — more than two files, diverging content or equal statuses. A divergent pair is `2`, never `1`.

## Status lifecycle

`open` → `wip` → `done` / `closed`.

| Status | Meaning |
|--------|---------|
| `open` | new |
| `wip` | in progress |
| `done` | fixed |
| `closed` | rejected / duplicate |

Types: `BUG` — breakage; `OPS` — incident/maintenance; `TD` — tech debt; `ENH` — enhancement.

## Ticket file format

Labels are bilingual under the RU default locale:

- Header: `# T-NNNN · TYPE: title`.
- Bullets: `- Status (Статус):`, `- Priority (Приоритет):`, `- Created (Создан): … · by (кем): …`, `- Project (Проект):`.
- Sections: `## Summary (Кратко)`, `## Details (Подробности)`, `## User comments (Комментарии от пользователя)`, `## Comments (Комментарии)`, `## Journal (Журнал)`.
- `Details` — specifics as a numbered list, one item per line: `N. <what> — <file:line>; <fix>`.
- With `TICKET_LANG=en` plain labels are produced: `## Summary`, `- Status: open`.
- Journal line format: `YYYY-MM-DD HH:MM — статус: X → Y · comment (who)`.
- `Journal` is append-only, written automatically by `new`/`set`.

## Pitfalls

1. `ticket new --help` now prints help (bug T-0041 fixed; verified on v1.2.3 — no garbage ticket is created). Still: after `new`, check the created file before moving on.
2. 🔴 Test/smoke runs ONLY in a sandbox: set `TICKETS_DIR` or pass `-C /tmp/…`. NEVER experiment in the live `tickets/` dir (incident T-0023).
3. 🔴 NEVER write secrets (passwords, tokens, keys) into tickets.
4. 🔴 Closing gate: verify the fix FIRST, then IMMEDIATELY `ticket set N done "<comment with evidence>"`. The comment MUST state what was checked: command + result, file:line. `done` without evidence is forbidden; NEVER batch status flips "for later".
5. 🔴 BEFORE working on a ticket, read its `## User comments` section — the user leaves remarks there; the agent NEVER writes there. The agent's working notes go to `## Comments`.
6. Tickets with Russian-only headers (`## Кратко`, `- Статус:`) are NOT parsed: those fields stay empty and the next `set` rewrites the file in the current format without them — fix such files by hand. There is no migration command. Files in the pre-MD022 format (current format without the blank line after the heading) are read and normalised on the next `set`.
7. `TICKET_LOCK_TIMEOUT` controls how long `ticket` waits for the tickets-directory lock (`.lock`) before failing; the value is a Go duration with a mandatory unit (`30s`, `2m`, `500ms`), and an empty or invalid value silently falls back to the default — a typo never errors out. Raise it for parallel agent runs or CI. The default itself is owned by the code (`defaultLockTimeout` in `internal/store`), so this skill does not repeat the number.
8. `-C`/`--tickets-dir` and `--verbose` MUST come BEFORE the command name; after it the token is a subcommand argument and the command fails — except after `help`/`version`, where it is ignored — see `docs/configuration.md` for the exact messages.

## Workflow quick reference

1. `ticket list` — see open tickets (remind the user at session start).
2. `ticket new "..." -t ... -p ... -d "..."` — file a problem/incident immediately, with file:line in details; long details go through `-d @file` or `-d -`.
3. `ticket set N <status> "<comment>"` — move statuses; close with evidence only.
4. `ticket archive` — tidy up closed tickets.
5. `ticket show N` — works on archived tickets too.

## Structure

```text
ticket-cli/
├── SKILL.md
└── reference.md
```

## See also

- `reference.md` — verbatim `ticket` help output (RU).

## File History & Known Issues

| Date | Issue | Fix Applied | Version |
|------|-------|-------------|---------|
| 2026-09-04 | Initial version | Created workflow skill from `ticket` CLI help | 1.0 |
| 2026-09-04 | Review round 1: bilingual file format missing, type non-canonical, no See also | Added bilingual/EN label forms, TICKET_LANG + dir discovery notes, type: guide, See also | 1.1 |
| 2026-09-07 | T-0057: global flag + 3-tier discovery, init simplified | — | 1.2 |
| 2026-09-09 | Stale vs v1.2.3: `list --json` (T-0081) and `-P` on new/list missing; pitfall 1 described fixed bug T-0041 | Added `-P`/`--json` to new+list, rewrote pitfall 1 (verified empirically in /tmp sandbox) | 1.3 |
| 2026-10-27 | T-0094: `TICKET_LOCK_TIMEOUT` not documented; T-0095: migration sed snippet removed | Added the lock-timeout pitfall; rewrote pitfall 6 and See also to match the parser behaviour | 1.4 |
| 2026-10-29 | T-0109: the Details format rule and the `-d @file` / `-d -` input were undocumented | Documented the numbered-list Details format and the file/stdin details input (literal, `@file`, `@@name`, bare `@`, `-`, byte-exact content, no size limit, no ticket on failure) | 1.5 |
| 2026-11-06 | Debug log is not documented in the skill | Added `--verbose`/`TICKET_LOG` to Global flags and a cross-reference to `docs/configuration.md`; version bumped to 1.6 | 1.6 |
| 2026-11-06 | T-0110 Wave 7: pitfall 8 claimed that a misplaced `--verbose` always fails | Added the exception: after `help`/`version` the token is ignored; version bumped to 1.7 | 1.7 |
| 2026-11-10 | T-0158: exit code 2 (ambiguous ticket number) was undocumented | Added the "Exit codes" section | 1.9 |
| 2026-11-11 | T-0159: `ticket recover` was undocumented | Added the `recover` section (exact crash signature, keep/discard confirmation, non-TTY, archive pairs, idempotence, keep-old refusal, rollback via `set`, sandbox warning) and the recover exit-code notes | 1.10 |
