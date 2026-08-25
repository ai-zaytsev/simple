#!/usr/bin/env bash
#
# Installs the agent that keeps this node's user list equal to the Control
# Plane's, and gives Xray the loopback interface it needs to be told about
# users while it runs.
#
# Run on the node, after /etc/simple-vpn-node.env has been written. The token
# is deliberately not an argument to this script and not an environment
# variable set on its command line: an argument list is readable by every
# process on the machine, and this one is the node's whole authority to ask who
# may connect.
#
# Expects /tmp/users-agent.py to have been copied over first.

set -euo pipefail

if [ ! -f /etc/simple-vpn-node.env ]; then
  echo "/etc/simple-vpn-node.env is missing. Write it first, over stdin, so the"
  echo "token never appears in a command line:"
  echo
  echo "  ssh node 'umask 077 && cat > /etc/simple-vpn-node.env' <<ENVFILE"
  echo "  CP_URL=https://simple-syncbridge.download"
  echo "  NODE_TOKEN=..."
  echo "  ENVFILE"
  exit 1
fi

chmod 600 /etc/simple-vpn-node.env
install -m 0755 /tmp/users-agent.py /usr/local/bin/simple-vpn-users
rm -f /tmp/users-agent.py

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
