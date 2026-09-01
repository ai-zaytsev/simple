#!/usr/bin/env bash
#
# Nothing under the database prefix may be uploaded readable by the world.
#
# The bucket holds two kinds of object that could not be more different. The
# APK must be public - it is served to strangers by design, and publish-apk.yml
# passes --acl public-read to say so. A database dump must never be, and it
# carries accounts and payments.
#
# One flag separates them. That is a thin thing to rely on memory for, so this
# refuses the combination outright: if a line mentions the db/ prefix and a
# public ACL, the build stops before anybody runs it.
#
# It cannot see what an operator types at a shell, and it is not trying to. It
# guards the path this repository takes.
set -euo pipefail

failed=0

for file in .github/workflows/*.yml .github/scripts/*.sh; do
    [ -f "${file}" ] || continue
    [ "${file}" = ".github/scripts/check-backup-privacy.sh" ] && continue

    # Same line, both facts. An upload names its key and its ACL together, so
    # the pairing is visible without understanding the whole script.
    while IFS= read -r hit; do
        echo "${file}:${hit}"
        failed=1
    done < <(grep -nE 'db/' "${file}" | grep -E 'acl[[:space:]=]+public|public-read' || true)
done

if [ "${failed}" -ne 0 ]; then
    echo
    echo "A database backup would be world-readable. The dump is encrypted, but"
    echo "an object nobody can read by accident is one fewer thing to be wrong"
    echo "about."
    exit 1
fi

echo "ok: nothing publishes the database prefix"
