package npd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"download.simplevpn/control-plane/internal/npd/lknpd"
)

// Adapter is the tax service as this package needs it.
//
// An interface and not the concrete client, because the whole point of the
// stage is that an unofficial API is isolated behind one replaceable thing. If
// lknpd.nalog.ru changes beyond recognition, or a real one appears, this is the
// seam that moves and nothing above it does.
type Adapter interface {
	Alive(ctx context.Context, s lknpd.Session) error
	Login(ctx context.Context, inn, password, deviceID string) (lknpd.Session, error)
	Refresh(ctx context.Context, s lknpd.Session) (lknpd.Session, error)

	// Profile answers with the taxpayer identifier. Needed because a session
	// built from a refresh token alone does not carry one, and every receipt
	// address is built from it.
	Profile(ctx context.Context, s lknpd.Session) (string, error)
	CreateReceipt(ctx context.Context, s lknpd.Session, name string, amountMinor int64, at time.Time) (string, string, error)
	CancelReceipt(ctx context.Context, s lknpd.Session, receiptUUID string, at time.Time) error
}

// Operation is one payment waiting for its receipts to be put right.
type Operation struct {
	ID        string
	PaymentID string
	Attempts  int
	Alerted   bool
}

// Settlement is everything needed to decide what НПД should hold for one
// payment. Read in one go so that the decision cannot see a half-updated world.
type Settlement struct {
	PaymentID     string
	AccountID     string
	AccountEmail  string
	ProductName   string
	PaidMinor     int64
	RefundedMinor int64
	PaidAt        time.Time

	// The receipt currently standing, if any. Nil when none exists.
	Active *Receipt

	// True when a previous attempt asked ФНС for a receipt and never learned
	// the answer. Nothing may be created for this payment until a person says
	// what happened.
	Unresolved bool
}

type Repository interface {
	// preferredDeviceID is the identifier that came with the configured refresh
	// token. It wins over anything stored, and over generating one: a refresh
	// token belongs to the device it was issued for, and pairing it with any
	// other identifier makes it useless.
	LoadSession(ctx context.Context, preferredDeviceID string) (lknpd.Session, error)
	SaveSession(ctx context.Context, s lknpd.Session) error

	SetAvailability(ctx context.Context, ok bool, detail string, at time.Time) error

	PendingOperations(ctx context.Context, limit int) ([]Operation, error)
	PendingCount(ctx context.Context) (int, error)
	OperationDone(ctx context.Context, operationID string) error
	OperationFailed(ctx context.Context, operationID, message string, alerted bool) error

	Settlement(ctx context.Context, paymentID string) (Settlement, error)
	BeginReceipt(ctx context.Context, paymentID string, amountMinor int64) (string, error)
	FinishReceipt(ctx context.Context, rowID, receiptUUID, printURL string) error
	CancelReceipt(ctx context.Context, rowID string, at time.Time) error
}

// Alerter is how a person is told. One email per payment whose receipt failed,
// not one per attempt: an alert that repeats is an alert that gets filtered.
type Alerter interface {
	ReceiptFailed(ctx context.Context, s Settlement, reason string, queued int) error
	TaxServiceDown(ctx context.Context, reason string, queued int) error
}

// Credentials are the ways in. Never logged, never printed, read from the
// environment rather than kept.
//
// Two ways exist and each is optional on its own. The token pair is the
// primary one: it is what an account signed in through Госуслуги has, where no
// password to lknpd exists at all. The ИНН and password stay because they were
// here first and still work, and because a pair can be revoked while a
// password is not.
type Credentials struct {
	// The token pair, issued together. They travel together too: a refresh
	// token belongs to the device it was issued for, and pairing it with any
	// other identifier makes it useless.
	RefreshToken string
	DeviceID     string

	// The older way, untouched.
	INN      string
	Password string
}

// usable says whether any way in is configured at all.
func (c Credentials) usable() bool {
	return c.RefreshToken != "" || (c.INN != "" && c.Password != "")
}

type Service struct {
	repo    Repository
	adapter Adapter
	alerter Alerter
	creds   Credentials
	log     *slog.Logger
	now     func() time.Time

	// What appears on the receipt.
	serviceName string
}

