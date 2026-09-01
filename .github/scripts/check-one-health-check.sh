#!/usr/bin/env bash
#
# The Control Plane is asked whether it is working in exactly one place.
#
# Three workflows depend on that answer - the deploy, the restore into
# production, and the rollback. They each had their own copy, and one of them
# waited for a status code the handler never returns, which turned the first
# successful restore into production into a red run.
#
# The mistake was not the number. It was writing a new check beside a working
# one instead of using it: deploy-control-plane.yml had been asking correctly
# the whole time. This refuses the next copy.
set -euo pipefail

SCRIPT=".github/scripts/wait-for-core-health.sh"
failed=0

for file in .github/workflows/*.yml; do
    [ -f "${file}" ] || continue
    grep -q '8080/healthz' "${file}" || continue
    echo "${file}: asks 8080/healthz directly instead of running ${SCRIPT}"
    failed=1
done

if [ ! -x "${SCRIPT}" ]; then
    echo "${SCRIPT} is missing or not executable; the one copy has to exist"
    failed=1
fi

if [ "${failed}" -ne 0 ]; then
    echo
    echo "An expectation written apart from the thing that produces it drifts"
    echo "in silence. Call the script; if it needs to change, change it there."
    exit 1
fi

echo "ok: the Control Plane health check has one copy"
