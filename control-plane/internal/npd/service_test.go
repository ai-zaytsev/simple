package npd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"download.simplevpn/control-plane/internal/npd/lknpd"
)

type fakeRepo struct {
	session     lknpd.Session
	savedTimes  int
	pending     []Operation
	settlements map[string]Settlement

	availabilitySet   bool
	availabilityOK    bool
	availabilityWhy   string
	begun             []int64
	finished          []string
	cancelledRows     []string
	doneOperations    []string
	failedOperations  []string
	failureAlerted    []bool
	pendingAfterDrain int
	askedDeviceID     string
}

func newRepo() *fakeRepo {
	return &fakeRepo{
		session:     lknpd.Session{DeviceID: "device", AccessToken: "tok", RefreshToken: "r", INN: "1"},
		settlements: map[string]Settlement{},
	}
}

func (r *fakeRepo) LoadSession(_ context.Context, preferredDeviceID string) (lknpd.Session, error) {
	r.askedDeviceID = preferredDeviceID
	if preferredDeviceID != "" && r.session.DeviceID != preferredDeviceID {
		// The store drops tokens that belonged to another device; the fake
		// does the same, so the tests exercise the real shape.
		return lknpd.Session{DeviceID: preferredDeviceID, INN: r.session.INN}, nil
	}
	return r.session, nil
}
func (r *fakeRepo) SaveSession(_ context.Context, s lknpd.Session) error {
	r.session = s
	r.savedTimes++
	return nil
}
func (r *fakeRepo) SetAvailability(_ context.Context, ok bool, detail string, _ time.Time) error {
	r.availabilitySet, r.availabilityOK, r.availabilityWhy = true, ok, detail
	return nil
}
func (r *fakeRepo) PendingOperations(context.Context, int) ([]Operation, error) {
	return r.pending, nil
}
func (r *fakeRepo) PendingCount(context.Context) (int, error) { return r.pendingAfterDrain, nil }
func (r *fakeRepo) OperationDone(_ context.Context, id string) error {
	r.doneOperations = append(r.doneOperations, id)
	return nil
}
func (r *fakeRepo) OperationFailed(_ context.Context, id, _ string, alerted bool) error {
	r.failedOperations = append(r.failedOperations, id)
	r.failureAlerted = append(r.failureAlerted, alerted)
	return nil
}
func (r *fakeRepo) Settlement(_ context.Context, paymentID string) (Settlement, error) {
	s, ok := r.settlements[paymentID]
	if !ok {
		return Settlement{}, fmt.Errorf("no settlement for %s", paymentID)
	}
	return s, nil
}
func (r *fakeRepo) BeginReceipt(_ context.Context, _ string, amountMinor int64) (string, error) {
	r.begun = append(r.begun, amountMinor)
	return fmt.Sprintf("row-%d", len(r.begun)), nil
}
func (r *fakeRepo) FinishReceipt(_ context.Context, _, receiptUUID, _ string) error {
	r.finished = append(r.finished, receiptUUID)
	return nil
}
func (r *fakeRepo) CancelReceipt(_ context.Context, rowID string, _ time.Time) error {
	r.cancelledRows = append(r.cancelledRows, rowID)
	return nil
}

type fakeAdapter struct {
	aliveErr     error
	createErr    error
	cancelErr    error
	created      []int64
	cancelled    []string
	refreshes    int
	logins       int
	refreshErr   error
	profileErr   error
	profileCalls int
	refreshSeen  []string
	rotateTo     string
	refuseToken  string

	// unauthorizedUntil makes the first N calls answer with a dead session, so
	// that renewal can be exercised.
	unauthorizedUntil int
	calls             int
}

