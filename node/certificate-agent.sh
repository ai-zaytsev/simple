#!/usr/bin/env bash
#
# Keeps this node's certificate current, without ever holding a key to our DNS.
#
# The private key is made here and never leaves. What travels is a signing
# request, which carries only the public half and a name, and what comes back is
# a certificate - a document meant to be shown to everybody. Taking this machine
# therefore takes one certificate for one machine, replaced by destroying the
# machine. It does not take the other nodes, and it does not take the domains.
#
# Runs daily and usually does nothing. Asking for a certificate that is not
# needed spends part of the authority's weekly allowance, and the allowance is
# what a genuine renewal depends on.

set -euo pipefail

# shellcheck source=/dev/null
. /etc/simple-vpn-node.env

: "${CP_URL:?CP_URL is required}"
: "${NODE_TOKEN:?NODE_TOKEN is required}"
: "${NODE_DOMAIN:?NODE_DOMAIN is required}"

DIR=/etc/simple-vpn-tls
KEY="${DIR}/${NODE_DOMAIN}.key"
CRT="${DIR}/${NODE_DOMAIN}.crt"
CSR="${DIR}/${NODE_DOMAIN}.csr"

# Thirty days. Long enough that a week of failures can be survived, short
# enough to follow the authority's own advice.
RENEW_WITHIN_DAYS=30

install -d -m 0700 "${DIR}"

# The key is made once and kept. A new key on every renewal would be no safer -
# the old one was never exposed - and would spend the weekly allowance faster,
# because every issuance would be a fresh certificate rather than a renewal.
if [ ! -s "${KEY}" ]; then
  echo "Making a key for ${NODE_DOMAIN}. It will not leave this machine."
  openssl ecparam -name prime256v1 -genkey -noout -out "${KEY}"
  chmod 600 "${KEY}"
fi

expires_at=""
if [ -s "${CRT}" ]; then
  end=$(openssl x509 -in "${CRT}" -noout -enddate 2>/dev/null | cut -d= -f2 || true)
  if [ -n "${end}" ]; then
    expires_at=$(date -u -d "${end}" +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || true)
    left=$(( ( $(date -u -d "${end}" +%s) - $(date -u +%s) ) / 86400 ))
    echo "Current certificate has ${left} day(s) left."
    if [ "${left}" -gt "${RENEW_WITHIN_DAYS}" ]; then
      echo "Nothing to do."
      exit 0
    fi
  fi
fi

# One name, the node's own. The service checks this too and refuses anything
# else; asking correctly here means the refusal never has to happen.
openssl req -new -key "${KEY}" -out "${CSR}" -subj "/CN=${NODE_DOMAIN}" \
  -addext "subjectAltName=DNS:${NODE_DOMAIN}"
chmod 600 "${CSR}"

body=$(python3 - "${CSR}" "${expires_at}" <<'PY'
import json, sys
csr = open(sys.argv[1]).read()
expires = sys.argv[2]
payload = {"csr": csr}
if expires:
    payload["expires_at"] = expires
print(json.dumps(payload))
PY
)

echo "Asking for a certificate."
code=$(printf '%s' "${body}" | curl -sS -o /tmp/cert-answer.json -w '%{http_code}' \
  --max-time 300 \
  -X POST \
  -H "authorization: Bearer ${NODE_TOKEN}" \
  -H "content-type: application/json" \
  --data-binary @- \
  "${CP_URL}/v1/node/certificate" || echo "000")

if [ "${code}" != "200" ]; then
  # The reason, never the body. A refusal echoes what was sent, and the answer
  # to a live challenge has no business in a log.
  said=$(python3 -c "import json,sys;print(json.load(open('/tmp/cert-answer.json')).get('error',''))" 2>/dev/null || true)
  rm -f /tmp/cert-answer.json
  echo "Not issued. HTTP ${code}. ${said}"
  exit 1
fi

python3 - <<'PY'
import json
answer = json.load(open("/tmp/cert-answer.json"))
open("/tmp/new.crt", "w").write(answer["certificate"])
print("Issued, valid until", answer.get("expires_at", "unknown"))
PY
rm -f /tmp/cert-answer.json

# Refused rather than installed if it does not match the key we hold. A
# certificate for somebody else's key would leave this node serving a
# handshake it cannot complete - working configuration, dead site.
key_pub=$(openssl pkey -in "${KEY}" -pubout -outform DER 2>/dev/null | sha256sum | cut -d' ' -f1)
crt_pub=$(openssl x509 -in /tmp/new.crt -pubkey -noout 2>/dev/null \
  | openssl pkey -pubin -pubout -outform DER 2>/dev/null | sha256sum | cut -d' ' -f1)

if [ "${key_pub}" != "${crt_pub}" ]; then
  rm -f /tmp/new.crt
  echo "The certificate does not match this node's key. Not installing it."
  exit 1
fi

install -m 0600 /tmp/new.crt "${CRT}"
rm -f /tmp/new.crt "${CSR}"

# Reloaded rather than restarted: a reload keeps every established connection,
# and the certificate matters to new ones only.
if nginx -t >/dev/null 2>&1; then
  systemctl reload nginx
  echo "Installed and in use."
else
  echo "Installed, but nginx refused its configuration. Not reloading."
  exit 1
fi
