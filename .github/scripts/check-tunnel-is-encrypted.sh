#!/usr/bin/env bash
#
# The tunnel cannot be reached except through TLS.
#
# VLESS has no encryption of its own - the protocol requires
# "decryption": "none" - so everything depends on what carries it. On a node
# that is Nginx: it terminates TLS on 443 and hands the WebSocket inward to
# Xray on the loopback. Two things hold that, and neither is visible in any
# test:
#
#   the engine binds 127.0.0.1, so nothing can reach it from outside;
#   port 80 only redirects, so nothing is carried before TLS starts.
#
# Change either and the node still works. It would serve the same tunnel to
# the same users, and the traffic would be readable by anyone on the path -
# which is a failure with no symptom at all.
set -euo pipefail

FILE="node/provision.sh"
failed=0

[ -f "${FILE}" ] || { echo "${FILE} is missing"; exit 1; }

# The engine listens to the device only through Nginx.
if ! grep -A 3 '"protocol": "vless"' "${FILE}" >/dev/null 2>&1; then
    echo "${FILE}: no VLESS inbound found; this check no longer knows what it is reading"
    failed=1
fi

vless_listen=$(grep -B 3 '"protocol": "vless"' "${FILE}" | grep '"listen"' | head -1)
case "${vless_listen}" in
    *127.0.0.1*) ;;
    "") echo "${FILE}: the VLESS inbound does not say what it listens on"; failed=1 ;;
    *) echo "${FILE}: the VLESS inbound is reachable beyond the loopback:"
       echo "  ${vless_listen}"
       failed=1 ;;
esac

# Port 80 carries nothing. It exists to send people to 443.
plain=$(awk '/listen 80/{inside=1} inside{print} inside && /^}/{exit}' "${FILE}")
case "${plain}" in
    *"return 301 https"*) ;;
    *) echo "${FILE}: the plain-HTTP server does not redirect to HTTPS"; failed=1 ;;
esac
case "${plain}" in
    *proxy_pass*) echo "${FILE}: the plain-HTTP server proxies something; it must only redirect"
                  failed=1 ;;
esac

if [ "${failed}" -ne 0 ]; then
    echo
    echo "A tunnel carried in the clear works exactly like one that is not."
    echo "Nothing downstream of this notices, which is why it is checked here."
    exit 1
fi

echo "ok: the tunnel is reachable only through TLS"
