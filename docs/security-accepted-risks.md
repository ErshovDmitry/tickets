# Security accepted risks

Registry of accepted security risks for the CI/CD surface, mirrored in
[docs/ru/security-accepted-risks.md](ru/security-accepted-risks.md). Each
entry records the risk, the attack scenario, the control that keeps it
acceptable, and the exact suppression in
[.github/zizmor.yml](../.github/zizmor.yml) (narrow `file:line:column`
ignores; no rule or severity class is ever disabled). The audit job itself is
described in `.github/workflows/ci.yml` (job `security`); the update and
baseline review procedure is in [docs/ci-triage.md](ci-triage.md).

## (a) ci-retry.yml: workflow_run + actions:write (T-0202)

- **Risk:** `.github/workflows/ci-retry.yml` holds `actions: write` and runs
  on `workflow_run` — a privileged, PR-adjacent workflow.
- **Scenario:** a malicious PR cannot execute code in this workflow (it has no
  checkout and no PR-controlled steps; it only calls `gh api` against
  trusted API data). The realistic drift scenario is a future edit that adds
  PR-controlled execution or artifact handling to the privileged context, or
  a `workflow_run` metadata change that reruns more than intended.
- **Control:** the change is a one-step, fail-closed design documented at
  T-0202; the zizmor auditor persona surfaces any drift of this privileged
  workflow (excessive-permissions High/High is not visible under the
  default regular persona); this registry entry makes the acceptance
  explicit. The file
  itself is out of scope for the T-0207 PR and must not be edited to silence
  findings.
- **Suppression (this finding only):**
  `dangerous-triggers ci-retry.yml:14:3`,
  `excessive-permissions ci-retry.yml:31:3`,
  `undocumented-permissions ci-retry.yml:31:3,50:7` (explicit permissions;
  duplication is deliberate),
  `concurrency-limits ci-retry.yml:13:1` (ci-retry starts for every CI
  run; a concurrency cap would break the retry function).
- **Links:** T-0202, T-0207, advisory `b431407f-7242-4963-9ea3-ee00ded6d0bb`.

## (b) Supply chain of CI tools

- **Risk:** the lint job downloads tools by mutable tags —
  `go install staticcheck@v0.8.1`, `go install gosec@v2.30.0`,
  `npx --yes markdownlint-cli2@0.23.3`. A retagged or compromised release
  would run in CI without a local integrity check.
- **Scenario:** upstream account compromise or a malicious release under an
  existing tag; the poisoned binary executes on the runner with the job's
  (read-only) token.
- **Control:** accepted as an incremental, not a new class of risk (advisory
  §2): these tools already ran in CI before the audit existed. Only zizmor
  gets a verifiable pin today: release asset v1.32.0 + SHA-256
  `bb110cc765eca9cd7efb2e284a956be5974c60027b0970f16397d86208a9957b`
  (equal to the GitHub API digest at pin time) + offline Sigstore
  attestation verification (SLSA provenance/v1) against vendored
  `.github/zizmor/bundle.json` /
  `.github/zizmor/trusted_root.jsonl`. The ghcr.io image was rejected: it
  carries no attestation. Triggers for tightening: any new secret,
  self-hosted runner, `pull_request_target`, or new privileged workflow in
  CI — then option A of the advisory is not cancelled, the audit job's
  isolation is tightened instead (advisory §5).

## (c) Baseline findings accepted in ci.yml

- **Risk:** `cache-poisoning` (setup-go/setup-node caches) and a missing
  workflow-level `concurrency` group.
- **Scenario:** cache poisoning needs a fork PR to write the base branch
  cache — it cannot (read-only token in PRs); artifacts uploaded are this
  repo's own sources. A concurrency group would cancel pending jobs with an
  empty `runner_name` and break capacity-cancel triage (T-0196/T-0203).
- **Control:** `cache:false` would rebuild the toolchain on every run and is
  rejected (finding confidence Low); `fail-fast: false` is kept deliberately.
- **Design limitation:** on `pull_request` the audit job checks out PR
  content, so a PR can edit the audited workflows/config to pass its own
  audit; the fail-closed guarantee holds on push to main (trusted base).
  Job isolation (no secrets/caches/artifacts) compensates.
- **Suppression (this finding only):**
  `cache-poisoning ci.yml:46:9,114:9,151:9`,
  `concurrency-limits ci.yml:5:1`.

## (d) ci-metrics.yml: schedule + actions:read (T-0209)

- **Risk:** `.github/workflows/ci-metrics.yml` runs repository code (`scripts/ci-metrics.sh`, default branch) on a timer and holds `actions: read` to read run/job data.
- **Scenario:** a future edit adds a third-party action or PR-controlled execution to this scheduled job, or the read scope is silently widened to `actions: write` in a workflow nobody reviews PR-wise. The data itself is aggregate-only: no secrets in the report, the token is never printed.
- **Control:** explicit workflow-level permissions (`contents: read`, `actions: read`, nothing else); the only action is a SHA-pinned `actions/checkout` with `persist-credentials: false`; no cache, no artifacts, no `GITHUB_ENV`/`GITHUB_OUTPUT` writes, no `${{ }}` interpolation inside `run:`; the script is fail-closed (any non-OK verdict = exit 1 = red run). Drift is caught by the pinned zizmor auditor persona in CI: the CI `security` job audits all three workflows (`ci.yml`, `ci-retry.yml`, `ci-metrics.yml`) in one `Audit workflows` step (ci.yml:263-268).
- **Design limitation:** `GITHUB_TOKEN` cannot read the organization audit log (admin/org scope) — that check stays manual and owner-run; the workflow does not automate it.
- **Suppression:** none. No baseline entry is added for this file; findings are prevented by design. Weakening `.github/zizmor.yml` is out of scope for the 5-file change.
- **Links:** T-0209, T-0212, T-0207, T-0211.

## Audit log

| Date | zizmor | Repo commit | Vendored bundle SHA-256 | Vendored root SHA-256 | Audit result | Next review |
|------|--------|-------------|-------------------------|-----------------------|--------------|-------------|
| 2026-11-19 | v1.32.0 | 6ad2f0b (PR #36; base b56993f) | `5cf002bf…b97b25ef` | `65ca537f…2eb2b9c` | auditor: 0 visible findings, 9 ignored (baseline above); without config: 9 findings | 2027-02-17 |
| 2026-11-20 | v1.32.0 | 9289645 (base; tip at audit) + t-0209 worktree changes | `5cf002bf…b97b25ef` | `65ca537f…2eb2b9c` | auditor: 0 visible findings, 9 ignored (baseline ci.yml+ci-retry.yml; 3 workflows audited); --no-ignores: 9 findings (4 low, 5 high), none in ci-metrics.yml | 2027-02-18 |

Policy: re-audit on every zizmor version/digest bump, every workflow change,
every new privileged CI contour, or after 90 days — whichever comes first.
Procedure: [docs/ci-triage.md](ci-triage.md) §"Security audit: updating
zizmor and reviewing the baseline".
