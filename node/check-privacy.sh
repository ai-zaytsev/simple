#!/usr/bin/env bash
#
# Looks for the things this node must never be writing down.
#
# Read-only. It changes nothing and exits non-zero when it finds something, so
# it can be run at any time, on any node, and believed. The stage that required
# it was explicit: before launch, check every log - the VPN server, the reverse
# proxy, DNS, the operating system - and confirm that nothing forbidden is
# being kept automatically.
#
# Each check exists because the thing it looks for was found, or was one
# configuration word away from being true. None of them are hypothetical.

set -uo pipefail

FOUND=0

fault() {
  FOUND=$((FOUND + 1))
  echo "  FOUND: $*"
}

echo "=== Xray: does it write down where people go ==="
CONFIG=/usr/local/etc/xray/config.json
if [ ! -s "${CONFIG}" ]; then
  echo "  no Xray on this machine"
else
  access=$(python3 -c "import json;print(json.load(open('${CONFIG}')).get('log',{}).get('access','') or 'none')")
  level=$(python3 -c "import json;print(json.load(open('${CONFIG}')).get('log',{}).get('loglevel','warning'))")
  dnslog=$(python3 -c "import json;print(json.load(open('${CONFIG}')).get('log',{}).get('dnsLog',False))")

  # The access log is a list of every destination, one line each. There is no
  # version of it that is acceptable.
  [ "${access}" = "none" ] || fault "Xray access log is '${access}'; it must be none"

  # Verified rather than assumed: at info and debug this version writes every
  # address it dials, which is the same list under another name.
  case "${level}" in
    debug|info) fault "Xray loglevel is '${level}'; at that level every destination is written to the journal" ;;
    warning|error|none) echo "  loglevel ${level}, access none" ;;
    *) fault "Xray loglevel is '${level}', which is not one this check knows" ;;
  esac

  [ "${dnslog}" = "False" ] || fault "Xray dnsLog is on; that is a query log"

  echo "=== Xray journal: did anything leak into it anyway ==="
  leaked=$(journalctl -u xray --no-pager 2>/dev/null \
    | grep -cE "accepted (tcp|udp):|dialing (TCP|UDP) to|default route for" || true)
  if [ "${leaked}" -gt 0 ]; then
    fault "${leaked} lines in the Xray journal name a destination"
  else
    echo "  nothing in the journal names a destination"
  fi
fi

echo
echo "=== Nginx: client addresses and our own hidden path ==="
if ! command -v nginx >/dev/null 2>&1; then
  echo "  no nginx on this machine"
else
  # access_log off does not touch error_log, which is how a real user's
  # address and the tunnel path ended up on disk together.
  if nginx -T 2>/dev/null | grep -qE "^\s*access_log\s+[^;]*;" \
     && ! nginx -T 2>/dev/null | grep -qE "^\s*access_log\s+off\s*;"; then
    fault "an access_log is enabled somewhere in the nginx configuration"
  fi

  level=$(nginx -T 2>/dev/null | grep -oE "^\s*error_log\s+\S+\s+(debug|info|notice|warn|error|crit|alert|emerg)" \
    | awk '{print $3}' | head -1)
  case "${level:-error}" in
    alert|emerg) echo "  error_log at ${level}, below the level that names clients" ;;
    *) fault "nginx error_log is at '${level:-error}'; at that level it records client addresses and requested paths" ;;
  esac

  addresses=$(grep -ohE "client: [0-9a-fA-F.:]+" /var/log/nginx/*.log 2>/dev/null | sort -u | wc -l)
  if [ "${addresses}" -gt 0 ]; then
    fault "${addresses} distinct client addresses are recorded in /var/log/nginx"
  else
    echo "  no client addresses in /var/log/nginx"
  fi

  if grep -qhE "GET /[0-9a-f]{16,}" /var/log/nginx/*.log 2>/dev/null; then
    fault "the tunnel path appears in an nginx log"
  fi
fi

echo
echo "=== Resolver: is anybody keeping a query log ==="
for service in dnsmasq unbound bind9 named coredns; do
  if systemctl is-active "${service}" >/dev/null 2>&1; then
    fault "${service} is running on a node; a caching resolver here would hold a query history"
  fi
done

if systemctl is-active systemd-resolved >/dev/null 2>&1; then
  level=$(resolvectl log-level 2>/dev/null || echo unknown)
  if [ "${level}" = "debug" ]; then
    fault "systemd-resolved is at debug; at that level every lookup is written to the journal"
  else
    echo "  systemd-resolved at ${level}, which does not log lookups"
  fi
  # The cache holds recently resolved names in memory. Permitted, and stated
  # rather than glossed over: it is never written to disk, it expires with the
  # records in it, and it is the one place recent names transiently exist.
  echo "  cache in memory only: $(resolvectl statistics 2>/dev/null | awk '/Current Cache Size/{print $NF}') entries now"
fi

echo
echo "=== Everything else that writes ==="
if systemctl is-active auditd >/dev/null 2>&1; then
  fault "auditd is running and its rules have not been reviewed by this check"
fi
if [ -d /var/log/journal ]; then
  echo "  journal is persistent; what it holds is what the checks above cover"
fi

leftovers=$(grep -rlE "GET /[0-9a-f]{16,}|client: [0-9]{1,3}\." /var/log 2>/dev/null \
  | grep -v "^/var/log/journal" | head -5)
if [ -n "${leftovers}" ]; then
  fault "these files under /var/log hold client addresses or the tunnel path:"
  echo "${leftovers}" | sed 's/^/    /'
fi

echo
if [ "${FOUND}" -gt 0 ]; then
  echo "${FOUND} finding(s). This node is keeping something it must not."
  exit 1
fi
echo "Nothing found. This node keeps no record of where anybody went."
