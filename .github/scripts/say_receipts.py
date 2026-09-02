#!/usr/bin/env python3

"""Say what the tax module owes, from the panel's own answer.

Its own file rather than a here-document inside the workflow, for the reason
the checkout comment in that file gives: a step that prints has to be a thing
that exists, and printing is the last step, so a mistake in it fails a run
whose change has already been applied.
"""

import json
import sys


def main(path):
    with open(path, encoding="utf-8") as handle:
        tax = (json.load(handle) or {}).get("tax") or {}

    lines = [
        "## Чеки НПД",
        "",
        "| | |",
        "| --- | --- |",
        "| чеки выдаются | %s |" % ("да" if tax.get("ok") else "**нет**"),
        "| платежей без чека | %s |" % (tax.get("pending") or 0),
    ]
    if tax.get("detail"):
        lines.append("| последняя проверка | %s |" % tax["detail"])
    if tax.get("checked_at"):
        lines.append("| проверено | %s |" % tax["checked_at"])
    lines.append("")

    if tax.get("pending"):
        lines.append(
            "Пока очередь не пуста, продажи не откроются даже при живой ФНС: "
            "продавать дальше, не закрыв уже принятые платежи, значит копить долг."
        )
        lines.append("")

    print("\n".join(lines))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1] if len(sys.argv) > 1 else "/tmp/overview.json"))
