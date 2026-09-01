#!/usr/bin/env bash
# Give one approved operator /32 a short window to install the existing CI
# public key on site-1. The exact original firewall is restored on every exit.
set -euo pipefail

for name in DIGITALOCEAN_TOKEN SITE_DROPLET_ID SITE_IP DEPLOY_KEY BOOTSTRAP_CIDR; do
  [ -n "${!name:-}" ] || { echo "Missing required bootstrap input: ${name}"; exit 1; }
done

valid_cidr=false
if printf '%s' "${BOOTSTRAP_CIDR}" | grep -Eq '^([0-9]{1,3}\.){3}[0-9]{1,3}/32$'; then
  ip=${BOOTSTRAP_CIDR%/32}
  IFS=. read -r a b c d <<< "${ip}"
  if [ "${a}" -le 255 ] && [ "${b}" -le 255 ] && [ "${c}" -le 255 ] && [ "${d}" -le 255 ] && \
     [ "${ip}" != "0.0.0.0" ]; then
    valid_cidr=true
  fi
fi
[ "${valid_cidr}" = "true" ] || { echo "bootstrap_cidr must be one exact public IPv4 /32."; exit 1; }

umask 077
token=$(printf '%s' "${DIGITALOCEAN_TOKEN}" | tr -d '[:space:]')
work=$(mktemp -d)
firewall_open=false

restore_firewall() {
  local code verify_code
  code=$(curl -sS -o "${work}/restore-answer.json" -w '%{http_code}' -X PUT \
    -H "Authorization: Bearer ${token}" \
    -H 'content-type: application/json' \
    --data-binary "@${work}/firewall-original.json" \
    "https://api.digitalocean.com/v2/firewalls/${firewall_id}" || echo 000)
  [ "${code}" = "200" ] || return 1

  verify_code=$(curl -sS -o "${work}/firewall-restored.json" -w '%{http_code}' \
    -H "Authorization: Bearer ${token}" \
    -H 'accept: application/json' \
    "https://api.digitalocean.com/v2/firewalls/${firewall_id}" || echo 000)
  [ "${verify_code}" = "200" ] && jq -e --arg operator "${BOOTSTRAP_CIDR}" --arg runner "${runner_cidr}" '
    [.firewall.inbound_rules[]?
      | select(.protocol == "tcp" and .ports == "22")
      | .sources.addresses[]?
      | select(. == $operator or . == $runner)]
    | length == 0
  ' "${work}/firewall-restored.json" >/dev/null
}

finish() {
  local status=$? restored=false
  trap - EXIT
  if [ "${firewall_open}" = "true" ]; then
    for attempt in $(seq 1 5); do
      if restore_firewall; then
        restored=true
        break
      fi
      [ "${attempt}" -lt 5 ] && sleep 3
    done
    if [ "${restored}" = "true" ]; then
      echo "Bootstrap SSH access was removed; the site firewall is web-only again."
    else
      echo "Failed to restore the site firewall after access bootstrap."
      status=1
    fi
  fi
  rm -rf -- "${work}"
  exit "${status}"
}
trap finish EXIT

code=$(curl -sS -o "${work}/firewalls.json" -w '%{http_code}' \
  -H "Authorization: Bearer ${token}" \
  -H 'accept: application/json' \
  'https://api.digitalocean.com/v2/firewalls?per_page=100' || echo 000)
[ "${code}" = "200" ] || { echo "Firewall inventory failed: HTTP ${code}."; exit 1; }

count=$(jq -r '(.firewalls // []) | map(select(.name == "site-1-public-web-only")) | length' "${work}/firewalls.json")
[ "${count}" = "1" ] || { echo "Expected exactly one site-1 firewall; refusing access bootstrap."; exit 1; }

jq -e --argjson droplet_id "${SITE_DROPLET_ID}" '
  (.firewalls // [])
  | map(select(.name == "site-1-public-web-only"))[0]
  | (.droplet_ids | length) == 1 and .droplet_ids[0] == $droplet_id
' "${work}/firewalls.json" >/dev/null || {
  echo "The site firewall is not attached only to the approved site-1."
  exit 1
}

firewall_id=$(jq -r '(.firewalls // []) | map(select(.name == "site-1-public-web-only"))[0].id' "${work}/firewalls.json")
echo "::add-mask::${firewall_id}"
echo "::add-mask::${SITE_IP}"
echo "::add-mask::${BOOTSTRAP_CIDR}"

jq '(.firewalls // []) | map(select(.name == "site-1-public-web-only"))[0]
  | {name, inbound_rules, outbound_rules, droplet_ids, tags}' \
  "${work}/firewalls.json" > "${work}/firewall-original.json"

runner_ip=$(curl -4fsS --max-time 15 https://api.ipify.org)
printf '%s' "${runner_ip}" | grep -Eq '^([0-9]{1,3}\.){3}[0-9]{1,3}$' || {
  echo "The runner did not return a valid IPv4 address."
  exit 1
}
echo "::add-mask::${runner_ip}"
runner_cidr="${runner_ip}/32"

jq --arg operator "${BOOTSTRAP_CIDR}" --arg runner "${runner_cidr}" '
  .inbound_rules += [{
    protocol: "tcp",
    ports: "22",
    sources: {addresses: [$operator, $runner] | unique}
  }]
' "${work}/firewall-original.json" > "${work}/firewall-open.json"

firewall_open=true
code=$(curl -sS -o "${work}/open-answer.json" -w '%{http_code}' -X PUT \
  -H "Authorization: Bearer ${token}" \
  -H 'content-type: application/json' \
  --data-binary "@${work}/firewall-open.json" \
  "https://api.digitalocean.com/v2/firewalls/${firewall_id}" || echo 000)
[ "${code}" = "200" ] || { echo "Bootstrap SSH rules were rejected: HTTP ${code}."; exit 1; }
echo "Maintenance SSH window is open only for the approved operator and CI runner /32 addresses."

printf '%s\n' "${DEPLOY_KEY}" > "${work}/deploy-key"
chmod 0600 "${work}/deploy-key"

for attempt in $(seq 1 60); do
  if ssh-keyscan -T 8 -H "${SITE_IP}" > "${work}/known-hosts" 2>/dev/null && \
     ssh -i "${work}/deploy-key" \
       -o BatchMode=yes -o ConnectTimeout=10 \
       -o StrictHostKeyChecking=yes -o UserKnownHostsFile="${work}/known-hosts" \
       "root@${SITE_IP}" true 2>/dev/null; then
    echo "site-1 now accepts the CI deployment key."
    exit 0
  fi
  [ "${attempt}" -lt 60 ] || { echo "CI access was not installed before the maintenance window expired."; exit 1; }
  sleep 5
done
