# Scenarios

Practical setups: wiring `ticket` into an AI agent's workflow, working across projects, archiving, and handling secrets.

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
ticket list all -P otherproj
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
ticket list archive    # list the archive
ticket show 7          # show works for archived tickets too
ticket set 7 wip "reopened"   # restores the ticket from the archive into work
```

Archived tickets stay first-class: `show` searches the archive, `set` works on them, and moving one back to `open` or `wip` restores it out of `tickets/archive/` into work.

## Secrets

Secrets (passwords, tokens, keys) must NOT be written into tickets. Ticket files are plain Markdown in the project tree, frequently committed to git and read by agents — anything written there is effectively published. Reference a secret by name or by its location in a secret store instead of pasting the value.

## Agent skill

The repo ships an agent skill at `skills/ticket-cli/` (`SKILL.md` + `reference.md`) that teaches AI agents the ticket workflow — commands, lifecycle, and pitfalls. Install by copying the directory to `~/.agents/skills/ticket-cli/`.
