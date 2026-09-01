#!/usr/bin/env python3

"""Repeat an exact YooKassa test refund POST within its idempotency window."""

import argparse
import base64
import datetime as dt
import json
import os
import pathlib
import sys
import urllib.error
import urllib.parse
import urllib.request

import payment_webhook_replay as durable


API = "https://api.yookassa.ru/v3"
WINDOW = dt.timedelta(hours=24)


class RetryError(RuntimeError):
    pass


def load_snapshot(path):
    try:
        return json.loads(pathlib.Path(path).read_text(encoding="utf-8"))
    except (OSError, ValueError) as error:
        raise RetryError("private snapshot could not be read") from error


def parse_time(value):
    if not value:
        raise RetryError("refund attempt time is absent")
    try:
        parsed = dt.datetime.fromisoformat(str(value).replace("Z", "+00:00"))
    except ValueError as error:
        raise RetryError("refund attempt time is invalid") from error
    if parsed.tzinfo is None:
        raise RetryError("refund attempt time has no timezone")
    return parsed.astimezone(dt.timezone.utc)


def validate_target(snapshot, now=None):
    base = durable.validate_target(snapshot)
    payment = snapshot["payment"]
    refund = snapshot["refund"]
    attempt = refund["attempt"]
    current = (now or dt.datetime.now(dt.timezone.utc)).astimezone(dt.timezone.utc)
    created = parse_time(attempt.get("created_at"))
    age = current - created
    if age < dt.timedelta(0) or age >= WINDOW:
        raise RetryError("refund attempt is outside YooKassa's 24-hour idempotency window")
    key = str(attempt.get("idempotency_key") or "").strip()
    if not key:
        raise RetryError("refund idempotency key is absent")
    return {
        **base,
        "internal_payment_id": str(payment["id"]),
        "internal_refund_id": str(refund["id"]),
        "provider_payment_id": str(payment["provider_payment_id"]),
        "provider_refund_id": str(attempt["provider_refund_id"]),
        "idempotency_key": key,
        "amount_minor": int(refund["amount_minor"]),
        "currency": str(refund["currency"]),
    }


def ruble_value(minor):
    return f"{minor // 100}.{minor % 100:02d}"


def request_json(method, url, headers, body=None):
    data = None if body is None else json.dumps(body, separators=(",", ":")).encode("utf-8")
    request = urllib.request.Request(url, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(request, timeout=20) as response:
            status = response.status
            payload = json.loads(response.read().decode("utf-8"))
    except urllib.error.HTTPError as error:
        raise RetryError(f"YooKassa retry returned HTTP {error.code}") from None
    except (OSError, ValueError):
        raise RetryError("YooKassa retry response could not be read") from None
    return status, payload


def verify_refund(target, raw):
    amount = raw.get("amount") or {}
    metadata = raw.get("metadata") or {}
    expected_amount = ruble_value(target["amount_minor"])
    checks = [
        raw.get("id") == target["provider_refund_id"],
        raw.get("payment_id") == target["provider_payment_id"],
        metadata.get("refund_id") == target["internal_refund_id"],
        amount.get("value") == expected_amount,
        amount.get("currency") == target["currency"],
        raw.get("status") == "succeeded",
    ]
    if not all(checks):
        raise RetryError("provider returned a different refund object")


def retry(snapshot, shop_id, secret_key, requester=request_json, now=None):
    target = validate_target(snapshot, now=now)
    if not shop_id or not secret_key:
        raise RetryError("YooKassa test credentials are unavailable")
    authorization = base64.b64encode(f"{shop_id}:{secret_key}".encode("utf-8")).decode("ascii")
    common = {"Accept": "application/json", "Authorization": f"Basic {authorization}"}
    payload = {
        "amount": {"value": ruble_value(target["amount_minor"]), "currency": target["currency"]},
        "payment_id": target["provider_payment_id"],
        "description": "Возврат VIP",
        "metadata": {"refund_id": target["internal_refund_id"]},
    }
    status, created = requester(
        "POST",
        f"{API}/refunds",
        {**common, "Content-Type": "application/json", "Idempotence-Key": target["idempotency_key"]},
        payload,
    )
    if status != 200:
        raise RetryError(f"YooKassa retry returned HTTP {status}")
    verify_refund(target, created)

    matches = []
    cursor = ""
    for _ in range(100):
        query = {"payment_id": target["provider_payment_id"], "limit": "100"}
        if cursor:
            query["cursor"] = cursor
        list_status, page = requester(
            "GET", f"{API}/refunds?{urllib.parse.urlencode(query)}", common, None
        )
        if list_status != 200 or not isinstance(page.get("items"), list):
            raise RetryError("YooKassa refund list could not be verified")
        for item in page["items"]:
            if (item.get("metadata") or {}).get("refund_id") == target["internal_refund_id"]:
                matches.append(item)
        cursor = str(page.get("next_cursor") or "").strip()
        if not cursor:
            break
    else:
        raise RetryError("YooKassa refund list did not terminate")

    if len(matches) != 1:
        raise RetryError("provider list does not contain exactly one logical refund")
    verify_refund(target, matches[0])
    return "\n".join(
        [
            "## YooKassa lost-response retry",
            f"- payment `{target['payment_prefix']}`; refund `{target['refund_prefix']}`",
            "- repeated POST: HTTP 200; provider refund: тот же",
            "- provider list: 1 operation with the internal refund metadata",
            "**Verdict: IDEMPOTENT — повтор с тем же ключом не создал второй возврат.**",
        ]
    )


def main(argv=None):
    parser = argparse.ArgumentParser()
    parser.add_argument("snapshot")
    args = parser.parse_args(argv)
    try:
        report = retry(
            load_snapshot(args.snapshot),
            os.environ.get("YOOKASSA_SHOP_ID", ""),
            os.environ.get("YOOKASSA_SECRET_KEY", ""),
        )
        print(report)
    except (RetryError, durable.ReplayError) as error:
        print(f"Refund retry refused: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
