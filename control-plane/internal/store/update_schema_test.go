package store

import (
	"regexp"
	"testing"
)

func TestUpdatePolicyStartsAtTheFirstBuild(t *testing.T) {
	schema := allMigrations(t)
	for _, pattern := range []string{
		`(?is)'latest_version_code',\s*1`,
		`(?is)'latest_version_name',\s*'0\.1\.0'`,
		`(?is)'app_updates'`,
	} {
		if !regexp.MustCompile(pattern).MatchString(schema) {
			t.Errorf("update migration does not match %s", pattern)
		}
	}
}

func TestMinimumIsKeptForBinaryRollback(t *testing.T) {
	source := readSource(t, "updates.go")
	writing := between(source, "func (s *Store) SetMinSupportedAppVersion", "\nfunc ")
	if !regexp.MustCompile(`update service_state[\s\S]*key = 'min_supported_app_version'`).MatchString(writing) {
		t.Error("raising the minimum does not update the row an older Core binary reads")
	}
}
