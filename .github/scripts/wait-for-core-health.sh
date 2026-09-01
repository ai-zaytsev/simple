#!/usr/bin/env bash
#
# Asking the Control Plane whether it is working, in one place.
#
# Three workflows decide things by this answer: the deploy, the restore into
# production, and the rollback. They asked in three slightly different ways,
# and one of them asked for a status code nobody had checked against the
# handler - it waited for 204 while the handler answers 200. That check turned
# the first successful restore into production into a red run.
#
# It was the fourth time in this piece of work that the check was the thing
# that broke rather than the thing checked. What all four shared: the
# expectation was written down apart from the thing that produces it, free to
# drift in silence. So there is one copy here, and control-plane's health_test
# holds the other end of the contract.
#
# The body, not the code. A word cannot drift the way a number typed from
# memory can, and 200 is also what a stray proxy or a placeholder returns
# while the service behind it is gone.
#
# Usage: wait-for-core-health.sh <host> [tries]
set -euo pipefail

HOST="${1:?host}"
TRIES="${2:-20}"

ssh_host() {
    ssh -i ~/.ssh/deploy -o BatchMode=yes "root@${HOST}" "$@"
}

# Tries, not one attempt. A service just handed a different database opens a
# pool and runs migrations before it listens; a single attempt reported a
# failure that had already finished succeeding.
answer=""
for _ in $(seq 1 "${TRIES}"); do
    answer=$(ssh_host 'curl -sS --max-time 10 http://127.0.0.1:8080/healthz' || true)
    case "${answer}" in *'"ok"'*) break ;; esac
    sleep 5
done

case "${answer}" in
    *'"ok"'*)
        echo "The service answers: ${answer}"
        exit 0
        ;;
esac

# What systemd and the service itself say. "No answer" is the symptom; these
# are the two places the reason is written.
ssh_host 'systemctl is-active simple-vpn-core; journalctl -u simple-vpn-core -n 20 --no-pager -o cat' || true
echo "The service did not answer. It said: ${answer:-nothing}"
exit 1
