# Configuration

Environment variables that control where `ticket` stores data, who it records as author, and which language it speaks.

## Environment variables

| Variable | Purpose |
|----------|---------|
| `TICKETS_DIR` | explicit tickets directory override |
| `TICKET_WHO` | ticket author; chain `TICKET_WHO → USER → USERNAME → agent` |
| `TICKET_LANG` | language of `ticket help` and of files created by `ticket new`; chain `TICKET_LANG → LC_ALL → LANG`; a known prefix selects the language, an unknown prefix falls back to EN, all unset → RU (default) |

Each chain is evaluated left to right: the first non-empty value wins. `TICKET_WHO` falls back to `USER`, then `USERNAME` (Windows), then the literal `agent`. `TICKET_LANG` falls back to `LC_ALL`, then `LANG`. Its prefix (up to the first `_` or `.`, case-insensitive) selects a known language; an unknown prefix falls back to English. Only when all three are empty is the default Russian used.

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
```

Building the binary and running the test suite: [development.md](development.md).
