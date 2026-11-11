# Changelog

This project follows Keep a Changelog 1.1.0 and its versions follow Semantic Versioning.
This file is English-only by design: README.ru.md translates README.md, while a single changelog keeps the release history in one place.

## [Unreleased]

### Added

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
