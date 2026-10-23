#!/usr/bin/env bash
#
# JS-versus-Go comparison on one realistic workload.
#
# Both sides do the same thing: parse a JSON filter, compile it to a WHERE
# clause, quote an eight-column select list and three identifiers, and assemble
# a paginated list query. Wall time and peak RSS are reported for each.
#
#   go/bench/compare.sh [iterations]
#
# Run it from anywhere; paths are resolved relative to this script.
set -euo pipefail

ITERATIONS="${1:-200000}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GODIR="$(dirname "$HERE")"
REPO="$(dirname "$GODIR")"

export PATH="$HOME/.local/go/bin:$PATH"

command -v go >/dev/null || { echo "go not on PATH (try: export PATH=\"\$HOME/.local/go/bin:\$PATH\")" >&2; exit 1; }
command -v bun >/dev/null || { echo "bun not on PATH" >&2; exit 1; }

echo "workload: build a paginated list query from a JSON filter"
echo "iterations: $ITERATIONS (plus 2000 warm-up on each side)"
echo

# /usr/bin/time gives the kernel's view of peak RSS for the whole process, which
# is the honest number for a runtime comparison: it includes the JIT and the
# garbage collector, not just what the program thinks it allocated.
TIMEFMT="max_rss_kb=%M wall_s=%e"

echo "--- JavaScript (bun) ---"
/usr/bin/time -f "$TIMEFMT" bun "$HERE/workload.ts" "$ITERATIONS" 2>&1

echo
echo "--- Go ---"
cd "$GODIR"
go build -o "$HERE/.workload-go" ./bench/workload
/usr/bin/time -f "$TIMEFMT" "$HERE/.workload-go" "$ITERATIONS" 2>&1
rm -f "$HERE/.workload-go"

echo
echo "ns_per_op in the JSON lines above is the like-for-like figure;"
echo "max_rss_kb is the process high-water mark including runtime overhead."
