---
name: ticket-cli
version: "1.27"
verified_with_cli: "2.1.0"
created: 2026-09-04
modified: 2026-11-27
type: guide
tags: [ticket, tickets, t-nnnn, cli, task-tracker, dogfooding]
description: "Workflow for the `ticket` CLI — file-based task tracker (tickets are plain `T-NNNN-<status>.md` files, no DB). Keywords: ticket, tickets, T-NNNN, CLI task tracker, dogfooding, ticket new/list/show/set/edit/comment/section set/archive/recover/delete/normalize. PROACTIVELY activate for: (1) any ticket CRUD via the `ticket` binary, (2) working on a ticket T-NNNN, (3) closing/archiving tickets, (4) resolving a crash pair (two files, ambiguous number) with `ticket recover`, (5) editing ticket metadata with `ticket edit`, (6) appending working notes with `ticket comment` or replacing a section body with `ticket section set`."
---

# ticket-cli

Workflow for operating the `ticket` CLI. Tickets are plain Markdown files — the files are the single source of truth; there is NO database. Commands below are imperative: MUST / NEVER / ONLY.

## Context

- Tickets are plain files `tickets/T-NNNN-<status>.md`; files are the single source of truth; there is NO database.
- The CLI is `ticket` on your `PATH` (`~/bin/ticket`; `%USERPROFILE%\bin\ticket.exe` on Windows); `-P <project>` flag: see README.
- Numbers are issued atomically via OS locks and are never reused — a soft-deleted ticket keeps its number, too. Status lives in the file name — NEVER rename ticket files manually.
- Ticket directory discovery for the store commands, first match wins: `-C`/`--tickets-dir` → `$TICKETS_DIR` → upward scan from cwd for `tickets/` (see README Installation). `init` is the exception: it honours `-C`/`$TICKETS_DIR` but never scans upward — without them it creates `<cwd>/tickets`, even when a parent already has one.
- i18n: language chain `TICKET_LANG → LC_ALL → LANG`, default **RU**; `TICKET_LANG=en` opts into English help and English-label ticket files.
- 🔴 Ticket CONTENT language: write titles, Details, comments and journal entries in **Russian** (project rule since 2026-11-06). The `-t`/`-p` flag values and file names stay English (`T-NNNN-<status>.md`); the i18n setting above only governs help text and section labels, not what you write.

## Store scopes: project vs general

The CLI has no global store and no merged view: ONE invocation resolves exactly ONE directory (first match wins: `-C` → `$TICKETS_DIR` → upward scan) and works only on that store.

| Scope | Directory | How it is used |
|-------|-----------|----------------|
| Project | the `tickets/` found by the upward scan (`./tickets`) | plain commands from inside the project tree: `ticket new "..." -t BUG -p high`, `ticket list`, `ticket set N done "..."` |
| General | `~/.config/tickets` — a deliberate CONVENTION of this workflow, NOT a CLI default | every call names it explicitly: `ticket -C "$HOME/.config/tickets" ...` |

One-time setup, then both workflows — general tickets always with `-C`, project tickets unchanged:

```bash
ticket -C "$HOME/.config/tickets" init                     # general store, once: store + archive/ + deleted/
ticket -C "$HOME/.config/tickets" new "..." -P general     # a general ticket
ticket -C "$HOME/.config/tickets" list -P general          # list it
ticket -C "$HOME/.config/tickets" set N done "<evidence>"  # close it
ticket init                                                # project store, once (creates ./tickets)
```

Rules that keep the two scopes apart:

- 🔴 Merely creating the directory (`mkdir -p ~/.config/tickets`) enables NOTHING: there is NO general-store auto-discovery and no fallback. Without `-C`/`TICKETS_DIR` the CLI sees only the project tree and outside such a tree exits 1 with `cannot locate tickets dir` — even when `~/.config/tickets` is fully populated.
- 🔴 A `TICKETS_DIR` exported in the shell profile OVERRIDES the project scan for every command in every cwd (the env tier beats the scan tier), so a plain project `ticket list` silently reads the general store instead. Prefer a per-command assignment (`TICKETS_DIR="$HOME/.config/tickets" ticket list`) or no export at all.
- There is NO combined `list`: the general and the project stores are separate worlds — run one command per store to see both sets.
- `-P <project>` is ticket METADATA, NEVER a store selector: `new` sets it, `list` filters on it, `normalize` limits the run to it and `edit --project` rewrites it (`edit` takes no `-P`) — but `ticket list -P general` outside a store still exits 1 with `cannot locate tickets dir`.
- 🔴 Pass `-P general` for general tickets: the implicit project name is `basename(dirname(<store>))`, which for `~/.config/tickets` is `.config` — an unflagged `new` files the ticket under the wrong project (the first-seen project warning names it).
- The path is NEVER tilde-expanded: write `"$HOME/.config/tickets"`, not a quoted `'~/...'` (that resolves to `<cwd>/~/.config/tickets`).

## Documentation verification

`verified_with_cli` (frontmatter) is the EXACT `ticket version` value against which this document was last checked — the version string with the `ticket version` prefix stripped (`2.0.3`). It is NOT a minimum supported version and NOT a compatibility range: it MUST be refreshed on every next verification pass and on every change to the documented behaviour, together with `version` and `modified`. It covers this `SKILL.md` and the INSTALLED `reference.md` (both checked against that one build).

