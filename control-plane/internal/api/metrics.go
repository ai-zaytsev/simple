package api

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"download.simplevpn/control-plane/internal/auth"
	"download.simplevpn/control-plane/internal/store"
)

// A node's report is larger than anything else this service accepts: it can
// carry a couple of hours of buffered windows after a network problem. Still
// bounded, because an unbounded body is an unbounded allocation.
const maxMetricsBytes = 256 << 10

// nodeSample is what a node says about itself for one window.
//
// Every field is a number or a name of ours. There is no field here for a
// destination, and adding one would be a visible change to a struct whose
// whole point is that it has none.
type nodeSample struct {
	At               int64              `json:"at"`
	WindowSeconds    int                `json:"window_s"`
	UsersConfigured  *int               `json:"users_configured"`
	SessionsOnline   *int               `json:"sessions_online"`
	CPUPercent       *float64           `json:"cpu_percent"`
	Load1            *float64           `json:"load1"`
	MemoryPercent    *float64           `json:"memory_percent"`
	Established      *int               `json:"established"`
	UplinkBytes      int64              `json:"uplink_bytes"`
	DownlinkBytes    int64              `json:"downlink_bytes"`
	UpstreamLatency  *float64           `json:"upstream_latency_ms"`
	UpstreamLoss     *float64           `json:"upstream_loss_percent"`
	Goroutines       *int               `json:"goroutines"`
	HeapBytes        *int64             `json:"heap_bytes"`
	XrayUptimeS      *int64             `json:"xray_uptime_s"`
	CredentialBytes  map[string]int64   `json:"credential_bytes"`

	// Two sets of class counters, because the node routes the heavier accounts
	// through a parallel set of outbounds. Neither set carries a user: the node
	// was handed a list of credentials to send the other way and told nothing
	// about what the list means.
	ClassBytes      map[string]direction `json:"class_bytes"`
	HeavyClassBytes map[string]direction `json:"heavy_class_bytes"`
}

type direction struct {
	Up   int64 `json:"up"`
	Down int64 `json:"down"`
}

// nodeMetrics takes a node's report and turns the half of it that is about
// people into something that is not.
//
// The credential totals arrive here, are resolved to accounts, become epoch
// pseudonyms, and are added to a monthly figure. Nothing in between is
// written down. This function is the anonymisation point the privacy model
// describes, and it is deliberately the only one.
func (s *Server) nodeMetrics(w http.ResponseWriter, r *http.Request) {
	token := bearer(r)
	if token == "" {
		writeError(w, http.StatusUnauthorized, "node is not identified")
		return
	}
	alias, err := s.store.NodeByToken(r.Context(), auth.HashToken(token))
	if err != nil {
		writeError(w, http.StatusUnauthorized, "node is not identified")
		return
	}

	var body struct {
		Samples []nodeSample `json:"samples"`
	}
	limited := http.MaxBytesReader(w, r.Body, maxMetricsBytes)
	if err := json.NewDecoder(limited).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "report could not be read")
		return
	}

	for _, sample := range body.Samples {
		at := time.Unix(sample.At, 0).UTC().Truncate(time.Second)
		if at.IsZero() || at.After(time.Now().Add(5*time.Minute)) {
			continue
		}

		if err := s.store.RecordNodeSample(r.Context(), alias, store.NodeSample{
			At:              at,
			WindowSeconds:   sample.WindowSeconds,
			UsersConfigured: sample.UsersConfigured,
			SessionsOnline:  sample.SessionsOnline,
			CPUPercent:      sample.CPUPercent,
			Load1:           sample.Load1,
			MemoryPercent:   sample.MemoryPercent,
			Established:     sample.Established,
			UplinkBytes:     sample.UplinkBytes,
			DownlinkBytes:   sample.DownlinkBytes,
			UpstreamLatency: sample.UpstreamLatency,
			UpstreamLoss:    sample.UpstreamLoss,
			Goroutines:      sample.Goroutines,
			HeapBytes:       sample.HeapBytes,
			XrayUptimeS:     sample.XrayUptimeS,
		}); err != nil {
			s.log.Error("cannot store a node sample", "node", alias, "error", err)
			writeError(w, http.StatusInternalServerError, "report could not be stored")
			return
		}

		classes := make([]store.ClassBytes, 0, len(sample.ClassBytes)+len(sample.HeavyClassBytes))
		for name, bytes := range sample.ClassBytes {
			classes = append(classes, store.ClassBytes{
				Class: name, Cohort: "ordinary", Uplink: bytes.Up, Downlink: bytes.Down,
			})
		}
		for name, bytes := range sample.HeavyClassBytes {
			classes = append(classes, store.ClassBytes{
				Class: name, Cohort: "heavy", Uplink: bytes.Up, Downlink: bytes.Down,
			})
		}
		if err := s.store.RecordTrafficClasses(r.Context(), alias, at, classes); err != nil {
			s.log.Error("cannot store traffic classes", "node", alias, "error", err)
		}

		s.foldUsage(r, at, sample.CredentialBytes)
	}

	// The answer carries which credentials the node should route the other
	// way. It is the whole conversation about cohorts: the node learns a list,
	// never what the list means, and never how it was arrived at.
	heavy, err := s.heavyCredentials(r)
	if err != nil {
		s.log.Error("cannot work out the heavy set", "error", err)
		heavy = []string{}
	}

	// And which are held to a speed, with the figure. Same arrangement: a list
	// and a number, with nothing said about tiers, prices or people.
	capped, speed, err := s.store.LimitedCredentials(r.Context())
	if err != nil {
		s.log.Error("cannot work out the capped set", "error", err)
		capped, speed = []string{}, 0
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":     "ok",
		"heavy":      heavy,
		"limited":    capped,
		"speed_mbit": speed,
	})
}

