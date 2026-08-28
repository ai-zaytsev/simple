#!/usr/bin/env python3
"""Decides what the Control Plane shows to the internet.

The proxy in front of this service is configured by hand on the host rather
than by the pipeline, so what it should contain is written down here instead of
living in somebody's shell history. Run it on the Control Plane host after a
deploy that changes this list.

Three things, and the third is the point of the stage that added it:

  - a node's report is larger than anything else here, so it gets a limit of
    its own rather than loosening the limit for everything;

  - the panel is not served to the internet at all. It is a monitoring tool,
    and monitoring belongs on the closed side of the line however good its
    lock is. The service already listens on 127.0.0.1:8080, so reaching it
    is one command:

        ssh -L 9000:127.0.0.1:8080 <control-plane>

    and then http://localhost:9000/panel in a browser;

  - the numbers behind the panel are refused by the proxy, not merely
    unadvertised. A secret in a header is a good lock; a door that is not
    there is better, and both together cost nothing.

Idempotent, tested before it is kept, and put back if nginx refuses it.
"""

import shutil
import subprocess
import sys
from pathlib import Path

SITE = Path("/etc/nginx/sites-available/default")
BACKUP = Path("/root/syncbridge.before-closing-admin")

CATCH_ALL = "    location / { try_files $uri $uri/ =404; }"

METRICS = """    # A node report is larger than anything else here: a node that could not
    # reach us buffers its windows and delivers them together. An exact match,
    # so it wins over the /v1/ prefix without loosening the limit for the rest.
    location = /v1/node/metrics {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_read_timeout 30s;
        client_max_body_size 512k;
    }
"""

CLOSED = """    # Monitoring, closed. The panel and the numbers behind it are internal
    # tools, and the stage that closed them asks that scanning this service
    # find no such thing. Reach them through the machine instead:
    #   ssh -L 9000:127.0.0.1:8080 <this host>   then http://localhost:9000/panel
    location = /panel { return 404; }
    location /v1/admin/ { return 404; }
"""

# The block this replaces, from when the panel was served publicly.
OLD_PANEL = """    # The panel. The page carries no data and needs no check; the numbers
    # behind it are under /v1/ and need the key.
    location = /panel {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_read_timeout 30s;
    }
"""


def main():
    body = SITE.read_text()
    original = body

    if OLD_PANEL in body:
        body = body.replace(OLD_PANEL, "", 1)

    if "location = /v1/node/metrics" not in body:
        if CATCH_ALL not in body:
            print("The catch-all location is not where this expects it. Change it by hand.")
            sys.exit(1)
        body = body.replace(CATCH_ALL, METRICS + "\n" + CATCH_ALL, 1)

    if "location /v1/admin/" not in body:
        if CATCH_ALL not in body:
            print("The catch-all location is not where this expects it. Change it by hand.")
            sys.exit(1)
        body = body.replace(CATCH_ALL, CLOSED + "\n" + CATCH_ALL, 1)

    if body == original:
        print("Already as it should be. Nothing to change.")
        return

    shutil.copy2(SITE, BACKUP)
    SITE.write_text(body)

    if subprocess.run(["nginx", "-t"], capture_output=True).returncode != 0:
        shutil.copy2(BACKUP, SITE)
        print("Nginx refused it. Put back what was there.")
        sys.exit(1)

    subprocess.run(["systemctl", "reload", "nginx"], check=True)
    print("Changed, and nginx has reloaded.")


if __name__ == "__main__":
    main()
