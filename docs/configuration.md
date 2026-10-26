# Configuration

Environment variables that control where `ticket` stores data, who it records as author, and which language it speaks.

## Environment variables

| Variable | Purpose |
|----------|---------|
| `TICKETS_DIR` | explicit tickets directory override |
| `TICKET_WHO` | ticket author; chain `TICKET_WHO → USER → USERNAME → agent` |
| `TICKET_LANG` | language of `ticket help` and of files created by `ticket new`; chain `TICKET_LANG → LC_ALL → LANG`; a known prefix selects the language, an unknown prefix falls back to EN, all unset → RU (default) |
| `TICKET_LOCK_TIMEOUT` | how long `ticket` waits for the tickets-directory lock (`.lock`) before giving up with a `lock: ... still held by another process` error; default `5s` |

Each chain is evaluated left to right: the first non-empty value wins. `TICKET_WHO` falls back to `USER`, then `USERNAME` (Windows), then the literal `agent`. `TICKET_LANG` falls back to `LC_ALL`, then `LANG`. Its prefix (up to the first `_` or `.`, case-insensitive) selects a known language; an unknown prefix falls back to English. Only when all three are empty is the default Russian used.

`TICKET_LOCK_TIMEOUT` is read on every lock acquisition, so it applies to each invocation independently. The value is a Go duration and must carry a unit: `30s`, `2m`, `500ms`, `1h`; the only unit-less value Go accepts is the literal `0`. The default `5s` is defined in the code (`internal/store/store.go`, `defaultLockTimeout`) — there is no second place where it is set. An empty or unparsable value — including one without a unit, such as `5` — silently falls back to that default, without any warning. A value `<= 0` means a single lock attempt, without waiting. The variable affects only the mutating commands (`new`, `set`, `archive`); the reading ones (`list`, `show`) open the tickets directory read-only and never take the lock.

Raise it when the tickets directory lives on a slow disk or on a network filesystem, or when many `ticket` processes run in parallel (CI, several agents at once) and the lock is held long enough to exceed 5 s. Lower it when a busy directory should fail fast instead of waiting.

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
