#!/usr/bin/env bash
#
# Lets in what people need and nothing else.
#
# Before this, every machine had no firewall at all: what was safe was that
# nothing happened to be listening on a public address. That is a different
# property from nothing being able to. A monitoring agent, a database opened
# for a moment to debug something, a package that starts a daemon - any of
# those turns the first property off without anybody noticing, and the second
# one holds regardless.
#
# On a node, management is not something the internet gets to see. Port 22 is
# opened to the Control Plane and to nobody else, which makes that machine the
# single way in and makes a scan of a VPN node show a web server and nothing
# else. The Control Plane itself keeps 22 open, because the pipeline has to
# reach it from addresses nobody can list in advance.
#
# Applying this over the connection it might close is why the revert exists: a
# timer disables the firewall a few minutes from now unless somebody, having
# checked that they can still get in, stops it.
#
# Expects in the environment:
#   ALLOW_SSH_FROM   an address, or "any"
#   REVERT_AFTER     seconds before the firewall undoes itself (default 180)

set -euo pipefail

: "${ALLOW_SSH_FROM:?ALLOW_SSH_FROM is required: an address, or the word any}"
REVERT_AFTER="${REVERT_AFTER:-180}"

if ! command -v ufw >/dev/null 2>&1; then
  export DEBIAN_FRONTEND=noninteractive
  apt-get update -qq
  apt-get install -y -qq ufw >/dev/null
fi

# Armed before anything changes. If the rules below cut this session off, the
# machine lets itself back in without anybody having to reach it.
#
# Zero means no revert, which is right when nothing is going to reach this
# machine again anyway: a firewall that undoes itself minutes after a build has
# finished leaves the node open with nobody watching.
systemctl stop simple-vpn-firewall-revert.timer 2>/dev/null || true
if [ "${REVERT_AFTER}" -gt 0 ]; then
  systemd-run --quiet --on-active="${REVERT_AFTER}" \
    --unit=simple-vpn-firewall-revert \
    /usr/sbin/ufw --force disable
  echo "  a revert is armed for ${REVERT_AFTER} seconds from now"
else
  echo "  no revert armed"
fi

ufw --force reset >/dev/null
ufw default deny incoming >/dev/null
ufw default allow outgoing >/dev/null

# The site and the tunnel. Both are the product; both are meant to be found.
ufw allow 80/tcp  >/dev/null
ufw allow 443/tcp >/dev/null

if [ "${ALLOW_SSH_FROM}" = "any" ]; then
  ufw allow 22/tcp >/dev/null
  echo "  22 open to everybody"
else
  ufw allow from "${ALLOW_SSH_FROM}" to any port 22 proto tcp >/dev/null
  echo "  22 open to ${ALLOW_SSH_FROM} only"
fi

ufw --force enable >/dev/null

echo "  now in force:"
ufw status | sed -n '1,12p' | sed 's/^/    /'
echo
echo "  If you can still reach this machine, stop the revert with:"
echo "    systemctl stop simple-vpn-firewall-revert.timer"
