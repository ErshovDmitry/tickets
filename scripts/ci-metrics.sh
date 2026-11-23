#!/usr/bin/env bash
# scripts/ci-metrics.sh - weekly CI health metrics for workflows/ci.yml (ticket T-0203).
#
# Blocks: queue p50/p95/max per OS over the window, macOS p95 over the last 14d
# of the window, cancelled split (infrastructure capacity-cancel vs other), PR
# rerun ratio. Verdict: OK/ALARM/INCOMPLETE/NO_DATA/ERROR (fail-closed). Data:
# gh api (auth via keyring, no secrets in output) or an offline TSV fixture;
# percentiles/JSON via python3 stdlib. Writes nothing into the repository.
#
# Exit codes: 0 OK, 1 ALARM/INCOMPLETE/NO_DATA/ERROR, 2 bad arguments.
set -euo pipefail

# Locale must be set before the first numeric printf: under ru_RU.UTF-8 the bash
# builtin rejects "21.824" (недопустимое число) and prints "21,0".
export LC_ALL=C LANG=C

# Alarm thresholds - owner: advisor decision in ticket T-0203, verbatim wording in
# docs/ci-triage.md: ">=3 infrastructure-cancel OR >10% of PR runs rerun within 30
# days OR macOS p95 > 15 min with >=30 jobs over 14 days".
readonly CANCEL_MIN=3          # infra-cancel count alarm threshold
readonly RERUN_PCT=10          # rerun% alarm threshold (strictly greater)
readonly MACOS_P95_MAX_SEC=900 # macOS p95 alarm threshold, seconds
readonly MACOS_MIN_JOBS=30     # macOS min sample size for the p95 alarm
readonly MACOS_DAYS=14         # macOS sub-window length, days

die() {
  printf '%s\n' "$1" >&2
  exit "${2:-1}"
}

usage() {
  cat <<'EOF'
Usage: ci-metrics.sh [--days N] [--repo OWNER/NAME] [--cache DIR] [--json]
                     [--thresh-cancel N] [--thresh-rerun-pct P]
                     [--thresh-macos-sec N] [--thresh-macos-jobs N]
                     [--thresh-macos-days N] [--fixture-tsv FILE] [-h]

CI queue/cancel/rerun metrics for workflows/ci.yml. Verdict:
OK/ALARM/INCOMPLETE/NO_DATA/ERROR. Exit: 0 OK, 1 non-OK, 2 bad arguments.

  --days N               window days (default 30)
  --repo OWNER/NAME      repository (default ErshovDmitry/tickets)
  --cache DIR            raw gh api cache dir, outside the repo
                         (default ~/.cache/ticket-ci-metrics)
  --json                 machine-readable report (pinned keys)
  --thresh-cancel N      infra-cancel threshold (default 3)
  --thresh-rerun-pct P   rerun% threshold (default 10)
  --thresh-macos-sec N   macOS p95 threshold, seconds (default 900)
  --thresh-macos-jobs N  macOS min jobs (default 30)
  --thresh-macos-days N  macOS sub-window days (default 14)
  --fixture-tsv FILE     offline 10-column TSV (no header: run_id attempt job_id
                         name conclusion created_at started_at completed_at
                         steps runner_name); no network; rerun% = attempt>=2
  -h                     this help
EOF
}

days=30
repo="ErshovDmitry/tickets"
cache_dir="$HOME/.cache/ticket-ci-metrics"
json=0
th_cancel=$CANCEL_MIN
th_rerun=$RERUN_PCT
th_msec=$MACOS_P95_MAX_SEC
th_mjobs=$MACOS_MIN_JOBS
th_mdays=$MACOS_DAYS
fixture=""

is_int() { case $1 in ''|*[!0-9]*) return 1 ;; *) return 0 ;; esac }

need_int() { # $1 flag name, $2 value; echoes the validated integer
  [ -n "$2" ] || die "$1 requires a value" 2
  is_int "$2" || die "$1 must be an integer >= 1, got '$2'" 2
  [ "$2" -ge 1 ] || die "$1 must be an integer >= 1, got '$2'" 2
  printf '%s' "$2"
}

