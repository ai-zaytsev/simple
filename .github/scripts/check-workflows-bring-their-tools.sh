#!/usr/bin/env bash
#
# A workflow that runs a file from this repository has to check it out first.
#
# Written after Purchases changed a live setting and then failed on the line
# that printed the result, because the renderer it called was not on the
# runner. That failure has the worst possible shape: the run is red, so it
# reads as "nothing happened", and something had already happened.
#
# panel-check.yml carries a comment saying exactly this, added after it failed
# the same way. A note in one file does not protect the next one; this does.
set -euo pipefail

failed=0

for file in .github/workflows/*.yml; do
    [ -f "${file}" ] || continue

    # Does it run something out of the repository at all?
    if ! grep -qE '(python3|bash|sh) +\.github/scripts/|bash +(infra|node)/' "${file}"; then
        continue
    fi

    if ! grep -q 'uses: actions/checkout' "${file}"; then
        echo "${file}: runs a file from this repository and never checks it out"
        failed=1
    fi
done

if [ "${failed}" -ne 0 ]; then
    echo
    echo "The file will not be on the runner, and the step that needs it fails"
    echo "last - after everything before it has already taken effect."
    exit 1
fi

echo "ok: every workflow that runs a repository file checks it out"
