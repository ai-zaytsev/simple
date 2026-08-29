"""Print the panel in a form that can be read in a public log.

The repository is public, so this prints numbers and never names. Servers and
domains become positions in a sorted list, which is enough to say "the second
domain is carrying everything" and nothing at all to somebody who would like
to know what our covers are called.

Everything printed here comes from the same overview the panel draws, so a
disagreement between this and the screen is a defect rather than a difference
of method.
"""

import json
import sys


def bar(fraction, width=24):
    """A fraction as a line, because a column of percentages does not read."""
    filled = max(0, min(width, round(fraction * width)))
    return "#" * filled + "." * (width - filled)


def pct(value):
    return "—" if value is None else "%.0f%%" % (value * 100)


def rate(value):
    """A percentage the service reports, where a negative means "not measured".

    Written out rather than folded into an `or` because that is what went
    wrong the first time this ran: `value or -1` turns a genuine zero into the
    fallback, so a domain nothing could reach was printed as a domain nobody
    had checked. A failure shown as an absence is worse than no panel: it does
    not look like a problem.
    """
    if value is None or value < 0:
        return "—"
    return "%.0f%%" % value


def number(value, scale=1.0, digits=1):
    """A number that may legitimately be zero."""
    if value is None:
        return "—"
    return ("%." + str(digits) + "f") % (value / scale)


def limit(value, unit=""):
    """A ceiling, where absent means unlimited and zero means none.

    Written out in words rather than left as a dash. Elsewhere in this file a
    dash means "not measured", and a tier having no limit is the opposite of
    an unknown - it is the policy. Zero is a third answer again: no external
    devices is what FREE is actually allowed, and reading it as unlimited
    would invert the whole point of the tiers.
    """
    if value is None:
        return "без предела"
    if value == 0:
        return "нет"
    return "%s%s" % (value, unit)


