// Package api serves the three documents a client asks for.
package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"download.simplevpn/control-plane/internal/analytics"
	"download.simplevpn/control-plane/internal/document"
	"download.simplevpn/control-plane/internal/mail"
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

	// planTTL is how long a plan stays usable before a client asks again.
	//
	// Configurable rather than fixed, because it is the lever deciding how
	// quickly a change made here reaches a device already in the field. A long
	// value spares the server and leaves people on a withdrawn node for longer;
	// a short one shortens that wait and costs more requests. Which trade is
	// right changes with circumstances, and changing it must not mean
	// rebuilding and redeploying.
	planTTL time.Duration

	// mail sends the sign-in link, and baseURL is what that link points at.
	// Both are configuration for the same reason the endpoint is: the address
	// inside a link has to be able to change without rebuilding anything.
	mail    *mail.Sender
	baseURL string

	// analytics turns an account into the only user key measurement may hold.
	// Kept here so that no handler is ever tempted to log an account instead.
	analytics *analytics.Deriver
}

func New(
	st *store.Store,
	signer *signing.Signer,
	hosts []string,
	planTTL time.Duration,
	sender *mail.Sender,
	baseURL string,
	deriver *analytics.Deriver,
	log *slog.Logger,
) *Server {
	return &Server{
		store:          st,
		signer:         signer,
		bootstrapHosts: hosts,
		planTTL:        planTTL,
		mail:           sender,
		baseURL:        baseURL,
		analytics:      deriver,
		log:            log,
	}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("POST /v1/auth/start", s.authStart)
	mux.HandleFunc("POST /v1/auth/poll", s.authPoll)

	// Short, because it goes in a message people read and sometimes retype.
	mux.HandleFunc("GET /a", s.authConfirm)
	mux.HandleFunc("POST /v1/plan", s.plan)
	mux.HandleFunc("POST /v1/devices", s.listDevices)
	mux.HandleFunc("POST /v1/devices/revoke", s.revokeDevice)
	mux.HandleFunc("POST /v1/whereami", s.whereFrom)
	mux.HandleFunc("POST /v1/plan/failed", s.planFailed)
	mux.HandleFunc("GET /v1/node/users", s.nodeUsers)
	mux.HandleFunc("GET /v1/config", s.config)
	mux.HandleFunc("GET /v1/bootstrap", s.bootstrap)
	return mux
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type planRequest struct {
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
	if err := decode(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "request could not be read")
		return
	}

	ctx := r.Context()

	// Who this is comes from a secret, never from a name in the request. The
	// identifier used to be enough, which made it a claim anyone could make;
	// swapping one for somebody else's is now worth nothing without the token
	// that was handed over when a mailbox was proved.
	device, ok := s.device(w, r)
	if !ok {
		return
	}

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

	// Where traffic goes, decided here and not by the client. A client that
	// chose its own routes would be a client whose wrong route needs a release
	// to correct.
	routing, err := s.store.LoadRouting(ctx)
	if err != nil {
		s.log.Error("cannot read routing rules", "error", err)
		writeError(w, http.StatusInternalServerError, "cannot issue a plan")
		return
	}

	// This device's own way in, on every node it might be sent to. One device
	// cut off leaves the rest untouched, and a credential taken off one phone
	// is that phone's alone.
	credential, err := s.store.EnsureCredential(ctx, device.ID)
	if err != nil {
		s.log.Error("cannot issue a credential", "error", err)
		writeError(w, http.StatusInternalServerError, "cannot issue a plan")
		return
	}
	for i := range nodes {
		nodes[i].Transport.Params["credential_uuid"] = credential.String()
	}

	// Numbered per device rather than per account, because two phones on one
	// account refresh independently and a shared counter would make each of
	// them see the other's plan as a rollback.
	scope := "plan:" + device.ID.String()
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
		ExpiresAt:   now.Add(s.planTTL).Format(time.RFC3339),
		AccountTier: "FREE",
		Primary:     nodes[0],
		Reserves:    reserves(nodes),
		DNS: document.DNS{
			Mode:    "tunnel",
			Servers: []string{"https://1.1.1.1/dns-query", "https://8.8.8.8/dns-query"},
		},
		Routing: routing,
		Policy: document.Policy{
			ConnectTimeoutMS:     8000,
			FailoverAfterFailure: 2,
			ReconnectBackoffMS:   []int{1000, 3000, 8000, 20000},
			PlanRefreshAfterS:    int(s.planTTL.Seconds()) / 2,
			TelemetrySampling:    1.0,
			ProbeIntervalS:       60,
		},
	}

	// The only user key that leaves this handler for a log or a measurement.
	// Not the account, not the device, not the credential: those three are how
	// a person, a phone and a way in are tied together, and a log archive is
	// the wrong place to keep that knot.
	s.log.Info("plan issued",
		"user", s.analytics.ID(device.AccountID, now),
		"node", nodes[0].Alias,
		"reserves", len(plan.Reserves))

	s.issue(w, r.Context(), "plan", scope, seq, plan)
}

