# Configuration

Environment variables that control where `ticket` stores data, who it records as author, and which language it speaks.

## Environment variables

| Variable | Purpose |
|----------|---------|
| `TICKETS_DIR` | explicit tickets directory override |
| `TICKET_WHO` | ticket author; chain `TICKET_WHO → USER → USERNAME → agent` |
| `TICKET_LANG` | language of `ticket help` and of files created by `ticket new`; chain `TICKET_LANG → LC_ALL → LANG`; a known prefix selects the language, an unknown prefix falls back to EN, all unset → RU (default) |
| `TICKET_LOCK_TIMEOUT` | how long `ticket` waits for the tickets-directory lock (`.lock`) before giving up with a `lock: ... still held by another process` error; default `5s` |
| `TICKET_LOG` | debug-log destination file; when set, logging is enabled and records go to this file instead of stderr |

Each chain is evaluated left to right: the first non-empty value wins. `TICKET_WHO` falls back to `USER`, then `USERNAME` (Windows), then the literal `agent`. `TICKET_LANG` falls back to `LC_ALL`, then `LANG`. Its prefix (up to the first `_` or `.`, case-insensitive) selects a known language; an unknown prefix falls back to English. Only when all three are empty is the default Russian used.

`TICKET_LOCK_TIMEOUT` is read on every lock acquisition, so it applies to each invocation independently. The value is a Go duration and must carry a unit: `30s`, `2m`, `500ms`, `1h`; the only unit-less value Go accepts is the literal `0`. The default `5s` is defined in the code (`internal/store/store_lock.go`, `defaultLockTimeout`) — there is no second place where it is set. An empty or unparsable value — including one without a unit, such as `5` — silently falls back to that default, without any warning. A value `<= 0` means a single lock attempt, without waiting. The variable affects only the mutating commands (`new`, `set`, `archive`); the reading ones (`list`, `show`) open the tickets directory read-only and never take the lock.

Raise it when the tickets directory lives on a slow disk or on a network filesystem, or when many `ticket` processes run in parallel (CI, several agents at once) and the lock is held long enough to exceed 5 s. Lower it when a busy directory should fail fast instead of waiting.

## Debug log (--verbose / TICKET_LOG)

`ticket` can keep a structured debug log of its own operation. Logging is off by default and is controlled by one global flag and one environment variable.

**Enabling.** Logging starts when the global flag `--verbose` is placed BEFORE the command name (`ticket --verbose list`) or when `TICKET_LOG` is non-empty. When neither is set, there is no logging at all and every output byte is identical to an invocation without the feature. This applies to the five logged commands (`new`, `list`, `show`, `set`, `archive`) — except their `-h`/`--help` subcommand form, which prints usage and builds no logger; `init`, `help`, `version` and a bare `ticket` build no logger at all and ignore `TICKET_LOG` entirely — see "Open failure" below.

**Destination.** `TICKET_LOG` wins over `--verbose`. With `--verbose` alone records go to stderr; with `TICKET_LOG` set they are appended to that file: the file is opened for append, created owner-only when absent, an existing file keeps its current mode, and parent directories are never created. No record ever reaches stdout — stdout stays pure command output. Both destinations receive byte-identical records — the same handler, only the writer differs.

**Open failure.** If the log file cannot be opened, `ticket` prints ONE stderr line naming the path twice and wrapping the OS error text, and exits 1 BEFORE the command runs. For the five logged commands (`new`, `list`, `show`, `set`, `archive`) a missing parent directory therefore fails pre-dispatch: stdout stays empty, the exit code is 1, there is no fallback and no ticket is mutated. `init`, `help`, `version` and a bare `ticket` build no logger at all and ignore `TICKET_LOG` entirely. Example (Linux; the OS error text on other platforms is illustrative only):

```text
ticket: open log file /home/user/logs/ticket.log: open /home/user/logs/ticket.log: no such file or directory
```

The `ticket: open log file ...:` prefix is hardcoded English wrapped around the OS error text, so the line is locale-independent: it looks the same under any `TICKET_LANG`.

**Write/close failure.** A failed record write is reported as ONE stderr line after the command output — `ticket: log write: ...`; a failed close of a file destination as ONE stderr line — `ticket: close log file <path>: ...`. If BOTH the write and the close fail, the two errors are joined and the two lines are emitted together. The exit code is unchanged: after a successful open the log is diagnostic and never fatal.

**Record format.** One record per line: `ticket: <level>: <event> key=value ...`. Levels are `debug`, `info`, `warn` and `error`; there is no level switch and no threshold — every record the code emits is written. Strings are double-quoted with Go `strconv.Quote` (`command="new"`, empty `path=""`); numbers, booleans and durations are bare (`code=0`, `timeout=5s`). The event name and keys stay bare unless they contain spaces, `=`, quotes, backslashes, control characters or non-printable runes. Attributes render in emission order, never sorted; group attributes render as `group.key`. There is no timestamp, no caller and no source file/line.