`reference.md` exists in two copies, and they answer different questions. The REPOSITORY copy (`skills/ticket-cli/reference.md`) is a verbatim snapshot of the SOURCE help template (`internal/cli/templates/usage.ru.txt`) and moves with the code, so it may describe a newer build than the one installed. The INSTALLED copy (`~/.agents/skills/ticket-cli/reference.md`) is a verbatim snapshot of the `ticket help` printed by the binary it was captured from — the `PATH` release or a named branch build, stated in its provenance — and records its own `verified_with_cli`. The two snapshots may therefore differ legitimately; never sync one from the other.

## Commands

### Global flags

Two flags are global, and both are accepted ONLY before the command name: a flag-like token after the command name is a subcommand argument and is rejected (e.g. `ticket list --verbose` fails).

`-C <path>` / `-C<path>` / `--tickets-dir <path>` / `--tickets-dir=<path>` — the value is the path to the tickets directory itself (not a parent). Overrides `TICKETS_DIR`. For every store-resolving command (`new`, `list`, `show`, `set`, `edit`, `comment`, `section set`, `recover`, `delete`, `archive`, `normalize`) the target MUST already exist — a missing path is exit 1 with `invalid tickets dir (--tickets-dir): path does not exist: <path>`. The store is NOT resolved by a bare invocation (usage), `help`/`--help`, `version` and `update-check`: a nonexistent target is simply ignored there (exit 0, and `3` for update-check when a release exists). `init` is the one command that accepts and CREATES a missing target, and its value is the store directory itself, not a parent: `ticket -C "$HOME/.config/tickets" init` creates exactly that directory (see `init`); it is also the only command that skips the upward scan — it creates, it does not resolve. An ordinary `-C` on an existing empty directory is accepted too (bootstrap via `new`). The value is never tilde-expanded.

`--verbose` enables the debug log; `TICKET_LOG=<path>` sends the records to that file instead — the file wins over the flag. Log records NEVER reach stdout: stdout stays pure command output.

Record format, destinations, failure behaviour and the complete event list are owned by `docs/configuration.md` (`## Debug log (--verbose / TICKET_LOG)`) — this skill cross-references them and does not repeat them.

### `init`

```text
ticket init
ticket -C <path> init
```

Creates the store itself plus `archive/` and `deleted/` inside it. The store is, first match wins (NO upward scan): `-C`/`--tickets-dir` → `$TICKETS_DIR` → `<cwd>/tickets`. An explicit target is made absolute but NOT existence-checked and NOT symlink-resolved — `init` is exactly the command that creates a missing target — and `tickets` is NEVER appended to it: `ticket -C /tmp/gen init` creates `/tmp/gen`, `/tmp/gen/archive` and `/tmp/gen/deleted`, NOT `/tmp/gen/tickets`; run with no flag or env, it creates `<cwd>/tickets`, `<cwd>/tickets/archive` and `<cwd>/tickets/deleted`. It prints the localized initialized message naming the created store.

Idempotent: a repeat run prints the same message and exits 0. Exit 1 if the store or either subdirectory exists as a non-directory (file/FIFO/broken link) — never overwrites, and a conflict is detected before anything is created, so no partial structure is left behind. It does NOT install the binary.

`-C` MUST precede the command name like every global flag: `ticket init -C <path>` is NOT parsed as a flag — the init does not run at all. It prints the full usage text to stdout (byte-identical to `ticket help`), leaves stderr empty, exits 1 and creates nothing.

### `new`

```text
ticket new "<brief>" [-t BUG|OPS|TD|ENH] [-p low|normal|high] [-d "<details>"] [-w who] [-P <project>]
```

Creates a ticket with status `open` and prints the file path. `-w` sets the author. `-P` overrides the project (default: basename of the tickets-dir parent); a stderr warning is printed when a project name is seen for the first time.

Details input (`-d`): plain text is used literally; `@<file>` reads that file; `@@name` reads the file named `@name` (exactly one `@` is stripped — `@@` is NOT an escape); `-` reads stdin; a bare `@` (no path) is an error. The content is copied byte for byte — no trimming, no newline conversion, no size limit (more than 64 KiB is accepted); an empty file or empty stdin gives an empty `Details` value — the created ticket then shows the Details format block (the same as `-d ""`). A failed read prints a localized message and creates NO ticket.

Details format: a numbered list, one item per line — `N. <what> — <file:line>; <fix>` (RU: `N. <что> — <файл:строка>; <решение>`). A ticket created without `-d` — or with an empty `Details` value, e.g. from an empty file or stdin — gets the same rule as a format block in `Details`.

### `list`

```text
ticket list [active|open|wip|done|closed|archive|deleted|all] [-P <project>] [--type BUG|OPS|TD|ENH] [--priority low|normal|high] [--json]
```

Default is `active` (= open + wip). `archive` lists tickets already moved into `tickets/archive/` (done/closed only). `deleted` lists soft-deleted tickets from `tickets/deleted/`; every OTHER mode — `active`, `open`, `wip`, `done`, `closed`, `archive` and `all` — HIDES them, and `all` keeps its old meaning: the main directory only. `-P` filters by project (exact, case-sensitive); the project column appears when projects in the output differ. `--json` (since v1.2.3, T-0081) prints a JSON array with 6 fields (`number, status, type, priority, title, created`); empty result = `[]`.

`--type` (`BUG`/`OPS`/`TD`/`ENH`) and `--priority` (`low`/`normal`/`high`) filter by the ticket metadata: exact values, case-sensitive. They combine with the status and `-P` filters by AND, so `ticket list open --type BUG --priority high -P proj` needs all four to match.

