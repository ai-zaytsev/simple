package api

import (
	"net/http"
	"strings"

	"download.simplevpn/control-plane/internal/purchase"
)

// adminPurchases reads or changes whether VIP may be bought.
//
// Behind the panel's key, like every other operator action, which since the
// closed-management stage means it is not served to the internet at all.
//
// Nothing here names a person. The two settings are service-wide by design:
// selling is either open or it is not, and the wait is the same for everybody.
// A per-account exception would be a discount scheme, which is a different
// thing and not this stage.
func (s *Server) adminPurchases(w http.ResponseWriter, r *http.Request) {
	if !s.admin(w, r) {
		return
	}

	var body struct {
		// Absent means read. Present means write, and both fields are then
		// required: the row is replaced whole, so a half-given body would
		// quietly keep whichever half was left out.
		Open     *bool `json:"open"`
		FreeDays *int  `json:"free_days"`
	}
	if err := decode(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "request could not be read")
		return
	}

	if body.Open != nil || body.FreeDays != nil {
		if body.Open == nil || body.FreeDays == nil {
			writeError(w, http.StatusBadRequest,
				"give both open and free_days; the setting is replaced whole")
			return
		}
		if *body.FreeDays < 0 {
			writeError(w, http.StatusBadRequest, "the wait cannot be negative")
			return
		}

		by := strings.TrimSpace(r.Header.Get("x-changed-by"))
		if by == "" {
			by = "operator"
		}

		settings := purchase.Settings{Open: *body.Open, FreeDays: *body.FreeDays}
		if err := s.store.SetPurchases(r.Context(), settings, by); err != nil {
			s.log.Error("cannot save the purchase settings", "error", err)
			writeError(w, http.StatusInternalServerError, "cannot save the settings")
			return
		}

		// Said out loud, because turning selling on or off is the kind of
		// change somebody will later want to date.
		s.log.Info("purchase settings changed",
			"open", settings.Open, "free_days", settings.FreeDays, "by", by)
	}

	state, err := s.store.LoadServiceState(r.Context())
	if err != nil {
		s.log.Error("cannot read the purchase settings", "error", err)
		writeError(w, http.StatusInternalServerError, "cannot read the settings")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"open":      state.Purchases.Open,
		"free_days": state.Purchases.FreeDays,
	})
}
