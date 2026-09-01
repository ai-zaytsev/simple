#!/usr/bin/env bash
#
# The one signing certificate an APK carries, read out of apksigner's output.
#
# One file because there were two copies of this parse - one before uploading
# and one checking the public copy afterwards - and the first real release met
# both. The anchored pattern they shared, "Signer #1 certificate SHA-256
# digest:", matched nothing on current build tools, which print one block per
# supported SDK range instead:
#
#   Signer (minSdkVersion=24, maxSdkVersion=32) certificate SHA-256 digest: ...
#
# I fixed the copy that failed first, the run got further, and the second copy
# failed the same way. Two places holding one rule is the defect; the pattern
# was only how it showed.
#
# Usage: apk-certificate.sh <apksigner-output-file>
# Prints the lowercase fingerprint, or explains itself and exits non-zero.
set -euo pipefail

FILE="${1:?usage: apk-certificate.sh <apksigner-output-file>}"

mapfile -t certs < <(
    grep -oiE 'certificate SHA-256 digest: *[0-9a-f]{64}' "${FILE}" \
        | grep -oiE '[0-9a-f]{64}' | tr 'A-F' 'a-f' | sort -u
)

if [ "${#certs[@]}" -eq 0 ]; then
    # Printed, because the version that did not print cost an hour. A check
    # that knows what it read and will not say it turns a changed line of
    # format into a hunt.
    echo "Could not read the signing certificate fingerprint. apksigner said:" >&2
    sed 's/^/  /' "${FILE}" >&2
    exit 1
fi

# More than one distinct certificate would mean the APK is signed by different
# keys for different Android versions. Nothing here builds that on purpose, and
# it would split the user base into two update paths - we hold the key to one.
if [ "${#certs[@]}" -ne 1 ]; then
    echo "The APK carries more than one signing certificate:" >&2
    printf '  %s\n' "${certs[@]}" >&2
    exit 1
fi

printf '%s\n' "${certs[0]}"
