#!/usr/bin/env bash
#
# The panel's hourly chart must say which window it covers.
#
# It was headed "Сутки по часам" while the query behind it takes the busiest
# each hour of the day has been over seven days. The same page then showed
# "пик за сутки | 60" above bars reaching 258, and both numbers were right -
# the heading described the wrong one. Read after a deploy, it says the day
# was four times busier than the line above it claims.
#
# Two ends again: the Go comment beside the query says "over the week", the
# Python heading said "сутки". Nothing connected them, so they drifted.
set -euo pipefail

PANEL=".github/scripts/read_panel.py"
QUERY="control-plane/internal/store/capacity.go"
failed=0

heading=$(grep -A 12 'hours = c.get("by_hour")' "${PANEL}" | grep -m1 'say("###')

case "${heading}" in
    *неделю*) ;;
    "") echo "${PANEL}: the by_hour chart has no heading to check"; failed=1 ;;
    *) echo "${PANEL}: the by_hour heading does not say the week it covers:"
       echo "  ${heading}"
       failed=1 ;;
esac

# And the other end: if the query stops being a week, the heading is wrong
# again in the opposite direction.
if ! grep -A 12 'func (s \*Store) hourlyPeaks' "${QUERY}" | grep -q '7\*24\*time.Hour'; then
    echo "${QUERY}: hourlyPeaks no longer covers seven days; the panel heading says it does"
    failed=1
fi

if [ "${failed}" -ne 0 ]; then
    echo
    echo "A chart whose label and query disagree does not fail. It answers,"
    echo "and the answer is wrong by exactly the amount nobody checks."
    exit 1
fi

echo "ok: the panel's hourly chart names the window its query covers"
