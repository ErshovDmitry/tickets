#!/usr/bin/env bash
# scripts/mutation.sh - reproducible mutation testing runner (gremlins v0.6.0).
#
# The script pins the gremlins version, resolves the binary from a controlled
# path, refuses to run on a dirty working tree or in parallel, runs a green
# baseline test suite, unleashes the mutants and prints a summary computed
# from the gremlins JSON report.
#
# The gremlins run mutates the working tree in place; every exit path rolls the
# mutated tracked files back (git checkout --) and removes the temporary report
# directory. Nothing is written into the repository except .cache/ (lock file
# and the optional binary directory).
#
# Linux/macOS only (flock).
#
# Exit codes: 0 run completed and summary printed, 1 error (environment, lock,
# dirty tree, baseline, binary version, mutation run), 2 bad arguments.
set -euo pipefail

# Locale must be set before the first numeric printf: under ru_RU.UTF-8 the bash
# builtin rejects fractional numbers ("73.33" is not a valid number).
export LC_ALL=C LANG=C

readonly GREMLINS_VERSION="v0.6.0"
readonly GREMLINS_MODULE="github.com/go-gremlins/gremlins"
readonly GREMLINS_PKG="${GREMLINS_MODULE}/cmd/gremlins"
readonly INSTALL_CMD="go install ${GREMLINS_PKG}@${GREMLINS_VERSION}"
readonly INSTALL_HINT="gremlins ${GREMLINS_VERSION} not found. Install it from the repository root: GOBIN=\"\$PWD/.cache/gobin\" ${INSTALL_CMD} (if GOBIN is already exported, install into it instead: GOBIN=\"\$GOBIN\" ${INSTALL_CMD}; an exported GOBIN must be an absolute path)"
readonly DEFAULT_SCOPE="internal/lock"
readonly WORKERS=2
readonly TIMEOUT_COEFFICIENT=20
readonly CACHE_DIR=".cache"
readonly LOCK_FILE="${CACHE_DIR}/mutation.lock"

die() {
  printf '%s\n' "$1" >&2
  exit "${2:-1}"
}

usage() {
  # One unquoted heredoc: GREMLINS_VERSION and INSTALL_CMD expand at call time.
  # Backslash-escaped dollars stay literal (shell variables the reader must set).
  cat <<EOF
Usage: mutation.sh [-h] [SCOPE...]

Reproducible mutation testing runner for gremlins ${GREMLINS_VERSION}.

  SCOPE       package or directory to mutate (default: internal/lock).
              The first scope is passed to gremlins as the analysis path,
              any further scope is passed as --coverpkg (comma
              separated list).
  -h          this help

Binary lookup: \$GOBIN/gremlins when GOBIN is set and not empty, otherwise
./.cache/gobin/gremlins (relative to the repository root). PATH and
\$(go env GOPATH)/bin are NOT searched.
Install gremlins into that same directory, run from the repository root:

  GOBIN="\$PWD/.cache/gobin" ${INSTALL_CMD}   (GOBIN unset or empty)
  GOBIN="\$GOBIN" ${INSTALL_CMD}   (GOBIN set; it must be absolute)

Guards, in order: exclusive lock on ./.cache/mutation.lock, clean working
tree, green baseline "go test -count=1", then the mutations themselves.
Exit codes: 0 run completed, 1 error, 2 bad arguments.
EOF
}

TMP_DIR=""
BIN=""
MUTATED=0

# Roll back the files mutated by gremlins (tracked files only).
# It is a no-op until the mutation run started: a dirty tree that is refused
# by the guard must keep the user changes untouched.
rollback_mutations() {
  local files
  [ "${MUTATED}" -eq 1 ] || return 0
  files="$(git diff --name-only 2>/dev/null || true)"
  [ -n "${files}" ] || return 0
  # shellcheck disable=SC2086 # word splitting is intended: a list of paths
  git checkout -- ${files} >/dev/null 2>&1 || true
}

cleanup() {
  local rc="${1:-0}"
  trap - EXIT INT TERM
  rollback_mutations
  if [ -n "${TMP_DIR}" ] && [ -d "${TMP_DIR}" ]; then
    rm -rf "${TMP_DIR}"
  fi
  exit "${rc}"
}

trap 'cleanup $?' EXIT
trap 'cleanup 130' INT
trap 'cleanup 143' TERM

# Print the value of a top level integer field of the gremlins JSON report.
json_int() {
  local file="$1" key="$2" out
  out="$(grep -oE "\"${key}\"[[:space:]]*:[[:space:]]*[0-9]+" "${file}" 2>/dev/null |
    head -n 1 | sed -E 's/.*:[[:space:]]*//')" || true
  [ -n "${out}" ] || return 1
  printf '%s\n' "${out}"
}

# Count the mutations with the given status in the gremlins JSON report.
json_status_count() {
  local file="$1" status="$2" out
  out="$(grep -o "\"${status}\"" "${file}" 2>/dev/null | wc -l | tr -d ' ')" || true
  printf '%s\n' "${out:-0}"
}

scope=()
while [ $# -gt 0 ]; do
  case "$1" in
    -h|--help)
      usage
      exit 0
      ;;
    --)
      shift
      while [ $# -gt 0 ]; do
        scope+=("$1")
        shift
      done
      ;;
    -*)
      printf 'unknown option: %s\n' "$1" >&2
      usage >&2
      exit 2
      ;;
    *)
      scope+=("$1")
      shift
      ;;
  esac
