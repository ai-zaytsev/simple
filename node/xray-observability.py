#!/usr/bin/env python3
"""Teaches this node's Xray to count, without teaching it to remember.

Adds counters, a management service allowed to read them, and a set of
outbounds that exist only so traffic can be counted separately by kind.

Every outbound here is the same freedom outbound doing the same thing; they
differ only in name. Routing sends a connection to one of them, and the only
trace that survives is a number of bytes against a class. There is no user in
that number, because Xray counts users and outbounds separately and offers no
way to ask for the pair. The question "which user watched video" has no answer
here, and not because we decline to answer it.

There is a second set of the same outbounds, prefixed `heavy-`, used for a
list of credentials the Control Plane names. That is how the two groups can be
compared without anybody being described: the node is handed a list and told
nothing about what it means, and what it reports is two sets of totals.

Run twice and the second run changes nothing.
"""

import json
import shutil
import subprocess
import sys
from pathlib import Path

CONFIG = Path("/usr/local/etc/xray/config.json")
BACKUP = Path("/root/xray-config.before-observability.json")

# Where the list of heavier credentials is kept between runs, so that a change
# can be recognised as a change. It holds credential identifiers and nothing
# else; the node never learns why they are on it.
HEAVY_LIST = Path("/etc/simple-vpn-heavy.json")

# The credentials held to a speed, and the figure they are held to. Kept the
# same way and for the same reason: the node is handed a list and a number and
# is told nothing about what either means.
LIMITED_LIST = Path("/etc/simple-vpn-limited.json")

# The firewall mark put on connections that are to be shaped. Traffic carrying
# it is what the shaper slows; everything else, including this machine's own
# administration, is untouched by construction.
SLOW_MARK = 0x51

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
# about their reach: a lot of traffic still lands in `web` and `other`. That is
# a limit of the method, stated rather than hidden.
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


def matchers():
    """Every rule this node classifies by, in the order it asks them.

    Order is the whole meaning of a routing table: the first match wins, so a
    broad rule placed early makes every rule after it dead. The protocol rule
    is first because a torrent that also matches a port rule is still a
    torrent, and the catch-all is last for the same reason in reverse.

    Returned as (matcher, class) pairs so that each one can be emitted twice -
    once for the heavier group, once for everybody else - without the order
    being written down in two places and drifting apart.
    """
    rules = []

    # Recognised by what it says rather than where it goes. Proven on a live
    # node with a real handshake: the twenty kilobytes landed in p2p and
    # nowhere else.
    rules.append(({"protocol": ["bittorrent"]}, "p2p"))

    for name in ("video", "audio", "calls", "games", "download", "background"):
        rules.append(({"domain": DOMAINS[name]}, name))

    # The classic BitTorrent range, as a backstop for what the sniffer cannot
    # see. Xray recognises the TCP handshake; a torrent running over uTP is
    # UDP and goes unrecognised, and this stage requires P2P to be separated
    # rather than mostly separated.
    rules.append(({"network": "tcp,udp", "port": "6881-6999"}, "p2p"))

    # Calls that announce no name: the standard rendezvous ports are the only
    # thing they have in common.
    rules.append(({"network": "udp", "port": "3478,5349,19302-19309"}, "calls"))

    # Game traffic that announces no name either. These are the ports the
    # large platforms publish for their own clients.
    rules.append((
        {"network": "udp", "port": "3074,3658-3659,6672,9305-9308,10070-10080,27000-27100"},
        "games",
    ))

    rules.append(({"network": "tcp,udp", "port": "80,443,8080,8443"}, "web"))
    rules.append(({"network": "tcp,udp"}, "other"))
    return rules