Each of these flags takes the NEXT token as its value, even when that token looks like a status or another flag: `--type open` is an invalid type, not a status filter. An invalid or empty value is an error (exit 1) — `ticket list --type BUGX` prints `ticket: type must be one of: BUG OPS TD ENH` to stderr. No matches is NOT an error: the plain output prints `No tickets (active --type BUG).` and `--json` prints `[]`, both with exit code 0. `archive` selects the archive as the source and `deleted` selects `tickets/deleted/`; the status filter does not apply to either, but `--type`, `--priority` and `-P` do (AND).

### `show`

```text
ticket show <number|filename> [--section <name>] [--json]
```

Shows a ticket; searches `tickets/`, `tickets/archive/` and `tickets/deleted/`. A duplicate number is exit 2 with every conflicting absolute path on stderr; an exact filename selects one file out of the duplicate. Three mutually exclusive modes: **plain** (no flag) prints the file byte for byte and only WARNS about extra arguments (exit 0 — a lone glued `--section=<name>` counts as one); **`--section <name>`** prints just the section body plus one LF (`summary`, `details`, `user-comments`, `comments`, `journal`, aliases `brief` and `user_comments`; an empty or absent section is exit 0 with an empty stdout, an unknown name exit 1); **`--json`** prints one JSON document with `number, status, type, priority, project, title, summary, created, who, details, user_comments, comments, journal[], unknown, unknown_encoding, warnings[]` — the status comes from the FILE NAME, dates are RFC3339 or `""`, `journal[]` holds objects (`{at, from, to, who, comment, archived}`) instead of markdown lines, the `unknown` tail is lossless (UTF-8 text or base64), parse warnings ride along, diagnostics go to stderr only and the document ends with one LF. In both new modes the flags are validated BEFORE the ticket is read: an unknown, repeated or glued flag, extra positionals, `--section`+`--json` or a missing ticket argument are exit 1 with an empty stdout.

### `set`

```text
ticket set <number> <status> ["comment"]
```

Renames the file and appends a journal line. Re-setting the SAME status WITH a comment appends a journal entry only (no rename); the same status WITHOUT a comment is an error. Moving an archived ticket to `open`/`wip` returns it from the archive into work.

The comment may be a SOURCE, but only when it is ONE argument: `-` is stdin, `@<file>` is the content of that file, `@@name` is the file named `@name` (exactly one `@` is stripped, `@@` is not an escape); any other single argument is used literally. `@` alone as that ONE argument is an error (exit 1). The content is copied byte for byte, except that exactly ONE trailing LF or CRLF is stripped (the newline `echo` or an editor appends). Any other CR or LF inside it — `x\n\n`, a lone trailing CR, an embedded or multi-line payload — is refused (exit 1) and nothing is written, so the ticket name, its bytes and the journal stay unchanged. An empty source yields an empty comment (a same-status `set` then fails as it does without a comment). TWO or more comment arguments are joined with a single space and used literally (no file, no stdin — a flag-like `-x` stays literal; the join strips nothing, not even a trailing newline). Trade-off: a lone `-` or `@...` argument is never a literal comment, and two arguments are not a byte-exact escape (the join inserts a space); `-` on a terminal reads stdin until EOF (Ctrl-D).

`deleted` is terminal and read-only, and `set` NEVER assigns it: naming that status, or addressing a ticket whose file lives in `tickets/deleted/`, is refused (exit 1, file unchanged). There is no undelete — a reopen to `open`/`wip` is not supported for a deleted ticket.

### `edit`

```text
ticket edit <number> [--title <title>] [--type BUG|OPS|TD|ENH] [--priority low|normal|high] [--project <project>]
```

Edits metadata in place: only title/type/priority/project of an existing ticket; the status and the file name stay unchanged. The body changes only where the edit demands it: a default Summary follows the new title, the Journal gains one line per changed field, and every unrelated section (Details, user comments, comments) is preserved as it was. The number comes first, then ONLY long split-form flags — the glued `--title=value` form is NOT recognised and there are no short flags. Order is free, a repeated flag is last-wins; an unknown flag, a missing value or an extra positional argument is exit 1, and so is a call without any field.

Values are validated before anything is written: an invalid type/priority, a whitespace-only title/project, control characters or a leading dash in the title, and CRLF in any value (journal injection) are exit 1 with no write. ANY explicitly passed value that equals the current one rejects the WHOLE operation (exit 1, nothing written) — even when other fields would change. Success is SILENT: exit 0, empty stdout/stderr.

Each changed field appends its OWN journal line `<field>: old → new` in the fixed order title → type → priority → project, under one timestamp and one author; the status transition of the line is the current status (From = To), so values with parentheses round-trip. A title change rewrites the H1 and updates Summary only when Summary was the default (empty or equal to the old title); a custom Summary survives. Archived tickets are edited in place — no reopen, no move; `--project` rewrites only the metadata field: there are no per-project directories and nothing moves. A ticket whose file lives in `tickets/deleted/` is refused (exit 1, nothing written): deleted is terminal, `edit` cannot resurrect it.

🔴 The ticket CONTENT language rule applies to the values you pass: write the new title/details in **Russian** (project rule since 2026-11-06); flag values and file names stay English.

Exit codes: `0` — silent success; `1` — usage/validation error, an explicitly unchanged field (no write), or a ticket whose file lives in `tickets/deleted/` (refused before any write); `2` — ambiguous number, like `show`/`set`.