func (s *Server) config(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Read before a number is taken. A failure here must not consume a
	// sequence number, because a client that saw a higher number will refuse
	// everything below it afterwards - including the document we failed to
	// build.
	state, err := s.store.LoadServiceState(ctx)
	if err != nil {
		s.log.Error("cannot read service state", "error", err)
		writeError(w, http.StatusInternalServerError, "cannot issue configuration")
		return
	}

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
		MinSupportedAppVersion: state.MinSupportedAppVersion,
		KillSwitch: document.KillSwitch{
			Enabled:    state.KillSwitch.Enabled,
			MessageKey: state.KillSwitch.MessageKey,
		},
		Features:      map[string]bool{"config_export_enabled": false},
		RefreshAfterS: configRefreshSeconds,
	}

	if cfg.KillSwitch.Enabled {
		s.log.Warn("kill switch is on", "seq", seq)
	}

	s.issue(w, ctx, "config", "config", seq, cfg)
}

// bootstrap says every way this service can be reached.
//
// The one document that has to survive its own subject being blocked, which is
// why it is served here, mirrored outside our infrastructure, and signed. The
// signature is what makes an untrusted mirror acceptable: its contents cannot
// be altered, only made unavailable.
func (s *Server) bootstrap(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Read before a number is taken, so a failure does not consume one. A
	// client that saw a higher number refuses everything below it afterwards,
	// including the descriptor we failed to build.
	entries, err := s.store.LoadBootstrapEntries(ctx)
	if err != nil {
		s.log.Error("cannot read bootstrap entries", "error", err)
		writeError(w, http.StatusInternalServerError, "cannot issue a descriptor")
		return
	}

	if len(entries) == 0 {
		// Falling back to the configured host is better than issuing a
		// descriptor with no way in at all, which every client would then
		// store and be unable to replace.
		for _, host := range s.bootstrapHosts {
			entries = append(entries, document.BootstrapEntry{
				Kind: "https-direct", Host: host, Port: 443, Weight: 100,
			})
		}
		s.log.Warn("no bootstrap entries configured, falling back to the host in the environment")
	}

	seq, err := s.store.NextSeq(ctx, "bootstrap")
	if err != nil {
		s.log.Error("cannot advance the sequence", "error", err)
		writeError(w, http.StatusInternalServerError, "cannot issue a descriptor")
		return
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

// decode reads a request body with a size limit, so that a client cannot make
// the server allocate by sending something enormous.
func decode(w http.ResponseWriter, r *http.Request, into any) error {
	return json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBytes)).Decode(into)
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
	bootstrapLifetime = 30 * 24 * time.Hour

	// How often a client asks whether the service has been stopped.
	//
	// The one number that decides it. A client schedules its next question by
	// this value and has no interval of its own, because it had two once: it
	// woke every five minutes, found a document it considered fresh for
	// fifteen, and asked nobody. The switch worked and took three times as
	// long as anybody had been told.
	configRefreshSeconds = 300
)
