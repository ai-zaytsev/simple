#!/usr/bin/env python3

import copy
import pathlib
import unittest

import payment_webhook_replay as replay


PAYMENT_ID = "11111111-1111-4111-8111-111111111111"
PROVIDER_PAYMENT_ID = "provider-payment-private-value"
REFUND_ID = "22222222-2222-4222-8222-222222222222"
PROVIDER_REFUND_ID = "provider-refund-private-value"


def snapshot():
    return {
        "match_count": 1,
        "account": {
            "id": "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
            "tier": "FREE",
            "vip_expires_at": None,
        },
        "payment": {
            "id": PAYMENT_ID,
            "provider": "yookassa",
            "provider_payment_id": PROVIDER_PAYMENT_ID,
            "amount_minor": 39900,
            "currency": "RUB",
            "status": "succeeded",
            "provider_test": True,
            "paid_at": "2026-08-23T20:56:45Z",
            "entitlement_applied_at": "2026-08-23T20:56:45Z",
            "entitlement_started_at": "2026-08-23T20:56:45Z",
            "entitlement_ends_at": "2026-09-23T20:56:45Z",
        },
        "refund": {
            "id": REFUND_ID,
            "payment_id": PAYMENT_ID,
            "provider": "yookassa",
            "amount_minor": 22201,
            "currency": "RUB",
            "mode": "pro_rata",
            "status": "succeeded",
            "succeeded_at": "2026-08-31T20:58:45Z",
            "entitlement_revoked_at": "2026-08-31T20:58:45Z",
            "attempt": {
                "attempt_no": 1,
                "provider_refund_id": PROVIDER_REFUND_ID,
                "status": "succeeded",
            },
        },
        "refund_count": 1,
        "succeeded_refund_count": 1,
        "succeeded_refund_total_minor": 22201,
        "refund_attempt_count": 1,
        "private_marker": "must-never-print",
    }


class PaymentWebhookReplayTest(unittest.TestCase):
    def test_replays_both_events_twice_and_redacts_private_ids(self):
        calls = []

        def sender(url, event, object_id):
            calls.append((url, event, object_id))
            return {"status": 200, "received": True, "applied": False}

        report = replay.replay(snapshot(), sender=sender)
        self.assertEqual(
            [(event, object_id) for _, event, object_id in calls],
            [
                ("payment.succeeded", PROVIDER_PAYMENT_ID),
                ("payment.succeeded", PROVIDER_PAYMENT_ID),
                ("refund.succeeded", PROVIDER_REFUND_ID),
                ("refund.succeeded", PROVIDER_REFUND_ID),
            ],
        )
        self.assertIn("ACKNOWLEDGED", report)
        self.assertIn("11111111", report)
        self.assertIn("22222222", report)
        for private in [
            PAYMENT_ID,
            REFUND_ID,
            PROVIDER_PAYMENT_ID,
            PROVIDER_REFUND_ID,
            "must-never-print",
        ]:
            self.assertNotIn(private, report)

    def test_production_payment_is_refused_before_delivery(self):
        data = snapshot()
        data["payment"]["provider_test"] = False
        calls = []
        with self.assertRaises(replay.ReplayError):
            replay.replay(data, sender=lambda *args: calls.append(args))
        self.assertEqual([], calls)

    def test_applied_true_is_a_blocking_result(self):
        def sender(*_):
            return {"status": 200, "received": True, "applied": True}

        with self.assertRaises(replay.ReplayError):
            replay.replay(snapshot(), sender=sender)

    def test_ambiguous_account_and_duplicate_refund_are_refused(self):
        ambiguous = snapshot()
        ambiguous["match_count"] = 2
        duplicate = snapshot()
        duplicate["refund_count"] = 2
        for data in [ambiguous, duplicate]:
            with self.assertRaises(replay.ReplayError):
                replay.validate_target(data)

    def test_unchanged_state_passes_and_mutation_fails(self):
        before = snapshot()
        report = replay.verify_unchanged(before, copy.deepcopy(before))
        self.assertIn("IDEMPOTENT", report)

        changed = copy.deepcopy(before)
        changed["payment"]["entitlement_ends_at"] = "2026-10-23T20:56:45Z"
        with self.assertRaises(replay.ReplayError):
            replay.verify_unchanged(before, changed)

    def test_workflow_keeps_provider_ids_private_and_cleans_snapshots(self):
        workflow = (
            pathlib.Path(__file__).resolve().parents[1]
            / "workflows"
            / "payment-webhook-replay.yml"
        ).read_text(encoding="utf-8")
        self.assertIn("account_prefix:", workflow)
        self.assertNotIn("provider_payment_id:", workflow)
        self.assertNotIn("provider_refund_id:", workflow)
        self.assertIn("if: always()", workflow)
        self.assertIn("rm -f /tmp/webhook-before.json /tmp/webhook-after.json", workflow)


if __name__ == "__main__":
    unittest.main()
