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
