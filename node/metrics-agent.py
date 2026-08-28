#!/usr/bin/env python3
"""Reports how this node is doing, and nothing about where anyone went.

Every number here is either a property of the machine or a total. There is no
field for a destination, a domain, a DNS query or an address, so no amount of
keeping these samples builds a browsing history - not because the history is
stripped on the way out, but because it is never assembled.

Counters are read with reset, so the node itself accumulates nothing. What has
been read and not yet delivered is held in memory and retried; if the process
dies, that window is lost, which is the correct trade for a node that is not
supposed to remember anything.

One command is deliberately not used. `xray api statsonlineiplist` returns the
source addresses of a user's live sessions together with the times they
connected. It exists, it would work, and it is exactly the record this stage
forbids. It is named here so that its absence is visibly a decision.
"""

import json
import os
import re
import subprocess
import sys
import time
import urllib.error
import urllib.request

API = "127.0.0.1:10085"
INBOUND_TAG = os.environ.get("INBOUND_TAG", "ws-in")
INBOUND_PORT = int(os.environ.get("INBOUND_PORT", "10000"))

# One minute. Short enough that a node going bad is visible while it matters,
# long enough that the reporting is not itself a load worth measuring.
INTERVAL_S = 60

# How many undelivered samples to hold while the Control Plane is unreachable.
# Two hours of them. Past that the oldest are dropped: a node that has been
# cut off for hours has a bigger problem than its missing counters, and a
# buffer that grows without limit is a node that eventually falls over.
MAX_PENDING = 120


def env(name, default=None):
    for line in open("/etc/simple-vpn-node.env"):
        line = line.strip()
        if line.startswith(name + "="):
            return line.split("=", 1)[1]
    if default is None:
        print(f"{name} is required in /etc/simple-vpn-node.env", file=sys.stderr)
        sys.exit(1)
    return default


CP_URL = env("CP_URL")
NODE_TOKEN = env("NODE_TOKEN")


def xray(*args):
    out = subprocess.run(
        ["/usr/local/bin/xray", "api", *args, "--server=" + API],
        capture_output=True, text=True, timeout=15,
    )
    if out.returncode != 0:
        return None
    try:
        return json.loads(out.stdout)
    except json.JSONDecodeError:
        return None


def counters():
    """Byte totals, taken and zeroed in one step.

    Reset is what keeps the node free of history: after this call the node
    holds nothing that was not produced in the last minute.
    """
    answer = xray("statsquery", "-reset")
    if not answer:
        return {}, {}, {}

    per_user, per_class, per_inbound = {}, {}, {}
    for stat in answer.get("stat", []):
        name = stat.get("name", "")
        value = int(stat.get("value", 0) or 0)
        if value == 0:
            continue
        parts = name.split(">>>")
        if len(parts) != 4:
            continue
        kind, who, _, direction = parts
        if kind == "user":
            per_user.setdefault(who, {"up": 0, "down": 0})
            per_user[who]["up" if direction == "uplink" else "down"] += value
        elif kind == "outbound" and who.startswith("class-"):
            cls = who[len("class-"):]
            per_class.setdefault(cls, {"up": 0, "down": 0})
            per_class[cls]["up" if direction == "uplink" else "down"] += value
        elif kind == "inbound":
            per_inbound.setdefault(who, {"up": 0, "down": 0})
            per_inbound[who]["up" if direction == "uplink" else "down"] += value

    return per_user, per_class, per_inbound


def tunnel_connections():
    """How many connections the node is carrying through the tunnel.

    Counted from the sockets nginx has open into the tunnel inbound, which is
    the number the question was really about: one open connection is one thing
    somebody is doing through the VPN right now.

    Xray offers `statsonline` for this and it does not work. It answers
    NotFound with the policy flag on and off, for a user declared in the
    configuration file and for one added at runtime, while a phone is watching
    video through the node - which is how the panel came to report nobody
    connected while somebody was connected. This number can be checked from a
    shell in one second, and that is why it is this one.

    The total connection count is reported separately and is not the same
    number: an idle node still shows a few, because anything scanning the
    internet reaches the cover site.
    """
    out = subprocess.run(
        ["ss", "-Htn", "state", "established", "( dport = :%d )" % INBOUND_PORT],
        capture_output=True, text=True, timeout=10,
    )
    if out.returncode != 0:
        return None
    return sum(1 for line in out.stdout.splitlines() if line.strip())


def configured_users():
    answer = xray("inbounduser", "-tag=" + INBOUND_TAG)
    if not answer:
        return []
    return [u.get("email", "") for u in answer.get("users", []) if u.get("email")]


