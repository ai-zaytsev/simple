#!/usr/bin/env bash
#
# Nothing operational depends on simple-vpn.download.
#
# That domain is blocked in Russia. Moving off it took a new site domain, a new
# sender in Brevo, a new support address baked into a release, and a new filename
# convention for published APKs. Each of those was a place where the old name
# was holding something up without anybody having written it down.
#
# A cleanup is true on the day it lands. What undoes it is ordinary: an old URL
# copied into a new workflow, an example pasted from a spec. So the name is
# allowed only where it is history or protection, and refused everywhere else.
set -euo pipefail

name='simple-vpn.download'

# Where the name belongs, and why:
#   specs/                      history of what was built, not what runs
#   docs/release-blockers.md    a closed blocker, accepted on the old domain
#   docs/business-owner-operations.md
#                               the instructions for retiring it have to name it
#   docs/tech-debt.md           closed entries explaining the move
#   .github/scripts/check-site-says-nothing-that-gets-it-blocked.sh
#                               refuses the name on the site
#   .github/scripts/check-nothing-depends-on-the-old-domain.sh
#                               this file
#   .github/workflows/baseline-checks.yml
#                               the reason written beside the site guard
#   sites/official/tools/update_release_manifest.py
#                               why 0.1.0 keeps the name it was published under
#
# specs/ is a prefix and everything else is a whole path, and both are followed
# by the colon git grep puts after the filename. The first version wrote specs/
# like the others and so demanded a file named exactly "specs/" - it refused
# every line of history on a clean tree, which is how it was caught.
allowed='^(\./)?(specs/[^:]*|docs/release-blockers\.md|docs/business-owner-operations\.md|docs/tech-debt\.md|\.github/scripts/check-site-says-nothing-that-gets-it-blocked\.sh|\.github/scripts/check-nothing-depends-on-the-old-domain\.sh|\.github/workflows/baseline-checks\.yml|sites/official/tools/update_release_manifest\.py):'

hits=$(git grep -nF "${name}" -- . ':!*.apk' 2>/dev/null | grep -vE "${allowed}" || true)

if [ -n "${hits}" ]; then
    echo "The retired domain is back somewhere that runs:"
    printf '%s\n' "${hits}" | sed 's/^/  /'
    echo
    echo "It is blocked in Russia. Whatever this points at will stop working there,"
    echo "and the failure will look like the thing it points at being broken."
    exit 1
fi

echo "ok: nothing operational depends on ${name}"
