package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"download.simplevpn/control-plane/internal/auth"
	"download.simplevpn/control-plane/internal/purchase"
	"download.simplevpn/control-plane/internal/store"
)

// bearer reads the secret a request authenticates with.
//
// A header rather than a field in the body, so that it is never part of what a
// handler happens to log, and never mistaken for data.
func bearer(r *http.Request) string {
	header := r.Header.Get("authorization")
	const prefix = "Bearer "
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(header[len(prefix):])
}

// device turns a request into the device that made it, or answers 401.
//
// Everything a device is allowed to do goes through here. There is deliberately
// no path that takes a device identifier from a request body: that was the
// hole, and leaving one such path open would leave it open entirely.
func (s *Server) device(w http.ResponseWriter, r *http.Request) (store.Device, bool) {
	token := bearer(r)
	if token == "" {
		writeError(w, http.StatusUnauthorized, "device is not signed in")
		return store.Device{}, false
	}

	device, err := s.store.DeviceByToken(r.Context(), auth.HashToken(token))
	if err != nil {
		// One answer for a token nobody was issued, one that was replaced, and
		// one that was cut off. The holder can act on none of the differences
		// and an attacker could.
		writeError(w, http.StatusUnauthorized, "device is not signed in")
		return store.Device{}, false
	}
	return device, true
}

// listDevices shows a person their own devices, so that a lost phone can be
// picked out from the others.
//
// Identifiers only. Nothing here says where a device is or was, because that
// would make this endpoint a way of locating somebody with a stolen token.
func (s *Server) listDevices(w http.ResponseWriter, r *http.Request) {
	device, ok := s.device(w, r)
	if !ok {
		return
	}

	devices, err := s.store.DevicesOfAccount(r.Context(), device.AccountID)
	if err != nil {
		s.log.Error("cannot list devices", "error", err)
		writeError(w, http.StatusInternalServerError, "cannot list devices")
		return
	}

	out := make([]map[string]any, 0, len(devices))
	for _, d := range devices {
		out = append(out, map[string]any{
			"device_id":  d.ID.String(),
			"state":      d.State,
			"created_at": d.CreatedAt,
			"this_one":   d.ID == device.ID,

			// One list holds phones and televisions, so it has to say which is
			// which and what the person called it. Revoking the right one must
			// not mean reading identifiers and guessing.
			"kind":  d.Kind,
			"label": d.Label,
		})
	}

	// The status, so the application knows which of itself to show.
	//
	// The screen for routers and televisions belongs to VIP, and an
	// application that cannot tell would have to either show it to everybody
	// and refuse on use - which teaches people the product is broken - or
	// guess from a failure, which is worse.
	//
	// Answered on this call because the application already makes it, and
	// because this is the call that says what an account has. It carries no
	// address: the caller proved a device token, and a tier is not personal.
	tier, created, err := s.store.TierOfAccount(r.Context(), device.AccountID)
	if err != nil {
		s.log.Error("cannot read the tier", "error", err)
		writeError(w, http.StatusInternalServerError, "cannot read the account")
		return
	}

	// Whether VIP may be bought, decided here rather than on the phone.
	//
	// The application draws the answer and does not compute it: the free
	// period and the switch both change from the server, and a device
	// applying its own copy of the rule would keep offering a purchase for as
	// long as it took somebody to install a new version. That is the whole
	// requirement of this stage.
	//
	// A failure to read the settings leaves the offer closed. Selling because
	// a query failed is the one outcome here worth refusing outright.
	offer := purchase.Offer{Reason: purchase.ReasonClosed}
	if state, err := s.store.LoadServiceState(r.Context()); err != nil {
		s.log.Error("cannot read the purchase settings", "error", err)
	} else {
		offer = purchase.Assess(time.Now().UTC(), created, tier, state.Purchases)
	}

	buy := map[string]any{"available": offer.Available, "reason": offer.Reason}
	if offer.AvailableAt != nil {
		buy["available_at"] = offer.AvailableAt.UTC().Format(time.RFC3339)
	}
	if offer.Available && s.payments != nil {
		products, err := s.payments.Products(r.Context())
		if err != nil {
			s.log.Error("cannot read payment products", "error", err)
			buy["available"] = false
			buy["reason"] = purchase.ReasonClosed
		} else {
			out := make([]map[string]any, 0, len(products))
			for _, product := range products {
				out = append(out, map[string]any{
					"id":              product.ID,
					"title":           product.Title,
					"amount_minor":    product.AmountMinor,
					"currency":        product.Currency,
					"duration_months": product.DurationMonths,
				})
			}
			buy["products"] = out
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"devices":  out,
		"tier":     tier,
		"purchase": buy,
	})
}

type revokeRequest struct {
	DeviceID string `json:"device_id"`
}

