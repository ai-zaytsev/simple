package npd

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"

	"download.simplevpn/control-plane/internal/npd/lknpd"
)

func serviceWith(t *testing.T, repo *fakeRepo, adapter *fakeAdapter, creds Credentials) *Service {
	t.Helper()
	s, err := NewService(repo, adapter, &fakeAlerter{}, creds, "Simple VPN",
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestTheConfiguredPairIsUsedWhenNothingIsStored(t *testing.T) {
	repo := newRepo()
	repo.session = lknpd.Session{}
	adapter := &fakeAdapter{}

	service := serviceWith(t, repo, adapter, Credentials{
		RefreshToken: "seed-token", DeviceID: "device-from-fns",
	})
	if _, err := service.CheckAvailability(context.Background()); err != nil {
		t.Fatal(err)
	}

	if adapter.refreshes != 1 {
		t.Fatalf("expected the configured pair to be used, refreshes=%d", adapter.refreshes)
	}
	if adapter.logins != 0 {
		t.Fatal("no password sign-in should happen when a token pair works")
	}
	if len(adapter.refreshSeen) != 1 || adapter.refreshSeen[0] != "seed-token@device-from-fns" {
		t.Fatalf("the token must travel with its own device id, saw %v", adapter.refreshSeen)
	}
}

func TestTheDeviceIdFromThePairIsUsedAndNotGenerated(t *testing.T) {
	// A refresh token is issued for a device. Generating an identifier beside
	// it would make the token useless, and the failure would then look like a
	// bad token rather than a bad pairing - which is a much longer evening.
	repo := newRepo()
	adapter := &fakeAdapter{}

	service := serviceWith(t, repo, adapter, Credentials{
		RefreshToken: "seed", DeviceID: "device-from-fns",
	})
	if _, err := service.CheckAvailability(context.Background()); err != nil {
		t.Fatal(err)
	}
	if repo.askedDeviceID != "device-from-fns" {
		t.Fatalf("the store was asked for %q", repo.askedDeviceID)
	}
}

func TestARotatedRefreshTokenIsStoredAndUsedNext(t *testing.T) {
	// ФНС rotates these. The value in the environment is a seed; after the
	// first renewal the current one is the stored one, and going back to the
	// seed would present a token that has already been spent.
	repo := newRepo()
	repo.session = lknpd.Session{DeviceID: "device-from-fns"}
	adapter := &fakeAdapter{rotateTo: "rotated-token"}

	service := serviceWith(t, repo, adapter, Credentials{
		RefreshToken: "seed", DeviceID: "device-from-fns",
	})
	if _, err := service.CheckAvailability(context.Background()); err != nil {
		t.Fatal(err)
	}
	if repo.session.RefreshToken != "rotated-token" {
		t.Fatalf("the rotated token was not stored: %q", repo.session.RefreshToken)
	}

	adapter.refreshSeen = nil
	repo.session.AccessToken = "" // force another renewal
	if _, err := service.CheckAvailability(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(adapter.refreshSeen) == 0 || adapter.refreshSeen[0] != "rotated-token@device-from-fns" {
		t.Fatalf("the stored token must be preferred over the seed, saw %v", adapter.refreshSeen)
	}
}

func TestARefusedStoredTokenFallsBackToTheConfiguredOne(t *testing.T) {
	// This is what lets a replaced pair take effect without editing the
	// database by hand.
	repo := newRepo()
	repo.session = lknpd.Session{DeviceID: "device-from-fns", RefreshToken: "stale"}
	adapter := &fakeAdapter{refuseToken: "stale"}

	service := serviceWith(t, repo, adapter, Credentials{
		RefreshToken: "fresh", DeviceID: "device-from-fns",
	})
	if _, err := service.CheckAvailability(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(adapter.refreshSeen) != 2 {
		t.Fatalf("expected the stale one then the configured one, saw %v", adapter.refreshSeen)
	}
	if adapter.refreshSeen[1] != "fresh@device-from-fns" {
		t.Fatalf("the configured token was not tried, saw %v", adapter.refreshSeen)
	}
}

func TestThePasswordWayStillWorksOnItsOwn(t *testing.T) {
	// The requirement is that the older way is neither changed nor removed.
	repo := newRepo()
	repo.session = lknpd.Session{DeviceID: "generated"}
	adapter := &fakeAdapter{}

	service := serviceWith(t, repo, adapter, Credentials{
		INN: "123456789012", Password: "secret",
	})
	if _, err := service.CheckAvailability(context.Background()); err != nil {
		t.Fatal(err)
	}
	if adapter.logins != 1 {
		t.Fatalf("expected one password sign-in, got %d", adapter.logins)
	}
	if repo.askedDeviceID != "" {
		t.Fatalf("no device id was configured, so none should be demanded: %q", repo.askedDeviceID)
	}
}

func TestARevokedPairFallsBackToThePassword(t *testing.T) {
	repo := newRepo()
	repo.session = lknpd.Session{DeviceID: "device-from-fns"}
	adapter := &fakeAdapter{refuseToken: "revoked"}

	service := serviceWith(t, repo, adapter, Credentials{
		RefreshToken: "revoked", DeviceID: "device-from-fns",
		INN: "123456789012", Password: "secret",
	})
	if _, err := service.CheckAvailability(context.Background()); err != nil {
		t.Fatal(err)
	}
	if adapter.logins != 1 {
		t.Fatalf("the older way must catch a revoked pair, logins=%d", adapter.logins)
	}
}

func TestARevokedPairWithNoPasswordSaysWhichSideIsWrong(t *testing.T) {
	// Otherwise somebody spends the morning looking at the tax service for a
	// problem that is in our own configuration.
	repo := newRepo()
	repo.session = lknpd.Session{DeviceID: "device-from-fns"}
	adapter := &fakeAdapter{refuseToken: "revoked"}

	service := serviceWith(t, repo, adapter, Credentials{
		RefreshToken: "revoked", DeviceID: "device-from-fns",
	})
	if _, err := service.CheckAvailability(context.Background()); err != nil {
		t.Fatal(err)
	}
	if repo.availabilityOK {
		t.Fatal("selling must be closed")
	}
	if !strings.Contains(repo.availabilityWhy, "вход по паролю не настроен") {
		t.Fatalf("the reason must name the configuration: %q", repo.availabilityWhy)
	}
}

func TestAnOutageDoesNotBurnThroughEveryWayIn(t *testing.T) {
	// Repeated sign-ins to an unofficial API earn a CAPTCHA, and a service
	// that is down will not answer the second attempt any better than the
	// first. An outage is a reason to stop, not to try harder.
	repo := newRepo()
	repo.session = lknpd.Session{DeviceID: "device-from-fns", RefreshToken: "stored"}
	adapter := &fakeAdapter{refreshErr: fmt.Errorf("%w: работы", lknpd.ErrServiceUnavailable)}

	service := serviceWith(t, repo, adapter, Credentials{
		RefreshToken: "configured", DeviceID: "device-from-fns",
		INN: "123456789012", Password: "secret",
	})
	if _, err := service.CheckAvailability(context.Background()); err != nil {
		t.Fatal(err)
	}
	if adapter.refreshes != 1 {
		t.Fatalf("an outage must stop after the first attempt, refreshes=%d", adapter.refreshes)
	}
	if adapter.logins != 0 {
		t.Fatal("an outage must not turn into a password sign-in")
	}
}

func TestTheTaxpayerIdentifierIsLearnedWhenOnlyAPairIsConfigured(t *testing.T) {
	// The refresh response carries no profile, and every receipt's printable
	// address is built from the ИНН. Asked once, then stored.
	repo := newRepo()
	repo.session = lknpd.Session{DeviceID: "device-from-fns"}
	adapter := &fakeAdapter{}

	service := serviceWith(t, repo, adapter, Credentials{
		RefreshToken: "seed", DeviceID: "device-from-fns",
	})
	if _, err := service.CheckAvailability(context.Background()); err != nil {
		t.Fatal(err)
	}
	if adapter.profileCalls != 1 {
		t.Fatalf("expected one profile read, got %d", adapter.profileCalls)
	}
	if repo.session.INN != "123456789012" {
		t.Fatalf("the identifier was not stored: %q", repo.session.INN)
	}

	if _, err := service.CheckAvailability(context.Background()); err != nil {
		t.Fatal(err)
	}
	if adapter.profileCalls != 1 {
		t.Fatalf("the identifier should be asked for once, got %d", adapter.profileCalls)
	}
}

func TestNeitherWayConfiguredIsRefusedAtStartup(t *testing.T) {
	_, err := NewService(newRepo(), &fakeAdapter{}, nil, Credentials{}, "",
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err == nil {
		t.Fatal("a tax module with no way in would only fail after taking money")
	}
}

func TestAPairAloneIsEnoughToStart(t *testing.T) {
	// An account signed in through Госуслуги has no lknpd password at all.
	_, err := NewService(newRepo(), &fakeAdapter{}, nil,
		Credentials{RefreshToken: "t", DeviceID: "d"}, "",
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("a token pair is a complete way in: %v", err)
	}
}

func TestAPasswordAloneIsStillEnoughToStart(t *testing.T) {
	_, err := NewService(newRepo(), &fakeAdapter{}, nil,
		Credentials{INN: "123456789012", Password: "secret"}, "",
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("the older way must still stand on its own: %v", err)
	}
}
