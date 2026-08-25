#!/usr/bin/env bash
#
# Installs the agent that keeps this node's user list equal to the Control
# Plane's, and gives Xray the loopback interface it needs to be told about
# users while it runs.
#
# Run on the node. Expects CP_URL and NODE_TOKEN in the environment; the token
# is written to a file readable only by root and never appears in an argument
# list, because an argument list is visible to every process on the machine.

set -euo pipefail

: "${CP_URL:?CP_URL is required}"
: "${NODE_TOKEN:?NODE_TOKEN is required}"

INBOUND_TAG="${INBOUND_TAG:-ws-in}"
INBOUND_PORT="${INBOUND_PORT:-10000}"

install -m 0755 /tmp/users-agent.py /usr/local/bin/simple-vpn-users

umask 077
cat > /etc/simple-vpn-node.env <<ENVFILE
CP_URL=${CP_URL}
NODE_TOKEN=${NODE_TOKEN}
INBOUND_TAG=${INBOUND_TAG}
INBOUND_PORT=${INBOUND_PORT}
ENVFILE

cat > /etc/systemd/system/simple-vpn-users.service <<'UNIT'
[Unit]
Description=Simple VPN node user list
After=network-online.target xray.service
Wants=xray.service

[Service]
EnvironmentFile=/etc/simple-vpn-node.env
ExecStart=/usr/local/bin/simple-vpn-users
Restart=always
RestartSec=5
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
UNIT

systemctl daemon-reload
systemctl enable -q --now simple-vpn-users
systemctl restart simple-vpn-users
sleep 3
systemctl is-active simple-vpn-users
