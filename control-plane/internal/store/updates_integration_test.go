package store

import (
	"context"
	"errors"
	"os"
	"testing"

	"download.simplevpn/control-plane/internal/appupdate"
)

func TestAppUpdateLifecycleOnPostgres(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	st, err := Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	first := AppRelease{
		VersionCode: 1,
		VersionName: "0.1.0",
		Channel:     appupdate.DirectAPK,
		Artifact: appupdate.Artifact{
			URL:    "https://simple-vpn.download/download/releases/0.1.0/simple-vpn-0.1.0.apk",
			SHA256: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		},
	}
	if policy, err := st.PublishAppRelease(ctx, first, "first-release"); err != nil ||
		policy.Channels[appupdate.DirectAPK] != first.Artifact {
		t.Fatalf("first artifact: policy=%+v err=%v", policy, err)
	}

	release := AppRelease{
		VersionCode: 2,
		VersionName: "0.2.0",
		Channel:     appupdate.DirectAPK,
		Artifact: appupdate.Artifact{
			URL:    "https://simple-vpn.download/download/releases/0.2.0/simple-vpn-0.2.0.apk",
			SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
	}
	policy, err := st.PublishAppRelease(ctx, release, "update-test")
	if err != nil || policy.Verdict(1) != appupdate.Optional {
		t.Fatalf("publication: policy=%+v err=%v", policy, err)
	}
	if _, err := st.PublishAppRelease(ctx, release, "update-test-retry"); err != nil {
		t.Fatalf("identical publication is not idempotent: %v", err)
	}
	policy, err = st.SetMinSupportedAppVersion(ctx, 2, "update-test")
	if err != nil || policy.Verdict(1) != appupdate.Required || policy.Verdict(2) != appupdate.Current {
		t.Fatalf("minimum: policy=%+v err=%v", policy, err)
	}
	policy, err = st.SetMinSupportedAppVersion(ctx, 1, "incident-recovery")
	if err != nil || policy.Verdict(1) != appupdate.Optional {
		t.Fatalf("minimum relaxation: policy=%+v err=%v", policy, err)
	}
	if _, err := st.PublishAppRelease(ctx, AppRelease{
		VersionCode: 1,
		VersionName: "0.1.0",
		Channel:     appupdate.DirectAPK,
		Artifact:    release.Artifact,
	}, "rollback"); !errors.Is(err, ErrUpdateRollback) {
		t.Fatalf("latest rollback got %v", err)
	}

	state, err := st.LoadServiceState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if state.MinSupportedAppVersion != 1 || state.AppUpdates.MinSupportedVersionCode != 1 {
		t.Fatalf("legacy and policy minimum differ: %+v", state)
	}
}
