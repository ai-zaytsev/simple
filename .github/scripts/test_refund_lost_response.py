#!/usr/bin/env python3

import copy
import datetime as dt
import pathlib
import unittest

import refund_lost_response as retry


PAYMENT_ID = "11111111-1111-4111-8111-111111111111"
PROVIDER_PAYMENT_ID = "provider-payment-private"
REFUND_ID = "22222222-2222-4222-8222-222222222222"
PROVIDER_REFUND_ID = "provider-refund-private"
IDEMPOTENCY_KEY = "33333333-3333-4333-8333-333333333333"
NOW = dt.datetime(2026, 9, 1, 6, 0, tzinfo=dt.timezone.utc)


def snapshot():
    return {
        "match_count": 1,
        "account": {"id": "account-private", "tier": "FREE", "vip_expires_at": None},
        "payment": {
            "id": PAYMENT_ID,
            "provider": "yookassa",
            "provider_payment_id": PROVIDER_PAYMENT_ID,
            "amount_minor": 39900,
            "currency": "RUB",
            "status": "succeeded",
            "provider_test": True,
            "paid_at": "2026-08-23T20:56:45+00:00",
            "entitlement_applied_at": "2026-08-23T20:56:45+00:00",
            "entitlement_started_at": "2026-08-23T20:56:45+00:00",
            "entitlement_ends_at": "2026-09-23T20:56:45+00:00",
        },
        "refund": {
            "id": REFUND_ID,
            "payment_id": PAYMENT_ID,
            "provider": "yookassa",
            "amount_minor": 22201,
            "currency": "RUB",
            "mode": "pro_rata",
            "status": "succeeded",
            "succeeded_at": "2026-08-31T20:58:45+00:00",
            "entitlement_revoked_at": "2026-08-31T20:58:45+00:00",
            "attempt": {
                "attempt_no": 1,
                "idempotency_key": IDEMPOTENCY_KEY,
                "provider_refund_id": PROVIDER_REFUND_ID,
                "status": "succeeded",
                "created_at": "2026-08-31T20:58:40+00:00",
            },
        },
        "refund_count": 1,
        "succeeded_refund_count": 1,
        "succeeded_refund_total_minor": 22201,
        "refund_attempt_count": 1,
        "private_marker": "must-never-print",
    }


def provider_refund():
    return {
        "id": PROVIDER_REFUND_ID,
        "status": "succeeded",
        "payment_id": PROVIDER_PAYMENT_ID,
        "amount": {"value": "222.01", "currency": "RUB"},
        "metadata": {"refund_id": REFUND_ID},
    }


class RefundLostResponseTest(unittest.TestCase):
    def test_exact_retry_returns_one_operation_and_redacts_private_values(self):
        calls = []

        def requester(method, url, headers, body):
            calls.append((method, url, headers, body))
            if method == "POST":
                return 200, provider_refund()
            return 200, {"items": [provider_refund()]}

        report = retry.retry(snapshot(), "test-shop", "test-secret", requester, NOW)
        self.assertEqual(["POST", "GET"], [call[0] for call in calls])
        post = calls[0]
        self.assertEqual(IDEMPOTENCY_KEY, post[2]["Idempotence-Key"])
        self.assertEqual("222.01", post[3]["amount"]["value"])
        self.assertEqual(REFUND_ID, post[3]["metadata"]["refund_id"])
        self.assertIn("IDEMPOTENT", report)
        for private in [
            PAYMENT_ID,
            PROVIDER_PAYMENT_ID,
            REFUND_ID,
            PROVIDER_REFUND_ID,
            IDEMPOTENCY_KEY,
            "test-shop",
            "test-secret",
            "must-never-print",
        ]:
            self.assertNotIn(private, report)

    def test_expired_window_refuses_before_network(self):
        data = snapshot()
        data["refund"]["attempt"]["created_at"] = "2026-08-31T05:59:59+00:00"
        calls = []
        with self.assertRaises(retry.RetryError):
            retry.retry(data, "shop", "secret", lambda *args: calls.append(args), NOW)
        self.assertEqual([], calls)

    def test_production_and_duplicate_refund_are_refused(self):
        production = snapshot()
        production["payment"]["provider_test"] = False
        duplicate = snapshot()
        duplicate["refund_count"] = 2
        for data in [production, duplicate]:
            with self.assertRaises((retry.RetryError, retry.durable.ReplayError)):
                retry.validate_target(data, NOW)

    def test_different_post_object_is_rejected(self):
        wrong = provider_refund()
        wrong["id"] = "another-refund"
        with self.assertRaises(retry.RetryError):
            retry.retry(snapshot(), "shop", "secret", lambda *_: (200, wrong), NOW)

    def test_duplicate_provider_list_is_rejected(self):
        def requester(method, *_):
            if method == "POST":
                return 200, provider_refund()
            return 200, {"items": [provider_refund(), copy.deepcopy(provider_refund())]}

        with self.assertRaises(retry.RetryError):
            retry.retry(snapshot(), "shop", "secret", requester, NOW)

    def test_workflow_and_sql_keep_retry_private_and_test_only(self):
        scripts = pathlib.Path(__file__).resolve().parent
        workflow = (scripts.parent / "workflows" / "refund-lost-response.yml").read_text(
            encoding="utf-8"
        )
        sql = (scripts / "refund_retry_snapshot.sql").read_text(encoding="utf-8")
        self.assertIn("account_prefix:", workflow)
        self.assertNotIn("idempotency_key:", workflow)
        self.assertNotIn("provider_payment_id:", workflow)
        self.assertIn("if: always()", workflow)
        for guard in [
            "p.provider_test is true",
            "p.status = 'succeeded'",
            "r.status = 'succeeded'",
            "ra.idempotency_key",
        ]:
            self.assertIn(guard, sql)


if __name__ == "__main__":
    unittest.main()