func (a *fakeAdapter) Alive(context.Context, lknpd.Session) error { return a.aliveErr }
func (a *fakeAdapter) Profile(_ context.Context, s lknpd.Session) (string, error) {
	a.profileCalls++
	if a.profileErr != nil {
		return "", a.profileErr
	}
	if s.INN != "" {
		return s.INN, nil
	}
	return "123456789012", nil
}
func (a *fakeAdapter) Login(context.Context, string, string, string) (lknpd.Session, error) {
	a.logins++
	return lknpd.Session{AccessToken: "fresh", DeviceID: "device", INN: "1"}, nil
}
func (a *fakeAdapter) Refresh(_ context.Context, s lknpd.Session) (lknpd.Session, error) {
	a.refreshes++
	a.refreshSeen = append(a.refreshSeen, s.RefreshToken+"@"+s.DeviceID)
	if a.refreshErr != nil {
		return lknpd.Session{}, a.refreshErr
	}
	if a.refuseToken != "" && s.RefreshToken == a.refuseToken {
		return lknpd.Session{}, fmt.Errorf("%w: отозван", lknpd.ErrUnauthorized)
	}
	renewed := lknpd.Session{
		AccessToken: "renewed", DeviceID: s.DeviceID, INN: s.INN,
		RefreshToken: s.RefreshToken,
	}
	if a.rotateTo != "" {
		renewed.RefreshToken = a.rotateTo
	}
	return renewed, nil
}
func (a *fakeAdapter) CreateReceipt(
	_ context.Context, _ lknpd.Session, _ string, amountMinor int64, _ time.Time,
) (string, string, error) {
	a.calls++
	if a.calls <= a.unauthorizedUntil {
		return "", "", fmt.Errorf("%w: stale", lknpd.ErrUnauthorized)
	}
	if a.createErr != nil {
		return "", "", a.createErr
	}
	a.created = append(a.created, amountMinor)
	return fmt.Sprintf("receipt-%d", len(a.created)), "https://example/print", nil
}
func (a *fakeAdapter) CancelReceipt(_ context.Context, _ lknpd.Session, uuid string, _ time.Time) error {
	if a.cancelErr != nil {
		return a.cancelErr
	}
	a.cancelled = append(a.cancelled, uuid)
	return nil
}

type fakeAlerter struct {
	receiptFailed []string
	serviceDown   []string
}

func (a *fakeAlerter) ReceiptFailed(_ context.Context, s Settlement, reason string, _ int) error {
	a.receiptFailed = append(a.receiptFailed, s.PaymentID+": "+reason)
	return nil
}
func (a *fakeAlerter) TaxServiceDown(_ context.Context, reason string, _ int) error {
	a.serviceDown = append(a.serviceDown, reason)
	return nil
}

