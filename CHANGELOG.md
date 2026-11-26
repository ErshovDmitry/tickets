# Changelog

This project follows Keep a Changelog 1.1.0 and its versions follow Semantic Versioning.
This file is English-only by design: README.ru.md translates README.md, while a single changelog keeps the release history in one place.

## [Unreleased]

## [2.1.0] - 2026-11-27

### Added

- `ticket comment <number> [-] [--file <path>] [--json] [--schema]`: appends a text block to the `Comments` section — never a replacement. Exactly one blank line separates the new block from the old body, and the journal records the append (`comment: +N lines`, who, timestamp). The payload source is exactly one of `-` (stdin) or `--file <path>`; no source or both is a usage error (exit 1) and the terminal is never read. The payload is read and decoded BEFORE the ticket lookup, so a refused input leaves the ticket file and the journal byte-identical; the payload is taken verbatim except that trailing CR/LF is trimmed (an empty file or an empty stdin is an empty payload). An empty payload is refused (exit 1), and a payload line that is a markdown heading (1–6 `#` then whitespace or end of line) is refused (exit 1) with its line number. `--json` takes the payload as the strict single-field object `{"text":"<payload>"}`: unknown fields are exit 1 with the field list, invalid JSON or trailing garbage is exit 1 with the error position, the expected format and a ready example, and there is never a plain-text fallback; the reported byte position is the first byte of the offending value in the ORIGINAL payload document, and the refused write changes nothing. `--schema` prints that JSON Schema to stdout (exit 0, no lookup, no write) and cannot be combined with a source (exit 1). Success is silent (exit 0), parse warnings go to stderr, a ticket in `tickets/deleted/` is refused (exit 1) and an ambiguous number is exit 2 with every conflicting path (T-0240, T-0244, T-0246).
- `ticket section set <number> <section> [-] [--file <path>] [--json]`: replaces the whole body of one editable section. The section name is a canonical CLI token — `summary`, `details` and `comments` are editable, `user-comments` and `journal` are protected and an unknown name is refused (exit 1) with the list of editable names; the name is case-insensitive, surrounding whitespace is trimmed and inner spaces are not normalised, and the aliases `brief` (= `summary`) and `user_comments` (= `user-comments`) are accepted. An empty payload clears the section (an empty summary renders as the title). The payload source, the `--json` contract, the refusal rules and the exit codes are exactly those of `comment` (T-0240, T-0244, T-0246).
- `ticket show <number|filename> --section <name>`: prints only the body of one section — no `##` header line — followed by exactly one LF. The canonical names are `summary`, `details`, `user-comments`, `comments` and `journal`, the aliases are `brief` (= `summary`) and `user_comments` (= `user-comments`), and the name is case-insensitive with surrounding whitespace trimmed. A known but empty section, or a section an older ticket does not carry at all, is a success (exit 0, empty stdout); an unknown name is exit 1 with the canonical names listed. The body is language independent — `## Summary`, `## Summary (Кратко)` and `## Summary (<free suffix>)` select the same text — and `summary` is the Summary body, not the title (T-0241).
- `ticket show <number|filename> --json`: prints ONE JSON document on stdout (every diagnostic goes to stderr) with the stable keys `number`, `status`, `type`, `priority`, `project`, `title`, `summary`, `created`, `who`, `details`, `user_comments`, `comments`, `journal`, `unknown`, `unknown_encoding` and `warnings`; every key is always present (an empty body is `""`, an empty journal is `[]`), `status` comes from the file name and never from the body's `- Status:` line, and the manual tail after the journal is preserved losslessly — as text with `"utf-8"` or as base64 with `"base64"`. The `journal` is an array of `{at, from, to, who, comment, archived}` entries, `created` and `at` are RFC3339 (`""` for an unparsable timestamp), and `warnings` is an array of `"<file name>: <error>"` strings (T-0241).
- `ticket show` has three mutually exclusive output modes — the plain one (the ticket file byte for byte on stdout, unchanged), `--section` and `--json` — and a mode is entered only by a bare `--section` or `--json` token (`--section` takes the section name as the next separate argument): the flags are validated BEFORE the ticket is read, so an unknown flag, a repeated flag, a glued `--section=<name>`, extra positional arguments or `--section` together with `--json` are exit 1 with an empty stdout, while the plain form keeps the previous extra-argument warning on stderr (exit 0) — a glued `--section=<name>` alone never selects a mode (it stays on the plain path with that warning, exit 0), and next to a mode token it is an unknown flag (exit 1, empty stdout) (T-0241).