### `comment`

```text
ticket comment <number> [-] [--file <path>] [--json] [--schema]
```

Appends a text block to `## Comments` — NEVER a replacement; status and metadata stay untouched. The payload source is exactly one of `-` (stdin) or `--file <path>`; no source or both is exit 1 and the terminal is never read. stdin is the primary channel: heredoc on Unix, pipe on Windows. The input is read and decoded BEFORE the lookup — a refused input leaves the file and journal byte-identical.

- 🔴 An empty payload is refused (exit 1). A payload markdown-heading line (1–6 `#` then whitespace or end of line) is refused (exit 1) with its line number.
- `--file` is read byte for byte (trailing CR/LF trimmed); only separate `--flag value` pairs after the number.
- `--json` — the strict single-field object `{"text":"<payload>"}`; unknown fields, invalid JSON or trailing garbage are exit 1, never a plain-text fallback. `--schema` prints the JSON Schema (exit 0, no lookup, no write) and cannot be combined with a source (exit 1).
- Success is silent (exit 0); the journal records `comment: +N lines`. Exit codes: `0` ok; `1` usage/IO/empty/heading/deleted/not found; `2` ambiguous number.
- 🔴 Write the comment text in **Russian** (project rule); NEVER write secrets; NEVER write `## User comments`.

### `section set`

```text
ticket section set <number> <section> [-] [--file <path>] [--json]
```

REPLACES the whole body of an editable section: `summary`, `details`, `comments` (canonical CLI tokens, case-insensitive; `brief` = `summary` alias; surrounding whitespace trimmed, inner spaces not normalised).

- 🔴 Protected sections — `user-comments` («Комментарии от пользователя») and `journal` («Журнал») — and unknown names are refused (exit 1) with the editable list. The agent NEVER writes «Комментарии от пользователя».
- Same payload contract as `comment`: exactly one of `-`/`--file`, strict `--json` object `{"text":"<payload>"}`, heading guard, separate flag pairs, input decoded before the lookup.
- An empty payload CLEARS the section (an empty summary renders as the title) — unlike `comment`, empty is allowed here.
- Success is silent (exit 0); the journal records `section <token>: replaced`. Exit codes: `0` ok; `1` usage/IO/protected/heading/deleted/not found; `2` ambiguous number.

### `recover`

```text
ticket recover <number> <new-file> [--yes]
```

Repairs the pair an interrupted status change leaves behind (`T-NNNN-old.md` + `T-NNNN-new.md`, e.g. `T-0007-open.md` + `T-0007-done.md`): `show`, `set` and `archive` report the number as ambiguous (exit 2) and print both conflicting ABSOLUTE paths. `<new-file>` is the basename from that diagnostic (e.g. `T-0007-done.md`) — it selects the canonical file, and the path is NEVER guessed: the basename is matched against `tickets/`, `tickets/archive/` and `tickets/deleted/` (an identical basename in two dirs is a refusal).

Only the exact crash signature is accepted: the new file must equal the old one plus exactly one trailing journal entry `From=old status → To=new status`, and the old file must LACK that entry. The comparison runs after parse/render (all fields, unknown/RawTemplates included), so any other difference is rejected and the pair is left byte-identical.

What it does NOT do: it deletes ONLY the old file — the new file is never opened for writing, its content and journal stay byte-identical, no `tmp/` and no recovery journal entry are created. Repeating the command when only the new file remains is a conservative no-op with exit 0, and so is a single file that is already named `<new-file>`: recover only checks that the wanted name is on disk, it cannot prove that this is the previously saved file rather than a lone stale one, so exit 0 never proves that a recovery happened. The keep-old choice is refused; roll the status back afterwards with a normal `ticket set <number> <old-status> "reason"`. A crash pair that includes `tickets/deleted/` (an interrupted `delete`: the stale source in the main directory plus `T-NNNN-deleted.md`) is recovered by the same signature rule — keeping the deleted file succeeds, keeping the stale source stays the ordinary keep-old refusal — and it does NOT undelete anything: the ticket stays terminal.

Confirmation: without `--yes` it prints keep/discard and asks yes/no; a non-TTY run without `--yes` exits 1 having done nothing, so automation MUST pass `--yes`. Any error keeps the pair for a retry, but path reporting differs: the ambiguity report (more than two files, also from `show`/`set`/`archive`) prints every conflicting ABSOLUTE path, while a `RecoveryState` refusal prints only the conflicting BASENAMES (same-status names none). Never assume paths appear in every error — pass the bare basename.

🔴 Run `recover` only in a sandbox (`-C /tmp/…` or `TICKETS_DIR`): it deletes a file — NEVER in the live `tickets/`.

### `delete`

```text
ticket delete <number|filename> [--yes] [--force]
```

Soft-deletes a ticket: the file is MOVED into `tickets/deleted/T-NNNN-deleted.md` — body, metadata, unknown sections and the existing journal are preserved, and exactly one journal line records the transition to `deleted`. Success prints the deleted file's path. `<number|filename>` is ONE ref — a number or an exact basename (for example `T-0001-open.md`), never a path; the exact basename is also how one file is picked out of a duplicate number.

Guards: without `--force` only an `open` ticket in the main directory is deletable; `--force` also covers the other main statuses and the archive. Without `--yes` the command prints the candidate path and asks yes/no; a non-TTY run without `--yes` exits 1 having done nothing, so automation MUST pass `--yes`.