while [ $# -gt 0 ]; do
  case $1 in
    -h|--help)
      usage
      exit 0
      ;;
    --days|--repo|--cache|--thresh-cancel|--thresh-rerun-pct|--thresh-macos-sec|--thresh-macos-jobs|--thresh-macos-days|--fixture-tsv)
      [ $# -ge 2 ] || die "$1 requires a value" 2
      val=$2
      case $1 in
        --days)
          days=$(need_int "$1" "$val") ;;
        --repo)
          if [[ $val =~ ^[^/]+/[^/]+$ ]]; then
            repo=$val
          else
            die "--repo must look like OWNER/NAME, got '$val'" 2
          fi ;;
        --cache)
          [ -n "$val" ] || die "$1 requires a non-empty value" 2
          cache_dir=$val ;;
        --fixture-tsv)
          [ -r "$val" ] || die "--fixture-tsv: file not found or unreadable: $val" 2
          [ -s "$val" ] || die "--fixture-tsv: empty file: $val" 2
          fixture=$val ;;
        --thresh-cancel)     th_cancel=$(need_int "$1" "$val") ;;
        --thresh-rerun-pct)  th_rerun=$(need_int "$1" "$val") ;;
        --thresh-macos-sec)  th_msec=$(need_int "$1" "$val") ;;
        --thresh-macos-jobs) th_mjobs=$(need_int "$1" "$val") ;;
        --thresh-macos-days) th_mdays=$(need_int "$1" "$val") ;;
      esac
      shift 2
      ;;
    --json)
      json=1
      shift
      ;;
    *)
      die "unexpected argument '$1' (see -h)" 2
      ;;
  esac
done
command -v python3 >/dev/null 2>&1 || die "ERROR: python3 not found in PATH" 1
tmp=$(mktemp -d "${TMPDIR:-/tmp}/ci-metrics.XXXXXX") || die "ERROR: mktemp failed" 1
trap 'rm -rf "$tmp"' EXIT
if [ -z "$fixture" ]; then
  command -v gh >/dev/null 2>&1 || die "ERROR: gh not found in PATH" 1
  mkdir -p "$cache_dir" || die "ERROR: cannot create cache dir: $cache_dir" 1
  date_from=$(python3 -c 'import datetime,sys; print((datetime.datetime.now(datetime.timezone.utc)
       - datetime.timedelta(days=int(sys.argv[1]))).strftime("%Y-%m-%dT%H:%M:%SZ"))' "$days")
  # Runs list: paginate until collected >= total_count (a page error is fatal).
  page=1
  collected=0
  total=0
  while :; do
    url="repos/$repo/actions/workflows/ci.yml/runs?created=%3E%3D$date_from&per_page=100&page=$page"
    if ! resp=$(gh api "$url"); then
      die "ERROR: gh api failed: $url"
    fi
    printf '%s' "$resp" > "$tmp/runs_page_$page.json"
    counts=$(python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); wr=d.get("workflow_runs")
sys.exit("malformed runs payload: no workflow_runs") if not isinstance(wr, list) else print(d.get("total_count", 0), len(wr))' "$tmp/runs_page_$page.json") \
      || die "ERROR: malformed runs page $page"
    read -r total n <<<"$counts"
    collected=$((collected + n))
    [ "$collected" -ge "$total" ] && break
    [ "$n" -gt 0 ] || die "ERROR: pagination stopped early (page $page: collected=$collected total=$total)"
    page=$((page + 1))
  done
  # Flatten runs to TSV (bash cannot parse JSON): id, run_attempt, created_at, status.
  TMPD="$tmp" python3 - <<'PY'
import glob, json, os
tmp = os.environ["TMPD"]
pageno = lambda p: int(p.rsplit("_", 1)[1].split(".")[0])
with open(os.path.join(tmp, "runs.tsv"), "w") as out:
    for f in sorted(glob.glob(os.path.join(tmp, "runs_page_*.json")), key=pageno):
        for r in json.load(open(f))["workflow_runs"]:
            out.write("%s\t%s\t%s\t%s\n" % (
                r["id"], r["run_attempt"], r["created_at"], r.get("status") or ""))
PY
  # Jobs per attempt 1..run_attempt; raw responses cached outside the repo and
  # written only for completed runs (immutable; an in-flight run is not cached).
  while IFS="$(printf '\t')" read -r rid ratt _created rstatus; do
    [ -n "$rid" ] || continue
    a=1
    while [ "$a" -le "$ratt" ]; do
      cf=$cache_dir/${rid}_${a}.json
      if [ -f "$cf" ]; then
        resp=$(cat "$cf") || die "ERROR: cannot read cache: $cf"
      else
        url="repos/$repo/actions/runs/$rid/attempts/$a/jobs?per_page=100"
        if ! resp=$(gh api "$url"); then
          die "ERROR: gh api failed: $url"
        fi
        if [ "$rstatus" = "completed" ]; then
          printf '%s' "$resp" > "$cf" || die "ERROR: cannot write cache: $cf"
        fi
      fi
      printf '%s' "$resp" > "$tmp/jobs_${rid}_${a}.json"
      a=$((a + 1))
    done
  done < "$tmp/runs.tsv"
