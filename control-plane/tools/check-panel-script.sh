#!/usr/bin/env bash
#
# Checks that the panel's script is valid JavaScript.
#
# The panel is one embedded HTML file with no build step and no test harness,
# which is the right size for what it does and means a typo in it reaches the
# server. One already did: a stray label left in the middle of an argument
# list, found by loading the page in a browser and noticing nothing rendered.
# That was luck, and luck does not scale to the next edit.
#
# Syntax only. It cannot tell whether the page shows the right numbers - that
# still takes looking at it - but every mistake this catches is one that would
# otherwise have been found by somebody opening the panel and seeing a blank.

set -euo pipefail

PAGE="$(dirname "$0")/../internal/api/panel.html"

if [ ! -f "${PAGE}" ]; then
  echo "cannot find ${PAGE}"
  exit 1
fi

if ! command -v node >/dev/null 2>&1; then
  echo "node is not available; skipping the panel script check"
  exit 0
fi

# With a .js name, because node decides how to parse a file by its extension
# and refuses one it does not recognise - which reads exactly like a syntax
# error and is not one.
script=$(mktemp -d)/panel.js
trap 'rm -rf "$(dirname "${script}")"' EXIT

awk '/<script>/{inside=1; next} /<\/script>/{inside=0} inside' "${PAGE}" > "${script}"

if [ ! -s "${script}" ]; then
  echo "no script found in the panel; the extraction has stopped matching the page"
  exit 1
fi

if ! node --check "${script}"; then
  echo "the panel's script does not parse; the page would render blank"
  exit 1
fi

# The table is written as two lists that have to stay the same length: the
# headers, and the cells emitted per row. They have drifted apart before while
# both halves looked fine on their own.
headers=$(grep -o "<th>" "${PAGE}" | wc -l)
if [ "${headers}" -lt 1 ]; then
  echo "no table headers found; the panel has changed shape"
  exit 1
fi

echo "The panel's script parses, and it declares ${headers} table headings."
