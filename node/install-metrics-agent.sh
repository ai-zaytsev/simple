#!/usr/bin/env bash
#
# Makes this node countable, and stops it writing down what it must not.
#
# Two jobs in one script because they are one decision. Turning on measurement
# without first turning off the recording would be the exact mistake this stage
# exists to prevent: a service that watches itself by keeping a history of
# where its users went.
#
# Expects /tmp/xray-observability.py, /tmp/metrics-agent.py and
# /tmp/check-privacy.sh to have been copied over, and
# /etc/simple-vpn-node.env to carry CP_URL and NODE_TOKEN.

set -euo pipefail

for file in xray-observability.py metrics-agent.py check-privacy.sh; do
  if [ ! -s "/tmp/${file}" ]; then
    echo "/tmp/${file} is missing. Copy it over first."
    exit 1
  fi
done

echo "=== Counting ==="
# Installed as a module, not run and thrown away: the metrics agent imports it
# to rebuild the routing table when the group list changes, and two copies of
# the rule order would be two copies to drift apart.
install -d -m 0755 /usr/local/lib/simple-vpn
install -m 0644 /tmp/xray-observability.py /usr/local/lib/simple-vpn/xray_observability.py
python3 /usr/local/lib/simple-vpn/xray_observability.py

echo
echo "=== Logs ==="

# access_log off does not touch error_log, and error_log at its default level
# records the client address and the requested path for every connection that
# ends badly. On this fleet that meant a real user's address on disk next to
# the tunnel path, from a node whose whole design is to reveal neither.
#
# alert rather than off: the messages that matter for running the machine -
# running out of worker connections, failing to bind - are logged above this
# level and carry no client. The per-connection noise is below it.
CONF=/etc/nginx/nginx.conf
if [ -f "${CONF}" ] && ! grep -qE "^\s*error_log\s+\S+\s+alert;" "${CONF}"; then
  cp "${CONF}" /root/nginx.conf.before-privacy
  sed -i -E "s#^(\s*)error_log\s+([^;[:space:]]+)([^;]*);#\1error_log \2 alert;#" "${CONF}"
  if ! nginx -t >/dev/null 2>&1; then
    cp /root/nginx.conf.before-privacy "${CONF}"
    echo "Nginx refused the change. Put back what was there."
    exit 1
  fi
  systemctl reload nginx
  echo "  nginx no longer records client addresses"
else
  echo "  nginx error level already set"
fi

# What is already written stays written until it is removed. Truncated rather
# than deleted so that ownership and permissions survive and nginx keeps
# writing to the same open handles.
purged=0
for log in /var/log/nginx/*.log /var/log/nginx/*.log.[0-9]; do
  [ -f "${log}" ] || continue
  if grep -qE "client: |GET /[0-9a-f]{16,}" "${log}" 2>/dev/null; then
    : > "${log}"
    purged=$((purged + 1))
  fi
done
[ "${purged}" -eq 0 ] && echo "  nothing to purge" || echo "  purged ${purged} log file(s) holding client addresses"

# Old rotated copies are compressed and cannot be truncated in place.
gone=$(find /var/log/nginx -name "*.gz" -delete -print 2>/dev/null | wc -l)
[ "${gone}" -gt 0 ] && echo "  removed ${gone} compressed rotation(s)"

echo
echo "=== Agent ==="
install -m 0755 /tmp/metrics-agent.py /usr/local/bin/simple-vpn-metrics
install -m 0755 /tmp/check-privacy.sh /usr/local/bin/simple-vpn-check-privacy
rm -f /tmp/metrics-agent.py /tmp/check-privacy.sh /tmp/xray-observability.py

cat > /etc/systemd/system/simple-vpn-metrics.service <<'UNIT'
[Unit]
Description=Simple VPN node metrics
After=network-online.target xray.service
Wants=network-online.target

[Service]
# A service rather than a timer, because the counters are read with reset: what
# has been read and not yet delivered exists only in this process's memory, and
# a process that exits after every reading would drop it.
Type=simple
ExecStart=/usr/local/bin/simple-vpn-metrics
Restart=always
RestartSec=30

# Nothing it produces belongs on disk.
StandardOutput=null
StandardError=journal

[Install]
WantedBy=multi-user.target
UNIT

systemctl daemon-reload
systemctl enable -q --now simple-vpn-metrics.service
sleep 2
systemctl is-active --quiet simple-vpn-metrics.service \
  && echo "  agent running" \
  || { echo "  agent did not start"; journalctl -u simple-vpn-metrics -n 10 --no-pager; exit 1; }

echo
echo "=== Audit ==="
# The installation ends with the check, not with a claim. A node that has just
# been set up for measurement is exactly the node most likely to have acquired
# a log nobody asked for.
/usr/local/bin/simple-vpn-check-privacy
