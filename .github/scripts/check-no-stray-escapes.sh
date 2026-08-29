#!/usr/bin/env bash
#
# A backslash-n where a line break was meant.
#
# It gets there when a multi-line edit is written by a tool that escapes the
# newline instead of making one. The shell then reads it as an argument: the
# command gains a stray "n", usually fails, and whatever was supposed to be on
# the next line runs as part of the same command.
#
# Three times in one sitting, and the worst of them was a diagnostic. A `tc`
# listing piped into `sed` gained the extra argument, sed failed, the `||`
# after it fired, and the tool reported that nothing was redirecting incoming
# traffic on a node where something was. A check that lies about the thing it
# exists to check is worse than no check, because it ends the looking.
#
# Bare ones only. Inside a format string - printf, awk, tr - a backslash-n is
# the point, so those lines are left alone. The accident always looks the same:
# a backslash-n with a space after it, sitting in the middle of a command.
set -euo pipefail

failed=0

for file in .github/workflows/*.yml .github/scripts/*.sh node/*.sh infra/*.sh; do
    [ -f "${file}" ] || continue

    # This file holds the pattern it is looking for, and found itself on the
    # first run. Skipped by name rather than by cleverness with the pattern,
    # because a guard that has to hide from itself is one edit away from
    # hiding a real one too.
    [ "${file}" = ".github/scripts/check-no-stray-escapes.sh" ] \
        && continue

    # -F, so the pattern is the two characters themselves. Written as a regular
    # expression the first time, where a backslash before an "n" means an "n" -
    # and this refused every English word ending in one. Fixing a guard that
    # cried wolf by making it cry louder was not the answer.
    while IFS= read -r hit; do
        echo "${file}:${hit}"
        failed=1
    done < <(grep -nF '\n ' "${file}" \
             | grep -vE 'printf|awk|[[:space:]]tr[[:space:]]' || true)
done

if [ "${failed}" -ne 0 ]; then
    echo
    echo "A backslash-n outside a format string is a line break that did not happen."
    echo "The shell reads it as an argument, and the rest of the line runs anyway."
    exit 1
fi

echo "ok: no stray escapes where a line break was meant"
