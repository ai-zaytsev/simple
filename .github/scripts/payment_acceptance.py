#!/usr/bin/env python3
"""Redacted DB/provider readback for the YooKassa test-store matrix."""

from __future__ import annotations

import argparse
import base64
import json
import os
import sys
import urllib.error
import urllib.parse
import urllib.request
from decimal import Decimal, InvalidOperation
from typing import Any, Callable


API = "https://api.yookassa.ru/v3"


class ReadbackError(Exception):
    """A bounded error that is safe to print in a public Actions log."""


def money_minor(amount: Any) -> int:
    try:
        return int(Decimal(str(amount)) * 100)
    except (InvalidOperation, ValueError) as problem:
        raise ReadbackError("provider returned an unreadable amount") from problem


def short_id(value: Any) -> str:
    text = str(value or "")
    return text[:8] if text else "—"


def yes_no(value: Any) -> str:
    if value is None:
        return "неизвестно"
    return "да" if bool(value) else "нет"


def rubles(minor: Any) -> str:
    value = int(minor or 0)
    return f"{value // 100},{value % 100:02d} ₽"


def provider_reader(shop_id: str, secret: str) -> Callable[[str, str], dict[str, Any]]:
    token = base64.b64encode(f"{shop_id}:{secret}".encode()).decode()

    def read(kind: str, object_id: str) -> dict[str, Any]:
        if kind not in {"payments", "refunds"} or not object_id:
            raise ReadbackError("provider object is not attached")
        request = urllib.request.Request(
            f"{API}/{kind}/{urllib.parse.quote(object_id, safe='')}",
            headers={"Authorization": f"Basic {token}", "Accept": "application/json"},
        )
        try:
            with urllib.request.urlopen(request, timeout=20) as response:
                return json.load(response)
        except urllib.error.HTTPError as problem:
            raise ReadbackError(f"provider {kind[:-1]} read failed: HTTP {problem.code}") from None
        except (urllib.error.URLError, TimeoutError, json.JSONDecodeError):
            raise ReadbackError(f"provider {kind[:-1]} read failed") from None

    return read


def compare(problems: list[str], name: str, local: Any, remote: Any) -> None:
    if local != remote:
        problems.append(f"{name}: DB и provider расходятся")


def build_report(
    snapshot: dict[str, Any], read_provider: Callable[[str, str], dict[str, Any]]
) -> tuple[str, bool]:
    if snapshot.get("match_count") != 1 or not snapshot.get("account"):
        raise ReadbackError("account prefix must match exactly one account")

    account = snapshot["account"]
    payment = snapshot.get("payment")
    refund = snapshot.get("refund")
    lines = [
        "## Payment acceptance readback",
        "",
        f"- account: `{short_id(account.get('id'))}`; tier: `{account.get('tier', '—')}`; "
        f"VIP expiry stored: {yes_no(account.get('vip_expires_at'))}",
    ]
    if not payment:
        lines.extend(["- payment: нет", "", "**Verdict: DB only — payment отсутствует.**"])
        return "\n".join(lines), True

    if payment.get("provider") != "yookassa":
        raise ReadbackError("latest payment belongs to an unsupported provider")
    provider_payment_id = str(payment.get("provider_payment_id") or "")
    remote_payment = read_provider("payments", provider_payment_id)
    remote_amount = remote_payment.get("amount") or {}
    remote_method = (remote_payment.get("payment_method") or {}).get("type") or ""
    problems: list[str] = []
    compare(problems, "payment object", provider_payment_id, str(remote_payment.get("id") or ""))
    compare(problems, "payment status", payment.get("status"), remote_payment.get("status"))
    compare(problems, "payment amount", int(payment.get("amount_minor") or 0), money_minor(remote_amount.get("value")))
    compare(problems, "payment currency", payment.get("currency"), remote_amount.get("currency"))
    compare(problems, "payment metadata", str(payment.get("id")), str((remote_payment.get("metadata") or {}).get("payment_id")))
    if payment.get("payment_method"):
        compare(problems, "payment method", payment.get("payment_method"), remote_method)
    if payment.get("provider_test") is not True or remote_payment.get("test") is not True:
        problems.append("payment is not confirmed as test-store on both sides")
    if payment.get("status") == "succeeded" and remote_payment.get("paid") is not True:
        problems.append("provider does not mark the succeeded payment as paid")

    lines.append(
        f"- payment `{short_id(payment.get('id'))}`: product `{payment.get('product_id', '—')}`, "
        f"DB/provider `{payment.get('status', '—')}`/`{remote_payment.get('status', '—')}`, "
        f"{rubles(payment.get('amount_minor'))} {payment.get('currency', '—')}, "
        f"test {yes_no(remote_payment.get('test'))}, method `{remote_method or '—'}`, "
        f"provider refundable now {yes_no(remote_payment.get('refundable'))}"
    )
    lines.append(
        f"- paid at stored: {yes_no(payment.get('paid_at'))}; entitlement applied: "
        f"{yes_no(payment.get('entitlement_applied_at'))}; interval stored: "
        f"{yes_no(payment.get('entitlement_started_at') and payment.get('entitlement_ends_at'))}"
    )

    if refund:
        attempt = refund.get("attempt") or {}
        provider_refund_id = str(attempt.get("provider_refund_id") or "")
        remote_refund = read_provider("refunds", provider_refund_id)
        remote_refund_amount = remote_refund.get("amount") or {}
        compare(problems, "refund object", provider_refund_id, str(remote_refund.get("id") or ""))
        compare(problems, "refund status", refund.get("status"), remote_refund.get("status"))
        compare(problems, "refund amount", int(refund.get("amount_minor") or 0), money_minor(remote_refund_amount.get("value")))
        compare(problems, "refund currency", refund.get("currency"), remote_refund_amount.get("currency"))
        compare(problems, "refund payment", provider_payment_id, str(remote_refund.get("payment_id") or ""))
        compare(problems, "refund metadata", str(refund.get("id")), str((remote_refund.get("metadata") or {}).get("refund_id")))
        lines.append(
            f"- refund `{short_id(refund.get('id'))}`: mode `{refund.get('mode', '—')}`, "
            f"DB/provider `{refund.get('status', '—')}`/`{remote_refund.get('status', '—')}`, "
            f"{rubles(refund.get('amount_minor'))} {refund.get('currency', '—')}, "
            f"attempt {attempt.get('attempt_no', '—')} `{attempt.get('status', '—')}`"
        )
        lines.append(
            f"- entitlement revoked after canonical success: "
            f"{yes_no(refund.get('entitlement_revoked_at'))}"
        )
    else:
        lines.append("- refund: нет")

    lines.append("")
    if problems:
        lines.append("**Verdict: MISMATCH.**")
        lines.extend(f"- {problem}" for problem in problems)
        return "\n".join(lines), False
    lines.append("**Verdict: MATCH — PostgreSQL и канонический provider status согласованы.**")
    return "\n".join(lines), True


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("snapshot")
    args = parser.parse_args()
    try:
        with open(args.snapshot, encoding="utf-8") as source:
            snapshot = json.load(source)
        shop_id = os.environ.get("YOOKASSA_SHOP_ID", "")
        secret = os.environ.get("YOOKASSA_SECRET_KEY", "")
        if not shop_id or not secret:
            raise ReadbackError("test-store credentials are not configured")
        report, ok = build_report(snapshot, provider_reader(shop_id, secret))
        print(report)
        return 0 if ok else 1
    except (OSError, json.JSONDecodeError, ReadbackError) as problem:
        print(f"Payment acceptance readback failed: {problem}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
