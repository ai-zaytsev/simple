#!/usr/bin/env python3
"""Teaches this node's Xray to count, without teaching it to remember.

Adds three things to the configuration: counters, a management service allowed
to read them, and a set of outbounds that exist only so that traffic can be
counted separately by kind.

The last one is worth explaining. Every outbound added here is the same freedom
outbound doing the same thing; they differ only in name. Routing sends a
connection to one of them, and the only trace that survives is a number of
bytes against a class. There is no user in that number, because Xray counts
users and outbounds separately and offers no way to ask for the pair. The
question "which user watched video" has no answer here, and not because we
decline to answer it.

Run it twice and the second run changes nothing.
"""

import json
import shutil
import subprocess
import sys
from pathlib import Path

CONFIG = Path("/usr/local/etc/xray/config.json")
BACKUP = Path("/root/xray-config.before-observability.json")

# The classes the Business Owner asked to see. Names are short because they end
# up as counter names, and a counter name is not a place for prose.
CLASSES = [
    "video", "audio", "calls", "games",
    "p2p", "download", "web", "background", "other",
]

# What each class is recognised by.
#
# Written out here rather than pulled from a published category file, because
# that file would have to be fetched onto every node and kept current, and a
# stale one misclassifies quietly. These lists are short, ours, and honest
# about their reach: a lot of traffic will land in `web` and `other`. That is a
# limit of the method, stated rather than hidden.
DOMAINS = {
    "video": [
        "domain:youtube.com", "domain:googlevideo.com", "domain:ytimg.com",
        "domain:netflix.com", "domain:nflxvideo.net", "domain:twitch.tv",
        "domain:ttvnw.net", "domain:rutube.ru", "domain:vkvideo.ru",
        "domain:okko.tv", "domain:ivi.ru", "domain:kinopoisk.ru",
        "domain:primevideo.com", "domain:dssott.com", "domain:tiktokcdn.com",
        "domain:vimeo.com", "domain:dailymotion.com",
    ],
    "audio": [
        "domain:spotify.com", "domain:scdn.co", "domain:music.yandex.ru",
        "domain:soundcloud.com", "domain:sndcdn.com", "domain:deezer.com",
        "domain:music.apple.com",
    ],
    "calls": [
        "domain:zoom.us", "domain:whatsapp.net", "domain:telegram.org",
        "domain:t.me", "domain:discord.gg", "domain:discordapp.com",
        "domain:teams.microsoft.com", "domain:meet.google.com",
        "domain:skype.com", "domain:webex.com",
    ],
    "games": [
        "domain:steampowered.com", "domain:steamcontent.com",
        "domain:epicgames.com", "domain:riotgames.com",
        "domain:battle.net", "domain:blizzard.com",
        "domain:playstation.net", "domain:xboxlive.com",
        "domain:ea.com", "domain:ubisoft.com", "domain:roblox.com",
    ],
    "download": [
        "domain:windowsupdate.com", "domain:download.microsoft.com",
        "domain:mzstatic.com", "domain:githubusercontent.com",
        "domain:archive.org", "domain:sourceforge.net",
        "domain:ubuntu.com", "domain:debian.org",
    ],
    "background": [
        "domain:googleapis.com", "domain:gstatic.com",
        "domain:push.apple.com", "domain:icloud.com",
        "domain:mozilla.net", "domain:crashlytics.com",
        "domain:sentry.io", "domain:doubleclick.net",
    ],
}


def classification_rules(api_rules):
    """The rules, in the order they are asked.

    Order is the whole meaning of a routing table: the first match wins, so a
    broad rule placed early makes every rule after it dead. The catch-all is
    last for that reason, and the protocol rule is first because a torrent that
    also matches a port rule is still a torrent.
    """
    rules = list(api_rules)

    rules.append({
        "type": "field",
        "protocol": ["bittorrent"],
        "outboundTag": "class-p2p",
    })

    for name in ("video", "audio", "calls", "games", "download", "background"):
        rules.append({
            "type": "field",
            "domain": DOMAINS[name],
            "outboundTag": "class-" + name,
        })

    # Calls that announce no name: the standard rendezvous ports are the only
    # thing they have in common.
    rules.append({
        "type": "field",
        "network": "udp",
        "port": "3478,5349,19302-19309",
        "outboundTag": "class-calls",
    })

    rules.append({
        "type": "field",
        "network": "tcp,udp",
        "port": "80,443,8080,8443",
        "outboundTag": "class-web",
    })

    rules.append({
        "type": "field",
        "network": "tcp,udp",
        "outboundTag": "class-other",
    })

    return rules


