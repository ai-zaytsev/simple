#!/usr/bin/env bash
#
# Holds marked traffic to a speed, and leaves everything else alone.
#
# The mark comes from Xray: connections belonging to a limited account are sent
# through outbounds that set it on the socket. Nothing here knows about
# accounts, tiers or people - it sees packets carrying a number, and slows
# those. That is the whole reason the limit is expressed as a mark: a shaper
# works below the engine, on the kernel's queues, where there are no users.
#
# Both directions. Egress is straightforward. Ingress cannot be shaped
# directly, so it is redirected onto a virtual device and shaped there, which
# is the standard way and the only one that does not require the far end to
# cooperate. Download is the direction people notice, so getting only egress
# right would look like the limit not working.
#
# The return packets carry no mark of their own - the mark was set on the
# outgoing socket - so the connection's mark is saved and restored. Without
# that, a limit would apply to uploads and to nothing else.
#
# And it did, for a while, because saving and restoring is not sufficient on
# its own. The restore lives in netfilter; traffic control on the way in runs
# before netfilter, so the mark was not there yet when the incoming filter
# looked for it. The fetch has to happen inside the filter, and that is what
# `action connmark` below is for. This paragraph is here because the two lines
# above read as though they were enough, and a phone measured eight megabits
# up against a hundred and eleven down while they were.
#
# Run twice and the second run changes nothing.
set -euo pipefail

MBIT="${1:-}"
IFACE="${2:-}"
MARK="${MARK:-0x51}"

if [ -z "${MBIT}" ]; then
    echo "usage: speed-limit.sh <mbit|none> [interface]" >&2
    exit 2
fi

if [ -z "${IFACE}" ]; then
    # The interface the default route leaves by. Asked rather than assumed:
    # providers name them differently and a wrong guess shapes nothing while
    # reporting success.
    IFACE=$(ip route show default | awk '/default/ {print $5; exit}')
fi
if [ -z "${IFACE}" ]; then
    echo "cannot tell which interface carries the default route" >&2
    exit 1
fi

# Everything this script installed, removed in the order that leaves the
# machine reachable at every step. Ingress redirection goes first, because it
# is the piece that would otherwise keep sending packets to a device that no
# longer has a queue.
teardown() {
    tc qdisc del dev "${IFACE}" ingress 2>/dev/null || true
    tc qdisc del dev ifb-slow root 2>/dev/null || true
    ip link del ifb-slow 2>/dev/null || true
    tc qdisc del dev "${IFACE}" root 2>/dev/null || true

    iptables -t mangle -D POSTROUTING -m mark --mark "${MARK}" \
        -j CONNMARK --save-mark 2>/dev/null || true
    iptables -t mangle -D PREROUTING -j CONNMARK --restore-mark 2>/dev/null || true
}

if [ "${MBIT}" = "none" ]; then
    teardown
    echo "No speed limit. Shaping removed from ${IFACE}."
    exit 0
fi

case "${MBIT}" in
    ''|*[!0-9]*) echo "the speed must be a whole number of megabits" >&2; exit 2 ;;
esac

teardown

# The mark travels with the connection, so that packets coming back carry it
# too. Saved on the way out, restored on the way in.
iptables -t mangle -A POSTROUTING -m mark --mark "${MARK}" -j CONNMARK --save-mark
iptables -t mangle -A PREROUTING -j CONNMARK --restore-mark

# Egress. The default class is the whole link: anything without the mark - our
# own administration included - is not touched by this at all.
tc qdisc add dev "${IFACE}" root handle 1: htb default 1
tc class add dev "${IFACE}" parent 1: classid 1:1 htb rate 10gbit
tc class add dev "${IFACE}" parent 1: classid 1:9 htb rate "${MBIT}mbit" ceil "${MBIT}mbit"
tc qdisc add dev "${IFACE}" parent 1:9 handle 90: fq_codel
tc filter add dev "${IFACE}" parent 1: protocol all prio 1 handle "${MARK}" fw flowid 1:9

# Ingress, by way of a virtual device. A real interface has no queue on the
# way in, so the packets are moved to one that does.
modprobe ifb numifbs=0 2>/dev/null || true
ip link add ifb-slow type ifb 2>/dev/null || true
ip link set ifb-slow up

# The action that reads a connection's mark, asked for by name.
#
# Loaded on purpose rather than left to appear when first used: a missing
# module makes the filter below fail, the script stop, and the shaping that
# was already applied stay - which is a limit on uploads only, and that is
# precisely the fault this change exists to remove.
modprobe act_connmark 2>/dev/null || true

tc qdisc add dev "${IFACE}" handle ffff: ingress

# The mark is fetched here, before the packet is moved.
#
# This is the whole of why the limit held one direction and not the other. A
# packet arriving from the internet carries no mark of its own: the mark was
# put on the socket going out, and the rule that copies it back from the
# connection lives in netfilter's PREROUTING. Traffic control on the way in
# runs *before* netfilter, so by the time this filter looked, there was
# nothing to look at - every incoming packet fell into the default class and
# went at full speed.
#
# Uploads were held correctly the whole time, because on the way out traffic
# control runs after netfilter and the mark is already there. A limit that
# works in one direction is the shape this mistake takes, and it is what a
# person notices: the download is the number on the speed test.
#
# `action connmark` reads the connection's mark into the packet at this point,
# which is exactly what it exists for.
tc filter add dev "${IFACE}" parent ffff: protocol all prio 1 u32 \
    match u32 0 0 \
    action connmark \
    action mirred egress redirect dev ifb-slow

tc qdisc add dev ifb-slow root handle 1: htb default 1
tc class add dev ifb-slow parent 1: classid 1:1 htb rate 10gbit
tc class add dev ifb-slow parent 1: classid 1:9 htb rate "${MBIT}mbit" ceil "${MBIT}mbit"
tc qdisc add dev ifb-slow parent 1:9 handle 90: fq_codel
tc filter add dev ifb-slow parent 1: protocol all prio 1 handle "${MARK}" fw flowid 1:9

echo "Marked traffic held to ${MBIT} Mbit/s on ${IFACE}, both directions."
echo "Everything unmarked, including this connection, is untouched."
