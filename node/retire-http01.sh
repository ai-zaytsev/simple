#!/usr/bin/env bash
#
# Takes the old certificate scheme off a node, once the new one is serving.
#
# Until this runs a node carries two ways to get a certificate: the agent,
# which never holds anything but its own key, and a local ACME client with its
# own account key and a copy of an old private key. The second one is the
# reason this exists. Whoever takes the machine takes whatever is on it, so
# what is on it should be only what it needs.
#
# Deliberately not part of the installer. Removing key material is not
# something a setup script should do while somebody is watching a different
# line of output, and it must not happen until the replacement is proven to be
# the certificate the world is actually receiving - which is checked here
# before anything is deleted.

set -euo pipefail

# shellcheck source=/dev/null
. /etc/simple-vpn-node.env
: "${NODE_DOMAIN:?NODE_DOMAIN is required}"

CRT="/etc/simple-vpn-tls/${NODE_DOMAIN}.crt"

if [ ! -s "${CRT}" ]; then
  echo "No certificate at ${CRT}. Install the agent first; nothing removed."
  exit 1
fi

served=$(echo | openssl s_client -connect "127.0.0.1:443" -servername "${NODE_DOMAIN}" 2>/dev/null \
  | openssl x509 -noout -enddate 2>/dev/null | cut -d= -f2)
installed=$(openssl x509 -in "${CRT}" -noout -enddate 2>/dev/null | cut -d= -f2)

if [ -z "${served}" ] || [ "${served}" != "${installed}" ]; then
  echo "The new certificate is not the one being served. Nothing removed."
  echo "  serving:   ${served:-nothing}"
  echo "  installed: ${installed}"
  exit 1
fi

echo "New certificate confirmed in use, valid until ${served}."

# The unused copy of the site, which nobody loads but which still names the old
# paths. Left alone it is a trap: re-enabling it one day would quietly put the
# old certificate back and undo all of this.
AVAILABLE=/etc/nginx/sites-available/default
if [ -f "${AVAILABLE}" ] && grep -q "/etc/lego/" "${AVAILABLE}"; then
  sed -i \
    -e "s#ssl_certificate  *[^;]*;#ssl_certificate     /etc/simple-vpn-tls/${NODE_DOMAIN}.crt;#" \
    -e "s#ssl_certificate_key  *[^;]*;#ssl_certificate_key /etc/simple-vpn-tls/${NODE_DOMAIN}.key;#" \
    "${AVAILABLE}"
  echo "The spare copy of the site no longer points at the old certificate."
fi

removed=0
for path in /etc/lego /usr/local/bin/lego; do
  if [ -e "${path}" ]; then
    rm -rf "${path}"
    echo "Removed ${path}."
    removed=1
  fi
done
[ "${removed}" = "1" ] || echo "Nothing of the old scheme was left."

# Said afterwards rather than assumed: removing files should not be able to
# take the site down, and if it somehow did, this is where it would show.
if ! nginx -t >/dev/null 2>&1; then
  echo "Nginx no longer accepts its configuration. Look at it now."
  exit 1
fi

again=$(echo | openssl s_client -connect "127.0.0.1:443" -servername "${NODE_DOMAIN}" 2>/dev/null \
  | openssl x509 -noout -enddate 2>/dev/null | cut -d= -f2)
if [ "${again}" != "${installed}" ]; then
  echo "The site stopped serving the right certificate. Look at it now."
  exit 1
fi

echo "Still served, and the old scheme is gone."
