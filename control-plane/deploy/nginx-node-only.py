#!/usr/bin/env python3
"""Lets only our own nodes reach the endpoints only our own nodes use.

Three endpoints exist for nodes and for nobody else: the list of who may
connect, the certificate request, and the report. All three were answering the
internet - with 401 and 405 rather than data, but answering.

That is a fingerprint. This domain is supposed to look like an ordinary site,
and an ordinary site does not have a /v1/node/users that says "unauthorised".
Anyone sweeping the internet for control planes finds one by asking.

The addresses come from the node table, so the list is what the service
believes rather than something maintained beside it. Run after adding or
removing a node.

Idempotent, tested before it is kept, and put back if nginx refuses it.
"""

import shutil
import subprocess
import sys
from pathlib import Path

SITE = Path("/etc/nginx/sites-available/default")
ALLOW = Path("/etc/nginx/simple-vpn-nodes.conf")
BACKUP = Path("/root/syncbridge.before-node-only")
DSN = Path("/etc/simple-vpn-cp.dsn")

CATCH_ALL_ANCHOR = "    location / { try_files $uri $uri/ =404; }"

INCLUDE = (
    "        include /etc/nginx/simple-vpn-nodes.conf;\n"
    # A refusal still says there is something here to refuse. Turned into the
    # same answer every other missing path gives, so that asking tells a
    # stranger nothing at all.
    "        error_page 403 = @hidden;"
)

HIDDEN = "    location @hidden { return 404; }\n"


def node_addresses():
    out = subprocess.run(
        ["psql", DSN.read_text().strip(), "-tAc",
         "select host from nodes where state <> 'removed' and host <> '0.0.0.0'"],
        capture_output=True, text=True,
    )
    if out.returncode != 0:
        print("Cannot read the node list; changing nothing.")
        sys.exit(1)

    found = [line.strip() for line in out.stdout.splitlines() if line.strip()]
    if not found:
        # Refused rather than guessed. An empty allow list would lock every
        # node out of the service that tells it who may connect, and the nodes
        # would go on serving until their lists went stale.
        print("The node table names no addresses. Refusing to write an empty list.")
        sys.exit(1)
    return found


def main():
    addresses = node_addresses()

    lines = ["# Written by control-plane/deploy/nginx-node-only.py from the node table.",
             "# Every address here is one of our own machines."]
    lines += [f"allow {address};" for address in sorted(addresses)]
    lines.append("deny all;")
    wanted = "\n".join(lines) + "\n"

    changed = False
    if not ALLOW.exists() or ALLOW.read_text() != wanted:
        ALLOW.write_text(wanted)
        changed = True
        print(f"  {len(addresses)} node address(es) allowed")

    body = SITE.read_text()
    original = body

    # Both places: the prefix location for /v1/node/ and the exact match for
    # the report, which would otherwise win and let the whole internet past.
    for anchor in ("location /v1/node/ {", "location = /v1/node/metrics {"):
        if anchor not in body:
            if anchor == "location /v1/node/ {":
                # The prefix location does not exist yet; make it, in front of
                # the general /v1/ one so that it wins.
                general = "    location /v1/ {"
                if general not in body:
                    print("Cannot find the /v1/ location. Change this by hand.")
                    sys.exit(1)
                block = (
                    "    # Only our own machines. These three endpoints are for nodes and\n"
                    "    # for nobody else, and answering a stranger at all - even with a\n"
                    "    # refusal - tells them what this domain is.\n"
                    "    location /v1/node/ {\n"
                    f"{INCLUDE}\n"
                    "        proxy_pass http://127.0.0.1:8080;\n"
                    "        proxy_http_version 1.1;\n"
                    "        proxy_set_header Host $host;\n"
                    "        proxy_read_timeout 30s;\n"
                    "        client_max_body_size 16k;\n"
                    "    }\n\n"
                )
                body = body.replace(general, block + general, 1)
                changed = True
            continue

        start = body.index(anchor)
        if INCLUDE not in body[start:start + 400]:
            body = body[:start + len(anchor)] + "\n" + INCLUDE + body[start + len(anchor):]
            changed = True

    if "location @hidden" not in body:
        body = body.replace(CATCH_ALL_ANCHOR, HIDDEN + "\n" + CATCH_ALL_ANCHOR, 1)
        changed = True

    if not changed:
        print("Already as it should be. Nothing to change.")
        return

    if body != original:
        shutil.copy2(SITE, BACKUP)
        SITE.write_text(body)

    if subprocess.run(["nginx", "-t"], capture_output=True).returncode != 0:
        if BACKUP.exists():
            shutil.copy2(BACKUP, SITE)
        print("Nginx refused it. Put back what was there.")
        subprocess.run(["nginx", "-t"])
        sys.exit(1)

    subprocess.run(["systemctl", "reload", "nginx"], check=True)
    print("Changed, and nginx has reloaded.")


if __name__ == "__main__":
    main()
