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
# Runs daily and usually issues nothing. Asking for a certificate that is not
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

# What is actually being offered on the wire, which is not the same thing as
# what is on disk. The first run of this agent installed a valid certificate
# into a directory nginx was not reading, reported success, and changed
# nothing: the site went on serving the old one, and nobody would have known
# until it expired.
#
# Checked on every run rather than only after an issuance. A certificate that
# stops being served is exactly as broken as one that was never obtained, and
# it can stop at any time - somebody edits the site, a package replaces it.
# This way at most a day passes before something says so.
confirm_being_served() {
  # Nginx not reading our path is normally the whole problem, not an excuse to
  # pass: a site edited by hand or replaced by a package leaves a node with a
  # perfectly maintained certificate that nobody is ever shown.
  #
  # It is legitimate exactly once, during installation, because the paths
  # cannot name files that do not exist and the files cannot exist until this
  # has run. The installer says so out loud for that one run rather than this
  # check guessing, so the exception cannot quietly cover a real regression
  # later.
  if ! grep -q "${CRT}" /etc/nginx/sites-enabled/* 2>/dev/null; then
    if [ "${ALLOW_UNSERVED:-}" = "1" ]; then
      echo "Installed. Nginx is not pointed here yet, which the installer does next."
      return 0
    fi
    echo "Installed, but nginx does not read ${CRT}, so this certificate is not served."
    return 1
  fi

  local served installed
  installed=$(openssl x509 -in "${CRT}" -noout -enddate 2>/dev/null | cut -d= -f2)

  # Asked more than once, because a reload is not instant: nginx starts new
  # workers and lets the old ones finish what they were already doing, so for
  # a moment after a reload the honest answer is still the old certificate.
  # Failing in that moment would be a check that goes off on a working node,
  # which teaches everybody to ignore it.
  for _ in 1 2 3 4 5; do
    served=$(echo | openssl s_client -connect "127.0.0.1:443" -servername "${NODE_DOMAIN}" 2>/dev/null \
      | openssl x509 -noout -enddate 2>/dev/null | cut -d= -f2)
    [ "${served}" = "${installed}" ] && break
    sleep 1
  done

  if [ "${served}" != "${installed}" ]; then
    echo "The server is offering a different certificate than the one installed."
    echo "  serving:   ${served:-nothing}"
    echo "  installed: ${installed}"
    echo "Check that nginx reads ${CRT}."
    return 1
  fi

  echo "Being served, valid until ${served}."
  return 0
}

# The key is made once and kept. A new key on every renewal would be no safer -
# the old one was never exposed - and would spend the weekly allowance faster,
# because every issuance would be a fresh certificate rather than a renewal.
if [ ! -s "${KEY}" ]; then
  echo "Making a key for ${NODE_DOMAIN}. It will not leave this machine."
  openssl ecparam -name prime256v1 -genkey -noout -out "${KEY}"
  chmod 600 "${KEY}"
fi

expires_at=""
issuer=""
if [ -s "${CRT}" ]; then
  # Who signed what is being served, sent along with when it runs out.
  #
  # A certificate can be perfectly fresh and still be the wrong one: a node
  # proved out against the test authority holds thirty days of a certificate no
  # phone will accept. Only the service knows which authority that node is
  # meant to use now, so it is the service that has to decide - and it cannot
  # decide without being told what is there.
  issuer=$(openssl x509 -in "${CRT}" -noout -issuer 2>/dev/null | sed 's/^issuer=//' || true)

  end=$(openssl x509 -in "${CRT}" -noout -enddate 2>/dev/null | cut -d= -f2 || true)
  if [ -n "${end}" ]; then
    expires_at=$(date -u -d "${end}" +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || true)
    left=$(( ( $(date -u -d "${end}" +%s) - $(date -u +%s) ) / 86400 ))
    echo "Current certificate has ${left} day(s) left."

    # Asked anyway when the authority might have changed. The service refuses
    # early renewals, so this costs a refusal rather than an issuance when
    # nothing has changed - and that refusal is cheap and says why.
    if [ "${left}" -gt "${RENEW_WITHIN_DAYS}" ] && [ -z "${issuer}" ]; then
      echo "No renewal needed."
      confirm_being_served
      exit $?
    fi
  fi
fi

# One name, the node's own. The service checks this too and refuses anything
# else; asking correctly here means the refusal never has to happen.
openssl req -new -key "${KEY}" -out "${CSR}" -subj "/CN=${NODE_DOMAIN}" \
  -addext "subjectAltName=DNS:${NODE_DOMAIN}"
chmod 600 "${CSR}"

body=$(python3 - "${CSR}" "${expires_at}" "${issuer}" <<'PY'
import json, sys
csr = open(sys.argv[1]).read()
expires, issuer = sys.argv[2], sys.argv[3]
payload = {"csr": csr}
if expires:
    payload["expires_at"] = expires
if issuer:
    payload["issuer"] = issuer
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

  # A refusal is not a fault when there is already a good certificate here.
  #
  # This agent now asks on every run, because only the service knows which
  # authority this node is meant to use and the node cannot decide alone. The
  # ordinary answer to most of those asks is "not yet, it has months left" -
  # which is the service working, not failing.
  #
  # So the question is not whether the ask was refused but whether this node is
  # serving something usable. If it is, there is nothing wrong here; if it is
  # not, the refusal is the reason and the run should say so.
  if [ "${code}" = "409" ] && [ -s "${CRT}" ]; then
    echo "Nothing to do: what is being served is still good."
    confirm_being_served
    exit $?
  fi
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
if ! nginx -t >/dev/null 2>&1; then
  echo "Installed, but nginx refused its configuration. Not reloading."
  exit 1
fi
systemctl reload nginx

confirm_being_served