fi
# All classification, percentiles and output happen here (python3 stdlib).
if [ -n "$fixture" ]; then mode=fixture; else mode=live; fi
python3 - "$mode" "$tmp" "$fixture" "$days" "$repo" "$json" \
  "$th_cancel" "$th_rerun" "$th_msec" "$th_mjobs" "$th_mdays" <<'PY'
import datetime, glob, json, math, os, sys
mode, tmp, fixture, days, repo = sys.argv[1:6]
json_flag = sys.argv[6] == "1"
th_cancel, th_rerun, th_msec, th_mjobs, th_mdays = map(int, sys.argv[7:12])
days = int(days)
FMT = "%Y-%m-%dT%H:%M:%SZ"
parse_ts = lambda s: datetime.datetime.strptime(s, FMT).replace(tzinfo=datetime.timezone.utc)
num = lambda v: int(v) if float(v) == int(v) else round(v, 3)
fmt = lambda v: v if v is not None else "n/a"
def pctile(vals, q):  # nearest-rank, 1-based: sorted index ceil(q/100*n)
    return sorted(vals)[int(math.ceil(q / 100.0 * len(vals))) - 1]
incomplete = False
jobs = []   # (run_id, attempt, name, conclusion, created, started, steps, runner, event)
runs = {}   # run_id -> [created_at, max_attempt, event]
if mode == "fixture":
    rows = [l.rstrip("\n").split("\t") for l in open(fixture) if l.strip()]
    if not rows:
        sys.exit("ERROR: fixture TSV is empty")
    if len(rows[0]) != 10:
        sys.exit("ERROR: fixture TSV must have 10 fields, got %d" % len(rows[0]))
    anchor = max(parse_ts(r[5]) for r in rows)
    for r in rows:
        rid, att = r[0], int(r[1])
        jobs.append((rid, att, r[3], r[4], r[5], r[6], int(r[8]), r[9], "", r[5]))
        rec = runs.setdefault(rid, [r[5], att, ""]); rec[1] = max(rec[1], att)
else:
    anchor = datetime.datetime.now(datetime.timezone.utc)
    pageno = lambda p: int(p.rsplit("_", 1)[1].split(".")[0])
    for f in sorted(glob.glob(os.path.join(tmp, "runs_page_*.json")), key=pageno):
        for r in json.load(open(f))["workflow_runs"]:
            runs[r["id"]] = [r["created_at"], r["run_attempt"], r.get("event") or ""]
    for rid, (created, ratt, event) in runs.items():
        for a in range(1, int(ratt) + 1):
            p = os.path.join(tmp, "jobs_%s_%s.json" % (rid, a))
            try:
                d = json.load(open(p))
            except (OSError, ValueError):
                sys.exit("ERROR: cannot parse jobs payload: %s" % p)
            if not isinstance(d, dict) or not isinstance(d.get("jobs"), list):
                sys.exit("ERROR: malformed jobs payload (no .jobs): %s" % p)
            if d.get("total_count", 0) > 100:
                incomplete = True  # T-0204: jobs beyond the first page are not fetched
            for j in d["jobs"]:
                jobs.append((rid, a, j.get("name") or "", j.get("conclusion") or "", created,
                             j.get("started_at") or "", len(j.get("steps") or []),
                             j.get("runner_name") or "", event, j.get("created_at") or ""))
start = anchor - datetime.timedelta(days=days)
sel = [j for j in jobs if start <= parse_ts(j[4]) <= anchor]
no_data = not sel
OS_OF = {"Build (macos-latest)": "macos", "Build (ubuntu-latest)": "ubuntu", "Lint": "ubuntu", "Build (windows-latest)": "windows"}
queue = {"ubuntu": [], "macos": [], "windows": []}  # (queue_seconds, run created_at)
for j in sel:
    # queue = job.started_at - job.created_at (brief T-0203); run.created (j[4]) only for windowing
    jc = j[9] or j[4]
    if j[5] and jc and j[5] >= jc and j[2] in OS_OF:
        queue[OS_OF[j[2]]].append(((parse_ts(j[5]) - parse_ts(jc)).total_seconds(), j[4]))