func NewService(
	repo Repository, adapter Adapter, alerter Alerter,
	creds Credentials, serviceName string, log *slog.Logger,
) (*Service, error) {
	if repo == nil || adapter == nil {
		return nil, errors.New("npd: нужны хранилище и адаптер")
	}
	if !creds.usable() {
		// Either way in will do; neither will not. Refusing here rather than at
		// the first payment, because a service that cannot issue receipts must
		// not be one that discovers it after taking money.
		return nil, errors.New(
			"npd: нужен либо refresh token, либо ИНН с паролем")
	}
	if serviceName == "" {
		serviceName = "Доступ к сервису Simple VPN"
	}
	return &Service{
		repo: repo, adapter: adapter, alerter: alerter, creds: creds,
		log: log, now: time.Now, serviceName: serviceName,
	}, nil
}

// CheckAvailability is the once-a-day question that governs selling.
//
// Deliberately not run more often. The tax service is not polled through the
// day: an outage at noon does not stop sales that are already allowed, and
// asking repeatedly would only produce a different answer to a question nobody
// acts on until morning.
//
// Success alone does not reopen sales. Everything owed a receipt is settled
// first, and only an empty queue reopens them - otherwise recovering from an
// outage would mean selling more while still owing receipts for the last lot.
func (s *Service) CheckAvailability(ctx context.Context) (bool, error) {
	session, err := s.session(ctx)
	if err != nil {
		return false, s.unavailable(ctx, err)
	}
	if err := s.adapter.Alive(ctx, session); err != nil {
		return false, s.unavailable(ctx, err)
	}

	// Up. Now clear what is owed before anything else is sold.
	drained, drainErr := s.Drain(ctx, 200)
	if drainErr != nil {
		s.log.Error("не удалось разобрать очередь чеков", "error", drainErr)
	}

	left, err := s.repo.PendingCount(ctx)
	if err != nil {
		return false, err
	}
	if left > 0 {
		// The tax service answers and we still cannot file. That is not the
		// weather; it is us. Selling more would add to a debt we have already
		// failed to pay.
		detail := fmt.Sprintf("ФНС отвечает, но не закрыто операций: %d", left)
		if setErr := s.repo.SetAvailability(ctx, false, detail, s.now()); setErr != nil {
			return false, setErr
		}
		if s.alerter != nil {
			if alertErr := s.alerter.TaxServiceDown(ctx, detail, left); alertErr != nil {
				s.log.Error("не удалось отправить письмо о чеках", "error", alertErr)
			}
		}
		return false, nil
	}

	detail := "проверка пройдена"
	if drained > 0 {
		detail = fmt.Sprintf("проверка пройдена, закрыто операций: %d", drained)
	}
	return true, s.repo.SetAvailability(ctx, true, detail, s.now())
}

func (s *Service) unavailable(ctx context.Context, cause error) error {
	// The reason is written down as the adapter classified it. A message from
	// ФНС is safe here; a password or a token never reaches this point.
	detail := cause.Error()
	if err := s.repo.SetAvailability(ctx, false, detail, s.now()); err != nil {
		return err
	}
	queued, _ := s.repo.PendingCount(ctx)
	if s.alerter != nil {
		if err := s.alerter.TaxServiceDown(ctx, detail, queued); err != nil {
			s.log.Error("не удалось отправить письмо о недоступности ФНС", "error", err)
		}
	}
	return nil
}

// Drain works through what is owed and returns how many payments it settled.
//
// A failure here does not stop selling and does not touch the payment or the
// VIP it bought. The money changed hands; the receipt is our obligation to
// catch up on, and taking away what somebody paid for because our bookkeeping
// stumbled would be the wrong end to fix.
func (s *Service) Drain(ctx context.Context, limit int) (int, error) {
	operations, err := s.repo.PendingOperations(ctx, limit)
	if err != nil {
		return 0, err
	}
	if len(operations) == 0 {
		return 0, nil
	}

	session, err := s.session(ctx)
	if err != nil {
		return 0, err
	}

	settled := 0
	for _, operation := range operations {
		if err := ctx.Err(); err != nil {
			return settled, err
		}
		done, err := s.settle(ctx, &session, operation)
		if err != nil {
			s.recordFailure(ctx, operation, err)
			// An outage will not improve for the next payment in the list.
			if errors.Is(err, lknpd.ErrServiceUnavailable) {
				return settled, nil
			}
			continue
		}
		if done {
			settled++
		}
	}
	return settled, nil
}

