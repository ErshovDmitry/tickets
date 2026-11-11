# ticket

**English** | [Русский](README.ru.md)

Cross-platform (Windows / Linux / macOS) CLI ticket system in Go.

## What is it

- Tickets are plain Markdown files `T-NNNN-<status>.md` in the project's `tickets/` directory — the single source of truth: readable by a human without any tools (`cat`, editor, `git diff`), no database.
- One static binary `ticket` (`ticket.exe` on Windows), built with `CGO_ENABLED=0`, zero runtime dependencies.
- Cross-platform counterpart of the bash version [init-tickets.md](https://gitlab.com/ai-dmitry/promts/-/blob/main/init-tickets.md).

## 🎬 Video

[![Watch on YouTube](https://img.youtube.com/vi/UW6O2zhEPwc/hqdefault.jpg)](https://youtu.be/UW6O2zhEPwc)

- [YouTube](https://youtu.be/UW6O2zhEPwc)
- [Dzen](https://dzen.ru/video/watch/6a9a61d4b6339503daf3830e)
- [Rutube](https://rutube.ru/video/29ab80e52b2d3c7afaef1eae0d0a0565/)

## Quick start

From the project root:

```bash
ticket init                                             # create tickets/ (idempotent)
ticket new "Broken export" -t BUG -p high -d "Fails on large files"
ticket list                                             # active tickets (open + wip)
ticket show 7                                           # print a ticket (searches the archive)
ticket set 7 done "fixed, tests green"                  # change status, renames the file
ticket recover 7 T-0007-done.md                        # resolve a crash pair (ambiguous number, exit 2)
ticket normalize --dry-run                              # list legacy ticket files that would be re-rendered
ticket normalize                                        # re-render them in place (name untouched; body status reconciled to it, no journal line)
```

## Exit codes

- `0` — success.
- `1` — error: bad arguments, ticket not found, collision, input or store error.
- `2` — ambiguous ticket number: the number belongs to several ticket files; the number and the conflicting files are printed to stderr.
- `3` — update available: the explicit `ticket update-check` found a newer release on GitHub (the command never runs automatically; `0` covers up to date / no releases / dev build, a failed check is `1` and never blocks other commands).

`ticket list` and `ticket normalize` report parse warnings and duplicate numbers on stderr but still exit 0: they are inventory and migration commands, never a gate.

`ticket recover` uses the same codes: `0` also covers a conservative no-op — a repeat after a successful recovery, or a single file already named `<new-file>` — so `0` means the wanted file name is on disk and never proves that a recovery happened; `1` covers a usage error, a refused confirmation, a missing ticket, an I/O error (archive scan included), the keep-old choice and a name that is not the crash pair; `2` means the number cannot be recovered automatically — more than two files, diverging content or equal statuses. The ambiguity report prints both conflicting absolute paths, while `<new-file>` for `recover` is always a bare basename.

## Documentation

- [Installation](docs/installation.md) — putting `ticket` on your `PATH`, tickets-directory resolution (`-C`, `$TICKETS_DIR`, upward scan), one binary for many projects, migration note.
- [Usage](docs/usage.md) — full command reference, ticket file format.
- [Configuration](docs/configuration.md) — environment variables `TICKETS_DIR`, `TICKET_WHO`, `TICKET_LANG`, `TICKET_LOCK_TIMEOUT`, `TICKET_LOG`; the debug-log section covers `--verbose`, the record format and the event list.
- [Scenarios](docs/scenarios.md) — typical workflows: agent integration (`AGENTS.md` block), agent skill, archiving, cross-project tickets.
- [Development](docs/development.md) — building from source, cross-compilation, testing, Windows smoke.
- [Changelog](CHANGELOG.md) — release history, Keep a Changelog format.

## Links

- bash original: [init-tickets.md](https://gitlab.com/ai-dmitry/promts/-/blob/main/init-tickets.md)

## Status

**v1 done.**

## What's next

There are ideas for further development of ticket, but they will be published later in a complete form. When there is something to show, a link will appear here.
