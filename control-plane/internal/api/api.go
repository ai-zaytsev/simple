// Package api serves the three documents a client asks for.
package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"download.simplevpn/control-plane/internal/document"
	"download.simplevpn/control-plane/internal/signing"
	"download.simplevpn/control-plane/internal/store"
)

type Server struct {
	store  *store.Store
	signer *signing.Signer
	log    *slog.Logger

	// bootstrapHosts is where clients are told to look for this service. It is
	// configuration rather than a constant so that moving the Control Plane
	// does not require rebuilding it.
	bootstrapHosts []string
}

func New(st *store.Store, signer *signing.Signer, hosts []string, log *slog.Logger) *Server {
	return &Server{store: st, signer: signer, bootstrapHosts: hosts, log: log}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("POST /v1/plan", s.plan)
	mux.HandleFunc("GET /v1/config", s.config)
	mux.HandleFunc("GET /v1/bootstrap", s.bootstrap)
	return mux
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type planRequest struct {
	DeviceID            string   `json:"device_id"`
	AccountID           string   `json:"account_id"`
	SupportedTransports []string `json:"supported_transports"`
	AppVersion          int      `json:"app_version"`
}

// plan issues a connection plan for one device.
//
// The transport is chosen from what the client says it can speak, and a client
// that speaks nothing we serve gets a refusal rather than a plan it cannot
// use. In the MVP there is one transport, so this is trivial - and it exists
// anyway, because introducing negotiation later would require every
// installation to update at the same moment.
func (s *Server) plan(w http.ResponseWriter, r *http.Request) {
	var req planRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBytes)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "request could not be read")
		return
	}
	if req.DeviceID == "" || req.AccountID == "" {
		writeError(w, http.StatusBadRequest, "device and account are required")
		return
	}

	ctx := r.Context()
	kind := s.chooseTransport(req.SupportedTransports)
	if kind == "" {
		writeError(w, http.StatusConflict, "no transport in common")
		return
	}

	nodes, err := s.store.ActiveNodes(ctx, kind)
	if err != nil {
		s.log.Error("cannot build a plan", "error", err)
		writeError(w, http.StatusServiceUnavailable, "no endpoint available")
		return
	}

	if err := s.store.TouchDevice(ctx, req.DeviceID, req.AccountID); err != nil {
		// Not fatal. A plan that works matters more than a row recording that
		// it was handed out, and refusing here would turn a bookkeeping
		// failure into an outage.
		s.log.Warn("could not record the device", "error", err)
	}

	scope := "plan:" + req.AccountID
	seq, err := s.store.NextSeq(ctx, scope)
	if err != nil {
		s.log.Error("cannot advance the sequence", "error", err)
		writeError(w, http.StatusInternalServerError, "cannot issue a plan")
		return
	}

	now := time.Now().UTC()
	plan := document.Plan{
		V:           1,
		PlanID:      uuid.NewString(),
		Seq:         seq,
		IssuedAt:    now.Format(time.RFC3339),
		ExpiresAt:   now.Add(planLifetime).Format(time.RFC3339),
		AccountTier: "FREE",
		Primary:     nodes[0],
		Reserves:    reserves(nodes),
		DNS: document.DNS{
			Mode:    "tunnel",
			Servers: []string{"https://1.1.1.1/dns-query", "https://8.8.8.8/dns-query"},
		},
		Routing: document.Routing{Profile: "default-ru", Rules: []any{}},
		Policy: document.Policy{
			ConnectTimeoutMS:     8000,
			FailoverAfterFailure: 2,
			ReconnectBackoffMS:   []int{1000, 3000, 8000, 20000},
			PlanRefreshAfterS:    43200,
			TelemetrySampling:    1.0,
			ProbeIntervalS:       60,
		},
	}

	s.issue(w, r.Context(), "plan", scope, seq, plan)
}

func (s *Server) config(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	seq, err := s.store.NextSeq(ctx, "config")
	if err != nil {
		s.log.Error("cannot advance the sequence", "error", err)
		writeError(w, http.StatusInternalServerError, "cannot issue configuration")
		return
	}

	cfg := document.Config{
		V:                      1,
		Seq:                    seq,
		IssuedAt:               time.Now().UTC().Format(time.RFC3339),
		MinSupportedAppVersion: 1,
		KillSwitch:             document.KillSwitch{Enabled: false},
		Features:               map[string]bool{"config_export_enabled": false},
	}

	s.issue(w, ctx, "config", "config", seq, cfg)
}

func (s *Server) bootstrap(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	seq, err := s.store.NextSeq(ctx, "bootstrap")
	if err != nil {
		s.log.Error("cannot advance the sequence", "error", err)
		writeError(w, http.StatusInternalServerError, "cannot issue a descriptor")
		return
	}

	entries := make([]document.BootstrapEntry, 0, len(s.bootstrapHosts))
	for _, host := range s.bootstrapHosts {
		entries = append(entries, document.BootstrapEntry{
			Kind: "https-direct", Host: host, Weight: 100,
		})
	}

	now := time.Now().UTC()
	desc := document.Bootstrap{
		V:             1,
		Seq:           seq,
		IssuedAt:      now.Format(time.RFC3339),
		ValidUntil:    now.Add(bootstrapLifetime).Format(time.RFC3339),
		Entries:       entries,
		RefreshAfterS: 86400,
	}

	s.issue(w, ctx, "bootstrap", "bootstrap", seq, desc)
}

// issue signs, records and returns a document.
//
// Recording happens before the response so that what a client received is
// known. Without it, going back to a previous configuration would mean
// reconstructing from memory what was once sent.
func (s *Server) issue(w http.ResponseWriter, ctx context.Context, kind, scope string, seq int64, doc any) {
	envelope, err := s.signer.Seal(doc)
	if err != nil {
		s.log.Error("cannot sign", "kind", kind, "error", err)
		writeError(w, http.StatusInternalServerError, "cannot sign the document")
		return
	}

	if err := s.store.Record(ctx, kind, scope, seq, doc); err != nil {
		s.log.Error("cannot record", "kind", kind, "error", err)
		writeError(w, http.StatusInternalServerError, "cannot record the document")
		return
	}

	writeJSON(w, http.StatusOK, envelope)
}

// chooseTransport picks what both sides can speak.
func (s *Server) chooseTransport(supported []string) string {
	for _, kind := range supported {
		if kind == "vless-ws-tls" {
			return kind
		}
	}
	return ""
}

// reserves returns up to two alternatives, never the primary.
func reserves(nodes []document.Node) []document.Node {
	if len(nodes) <= 1 {
		return []document.Node{}
	}
	if len(nodes) > 3 {
		return nodes[1:3]
	}
	return nodes[1:]
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

const (
	maxRequestBytes   = 8 << 10
	planLifetime      = 24 * time.Hour
	bootstrapLifetime = 30 * 24 * time.Hour
)
