#!/usr/bin/env bash
#
# The numbers the site tells people are the numbers the product enforces.
#
# The site is static HTML on a different host from Core, so it cannot ask what
# the limits are - it states them. That makes two places holding one fact, and
# two places holding one fact drift. This repository has spent a whole run of
# work on that: a status code remembered rather than read, a chart labelled
# with the wrong window, a verdict computed from the wrong half of the truth.
#
# Here the drift has a customer on the other end of it. A site promising
# 10 Mbit/s beside a service enforcing something else is not a stale comment,
# it is a claim somebody made a decision on.
#
# What this compares is the seeded product default. An operator can change the
# live value with a SQL update and the site would then be wrong with nothing
# failing - that gap is recorded in docs/tech-debt.md, because closing it needs
# the site to be told, not a check to be cleverer.
set -euo pipefail

PAGE="sites/official/index.html"
DEVICES_SQL="control-plane/internal/store/migrations/0005_device_limit.sql"
SPEED_SQL="control-plane/internal/store/migrations/0017_speed.sql"
failed=0

for file in "${PAGE}" "${DEVICES_SQL}" "${SPEED_SQL}"; do
    [ -f "${file}" ] || { echo "${file} is missing"; exit 1; }
done

# What the product enforces, read out of the migrations rather than typed here.
product_speed=$(grep -oE "speed_mbit = [0-9]+ +where tier = 'FREE'" "${SPEED_SQL}" \
    | grep -oE '[0-9]+' | head -1)
product_devices=$(grep -oE "values \('FREE', [0-9]+\)" "${DEVICES_SQL}" \
    | grep -oE '[0-9]+' | head -1)

# What the site says.
site_speed=$(grep -oE 'data-free-speed="[0-9]+"' "${PAGE}" | grep -oE '[0-9]+' | head -1)
site_devices=$(grep -oE 'data-free-devices="[0-9]+"' "${PAGE}" | grep -oE '[0-9]+' | head -1)

if [ -z "${product_speed}" ] || [ -z "${product_devices}" ]; then
    echo "cannot read the FREE limits out of the migrations; this check no longer"
    echo "knows what it is comparing against, which is worse than not checking"
    exit 1
fi
if [ -z "${site_speed}" ] || [ -z "${site_devices}" ]; then
    echo "${PAGE}: the FREE limits are not marked up, so nothing can compare them"
    echo "  expected data-free-speed and data-free-devices attributes"
    exit 1
fi

if [ "${site_speed}" != "${product_speed}" ]; then
    echo "${PAGE} promises ${site_speed} Mbit/s; the product enforces ${product_speed}"
    failed=1
fi
if [ "${site_devices}" != "${product_devices}" ]; then
    echo "${PAGE} promises ${site_devices} device(s); the product allows ${product_devices}"
    failed=1
fi

# The figure a person reads has to agree with the one the machine compares.
# Marking it up correctly and writing something else in the sentence beside it
# would pass everything above and still mislead the reader.
if ! grep -q "До ${product_speed} Мбит/с" "${PAGE}"; then
    echo "${PAGE}: the visible text does not say ${product_speed} Мбит/с"
    failed=1
fi

if [ "${failed}" -ne 0 ]; then
    echo
    echo "Somebody chose this product by reading that number."
    exit 1
fi

echo "ok: the site states the FREE limits the product enforces (${product_speed} Мбит/с, ${product_devices})"