func serviceFor(t *testing.T, repo *fakeRepo, adapter *fakeAdapter, alerter *fakeAlerter) *Service {
	t.Helper()
	s, err := NewService(repo, adapter, alerter,
		Credentials{INN: "123456789012", Password: "secret"},
		"Simple VPN", slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSuccessfulPaymentIsSettledOnce(t *testing.T) {
	repo := newRepo()
	repo.pending = []Operation{{ID: "op1", PaymentID: "p1"}}
	repo.settlements["p1"] = Settlement{PaymentID: "p1", PaidMinor: 39900, PaidAt: time.Now()}
	adapter := &fakeAdapter{}

	settled, err := serviceFor(t, repo, adapter, &fakeAlerter{}).Drain(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if settled != 1 || len(adapter.created) != 1 || adapter.created[0] != 39900 {
		t.Fatalf("expected one receipt for 39900, settled=%d created=%v", settled, adapter.created)
	}
	if len(repo.doneOperations) != 1 {
		t.Fatalf("the operation must be closed, got %v", repo.doneOperations)
	}
}

func TestARowIsWrittenBeforeAskingForAReceipt(t *testing.T) {
	// The window between ФНС accepting and us learning the identifier is the
	// one place a duplicate receipt could be born. The row exists first, so a
	// crash there blocks the next attempt instead of creating a second.
	repo := newRepo()
	repo.pending = []Operation{{ID: "op1", PaymentID: "p1"}}
	repo.settlements["p1"] = Settlement{PaymentID: "p1", PaidMinor: 39900, PaidAt: time.Now()}
	adapter := &fakeAdapter{createErr: errors.New("сеть отвалилась")}

	_, _ = serviceFor(t, repo, adapter, &fakeAlerter{}).Drain(context.Background(), 10)

	if len(repo.begun) != 1 {
		t.Fatalf("the receipt row must be opened before the call, got %v", repo.begun)
	}
	if len(repo.finished) != 0 {
		t.Fatal("nothing may be marked issued when the call failed")
	}
}

func TestAnUnresolvedReceiptStopsAnotherAttempt(t *testing.T) {
	repo := newRepo()
	repo.pending = []Operation{{ID: "op1", PaymentID: "p1"}}
	repo.settlements["p1"] = Settlement{PaymentID: "p1", PaidMinor: 39900, Unresolved: true}
	adapter := &fakeAdapter{}
	alerter := &fakeAlerter{}

	settled, err := serviceFor(t, repo, adapter, alerter).Drain(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if settled != 0 || len(adapter.created) != 0 {
		t.Fatal("a payment whose receipt may already exist must not get another")
	}
	if len(alerter.receiptFailed) != 1 {
		t.Fatal("a person has to be told, because only a person can check Мой налог")
	}
}

func TestPartialRefundCancelsThenCreatesTheRemainder(t *testing.T) {
	repo := newRepo()
	repo.pending = []Operation{{ID: "op1", PaymentID: "p1"}}
	repo.settlements["p1"] = Settlement{
		PaymentID: "p1", PaidMinor: 120000, RefundedMinor: 60000,
		Active: &Receipt{RowID: "row-old", UUID: "old", AmountMinor: 120000},
	}
	adapter := &fakeAdapter{}

	if _, err := serviceFor(t, repo, adapter, &fakeAlerter{}).Drain(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	if len(adapter.cancelled) != 1 || adapter.cancelled[0] != "old" {
		t.Fatalf("the standing receipt must be voided, got %v", adapter.cancelled)
	}
	if len(adapter.created) != 1 || adapter.created[0] != 60000 {
		t.Fatalf("the replacement is the remainder, got %v", adapter.created)
	}
	if len(repo.cancelledRows) != 1 || repo.cancelledRows[0] != "row-old" {
		t.Fatalf("our own row must be marked cancelled, got %v", repo.cancelledRows)
	}
}

func TestOneEmailPerPaymentNotPerAttempt(t *testing.T) {
	repo := newRepo()
	repo.pending = []Operation{{ID: "op1", PaymentID: "p1", Attempts: 3, Alerted: true}}
	repo.settlements["p1"] = Settlement{PaymentID: "p1", PaidMinor: 39900}
	adapter := &fakeAdapter{createErr: errors.New("отказ")}
	alerter := &fakeAlerter{}

	_, _ = serviceFor(t, repo, adapter, alerter).Drain(context.Background(), 10)

	if len(alerter.receiptFailed) != 0 {
		t.Fatalf("this payment was already reported; a mailbox that repeats gets filtered")
	}
	if len(repo.failureAlerted) != 1 || !repo.failureAlerted[0] {
		t.Fatal("the operation must stay marked as already reported")
	}
}

func TestAnOutageStopsWorkingThroughTheRest(t *testing.T) {
	// Hammering a service that is down neither helps it nor us, and against an
	// unofficial API it is how an address gets refused entirely.
	repo := newRepo()
	repo.pending = []Operation{
		{ID: "op1", PaymentID: "p1"},
		{ID: "op2", PaymentID: "p2"},
	}
	repo.settlements["p1"] = Settlement{PaymentID: "p1", PaidMinor: 39900}
	repo.settlements["p2"] = Settlement{PaymentID: "p2", PaidMinor: 39900}
	adapter := &fakeAdapter{createErr: fmt.Errorf("%w: работы", lknpd.ErrServiceUnavailable)}

	if _, err := serviceFor(t, repo, adapter, &fakeAlerter{}).Drain(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	if len(repo.failedOperations) != 1 {
		t.Fatalf("expected to stop after the first outage, got %v", repo.failedOperations)
	}
}

func TestAFailureDuringTheDayDoesNotCloseSales(t *testing.T) {
	// The stage is explicit: a receipt that fails after a payment must not
	// stop selling by itself. Only the morning check decides that.
	repo := newRepo()
	repo.pending = []Operation{{ID: "op1", PaymentID: "p1"}}
	repo.settlements["p1"] = Settlement{PaymentID: "p1", PaidMinor: 39900}
	adapter := &fakeAdapter{createErr: fmt.Errorf("%w: работы", lknpd.ErrServiceUnavailable)}

	_, _ = serviceFor(t, repo, adapter, &fakeAlerter{}).Drain(context.Background(), 10)

	if repo.availabilitySet {
		t.Fatal("draining must not touch the switch that governs selling")
	}
}

func TestMorningCheckOpensSalesOnlyWithNothingOwed(t *testing.T) {
	repo := newRepo()
	repo.pendingAfterDrain = 0
	adapter := &fakeAdapter{}

	ok, err := serviceFor(t, repo, adapter, &fakeAlerter{}).CheckAvailability(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !ok || !repo.availabilityOK {
		t.Fatal("a healthy service with nothing owed allows selling")
	}
}

func TestMorningCheckKeepsSalesClosedWhileReceiptsAreOwed(t *testing.T) {
	// ФНС answers and we still cannot file. That is not the weather - it is
	// us, and selling more would add to a debt already unpaid.
	repo := newRepo()
	repo.pendingAfterDrain = 2
	adapter := &fakeAdapter{}
	alerter := &fakeAlerter{}

	ok, err := serviceFor(t, repo, adapter, alerter).CheckAvailability(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if ok || repo.availabilityOK {
		t.Fatal("selling must stay closed while receipts are owed")
	}
	if len(alerter.serviceDown) != 1 {
		t.Fatal("a person has to be told why selling is still closed")
	}
}

func TestMorningCheckClosesSalesWhenTheServiceIsDown(t *testing.T) {
	repo := newRepo()
	adapter := &fakeAdapter{aliveErr: fmt.Errorf("%w: работы", lknpd.ErrServiceUnavailable)}
	alerter := &fakeAlerter{}

	ok, err := serviceFor(t, repo, adapter, alerter).CheckAvailability(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if ok || repo.availabilityOK {
		t.Fatal("selling must close when receipts cannot be issued")
	}
	if len(alerter.serviceDown) != 1 {
		t.Fatalf("Business Owner must be told, got %v", alerter.serviceDown)
	}
}

func TestADeadSessionIsRenewedOnceAndTheCallRetried(t *testing.T) {
	repo := newRepo()
	repo.pending = []Operation{{ID: "op1", PaymentID: "p1"}}
	repo.settlements["p1"] = Settlement{PaymentID: "p1", PaidMinor: 39900}
	adapter := &fakeAdapter{unauthorizedUntil: 1}

	settled, err := serviceFor(t, repo, adapter, &fakeAlerter{}).Drain(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if adapter.refreshes != 1 {
		t.Fatalf("expected exactly one renewal, got %d", adapter.refreshes)
	}
	if settled != 1 {
		t.Fatal("the call should have succeeded on the retry")
	}
}

func TestAnOutageDuringRenewalDoesNotBecomeAPasswordLogin(t *testing.T) {
	// Logging in with a password at a service that is already unwell is one
	// more request it cannot answer, and repeated logins to an unofficial API
	// are what earns a CAPTCHA.
	repo := newRepo()
	repo.session = lknpd.Session{DeviceID: "device", RefreshToken: "r", ExpiresAt: time.Now().Add(-time.Hour)}
	adapter := &fakeAdapter{refreshErr: fmt.Errorf("%w: работы", lknpd.ErrServiceUnavailable)}

	_, err := serviceFor(t, repo, adapter, &fakeAlerter{}).CheckAvailability(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if adapter.logins != 0 {
		t.Fatal("an outage must not turn into a password login")
	}
	if repo.availabilityOK {
		t.Fatal("selling must be closed")
	}
}

func TestServiceRefusesToStartWithoutCredentials(t *testing.T) {
	_, err := NewService(newRepo(), &fakeAdapter{}, nil, Credentials{}, "",
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err == nil {
		t.Fatal("a tax module with no credentials would fail silently at the worst moment")
	}
}
