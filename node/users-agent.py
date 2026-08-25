#!/usr/bin/env python3
"""Keeps this node's list of who may connect equal to the Control Plane's.

Why a node asks instead of being told: a node sits behind whatever network its
provider gives it, can be rebuilt at any moment, and must not accept
connections from anything but the people it serves. A service that reached in
to push changes would need a way through to every node and a port open to
receive them. Asking needs neither.

Why the whole list rather than what changed: a node that misses half a delta
has an idea of who may connect that quietly drifts from the truth and never
comes back. With the whole list, this node's state is a function of the last
answer rather than of every answer it has ever seen.

Users are added and removed while Xray runs. Rewriting the configuration and
restarting would drop every established connection, so one person signing in
would disconnect everybody else.
"""
import json
import os
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.request

CONTROL_PLANE = os.environ.get("CP_URL", "").rstrip("/")
TOKEN = os.environ.get("NODE_TOKEN", "")
INBOUND_TAG = os.environ.get("INBOUND_TAG", "ws-in")
INBOUND_PORT = int(os.environ.get("INBOUND_PORT", "10000"))
API = os.environ.get("XRAY_API", "127.0.0.1:10085")
INTERVAL = int(os.environ.get("SYNC_INTERVAL_S", "10"))

# Ten seconds because that is roughly the gap between somebody following a link
# in their mailbox and pressing the button in the application. Their own
# credential has to be here by then, and nothing is gained by asking faster.


def log(message):
    print(message, flush=True)


def wanted():
    """Who the Control Plane says may connect."""
    request = urllib.request.Request(
        CONTROL_PLANE + "/v1/node/users",
        headers={"authorization": "Bearer " + TOKEN, "accept": "application/json"},
    )
    with urllib.request.urlopen(request, timeout=15) as response:
        body = json.load(response)
    return set(body.get("credentials", []))


def present():
    """Who Xray currently accepts.

    Users added through the configuration file have no label and do not appear
    here. That is deliberate rather than a limitation: it means this agent can
    never remove the shared credential the node was built with, and removing
    that is a decision taken once, by hand, when every device has its own.
    """
    result = subprocess.run(
        ["xray", "api", "inbounduser", "--server=" + API, "-tag=" + INBOUND_TAG],
        capture_output=True, text=True, timeout=20,
    )
    if result.returncode != 0:
        raise RuntimeError("xray did not answer: " + result.stderr.strip())
    if not result.stdout.strip() or result.stdout.strip() == "{}":
        return set()
    body = json.loads(result.stdout)
    return {user["email"] for user in body.get("users", []) if user.get("email")}


def add(credentials):
    """Adds users in one call, because each call is a round trip to Xray."""
    clients = [{"id": credential, "email": credential} for credential in credentials]
    document = {
        "inbounds": [{
            "tag": INBOUND_TAG,
            "listen": "127.0.0.1",
            "port": INBOUND_PORT,
            "protocol": "vless",
            "settings": {"clients": clients, "decryption": "none"},
        }]
    }

    with tempfile.NamedTemporaryFile("w", suffix=".json", delete=False) as handle:
        json.dump(document, handle)
        path = handle.name
    try:
        result = subprocess.run(
            ["xray", "api", "adu", "--server=" + API, path],
            capture_output=True, text=True, timeout=30,
        )
        if result.returncode != 0:
            raise RuntimeError("could not add users: " + result.stderr.strip())
    finally:
        os.unlink(path)


def remove(credentials):
    result = subprocess.run(
        ["xray", "api", "rmu", "--server=" + API, "-tag=" + INBOUND_TAG] + list(credentials),
        capture_output=True, text=True, timeout=30,
    )
    if result.returncode != 0:
        raise RuntimeError("could not remove users: " + result.stderr.strip())


def sync():
    should = wanted()
    has = present()

    missing = should - has
    extra = has - should

    if missing:
        add(missing)
    if extra:
        remove(extra)

    # Counts, never the credentials themselves. A node is allowed to know who
    # may connect; its log archive is not a second copy of that list.
    if missing or extra:
        log("users: %d total, %d added, %d removed" % (len(should), len(missing), len(extra)))
    return len(should)


def main():
    if not CONTROL_PLANE or not TOKEN:
        log("CP_URL and NODE_TOKEN are required")
        return 1

    log("watching %s every %ds, inbound %s" % (CONTROL_PLANE, INTERVAL, INBOUND_TAG))
    failures = 0
    while True:
        try:
            sync()
            failures = 0
        except (urllib.error.URLError, OSError, RuntimeError, ValueError) as problem:
            # A failure to reach the Control Plane must not empty the list. The
            # people already on this node keep working; the only thing lost is
            # learning about changes, and that resumes by itself.
            failures += 1
            if failures in (1, 10) or failures % 60 == 0:
                log("sync failed (%d in a row): %s" % (failures, problem))
        time.sleep(INTERVAL)


if __name__ == "__main__":
    sys.exit(main())
