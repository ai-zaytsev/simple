"""Whether VIP may be bought, and how long a new account waits.

A file rather than a heredoc in the workflow: a program inside a block scalar
has to be indented to stay inside it, and the last one written the other way
stopped the workflow parsing at all. read_panel.py is next door for the same
reason.
"""

import json
import sys


def main(path):
    with open(path, encoding="utf-8") as handle:
        settings = json.load(handle)

    open_now = settings.get("open")

    print("## Продажа VIP")
    print()
    print("| | |")
    print("| --- | --- |")
    print("| продажи | %s |" % ("открыты" if open_now else "**закрыты**"))
    print("| FREE до покупки | %s дн. |" % settings.get("free_days"))
    print()

    # Said every time, not only when it is off. The question "does this touch
    # people who already paid" is asked by whoever presses the switch, and the
    # answer should be on the same page as the switch.
    print("Тех, у кого VIP уже есть, это не касается: их статус решается")
    print("раньше, чем читается этот переключатель.")

    if not open_now:
        print()
        print("Сейчас купить VIP нельзя никому.")


if __name__ == "__main__":
    main(sys.argv[1])
