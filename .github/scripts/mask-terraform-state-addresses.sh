#!/usr/bin/env bash
# Mask every public or private address recorded in Terraform state before a
# plan can describe drift. A deleted test node once disappeared at the provider
# while its old address remained in state; refresh printed that address before
# the workflow had any new output to mask.
set -euo pipefail

state=$(mktemp)
trap 'rm -f "${state}"' EXIT

if ! terraform state pull > "${state}" 2>/dev/null; then
    echo "Terraform state is not readable; the following command will report the real error."
    exit 0
fi

jq -r '
  .. | objects |
  [.ipv4_address?, .ipv4_address_private?, .ipv6_address?] |
  .[] | select(type == "string" and length > 0)
' "${state}" | sort -u | while IFS= read -r address; do
    echo "::add-mask::${address}"
done

echo "Addresses already present in Terraform state are masked."
