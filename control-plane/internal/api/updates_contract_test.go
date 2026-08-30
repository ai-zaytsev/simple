package api

import (
	"os"
	"strings"
	"testing"
)

func TestUnsupportedVersionIsRejectedBeforeVPNMaterial(t *testing.T) {
	source, err := os.ReadFile("api.go")
	if err != nil {
		t.Fatal(err)
	}
	plan := between(string(source), "func (s *Server) plan", "\nfunc ")
	verdict := strings.Index(plan, "state.AppUpdates.Verdict(req.AppVersion)")
	credential := strings.Index(plan, "EnsureCredential")
	issued := strings.Index(plan, "s.issue(")
	if verdict < 0 {
		t.Fatal("plan does not check the application update policy")
	}
	if credential < 0 || verdict > credential {
		t.Error("unsupported build reaches credential issuance before it is refused")
	}
	if issued < 0 || verdict > issued {
		t.Error("unsupported build can reach plan issuance before it is refused")
	}
	if !strings.Contains(plan, "http.StatusUpgradeRequired") {
		t.Error("plan refusal is not distinguishable as an upgrade requirement")
	}
}

func TestConfigKeepsLegacyMinimumAndAddsOnePolicy(t *testing.T) {
	source, err := os.ReadFile("api.go")
	if err != nil {
		t.Fatal(err)
	}
	config := between(string(source), "func (s *Server) config", "\nfunc ")
	for _, want := range []string{
		"MinSupportedAppVersion: state.MinSupportedAppVersion",
		"Update:                 state.AppUpdates",
	} {
		if !strings.Contains(config, want) {
			t.Errorf("signed config is missing %q", want)
		}
	}
}

func TestUpdateChangesAreOperatorOnly(t *testing.T) {
	source, err := os.ReadFile("updates.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, handler := range []string{"adminUpdates", "adminPublishUpdate", "adminMinimumUpdate"} {
		body := between(string(source), "func (s *Server) "+handler, "\nfunc ")
		if !strings.Contains(body, "if !s.admin(w, r)") {
			t.Errorf("%s is not protected by operator authentication", handler)
		}
	}
}

func TestAPKPublicationCanRetryCoreSynchronizationSafely(t *testing.T) {
	source, err := os.ReadFile("../../../.github/workflows/publish-apk.yml")
	if err != nil {
		t.Fatal(err)
	}
	publish := string(source)
	verify := strings.Index(publish, "name: Verify through the official domain")
	advance := strings.Index(publish, "name: Advance Core latest after public verification")
	if verify < 0 || advance < 0 || verify > advance {
		t.Fatal("Core latest can advance before the public APK is verified")
	}
	for _, contract := range []string{
		"already_published=true",
		"selected_sha256",
		"Public APK signing certificate mismatch",
		"steps.manifest.outputs.selected_sha256",
	} {
		if !strings.Contains(publish, contract) {
			t.Errorf("publication retry contract is missing %q", contract)
		}
	}
}
