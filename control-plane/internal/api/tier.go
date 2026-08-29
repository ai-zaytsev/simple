package api

import (
	"errors"
	"net/http"
	"strings"

	"download.simplevpn/control-plane/internal/store"
)

// adminTier reads or sets an account's status.
//
// Behind the panel's key, which since the closed-management stage means it is
// not served to the internet at all and is reached over the same tunnel the
// panel is. That is not only a security choice: the handle for an account is
// an email address, and the alternative - a workflow with the address as an
// input - would print it into a public log on every run.
//
// The address is used to find the row and is never returned or logged. What
// comes back is the account identifier, which is what everything else in this
// system uses to talk about a person.
func (s *Server) adminTier(w http.ResponseWriter, r *http.Request) {
	if !s.admin(w, r) {
		return
	}

	var body struct {
		Email string `json:"email"`
		Tier  string `json:"tier"`
	}
	if err := decode(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "request could not be read")
		return
	}

	email := strings.TrimSpace(body.Email)
	if email == "" {
		writeError(w, http.StatusBadRequest, "no address given")
		return
	}

	// An empty tier reads rather than writes. One endpoint rather than two,
	// because "what is this account on" is the question asked immediately
	// before and immediately after the one that changes it.
	tier := strings.TrimSpace(body.Tier)

	var (
		result store.AccountTier
		err    error
	)
	if tier == "" {
		result, err = s.store.AccountTierByEmail(r.Context(), email)
	} else {
		result, err = s.store.SetAccountTier(r.Context(), email, strings.ToUpper(tier))
	}

	switch {
	case errors.Is(err, store.ErrNoSuchAccount):
		writeError(w, http.StatusNotFound, "no account has signed in with that address")
		return
	case err != nil:
		// The error is not passed on. It can carry the statement that failed,
		// and the statement carries the address.
		s.log.Error("cannot change an account tier")
		writeError(w, http.StatusBadRequest,
			"the tier could not be set; it must be one the service knows")
		return
	}

	if tier != "" {
		// The account, never the address. This is the one line that says a
		// status was changed, and it has to be readable without being a record
		// of who somebody is.
		s.log.Info("account tier set", "account", result.AccountID, "tier", result.Tier)
	}

	// A null limit is sent as null rather than as a large number. An operator
	// reading this is deciding whether a tier restricts something, and a
	// number would answer a different question - "how much" - that does not
	// apply. The pointers carry that straight through to the JSON.
	writeJSON(w, http.StatusOK, map[string]any{
		"account":      result.AccountID,
		"tier":         result.Tier,
		"max_devices":  result.MaxDevices,
		"max_external": result.MaxExternal,
		"devices":      result.Devices,
	})
}