Exit codes: `0` — the ticket is now in `deleted/`; `1` — usage error, not-found ref, already deleted, target collision (that name already exists in `deleted/`), refused confirmation or an I/O error — nothing changes; `2` — an ambiguous number or an identical basename in several directories, every conflicting path printed to stderr and nothing changed. A repeat after a successful delete is `1` (already deleted), never a no-op success.

`deleted` is TERMINAL and read-only: `set` (a reopen to `open`/`wip` included) and `edit` refuse such a ticket (exit 1, file unchanged), `set` never assigns the status, and there is no undelete. `archive` ignores deleted tickets; `list deleted` is the only mode that shows them. Numbers are never reused — the deleted file keeps its number.

🔴 `delete` moves a file: run it only in a sandbox (`-C /tmp/…` or `TICKETS_DIR`), NEVER in the live `tickets/`.

### `archive`

```text
ticket archive [<number>]
```

Moves done/closed tickets into `tickets/archive/`; no number = all closed, with a number = only the given one. Deleted tickets are IGNORED: never moved into the archive and never restored from it.

### `normalize`

```text
ticket normalize [-P <project>] [--dry-run]
```

One-shot migration of legacy ticket files to the canonical markdown layout (the blank line before and after every header, one trailing newline, no CRLF). Only files whose bytes differ from the canonical render are rewritten: the file name never changes and the journal stays as it is, and NO migration journal line is appended — that is the whole point, because a plain `ticket set` used to write "статус: X → X" into every legacy file it touched. The file name is authoritative (V19): a status line in the body that disagrees with the name is rendered to the name's status, silently and without a journal entry.

