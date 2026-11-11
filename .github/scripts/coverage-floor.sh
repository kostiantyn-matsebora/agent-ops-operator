#!/usr/bin/env bash
# Hold a statement-coverage floor on one package, read from a Go profile.
#
#   coverage-floor.sh <profile> <path-fragment> <floor-percent>
#
# EACH BLOCK COUNTS ONCE. With -coverpkg=./... every test binary writes every
# block of every package, hit only by the binary that exercises it, so the
# profile lists a block once per binary. Summing the lines reads a fully covered
# package as covered by 1/N of the binaries. A block is covered when ANY line for
# it has a count above zero, and weighs its statement count.
set -euo pipefail
profile=$1 frag=$2 floor=$3

pct=$(awk -v frag="$frag" '
  NR > 1 && index($1, frag) {
    n[$1] = $2
    if ($3 > 0) hit[$1] = 1
  }
  END {
    for (k in n) { t += n[k]; if (k in hit) c += n[k] }
    if (t) printf "%.1f", 100 * c / t; else print 0
  }' "$profile")

echo "${frag} statement coverage: ${pct}%"
awk -v p="$pct" -v f="$floor" 'BEGIN { exit !(p >= f) }' || { echo "below the ${floor}% floor"; exit 1; }