func (s *Service) settle(ctx context.Context, session *lknpd.Session, operation Operation) (bool, error) {
	settlement, err := s.repo.Settlement(ctx, operation.PaymentID)
	if err != nil {
		return false, err
	}
	if settlement.Unresolved {
		return false, errors.New(
			"предыдущая попытка не узнала, создан ли чек; закрыть вручную после проверки в «Мой налог»")
	}

	steps, err := Plan(settlement.PaidMinor, settlement.RefundedMinor, settlement.Active)
	if err != nil {
		return false, err
	}
	if len(steps) == 0 {
		return true, s.repo.OperationDone(ctx, operation.ID)
	}

	for _, step := range steps {
		switch step.Kind {
		case StepCancel:
			if err := s.withSession(ctx, session, func(active lknpd.Session) error {
				return s.adapter.CancelReceipt(ctx, active, step.ReceiptUUID, s.now())
			}); err != nil {
				return false, err
			}
			if err := s.repo.CancelReceipt(ctx, settlement.Active.RowID, s.now()); err != nil {
				return false, err
			}

		case StepCreate:
			// The row is written before the call. A crash after ФНС accepts
			// but before we learn the identifier leaves this row behind, and
			// it blocks a second attempt until a person checks - because two
			// receipts for one payment is the failure this must never have.
			rowID, err := s.repo.BeginReceipt(ctx, settlement.PaymentID, step.AmountMinor)
			if err != nil {
				return false, err
			}
			var uuid, printURL string
			if err := s.withSession(ctx, session, func(active lknpd.Session) error {
				var callErr error
				uuid, printURL, callErr = s.adapter.CreateReceipt(
					ctx, active, s.serviceName, step.AmountMinor, settlement.PaidAt)
				return callErr
			}); err != nil {
				return false, err
			}
			if err := s.repo.FinishReceipt(ctx, rowID, uuid, printURL); err != nil {
				return false, err
			}
		}
	}

	return true, s.repo.OperationDone(ctx, operation.ID)
}

// withSession runs a call and, if the session turns out to be dead, renews it
// once and tries again. Once: an unofficial API answers a login loop with a
// CAPTCHA, and then nothing works until a person intervenes.
func (s *Service) withSession(ctx context.Context, session *lknpd.Session, call func(lknpd.Session) error) error {
	err := call(*session)
	if !errors.Is(err, lknpd.ErrUnauthorized) {
		return err
	}
	renewed, renewErr := s.renew(ctx, *session)
	if renewErr != nil {
		return renewErr
	}
	*session = renewed
	return call(renewed)
}

func (s *Service) recordFailure(ctx context.Context, operation Operation, cause error) {
	// One email per payment, on the first failure. Later attempts keep
	// retrying quietly: a mailbox filling with the same payment is how the one
	// that matters gets missed.
	alerted := operation.Alerted
	if !alerted && s.alerter != nil {
		settlement, err := s.repo.Settlement(ctx, operation.PaymentID)
		if err == nil {
			queued, _ := s.repo.PendingCount(ctx)
			if alertErr := s.alerter.ReceiptFailed(ctx, settlement, cause.Error(), queued); alertErr == nil {
				alerted = true
			} else {
				s.log.Error("не удалось отправить письмо о неудавшемся чеке", "error", alertErr)
			}
		}
	}
	if err := s.repo.OperationFailed(ctx, operation.ID, cause.Error(), alerted); err != nil {
		s.log.Error("не удалось записать неудачу по чеку", "error", err)
	}
}

// session hands back a usable session, renewing or signing in as needed.
//
// Two ways in exist and the token pair is the primary one. Nothing below this
// function knows which was used: creating, cancelling and reconciling receipts
// are the same work either way, and that is the point of the split.
func (s *Service) session(ctx context.Context) (lknpd.Session, error) {
	stored, err := s.repo.LoadSession(ctx, s.creds.DeviceID)
	if err != nil {
		return lknpd.Session{}, err
	}
	if stored.AccessToken != "" && !s.expired(stored) {
		return s.withINN(ctx, stored)
	}
	renewed, err := s.renew(ctx, stored)
	if err != nil {
		return lknpd.Session{}, err
	}
	return s.withINN(ctx, renewed)
}