### Changed

- The help text and the docs (`docs/usage.md`, `docs/ru/usage.md`) now describe the two new write commands and the two new `show` modes, the documented exit code 2 (an ambiguous ticket number) now lists `comment` and `section` among the commands that print every conflicting path, and the English and Russian READMEs show the new commands and the new `show` signature in their quick-start lists (T-0240, T-0241, T-0242, T-0247).
- The shipped repo copies of the `ticket-cli` skill (`skills/ticket-cli/SKILL.md` and `skills/ticket-cli/reference.md`) are updated as well: `SKILL.md` documents the new `ticket comment` and `ticket section set` commands and the two new `show` modes, and `reference.md` shows the new `ticket show <number|filename> [--section <name>] [--json]` signature (T-0240, T-0241).
- The Windows smoke-check inventory in the release runbooks (`docs/releasing.md`, `docs/ru/releasing.md`) is synchronized with the smoke script: the former summary of eight steps is replaced by the list of the 11 PASS checks (several steps contribute more than one check, including a repeated `init` and an `init` blocked by a file named `tickets`) (T-0239).

### Fixed

- Rewriting a ticket no longer normalizes the line endings of the section bodies it leaves untouched: `ticket set`, `ticket edit`, `ticket archive` and the new write commands keep the original bytes of an unchanged `Summary`, `Details`, `User comments` or `Comments` body (an internal CRLF stays CRLF) instead of rewriting it to LF, and a body is re-rendered only when its content actually changes — `ticket normalize` still canonicalizes the file to the LF form (T-0245).

## [2.0.3] - 2026-11-24

### Fixed

- The help text and the docs no longer claim that a `set` comment refuses every CR or LF. A resolved single-argument comment (`-` for stdin, `@<file>` for file content) is copied byte for byte and exactly one trailing LF or CRLF is stripped — so a file written by `echo` is accepted; any CR or LF left after that strip (a second trailing LF in `x\n\n`, a lone trailing CR in `x\r`, an embedded or multiline break) is still refused with exit 1 and nothing is written. Corrected in the RU and EN templates, in the mirrored help of `docs/usage.md` and `docs/ru/usage.md`, in the `set` sections of both scenarios docs and in the shipped `skills/ticket-cli/reference.md`; the help regression tests pin the new wording in both languages and fail on the retired one. The multi-argument literal join is unchanged (T-0237).

## [2.0.2] - 2026-11-23

### Changed

- The README files (English and Russian) publish the release OpenPGP key: fingerprint `0CAF1003F44DD0E4990F4EFF6AB4D0E6AA282FC9`, keyserver `hkps://keys.openpgp.org`, the user ID and the `gpg --recv-keys` / `gpg --fingerprint` verification commands, stating that this key signs the release tag and the detached `SHA256SUMS.txt` signature while commits stay SSH-signed with a different key (T-0229).
- The CI triage docs (English and Russian) describe `ci-retry` stage 2 as already enabled instead of planned: capacity-cancelled jobs of the `CI` workflow are rerun automatically on the `workflow_run` event (`DRY_RUN_DEFAULT="false"`), a manual `workflow_dispatch` run is a dry-run report unless `dry_run=false` is passed explicitly, and jobs with conclusion `failure` or `timed_out` are never retried; the `rerun%` caveat is reworded to match (T-0228).

### Fixed

