#!/usr/bin/env bash
# scripts/crap.sh - advisory CRAP ranking for production Go functions.
#
# CRAP = CC^2 * (1 - cov/100)^3 + CC   (skill "crap": an ordering hint, never a gate)
#   CC  - cyclomatic complexity reported by gocyclo v0.6.0
#   cov - per-function statement coverage from go test ./... + go tool cover -func
#
# Read-only: writes nothing into the repository (temporary dir only), no network.
# Linux/macOS only; Windows is out of scope (gocyclo/cover path handling).
#
# Exit codes: 0 report, 1 environment/baseline failure or empty join, 2 bad arguments.
set -euo pipefail

# Locale must be set before the first numeric printf: under ru_RU.UTF-8 the bash
# builtin rejects "21.824" (недопустимое число) and prints "21,0".
export LC_ALL=C LANG=C

readonly KNOWN_PKGS="cli domain lock main paths store"
readonly INSTALL_HINT="gocyclo not found. Install: go install github.com/fzipp/gocyclo/cmd/gocyclo@v0.6.0"

die() {
  printf '%s\n' "$1" >&2
  exit "${2:-1}"
}

usage() {
  cat <<'EOF'
Usage: crap.sh [-n TOP] [--pkg NAME[,NAME...]] [--root DIR] [-h]

Advisory CRAP report: CRAP = CC^2 * (1 - cov/100)^3 + CC for every production
function, highest risk first. No thresholds, no gate.

  -n TOP     rows per block (default 20)
  --pkg      restrict to gocyclo package names, comma separated, repeatable
             (known: cli domain lock main paths store)
  --root DIR Go module root containing go.mod (default: parent of this script)
  -h         this help

gocyclo lookup order: $GOCYCLO, then PATH, then $HOME/go/bin/gocyclo (v0.6.0).
Exit codes: 0 report, 1 environment/baseline/empty join, 2 bad arguments.
EOF
}

top=20
pkg_sel=""
root=""

while [ $# -gt 0 ]; do
  case $1 in
    -h|--help)
      usage
      exit 0
      ;;
    -n)
      [ $# -ge 2 ] || die "-n requires a value" 2
      case $2 in
        ''|*[!0-9]*) die "-n must be an integer >= 1, got '$2'" 2 ;;
      esac
      [ "$2" -ge 1 ] || die "-n must be an integer >= 1, got '$2'" 2
      top=$2
      shift 2
      ;;
    --pkg)
      [ $# -ge 2 ] || die "--pkg requires a value" 2
      [ -n "$2" ] || die "--pkg requires a value" 2
      IFS=, read -r -a names <<<"$2"
      for name in "${names[@]}"; do
        case " $KNOWN_PKGS " in
          *" $name "*) ;;
          *) die "unknown --pkg '$name' (known: $KNOWN_PKGS)" 2 ;;
        esac
        pkg_sel="${pkg_sel:+$pkg_sel,}$name"
      done
      shift 2
      ;;
    --root)
      [ $# -ge 2 ] || die "--root requires a value" 2
      [ -n "$2" ] || die "--root requires a non-empty value" 2
      root=$2
      shift 2
      ;;
    *)
      die "unexpected argument '$1' (see -h)" 2
      ;;
  esac
done

