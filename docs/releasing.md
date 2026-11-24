# Releasing

Full runbook for publishing a signed release: creating the GPG-signed annotated tag, building the six platform binaries, signing the checksums, publishing through `gh`, and verifying the published assets. Build targets and flags live in [development.md](development.md); host access and the deploy gate live in the local `DEPLOY.md` on the development PC (an untracked agent file, not part of the repository). This page covers the whole release path.

Commands below use `v2.0.1` as the example version — substitute the version you are releasing. The release GPG key is `6AB4D0E6AA282FC9`, full fingerprint `0CAF1003F44DD0E4990F4EFF6AB4D0E6AA282FC9`.

## Prerequisites

- The final `origin/main` commit is fixed (the changelog PR is merged) and CI is green on it: Lint, Build (ubuntu/macos/windows), Security audit.
- `gofmt -l .` is empty, `go vet ./...` and `go test ./...` are clean (the deploy gate in `DEPLOY.md`).
- The release GPG key `6AB4D0E6AA282FC9` is present in the local keyring (`gpg --list-keys 6AB4D0E6AA282FC9`).
- Commits stay SSH-signed with the ai-base key through the TPM2-backed ssh-agent, as configured globally. Do **not** change the global git config for the release: tag signing uses per-command OpenPGP overrides (below), because the global `gpg.format` is `ssh` for commits.

## Build from a normal clone

Release artifacts must be built from a **normal `git clone`**, never from a linked worktree:

```bash
git clone https://github.com/ErshovDmitry/tickets.git
cd tickets
git checkout main
git rev-parse HEAD   # the final release commit; record it
```

Do **not** build release artifacts in a linked worktree (`git worktree add`). With Go 1.26, a binary built in a linked worktree *may* omit the `vcs.revision` and `vcs.modified` build settings. This is not a claim that every such build fails — a particular checkout may still record them — but it cannot be relied on, so a linked worktree cannot guarantee provenance. A normal clone is the supported release path.

## Create and verify the signed tag

Create an annotated tag signed with the release GPG key. The signing backend and the key are selected per command, so the global SSH signing config for commits stays untouched:

```bash
git -c gpg.format=openpgp -c user.signingkey=6AB4D0E6AA282FC9 \
  tag -s -u 6AB4D0E6AA282FC9 -m v2.0.1 v2.0.1 <commit>
```

`<commit>` is the final release commit recorded above. `-s`/`-u` create a cryptographically signed annotated tag object; `-u` selects the signing key, and `gpg.format` selects the backend (OpenPGP here, overriding the global `ssh`).

Verify the tag with the same OpenPGP overrides:

```bash
git -c gpg.format=openpgp -c user.signingkey=6AB4D0E6AA282FC9 tag -v v2.0.1
```

Expected: a good signature from `0CAF1003F44DD0E4990F4EFF6AB4D0E6AA282FC9`, and the tag object points at the final release commit. Without the overrides `git tag -v` uses the global `ssh` format and cannot verify an OpenPGP tag.

Check out the tag for the build and derive the version from it:

```bash
git checkout v2.0.1
VER=$(git describe --tags --exact-match | sed 's/^v//')
go version                 # Go >= 1.26, see development.md
```

## Build all release assets

Build every target with `CGO_ENABLED=0`, keeping the asset names stable (`ticket-<os>-<arch>`, `.exe` on Windows):

```bash
for os in linux darwin windows; do
  for arch in amd64 arm64; do
    ext=""
    [ "$os" = windows ] && ext=".exe"
    CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
      go build -ldflags "-X ticket/internal/cli.version=$VER" \
      -o "dist/ticket-$os-$arch$ext" ./cmd/ticket
  done
done
```

The result is six binaries: `ticket-linux-amd64`, `ticket-linux-arm64`, `ticket-darwin-amd64`, `ticket-darwin-arm64`, `ticket-windows-amd64.exe`, `ticket-windows-arm64.exe`. The windows/amd64 release asset is `ticket-windows-amd64.exe` — **not** the dev `dist/ticket.exe` named in the cross-compilation table of [development.md](development.md).

Generate the checksum file and its detached signature next to them:

```bash
cd dist
sha256sum ticket-linux-amd64 ticket-linux-arm64 \
  ticket-darwin-amd64 ticket-darwin-arm64 \
  ticket-windows-amd64.exe ticket-windows-arm64.exe > SHA256SUMS.txt
gpg --armor --detach-sign -u 6AB4D0E6AA282FC9 SHA256SUMS.txt
```

The signature must be a **detached ASCII-armored** signature (`SHA256SUMS.txt.asc`, starting with `-----BEGIN PGP SIGNATURE-----`) — not a clearsign, which would rewrite the checksum file itself. `-u 6AB4D0E6AA282FC9` pins the signing key. A release therefore carries eight assets: six platform binaries, `SHA256SUMS.txt`, and `SHA256SUMS.txt.asc`.

## Verify locally before publishing

Inspect the build information embedded in each of the six binaries:

```bash
for f in dist/ticket-*; do go version -m "$f" | grep -E 'vcs\.|cli.version'; done
```

Every release binary must report:

- a `build vcs.revision=<commit>` line whose commit equals `git rev-parse HEAD` of the tagged checkout;
- `build vcs.modified=false`;
- the `-ldflags` value containing `ticket/internal/cli.version=<version>`.

A module without a `/vN` suffix is recorded by Go as a pseudo-version (for example `v1.2.7-0.<timestamp>-<commit>`), not as the tag name; the authoritative version is the tag plus the `-ldflags` line, not that pseudo-version.

Then check the checksums, the signature, and the version locally:

