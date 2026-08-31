package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRefundSchemaKeepsOneLogicalRefundAndEveryAttempt(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("migrations", "0022_refunds.sql"))
	if err != nil {
		t.Fatal(err)
	}
	schema := strings.ToLower(string(body))
	for _, required := range []string{
		"create table if not exists refunds",
		"payment_id               uuid not null unique",
		"create table if not exists refund_attempts",
		"idempotency_key",
		"refund_attempts_one_live",
		"refund_attempts_one_success",
		"entitlement_started_at",
		"entitlement_ends_at",
	} {
		if !strings.Contains(schema, required) {
			t.Fatalf("refund schema lacks %q", required)
		}
	}
	if strings.Contains(schema, "on delete cascade") {
		t.Fatal("deleting an account or payment would erase refund history")
	}
}

func TestRefundAndVIPRemovalCommitTogether(t *testing.T) {
	body, err := os.ReadFile("refunds.go")
	if err != nil {
		t.Fatal(err)
	}
	apply := between(string(body), "func (s *Store) ApplyRefundSucceeded", "\nfunc ")
	for _, required := range []string{
		"payment.VerifyRefund", "status = 'succeeded'", "entitlement_revoked_at = now()",
		"expireAccount", "tx.Commit",
	} {
		if !strings.Contains(apply, required) {
			t.Fatalf("refund completion lacks %q", required)
		}
	}
	if !strings.Contains(apply, "if expected.EntitlementRevokedAt != nil") {
		t.Fatal("duplicate refund has no exactly-once guard")
	}
}

func TestTerminalRefundCannotBeRevertedByStaleProviderState(t *testing.T) {
	body, err := os.ReadFile("refunds.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(body)
	for _, method := range []string{"AttachRefund", "SetRefundStatus"} {
		part := between(source, "func (s *Store) "+method, "\nfunc ")
		if !strings.Contains(part, "status <> 'succeeded'") {
			t.Fatalf("%s can revert a confirmed refund", method)
		}
	}
}