# Resolve this script through symlinks without relying on readlink -f (macOS).
self=$0
while [ -L "$self" ]; do
  target=$(readlink "$self")
  case $target in
    /*) self=$target ;;
    *) self=$(dirname "$self")/$target ;;
  esac
done
script_dir=$(cd -P "$(dirname "$self")" && pwd)
[ -n "$root" ] || root=$script_dir/..
[ -f "$root/go.mod" ] || die "not a Go module root (no go.mod): $root" 2
root=$(cd -P "$root" && pwd)

gy=${GOCYCLO:-}
if [ -z "$gy" ]; then
  if gy=$(command -v gocyclo 2>/dev/null); then :; else gy=$HOME/go/bin/gocyclo; fi
fi
[ -x "$gy" ] || die "$INSTALL_HINT" 1

tmpdir=$(mktemp -d) || die "cannot create a temporary directory" 1
trap 'rm -rf "$tmpdir"' EXIT

cd "$root" || die "cannot enter root: $root" 1
# stdout is reserved for the report; test chatter goes to stderr.
if ! go test ./... -coverprofile="$tmpdir/cover.out" >&2; then
  die "baseline tests failed: CRAP/coverage evidence is invalid" 1
fi

go tool cover -func="$tmpdir/cover.out" >"$tmpdir/cover-func.txt"
# Tolerate CRLF line endings and a UTF-8 BOM in go.mod (octal escapes are POSIX awk).
mod=$(awk 'NR == 1 { sub(/^\357\273\277/, "") }
  { sub(/\r$/, ""); if ($1 == "module") { print $2; exit } }' go.mod)

# cover -func: "path:line:" TAB+ func TAB+ "cov%"; the total line is kept apart.
# Path normalization, identical on both join sides: (a) strip "$root/",
# (b) strip the go.mod module prefix, (c) fall back to the first
# "internal/" or "cmd/" segment so divergent prefixes still converge.
awk -F'\t+' -v mod="$mod" -v root="$root/" -v tfile="$tmpdir/total.txt" '
  function norm(p,   i, j) {
    if (index(p, root) == 1) return substr(p, length(root) + 1)
    if (mod != "" && index(p, mod "/") == 1) return substr(p, length(mod) + 2)
    i = index(p, "internal/"); j = index(p, "cmd/")
    if (i == 0) i = j; else if (j > 0 && j < i) i = j
    if (i > 0) p = substr(p, i)
    return p
  }
  $1 == "total:" { print $3 > tfile; next }
  {
    split($1, a, ":"); p = norm(a[1])
    c = $3; sub(/%$/, "", c)
    print p "\t" a[2] "\t" $2 "\t" c
  }' "$tmpdir/cover-func.txt" >"$tmpdir/cov.tsv"

"$gy" "$root" >"$tmpdir/cyc.raw" || die "gocyclo failed: $gy" 1
# gocyclo: "CC pkg func path:line:col"; tests are dropped here, before the join.
awk -v mod="$mod" -v root="$root/" '
  function norm(p,   i, j) {
    if (index(p, root) == 1) return substr(p, length(root) + 1)
    if (mod != "" && index(p, mod "/") == 1) return substr(p, length(mod) + 2)
    i = index(p, "internal/"); j = index(p, "cmd/")
    if (i == 0) i = j; else if (j > 0 && j < i) i = j
    if (i > 0) p = substr(p, i)
    return p
  }
  index($4, "_test.go:") { next }
  {
    split($4, a, ":"); p = norm(a[1])
    print $1 "\t" $2 "\t" $3 "\t" p "\t" a[2]
  }' "$tmpdir/cyc.raw" >"$tmpdir/cyc.tsv"

# Join on (path, line), fall back to (path, func); leftovers are unmeasured.
awk -F'\t' -v pkgs="$pkg_sel" -v ufile="$tmpdir/unmeasured.tsv" -v sfile="$tmpdir/summary.tsv" '
  function inscope(p) { return pkgs == "" || index("," pkgs ",", "," p ",") > 0 }
  FILENAME == ARGV[1] {
    cv[$1 SUBSEP $2] = $4
    cf[$1 SUBSEP $3] = $4
    next
  }
  {
    if (!inscope($2)) next
    key = $4 SUBSEP $5
    found = 0
    if (key in cv) { cov = cv[key]; found = 1 }
    else if (($4 SUBSEP $3) in cf) { cov = cf[$4 SUBSEP $3]; found = 1 }
    if (!found) {
      print $2 "\t" $1 "\t" $3 "\t" $4 ":" $5 > ufile
      next
    }
    crap = $1 * $1 * (1 - cov / 100) ^ 3 + $1
    count++
    crapsum += crap
    ccsum += $1
    covsum += cov
    printf "%.1f\t%d\t%.1f\t%s\t%s\t%s:%s\n", crap, $1, cov, $2, $3, $4, $5
  }
  END {
    close(ufile)
    if (count > 0) printf "%.1f\t%.1f\t%.1f\t%d\n", crapsum / count, ccsum / count, covsum / count, count > sfile
  }' "$tmpdir/cov.tsv" "$tmpdir/cyc.tsv" >"$tmpdir/rows.tsv"

[ -s "$tmpdir/rows.tsv" ] || die "no measurable functions" 1
sort -k1,1nr -k2,2nr -k6,6 "$tmpdir/rows.tsv" >"$tmpdir/sorted.tsv"
[ -f "$tmpdir/unmeasured.tsv" ] || : >"$tmpdir/unmeasured.tsv"
sort -k1,1 -k3,3 "$tmpdir/unmeasured.tsv" >"$tmpdir/unmeasured.sorted.tsv"

read -r avg_crap avg_cc avg_cov mcount <"$tmpdir/summary.tsv"
cov_total=$(cat "$tmpdir/total.txt" 2>/dev/null || printf 'n/a')
um_count=$(wc -l <"$tmpdir/unmeasured.sorted.tsv")
uc_count=$(awk -F'\t' '$3 + 0 == 0' "$tmpdir/sorted.tsv" | wc -l)
[ -n "$pkg_sel" ] || pkg_sel="all packages"

# Report: the column header is line 1, data rows from line 2 (no title lines).
awk -F'\t' -v n="$top" -v ac="$avg_crap" -v acc="$avg_cc" -v acov="$avg_cov" \
    -v mcount="$mcount" -v total="$cov_total" -v root="$root" -v scope="$pkg_sel" '
  BEGIN { printf "%-8s %-4s %-6s %-8s %-32s %s\n", "CRAP", "CC", "cov%", "pkg", "function", "location" }
  {
    rows++
    if (rows <= n) printf "%-8.1f %-4d %-6.1f %-8s %-32s %s\n", $1, $2, $3, $4, $5, $6
  }
  END {
    if (rows > n) printf "... %d more (top %d of %d; raise -n to see them)\n", rows - n, n, rows
    printf "\n"
    printf "measured: %d | avg CRAP %s | avg CC %s | avg cov %s%%\n", mcount, ac, acc, acov
    printf "baseline coverage: %s of statements | scope: %s | root: %s\n", total, scope, root
  }' "$tmpdir/sorted.tsv"

awk -F'\t' -v n="$top" -v cnt="$um_count" '
  BEGIN { printf "\nunmeasured: %d functions with no coverage line (not zero coverage)\n", cnt }
  {
    if (++rows <= n) printf "%-8s %-4d %-6s %-8s %-32s %s\n", "-", $2, "-", $1, $3, $4
  }
  END { if (rows > n) printf "... %d more\n", rows - n }' "$tmpdir/unmeasured.sorted.tsv"

awk -F'\t' -v n="$top" -v cnt="$uc_count" '
  BEGIN { printf "\nuncovered: %d measured functions at 0.0%% coverage\n", cnt }
  $3 + 0 == 0 {
    if (++rows <= n) printf "%-8.1f %-4d %-6.1f %-8s %-32s %s\n", $1, $2, $3, $4, $5, $6
  }
  END { if (rows > n) printf "... %d more\n", rows - n }' "$tmpdir/sorted.tsv"