`--dry-run` prints the paths that would be rewritten and writes nothing (it takes no lock and no write probe, so it also works on a read-only `tickets/` directory). `-P` limits the run to one project (exact match against the ticket's own Project field, case-sensitive). Changed paths are printed one per line on stdout; warnings such as `ticket: T-0007-open.md: bash-era metadata labels, file left unchanged` go to stderr.

Guarded — these files are skipped with a warning, never silently rewritten:

- bash-era label lines (`- Статус:`, `- Приоритет:`, `- Создан:`, `- Проект:`; also with a leading indent, a `*`/`+` bullet and a space before the colon): the parser does not know them, so a rewrite would drop the line;
- incomplete metadata (empty type/priority/project, blank title, zero created date): a rewrite would invent a value.

Deleted tickets are normalised IN PLACE as well: the walk order is main → archive → deleted, an unreadable or invalid deleted file is skipped with the same warning an archived one gets, and the file name stays authoritative. `--dry-run` writes nothing, deleted files included.

🔴 Run it in a sandbox first (`--dry-run` on a copy: `rm -rf /tmp/corpus && cp -r tickets /tmp/corpus && ticket -C /tmp/corpus normalize --dry-run`) — the command rewrites files in place, and `-C` is a global flag: it MUST precede the command name. Exit code 2 is never returned: duplicate numbers are reported like `list` does and both files are migrated.

### `update-check`

```text
ticket update-check
```

Explicit check for a newer release on GitHub — never automatic, never blocks other commands. Per-user cache (24 h), 1 h backoff after a failed check. Exit `0` = up to date / no releases / dev build, `3` = update available, `1` = check failed.

## Exit codes

- `0` — success.
- `1` — error (bad arguments, ticket not found, collision, input or store error, failed update check).
- `2` — ambiguous ticket number: the number belongs to several ticket files; the number and the conflicting files are printed to stderr. Duplicates inside `tickets/deleted/` follow the same split: `list` (any mode) warns and exits 0, `show` exits 2 with every conflicting path, and `set`/`edit`/`comment`/`section` refuse a deleted ticket with exit 1.
- `3` — update available: the explicit `ticket update-check` found a newer release on GitHub.

`ticket normalize` uses only `0` and `1`: it never reports an ambiguous number (both files are migrated and the duplicate is warned about) and never returns 2.

`ticket edit` uses `0`/`1`/`2`: `0` — silent success; `1` — a usage/validation error or an explicitly unchanged field (the whole operation is refused, nothing is written); `2` — ambiguous number (both conflicting absolute paths on stderr).

`ticket comment` and `ticket section set` use `0`/`1`/`2`: `0` — silent success (parse warnings on stderr); `1` — usage, I/O, a protected/unknown section, an empty comment payload, a heading-guard refusal, a deleted or missing ticket; `2` — ambiguous number (every conflicting absolute path on stderr). A missing ticket is `1` — there is no separate not-found code.

`ticket recover` reuses these codes: `0` also covers a conservative no-op (a repeat after a successful recovery, or a single file already named `<new-file>`) and never proves that a recovery happened; `1` covers a usage error, a refused confirmation, a missing file, an I/O error (archive scan included), the keep-old choice and a selection that is not the crash pair; `2` means the number cannot be recovered automatically — more than two files, diverging content or equal statuses. A divergent pair is `2`, never `1`.

## Status lifecycle

`open` → `wip` → `done` / `closed`; `deleted` — terminal.

`deleted` is assigned ONLY by `ticket delete` (an ordinary `set` never produces it). Once a ticket is deleted, `set` (including any reopen) and `edit` refuse it and there is no undelete; `recover` only closes crash pairs and never resurrects a deleted ticket. `archive` ignores deleted tickets; `list deleted` is the only mode that shows them.

| Status | Meaning |
|--------|---------|
| `open` | new |
| `wip` | in progress |
| `done` | fixed |
| `closed` | rejected / duplicate |
| `deleted` | soft-deleted into `tickets/deleted/` — terminal, read-only |

Types: `BUG` — breakage; `OPS` — incident/maintenance; `TD` — tech debt; `ENH` — enhancement.

## Ticket file format

Labels are bilingual under the RU default locale:

- Header: `# T-NNNN · TYPE: title`.
- Bullets: `- Status (Статус):`, `- Priority (Приоритет):`, `- Created (Создан): … · by (кем): …`, `- Project (Проект):`.
- Sections: `## Summary (Кратко)`, `## Details (Подробности)`, `## User comments (Комментарии от пользователя)`, `## Comments (Комментарии)`, `## Journal (Журнал)`.
- `Details` — specifics as a numbered list, one item per line: `N. <what> — <file:line>; <fix>`.
- With `TICKET_LANG=en` plain labels are produced: `## Summary`, `- Status: open`.
- Journal line format: `YYYY-MM-DD HH:MM — статус: X → Y · comment (who)`.
- `Journal` is append-only, written automatically by `new`/`set`/`edit`/`comment`/`section set`.

## Pitfalls

1. `ticket new --help` now prints help (bug T-0041 fixed; verified on v1.2.3 — no garbage ticket is created). Still: after `new`, check the created file before moving on.
2. 🔴 Test/smoke runs ONLY in a sandbox: set `TICKETS_DIR` or pass `-C /tmp/…`. NEVER experiment in the live `tickets/` dir (incident T-0023).
3. 🔴 NEVER write secrets (passwords, tokens, keys) into tickets.
4. 🔴 Closing gate: verify the fix FIRST, then IMMEDIATELY `ticket set N done "<comment with evidence>"`. The comment MUST state what was checked: command + result, file:line. `done` without evidence is forbidden; NEVER batch status flips "for later".
5. 🔴 BEFORE working on a ticket, read its `## User comments` section — the user leaves remarks there; the agent NEVER writes there. The agent's working notes go to `## Comments` — append them with `ticket comment N -` (stdin); `section set comments` REPLACES the whole section, never use it for a note.
6. Tickets with Russian-only headers (`## Кратко`, `- Статус:`) are NOT parsed: those fields stay empty and the next `set` rewrites the file in the current format without them — fix such files by hand. There is no migration command. Files in the pre-MD022 format (current format without the blank line after the heading) are read and normalised on the next `set`.
7. `TICKET_LOCK_TIMEOUT` is a holder-STALL budget, NOT the waiter's wall clock: `ticket` fails when the lock holder makes no progress for the configured value, while a holder that keeps working extends the wait — the holder refreshes the `.lock` mtime about every 250 ms and the waiter extends its deadline on every observed progress, capped at 12× the configured value, after which a heartbeating-but-stuck holder fails the waiter exactly as without progress tracking (`internal/lock/lock_progress.go`, T-0226). This holder-stall behaviour is what the build this skill was verified against ships (`2.0.3`); the deadline error reads `still held by another process ... without holder progress`. The value is a Go duration that must carry a unit (`30s`, `2m`, `500ms`) — the only unit-less value accepted is the literal `0`, and any value `<= 0` means a single lock attempt without waiting — and an empty or invalid value silently falls back to the default — a typo never errors out. Keep the value above the filesystem mtime granularity (FAT 2 s, HFS+ 1 s) and above the ~250 ms refresh interval; raise it for parallel agent runs or CI. The default itself is owned by the code (`defaultLockTimeout` in `internal/store`), so this skill does not repeat the number.
8. `-C`/`--tickets-dir` and `--verbose` MUST come BEFORE the command name; after it the token is a subcommand argument and the command fails — except after `help`/`version`, where it is ignored — see `docs/configuration.md` for the exact messages.
9. 🔴 `tickets/deleted/` is NOT `tickets/archive/`: deleted is terminal and read-only, `list deleted` is the only mode that shows it, `all` hides it, `archive` ignores it, and there is NO undelete — `set` (any reopen included) and `edit` refuse such a ticket and change nothing.
10. Numbers are never reused: a deleted ticket keeps its number in `tickets/deleted/T-NNNN-deleted.md`, and the next `new` skips it. Never renumber or hand-copy a ticket file.

## Workflow quick reference

1. `ticket list` — see open tickets (remind the user at session start).
2. `ticket new "..." -t ... -p ... -d "..."` — file a problem/incident immediately, with file:line in details; long details go through `-d @file` or `-d -`.
3. `ticket set N <status> "<comment>"` — move statuses; close with evidence only.
4. `ticket comment N -` — append working notes/evidence to `## Comments` (heredoc on Unix, pipe on Windows).
5. `ticket archive` — tidy up closed tickets.
6. `ticket show N` — works on archived and deleted tickets too.
7. `ticket delete N --yes` — soft-delete into `tickets/deleted/` (terminal, read-only; never a substitute for `set done`).

## Structure

```text
ticket-cli/
├── SKILL.md
└── reference.md
```

Tickets directory layout: `tickets/` (main) + `tickets/archive/` (closed) + `tickets/deleted/` (soft-deleted, terminal).

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
| 2026-11-11 | T-0162: a lone stale file whose name equals the wanted `keepBase` yields a conservative no-op (exit 0) without proof that it is the previously saved file | Documented the ambiguity — exit 0 covers a conservative no-op (repeat after a successful recovery, or a single file already named `<new-file>`) and never proves that a recovery happened | 1.11 |
| 2026-11-11 | T-0125: `ticket normalize` was undocumented; the new section had overwritten `ticket archive`, and the sandbox command put `-C` after the command | Restored the `archive` section, added `normalize` after it, fixed the sandbox command to `ticket -C /tmp/corpus normalize --dry-run`, and stated that the file name is authoritative for a disagreeing body status line | 1.12 |
| 2026-11-11 | T-0173: `ticket update-check` was undocumented | Added the `update-check` section and exit code `3` | 1.13 |
| 2026-11-16 | T-0188: the `list` filters `--type`/`--priority` were undocumented | Documented the new `list` syntax and the filter semantics (exact case-sensitive values, AND with the status/`-P` filters, next-token value rule, invalid value = exit 1, empty result = `No tickets`/`[]` with exit 0, archive source) | 1.14 |
| 2026-11-16 | T-0187 (GitHub #20): `ticket edit` was a non-goal and undocumented | Added the `edit` section (long-only split-form flags, per-field journal order title→type→priority→project, whole-operation no-op on explicitly unchanged values, silent success, archive edited in place, `--project` is metadata-only) and the edit exit-code notes | 1.15 |
| 2026-11-16 | Merge of PR #23 (list filters, T-0188) and PR #24 (`ticket edit`, T-0187) | Rebase resolution: both features kept — `list --type`/`--priority` and the `edit` section coexist; version bumped past both parents | 1.16 |
| 2026-11-16 | T-0189 W3 (GitHub #22): `ticket delete`, the `list deleted` mode and the `deleted/` integration were undocumented | Added the `delete` section (soft-delete into `tickets/deleted/`, `--yes`/`--force` guards, exit 0/1/2, repeat = exit 1), the `deleted` list mode and the hiding rule (`all` stays main-only), the `deleted/` notes for `show`/`set`/`edit`/`archive`/`recover`/`normalize`, the `deleted` lifecycle row, pitfalls 9-10 and the tickets-dir layout; version bumped to 1.17 | 1.17 |
| 2026-11-21 | T-0215 (GitHub #35): the `set` comment source rules (single-argument `-`/`@file`/`@@name`, multi-argument literal join, CR/LF refusal) were undocumented | Documented the comment-source contract in the `set` section and the workflow reference: one argument = a source, two or more = literal space-join, byte-exact copy, CR/LF refusal without mutation, and the trade-off (a lone `-`/`@...` is never literal); version bumped to 1.18 | 1.18 |
| 2026-11-24 | Skill drifted from the CLI at v2.0.2: `init` was described as creating only `tickets/`+`archive/` and as never honouring `-C`/`$TICKETS_DIR`; the `set` comment rule claimed that ANY CR/LF is refused | Re-synced against the binary built from HEAD: `init` creates `tickets/`, `tickets/archive/` AND `tickets/deleted/`, resolves `-C` → `$TICKETS_DIR` → `<cwd>/tickets` (no upward scan), and creates nothing when a subdir conflicts; a single trailing LF/CRLF in a `set` comment source is stripped and accepted, while `x\n\n`, a lone CR and multi-line payloads stay refused (T-0223, T-0225); version bumped to 1.19 | 1.19 |
| 2026-11-24 | T-0234: the skill versioned only itself and did not record which CLI build it was checked against; the installed copy lagged behind (v1.14 vs v1.18); the `init` section omitted `tickets/deleted/` and the subdirectory conflict probe | Added `verified_with_cli: "2.0.0-8-g07701fe"` plus the Documentation verification rule (exact checked version, refreshed on every pass, not a compatibility claim); corrected the `init` section to the current CLI (creates `deleted/` too, conflicts are probed before any creation) and pitfall 7 (unit-less `0` is accepted; `<= 0` = a single attempt without waiting, per `docs/configuration.md`); re-verified against `ticket help` of v2.0.0-8-g07701fe and synced the installed copy; version bumped to 1.19 | 1.19 |
| 2026-11-24 | T-0235: the skill claimed `-C` does not work with `init` and described `init` as always creating `<cwd>/tickets` | Verified against v2.0.0-8-g07701fe: `init` honours `-C`/`$TICKETS_DIR` and creates that directory itself (plus `archive/` and `deleted/`, no `tickets` suffix appended; an explicit target is absolute but not existence-checked or symlink-resolved); removed the "except `init`" claim from Global flags, added the existence requirement for ordinary commands and the `-C`-must-precede-`init` rule; post-review: pitfall 7 now states only the installed binary's lock-wait semantics (at T-0235 the source-only extension was omitted from pitfall 7; the 1.21 reconcile re-added it with explicit source-only scope), the Documentation verification section scopes `verified_with_cli` to `SKILL.md` plus the installed `reference.md` and explains why the repository snapshot may differ, and the installed snapshot was re-captured from `ticket help` | 1.20 |
| 2026-11-24 | T-0236: the general (`~/.config/tickets`) vs project (`./tickets`) split was undocumented, including the global `TICKETS_DIR` override and the `-P` vs `-C` confusion | Added "Store scopes: project vs general" (no auto-discovery, no merged list, one store per invocation, explicit `-C` or per-command `TICKETS_DIR`, `-P` is metadata only, `-P general` recommended because the implicit name is `.config`, no tilde expansion) and noted that `-P` is used by `new`, `list` and `normalize` | 1.20 |
| 2026-11-24 | The local 1.20 series (T-0234/35/36: store scopes, `verified_with_cli`, `init`/`-C` and lock semantics, installed reference) and the remote 1.19 series (T-0223/25/26: `init` corrections, one trailing LF/CRLF allowed in a `set` comment source, lock line reworded as a holder-stall budget) were written on parallel branches | Reconciled by a fast-forward merge of `origin/main` (`868d2cf`) with the local file stashed: every local addition and every remote correction is kept — `init` honours `-C`/`$TICKETS_DIR` but never scans upward, exactly one trailing LF/CRLF is stripped while every other CR/LF stays refused, and the source help snapshot carries the holder-stall wording; re-verified against the `PATH` binary (`2.0.0-8-g07701fe`): `init -C` creates the target itself (no `tickets` suffix, `archive/`+`deleted/`, nothing created on a subdir conflict), trailing-LF/CRLF comments accepted while `x\n\n` and a lone CR are refused, multi-argument join strips nothing, `TICKET_LOCK_TIMEOUT=0` is a single attempt and unit-less/`bogus` values fall back to 5s — the holder-progress extension is source-only (T-0226, newer than the installed build); repo `reference.md` re-diffed against `internal/cli/templates/usage.ru.txt` and the installed copy against `ticket help` — both bodies match; installed `SKILL.md` synced byte-identical | 1.21 |
| 2026-11-24 | The candidate build for the next release is `2.0.3`, and pitfall 7 still posed the holder-stall lock budget as source-only versus `2.0.0-8-g07701fe`, although that behaviour shipped in v2.0.1 | Re-verified against the `2.0.3` candidate built from `d30fe97` (normal clone, `-ldflags -X ticket/internal/cli.version=2.0.3`, `ticket version` = 2.0.3): holder-stall semantics confirmed from `internal/lock/lock_progress.go`/`lock.go` (heartbeat ~250 ms, patience cap 12×, deadline error `... without holder progress`) and from `internal/lock` tests `TestAcquireLockedExtendsOnHolderProgress`/`...TimesOutWithoutProgress`/`...ProgressPatienceCap` (all pass); `ticket help` states the same rule. Sandbox probes: a single trailing LF or CRLF in a `set` comment source is accepted and stripped, while `x\n\n`, a lone CR and an embedded LF are refused (exit 1, bytes unchanged); two arguments stay a literal space-join; repo `reference.md` body still equals `internal/cli/templates/usage.ru.txt` exactly. Pitfall 7 rewritten without the source-only/binary split, `verified_with_cli` set to `2.0.3`; version bumped to 1.22 | 1.22 |
| 2026-11-25 | The installed copy was reverted to the `2.0.3` PATH-binary snapshot (the newer `show` section/JSON modes of T-0241 are newer than the release) without a File History entry, while the repo copy `skills/ticket-cli/` documents those new modes | Appended this row to record the revert: the installed `SKILL.md` and `reference.md` describe only `2.0.3` behaviour, `verified_with_cli` stays `2.0.3`, and the new `show` modes are documented in the repo copy only | 1.23 |
| 2026-11-25 | T-0240: the text-write commands `ticket comment` and `ticket section set` were undocumented | Added `comment`/`section set` subsections (payload source contract, strict `--json`/`--schema`, protected sections, heading guard, exit codes 0/1/2), the store-command list, an exit-code paragraph, Journal/pitfall 5/quick-reference updates; verified against the build `2.0.3-7-g240d10b` (branch `feat/t-0240-0241-cli-sections-json`, `ticket help` body byte-identical to the RU/EN templates) | 1.24 |
| 2026-11-25 | T-0240 review pass 1: the `comment` bullet wrongly refused whitespace-only payloads and the `--file` note still said trailing-LF-only | Corrected to "An empty payload is refused" (a whitespace-only payload is written verbatim) and "trailing CR/LF trimmed"; re-verified against the binary built from `feat/t-0240-0241-cli-sections-json` (`2.0.3-8-g3615465`) | 1.25 |
| 2026-11-25 | T-0240 review pass 2: the frontmatter still cited `2.0.3-8-g3615465` although the text carries the `1391e13` edits, and the installed-copy policy named only the PATH binary | Re-verified SKILL.md and the installed reference.md against the binary built from branch `feat/t-0240-0241-cli-sections-json` at `3f7883f` (`2.0.3-10-g3f7883f`): `ticket help` body byte-identical to the RU template, unknown-fields refusal didactic (fields + format + example); the policy now names the PATH release or a recorded branch build as the snapshot source; `verified_with_cli` refreshed | 1.26 |
| 2026-11-27 | Release v2.1.0 published (tag on commit `0bd2ce7`); the skill still cited the pre-release branch build `2.0.3-10-g3f7883f` | Re-verified every documented behaviour against the published binary `ticket version 2.1.0` (`~/bin/ticket`) in a `/tmp` sandbox: `comment`/`section set`/`show --section`/`show --json` syntax and exit codes, the payload rules (empty comment refused, empty section cleared, heading guard, strict `--json`, `--schema`), the deleted-ticket refusals (`set`/`edit`/`comment`/`section` exit 1, `list deleted` the only showing mode) and the `TICKET_LOCK_TIMEOUT` fallbacks — all match the text, no wording changes; the actual `ticket help` body is byte-identical to the RU/EN templates at `0bd2ce7`; `verified_with_cli` set to `2.1.0` | 1.27 |
