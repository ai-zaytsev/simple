"""What an account is on now, after being changed.

The three answers a limit can give are kept apart here as they are on the
panel: absent is no limit, zero is none, and a number is a number. An operator
reading this has just granted something and is checking what it granted; a
zero printed as "unlimited" would be the worst possible moment for that.
"""

import json
import sys


def limit(value, unit=""):
    if value is None:
        return "без предела"
    if value == 0:
        return "нет"
    return "%s%s" % (value, unit)


def main(path):
    with open(path, encoding="utf-8") as handle:
        answer = json.load(handle)

    print("## Тариф назначен")
    print()
    print("| | |")
    print("| --- | --- |")
    print("| аккаунт | %s… |" % answer.get("prefix"))
    print("| тариф | %s |" % answer.get("tier"))
    print("| устройств приложения | %s |" % limit(answer.get("max_devices")))
    print("| внешних устройств | %s |" % limit(answer.get("max_external")))
    print("| устройств сейчас | %s |" % answer.get("devices"))


if __name__ == "__main__":
    main(sys.argv[1])
