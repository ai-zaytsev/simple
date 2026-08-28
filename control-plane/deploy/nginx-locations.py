#!/usr/bin/env python3
"""Adds the two locations the Control Plane needs in front of it.

The proxy in front of this service is configured by hand on the host rather
than by the pipeline, so this is written down here instead of living only in
somebody's shell history. Run it on the Control Plane host after a deploy that
adds these endpoints.

Idempotent, tested before it is kept, and put back if nginx refuses it.
"""

import shutil
import subprocess
import sys
from pathlib import Path

SITE = Path("/etc/nginx/sites-available/default")
BACKUP = Path("/root/syncbridge.before-panel")

CATCH_ALL = "    location / { try_files $uri $uri/ =404; }"

BLOCK = """
    # A node report is larger than anything else here: a node that could not
    # reach us buffers its windows and delivers them together. An exact match,
    # so it wins over the /v1/ prefix without loosening the limit for the rest.
    location = /v1/node/metrics {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_read_timeout 30s;
        client_max_body_size 512k;
    }

    # The panel. The page carries no data and needs no check; the numbers
    # behind it are under /v1/ and need the key.
    location = /panel {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_read_timeout 30s;
    }

""" + CATCH_ALL


def main():
    body = SITE.read_text()
    if "/v1/node/metrics" in body:
        print("Already there. Nothing to change.")
        return
    if CATCH_ALL not in body:
        print("The catch-all location is not where this expects it. Change it by hand.")
        sys.exit(1)

    shutil.copy2(SITE, BACKUP)
    SITE.write_text(body.replace(CATCH_ALL, BLOCK, 1))

    if subprocess.run(["nginx", "-t"], capture_output=True).returncode != 0:
        shutil.copy2(BACKUP, SITE)
        print("Nginx refused it. Put back what was there.")
        sys.exit(1)

    subprocess.run(["systemctl", "reload", "nginx"], check=True)
    print("Added, and nginx has reloaded.")


if __name__ == "__main__":
    main()