```bash
cd dist
sha256sum -c SHA256SUMS.txt
gpg --verify SHA256SUMS.txt.asc SHA256SUMS.txt
./ticket-linux-amd64 --version   # ticket version 2.0.1
```

If any buildinfo point is missing, the artifact was not built from a clean checkout at the tag — rebuild it from the normal clone; do not publish it.

## Push the tag

Push the signed tag **before** creating the release:

```bash
git push origin v2.0.1
```

`gh release create --verify-tag` aborts when the tag does not already exist in the remote repository, so the push is a hard prerequisite.

The push over HTTPS authenticates through the `gh` credential helper (`gh auth git-credential`), already configured on the development PC; no token or password appears in the command or in this runbook. If a push on a machine without the helper prompts for credentials, run `gh auth setup-git` once (the host must already be authenticated via `gh auth login`) and retry the push.

## Publish the release

Prepare the notes file first. It must contain the English `[2.0.1]` changelog section with the release date and only the changes actually shipped (from `git log v2.0.0..HEAD` and the diffs, not from an empty Unreleased section), the full key fingerprint `0CAF1003F44DD0E4990F4EFF6AB4D0E6AA282FC9`, and the verify commands (`gpg --verify SHA256SUMS.txt.asc SHA256SUMS.txt`, `sha256sum -c SHA256SUMS.txt`) — model it on the Assets section of the published v2.0.0 notes.

Publish with `--verify-tag` and **without** `--target`:

```bash
gh release create v2.0.1 --verify-tag --notes-file notes-v2.0.1.md \
  dist/ticket-linux-amd64 dist/ticket-linux-arm64 \
  dist/ticket-darwin-amd64 dist/ticket-darwin-arm64 \
  dist/ticket-windows-amd64.exe dist/ticket-windows-arm64.exe \
  dist/SHA256SUMS.txt dist/SHA256SUMS.txt.asc
```

`--verify-tag` aborts unless the tag already exists on the remote, which guarantees the release hangs on the pushed GPG-signed annotated tag. Without it, `gh` would silently create a lightweight tag from the default branch when the tag is missing, losing the signature; `--target` only steers that automatic tag creation and has no place in this flow.

Confirm the uploaded set by name — exactly eight assets:

```bash
gh release view v2.0.1 --repo ErshovDmitry/tickets --json assets --jq '.assets[].name'
```

## Verify a published release (fresh download)

Download the assets fresh from the published release into a clean directory (not from the build directory) and repeat all checks:

```bash
gh release download v2.0.1 --repo ErshovDmitry/tickets --dir release-check
cd release-check
sha256sum -c SHA256SUMS.txt
gpg --verify SHA256SUMS.txt.asc SHA256SUMS.txt
go version -m ticket-linux-amd64 | grep -E 'vcs\.|cli.version'
chmod +x ticket-linux-amd64      # downloaded assets lose the executable bit
./ticket-linux-amd64 --version   # ticket version 2.0.1
cd ..                            # return to the repo root for the Windows smoke below
```

`sha256sum -c` must report every file as `OK`, and `gpg` must report a good signature from `0CAF1003F44DD0E4990F4EFF6AB4D0E6AA282FC9`. The buildinfo must again show `vcs.revision` equal to the tagged commit and `vcs.modified=false`.

When comparing against the previous release (v2.0.0), use `gh release view` / a fresh `gh release download` as the reference — never an old local `dist/` directory. Record the observed results for that release in its evidence note; never copy checksums or verification outcomes from an earlier release into a later one — digests differ for every build.

## Windows version and smoke (dser97)

SSH access to dser97 is already configured: the ai-base key lives in the TPM2-backed ssh-agent on the development PC, so `ssh dser97.home` / `scp` work as-is — do not ask for the key (see `DEPLOY.md`).

From the repo root (the fresh-download block above returns there), copy the release binary and the smoke script into the SSH home directory, then check the version and run the smoke from there:

```bash
scp dist/ticket-windows-amd64.exe scripts/smoke-windows.ps1 dser97.home:
```

```powershell
.\ticket-windows-amd64.exe --version   # ticket version 2.0.1
powershell -ExecutionPolicy Bypass -File .\smoke-windows.ps1 -TicketExe .\ticket-windows-amd64.exe
```

The smoke script runs entirely inside a temp sandbox (the repo and user data are untouched). It reports 11 PASS checks grouped into eight logical steps — several steps contribute more than one check. In the order in which the script runs them, the checks are:

1. `new` — `T-0001-open.md` is created and the printed path matches it.
2. `list` — the new ticket is listed as `open`.
3. `show` — the ticket body contains its title.
4. `set` — the ticket is renamed to `T-0001-wip.md`, with the status line and the journal updated.
5. foreign CWD — `-C` and `--tickets-dir=` both resolve the tickets directory when the command runs from a foreign working directory.
6. `TICKETS_DIR` — a new ticket is created in the override directory.
7. flag over env — `-C` wins over `TICKETS_DIR`.
8. five parallel `new` calls — all five exit 0, yield five unique ticket numbers, and their files exist.
9. `init` in a clean directory — `tickets/` and `archive/` are created, and `new`/`list` see `T-0001`.
10. `init` repeated — a no-op that still exits 0.
11. `init` blocked by a file named `tickets` — exits 1, prints the conflict label on stderr, and leaves the file untouched.

Exit code 0 means all steps passed, 1 means at least one failed. For readable UTF-8 output over SSH, run the commands through `powershell -NoProfile -Command "[Console]::OutputEncoding=[Text.Encoding]::UTF8; ..."`.

## Immutable release

After publication the tag and the assets are immutable: no re-tagging, no re-uploading, no replacing assets. A defect found after publication is fixed in the next patch version, never by editing the published release.
