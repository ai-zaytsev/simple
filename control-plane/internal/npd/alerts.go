package npd

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// Channel is where a message goes. The same shape as internal/alert uses, so
// that email, a messenger and a log are interchangeable here too.
type Channel interface {
	Name() string
	Send(ctx context.Context, subject, body string) error
}

// MailAlerter writes the two letters this stage needs.
//
// It does not decide whether to speak - the service does that, and it decides
// once per payment rather than once per attempt. Rendering lives here so that
// the wording of a letter somebody acts on at seven in the morning is in one
// place and not spread through the logic that produced it.
type MailAlerter struct {
	Channels []Channel
	Log      *slog.Logger
}

// ReceiptFailed is the letter that lets Business Owner issue a receipt by hand
// the same day.
//
// Every field the stage asks for is here, and each one is here because without
// it the letter cannot be acted on: which payment, whose, how much, when, what
// went wrong, and what the queue is doing about it.
func (m MailAlerter) ReceiptFailed(ctx context.Context, s Settlement, reason string, queued int) error {
	owed := s.PaidMinor - s.RefundedMinor

	var b strings.Builder
	b.WriteString("Чек НПД не создан. Платёж прошёл, VIP выдан, деньги у нас.\n")
	b.WriteString("Оформить чек вручную в «Мой налог» сегодня.\n\n")
	fmt.Fprintf(&b, "Payment ID:      %s\n", s.PaymentID)
	fmt.Fprintf(&b, "Пользователь:    %s\n", personOf(s))
	fmt.Fprintf(&b, "Сумма:           %s\n", rubles(owed))
	fmt.Fprintf(&b, "Время платежа:   %s\n", moscow(s.PaidAt))
	fmt.Fprintf(&b, "Ошибка НПД:      %s\n", reason)
	fmt.Fprintf(&b, "Статус очереди:  %s\n", queueState(queued))

	if s.RefundedMinor > 0 {
		fmt.Fprintf(&b, "\nПо платежу возвращено %s из %s, поэтому чек нужен на остаток.\n",
			rubles(s.RefundedMinor), rubles(s.PaidMinor))
	}
	if s.Unresolved {
		b.WriteString("\nВНИМАНИЕ: предыдущая попытка не узнала, создан ли чек. " +
			"Сначала проверить в «Мой налог», есть ли уже чек по этому платежу, " +
			"иначе можно выпустить второй.\n")
	}
	b.WriteString("\nПосле ручного оформления закрыть операцию через workflow " +
		"Tax Receipts, иначе очередь останется незакрытой и продажи не откроются.\n")

	return m.send(ctx, "Simple VPN: чек НПД не создан", b.String())
}

// TaxServiceDown is the letter that explains why nothing is being sold.
func (m MailAlerter) TaxServiceDown(ctx context.Context, reason string, queued int) error {
	var b strings.Builder
	b.WriteString("Новые продажи VIP заблокированы: чек выдать нечем.\n")
	b.WriteString("Действующие VIP продолжают работать, ничего не отключено.\n\n")
	fmt.Fprintf(&b, "Причина:         %s\n", reason)
	fmt.Fprintf(&b, "Статус очереди:  %s\n", queueState(queued))
	fmt.Fprintf(&b, "Проверено:       %s\n", moscow(time.Now()))
	b.WriteString("\nСледующая проверка — завтра в 06:00 МСК. " +
		"Если ФНС восстановится раньше, продажи откроются на этой проверке, " +
		"но только когда очередь чеков будет пуста.\n")

	return m.send(ctx, "Simple VPN: продажи закрыты, ФНС недоступна", b.String())
}

func (m MailAlerter) send(ctx context.Context, subject, body string) error {
	if len(m.Channels) == 0 {
		return nil
	}
	var failures []string
	for _, channel := range m.Channels {
		if err := channel.Send(ctx, subject, body); err != nil {
			failures = append(failures, channel.Name())
			if m.Log != nil {
				m.Log.Error("не удалось отправить письмо о чеках",
					"channel", channel.Name(), "error", err)
			}
		}
	}
	if len(failures) == len(m.Channels) {
		return fmt.Errorf("ни один канал не принял письмо: %s", strings.Join(failures, ", "))
	}
	return nil
}

// personOf names the account without spreading an address further than it has
// to go. The letter goes to Business Owner, who needs to find the customer;
// the account identifier does that, and the address is there because a support
// reply needs it.
func personOf(s Settlement) string {
	if s.AccountEmail == "" {
		return s.AccountID
	}
	return fmt.Sprintf("%s (%s)", s.AccountEmail, s.AccountID)
}

func rubles(minor int64) string {
	return fmt.Sprintf("%d,%02d ₽", minor/100, minor%100)
}

func moscow(at time.Time) string {
	if at.IsZero() {
		return "неизвестно"
	}
	return at.In(time.FixedZone("MSK", 3*60*60)).Format("2006-01-02 15:04 МСК")
}

func queueState(queued int) string {
	switch {
	case queued <= 0:
		return "операция осталась в очереди, повтор автоматический"
	case queued == 1:
		return "в очереди 1 операция, повтор автоматический"
	default:
		return fmt.Sprintf("в очереди %d операций, повтор автоматический", queued)
	}
}