**Privacy.** Records carry only paths and operational metadata — never a ticket title, details, comments, ticket body, or environment-variable values. The CLI does not enforce the log location: keeping the log file outside `tickets/` is your responsibility, because `tickets/` is scanned and parsed by the tool.

**Canonical events.** This file is the only place that lists the events; [usage.md](usage.md) and the CLI skill only cross-reference it.

| Event | Level | Attributes | Emitted when |
|-------|-------|------------|--------------|
| `command.start` | info | `command` | each logged command, before the tickets dir is resolved |
| `command.done` | info | `command`, `code` | after the command handler returns; NOT emitted when the run aborts before the handler (tickets-dir resolution, store open, journal `who` check) |
| `dir.resolve` | debug | `source`, `dir` | tickets dir resolved (`source` is `--tickets-dir`, `TICKETS_DIR` or `scan`) |
| `dir.resolve.fail` | error | `source` | tickets-dir resolution failed |
| `store.open` | error | `dir` | the store failed to open |
| `store.read.warning` | warn | `path`, `reason` | one record per parse warning surfaced by `new`/`list`/`set`/`archive` |
| `store.lock.acquired` | debug | `path` | the tickets-directory lock was acquired |
| `store.lock.released` | debug | `path` | the lock was released |
| `store.lock.error` | error | `path`, `reason` | the lock could not be acquired or released, or its file was replaced (`reason` is `timeout`, `error` or `swapped`) |
| `store.number.assigned` | info | `number` | `new` was given a ticket number |
| `store.number.retry` | debug | `attempt`, `max`, `number` | the number was taken by another process, so `new` retried |
| `store.set.append` | info | `number`, `status`, `path` | a journal entry was appended in place (status unchanged) |
| `store.set.commit` | info | `number`, `from`, `to`, `path` | the status change was committed |
| `store.set.failure` | error | `number`, `to`, `stage` | the status change failed (`stage` is `collision`, `already`, `tmp`, `link` or `old`) |
| `store.set.append.failure` | error | `number`, `stage` | the in-place journal append failed (`stage` is `tmp` or `rename`) |
| `store.archive.commit` | info | `number`, `status`, `path` | the ticket was archived |
| `store.archive.failure` | error | `number`, `stage` | the archive failed (`stage` is `already`, `not_closed`, `collision`, `tmp`, `link` or `old`) |
| `store.recover.commit` | info | `number`, `keep`, `discard` | a crash pair was recovered; the stale `discard` file was removed |
| `store.recover.noop` | info | `number`, `keep` | the single file already carried the selected name; nothing was removed |
| `store.recover.failure` | error | `number`, `keep`, `discard`, `stage`, `err` | the recovery was refused or hit an I/O error (`stage` is `selection`, `scan`, `ambiguous`, `state` or `remove`) |

Byte-exact records produced by the built binary (Linux):

```text
ticket: info: command.start command="list"
ticket: debug: dir.resolve source="scan" dir="/home/user/proj/tickets"
ticket: info: command.done command="list" code=0
```

An error record looks the same, e.g. a store-open failure:

```text
ticket: error: store.open dir="/home/user/proj/tickets"
```

**Misplaced flag.** `--verbose` must precede the command name: the global-flag scan stops at the first non-flag token, so after the command name the token is a subcommand argument. These flag/argument error texts follow the ACTIVE LOCALE (RU is the default when no locale is set); the examples below were verified with `TICKET_LANG=en` and are EN-only:

| Invocation | Error (EN-only) | Exit |
|------------|-----------------|------|
| `ticket new "T" -d x --verbose` | `ticket: unknown argument: --verbose` | 1 |
| `ticket new --verbose "T"` | `ticket: title must not start with '-'` | 1 |
| `ticket list --verbose` | `ticket: unknown flag: --verbose` | 1 |

The glued form `--verbose=x` is not recognised and not stripped (`ticket list --verbose=x` fails with `ticket: unknown flag: --verbose=x`, exit 1); `--verbose` after `help` or `version` is ignored.

```bash
# records to stderr
TICKET_LANG=en ticket --verbose list

# records appended to a file (TICKET_LOG wins over --verbose, so the flag can be omitted)
TICKET_LOG=$HOME/ticket.log ticket list
```

## Flag precedence

The global `-C` / `--tickets-dir` flag overrides `TICKETS_DIR` for a single invocation. Full resolution order for the tickets directory is in [installation.md](installation.md).

## Examples

```bash
# run against a sandbox tickets directory for one command
ticket -C /tmp/sandbox/tickets list

# same, via environment
TICKETS_DIR=/tmp/sandbox/tickets ticket list

# English help output
TICKET_LANG=en ticket help

# record a specific author
TICKET_WHO=ivan ticket new "Export breaks" -t BUG -p high

# wait up to 30 seconds for the tickets-directory lock (mutating commands only)
TICKET_LOCK_TIMEOUT=30s ticket new "Export breaks" -t BUG -p high
```

Building the binary and running the test suite: [development.md](development.md).