// heavyCredentials is the set of credentials belonging to the heaviest
// accounts this period.
//
// Computed here rather than queried, because the two halves of the schema are
// not joinable and must not become so: usage is keyed by an epoch pseudonym,
// credentials are keyed by account, and the only thing that can hold both at
// once is a running function. It holds them for the length of this call.
//
// Empty while there are too few accounts to speak of groups at all. That is
// the honest answer at this size, and it is also the safe one: a "heavy
// cohort" of one is a person.
func (s *Server) heavyCredentials(r *http.Request) ([]string, error) {
	s.heavyOnce.Lock()
	defer s.heavyOnce.Unlock()

	if time.Since(s.heavyAt) < time.Minute && s.heavySet != nil {
		return s.heavySet, nil
	}

	now := time.Now().UTC()
	period := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)

	above, people, err := s.store.HeavinessThreshold(r.Context(), period)
	if err != nil {
		return nil, err
	}

	owners, err := s.store.CredentialOwners(r.Context())
	if err != nil {
		return nil, err
	}

	heavy := []string{}
	if above > 0 {
		pseudonyms, err := s.store.HeavyPseudonyms(r.Context(), period, above)
		if err != nil {
			return nil, err
		}
		for credential, account := range owners {
			if pseudonyms[s.analytics.ID(account, period)] {
				heavy = append(heavy, credential)
			}
		}
	}
	sort.Strings(heavy)

	// Group sizes, kept alongside the traffic so that a share is never read
	// without knowing how many people it is a share of.
	ordinary := people - len(heavy)
	if ordinary < 0 {
		ordinary = 0
	}
	if err := s.store.RecordCohortSizes(r.Context(), now.Truncate(time.Hour), len(heavy), ordinary); err != nil {
		s.log.Error("cannot record cohort sizes", "error", err)
	}

	s.heavySet, s.heavyAt = heavy, time.Now()
	return heavy, nil
}

// foldUsage turns credentials into pseudonyms and adds up the month.
func (s *Server) foldUsage(r *http.Request, at time.Time, byCredential map[string]int64) {
	if len(byCredential) == 0 {
		return
	}

	ids := make([]uuid.UUID, 0, len(byCredential))
	for raw := range byCredential {
		if parsed, err := uuid.Parse(raw); err == nil {
			ids = append(ids, parsed)
		}
	}

	accounts, err := s.store.AccountsForCredentials(r.Context(), ids)
	if err != nil {
		s.log.Error("cannot resolve credentials", "error", err)
		return
	}

	// The period's first day, used both as the row key and as the moment the
	// pseudonym is derived from. Deriving it from "now" instead would split a
	// person's month across two keys the day the epoch turns over.
	period := time.Date(at.Year(), at.Month(), 1, 0, 0, 0, 0, time.UTC)
	hour := at.Truncate(time.Hour)

	totals := map[string]int64{}
	for raw, bytes := range byCredential {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			continue
		}
		account, ok := accounts[parsed]
		if !ok {
			continue
		}
		totals[s.analytics.ID(account, period)] += bytes
		if bytes > 0 {
			if err := s.store.MarkActive(r.Context(), hour, s.analytics.ID(account, period)); err != nil {
				s.log.Error("cannot mark activity", "error", err)
			}
		}
	}

	for id, bytes := range totals {
		if bytes == 0 {
			continue
		}
		if err := s.store.AddAccountUsage(r.Context(), period, id, bytes); err != nil {
			s.log.Error("cannot add usage", "error", err)
		}
	}
}

