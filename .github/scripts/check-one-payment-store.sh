#!/usr/bin/env bash
#
# Core and the readback that checks Core must use the same YooKassa store.
#
# The deploy hands Core one shop's credentials; payment-acceptance.yml asks
# YooKassa about the payments Core recorded. Point them at different stores and
# the readback reports every real payment as absent from the provider - not a
# check that fails, a check that lies, which is the shape of failure this
# repository has already paid for four times.
#
# The two acceptance harnesses are exempt by name, and the reason is written
# next to their credentials: they repeat requests against fixtures that exist
# only in the test store, and repeating them against the live one would move
# real money.
set -euo pipefail

CORE=".github/workflows/deploy-control-plane.yml"
READBACK=".github/workflows/payment-acceptance.yml"

store_of() {
    # PROD or TEST, from the shop id the file names. One distinct value only.
    grep -oE 'YOOKASSA_(PROD|TEST)_SHOP_ID' "$1" | sed -E 's/YOOKASSA_(.*)_SHOP_ID/\1/' | sort -u
}

core_store=$(store_of "${CORE}")
readback_store=$(store_of "${READBACK}")

failed=0

for pair in "${CORE}:${core_store}" "${READBACK}:${readback_store}"; do
    file="${pair%%:*}"
    store="${pair#*:}"
    case "${store}" in
        PROD|TEST) ;;
        "") echo "${file}: names no YooKassa shop id at all"; failed=1 ;;
        *) echo "${file}: names more than one store: $(echo "${store}" | tr '\n' ' ')"; failed=1 ;;
    esac
done

if [ "${failed}" -eq 0 ] && [ "${core_store}" != "${readback_store}" ]; then
    echo "${CORE} uses the ${core_store} store; ${READBACK} uses ${readback_store}."
    failed=1
fi

if [ "${failed}" -ne 0 ]; then
    echo
    echo "Core would record payments in one shop while the readback asks another,"
    echo "and every real payment would be reported as missing from the provider."
    exit 1
fi

echo "ok: Core and the payment readback both use the ${core_store} store"