def main(path):
    with open(path, encoding="utf-8") as handle:
        d = json.load(handle)

    out = []
    say = out.append

    say("## Панель после выката")
    say("")
    say("Снято: `%s`" % d.get("generated_at", "?"))
    say("")

    c = d.get("capacity") or {}
    say("### Мощность")
    say("")
    say("**%s**" % (c.get("state") or "—"))
    for reason in c.get("reasons") or []:
        say("- %s" % reason)
    say("")
    say("| | |")
    say("| --- | --- |")
    say("| занято сейчас | %s (%s из %s) |" % (
        pct(c.get("utilisation")), c.get("sessions_now"), c.get("capacity_total")))
    say("| свободно | %s соединений |" % c.get("spare_room"))
    say("| пик за сутки | %s |" % c.get("peak_today"))
    say("| пик за неделю | %s (%s мощности) |" % (
        c.get("peak_week"), pct(c.get("peak_used"))))
    say("| пики дня и вечера | %s / %s |" % (c.get("peak_day"), c.get("peak_evening")))
    say("| P95 | %s |" % pct(c.get("p95_utilisation")))
    growth = c.get("growth_week_on_week")
    say("| рост | %s |" % ("истории не хватает" if growth is None
                           else "%+.0f%%" % (growth * 100)))
    say("| серверов в работе | %s |" % c.get("nodes_usable"))
    say("| серверов в запасе | %s |" % c.get("nodes_spare"))
    say("| заблокировано / неисправно | %s / %s |" % (
        c.get("nodes_blocked"), c.get("nodes_faulty")))
    say("| доменов в запасе | %s |" % c.get("domains_spare"))
    say("")

    # A peak over ten hours and a peak over seven days sit in the same box and
    # only one of them means what the label says.
    hours = c.get("history_hours")
    if hours is not None:
        if hours < 24:
            span = "%.0f ч" % hours
        else:
            span = "%.1f сут" % (hours / 24.0)
        say("Пики и P95 посчитаны по %s истории." % span)
        if hours < 7 * 24:
            say("Это меньше недели, так что «за неделю» здесь значит «за всё, что есть».")
        say("")

    hours = c.get("by_hour") or []
    if any(hours):
        top = max(hours)
        say("### Сутки по часам, МСК")
        say("")
        say("```")
        for hour, value in enumerate(hours):
            say("%02d  %-24s %d" % (hour, bar(value / top if top else 0), value))
        say("```")
        say("")

    groups = c.get("groups") or []
    if groups:
        say("### По группам серверов")
        say("")
        say("| группа | серверов | в запасе | сессий | мощность | занято |")
        say("| --- | --- | --- | --- | --- | --- |")
        for g in groups:
            say("| %s | %s | %s | %s | %s | %s |" % (
                g.get("group"), g.get("nodes"), g.get("spare"),
                g.get("sessions"), g.get("capacity"), pct(g.get("utilisation"))))
        say("")

    domains = c.get("domains") or []
    if domains:
        say("### По доменам")
        say("")
        say("Имена заменены позициями: журнал публичный.")
        say("")
        say("| домен | сессий | доля | вывод |")
        say("| --- | --- | --- | --- |")
        for i, x in enumerate(domains, 1):
            say("| №%d | %s | %s | %s |" % (
                i, x.get("sessions"), pct(x.get("share")), x.get("verdict") or "—"))
        say("")

    now = d.get("now") or {}
    say("### Сейчас")
    say("")
    say("| | |")
    say("| --- | --- |")
    say("| активны за час | %s |" % now.get("active_users_hour"))
    say("| активны за сутки | %s |" % now.get("active_users_day"))
    say("| соединений | %s |" % now.get("sessions_online"))
    say("| серверов на связи | %s из %s |" % (
        now.get("nodes_reporting"), len(d.get("nodes") or [])))
    say("| отдача / приём | %s / %s Мбит/с |" % (
        number(now.get("downlink_bps"), 1e6), number(now.get("uplink_bps"), 1e6)))

    # Connections, not people. One phone holds tens of them, so the two
    # numbers above are not the same quantity and the ratio between them is
    # what turns a capacity figure in connections into one in users.
    users = now.get("active_users_hour") or 0
    sessions = now.get("sessions_online") or 0
    if users > 0:
        say("| соединений на пользователя | %s |" % number(sessions / float(users), 1.0))
    say("")

    nodes = d.get("nodes") or []
    if nodes:
        say("### Серверы")
        say("")
        # Aliases are printed, unlike domains. An alias is opaque by design -
        # not an index, not a hostname, not a region - so it says nothing to a
        # stranger, and the fleet size it might hint at is already in the table
        # above. Withholding it only made the panel unusable for acting on: a
        # workflow that updates one node asks for a name, and there was nowhere
        # to read one.

        # The domain verdict is why a node is or is not offered, and it was
        # the one column missing when two healthy nodes stopped being handed
        # out. Everything on the row said the machine was well; nothing said
        # what devices were finding when they tried to reach it.
        say("| сервер | состояние | выдаётся | домен | запас | сессий | CPU | память | задержка | потери |")
        say("| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |")
        for n in sorted(nodes, key=lambda x: x.get("alias") or ""):
            say("| %s | %s | %s | %s | %s | %s | %s | %s | %s | %s |" % (
                n.get("alias"), n.get("verdict"), "да" if n.get("offered") else "нет",
                n.get("domain_verdict") or "не измерен",
                pct(n.get("room")), n.get("sessions_online"),
                rate(n.get("cpu_percent")),
                rate(n.get("memory_percent")),
                "—" if n.get("latency_ms") is None else "%.0f мс" % n["latency_ms"],
                rate(n.get("loss_percent"))))
        say("")

    # Lifecycles carry names, so only the counts and the four answers survive.
    life = d.get("lifecycles") or []
    if life:
        say("### Жизненный цикл")
        say("")
        say("| что | всего | выдавать | снять | заменить | удалять |")
        say("| --- | --- | --- | --- | --- | --- |")
        for kind, label in (("server", "серверы"), ("domain", "домены")):
            rows = [s for s in life if s.get("kind") == kind]
            if not rows:
                continue
            say("| %s | %d | %d | %d | %d | %d |" % (
                label, len(rows),
                sum(1 for s in rows if s.get("may_hand_out")),
                sum(1 for s in rows if s.get("stop_handing_out")),
                sum(1 for s in rows if s.get("needs_replacing")),
                sum(1 for s in rows if s.get("may_delete"))))
        say("")
        states = {}
        for s in life:
            states[s.get("state")] = states.get(s.get("state"), 0) + 1
        say("Состояния: " + ", ".join(
            "%s×%d" % (k, v) for k, v in sorted(states.items())))
        say("")

    entries = d.get("endpoints") or []
    if entries:
        # What was decided about a domain, next to what is measured about it.
        # Separately the two are readable and say nothing: a domain nothing can
        # reach is only a problem if it is still being handed out, and that is
        # the pair somebody has to see at once.
        decided = {
            s.get("name"): s for s in life if s.get("kind") == "domain"
        }

        # What an address is for, without saying what it is called. A cover in
        # front of a node and the way in to the Control Plane fail differently
        # and are replaced differently, and a row that does not say which is a
        # row nobody can act on.
        covers = {x.get("domain"): x for x in (c.get("domains") or [])}

        say("### Точки входа")
        say("")
        say("| адрес | что это | вывод | отсюда | с устройств | проверок | выдаётся | заменить |")
        say("| --- | --- | --- | --- | --- | --- | --- | --- |")
        for i, p in enumerate(sorted(entries, key=lambda x: x.get("target") or ""), 1):
            standing = decided.get(p.get("target")) or {}
            cover = covers.get(p.get("target"))
            role = "прикрытие ноды" if cover else "точка входа"
            if cover:
                role += " (%s сессий)" % cover.get("sessions")
            say("| №%d | %s | %s | %s | %s | %s | %s | %s |" % (
                i, role, p.get("verdict"),
                rate(p.get("ok_from_us")),
                rate(p.get("ok_from_devices")),
                p.get("device_checks"),
                "да" if standing.get("may_hand_out") else "нет",
                "да" if standing.get("needs_replacing") else "—"))
        say("")

    u = d.get("usage") or {}
    say("### Нагрузка по пользователям за месяц")
    say("")
    say("Пользователей: %s. Всего: %.1f ГБ." % (
        u.get("users"), (u.get("total_bytes") or 0) / 1e9))
    say("")

    # What the tiers actually allow, read from the rows rather than believed
    # from a migration. A limit that was supposed to be lifted and was not
    # looks identical from every other angle: the migration is logged as
    # applied, the tests pass against its own text, and the service starts.
    tiers = d.get("tiers") or []
    if tiers:
        say("### Что даёт тариф")
        say("")
        say("| тариф | аккаунтов | устройств | внешних | скорость |")
        say("| --- | --- | --- | --- | --- |")
        for t in tiers:
            say("| %s | %s | %s | %s | %s |" % (
                t.get("tier"),
                t.get("accounts"),
                limit(t.get("max_devices")),
                limit(t.get("max_external")),
                limit(t.get("speed_mbit"), " Мбит/с")))
        say("")

    print("\n".join(out))


if __name__ == "__main__":
    main(sys.argv[1])
