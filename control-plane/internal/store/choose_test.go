package store

import (
	"fmt"
	"testing"
	"time"

	"download.simplevpn/control-plane/internal/document"
)

func healthy(alias string, now time.Time) NodeStanding {
	seen := now.Add(-30 * time.Second)
	cpu, mem, latency, loss := 10.0, 30.0, 20.0, 0.0
	return NodeStanding{
		Node:              document.Node{Alias: alias},
		ServerName:        alias + ".invalid",
		Capacity:          500,
		LastSeen:          &seen,
		CPUPercent:        &cpu,
		MemoryPercent:     &mem,
		Sessions:          10,
		LossPercent:       &loss,
		UpstreamLatencyMS: &latency,
		DomainVerdict:     "works",
	}
}

func TestUsable(t *testing.T) {
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	stale := now.Add(-10 * time.Minute)

	cases := []struct {
		name string
		make func(NodeStanding) NodeStanding
		want bool
	}{
		{"healthy", func(s NodeStanding) NodeStanding { return s }, true},
		{"never reported", func(s NodeStanding) NodeStanding {
			s.LastSeen = nil
			return s
		}, false},
		{"stopped reporting", func(s NodeStanding) NodeStanding {
			s.LastSeen = &stale
			return s
		}, false},
		{"domain unreachable for devices", func(s NodeStanding) NodeStanding {
			s.DomainVerdict = "unreachable"
			return s
		}, false},
		{"domain likely blocked", func(s NodeStanding) NodeStanding {
			s.DomainVerdict = "likely blocked"
			return s
		}, false},
		// Slow is a reason to prefer somewhere else, not a reason to refuse.
		// Refusing every slow node during a bad hour would leave nowhere.
		{"domain slow", func(s NodeStanding) NodeStanding {
			s.DomainVerdict = "slower"
			return s
		}, true},
		{"nothing has checked the domain yet", func(s NodeStanding) NodeStanding {
			s.DomainVerdict = ""
			return s
		}, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.make(healthy("n-1", now)).Usable(now); got != c.want {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestRoomIsTheWorstPressure(t *testing.T) {
	now := time.Now()

	// A node comfortable everywhere except memory is not a comfortable node.
	// Averaging the three would hide exactly the one about to matter.
	full := healthy("n-1", now)
	mem := 96.0
	full.MemoryPercent = &mem

	if room := full.Room(); room > 0.05 {
		t.Errorf("a node at 96%% memory reports %.2f room; the worst pressure is the room", room)
	}

	// Sessions count too, against what the node was sized for. A node with
	// idle processors and every connection slot taken has no room.
	crowded := healthy("n-2", now)
	crowded.Sessions = 500
	if room := crowded.Room(); room != 0 {
		t.Errorf("a node at its connection limit reports %.2f room", room)
	}
}

func TestQualityFallsWithLatencyLossAndErrors(t *testing.T) {
	now := time.Now()
	base := healthy("n-1", now).Quality(0)

	slow := healthy("n-2", now)
	high := 400.0
	slow.UpstreamLatencyMS = &high
	if slow.Quality(0) >= base {
		t.Error("a node twenty times slower scores no worse")
	}

	lossy := healthy("n-3", now)
	loss := 20.0
	lossy.LossPercent = &loss
	if lossy.Quality(0) >= base {
		t.Error("a node losing a fifth of its packets scores no worse")
	}

	failing := healthy("n-4", now)
	failing.Attempts, failing.Successes = 100, 40
	if failing.Quality(0) >= base {
		t.Error("a node devices fail to reach scores no worse")
	}

	// A handful of attempts is not a failure rate. Judging on four would let
	// one person on a train take a node out of service for everybody.
	unlucky := healthy("n-5", now)
	unlucky.Attempts, unlucky.Successes = 4, 1
	if unlucky.Quality(0) != base {
		t.Error("four attempts were treated as evidence")
	}
}

func TestRankPutsUnusableLast(t *testing.T) {
	now := time.Now()

	dead := healthy("n-dead", now)
	dead.LastSeen = nil

	order := Rank([]NodeStanding{dead, healthy("n-live", now)}, "device-1", now)
	if order[0].Node.Alias != "n-live" {
		t.Errorf("a node that has never reported was offered first: %v", order[0].Node.Alias)
	}

	// Last rather than absent. The client probes before it commits and moves
	// on by itself; a device with an empty plan has nothing to move on to.
	if len(order) != 2 {
		t.Fatalf("a bad node was dropped from the plan entirely: %d left", len(order))
	}
}

func TestRankIsStableForOneDevice(t *testing.T) {
	now := time.Now()
	nodes := []NodeStanding{healthy("n-1", now), healthy("n-2", now), healthy("n-3", now)}

	first := Rank(nodes, "device-1", now)
	for i := 0; i < 20; i++ {
		again := Rank(nodes, "device-1", now)
		for j := range first {
			if first[j].Node.Alias != again[j].Node.Alias {
				t.Fatal("one device asking twice got a different order; every refresh would reconnect it")
			}
		}
	}
}

// TestRankSpreadsAcrossDevices is the property that keeps the chooser from
// causing the outage it exists to prevent.
//
// If every device were told the same best node, the best node would become the
// busiest and then the worst, and the whole fleet would move to the next one
// together. The order has to differ between devices while still favouring the
// better node.
func TestRankSpreadsAcrossDevices(t *testing.T) {
	now := time.Now()

	roomy := healthy("n-roomy", now)
	roomy.Sessions = 10

	tight := healthy("n-tight", now)
	tight.Sessions = 400 // four fifths of its capacity

	firsts := map[string]int{}
	const devices = 2000
	for i := 0; i < devices; i++ {
		order := Rank([]NodeStanding{roomy, tight}, fmt.Sprintf("device-%d", i), now)
		firsts[order[0].Node.Alias]++
	}

	if firsts["n-tight"] == 0 {
		t.Error("every device was sent to the same node; the busiest node will stay busiest")
	}
	if firsts["n-roomy"] <= firsts["n-tight"] {
		t.Errorf("the node with more room was not preferred: roomy %d, tight %d",
			firsts["n-roomy"], firsts["n-tight"])
	}

	// And the preference should be pronounced, not a coin flip with a lean.
	if firsts["n-roomy"] < devices*6/10 {
		t.Errorf("the better node was chosen only %d times out of %d", firsts["n-roomy"], devices)
	}
}

func TestVerdictFromRate(t *testing.T) {
	cases := []struct {
		rate float64
		want string
	}{
		{-1, ""},
		{0, "unreachable"},
		{29, "unreachable"},
		{50, "slower"},
		{89, "slower"},
		{95, "works"},
		{100, "works"},
	}
	for _, c := range cases {
		if got := verdictFromRate(c.rate); got != c.want {
			t.Errorf("rate %.0f gave %q, want %q", c.rate, got, c.want)
		}
	}
}

// TestMeasuringANodeDoesNotPunishIt is the bug this rule was written wrong
// for, found on the panel an hour after it went live.
//
// A device measuring a domain measures a whole handshake - connect, TLS,
// upgrade - which on a real phone was averaging 535ms while the node's own
// round trip was 1.1ms. Both were being judged against the same threshold, so
// the only node anybody had actually checked scored 0.119 while the node
// nobody had checked scored 0.629. Being verified made a node worse.
func TestMeasuringANodeDoesNotPunishIt(t *testing.T) {
	now := time.Now()

	// Two identical nodes. One has been checked by devices and answers in the
	// time a handshake takes; the other has never been checked.
	checked := healthy("n-checked", now)
	handshake := 535.0
	checked.DomainLatencyMS = &handshake
	checked.DomainVerdict = "works"

	unchecked := healthy("n-unchecked", now)
	unchecked.DomainVerdict = ""

	standings := []NodeStanding{checked, unchecked}
	typical := TypicalHandshake(standings)

	if checked.Score(typical) < unchecked.Score(typical) {
		t.Errorf(
			"a node proven to work scores %.3f against %.3f for one nobody has checked; "+
				"measuring a node must not be what makes it look bad",
			checked.Score(typical), unchecked.Score(typical))
	}
}

// TestHandshakeIsJudgedAgainstTheOtherNodes checks the replacement rule: a
// node is compared with its peers rather than with a number somebody chose.
func TestHandshakeIsJudgedAgainstTheOtherNodes(t *testing.T) {
	now := time.Now()

	usual, twice := 500.0, 1000.0
	ordinary := healthy("n-ordinary", now)
	ordinary.DomainLatencyMS = &usual
	expensive := healthy("n-expensive", now)
	expensive.DomainLatencyMS = &twice

	standings := []NodeStanding{ordinary, expensive}
	typical := TypicalHandshake(standings)

	if expensive.Score(typical) >= ordinary.Score(typical) {
		t.Error("a node twice as expensive to reach as its peers was not preferred against")
	}

	// And a whole fleet being slow penalises nobody: the question a chooser
	// answers is which of these, not whether any of them is good.
	slowUsual, alsoSlow := 5000.0, 5000.0
	a := healthy("n-a", now)
	a.DomainLatencyMS = &slowUsual
	b := healthy("n-b", now)
	b.DomainLatencyMS = &alsoSlow

	slowFleet := []NodeStanding{a, b}
	if a.Score(TypicalHandshake(slowFleet)) != b.Score(TypicalHandshake(slowFleet)) {
		t.Error("two equally slow nodes were scored differently")
	}
}
