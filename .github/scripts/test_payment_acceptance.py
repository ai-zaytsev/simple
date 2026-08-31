#!/usr/bin/env python3

import pathlib
import unittest

import payment_acceptance as acceptance


PAYMENT_ID = "11111111-1111-4111-8111-111111111111"
PROVIDER_PAYMENT_ID = "2f8c-provider-payment-secret"
REFUND_ID = "22222222-2222-4222-8222-222222222222"
PROVIDER_REFUND_ID = "2f8c-provider-refund-secret"


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
            "product_id": "vip_1_month",
            "provider": "yookassa",
            "provider_payment_id": PROVIDER_PAYMENT_ID,
            "amount_minor": 39900,
            "currency": "RUB",
            "status": "succeeded",
            "provider_test": True,
            "payment_method": "bank_card",
            "provider_refundable": True,
            "paid_at": "2026-08-31T00:00:00Z",
            "entitlement_applied_at": "2026-08-31T00:00:00Z",
            "entitlement_started_at": "2026-08-31T00:00:00Z",
            "entitlement_ends_at": "2026-09-30T00:00:00Z",
            "checkout": "must-never-print",
        },
        "refund": {
            "id": REFUND_ID,
            "payment_id": PAYMENT_ID,
            "provider": "yookassa",
            "amount_minor": 39900,
            "currency": "RUB",
            "mode": "full",
            "status": "succeeded",
            "cancellation_reason": None,
            "entitlement_revoked_at": "2026-08-31T01:00:00Z",
            "attempt": {
                "attempt_no": 1,
                "provider_refund_id": PROVIDER_REFUND_ID,
                "status": "succeeded",
                "idempotency_key": "must-never-print-either",
            },
        },
    }


def provider(kind, object_id):
    if (kind, object_id) == ("payments", PROVIDER_PAYMENT_ID):
        return {
            "id": PROVIDER_PAYMENT_ID,
            "status": "succeeded",
            "paid": True,
            "test": True,
            "refundable": False,
            "amount": {"value": "399.00", "currency": "RUB"},
            "payment_method": {"type": "bank_card"},
            "metadata": {"payment_id": PAYMENT_ID},
        }
    if (kind, object_id) == ("refunds", PROVIDER_REFUND_ID):
        return {
            "id": PROVIDER_REFUND_ID,
            "status": "succeeded",
            "amount": {"value": "399.00", "currency": "RUB"},
            "payment_id": PROVIDER_PAYMENT_ID,
            "metadata": {"refund_id": REFUND_ID},
        }
    raise AssertionError("unexpected provider lookup")


class PaymentAcceptanceTest(unittest.TestCase):
    def test_matching_full_refund_is_redacted(self):
        report, ok = acceptance.build_report(snapshot(), provider)
        self.assertTrue(ok)
        self.assertIn("Verdict: MATCH", report)
        self.assertIn("399,00 ₽", report)
        self.assertIn("bank_card", report)
        for forbidden in [
            PAYMENT_ID,
            PROVIDER_PAYMENT_ID,
            REFUND_ID,
            PROVIDER_REFUND_ID,
            "must-never-print",
            "must-never-print-either",
        ]:
            self.assertNotIn(forbidden, report)

    def test_provider_mismatch_fails_the_verdict(self):
        def mismatched(kind, object_id):
            answer = provider(kind, object_id).copy()
            if kind == "refunds":
                answer["amount"] = {"value": "1.00", "currency": "RUB"}
            return answer

        report, ok = acceptance.build_report(snapshot(), mismatched)
        self.assertFalse(ok)
        self.assertIn("refund amount", report)
        self.assertIn("MISMATCH", report)

    def test_ambiguous_account_is_refused(self):
        data = snapshot()
        data["match_count"] = 2
        with self.assertRaises(acceptance.ReadbackError):
            acceptance.build_report(data, provider)

    def test_partial_preparation_has_all_test_only_guards(self):
        source = pathlib.Path(__file__).with_name("prepare_partial_payment.sql").read_text()
        for guard in [
            "provider_test is true",
            "p.status = 'succeeded'",
            "p.provider_refundable is true",
            "p.payment_method = 'bank_card'",
            "target_tier <> 'VIP'",
            "not exists (select 1 from refunds",
            "into strict target_account_id",
        ]:
            self.assertIn(guard, source)
        self.assertIn("statement_timestamp() - interval '8 days'", source)
        self.assertNotIn("entitlement_started_at - interval", source)


if __name__ == "__main__":
    unittest.main()
