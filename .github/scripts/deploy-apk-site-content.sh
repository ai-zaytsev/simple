#!/usr/bin/env bash
# Deploy the repository-owned static page to the existing site-1 without
# leaving SSH exposed. The DigitalOcean firewall is restored on every exit.
set -euo pipefail

for name in DIGITALOCEAN_TOKEN SITE_DROPLET_ID SITE_IP DEPLOY_KEY SITE_DOMAIN; do
  [ -n "${!name:-}" ] || { echo "Missing required deployment input: ${name}"; exit 1; }
done

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
  [ "${verify_code}" = "200" ] && jq -e --arg source "${runner_cidr}" '
    [.firewall.inbound_rules[]?
      | select(.protocol == "tcp" and .ports == "22")
      | .sources.addresses[]?
      | select(. == $source)]
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
      echo "Temporary SSH access was removed; the site firewall is web-only again."
    else
      echo "Failed to restore the site firewall after content deployment."
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
[ "${count}" = "1" ] || { echo "Expected exactly one site-1 firewall; refusing content deployment."; exit 1; }

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

jq --arg source "${runner_cidr}" '
  .inbound_rules += [{
    protocol: "tcp",
    ports: "22",
    sources: {addresses: [$source]}
  }]
' "${work}/firewall-original.json" > "${work}/firewall-open.json"

firewall_open=true
code=$(curl -sS -o "${work}/open-answer.json" -w '%{http_code}' -X PUT \
  -H "Authorization: Bearer ${token}" \
  -H 'content-type: application/json' \
  --data-binary "@${work}/firewall-open.json" \
  "https://api.digitalocean.com/v2/firewalls/${firewall_id}" || echo 000)
[ "${code}" = "200" ] || { echo "Temporary SSH rule was rejected: HTTP ${code}."; exit 1; }
echo "Temporary SSH access is limited to the current CI runner /32."

printf '%s\n' "${DEPLOY_KEY}" > "${work}/deploy-key"
chmod 0600 "${work}/deploy-key"

for attempt in $(seq 1 24); do
  if ssh-keyscan -T 8 -H "${SITE_IP}" > "${work}/known-hosts" 2>/dev/null && \
     ssh -i "${work}/deploy-key" \
       -o BatchMode=yes -o ConnectTimeout=10 \
       -o StrictHostKeyChecking=yes -o UserKnownHostsFile="${work}/known-hosts" \
       "root@${SITE_IP}" true; then
    break
  fi
  [ "${attempt}" -lt 24 ] || { echo "site-1 did not accept the deployment key."; exit 1; }
  sleep 5
done

tar -C sites/official -czf "${work}/site-content.tar.gz" \
  index.html 404.html styles.css app.js

scp -i "${work}/deploy-key" \
  -o BatchMode=yes -o ConnectTimeout=10 \
  -o StrictHostKeyChecking=yes -o UserKnownHostsFile="${work}/known-hosts" \
  "${work}/site-content.tar.gz" "root@${SITE_IP}:/tmp/simple-vpn-site-content.tar.gz.new"

ssh -i "${work}/deploy-key" \
  -o BatchMode=yes -o ConnectTimeout=10 \
  -o StrictHostKeyChecking=yes -o UserKnownHostsFile="${work}/known-hosts" \
  "root@${SITE_IP}" 'bash -se' <<'REMOTE'
set -euo pipefail
stage=$(mktemp -d /var/www/simple-vpn-stage.XXXXXX)
archive=/tmp/simple-vpn-site-content.tar.gz.new
nginx_target=/etc/nginx/sites-available/simple-vpn
nginx_backup=/tmp/simple-vpn-nginx.conf.backup
restore_nginx=false
cleanup() {
  status=$?
  trap - EXIT
  if [ "${restore_nginx}" = "true" ] && [ -f "${nginx_backup}" ]; then
    mv -f "${nginx_backup}" "${nginx_target}"
    nginx -t >/dev/null 2>&1 || true
  fi
  rm -rf -- "${stage}"
  rm -f -- "${archive}" "${nginx_backup}"
  exit "${status}"
}
trap cleanup EXIT
tar -xzf "${archive}" -C "${stage}"
for file in index.html 404.html styles.css app.js; do
  test -s "${stage}/${file}"
  install -o root -g www-data -m 0644 "${stage}/${file}" "/var/www/simple-vpn/${file}.new"
done
for file in index.html 404.html styles.css app.js; do
  mv -f "/var/www/simple-vpn/${file}.new" "/var/www/simple-vpn/${file}"
done
cp -a "${nginx_target}" "${nginx_backup}"
restore_nginx=true
if ! grep -Eq '^[[:space:]]*listen[[:space:]].*443' "${nginx_target}"; then
  certbot --nginx --non-interactive --agree-tos \
    --register-unsafely-without-email --redirect --reinstall \
    -d simple-vpn.download
fi
if ! grep -Fq 'expires -1;' "${nginx_target}"; then
  sed -i '/^[[:space:]]*location \/ {$/a\        expires -1;' "${nginx_target}"
fi
nginx -t
systemctl reload nginx
systemctl is-active --quiet nginx
restore_nginx=false
REMOTE

for attempt in $(seq 1 12); do
  if curl -fsS --max-time 15 --resolve "${SITE_DOMAIN}:443:${SITE_IP}" \
       "https://${SITE_DOMAIN}/" -o "${work}/origin-index.html"; then
    break
  fi
  [ "${attempt}" -lt 12 ] || { echo "The origin did not recover after the verified Nginx reload."; exit 1; }
  sleep 2
done
grep -q 'data-install-guide' "${work}/origin-index.html" || {
  echo "The origin does not serve the new mobile installation guide."
  exit 1
}

echo "Static site content was deployed to the existing site-1."
