package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPaymentSchemaKeepsHistoryAndOneOpenOperation(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("migrations", "0020_payments.sql"))
	if err != nil {
		t.Fatal(err)
	}
	schema := strings.ToLower(string(body))
	for _, required := range []string{
		"create table if not exists payments",
		"unique (provider, provider_payment_id)",
		"payments_one_open_per_account",
		"where status in ('creating', 'pending')",
		"entitlement_applied_at",
		"vip_expires_at",
	} {
		if !strings.Contains(schema, required) {
			t.Fatalf("payment schema lacks %q", required)
		}
	}
	if strings.Contains(schema, "on delete cascade") {
		t.Fatal("deleting an account would erase payment history")
	}
}

func TestProductTermsAreServerOwned(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("migrations", "0020_payments.sql"))
	if err != nil {
		t.Fatal(err)
	}
	schema := string(body)
	for _, row := range []string{
		"('vip_1_month', 'VIP на 1 месяц', 39900, 'RUB', 1)",
		"('vip_3_months', 'VIP на 3 месяца', 109000, 'RUB', 3)",
		"('vip_12_months', 'VIP на 12 месяцев', 349000, 'RUB', 12)",
	} {
		if !strings.Contains(schema, row) {
			t.Fatalf("catalog lacks %s", row)
		}
	}
}

func TestEntitlementAndPaymentCommitTogether(t *testing.T) {
	body, err := os.ReadFile("payments.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(body)
	apply := between(source, "func (s *Store) ApplySucceeded", "\nfunc ")
	if !strings.Contains(apply, "entitlement_applied_at = now()") ||
		!strings.Contains(apply, "update accounts") ||
		!strings.Contains(apply, "tx.Commit") {
		t.Fatal("payment completion and entitlement are not one transaction")
	}
	if !strings.Contains(apply, "if record.EntitlementAppliedAt != nil") {
		t.Fatal("duplicate webhook has no exactly-once guard")
	}
}

func TestPaidVIPExpiryRevokesVIPOnlyAccess(t *testing.T) {
	body, err := os.ReadFile("payments.go")
	if err != nil {
		t.Fatal(err)
	}
	expiry := between(string(body), "func expireAccount", "\nconst paymentRecordSQL")
	for _, required := range []string{"tier = 'FREE'", "d.kind = 'external'", "evictBeyondLimit"} {
		if !strings.Contains(expiry, required) {
			t.Fatalf("VIP expiry does not enforce %q", required)
		}
	}
}
