#!/usr/bin/env bash
#
# Refuses a release while a blocker is open - except the one the release
# closes.
#
# The gate used to be a single grep for an open status, which was right until
# the list contained "the first official APK has not been published". That item
# closes by publishing, so the grep refused the only run that could ever clear
# it: a deadlock nobody would notice until the day of the release, and one that
# reads in the log like an ordinary, correct refusal.
#
# Excluded by name, and by name only. An exclusion by shape - "anything whose
# closing condition mentions publishing" - would grow to fit whatever somebody
# wanted excluded next, which is how a gate stops being one.
set -euo pipefail

FILE="${1:-docs/release-blockers.md}"

# The single item whose closing condition is this publication itself.
SELF="Первая Официальная APK Не Опубликована"

section=""
open=""

while IFS= read -r line; do
    # Checked out with CRLF on Windows, so a heading arrives with a carriage
    # return attached and compares equal to nothing. Written without this the
    # exclusion below silently never matches, and the gate deadlocks exactly
    # as it did before - while looking like it works.
    line="${line%$'\r'}"

    case "${line}" in
        '## '*)
            section="${line#\#\# }"
            ;;
        '**Статус:** открыт'*)
            if [ "${section}" != "${SELF}" ]; then
                open="${open}  - ${section}"$'\n'
            fi
            ;;
    esac
done < "${FILE}"

if [ -n "${open}" ]; then
    echo "Release blockers are still open. The official channel must not publish around them:"
    printf '%s' "${open}"
    exit 1
fi

echo "ok: no release blocker is open except the one this publication closes"