def cpu_percent(previous):
    """Busy time as a share of all time, between this call and the last."""
    with open("/proc/stat") as f:
        fields = [int(x) for x in f.readline().split()[1:]]
    idle = fields[3] + fields[4]
    total = sum(fields)
    if previous is None:
        return None, (idle, total)
    idle0, total0 = previous
    d_total = total - total0
    if d_total <= 0:
        return None, (idle, total)
    return round(100.0 * (1.0 - (idle - idle0) / d_total), 1), (idle, total)


def memory_percent():
    values = {}
    for line in open("/proc/meminfo"):
        key, _, rest = line.partition(":")
        values[key] = int(rest.split()[0])
    total = values.get("MemTotal", 0)
    available = values.get("MemAvailable", 0)
    if not total:
        return None
    return round(100.0 * (total - available) / total, 1)


def established():
    """Live TCP connections on the node.

    A count and only a count. `ss` prints addresses; they are consumed by the
    pipe and never leave this function.
    """
    out = subprocess.run(
        ["ss", "-Htn", "state", "established"],
        capture_output=True, text=True, timeout=10,
    )
    if out.returncode != 0:
        return None
    return sum(1 for line in out.stdout.splitlines() if line.strip())


def upstream_quality():
    """Latency and loss from this node outwards.

    Measured against a fixed anchor of our choosing, not against anywhere a
    user went. It answers "is this machine's own network healthy", which is a
    question about the node and not about anybody using it.
    """
    out = subprocess.run(
        ["ping", "-n", "-q", "-c", "5", "-w", "8", "1.1.1.1"],
        capture_output=True, text=True, timeout=15,
    )
    text = out.stdout
    loss = re.search(r"(\d+(?:\.\d+)?)% packet loss", text)
    rtt = re.search(r"= [\d.]+/([\d.]+)/", text)
    return (
        float(rtt.group(1)) if rtt else None,
        float(loss.group(1)) if loss else None,
    )


def system_stats():
    answer = xray("statssys")
    if not answer:
        return {}
    return {
        "goroutines": int(answer.get("NumGoroutine", 0) or 0),
        "heap_bytes": int(answer.get("Alloc", 0) or 0),
        "xray_uptime_s": int(answer.get("Uptime", 0) or 0),
    }


def sample(cpu_prev):
    per_user, per_class, per_inbound = counters()
    emails = configured_users()
    cpu, cpu_prev = cpu_percent(cpu_prev)
    latency, loss = upstream_quality()

    inbound = per_inbound.get(INBOUND_TAG, {"up": 0, "down": 0})

    with open("/proc/loadavg") as f:
        load1 = float(f.readline().split()[0])

    body = {
        "at": int(time.time()),
        "window_s": INTERVAL_S,
        "users_configured": len(emails),
        "sessions_online": tunnel_connections(),
        "cpu_percent": cpu,
        "load1": load1,
        "memory_percent": memory_percent(),
        "established": established(),
        "uplink_bytes": inbound["up"],
        "downlink_bytes": inbound["down"],
        "upstream_latency_ms": latency,
        "upstream_loss_percent": loss,
        # Per credential, so the Control Plane can turn it into a total per
        # account. The node cannot do that itself: it has never been told which
        # credential belongs to whom, and this is one of the reasons.
        "credential_bytes": {k: v["up"] + v["down"] for k, v in per_user.items()},
        # Per class, with no user in it at all. The two dictionaries above and
        # below cannot be joined, here or anywhere downstream.
        "class_bytes": {k: v for k, v in per_class.items()},
    }
    body.update(system_stats())
    return body, cpu_prev


def deliver(pending):
    payload = json.dumps({"samples": pending}).encode()
    request = urllib.request.Request(
        CP_URL + "/v1/node/metrics",
        data=payload,
        headers={
            "authorization": "Bearer " + NODE_TOKEN,
            "content-type": "application/json",
        },
        method="POST",
    )
    with urllib.request.urlopen(request, timeout=30) as answer:
        return answer.status == 200


def main():
    pending = []
    cpu_prev = None

    # A first reading only establishes the baseline for the CPU delta, and its
    # counters cover an unknown stretch of time since the last restart. Taken
    # and thrown away rather than reported as if it were a minute.
    _, cpu_prev = sample(cpu_prev)

    while True:
        time.sleep(INTERVAL_S)
        try:
            body, cpu_prev = sample(cpu_prev)
            pending.append(body)
        except Exception as problem:  # noqa: BLE001 - a bad sample must not stop the agent
            print("could not take a sample:", type(problem).__name__, file=sys.stderr)

        if not pending:
            continue

        pending = pending[-MAX_PENDING:]
        try:
            if deliver(pending):
                pending = []
        except (urllib.error.URLError, OSError) as problem:
            print("could not deliver:", type(problem).__name__, file=sys.stderr)


if __name__ == "__main__":
    main()
