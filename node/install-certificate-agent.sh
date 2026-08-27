#!/usr/bin/env bash
#
# Installs the agent that keeps this node's certificate current.
#
# Run on the node, after /etc/simple-vpn-node.env carries NODE_DOMAIN as well
# as CP_URL and NODE_TOKEN. Expects /tmp/certificate-agent.sh to have been
# copied over first.

set -euo pipefail

if ! grep -q '^NODE_DOMAIN=' /etc/simple-vpn-node.env; then
  echo "/etc/simple-vpn-node.env has no NODE_DOMAIN. Add it first:"
  echo "  NODE_DOMAIN=example.invalid"
  exit 1
fi

install -m 0755 /tmp/certificate-agent.sh /usr/local/bin/simple-vpn-certificate
rm -f /tmp/certificate-agent.sh

# Obtained before nginx is pointed at it, and that order is forced rather than
# chosen: nginx refuses a configuration naming files that do not exist, and the
# files cannot exist until this has run. Doing it the other way round leaves a
# node whose web server will not start.
echo "Obtaining a certificate before changing any paths."
# The one run that is allowed to end with nothing being served, because the
# paths are set in the next step. Every later run treats that as a fault.
ALLOW_UNSERVED=1 /usr/local/bin/simple-vpn-certificate

# Nginx has to read the files the agent writes, and this is the step that is
# easy to leave out: the agent installs a perfectly good certificate, reloads
# nginx, reports success - and the site goes on serving the old one from
# wherever it was originally put. That happened on the first run.
DOMAIN=$(sed -n 's/^NODE_DOMAIN=//p' /etc/simple-vpn-node.env)
SITE=/etc/nginx/sites-enabled/default

if ! grep -q "/etc/simple-vpn-tls/${DOMAIN}.crt" "${SITE}"; then
  cp "${SITE}" "/root/nginx-default.before-certificate-agent"
  sed -i \
    -e "s#ssl_certificate  *[^;]*;#ssl_certificate     /etc/simple-vpn-tls/${DOMAIN}.crt;#" \
    -e "s#ssl_certificate_key  *[^;]*;#ssl_certificate_key /etc/simple-vpn-tls/${DOMAIN}.key;#" \
    "${SITE}"

  if ! nginx -t >/dev/null 2>&1; then
    cp "/root/nginx-default.before-certificate-agent" "${SITE}"
    echo "Nginx refused the new certificate paths. Put back what was there."
    exit 1
  fi
  systemctl reload nginx
  echo "Nginx now reads the certificate the agent maintains."
fi

# And the agent says so, rather than this script assuming it. A configuration
# that passes nginx -t and a server that offers the right certificate are two
# different claims.
/usr/local/bin/simple-vpn-certificate

cat > /etc/systemd/system/simple-vpn-certificate.service <<'UNIT'
[Unit]
Description=Simple VPN node certificate
After=network-online.target nginx.service
Wants=network-online.target

[Service]
Type=oneshot
ExecStart=/usr/local/bin/simple-vpn-certificate
UNIT

cat > /etc/systemd/system/simple-vpn-certificate.timer <<'UNIT'
[Unit]
Description=Check this node's certificate daily

[Timer]
# Daily, at a time drawn from the machine's own name rather than a fixed hour.
# Every node asking at midnight would arrive at the authority together, which
# looks like one thing rather than several and wastes the allowance in bursts.
OnCalendar=daily
RandomizedDelaySec=6h
Persistent=true

[Install]
WantedBy=timers.target
UNIT

systemctl daemon-reload
systemctl enable -q --now simple-vpn-certificate.timer
echo "Timer installed. Next run:"
systemctl list-timers simple-vpn-certificate.timer --no-pager | sed -n '2p'