// withINN fills in the taxpayer identifier when only a token pair was
// configured.
//
// The refresh response carries no profile, so a deployment that has never
// signed in with a password does not know its own ИНН - and the printable
// address of every receipt is built from it. Asked once, then stored.
func (s *Service) withINN(ctx context.Context, session lknpd.Session) (lknpd.Session, error) {
	if session.INN != "" {
		return session, nil
	}
	inn, err := s.adapter.Profile(ctx, session)
	if err != nil {
		return lknpd.Session{}, err
	}
	if inn == "" {
		return lknpd.Session{}, errors.New("npd: ФНС не назвала ИНН")
	}
	session.INN = inn
	return session, s.repo.SaveSession(ctx, session)
}

// renew gets a working session, preferring the token pair.
//
// The order is the design. A stored refresh token comes first because ФНС
// rotates them: the value in the environment is a seed, and after the first
// renewal the current one is the stored one. Falling back to the configured
// token when the stored one is refused is what makes a replaced pair take
// effect without a database edit. The password comes last and only if it is
// configured at all, which is the requirement that the older way keeps working
// and stays untouched.
func (s *Service) renew(ctx context.Context, stored lknpd.Session) (lknpd.Session, error) {
	tried := ""

	if stored.RefreshToken != "" {
		tried = stored.RefreshToken
		renewed, err := s.adapter.Refresh(ctx, stored)
		if err == nil {
			return renewed, s.repo.SaveSession(ctx, renewed)
		}
		if errors.Is(err, lknpd.ErrServiceUnavailable) {
			// Not a credentials problem. Trying another way in now would be
			// one more request at a service that is already unwell, and
			// repeated sign-ins to an unofficial API earn a CAPTCHA.
			return lknpd.Session{}, err
		}
		s.log.Warn("сохранённый refresh token ФНС не принят")
	}

	// The configured pair. Skipped when it is the one that just failed.
	if s.creds.RefreshToken != "" && s.creds.RefreshToken != tried {
		seed := lknpd.Session{
			INN:          stored.INN,
			DeviceID:     s.deviceID(stored),
			RefreshToken: s.creds.RefreshToken,
		}
		renewed, err := s.adapter.Refresh(ctx, seed)
		if err == nil {
			return renewed, s.repo.SaveSession(ctx, renewed)
		}
		if errors.Is(err, lknpd.ErrServiceUnavailable) {
			return lknpd.Session{}, err
		}
		s.log.Warn("настроенный refresh token ФНС не принят")
	}

	if s.creds.INN == "" || s.creds.Password == "" {
		// No password configured. Saying so plainly, because the alternative
		// is a generic authorisation error that sends somebody looking at the
		// tax service for a problem that is in our own configuration.
		return lknpd.Session{}, fmt.Errorf(
			"%w: refresh token не принят, а вход по паролю не настроен", lknpd.ErrUnauthorized)
	}

	deviceID := s.deviceID(stored)
	if deviceID == "" {
		return lknpd.Session{}, errors.New("npd: нет идентификатора устройства для входа")
	}
	fresh, err := s.adapter.Login(ctx, s.creds.INN, s.creds.Password, deviceID)
	if err != nil {
		return lknpd.Session{}, err
	}
	return fresh, s.repo.SaveSession(ctx, fresh)
}

// deviceID prefers the configured one. A refresh token is issued for a device
// and is worthless beside any other identifier, so a configured pair travels
// together and a generated identifier is never substituted for it.
func (s *Service) deviceID(stored lknpd.Session) string {
	if s.creds.DeviceID != "" {
		return s.creds.DeviceID
	}
	return stored.DeviceID
}

func (s *Service) expired(session lknpd.Session) bool {
	if session.ExpiresAt.IsZero() {
		return false
	}
	// A minute of margin: a token that expires while the request is in flight
	// costs a round trip and a retry.
	return !s.now().Add(time.Minute).Before(session.ExpiresAt)
}