- `ticket new` now inserts the missing MD022/MD032 blank lines around the supported block shapes inside a non-empty `Details` payload: ATX headings (1–6 `#`, up to three leading spaces) and column-0 list blocks (a `-`, `+`, `*`, `N.` or `N)` marker followed by a space or tab). The pass is whitespace-only and idempotent — every non-whitespace byte of the resolved payload is written unchanged — while other shapes and adjacency cases (nested items, indented markers, thematic breaks, non-column-0 blocks) are deliberately left alone and carry no promise. Existing files are not migrated: `ticket normalize` still copies the body verbatim and has no `--prose` pass. The same change documents the pass in the English and Russian READMEs and in `docs/usage.md`, including the supported-shape limits and the no-migration rule (T-0230, issue #56, PR #59).

## [2.0.1] - 2026-11-22

### Changed

- The English and Russian release runbooks (`docs/releasing.md`, `docs/ru/releasing.md`) are restored with the full signed-release flow: annotated GPG-signed release tag, six cross-platform `CGO_ENABLED=0` assets, `SHA256SUMS.txt` with a detached armored signature, and a fresh-download verification procedure (T-0227).

### Fixed

- `ticket init` now honors the global `-C`/`--tickets-dir` flag and the `TICKETS_DIR` environment variable: it initializes the explicitly selected tickets directory instead of always creating `<current directory>/tickets`, and `ticket help` documents the new behavior (T-0223).
- `ticket set` comment sources (`@file`, stdin `-`) now accept exactly one trailing LF or CRLF — the newline an editor or `echo` typically appends — and strip it; embedded or multiple newlines (for example `x\n\n`), a lone trailing CR and other multi-line payloads are still rejected unchanged (a clarification of the T-0215 source rules) (T-0225).
- `TICKET_LOCK_TIMEOUT` is now a holder-stall budget instead of a wall-clock wait: while the holder keeps making progress (it refreshes the lock file), the waiter's deadline is extended on every observed progress, capped at 12× the configured timeout, so `ticket` no longer fails spuriously with "still held by another process" on slow or oversubscribed machines when the peer is healthy (T-0226). This also fixes the parallel `ticket new`/`ticket set` runs on Windows shared runners (T-0226).

## [2.0.0] - 2026-11-21

### Added

- `ticket list --type BUG|OPS|TD|ENH` and `ticket list --priority low|normal|high`: exact, case-sensitive filters that combine with the status and `-P` filters by AND. Each flag takes the next token as its value, a repeated flag keeps the last value, and an invalid value is exit 1 (`ticket: type must be one of: BUG OPS TD ENH`); no match is not an error (exit 0, `[]` with `--json`). The filters apply to `archive` and to the new `deleted` mode as well (T-0188).
- `ticket edit <number> [--title <text>] [--type T] [--priority P] [--project <name>]`: in-place metadata edit. Only split long-flag pairs are accepted, values are validated before any write, and each changed field appends its own `<field>: old → new` journal line under one timestamp. An explicitly unchanged value rejects the whole edit (exit 1), and an archived ticket is edited in place without a reopen. Success is silent (T-0187).
- `ticket delete <number|filename> [--yes] [--force]`: soft delete into `tickets/deleted/`, preserving the body, metadata and journal and appending exactly one transition journal line. Without `--force` only an open ticket in the main directory is deletable; without `--yes` a terminal confirmation is required, and a non-TTY run refuses without changing anything. The single ref (a number or an exact base name) resolves across main, archive and deleted: an ambiguous ref is exit 2 with every conflicting path, while a repeat delete, a collision, a not-found ref or an already deleted file is exit 1 (T-0189).
- The `deleted` status and the `tickets/deleted/` directory: `ticket init` creates it, `ticket list deleted` is the only mode that shows such tickets (every other mode, `all` included, hides them), `ticket show` searches it, `ticket recover` resolves crash pairs involving it, `ticket normalize` rewrites deleted files in place, and the first-use project warning counts projects seen only in `tickets/deleted/` (T-0189).
- `deleted` is terminal and read-only: `ticket set` (a reopen included) and `ticket edit` refuse a ticket whose file lives in `tickets/deleted/` before any write, with a localized message and exit 1; a number carried by both a deleted and a live file gets the ambiguity report (exit 2); `ticket archive` neither moves nor restores deleted tickets. There is no undelete (T-0189).
- `ticket set` reads the comment from a file or stdin when the comment is given as ONE argument: `-` is stdin, `@<file>` is the content of that file, `@@name` is the file named `@name` (exactly one `@` is stripped). The content is copied byte for byte; a bare `@` as that ONE argument is refused, and a CR or LF inside the content is refused before anything is written — the ticket name, its bytes and the journal stay unchanged (T-0215).
- CI: a weekly CI metrics workflow (Monday 03:00 UTC) runs `scripts/ci-metrics.sh --days 30` and reports queue and cancellation metrics in the job summary, with an ALARM as a red run; the triage procedure lives in `docs/ci-triage.md` (T-0203, T-0209).
- CI: a `ci-retry` workflow re-runs CI jobs cancelled by hosted-runner capacity before any runner was acquired. Its job listing is fail-closed: a non-numeric, missing or over-one-page `total_count` fails the step instead of silently ignoring jobs (T-0202, T-0204, T-0206).
- CI: a dedicated `Security audit` job runs zizmor v1.32.0 pinned by release-asset SHA-256 with offline Sigstore attestation verification and audits all workflows; accepted findings are recorded in `docs/security-accepted-risks.md` with narrow inline ignores, and no rule or severity class is disabled (T-0207, T-0212).

### Changed

- BREAKING: `ticket set` with ONE comment argument treats that argument as a SOURCE, so a comment that is exactly `-` or that starts with `@` is no longer written literally (T-0215). A failed read — a bare `@` as that ONE argument, a missing file, a failed stdin read — is an error: the command exits 1 and the ticket name, its bytes and the journal stay unchanged. TWO or more comment arguments are unchanged: they are joined with a single space and used literally, and neither a file nor stdin is read (T-0215).
- The read-only ticket store refusal is localized: every mutating command now prints the translated message instead of the double-prefixed `ticket: store: read-only` line (T-0192).
- CI build matrix legs no longer cancel each other: `strategy.fail-fast` is disabled, so one failing platform does not discard the results of the other legs in the same cycle (T-0196).
- Agent worktrees are documented outside `$TMPDIR` (convention `/nob/dmitry/.wt/<ticket>`), and `.gitignore` ignores `**/.pi-lens-probe-home/` so third-party probe logs cannot pollute `git status` or a commit (T-0200).

### Fixed

- `ticket recover` no longer answers a flag in the `<number>`/`<new-file>` slot with the `set`-specific "unknown flag for set command" text; it uses the command-agnostic message (T-0191).
- The RU `ticket edit` "value is already set" message no longer wraps the value in guillemets on top of `%q` (T-0190).

## [1.2.6] - 2026-11-16

### Changed

- Internal hardening only, no user-visible CLI changes: expanded test coverage (lock retry/EINTR bounds, store empty-file handling, domain i18n suffixes), a new lint guard against bare `fmt.Print`, and a behavior-preserving domain refactor.

## [1.2.5] - 2026-11-12

### Added

- `ticket update-check`: explicit check for a newer release on GitHub — never automatic, never blocks other commands. Per-user cache (24 h), 1 h backoff after a failed check; exit `0` = up to date / no releases / dev build, `3` = update available, `1` = check failed (T-0173).
- Exit code `2` for an ambiguous ticket number: one number owned by several ticket files; the number and the conflicting paths are printed to stderr (T-0157).
- The exit codes are documented in `ticket help`, in the README files and in the docs (T-0158).
- `ticket normalize [-P <project>] [--dry-run]`: one-shot migration of legacy ticket files to the canonical markdown layout. Only files whose bytes differ from the canonical render are rewritten; the file name, its status and the journal stay as they are — no migration journal line is appended, which is what a plain `ticket set` used to do on every legacy file. `--dry-run` prints the paths that would be rewritten and touches nothing (it also works on a read-only tickets directory); `-P` limits the run to one project. Files the command cannot reproduce byte for byte are skipped with a warning instead of being silently rewritten: bash-era label lines (`- Статус: …`) and tickets with incomplete metadata. MD022/MD032 findings in the generated markup drop to 0 after the run; the remaining findings sit in preserved prose (T-0125).
- `ticket recover <number> <new-file> [--yes]`: resolves the two-file pair (`T-NNNN-old.md` + `T-NNNN-new.md`) an interrupted status change leaves behind. The command accepts only the exact crash signature — the new file equals the old one plus exactly one trailing journal entry `From=old status → To=new status` — never rewrites ticket content and never adds a recovery journal entry. It works for `tickets/` and `tickets/archive/` pairs, asks for a yes/no confirmation (`--yes` for automation; a non-TTY run without `--yes` exits 1 having done nothing), is idempotent on repeat, refuses the keep-old choice, and returns exit code 2 when the pair cannot be recovered automatically; roll the status back afterwards with a normal `ticket set` (T-0159).

## [1.2.4] - 2026-11-06

### Added

- `--verbose` and the `TICKET_LOG` environment variable: a structured event log for store and CLI operations.
- Numbered Details format in `--help`, so a ticket detail list reads as one numbered item per line.
- Ticket details can be read from a source: `-d @file`, `-d @@name` and `-d -` (stdin).

### Changed

- A ticket created by an older version gets a blank line after each section header on its next `set` or `archive`; the file is rewritten, its content is preserved.
- Concurrency tests pin their lock budget and back off `Find` polling, so the N=50 stress tests no longer trip the shipped 5s timeout on slow CI runners (T-0092, T-0093).
- The `TICKET_LOCK_TIMEOUT` variable and the `--verbose` event lifecycle are now documented in `--help` and the docs, covering the mandatory duration unit and the silent fallback (T-0094).

### Removed

- The bash-era ticket migration section from `--help` and the documentation (T-0095).

### Fixed

- CLI panics replaced by errors in the i18n and usage paths.
- Byte-stable Parse to Render round trip for a ticket whose `## Journal` section is empty.
- MD022/MD032: a blank line after every section header, so generated tickets are markdownlint-clean.

## [1.2.3] - 2026-09-09

### Added

- `list --json`: machine-readable ticket list output (T-0081).

### Changed

- `list` reads ticket headers only (performance), with a `BenchmarkList` baseline (T-0071, T-0083).

### Removed

- Dead `errTitleNewline` i18n key (T-0085).

### Fixed

- `set` on mixed-section tickets no longer duplicates Summary/Details or injects a placeholder; duplicate Journal sections merge with a warning (T-0070).
- `set` rejects flag-like arguments and `show` reports a non-numeric argument instead of guessing (T-0068, T-0072).
- `set` on an unchanged status number, extra-argument warnings for `show`, not-found hints, and the first-use destination line for `new` (T-0076, T-0077).
- Ticket titles reject C0 control characters, and comments/authors reject CR/LF (T-0065, T-0075).
- `set` detects a same-number file collision before parsing, so a foreign ticket with a broken H1 heading is reported as an existing file rather than a parse error (T-0074).
- Archive symlink escape closed and FIFO open DoS blocked in the store; bounded lock acquire timeout with an inode-swap guard (T-0066, T-0073, T-0078, T-0079).
- `init` Lstat-hardens `tickets/` and `tickets/archive`; a missing or empty `-C` value is rejected; a non-tickets directory fails fast (T-0062, T-0067, T-0069).
- 28 hardcoded Russian CLI errors localized (T-0080).
- Windows-only CI failures in lock, paths and store tests (T-0086).

Releases older than 1.2.3 are itemized by their git tags.
