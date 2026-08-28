#!/usr/bin/env bash
#
# Points one of our domains at an address, whoever serves its zone.
#
# One copy, called by both the workflow that does this on its own and the one
# that adds a server. The comment further down about a fix that reached one
# provider and not the other is the reason: this logic already drifted once,
# and a domain ended up resolving to one live machine and two destroyed ones.
#
# Expects in the environment:
#   DOMAIN     the name to point
#   ADDRESS    the IPv4 address it should resolve to
#   CF_TOKEN   Cloudflare API token
#   SS_KEY     Spaceship API key
#   SS_SECRET  Spaceship API secret

set -euo pipefail

: "${DOMAIN:?DOMAIN is required}"
: "${ADDRESS:?ADDRESS is required}"

# Four numbers, each in range. A malformed value would otherwise be written
# into the zone and only noticed when nothing resolved.
if ! printf '%s' "${ADDRESS}" | grep -qE '^([0-9]{1,3}\.){3}[0-9]{1,3}$'; then
  echo "Not an IPv4 address."
  exit 1
fi
for part in $(printf '%s' "${ADDRESS}" | tr '.' ' '); do
  if [ "${part}" -gt 255 ]; then
    echo "Not an IPv4 address."
    exit 1
  fi
done

token=$(printf '%s' "${CF_TOKEN:-}" | tr -d '[:space:]')

zone_id=""
if [ -n "${token}" ]; then
  zone_id=$(curl -sS -H "Authorization: Bearer ${token}" \
    -G --data-urlencode "name=${DOMAIN}" \
    https://api.cloudflare.com/client/v4/zones \
    | jq -r '(.result // []) | .[0].id // empty')
fi

if [ -n "${zone_id}" ]; then
  # Existing records on the exact name go first: a second record beside the
  # first makes the name resolve to both, at random.
  existing=$(curl -sS -H "Authorization: Bearer ${token}" \
    -G --data-urlencode "type=A" --data-urlencode "name=${DOMAIN}" \
    "https://api.cloudflare.com/client/v4/zones/${zone_id}/dns_records" \
    | jq -r --arg n "${DOMAIN}" \
      '(.result // [])[] | select(.name == $n and .type == "A") | .id')

  for stale in ${existing}; do
    curl -sS -X DELETE -H "Authorization: Bearer ${token}" \
      "https://api.cloudflare.com/client/v4/zones/${zone_id}/dns_records/${stale}" > /dev/null
  done

  # proxied stays false: proxying terminates TLS, which breaks both the
  # certificate and the tunnel behind it.
  created=$(curl -sS -X POST \
    -H "Authorization: Bearer ${token}" \
    -H "content-type: application/json" \
    -d "{\"type\":\"A\",\"name\":\"${DOMAIN}\",\"content\":\"${ADDRESS}\",\"ttl\":60,\"proxied\":false}" \
    "https://api.cloudflare.com/client/v4/zones/${zone_id}/dns_records" \
    | jq -r '.result.id // empty')

  if [ -z "${created}" ]; then
    echo "Could not write the record."
    exit 1
  fi
  echo "${DOMAIN} now points at ${ADDRESS} (Cloudflare)."
  exit 0
fi

ss_key=$(printf '%s' "${SS_KEY:-}" | tr -d '[:space:]')
ss_secret=$(printf '%s' "${SS_SECRET:-}" | tr -d '[:space:]')

if [ -z "${ss_key}" ] || [ -z "${ss_secret}" ]; then
  echo "Neither provider serves ${DOMAIN}, or no credentials were given for the one that does."
  exit 1
fi

# Existing records on the apex go first, explicitly.
#
# The force flag on the write turned out to mean "overwrite a matching record",
# not "be the only record": a domain ended up resolving to one live address and
# two machines that had been destroyed, which sends both certificate validation
# and clients to a dead host two times in three. The same mistake was fixed on
# the other provider earlier; this branch of the code never had the fix.
existing=$(curl -sS \
  -H "X-API-Key: ${ss_key}" -H "X-API-Secret: ${ss_secret}" \
  -H "Accept: application/json" \
  -G --data-urlencode "take=100" --data-urlencode "skip=0" \
  "https://spaceship.dev/api/v1/dns/records/${DOMAIN}" \
  | jq -c '[(.items // [])[] | select(.type == "A")]' 2>/dev/null || echo '[]')

count=$(printf '%s' "${existing}" | jq -r 'length')
if [ "${count}" != "0" ]; then
  code=$(curl -sS -o /tmp/ss-del.json -w '%{http_code}' -X DELETE \
    -H "X-API-Key: ${ss_key}" -H "X-API-Secret: ${ss_secret}" \
    -H "content-type: application/json" \
    -d "${existing}" \
    "https://spaceship.dev/api/v1/dns/records/${DOMAIN}" || echo "000")
  echo "Removing ${count} existing A record(s): HTTP ${code}"
  if [ "${code}" != "200" ] && [ "${code}" != "204" ]; then
    # Field names only, never the body.
    fields=$(jq -r 'keys_unsorted | join(", ")' /tmp/ss-del.json 2>/dev/null || echo "unreadable")
    said=$(jq -r '.detail // .message // empty' /tmp/ss-del.json 2>/dev/null || echo "")
    echo "  fields in the refusal: ${fields}"
    [ -n "${said}" ] && echo "  provider says: ${said}"
    echo "Refusing to add a record beside ones that could not be removed."
    exit 1
  fi
fi

body=$(jq -n --arg ip "${ADDRESS}" \
  '{force: true, items: [{type: "A", name: "@", address: $ip, ttl: 60}]}')

code=$(curl -sS -o /tmp/ss.json -w '%{http_code}' -X PUT \
  -H "X-API-Key: ${ss_key}" \
  -H "X-API-Secret: ${ss_secret}" \
  -H "content-type: application/json" \
  -d "${body}" \
  "https://spaceship.dev/api/v1/dns/records/${DOMAIN}" || echo "000")

if [ "${code}" != "200" ] && [ "${code}" != "204" ]; then
  echo "The provider refused the record. HTTP ${code}."
  sed 's/^/  /' /tmp/ss.json 2>/dev/null | head -20
  exit 1
fi
echo "${DOMAIN} now points at ${ADDRESS} (Spaceship)."