def classification_rules(api_rules, heavy, limited):
    """The routing table, split by two questions that are asked separately.

    A connection has to answer both: which kind of traffic it is, which decides
    the counter, and whether the person is held to a speed, which decides
    whether the socket carries the shaping mark. Xray picks one outbound, so
    the two answers have to meet in the outbound tag.

    Order is everything here. The first match wins, so the narrowest rules come
    first: slowed and heavy, then slowed, then heavy, then everybody else. Put
    the other way round, the broad rule would answer first and the narrow ones
    would never be reached - which is the failure that leaves a limit written
    down and not applied.
    """
    rules = list(api_rules)

    for matcher, name in matchers():
        # Both, then one, then the other, then neither. Written as a list so
        # the order is visible in one place rather than in four blocks that
        # can be edited apart.
        for users, tag in (
            (both(heavy, limited), "slow-heavy-" + name),
            (limited, "slow-class-" + name),
            (heavy, "heavy-" + name),
            (None, "class-" + name),
        ):
            if users is not None and not users:
                continue
            rule = dict(matcher)
            rule["type"] = "field"
            if users:
                rule["user"] = users
            rule["outboundTag"] = tag
            rules.append(rule)

    return rules


def both(heavy, limited):
    """The credentials on both lists, in a stable order."""
    return sorted(set(heavy) & set(limited))


def read_list(path):
    """A list as it was last applied, or empty if it never has been."""
    try:
        return sorted(json.loads(path.read_text()))
    except Exception:  # noqa: BLE001 - a missing or broken list means none
        return []


def write_list(path, values):
    path.write_text(json.dumps(sorted(values)))
    path.chmod(0o600)


def read_heavy():
    return read_list(HEAVY_LIST)


def write_heavy(heavy):
    write_list(HEAVY_LIST, heavy)


def read_limited():
    return read_list(LIMITED_LIST)


def write_limited(limited):
    write_list(LIMITED_LIST, limited)


def patch(config, heavy, limited):
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
        for tag in ("class-" + name, "heavy-" + name):
            if tag not in have:
                outbounds.append({"tag": tag, "protocol": "freedom"})
                if "class outbounds added" not in changed:
                    changed.append("class outbounds added")

        # The same outbounds again, differing in one field: a mark on the
        # socket. The mark is the only thing the shaper can see - it works
        # below Xray, on the kernel's queues, where there are no users and no
        # connections, only packets - so a speed limit has to be written into
        # the socket at the moment it is opened or it cannot be applied at all.
        for tag in ("slow-class-" + name, "slow-heavy-" + name):
            if tag not in have:
                outbounds.append({
                    "tag": tag,
                    "protocol": "freedom",
                    "streamSettings": {"sockopt": {"mark": SLOW_MARK}},
                })
                if "shaped outbounds added" not in changed:
                    changed.append("shaped outbounds added")

    routing = config.setdefault("routing", {})
    rules = routing.get("rules", [])
    api_rules = [r for r in rules if r.get("outboundTag") == "api"]
    if not api_rules:
        print("The management interface has no routing rule. Refusing to touch this.")
        sys.exit(1)

    wanted = classification_rules(api_rules, heavy, limited)
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


def apply(heavy=None, limited=None, restart=True):
    """Writes the configuration and, if anything changed, restarts the engine.

    Returns True when the engine was restarted.

    The restart is not avoidable. Xray reads its routing table once, at start,
    and `xray api adrules` - the documented way to replace it on a running
    engine - crashes in this version: tested on a live pair, the call panicked
    and the rules were not replaced. So a change of the heavy list costs a
    reconnect, and the caller chooses the moment.
    """
    if heavy is None:
        heavy = read_heavy()
    if limited is None:
        limited = read_limited()

    config = json.loads(CONFIG.read_text())
    refuse_if_logging(config)
    changed = patch(config, heavy, limited)

    if not changed:
        write_heavy(heavy)
        write_limited(limited)
        return False

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
        return False

    if not BACKUP.exists():
        shutil.copy2(CONFIG, BACKUP)
    shutil.copy2(candidate, CONFIG)
    candidate.unlink(missing_ok=True)
    write_heavy(heavy)
    write_limited(limited)

    for line in changed:
        print("  " + line)

    if not restart:
        return False
    print("Restarting Xray. Connected devices reconnect on their own.")
    subprocess.run(["systemctl", "restart", "xray"], check=True)
    return True


def main():
    if not apply():
        print("Already counting. Nothing to change.")


if __name__ == "__main__":
    main()
