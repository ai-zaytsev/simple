// Command core is the Control Plane.
//
// It answers three questions and nothing else: where to connect, how the
// application should behave, and where to find this service next time. Every
// answer is signed, because a client that would accept an unsigned answer can
// be sent anywhere by anyone able to answer first.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"download.simplevpn/control-plane/internal/alert"
	"download.simplevpn/control-plane/internal/analytics"
	"download.simplevpn/control-plane/internal/api"
	"download.simplevpn/control-plane/internal/certs"
	"download.simplevpn/control-plane/internal/dnsedit"
	"download.simplevpn/control-plane/internal/mail"
	"download.simplevpn/control-plane/internal/npd"
	"download.simplevpn/control-plane/internal/npd/lknpd"
	"download.simplevpn/control-plane/internal/payment"
	"download.simplevpn/control-plane/internal/payment/yookassa"
	"download.simplevpn/control-plane/internal/probe"
	"download.simplevpn/control-plane/internal/signing"
	"download.simplevpn/control-plane/internal/store"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	if err := run(log); err != nil {
		log.Error("stopped", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	// Everything the service needs arrives through the environment, which is
	// what lets the same build run on any machine. Nothing here has a default
	// that would let it start half-configured and fail later, when a device is
	// already waiting for an answer.
	dsn := os.Getenv("CP_DATABASE_URL")
	if dsn == "" {
		return errors.New("CP_DATABASE_URL is not set")
	}

	keyID := os.Getenv("CP_SIGNING_KEY_ID")
	seed := os.Getenv("CP_SIGNING_SEED")
	if keyID == "" || seed == "" {
		return errors.New("CP_SIGNING_KEY_ID and CP_SIGNING_SEED are required")
	}

	hosts := strings.Split(os.Getenv("CP_BOOTSTRAP_HOSTS"), ",")
	cleaned := hosts[:0]
	for _, host := range hosts {
		if trimmed := strings.TrimSpace(host); trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	if len(cleaned) == 0 {
		return errors.New("CP_BOOTSTRAP_HOSTS is empty; clients would be told nowhere to look")
	}

	addr := os.Getenv("CP_LISTEN")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}

	// How long a plan stays usable. Short while a replacement is being rolled
	// out, long the rest of the time.
	planTTL := 24 * time.Hour
	if raw := os.Getenv("CP_PLAN_TTL"); raw != "" {
		parsed, parseErr := time.ParseDuration(raw)
		if parseErr != nil {
			return fmt.Errorf("CP_PLAN_TTL is not a duration: %w", parseErr)
		}
		if parsed < time.Minute {
			return errors.New("CP_PLAN_TTL below a minute would make every connection wait on this service")
		}
		planTTL = parsed
	}

	// How often our own addresses are checked. Configurable because the right
	// answer changes with circumstances: minutes while something is being
	// blocked and we are watching it happen, longer the rest of the time, when
	// every check is a request somebody could count.
	probeEvery := 5 * time.Minute
	if raw := os.Getenv("CP_PROBE_EVERY"); raw != "" {
		parsed, parseErr := time.ParseDuration(raw)
		if parseErr != nil {
			return fmt.Errorf("CP_PROBE_EVERY is not a duration: %w", parseErr)
		}
		if parsed < time.Minute {
			return errors.New("CP_PROBE_EVERY below a minute would make us the busiest visitor our own sites have")
		}
		probeEvery = parsed
	}

	// Sending mail is not optional: without it nobody can sign in, and a
	// service that starts anyway would fail one person at a time instead of
	// once, loudly, at deploy.
	brevoKey := os.Getenv("CP_BREVO_API_KEY")
	senderAddress := os.Getenv("CP_MAIL_FROM")
	if brevoKey == "" || senderAddress == "" {
		return errors.New("CP_BREVO_API_KEY and CP_MAIL_FROM are required")
	}
	sender := mail.NewSender(brevoKey, senderAddress, "Simple VPN")

	baseURL := os.Getenv("CP_BASE_URL")
	if baseURL == "" {
		return errors.New("CP_BASE_URL is not set; sign-in links would point nowhere")
	}

	// Measurement gets a key derived from the account, never the account. The
	// key is required rather than optional because a missing one would leave
	// the obvious thing - logging the account itself - as the path of least
	// resistance, and that is exactly what must not happen.
	analyticsEpoch := 30 * 24 * time.Hour
	if raw := os.Getenv("CP_ANALYTICS_EPOCH"); raw != "" {
		parsed, parseErr := time.ParseDuration(raw)
		if parseErr != nil {
			return fmt.Errorf("CP_ANALYTICS_EPOCH is not a duration: %w", parseErr)
		}
		analyticsEpoch = parsed
	}
	deriver, err := analytics.NewDeriver(os.Getenv("CP_ANALYTICS_KEY"), analyticsEpoch)
	if err != nil {
		return fmt.Errorf("CP_ANALYTICS_KEY: %w", err)
	}

	// Certificates are issued centrally so that a node never holds a key to
	// our DNS. Optional rather than required: a service that cannot issue is
	// still a service, and refusing to start would turn a missing token into
	// an outage of everything else.
	var issuer, testIssuer *certs.Issuer
	if accountKey := os.Getenv("CP_ACME_KEY"); accountKey != "" {
		editor := dnsedit.New(
			os.Getenv("CP_DNS_CLOUDFLARE_TOKEN"),
			os.Getenv("CP_DNS_SPACESHIP_KEY"),
			os.Getenv("CP_DNS_SPACESHIP_SECRET"),
		)
		issuer, err = certs.New(accountKey, os.Getenv("CP_ACME_DIRECTORY"), editor, log)
		if err != nil {
			return fmt.Errorf("CP_ACME_KEY: %w", err)
		}

		// The same key against the authority whose certificates nobody trusts,
		// for rehearsal nodes. One key registered at two directories becomes
		// two separate accounts, which is exactly right: their allowances are
		// counted separately, so proving the whole path from nothing to a
		// working node costs none of the real one.
		staging := os.Getenv("CP_ACME_TEST_DIRECTORY")
		if staging == "" {
			staging = "https://acme-staging-v02.api.letsencrypt.org/directory"
		}
		testIssuer, err = certs.New(accountKey, staging, editor, log)
		if err != nil {
			return fmt.Errorf("CP_ACME_KEY against the test authority: %w", err)
		}

		log.Info("certificate issuance is available", "test_authority", staging)
	} else {
		log.Warn("no ACME account key; nodes cannot be issued certificates")
	}

	signer, err := signing.NewSigner(keyID, seed)
	if err != nil {
		return err
	}
	// The public half is printed at every start, so that what the server signs
	// with can always be compared against what the application trusts without
	// touching the private half.
	log.Info("signing key loaded", "key_id", signer.KeyID(), "public_key", signer.PublicKeyB64())

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	st, err := store.Open(ctx, dsn)
	if err != nil {
		return err
	}
	defer st.Close()

	// Before anything is served, and not by hand beforehand. A binary that is
	// running is a binary whose schema is already what it expects.
	applied, err := st.Migrate(ctx)
	if err != nil {
		return err
	}
	if len(applied) > 0 {
		log.Info("schema updated", "applied", strings.Join(applied, ","))
	}

	// Payment credentials exist only here. The Android application asks this
	// service for a checkout and therefore never needs a shop identifier, a
	// secret key or even the name of the configured provider.
	provider, err := yookassa.New(
		os.Getenv("CP_YOOKASSA_SHOP_ID"),
		os.Getenv("CP_YOOKASSA_SECRET_KEY"),
		nil,
	)
	if err != nil {
		return fmt.Errorf("CP_YOOKASSA_SHOP_ID and CP_YOOKASSA_SECRET_KEY: %w", err)
	}
	returnURL := os.Getenv("CP_PAYMENT_RETURN_URL")
	if returnURL == "" {
		returnURL = "https://simple-syncbridge.download/v1/payments/return"
	}
	payments, err := payment.NewService(st, provider, returnURL)
	if err != nil {
		return fmt.Errorf("payment service: %w", err)
	}

	// Moscow, because both the tax service and the working day are there. A
	// fixed offset rather than a named zone: the container carries no tzdata,
	// and Moscow has not observed daylight saving since 2014.
	moscow := time.FixedZone("MSK", 3*60*60)

	// Tax receipts.
	//
	// Built here, next to payments, because they are one obligation with two
	// halves: money in, receipt out. The dependency on an unofficial API is
	// the whole reason npd and lknpd are separate packages - what is likely to
	// break is at the bottom, behind one interface, and nothing above it knows
	// the difference between lknpd.nalog.ru and its replacement.
	//
	// A missing ИНН or password is fatal rather than a warning. Selling
	// without being able to file is the one state this stage exists to
	// prevent, and starting quietly in it would hide that.
	npdChannels := []npd.Channel{}
	if to := os.Getenv("CP_ALERT_EMAIL"); to != "" {
		npdChannels = append(npdChannels, npd.Email{Sender: sender, To: to})
	} else {
		log.Warn("no CP_ALERT_EMAIL; failed tax receipts will only be logged")
	}
	receipts, err := npd.NewService(
		st,
		lknpd.New(nil, moscow),
		npd.MailAlerter{Channels: npdChannels, Log: log},
		npd.Credentials{
			// The primary way in. An account signed in through Госуслуги has
			// no lknpd password at all, so a pair is a complete answer on its
			// own - and the pair travels together, because the token belongs
			// to the device it was issued for.
			RefreshToken: os.Getenv("CP_NPD_REFRESH_TOKEN"),
			DeviceID:     os.Getenv("CP_NPD_DEVICE_ID"),

			// The older way, kept and unchanged. It catches a revoked pair,
			// and it is still the whole answer for an account that has a
			// password.
			INN:      os.Getenv("CP_NPD_INN"),
			Password: os.Getenv("CP_NPD_PASSWORD"),
		},
		os.Getenv("CP_NPD_SERVICE_NAME"),
		log,
	)
	if err != nil {
		return fmt.Errorf(
			"CP_NPD_REFRESH_TOKEN with CP_NPD_DEVICE_ID, or CP_NPD_INN with CP_NPD_PASSWORD: %w",
			err)
	}

	// The queue, worked through often. This is not polling ФНС: it runs only
	// when something is owed, and an outage stops the pass rather than
	// repeating it.
	go func() {
		ticker := time.NewTicker(2 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				settled, err := receipts.Drain(ctx, 50)
				if err != nil {
					log.Error("cannot work through the receipt queue", "error", err)
				} else if settled > 0 {
					log.Info("tax receipts settled", "payments", settled)
				}
			}
		}
	}()

	// The one full check a day, at 06:00 Moscow, and the only thing that
	// decides whether selling is allowed.
	//
	// A minute ticker rather than a daily timer: a daily timer restarts with
	// the process, so a deploy at 05:59 would push the check to tomorrow. This
	// asks "has 06:00 passed since the last check" of the database, which
	// survives restarts and cannot fire twice.
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				availability, err := st.TaxAvailability(ctx)
				if err != nil {
					log.Error("cannot read tax availability", "error", err)
					continue
				}
				if !dueForCheck(time.Now().In(moscow), availability.CheckedAt) {
					continue
				}
				allowed, err := receipts.CheckAvailability(ctx)
				if err != nil {
					log.Error("cannot check the tax service", "error", err)
					continue
				}
				log.Info("tax service checked", "sales_allowed", allowed)
			}
		}
	}()

	// A paid tier is access, not a label. Expiry therefore runs before the
	// service answers and continuously afterwards; each pass also revokes
	// credentials no longer allowed by FREE.
	expired, err := st.ExpireVIPs(ctx)
	if err != nil {
		return err
	}
	if expired > 0 {
		log.Info("paid VIP expired", "accounts", expired)
	}
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				count, expireErr := st.ExpireVIPs(ctx)
				if expireErr != nil {
					log.Error("cannot expire paid VIP", "error", expireErr)
				} else if count > 0 {
					log.Info("paid VIP expired", "accounts", count)
				}
			}
		}
	}()

	// Successful refunds have a webhook; canceled ones do not. Polling only
	// our own unresolved rows also recovers a lost create response even when
	// Android never opens again, while a provider outage leaves VIP untouched.
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				completed, refundErr := payments.ReconcileRefunds(ctx, 100)
				if refundErr != nil {
					log.Error("cannot reconcile pending refunds")
				} else if completed > 0 {
					log.Info("pending refunds reached a final state", "refunds", completed)
				}
			}
		}
	}()

	// Our own addresses are checked from here, and the same addresses are
	// checked by devices. Neither reads user traffic to find out whether a way
	// in still works, which is the point: the sensor is a test of our own, not
	// a record of where people went.
	go probe.New(st, probeEvery, log).Run(ctx)

	// Minute rows are the bulk and stop being useful once a day has passed.
	// Kept for five weeks so that a month can be compared with the one before
	// it; the daily summaries stay for thirteen.
	go probe.NewHousekeeper(st, 35*24*time.Hour, 400*24*time.Hour, log).Run(ctx)

	// How many tunnel connections a node is taken to be sized for when it does
	// not say so itself. Used to work out how much room is left before the
	// numbers get bad, which is too late to find out by watching them get bad.
	nodeCapacity := 500
	if raw := os.Getenv("CP_NODE_CAPACITY"); raw != "" {
		parsed, parseErr := strconv.Atoi(raw)
		if parseErr != nil || parsed <= 0 {
			return errors.New("CP_NODE_CAPACITY must be a positive number of connections")
		}
		nodeCapacity = parsed
	}

	// Empty means the panel is not served at all. A dashboard is the one place
	// where every number in this system is visible at once, so it is closed
	// unless somebody has deliberately opened it.
	adminToken := os.Getenv("CP_ADMIN_TOKEN")
	if adminToken == "" {
		log.Warn("no CP_ADMIN_TOKEN; the panel is not available")
	}

	// Where a capacity warning goes. The channel list is built here and nowhere
	// else, so adding a messenger later is one line beside this one and no
	// change at all to what decided to warn.
	channels := []alert.Channel{alert.Logged{Log: log}}
	if to := os.Getenv("CP_ALERT_EMAIL"); to != "" {
		channels = append(channels, alert.Email{Sender: sender, To: to})
		log.Info("capacity warnings will be emailed")
	} else {
		log.Warn("no CP_ALERT_EMAIL; capacity warnings will only be logged")
	}

	go probe.NewCapacityWatch(
		st,
		alert.New(st, 12*time.Hour, log, channels...),
		10*time.Minute, nodeCapacity, log,
	).Run(ctx)

	server := &http.Server{
		Addr:              addr,
		Handler:           api.New(st, signer, cleaned, planTTL, sender, baseURL, deriver, issuer, testIssuer, adminToken, nodeCapacity, payments, log).Routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		<-ctx.Done()
		// A shutdown that hangs is a deploy that hangs. The bounded context
		// makes the worst case a slow restart rather than a stuck one.
		shutdown, done := context.WithTimeout(context.Background(), 10*time.Second)
		defer done()
		_ = server.Shutdown(shutdown)
	}()

	log.Info("listening", "addr", addr, "bootstrap_hosts", strings.Join(cleaned, ","), "plan_ttl", planTTL.String())

	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// dueForCheck says whether the once-a-day tax check is owed.
//
// The rule is "06:00 Moscow has passed and we have not checked since", not "it
// is 06:00 now". A service that was restarting at six would otherwise skip the
// day entirely, and the day it skips is the day nobody notices sales are open
// with no way to file a receipt.
func dueForCheck(now time.Time, lastCheck *time.Time) bool {
	sixThisMorning := time.Date(now.Year(), now.Month(), now.Day(), 6, 0, 0, 0, now.Location())
	if now.Before(sixThisMorning) {
		return false
	}
	if lastCheck == nil {
		return true
	}
	return lastCheck.In(now.Location()).Before(sixThisMorning)
}
