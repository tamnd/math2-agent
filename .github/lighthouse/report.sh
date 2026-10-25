#!/bin/sh
# report.sh M2 OUT builds the wiki of the fixtures with the m2 binary M2, tests it against the targets of spec 13.7 and writes a Markdown report to OUT. It puts the Lighthouse reports in the directory lighthouse-report next to OUT. It exits with 1 when a page misses a target. Run it from the root of the repository, after npm ci in this directory. Set LHCI to use another Lighthouse CI binary.
set -eu
m2=$1
out=$2
here=$(cd "$(dirname "$0")" && pwd)
lhci=${LHCI:-$here/node_modules/.bin/lhci}
work=$(mktemp -d)
url=http://127.0.0.1:8000/math2

"$m2" build -root testdata/nodes/valid -out "$work/build" > /dev/null
"$m2" site build -content testdata/nodes/valid -spec spec -build "$work/build" -out "$work/site" -url "$url" > /dev/null

status=0
"$m2" site check -url "$url" -json "$work/site" > "$work/check.json" || status=1

"$m2" site serve -url "$url" -addr 127.0.0.1:8000 "$work/site" > /dev/null &
server=$!
trap 'kill "$server"; rm -rf "$work"' EXIT
(cd "$work" && "$lhci" autorun --config="$here/lighthouserc.json") > "$work/lhci.txt" 2>&1 || status=1

mkdir -p "$(dirname "$out")"
rm -rf "$(dirname "$out")/lighthouse-report"
cp -R "$work/lighthouse-report" "$(dirname "$out")/"
node "$here/summary.mjs" "$work/check.json" "$work/lighthouse-report" "$here/lighthouserc.json" "$("$m2" version)" > "$out" || status=1
if [ "$status" != 0 ]; then
  cat "$work/lhci.txt" >&2
fi
exit "$status"
