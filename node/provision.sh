#!/usr/bin/env bash
#
# Takes a bare Ubuntu machine to a node that is carrying people.
#
# Everything a production node needs, in the order it needs it, with no step
# that expects somebody to be reading along. Until this existed a node was
# built by following node/README.md by hand, which meant every node was built
# slightly differently and the differences were found later, on the node that
# had them.
#
# Run by the Add Server workflow, and runnable on its own for the same reason
# every other script here is: an operation that only works from a pipeline is
# an operation nobody can debug.
#
# Expects, in the environment:
#   NODE_DOMAIN   the cover domain this node answers on
#   CP_URL        where the Control Plane is
#   NODE_TOKEN    this node's secret, already recorded in the Control Plane
#   WS_PATH       the hidden path the tunnel lives on, with a leading slash
#   EDGE_PATH     the hidden path that forwards the API inward, likewise
#
# And, in /tmp/site/, the cover site to serve.

set -euo pipefail

for name in NODE_DOMAIN CP_URL NODE_TOKEN WS_PATH EDGE_PATH; do
  if [ -z "${!name:-}" ]; then
    echo "${name} is required."
    exit 1
  fi
done

INBOUND_PORT="${INBOUND_PORT:-10000}"
API_PORT="${API_PORT:-10085}"
INBOUND_TAG="${INBOUND_TAG:-ws-in}"

# The exact build already carrying people on the existing fleet.
#
# Pinned by the hash of the binary rather than by a version number, and
# installed from the release archive rather than by running an installer script
# fetched from the internet. Two builds of one version can differ; a hash
# cannot, and this one is not a number somebody read off a release page but the
# file that is serving users right now.
XRAY_VERSION="v25.8.3"
XRAY_SHA256="e8e31ee3d57a5431a861b811baded3e304fc72e40c524704099e99c1d49f2c77"

say() { echo "== $*"; }

say "Passwords off"
# Before anything else that takes time. A machine is born with password
# authentication on and a password the provider generated, and the brute force
# starts within minutes: the two existing nodes had taken 26,584 and 20,612
# failed attempts by the time anybody looked.
if [ -s /tmp/harden-ssh.sh ]; then
  bash /tmp/harden-ssh.sh
else
  echo "  /tmp/harden-ssh.sh is missing; this machine still accepts passwords"
  exit 1
fi

say "Packages"
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq nginx curl unzip openssl python3 ca-certificates >/dev/null

say "Engine ${XRAY_VERSION}"
if [ "$(sha256sum /usr/local/bin/xray 2>/dev/null | cut -d' ' -f1)" != "${XRAY_SHA256}" ]; then
  work=$(mktemp -d)
  curl -fsSL -o "${work}/xray.zip" \
    "https://github.com/XTLS/Xray-core/releases/download/${XRAY_VERSION}/Xray-linux-64.zip"
  unzip -q -o "${work}/xray.zip" -d "${work}"

  got=$(sha256sum "${work}/xray" | cut -d' ' -f1)
  if [ "${got}" != "${XRAY_SHA256}" ]; then
    rm -rf "${work}"
    echo "The downloaded engine is not the build this fleet runs."
    echo "  expected ${XRAY_SHA256}"
    echo "  got      ${got}"
    exit 1
  fi

  install -m 0755 "${work}/xray" /usr/local/bin/xray
  rm -rf "${work}"
  echo "  installed and verified against the running fleet"
else
  echo "  already the right build"
fi

say "Engine configuration"
install -d -m 0755 /usr/local/etc/xray
cat > /usr/local/etc/xray/config.json <<JSON
{
  "log": { "access": "none", "loglevel": "warning", "dnsLog": false },
  "api": { "tag": "api", "services": ["HandlerService"] },
  "inbounds": [
    {
      "tag": "api-in",
      "listen": "127.0.0.1",
      "port": ${API_PORT},
      "protocol": "dokodemo-door",
      "settings": { "address": "127.0.0.1" }
    },
    {
      "tag": "${INBOUND_TAG}",
      "listen": "127.0.0.1",
      "port": ${INBOUND_PORT},
      "protocol": "vless",
      "settings": { "clients": [], "decryption": "none" },
      "streamSettings": {
        "network": "ws",
        "wsSettings": { "path": "${WS_PATH}" }
      }
    }
  ],
  "outbounds": [ { "tag": "direct", "protocol": "freedom" } ],
  "routing": {
    "rules": [ { "type": "field", "inboundTag": ["api-in"], "outboundTag": "api" } ]
  }
}
JSON

# No access in the file, deliberately. Users are added at runtime by the agent,
# and a user in the file is a user shared with everybody who reads it. The
# empty list is also what makes the agent unable to remove anybody it did not
# add: the management interface removes by label, and a user from the file has
# none.
/usr/local/bin/xray run -test -c /usr/local/etc/xray/config.json >/dev/null

cat > /etc/systemd/system/xray.service <<'UNIT'
[Unit]
Description=Xray
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/xray run -config /usr/local/etc/xray/config.json
Restart=on-failure
RestartSec=5
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
UNIT

say "Cover site"
if [ ! -d /tmp/site ]; then
  echo "No site was copied to /tmp/site."
  exit 1
