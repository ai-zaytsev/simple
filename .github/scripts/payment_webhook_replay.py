#!/usr/bin/env python3

"""Replay completed test-store notifications without exposing provider IDs."""

import argparse
import json
import pathlib
import sys
import urllib.error
import urllib.request


WEBHOOK_URL = "https://simple-syncbridge.download/v1/payments/webhooks/yookassa"


class ReplayError(RuntimeError):
    pass


def load_snapshot(path):
    try:
        return json.loads(pathlib.Path(path).read_text(encoding="utf-8"))
    except (OSError, ValueError) as error:
        raise ReplayError("private snapshot could not be read") from error


def _required(value, message):
    if value is None or value == "":
        raise ReplayError(message)
    return value


def validate_target(snapshot):
    if snapshot.get("match_count") != 1:
        raise ReplayError("account prefix is missing or ambiguous")

    account = snapshot.get("account") or {}
    payment = snapshot.get("payment") or {}
    refund = snapshot.get("refund") or {}
    attempt = refund.get("attempt") or {}

    if account.get("tier") != "FREE" or account.get("vip_expires_at") is not None:
        raise ReplayError("completed refund did not leave the account FREE")
    if payment.get("provider") != "yookassa" or payment.get("provider_test") is not True:
        raise ReplayError("only a YooKassa test-store payment may be replayed")
    if payment.get("status") != "succeeded" or not payment.get("entitlement_applied_at"):
        raise ReplayError("payment is not canonically completed")
    if refund.get("provider") != "yookassa" or refund.get("status") != "succeeded":
        raise ReplayError("refund is not canonically completed")
    if refund.get("payment_id") != payment.get("id"):
        raise ReplayError("refund does not belong to the selected payment")
    if not refund.get("succeeded_at") or not refund.get("entitlement_revoked_at"):
        raise ReplayError("refund success did not revoke the entitlement")
    if attempt.get("status") != "succeeded":
        raise ReplayError("latest provider refund attempt is not succeeded")

    if snapshot.get("refund_count") != 1 or snapshot.get("succeeded_refund_count") != 1:
        raise ReplayError("selected payment must have exactly one completed refund")
    if snapshot.get("refund_attempt_count") != 1:
        raise ReplayError("selected refund must have exactly one provider attempt")
    if snapshot.get("succeeded_refund_total_minor") != refund.get("amount_minor"):
        raise ReplayError("stored refund aggregate does not match the completed refund")
    if not isinstance(payment.get("amount_minor"), int) or not isinstance(refund.get("amount_minor"), int):
        raise ReplayError("payment or refund amount is invalid")
    if refund["amount_minor"] <= 0 or refund["amount_minor"] > payment["amount_minor"]:
        raise ReplayError("refund amount is outside the original payment")

    return {
        "payment_id": _required(payment.get("provider_payment_id"), "provider payment ID is absent"),
        "refund_id": _required(attempt.get("provider_refund_id"), "provider refund ID is absent"),
        "payment_prefix": str(payment.get("id", ""))[:8],
        "refund_prefix": str(refund.get("id", ""))[:8],
    }


