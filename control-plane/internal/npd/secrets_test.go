package npd

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The stage forbids a password, an access token, a refresh token or a session
// reaching code, git, documentation or a log. The first three are a matter of
// not writing them down; the log is the one that happens by accident, in a
// helpful error message added months later.
//
// So this reads the package's own source. A rule that lives only in a review
// comment is a rule that holds until the reviewer is busy.
func TestNothingSecretIsEverLogged(t *testing.T) {
	logCall := regexp.MustCompile(`(?s)\b(s\.log|m\.Log|log)\.(Debug|Info|Warn|Error)\(.*?\)`)
	forbidden := []string{
		"Password", "AccessToken", "RefreshToken", "creds", "session",
	}

	for _, dir := range []string{".", "lknpd"} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			name := entry.Name()
			if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			body, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			for _, call := range logCall.FindAllString(string(body), -1) {
				for _, word := range forbidden {
					if strings.Contains(call, word) {
						t.Errorf("%s/%s logs %s:\n  %s", dir, name, word, call)
					}
				}
			}
		}
	}
}

// A password reaching an error is the same leak by a slower route: errors are
// wrapped, returned, and eventually written somewhere.
func TestCredentialsNeverTravelInAnError(t *testing.T) {
	body, err := os.ReadFile("service.go")
	if err != nil {
		t.Fatal(err)
	}
	errorCall := regexp.MustCompile(`(?s)(errors\.New|fmt\.Errorf)\(.*?\)`)
	for _, call := range errorCall.FindAllString(string(body), -1) {
		for _, word := range []string{"creds.Password", "creds.INN", "AccessToken", "RefreshToken"} {
			if strings.Contains(call, word) {
				t.Errorf("an error carries %s:\n  %s", word, call)
			}
		}
	}
}

// The adapter must not put a response body into an error either. A maintenance
// page or a CAPTCHA is large, useless in a log, and may carry anything.
func TestTheAdapterDoesNotEchoBodies(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("lknpd", "client.go"))
	if err != nil {
		t.Fatal(err)
	}
	source := string(body)
	if strings.Contains(source, "string(raw)") {
		t.Error("the raw response body must not be turned into a message")
	}
	if strings.Contains(source, "%s\", raw") || strings.Contains(source, "%v\", raw") {
		t.Error("the raw response body must not be formatted into an error")
	}
}
