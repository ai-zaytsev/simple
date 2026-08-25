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
	"strings"
	"syscall"
	"time"

	"download.simplevpn/control-plane/internal/api"
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

	server := &http.Server{
		Addr:              addr,
		Handler:           api.New(st, signer, cleaned, planTTL, log).Routes(),
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
