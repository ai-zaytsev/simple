#!/usr/bin/env bash
#
# An address written to a step output must be masked first.
#
# A step output is not a secret. Every later step that takes one through `env:`
# prints it in that step's environment block, so an address reaches a public
# log a dozen times without anybody echoing it once. That is how it got there:
# one workflow masked its addresses and another, written later, did not, and
# nothing said the two disagreed.
#
# Checked by reading the workflows rather than by trusting the habit, because
# the habit is what failed.
set -euo pipefail

failed=0

for workflow in .github/workflows/*.yml; do
    # Lines that publish an output whose name is about where a machine is.
    while IFS=: read -r line _; do
        [ -n "${line}" ] || continue

        # The variable being published, so the mask can be matched to it.
        variable=$(sed -n "${line}p" "${workflow}" \
            | sed -E 's/.*\$\{?([A-Za-z_][A-Za-z0-9_]*)\}?.*\$\{?GITHUB_OUTPUT.*/\1/')

        from=$((line > 12 ? line - 12 : 1))
        if sed -n "${from},${line}p" "${workflow}" | grep -q "add-mask::\${\?${variable}"; then
            continue
        fi

        echo "${workflow}:${line}: an address is published without being masked first"
        sed -n "${line}p" "${workflow}"
        failed=1
    done < <(grep -nE '^\s*echo "[a-z_]*(ip|address|host)[a-z_]*=\$\{?[A-Za-z_]' "${workflow}" \
             | grep 'GITHUB_OUTPUT' | cut -d: -f1 | sed 's/$/:/')
done

if [ "${failed}" -ne 0 ]; then
    echo
    echo "Add: echo \"::add-mask::\${the_variable}\" before writing it to GITHUB_OUTPUT."
    exit 1
fi

echo "ok: every published address is masked first"