done

if [ "${#scope[@]}" -eq 0 ]; then
  scope=("${DEFAULT_SCOPE}")
fi

# Normalize the scopes: go tooling needs a path ("." or "./...") prefix.
normalized=()
for s in "${scope[@]}"; do
  s="${s%/}"
  case "${s}" in
    .*) normalized+=("${s}") ;;
    *) normalized+=("./${s}") ;;
  esac
done
scope=("${normalized[@]}")

ROOT="$(git rev-parse --show-toplevel 2>/dev/null)" ||
  die "not inside a git repository: run the script from the repository root"
cd "${ROOT}"

# --- environment: binary and pinned version ---------------------------------

if [ -n "${GOBIN:-}" ]; then
  BIN="${GOBIN%/}/gremlins"
else
  BIN="./${CACHE_DIR}/gobin/gremlins"
fi

if [ ! -x "${BIN}" ]; then
  die "${INSTALL_HINT}"
fi

mod_info="$(go version -m "${BIN}" 2>/dev/null || true)"
if ! grep -Eq 'mod[[:space:]]+github\.com/go-gremlins/gremlins[[:space:]]+v0\.6\.0' <<<"${mod_info}"; then
  die "gremlins binary at ${BIN} is not ${GREMLINS_MODULE} ${GREMLINS_VERSION}. Rebuild it: ${INSTALL_CMD}"
fi

bin_version="$(awk -v mod="${GREMLINS_MODULE}" '$1 == "mod" && $2 == mod { print $3; exit }' <<<"${mod_info}")"

printf 'go: %s\n' "$(go version)"
printf 'gremlins: %s (from go version -m)\n' "${bin_version:-${GREMLINS_VERSION}}"

# --- guard 1: exclusive lock --------------------------------------------------

mkdir -p "${CACHE_DIR}"
exec 9>"${LOCK_FILE}"
if ! flock -n 9; then
  die "another mutation run is in progress (${LOCK_FILE} is locked)"
fi

# --- guard 2: clean working tree ----------------------------------------------

dirty="$(git status --porcelain -- . ':(exclude).cache' || true)"
if [ -n "${dirty}" ]; then
  printf '%s\n' "${dirty}" >&2
  die "refusing to mutate a dirty working tree"
fi

# --- guard 3: green baseline --------------------------------------------------

printf 'baseline: go test -count=1 %s\n' "${scope[*]}"
if ! go test -count=1 "${scope[@]}"; then
  die "baseline test run failed, refusing to mutate"
fi

# --- mutations ----------------------------------------------------------------

TMP_DIR="$(mktemp -d)"
REPORT="${TMP_DIR}/report.json"
printf 'report: %s\n' "${REPORT}"

cover_pkgs=""
if [ "${#scope[@]}" -gt 1 ]; then
  cover_pkgs="$(printf '%s,' "${scope[@]:1}")"
  cover_pkgs="${cover_pkgs%,}"
fi

unleash_cmd=("${BIN}" unleash --workers "${WORKERS}" --timeout-coefficient "${TIMEOUT_COEFFICIENT}" \
  -o "${REPORT}" "${scope[0]}")
if [ -n "${cover_pkgs}" ]; then
  unleash_cmd+=(--coverpkg "${cover_pkgs}")
fi
printf 'unleash: %s\n' "${unleash_cmd[*]}"
MUTATED=1
if ! "${unleash_cmd[@]}"; then
  die "gremlins unleash failed"
fi

# --- post run: the tree must be clean again -----------------------------------

dirty="$(git status --porcelain -- . ':(exclude).cache' || true)"
if [ -n "${dirty}" ]; then
  printf '%s\n' "${dirty}" >&2
  die "mutations left in working tree"
fi

# --- summary (numbers come from the JSON report only) -------------------------

if [ ! -s "${REPORT}" ]; then
  die "gremlins report is missing or empty: ${REPORT}"
fi

killed="$(json_int "${REPORT}" mutants_killed)" || die "cannot read mutants_killed from ${REPORT}"
lived="$(json_int "${REPORT}" mutants_lived)" || die "cannot read mutants_lived from ${REPORT}"
not_covered="$(json_int "${REPORT}" mutants_not_covered)" || die "cannot read mutants_not_covered from ${REPORT}"
not_viable="$(json_int "${REPORT}" mutants_not_viable)" || not_viable=0
timed_out="$(json_status_count "${REPORT}" "TIMED OUT")"

total=$((killed + lived))
efficacy="$(awk -v k="${killed}" -v l="${lived}" 'BEGIN { if (k + l > 0) printf "%.2f", 100 * k / (k + l); else printf "0.00" }')"

printf '\n== Mutation summary (gremlins %s) ==\n' "${GREMLINS_VERSION}"
printf 'Scope:       %s\n' "${scope[*]}"
printf 'Report:      %s\n' "${REPORT}"
printf 'Note:        the report is deleted on exit (temporary directory)\n'
printf 'Total:       %s   (killed + lived; not covered, timed out and not viable are NOT included)\n' "${total}"
printf 'Killed:      %s\n' "${killed}"
printf 'Lived:       %s\n' "${lived}"
printf 'Timed out:   %s\n' "${timed_out}"
printf 'Not covered: %s\n' "${not_covered}"
printf 'Not viable:  %s\n' "${not_viable}"
printf 'Efficacy:    %s%%\n' "${efficacy}"
