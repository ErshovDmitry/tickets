# CI triage

This procedure separates two very different cases behind a red GitHub Actions run: a capacity cancel (the platform took the job away before it ever started) and a genuine test failure. The distinction is made from attempt-level job data, not from the run status, because a run whose jobs were capacity-cancelled reports `conclusion: failure` itself. The procedure then says what to do next: rerun the failed jobs, or fix the code.

## Capacity-cancel signs

A job was cancelled by the platform, not by the test suite, when all three job-level signs are present:

- `conclusion` is `cancelled`;
- `runner_name` is empty — no runner was ever assigned;
- `len(steps)` is `0` — the job never started a single step.

The queue time (about 902 s in the field measurement of T-0196) is a reference value only, not a required sign: in the fixture data of T-0196 an infrastructure cancel has `queue = 0`.

Because the run itself ends with `conclusion: failure`, filtering by `runs?status=cancelled` is useless — the run never reports `cancelled` in the first place.

## Test-failure contrast

A real test failure looks different on every axis:

- `conclusion` is `failure`;
- `runner_name` is not empty — a runner was assigned and the job started;
- the steps are present, and the log of the failed step shows the failed assertion or the failed build.

The action here is to fix the code. Do not rerun the job: the same failure repeats.

## Historical fail-fast cascades

The 15 cancelled jobs of the historical window decompose into two groups: 6 capacity cancels (no runner assigned) and 9 fail-fast neighbours (jobs cancelled because a sibling job of the same matrix failed while `fail-fast: true` was still active, before PR #30).

After `fail-fast: false` (ci.yml, PR #30) the neighbour cancels disappeared. Since then a cancelled job is a capacity suspect only when the job-level signs are present (empty `runner_name` and zero steps).

## What to do

Rerun only the failed jobs of the affected run:

```bash
gh run rerun <run-id> --failed
```

The standard path is planned to be ci-retry.yml stage 2 (T-0202); today only stage 1 exists, and it is a dry run.

## Weekly procedure

Run the metrics script once a week:

```bash
bash scripts/ci-metrics.sh --days 30 --cache ~/.cache/ticket-ci-metrics
```

The output consists of the following blocks:

- queue percentiles `p50`/`p95`/`max` per operating system over the last 30 days;
- a separate macOS block over the last 14 days (`p95` and job count) — the macOS queue is the slowest one and the alarm threshold is tied to it;
- the cancelled jobs summary: infrastructure cancels (`runner_name` empty, zero steps) versus the other cancels;
- the rerun share: PR runs with a rerun as a percentage of all PR runs in the window;
- the verdict: `OK`, `ALARM`, `INCOMPLETE`, `NO_DATA` or `ERROR`, each with its own marker.

`INCOMPLETE` means the data is incomplete (pagination, see T-0204), `NO_DATA` means there were zero jobs in the window.

Exit codes: `0` — only `OK`; `1` — `ALARM`, `INCOMPLETE`, `NO_DATA` or `ERROR`; `2` — bad arguments.

On `ALARM`: open a ticket, and consider the rerun policy and a self-hosted runner.

Data mechanics: job data is taken per attempt only, via `/actions/runs/<id>/attempts/<n>/jobs`; queue time is `started_at − created_at`, jobs with `started_at < created_at` (reused jobs) are excluded; the raw cache lives outside the repository.

## Alarm thresholds

The thresholds are the advisor decision of ticket T-0203:

> ≥3 infrastructure-cancels OR >10% of PR runs with rerun over 30 days OR p95 macOS > 15 min with ≥30 jobs over 14 days

The script keeps them as named constants: CANCEL_MIN=3, RERUN_PCT=10, MACOS_P95_MAX_SEC=900, MACOS_MIN_JOBS=30, MACOS_DAYS=14. Every constant can be overridden by a flag.

## Scheduling and cron honesty

The scheduled workflow is possible only after zizmor is connected (T-0207 is still open). Until then the weekly run is manual, and it follows this procedure:

```bash
bash scripts/ci-metrics.sh --days 30 --cache ~/.cache/ticket-ci-metrics
```

## Stage-2 caveat

After ci-retry stage 2 is enabled, the `rerun%` metric will also count the automatic reruns, not only the manual ones. The script prints this caveat in its output.

## Security audit: updating zizmor and reviewing the baseline

The `security` job in `.github/workflows/ci.yml` audits both workflows with
a pinned zizmor. A red Security audit run means a NEW finding (anything in
the documented baseline is ignored and stays green). Triage procedure:

1. **Version decision.** Read the upstream changelog/release notes for the
   new zizmor version. The bump is a separate, deliberate commit — never
   bundled with other changes.
2. **Fetch the asset.** Download the release asset by its exact URL and
   compare `sha256sum` output with the `digest` field from
   `gh api repos/zizmorcore/zizmor/releases/latest` (or the release page).
3. **Attestation.** Confirm the release carries a Sigstore attestation from
   the `release-binaries.yml` workflow:
   `gh attestation verify <asset> --repo zizmorcore/zizmor`.
4. **Vendor (offline path, LOCK).** Update the vendored files
   `.github/zizmor/bundle.json` and `.github/zizmor/trusted_root.jsonl`
   under a wiki lock. Pick the attestation bundle BY PREDICATE — the entry
   with `SLSA provenance/v1` (e.g. `gh api
   repos/zizmorcore/zizmor/attestations/<digest>`), not by array index;
   fetch the trusted root with `gh attestation trusted-root`. Then verify
   OFFLINE with credentials isolated (empty config dir, tokens unset):
   `env -u GH_TOKEN -u GITHUB_TOKEN GH_CONFIG_DIR=<empty>
   gh attestation verify <asset> --bundle .github/zizmor/bundle.json
   --custom-trusted-root .github/zizmor/trusted_root.jsonl
   --repo zizmorcore/zizmor
   --signer-workflow zizmorcore/zizmor/.github/workflows/release-binaries.yml
   --source-ref refs/tags/vX.Y.Z`.
   Expect exit 0 for the genuine asset, 1 for a tampered one, 1 for a
   bundle from a different signer — probe all three.
5. **Baseline BEFORE.** Run the audit on both workflows with the CURRENT
   config and record the numbers (`ZIZMOR_CONFIG=.github/zizmor.yml
   ./zizmor --persona auditor --offline .github/workflows/ci.yml
   .github/workflows/ci-retry.yml`, plus a no-config run).
6. **Pin in one commit.** Update the asset URL + SHA-256 in `ci.yml`, the
   vendored file hashes in `ci.yml`, the ignores (re-measured positions —
   never copied) in `.github/zizmor.yml`, and the audit-log row in
   `docs/security-accepted-risks.md` (+ RU mirror) — as ONE commit.
7. **AFTER.** Re-run the audit: expect "No findings to report" with the
   ignored count equal to the baseline size, exit 0, and a green Security
   audit job in CI. Put the before/after numbers in the report.

The accepted-risks registry (`docs/security-accepted-risks.md`) defines
which findings may stay in the baseline and when the next review is due
(any version/digest bump, any workflow change, any new privileged CI
contour, or 90 days — whichever comes first).

## Local home note

The tracked canonical home of this procedure is this document pair: `docs/ci-triage.md` (EN) and `docs/ru/ci-triage.md` (RU). The wave adds a pointer section into the local, gitignored `AGENTS_ARCHITECTURE.md` (live tree) that references `docs/ci-triage.md` — the full text is not duplicated there.
