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
    say("| отдача / приём | %.1f / %.1f Мбит/с |" % (
        (now.get("downlink_bps") or 0) / 1e6, (now.get("uplink_bps") or 0) / 1e6))
    say("")

    nodes = d.get("nodes") or []
    if nodes:
        say("### Серверы")
        say("")
        say("| сервер | состояние | выдаётся | запас | сессий | CPU | память | задержка | потери |")
        say("| --- | --- | --- | --- | --- | --- | --- | --- | --- |")
        for i, n in enumerate(sorted(nodes, key=lambda x: x.get("alias") or ""), 1):
            say("| №%d | %s | %s | %s | %s | %s | %s | %s | %s |" % (
                i, n.get("verdict"), "да" if n.get("offered") else "нет",
                pct(n.get("room")), n.get("sessions_online"),
                pct((n.get("cpu_percent") or 0) / 100),
                pct((n.get("memory_percent") or 0) / 100),
                "—" if n.get("latency_ms") is None else "%.0f мс" % n["latency_ms"],
                pct((n.get("loss_percent") or 0) / 100)))
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
        say("### Точки входа")
        say("")
        say("| адрес | вывод | отсюда | с устройств | проверок |")
        say("| --- | --- | --- | --- | --- |")
        for i, p in enumerate(sorted(entries, key=lambda x: x.get("target") or ""), 1):
            say("| №%d | %s | %s | %s | %s |" % (
                i, p.get("verdict"),
                "—" if (p.get("ok_from_us") or -1) < 0 else "%.0f%%" % p["ok_from_us"],
                "—" if (p.get("ok_from_devices") or -1) < 0 else "%.0f%%" % p["ok_from_devices"],
                p.get("device_checks")))
        say("")

    u = d.get("usage") or {}
    say("### Нагрузка по пользователям за месяц")
    say("")
    say("Пользователей: %s. Всего: %.1f ГБ." % (
        u.get("users"), (u.get("total_bytes") or 0) / 1e9))
    say("")

    print("\n".join(out))


if __name__ == "__main__":
    main(sys.argv[1])
