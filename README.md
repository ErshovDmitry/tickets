**English** | [Русский](README.ru.md)

# ticket

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
```

## Documentation

- [Installation](docs/installation.md) — putting `ticket` on your `PATH`, tickets-directory resolution (`-C`, `$TICKETS_DIR`, upward scan), one binary for many projects, migration note.
- [Usage](docs/usage.md) — full command reference, ticket file format.
- [Configuration](docs/configuration.md) — environment variables `TICKETS_DIR`, `TICKET_WHO`, `TICKET_LANG`.
- [Scenarios](docs/scenarios.md) — typical workflows: agent integration (`AGENTS.md` block), agent skill, archiving, cross-project tickets.
- [Development](docs/development.md) — building from source, cross-compilation, testing, Windows smoke.

## Links

- bash original: [init-tickets.md](https://gitlab.com/ai-dmitry/promts/-/blob/main/init-tickets.md)

## Status

**v1 done.**

## What's next

There are ideas for further development of ticket, but they will be published later in a complete form. When there is something to show, a link will appear here.