// appReport is what a device says about reaching us.
//
// Already summed by the device over its own reporting window. There is no
// per-attempt record and no timestamp per event, so this cannot be unwound
// into one person's pattern of use even by whoever holds the database.
type appReport struct {
	NodeAlias      string `json:"node_alias"`
	EntryKind      string `json:"entry_kind"`
	Attempts       int    `json:"attempts"`
	Successes      int    `json:"successes"`
	Reconnects     int    `json:"reconnects"`
	SessionSeconds int64  `json:"session_seconds"`
	LatencySumMS   int64  `json:"latency_ms_sum"`
	LatencySamples int    `json:"latency_samples"`

	// What the device found when it tried our own addresses. This is the
	// sensor for blocking: it is the one vantage point that sits where the
	// blocking happens.
	Probes []struct {
		Target    string `json:"target"`
		OK        bool   `json:"ok"`
		LatencyMS *int   `json:"latency_ms"`
	} `json:"probes"`
}

func (s *Server) appReport(w http.ResponseWriter, r *http.Request) {
	device, ok := s.device(w, r)
	if !ok {
		return
	}

	var body appReport
	if err := decode(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "report could not be read")
		return
	}

	now := time.Now().UTC()
	if body.Attempts > 0 || body.SessionSeconds > 0 || body.Reconnects > 0 {
		if err := s.store.RecordConnectReport(r.Context(), now, store.ConnectReport{
			NodeAlias:      trim(body.NodeAlias, 32),
			EntryKind:      trim(body.EntryKind, 32),
			Attempts:       body.Attempts,
			Successes:      body.Successes,
			Reconnects:     body.Reconnects,
			SessionSeconds: body.SessionSeconds,
			LatencySumMs:   body.LatencySumMS,
			LatencySamples: body.LatencySamples,
		}); err != nil {
			s.log.Error("cannot store a connect report", "error", err)
		}
	}

	// A device that has been used at all counts towards active users, and the
	// pseudonym is all that is kept of who it was.
	period := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	if body.SessionSeconds > 0 {
		if err := s.store.MarkActive(r.Context(), now.Truncate(time.Hour),
			s.analytics.ID(device.AccountID, period)); err != nil {
			s.log.Error("cannot mark activity", "error", err)
		}
	}

	served := s.servedNames(r)
	for _, probe := range body.Probes {
		// Refused rather than stored if it is not one of ours. This is what
		// keeps the single domain column in the schema from becoming a list of
		// places people go: a device can report on our addresses and on
		// nothing else, however it is modified.
		if !served[strings.ToLower(strings.TrimSpace(probe.Target))] {
			continue
		}
		if err := s.store.RecordProbe(r.Context(), now,
			strings.ToLower(probe.Target), "device", probe.OK, probe.LatencyMS, ""); err != nil {
			s.log.Error("cannot store a probe", "error", err)
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// servedNames is the set of addresses this service will accept a report about.
//
// Cached briefly because it is asked on every device report and changes when a
// node is added, which is not often.
func (s *Server) servedNames(r *http.Request) map[string]bool {
	s.namesOnce.Lock()
	defer s.namesOnce.Unlock()

	if time.Since(s.namesAt) < time.Minute && s.names != nil {
		return s.names
	}
	list, err := s.store.ServedNames(r.Context())
	if err != nil {
		s.log.Error("cannot list served names", "error", err)
		if s.names != nil {
			return s.names
		}
		return map[string]bool{}
	}
	set := map[string]bool{}
	for _, name := range list {
		set[strings.ToLower(name)] = true
	}
	s.names, s.namesAt = set, time.Now()
	return set
}

// adminOverview answers the panel.
func (s *Server) adminOverview(w http.ResponseWriter, r *http.Request) {
	if !s.admin(w, r) {
		return
	}
	overview, err := s.store.Overview(r.Context(), time.Now().UTC())
	if err != nil {
		s.log.Error("cannot build the overview", "error", err)
		writeError(w, http.StatusInternalServerError, "cannot read the numbers")
		return
	}
	sort.Slice(overview.Nodes, func(i, j int) bool {
		return overview.Nodes[i].Alias < overview.Nodes[j].Alias
	})
	w.Header().Set("cache-control", "no-store")
	writeJSON(w, http.StatusOK, overview)
}

// admin gates the panel behind a secret configured on the server.
//
// Compared in constant time, and absent means closed: a service with no admin
// secret shows nothing rather than showing everybody.
func (s *Server) admin(w http.ResponseWriter, r *http.Request) bool {
	if s.adminToken == "" {
		writeError(w, http.StatusNotFound, "not found")
		return false
	}
	if !auth.SameSecret(bearer(r), s.adminToken) {
		writeError(w, http.StatusUnauthorized, "not authorised")
		return false
	}
	return true
}

func trim(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) > limit {
		return value[:limit]
	}
	return value
}

// namesGuard keeps the served-name cache from being read while it is replaced.
type namesGuard = sync.Mutex
