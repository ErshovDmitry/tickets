# Installation

How to install the `ticket` binary, how it finds your tickets directory, and how to set up a project.

## Install the binary

Put the binary on your `PATH`:

- Linux / macOS: `~/bin/ticket`
- Windows: `%USERPROFILE%\bin\ticket.exe`

The binary is self-locating: it finds the tickets directory itself from any current directory, so you never need a per-project copy. It is a single static binary built with `CGO_ENABLED=0` and has zero runtime dependencies.

To build it from source, see [development.md](development.md).

## Tickets directory resolution

Resolution order, first match wins:

1. `-C <path>` / `--tickets-dir <path>` — per-invocation override; the path points at the tickets directory itself;
2. `$TICKETS_DIR`;
3. upward scan from the current directory for a child directory named `tickets` (git-style).

Environment variables are documented in [configuration.md](configuration.md).

## Migration note

Projects created by older versions keep a binary or symlink at `tickets/bin/`. Those keep working when invoked from inside the project tree, because the upward scan still finds `tickets/`.

What no longer works: calling that binary by absolute path from an unrelated directory and relying on its location to locate the tickets. Use `-C <tickets-dir>` or `TICKETS_DIR` instead. The stale `tickets/bin/` copy can be deleted at your convenience.

## Quick start

From the project root:

```bash
ticket init
```

This creates `tickets/` and `tickets/archive/`. The command is idempotent. It does **not** install the binary — keep one `ticket` on your `PATH`.

## One binary, many projects

One binary on your `PATH` serves all projects: build it once into `~/bin/ticket`, then run `ticket init` in each project to create its tickets structure.

```bash
VER=$(git describe --tags --always 2>/dev/null | sed 's/^v//'); VER=${VER:-dev}
CGO_ENABLED=0 go build -ldflags "-X ticket/internal/cli.version=$VER" -o ~/bin/ticket ./cmd/ticket
cd <project>
ticket init
```

Build details and cross-compilation targets: [development.md](development.md).

## Git: commit or ignore

If the project is under git, decide whether to commit `tickets/` together with the project or add it to `.gitignore`. Both are valid: committing makes tickets part of project history and reviewable in `git diff`; ignoring keeps them local to the machine.