fi
install -d -m 0755 /var/www/site
cp -r /tmp/site/. /var/www/site/
rm -rf /tmp/site
chown -R www-data:www-data /var/www/site

say "A certificate to start with"
# Self-signed and short-lived on purpose. Nginx will not listen on 443 without
# a certificate, and the real one is issued centrally over DNS - which cannot
# happen until the node is up enough to ask. So this exists for the few minutes
# between the two, made with the key the agent will keep using, so that the
# real certificate replaces it without a new key.
install -d -m 0700 /etc/simple-vpn-tls
KEY="/etc/simple-vpn-tls/${NODE_DOMAIN}.key"
CRT="/etc/simple-vpn-tls/${NODE_DOMAIN}.crt"
if [ ! -s "${KEY}" ]; then
  openssl ecparam -name prime256v1 -genkey -noout -out "${KEY}"
  chmod 600 "${KEY}"
fi
if [ ! -s "${CRT}" ]; then
  openssl req -new -x509 -key "${KEY}" -out "${CRT}" -days 1 \
    -subj "/CN=${NODE_DOMAIN}" >/dev/null 2>&1
  chmod 600 "${CRT}"
fi

say "Web server"
cat > /etc/nginx/sites-available/default <<NGINX
map \$http_upgrade \$ws_up { default 0; "websocket" 1; }
server {
    listen 80 default_server;
    server_name _;
    access_log off;
    location / { return 301 https://\$host\$request_uri; }
}
server {
    listen 443 ssl http2 default_server;
    server_name ${NODE_DOMAIN};
    access_log off;
    ssl_certificate     ${CRT};
    ssl_certificate_key ${KEY};
    root /var/www/site;
    index index.html;
    error_page 404 /404.html;

    # The tunnel. A request that is not a WebSocket upgrade gets the same 404
    # the rest of the site gives, so probing this path tells nobody anything.
    location ${WS_PATH} {
        if (\$ws_up = 0) { return 404; }
        proxy_pass http://127.0.0.1:${INBOUND_PORT};
        proxy_http_version 1.1;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host \$host;
        proxy_read_timeout 300s;
        proxy_send_timeout 300s;
    }

    # Forwards the Control Plane API inward, so that this node is a way in when
    # the service's own domain or address cannot be reached. Only /v1/, so this
    # is not an open proxy.
    location ${EDGE_PATH}/v1/ {
        proxy_pass ${CP_URL}/v1/;
        proxy_ssl_server_name on;
        proxy_http_version 1.1;
        proxy_read_timeout 30s;
        client_max_body_size 16k;
    }

    location / { try_files \$uri \$uri/ =404; }
}
NGINX

# Written is not the same as loaded.
#
# This wrote sites-available/default and relied on the package having left a
# symlink to it, which is true of a Debian nginx and was not true of the image
# the first automatically built node came up on. The site was written, nginx
# was happy, and what it served was the distribution's own page - with the
# distribution's access_log, which is what the privacy audit then refused to
# accept. The audit caught it; nothing before the audit would have.
install -d -m 0755 /etc/nginx/sites-enabled
if [ ! -e /etc/nginx/sites-enabled/default ]; then
  ln -sf /etc/nginx/sites-available/default /etc/nginx/sites-enabled/default
  echo "  the site was not enabled; it is now"
fi

# Said out loud rather than assumed, because the failure above was silent: a
# configuration that is present and not loaded looks exactly like one that is.
if ! nginx -T 2>/dev/null | grep -q "${NODE_DOMAIN}"; then
  echo "  nginx does not load a configuration naming ${NODE_DOMAIN}."
  echo "  It would serve the distribution's page, and its log, instead."
  exit 1
fi

# Client addresses and requested paths are recorded by error_log whatever
# access_log says. On this fleet that once put a real user's address on disk
# next to the tunnel path.
sed -i -E "s#^(\s*)error_log\s+([^;[:space:]]+)([^;]*);#\1error_log \2 alert;#" /etc/nginx/nginx.conf

nginx -t
systemctl enable -q nginx
systemctl restart nginx
systemctl daemon-reload
systemctl enable -q --now xray

say "What this node is"
umask 077
cat > /etc/simple-vpn-node.env <<ENVFILE
CP_URL=${CP_URL}
NODE_TOKEN=${NODE_TOKEN}
NODE_DOMAIN=${NODE_DOMAIN}
INBOUND_TAG=${INBOUND_TAG}
INBOUND_PORT=${INBOUND_PORT}
ENVFILE
umask 022

say "Who may connect"
# The users agent, and only it. The certificate and the measurement are
# installed by the caller as separate steps, because each of them is a phase of
# the node's life that somebody watching should be able to see it pass through
# - and a script that did all three would report one outcome for three things.
for required in users-agent.py install-users-agent.sh; do
  if [ ! -s "/tmp/${required}" ]; then
    echo "/tmp/${required} is missing."
    exit 1
  fi
done
bash /tmp/install-users-agent.sh

say "Configured"
echo "  xray:  $(systemctl is-active xray)"
echo "  nginx: $(systemctl is-active nginx)"
echo "  users: $(systemctl is-active simple-vpn-users)"
echo
echo "  Still to come: a certificate, then measurement, then the checks."