def os_stats(o):
    vals = [q for q, _ in queue[o]]
    if not vals:
        return {"p50_s": None, "p95_s": None, "max_s": None, "jobs": 0}
    return {"p50_s": num(pctile(vals, 50)), "p95_s": num(pctile(vals, 95)),
            "max_s": num(max(vals)), "jobs": len(vals)}
mstart = max(start, anchor - datetime.timedelta(days=th_mdays))
m14 = [q for q, c in queue["macos"] if parse_ts(c) >= mstart]
infra = sum(1 for j in sel if j[3] == "cancelled" and j[7] == "" and j[6] == 0)
other = sum(1 for j in sel
              if j[3] == "cancelled" and not (j[7] == "" and j[6] == 0))
sel_runs = [rid for rid, (c, a, e) in runs.items() if start <= parse_ts(c) <= anchor]
if mode == "fixture":  # LOCK 90677174: fixture has no event field
    prsel = sel_runs
else:  # live: denominator = runs event=pull_request in the window
    prsel = [rid for rid in sel_runs if runs[rid][2] == "pull_request"]
pr_runs = len(prsel)
rerun_pr_runs = sum(1 for rid in prsel if runs[rid][1] >= 2)
rerun_pct = round(100.0 * rerun_pr_runs / pr_runs, 2) if pr_runs else None
alarms = []
if infra >= th_cancel:
    alarms.append("infra_cancel_count %d >= %d" % (infra, th_cancel))
if rerun_pct is not None and rerun_pct > th_rerun:
    alarms.append("rerun_pct %s > %d" % (rerun_pct, th_rerun))
if m14 and len(m14) >= th_mjobs and pctile(m14, 95) > th_msec:
    alarms.append("macos_%dd p95 %ss > %ss with %d jobs >= %d"
                  % (th_mdays, num(pctile(m14, 95)), th_msec, len(m14), th_mjobs))
verdict = ("INCOMPLETE" if incomplete else "NO_DATA" if no_data
           else "ALARM" if alarms else "OK")
report = {"window_days": days, "repo": repo, "fixture": mode == "fixture",
          "queue": {o: os_stats(o) for o in ("ubuntu", "macos", "windows")},
          "macos_14d": {"p95_s": num(pctile(m14, 95)) if m14 else None, "jobs": len(m14)},
          "infra_cancel_count": infra, "other_cancel_count": other,
          "pr_runs": pr_runs, "rerun_pr_runs": rerun_pr_runs, "rerun_pct": rerun_pct,
          "verdict": verdict, "incomplete": incomplete, "no_data": no_data, "alarms": alarms}
if json_flag:
    print(json.dumps(report, indent=2))
else:
    lines = ["CI metrics: %s, window %dd (%s .. %s)%s" % (repo, days, start.strftime(FMT),
             anchor.strftime(FMT), " [fixture]" if mode == "fixture" else ""),
             "queue seconds by OS (reused-rerun phantoms excluded):"]
    for o in ("ubuntu", "macos", "windows"):
        s = report["queue"][o]
        lines.append("  %-8s p50=%-5s p95=%-5s max=%-5s jobs=%d"
                     % (o, fmt(s["p50_s"]), fmt(s["p95_s"]), fmt(s["max_s"]), s["jobs"]))
    rp = ("%s%%" % rerun_pct) if rerun_pct is not None else "n/a"
    lines += ["macOS %dd: p95=%s jobs=%d" % (th_mdays, fmt(report["macos_14d"]["p95_s"]), len(m14)),
              "cancelled: infra=%d other=%d" % (infra, other),
              "rerun: pr_runs=%d rerun_pr_runs=%d rerun_pct=%s" % (pr_runs, rerun_pr_runs, rp)]
    lines += ["ALARM: %s" % a for a in alarms]
    if incomplete:
        lines.append("INCOMPLETE: a jobs page reported total_count > 100;"
                     + " extra jobs not fetched (tracked in T-0204)")
    if no_data:
        lines.append("NO_DATA: no jobs in the window")
    lines += ["verdict: %s" % verdict,
              "note: ci-retry stage 2 is live (PR #47), so rerun% counts auto-reruns as well as manual ones."]
    print("\n".join(lines))
sys.exit(0 if verdict == "OK" else 1)
PY
