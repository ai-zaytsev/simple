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
		Handler:           api.New(st, signer, cleaned, planTTL, sender, baseURL, deriver, issuer, testIssuer, adminToken, nodeCapacity, log).Routes(),
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
