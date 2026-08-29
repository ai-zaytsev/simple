"""What accounts there are, as a table, with nothing about who they belong to.

A file rather than a heredoc inside the workflow. The embedded version broke
the YAML on its first run: a program inside a block scalar has to be indented
to stay inside it, and Python at column zero read as new top-level keys. The
workflow then did not parse at all, which GitHub reports as "no trigger" -
a sentence about a missing line rather than a broken file.

read_panel.py is next door for the same reason.
"""

import json
import sys


def main(path):
    with open(path, encoding="utf-8") as handle:
        accounts = json.load(handle).get("accounts", [])

    print("## Аккаунты")
    print()
    print("Адресов здесь нет. Префикс — начало идентификатора, которого")
    print("достаточно, чтобы назвать аккаунт, и мало, чтобы что-то о нём узнать.")
    print()
    print("| префикс | тариф | устройств | заведён |")
    print("| --- | --- | --- | --- |")
    for account in accounts:
        print("| %s… | %s | %s | %s |" % (
            account.get("prefix"), account.get("tier"),
            account.get("devices"), account.get("created")))
    print()
    print("Всего: %d." % len(accounts))


if __name__ == "__main__":
    main(sys.argv[1])
