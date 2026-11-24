# Scenarios

Practical setups: wiring `ticket` into an AI agent's workflow, working across projects, archiving, deleting, and handling secrets.

## Agent integration

Paste the block below into the `AGENTS.md` of the project that uses `tickets/` (create the file in the project root if it does not exist; paths are relative to the project root; the binary finds the tickets directory regardless of the current directory):

```markdown
## Tickets (`tickets/`)

- At session start: `ticket list` — if there are open tickets, briefly remind the user about them in your first reply.
- Found a problem, bug, or anything suspicious while working — create a ticket IMMEDIATELY: `ticket new "<brief>" -t BUG|OPS|TD|ENH -p low|normal|high -d "<details>"`. Do not stay silent, even if you worked around or fixed it on the spot. Put file:line into the details.
- Status changes — only via `ticket set <number> <status> "<comment>"` (the binary renames the file itself and appends the ticket journal); never rename ticket files manually.
- Before working on a ticket, read its `User comments` section — the user leaves remarks there; the agent does not write there. Free-form working notes go to `Comments`.
- Tests/smoke runs of `ticket` — only in a sandbox (a temp directory via `$TICKETS_DIR`), NEVER in the live `tickets/`.
- Many closed tickets (`done`/`closed`) — move them into the archive: `ticket archive`. Archived tickets are not lost: `ticket list archive`, `show`/`set` work with them, reopening returns a ticket into work.
- Do NOT write secrets (passwords, tokens, keys) into tickets.
```

## Cross-project

Every ticket records a project name. By default it is the basename of the `tickets/` parent directory; `-P <project>` overrides it.

```bash
# create a ticket attributed to another project
ticket new "Cross-project entry" -P otherproj

# filter a listing by project (exact match, case-sensitive)
ticket list all -P otherproj [--type BUG|OPS|TD|ENH] [--priority low|normal|high]
```

Behaviour worth knowing:

- The project column appears in `list` output only when the tickets being shown have differing projects. With a `-P` filter (single project) the column is dropped.
- When a project name is seen for the first time, a warning is printed to stderr — a guard against typos in `-P`.

Command syntax and example output: [usage.md](usage.md).

## Archive workflow

Closed tickets pile up; move them out of the active listing without losing anything.

```bash
ticket archive         # move all closed (done/closed) into tickets/archive/
ticket archive 7       # move one specific ticket
ticket list archive [--type BUG|OPS|TD|ENH] [--priority low|normal|high]   # list the archive
ticket show 7          # show works for archived tickets too
ticket set 7 wip "reopened"   # restores the ticket from the archive into work
```

Archived tickets stay first-class: `show` searches the archive, `set` works on them, and moving one back to `open` or `wip` restores it out of `tickets/archive/` into work.

## Long comments from a file or stdin

A status change with a long comment can take the comment from a file or from stdin instead of the command line:

```bash
printf 'the comment text' > comment.md   # create the comment file
ticket set 7 wip @comment.md              # the comment is the content of comment.md
ticket set 7 done @comment.md             # the same for the closing transition
cat comment.md | ticket set 7 wip -       # the comment is stdin
```

Behaviour worth knowing:

- The source forms apply to ONE comment argument only: `-` is stdin, `@<file>` is the content of that file, `@@name` is the file named `@name` (exactly one `@` is stripped).
- The content is copied byte for byte, and exactly one trailing LF or CRLF is stripped. Any other CR or LF — a second trailing LF (`x\n\n`), a lone trailing CR (`x\r`), an embedded or multi-line break — is refused (exit 1): the ticket name, its bytes and the journal stay unchanged.
- A file written by `echo` ends with one newline, and that single trailing LF (or CRLF) is stripped, so `echo`-written files are accepted as they are. A comment that must end with a blank line cannot be passed: the second trailing LF is refused like any other break inside the text.
- A bare `@` (no path) as the ONE comment argument is an error, and so is a missing file or a failed stdin read (exit 1, nothing written).
- Two or more comment arguments are joined with a single space and used literally — no file and no stdin are read, so a flag-like `-x` stays a literal comment.
- Trade-off: a lone `-` or `@...` argument is never a literal comment, and two arguments are not a byte-exact escape (the join inserts a space).

## Delete lifecycle

A ticket that must not be worked on any more is soft-deleted: the file moves into `tickets/deleted/T-NNNN-deleted.md`, keeps its number, body, metadata and journal, and becomes terminal.

```bash
ticket delete 7 --yes           # main directory, open: no --force needed
ticket delete 7                 # without --yes it prints the candidate path and asks yes/no
ticket delete 7 --yes --force   # wip/done/closed, or an archived ticket
ticket list deleted             # the only listing that shows deleted tickets
ticket show 7                   # show searches tickets/, tickets/archive/ and tickets/deleted/
```

Behaviour worth knowing:

- `deleted` is terminal and read-only: `ticket set 7 wip`, `ticket set 7 open` and `ticket edit 7 --title "…"` refuse such a ticket (exit 1) and the file stays byte-identical. There is NO undelete — a delete is a one-way decision; when the ticket may come back, use `ticket archive` instead.
- `ticket archive` ignores deleted tickets, and every list mode except `deleted` hides them: `active`/`open`/`wip`/`done`/`closed`/`archive`/`all`. `all` keeps its old meaning (the main directory), so existing scripts see no change.
- The number stays taken: the next `ticket new` skips it.
- An interrupted delete leaves a crash pair — the stale source in the main directory plus `T-NNNN-deleted.md`. `ticket recover 7 T-NNNN-deleted.md --yes` closes the pair, keeps the deleted file and does not undelete the ticket.
- `ticket normalize` walks main → archive → deleted and rewrites a legacy deleted file in place; `--dry-run` covers it too.
- Without `--yes` and without a terminal the command refuses and changes nothing: automation MUST pass `--yes`.

## Secrets

Secrets (passwords, tokens, keys) must NOT be written into tickets. Ticket files are plain Markdown in the project tree, frequently committed to git and read by agents — anything written there is effectively published. Reference a secret by name or by its location in a secret store instead of pasting the value.

## Agent skill

The repo ships an agent skill at `skills/ticket-cli/` (`SKILL.md` + `reference.md`) that teaches AI agents the ticket workflow — commands, lifecycle, and pitfalls. Install by copying the directory to `~/.agents/skills/ticket-cli/`.
