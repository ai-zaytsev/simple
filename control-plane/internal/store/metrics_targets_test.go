package store

import (
	"strings"
	"testing"
)

// The two allowlists are intentionally different. Core's ordinary HTTPS
// prober cannot test an IP with a separate SNI or an edge with a path prefix;
// Android can, and its report still has to be constrained to addresses we own.
func TestDeviceReportsMayNameEveryEnabledBootstrapEntry(t *testing.T) {
	source := readStoreSource(t, "metrics.go")

	device := between(source, "func (s *Store) DeviceReportTargets", "\nfunc ")
	if device == "" {
		t.Fatal("device report target allowlist is missing")
	}
	if !strings.Contains(device, "from bootstrap_entries") || !strings.Contains(device, "where enabled") {
		t.Error("device reports are not allowlisted from enabled bootstrap entries")
	}
	if strings.Contains(device, "kind = 'https-direct'") {
		t.Error("IP and edge entries are still excluded from device reports")
	}

	self := between(source, "func (s *Store) ServedNames", "\nfunc ")
	if !strings.Contains(self, "kind = 'https-direct'") {
		t.Error("Core's plain HTTPS prober was widened to transports it cannot test correctly")
	}
}
