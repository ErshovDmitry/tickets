---
name: ticket-cli
version: "1.1"
created: 2026-09-04
modified: 2026-09-04
type: guide
tags: [ticket, tickets, t-nnnn, cli, task-tracker, dogfooding]
description: "Workflow for the `ticket` CLI — file-based task tracker (tickets are plain `T-NNNN-<status>.md` files, no DB). Keywords: ticket, tickets, T-NNNN, CLI task tracker, dogfooding, ticket new/list/show/set/archive. PROACTIVELY activate for: (1) any ticket CRUD via the `ticket` binary, (2) working on a ticket T-NNNN, (3) closing/archiving tickets."
---

# ticket-cli

Workflow for operating the `ticket` CLI. Tickets are plain Markdown files — the files are the single source of truth; there is NO database. Commands below are imperative: MUST / NEVER / ONLY.

## Context

- Tickets are plain files `tickets/T-NNNN-<status>.md`; files are the single source of truth; there is NO database.
- In this repo the CLI is `./tickets/bin/ticket` (bash reference). The Go binary `ticket` (when built/installed) has the same commands plus a `-P <project>` flag (see README).
- Numbers are issued atomically via OS locks and are never reused. Status lives in the file name — NEVER rename ticket files manually.
- Ticket directory discovery: env `TICKETS_DIR` overrides; otherwise the binary scans upward from cwd for `tickets/` (see README Installation).
- i18n: language chain `TICKET_LANG → LC_ALL → LANG`, default **RU**; `TICKET_LANG=en` opts into English help and English-label ticket files.

## Commands

### `new`

```
ticket new "<brief>" [-t BUG|OPS|TD|ENH] [-p low|normal|high] [-d "<details>"] [-w who]
```

Creates a ticket with status `open` and prints the file path. `-w` sets the author.

### `list`

```
ticket list [active|open|wip|done|closed|archive|all]
```

Default is `active` (= open + wip). `archive` lists tickets already moved into `tickets/archive/` (done/closed only).

### `show`

```
ticket show <number|filename>
```

Shows a ticket; searches the archive too.

### `set`

```
ticket set <number> <status> ["comment"]
```

Renames the file and appends a journal line. Re-setting the SAME status WITH a comment appends a journal entry only (no rename); the same status WITHOUT a comment is an error. Moving an archived ticket to `open`/`wip` returns it from the archive into work.

### `archive`

```
ticket archive [<number>]
```

Moves done/closed tickets into `tickets/archive/`; no number = all closed, with a number = only the given one.

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
- With `TICKET_LANG=en` plain labels are produced: `## Summary`, `- Status: open`.
- Journal line format: `YYYY-MM-DD HH:MM — статус: X → Y · comment (who)`.
- `Journal` is append-only, written automatically by `new`/`set`.

## Pitfalls

1. 🔴 `ticket new --help` is NOT a help flag — it CREATES a garbage ticket titled `--help` (known bug T-0041). NEVER pass flags as the title; after `new`, check the created file.
2. 🔴 Test/smoke runs ONLY in a sandbox: copy the script to `/tmp` (bash version) or set `TICKETS_DIR` (Go version). NEVER experiment in the live `tickets/` dir (incident T-0023).
3. 🔴 NEVER write secrets (passwords, tokens, keys) into tickets.
4. 🔴 Closing gate: verify the fix FIRST, then IMMEDIATELY `ticket set N done "<comment with evidence>"`. The comment MUST state what was checked: command + result, file:line. `done` without evidence is forbidden; NEVER batch status flips "for later".
5. 🔴 BEFORE working on a ticket, read its `## User comments` section — the user leaves remarks there; the agent NEVER writes there. The agent's working notes go to `## Comments`.
6. Old Russian-header ticket format is unsupported; a one-time sed migration snippet exists — see `reference.md`.

## Workflow quick reference

1. `ticket list` — see open tickets (remind the user at session start).
2. `ticket new "..." -t ... -p ... -d "..."` — file a problem/incident immediately, with file:line in details.
3. `ticket set N <status> "<comment>"` — move statuses; close with evidence only.
4. `ticket archive` — tidy up closed tickets.
5. `ticket show N` — works on archived tickets too.

## Structure

```
ticket-cli/
├── SKILL.md
└── reference.md
```

## See also

- `reference.md` — verbatim `ticket` help output + sed migration snippet for old Russian-header tickets.

## File History & Known Issues

| Date | Issue | Fix Applied | Version |
|------|-------|-------------|---------|
| 2026-09-04 | Initial version | Created workflow skill from `ticket` CLI help | 1.0 |
| 2026-09-04 | Review round 1: bilingual file format missing, type non-canonical, no See also | Added bilingual/EN label forms, TICKET_LANG + dir discovery notes, type: guide, See also | 1.1 |
