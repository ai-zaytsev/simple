#!/usr/bin/env bash
#
# An address must be masked before anything can print it.
#
# The first version of this looked for one shape of the mistake: publishing an
# address as a step output. It passed while a workflow two files away printed
# two live node addresses with a plain echo, and the addresses reached a public
# log an hour after the guard was written.
#
# So it checks the mistake now rather than the shape it took the first time:
# any line that could put an address in front of somebody - published as an
# output, echoed, or added to a job summary - must be preceded by a mask of the
# same variable.
#
# Why it matters at all: the repository is public, so its Actions logs are. A
# node's address is the single fact somebody intending to block this service
# would most like to be handed, and the cover domain in front of the machine
# exists so that the address is not the obvious thing about it.
set -euo pipefail

# Variable names that hold somewhere a machine can be reached. Matched as whole
# words inside ${...} so that unrelated names - "hostname of the provider API" -
# do not drag every workflow into this.
names='address|addresses|ip|ips|node_ip|host|hosts|server_ip'

failed=0

for workflow in .github/workflows/*.yml; do
    while read -r line; do
        [ -n "${line}" ] || continue

        text=$(sed -n "${line}p" "${workflow}")

        # The variable this line would put in front of somebody.
        #
        # Case-insensitively, which it was not at first: workflows name their
        # environment in capitals, so the version that looked only for lower
        # case walked straight past a step printing ADDRESS into the summary of
        # every run. A guard that reads source has to read it the way it is
        # written, not the way the example that prompted it was written.
        variable=$(printf '%s' "${text}" \
            | grep -oiE '\$\{?('"${names}"')[:}]' | head -1 \
            | tr -d '${}:')
        [ -n "${variable}" ] || continue

        # A line that only masks is the fix, not the fault.
        case "${text}" in *add-mask*) continue ;; esac

        # Secrets are masked by the platform, so a line printing one is safe.
        case "${text}" in *secrets.*) continue ;; esac

        # Named on purpose, with the reason written above the line.
        #
        # An exception with a reason beside it is a decision somebody can argue
        # with. A blanket rule with no way out gets worked around instead, and
        # the workaround is where the next one hides: masking the answer of a
        # DNS check would leave a check that checks nothing.
        from=$((line > 4 ? line - 4 : 1))
        if sed -n "${from},${line}p" "${workflow}" | grep -q 'address-in-the-clear:'; then
            continue
        fi

        # Anywhere above, rather than within a handful of lines.
        #
        # A mask applies to everything the job prints afterwards, later steps
        # included, so demanding that it sit beside the print was a rule about
        # tidiness wearing the clothes of a rule about safety. It reported five
        # places that were masked perfectly well from another step.
        if sed -n "1,${line}p" "${workflow}" \
             | grep -qi "add-mask::\${\?${variable}"; then
            continue
        fi

        echo "${workflow}:${line}: an address is shown without being masked first"
        echo "  ${text}"
        failed=1
    done < <(grep -niE '(echo|printf).*\$\{?('"${names}"')[:}]' "${workflow}" \
             | grep -E 'GITHUB_OUTPUT|GITHUB_STEP_SUMMARY|echo "|printf ' \
             | cut -d: -f1)
done

if [ "${failed}" -ne 0 ]; then
    echo
    echo "Put: echo \"::add-mask::\${the_variable}\" before anything prints it."
    exit 1
fi

echo "ok: every address is masked before anything shows it"