def post_notification(url, event, object_id):
    body = json.dumps(
        {"type": "notification", "event": event, "object": {"id": object_id}},
        separators=(",", ":"),
    ).encode("utf-8")
    request = urllib.request.Request(
        url,
        data=body,
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    try:
        with urllib.request.urlopen(request, timeout=20) as response:
            status = response.status
            payload = json.loads(response.read().decode("utf-8"))
    except urllib.error.HTTPError as error:
        raise ReplayError(f"{event} replay returned HTTP {error.code}") from None
    except (OSError, ValueError):
        raise ReplayError(f"{event} replay could not be completed") from None
    if status != 200 or payload.get("received") is not True:
        raise ReplayError(f"{event} replay was not acknowledged")
    if payload.get("applied") is not False:
        raise ReplayError(f"{event} replay attempted to apply the entitlement again")
    return {"status": status, "received": True, "applied": False}


def replay(snapshot, sender=post_notification, url=WEBHOOK_URL):
    target = validate_target(snapshot)
    results = []
    for event, object_id in (
        ("payment.succeeded", target["payment_id"]),
        ("payment.succeeded", target["payment_id"]),
        ("refund.succeeded", target["refund_id"]),
        ("refund.succeeded", target["refund_id"]),
    ):
        result = sender(url, event, object_id)
        if (
            result.get("status") != 200
            or result.get("received") is not True
            or result.get("applied") is not False
        ):
            raise ReplayError(f"{event} replay was not idempotently acknowledged")
        results.append((event, result))

    lines = [
        "## YooKassa webhook replay",
        f"- payment `{target['payment_prefix']}`; refund `{target['refund_prefix']}`",
    ]
    seen = {}
    for event, result in results:
        seen[event] = seen.get(event, 0) + 1
        lines.append(
            f"- {event} repeat {seen[event]}: HTTP {result['status']}, "
            "received да, applied нет"
        )
    lines.append("- delivery verdict: ACKNOWLEDGED — четыре повтора приняты без повторного применения")
    return "\n".join(lines)


def durable_state(snapshot):
    account = snapshot.get("account") or {}
    payment = snapshot.get("payment") or {}
    refund = snapshot.get("refund") or {}
    attempt = refund.get("attempt") or {}
    return {
        "account_tier": account.get("tier"),
        "vip_expires_at": account.get("vip_expires_at"),
        "payment_id": payment.get("id"),
        "payment_status": payment.get("status"),
        "paid_at": payment.get("paid_at"),
        "entitlement_applied_at": payment.get("entitlement_applied_at"),
        "entitlement_started_at": payment.get("entitlement_started_at"),
        "entitlement_ends_at": payment.get("entitlement_ends_at"),
        "refund_id": refund.get("id"),
        "refund_status": refund.get("status"),
        "refund_amount_minor": refund.get("amount_minor"),
        "refund_mode": refund.get("mode"),
        "refund_succeeded_at": refund.get("succeeded_at"),
        "entitlement_revoked_at": refund.get("entitlement_revoked_at"),
        "attempt_no": attempt.get("attempt_no"),
        "attempt_status": attempt.get("status"),
        "refund_count": snapshot.get("refund_count"),
        "succeeded_refund_count": snapshot.get("succeeded_refund_count"),
        "succeeded_refund_total_minor": snapshot.get("succeeded_refund_total_minor"),
        "refund_attempt_count": snapshot.get("refund_attempt_count"),
    }


def verify_unchanged(before, after):
    first = validate_target(before)
    second = validate_target(after)
    if first["payment_prefix"] != second["payment_prefix"] or first["refund_prefix"] != second["refund_prefix"]:
        raise ReplayError("the selected payment or refund changed during replay")
    if durable_state(before) != durable_state(after):
        raise ReplayError("durable payment, refund or entitlement state changed during replay")
    return "\n".join(
        [
            "## Durable replay readback",
            "- account tier: FREE; VIP expiry: отсутствует",
            "- refunds: 1 succeeded; provider attempts: 1; amount unchanged",
            "- entitlement timestamps: unchanged; refund revocation: unchanged",
            "**Verdict: IDEMPOTENT — повторная доставка или операция не изменила деньги или VIP.**",
        ]
    )


def main(argv=None):
    parser = argparse.ArgumentParser()
    subparsers = parser.add_subparsers(dest="action", required=True)
    replay_parser = subparsers.add_parser("replay")
    replay_parser.add_argument("snapshot")
    verify_parser = subparsers.add_parser("verify")
    verify_parser.add_argument("before")
    verify_parser.add_argument("after")
    args = parser.parse_args(argv)

    try:
        if args.action == "replay":
            print(replay(load_snapshot(args.snapshot)))
        else:
            print(verify_unchanged(load_snapshot(args.before), load_snapshot(args.after)))
    except ReplayError as error:
        print(f"Webhook replay refused: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
