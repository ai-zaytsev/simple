package store

import (
	"testing"
	"time"
)

func at(now time.Time, ago time.Duration) *time.Time {
	t := now.Add(-ago)
	return &t
}

func f(v float64) *float64 { return &v }

func TestNodeVerdict(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name string
		node NodeHealth
		want string
	}{
		{"never reported", NodeHealth{}, "silent"},
		{"stopped talking", NodeHealth{LastSeen: at(now, 10 * time.Minute)}, "silent"},
		{"late once is not silent", NodeHealth{
			LastSeen: at(now, 90 * time.Second), CPUPercent: f(10),
		}, "ok"},
		{"losing packets", NodeHealth{
			LastSeen: at(now, time.Minute), LossPercent: f(12),
		}, "degraded"},
		{"slow upstream", NodeHealth{
			LastSeen: at(now, time.Minute), LatencyMS: f(400),
		}, "degraded"},
		{"working hard", NodeHealth{
			LastSeen: at(now, time.Minute), CPUPercent: f(92),
		}, "busy"},
		{"out of memory", NodeHealth{
			LastSeen: at(now, time.Minute), MemoryPercent: f(95),
		}, "busy"},
		{"quiet and healthy", NodeHealth{
			LastSeen: at(now, time.Minute), CPUPercent: f(4), Load1: f(0.2),
			MemoryPercent: f(30), LatencyMS: f(12), LossPercent: f(0),
		}, "ok"},
		// Loss matters more than being busy: a node dropping packets is failing
		// its users, and a node at 90% CPU that delivers everything is not.
		{"busy and losing packets is degraded", NodeHealth{
			LastSeen: at(now, time.Minute), CPUPercent: f(95), LossPercent: f(9),
		}, "degraded"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := nodeVerdict(c.node, now); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// TestEndpointVerdict is the rule that separates "our server is broken" from
// "somebody is keeping people away from it".
//
// Worth a test of its own because the two look identical from Helsinki, and
// telling them apart is the entire reason the same address is measured twice.
func TestEndpointVerdict(t *testing.T) {
	cases := []struct {
		name     string
		endpoint EndpointHealth
		want     string
	}{
		{"everything fine", EndpointHealth{
			FromUs: 100, FromDevices: 99, DeviceChecks: 200,
		}, "works"},

		{"we can reach it, devices cannot", EndpointHealth{
			FromUs: 100, FromDevices: 4, DeviceChecks: 150,
		}, "likely blocked"},

		{"nobody can reach it", EndpointHealth{
			FromUs: 0, FromDevices: 0, DeviceChecks: 150,
		}, "unreachable"},

		{"devices struggling", EndpointHealth{
			FromUs: 100, FromDevices: 61, DeviceChecks: 80,
		}, "slower"},

		// Two device reports are not evidence. One phone on a bad train is not
		// a blocked domain, and calling it one would have somebody rotating
		// domains over a tunnel.
		{"too few device reports to conclude", EndpointHealth{
			FromUs: 100, FromDevices: 0, DeviceChecks: 2,
		}, "works"},

		{"no device reports at all", EndpointHealth{
			FromUs: 100, FromDevices: -1, DeviceChecks: 0,
		}, "works"},

		{"only we have looked, and it is down", EndpointHealth{
			FromUs: 10, FromDevices: -1, DeviceChecks: 0,
		}, "unreachable"},

		{"answers, but slowly", EndpointHealth{
			FromUs: 100, FromDevices: -1, DeviceChecks: 0, LatencyMS: f(2200),
		}, "slower"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := endpointVerdict(c.endpoint); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}