def patch(config):
    changed = []

    # Counting has to be switched on in three places that do not know about one
    # another: the counters, the policy saying which counters to keep, and the
    # management service allowed to read them.
    if "stats" not in config:
        config["stats"] = {}
        changed.append("counters enabled")

    policy = config.setdefault("policy", {})
    level0 = policy.setdefault("levels", {}).setdefault("0", {})
    if not level0.get("statsUserUplink"):
        level0["statsUserUplink"] = True
        level0["statsUserDownlink"] = True
        changed.append("per-user totals enabled")

    system = policy.setdefault("system", {})
    for key in ("statsInboundUplink", "statsInboundDownlink",
                "statsOutboundUplink", "statsOutboundDownlink"):
        if not system.get(key):
            system[key] = True
            if "service totals enabled" not in changed:
                changed.append("service totals enabled")

    api = config.setdefault("api", {})
    services = api.setdefault("services", [])
    if "StatsService" not in services:
        services.append("StatsService")
        changed.append("counters readable on the management interface")

    # Sniffing reads the name out of the handshake so routing can tell one kind
    # of traffic from another. routeOnly keeps it at that: the connection still
    # goes where it was going, and the name is used for the decision and then
    # gone. Nothing writes it down, because the only thing downstream of the
    # decision is a byte counter.
    for inbound in config.get("inbounds", []):
        if inbound.get("tag") == "ws-in" and "sniffing" not in inbound:
            inbound["sniffing"] = {
                "enabled": True,
                "destOverride": ["http", "tls", "quic"],
                "routeOnly": True,
            }
            changed.append("traffic kind recognised for routing")

    outbounds = config.setdefault("outbounds", [])
    have = {o.get("tag") for o in outbounds}
    for name in CLASSES:
        if "class-" + name not in have:
            outbounds.append({"tag": "class-" + name, "protocol": "freedom"})
            if "class outbounds added" not in changed:
                changed.append("class outbounds added")

    routing = config.setdefault("routing", {})
    rules = routing.get("rules", [])
    api_rules = [r for r in rules if r.get("outboundTag") == "api"]
    if not api_rules:
        print("The management interface has no routing rule. Refusing to touch this.")
        sys.exit(1)

    wanted = classification_rules(api_rules)
    if rules != wanted:
        routing["rules"] = wanted
        if "classification rules written" not in changed:
            changed.append("classification rules written")

    return changed


def refuse_if_logging(config):
    """The one thing this script must never let through.

    It rewrites the configuration file that decides whether every destination a
    person visits is written to the journal. Raising the log level turns this
    node into a browsing history recorder, and it is one word. Checked here
    because this is the last place that word passes through before it is
    installed.
    """
    log = config.setdefault("log", {})
    if log.get("access") not in (None, "", "none"):
        print("This configuration writes an access log. Refusing.")
        sys.exit(1)
    if log.get("loglevel") in ("debug", "info"):
        print("This configuration logs every destination. Refusing.")
        sys.exit(1)
    log["access"] = "none"
    log.setdefault("loglevel", "warning")
    log["dnsLog"] = False


def main():
    config = json.loads(CONFIG.read_text())
    refuse_if_logging(config)
    changed = patch(config)

    if not changed:
        print("Already counting. Nothing to change.")
        return

    candidate = Path("/tmp/xray-observability-candidate.json")
    candidate.write_text(json.dumps(config, indent=2))

    # Tested before it is installed, not after. A configuration Xray refuses is
    # a node that does not come back, and it does not come back on its own.
    test = subprocess.run(
        ["/usr/local/bin/xray", "run", "-test", "-c", str(candidate)],
        capture_output=True, text=True,
    )
    if test.returncode != 0:
        candidate.unlink(missing_ok=True)
        print("Xray refuses the new configuration. Nothing changed.")
        print((test.stdout or test.stderr).strip()[-500:])
        sys.exit(1)

    if not BACKUP.exists():
        shutil.copy2(CONFIG, BACKUP)
    shutil.copy2(candidate, CONFIG)
    candidate.unlink(missing_ok=True)

    for line in changed:
        print("  " + line)
    print("Restarting Xray. Connected devices reconnect on their own.")
    subprocess.run(["systemctl", "restart", "xray"], check=True)


if __name__ == "__main__":
    main()