// revokeDevice cuts one device off.
//
// The caller may only name a device on their own account, and that is checked
// against the account the token proves rather than one the request states. A
// stolen phone is cut off from another phone the person still holds; nothing
// here lets anybody reach into somebody else's account.
func (s *Server) revokeDevice(w http.ResponseWriter, r *http.Request) {
	caller, ok := s.device(w, r)
	if !ok {
		return
	}

	var req revokeRequest
	if err := decode(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "request could not be read")
		return
	}
	target, err := uuid.Parse(req.DeviceID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "device is not identified")
		return
	}

	owned, err := s.store.DevicesOfAccount(r.Context(), caller.AccountID)
	if err != nil {
		s.log.Error("cannot list devices", "error", err)
		writeError(w, http.StatusInternalServerError, "cannot revoke")
		return
	}
	found := false
	for _, d := range owned {
		if d.ID == target {
			found = true
			break
		}
	}
	if !found {
		// Answered as "not found" rather than "not yours", so that this cannot
		// be used to discover whether a device identifier exists at all.
		writeError(w, http.StatusNotFound, "no such device")
		return
	}

	if _, err := s.store.RevokeDevice(r.Context(), target); err != nil {
		s.log.Error("cannot revoke a device", "error", err)
		writeError(w, http.StatusInternalServerError, "cannot revoke")
		return
	}

	s.log.Info("device revoked", "by_self", target == caller.ID)
	writeJSON(w, http.StatusOK, map[string]any{"revoked": target.String()})
}

// nodeUsers tells one node who may connect to it.
//
// The whole list every time, not what changed. A node that misses half a delta
// has an idea of who may connect that drifts from ours and never returns; with
// the whole list its state is a function of this answer rather than of its own
// history.
func (s *Server) nodeUsers(w http.ResponseWriter, r *http.Request) {
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

	credentials, err := s.store.LiveCredentials(r.Context())
	if err != nil {
		s.log.Error("cannot list credentials", "error", err)
		writeError(w, http.StatusInternalServerError, "cannot list users")
		return
	}

	// Not cached anywhere in between: this is the list of who may connect.
	w.Header().Set("cache-control", "no-store")
	s.log.Info("node asked for users", "node", alias, "count", len(credentials))
	writeJSON(w, http.StatusOK, map[string]any{"credentials": credentials})
}

// whereFrom tells a device what address it is seen from.
//
// It exists for one question: is this phone's network already going through
// one of our own nodes? That happens when somebody runs this VPN on their
// router, and building a second tunnel inside the first is at best wasteful
// and at worst a loop. The phone cannot answer it alone - a router's tunnel is
// invisible from the device, which sees ordinary Wi-Fi - so something outside
// has to say what address the traffic arrives from.
//
// Authenticated, so this is not a free "what is my address" service for the
// internet. Never logged and never stored: the address is told back to the
// only party that already knows it, and goes no further.
func (s *Server) whereFrom(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.device(w, r); !ok {
		return
	}

	address := r.Header.Get("x-real-ip")
	if address == "" {
		// Nginx is expected to supply it. Saying so plainly beats returning
		// the address of the proxy, which every device would then compare
		// against the node list and always fail to match - a check that
		// silently never fires.
		s.log.Error("x-real-ip is missing; the reverse proxy is not passing it")
		writeError(w, http.StatusServiceUnavailable, "cannot determine the address")
		return
	}

	w.Header().Set("cache-control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"address": address})
}

type planFailureReport struct {
	Seq    int64  `json:"seq"`
	Reason string `json:"reason"`
}

// planFailed records that a plan did not work on somebody's device.
//
// Rolling back on the device is only half of not breaking the product for
// everybody: without this, a plan that fails in the field fails silently, and
// the next person to install gets the same one. This is how an operator finds
// out, and it is the only signal that exists for it.
//
// A number and a reason. The user key is derived and irreversible, so the same
// device failing repeatedly is visible as one device without anybody being
// identifiable from the log.
func (s *Server) planFailed(w http.ResponseWriter, r *http.Request) {
	device, ok := s.device(w, r)
	if !ok {
		return
	}

	var report planFailureReport
	if err := decode(w, r, &report); err != nil {
		writeError(w, http.StatusBadRequest, "request could not be read")
		return
	}

	// Trimmed, because it is written by a client and ends up in a log. A
	// reason long enough to bury the rest of a line is a reason that hides
	// what it was supposed to reveal.
	reason := report.Reason
	if len(reason) > maxReasonLength {
		reason = reason[:maxReasonLength]
	}

	s.log.Warn("plan did not work",
		"user", s.analytics.ID(device.AccountID, time.Now().UTC()),
		"plan_seq", report.Seq,
		"reason", reason)

	writeJSON(w, http.StatusOK, map[string]any{"recorded": true})
}

const maxReasonLength = 200
