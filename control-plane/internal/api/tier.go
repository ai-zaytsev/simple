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

// adminAccounts lists what exists, without saying who anybody is.
//
// The first half of assigning a tier through the pipeline: an operator has to
// see what there is before naming one. What comes back is a prefix, a tier and
// a count - enough to pick an account out, and nothing that identifies its
// owner.
func (s *Server) adminAccounts(w http.ResponseWriter, r *http.Request) {
	if !s.admin(w, r) {
		return
	}

	accounts, err := s.store.Accounts(r.Context())
	if err != nil {
		s.log.Error("cannot list accounts", "error", err)
		writeError(w, http.StatusInternalServerError, "cannot list accounts")
		return
	}

	out := make([]map[string]any, 0, len(accounts))
	for _, a := range accounts {
		out = append(out, map[string]any{
			"prefix":  a.Prefix,
			"tier":    a.Tier,
			"devices": a.Devices,

			// The day it began, which is the day the free period counts from.
			// A date is not an identity, and an operator deciding about that
			// period cannot check it against anything without this.
			"created": a.Created,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"accounts": out})
}

// adminTierByPrefix sets a status on an account named by the start of its
// identifier.
//
// Separate from adminTier rather than another branch inside it. That one takes
// an address, which is why it can only ever be called by hand from the machine
// itself; this one takes a handle that can appear in a public log, which is
// what lets the operation happen through the pipeline at all.
//
// Both exist because they are not the same operation. An address is what a
// person tells support; a prefix is what an operator reads off a listing.
func (s *Server) adminTierByPrefix(w http.ResponseWriter, r *http.Request) {
	if !s.admin(w, r) {
		return
	}

	var body struct {
		Prefix string `json:"prefix"`
		Tier   string `json:"tier"`
	}
	if err := decode(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "request could not be read")
		return
	}

	prefix := strings.TrimSpace(body.Prefix)
	// Four characters is sixty-five thousand, which is the point below which a
	// typed prefix stops being a way of naming one account and starts being a
	// way of catching several. The refusal on ambiguity would hold anyway;
	// this makes the mistake cheap to understand.
	if len(prefix) < 4 {
		writeError(w, http.StatusBadRequest, "give at least four characters of the identifier")
		return
	}

	tier := strings.ToUpper(strings.TrimSpace(body.Tier))
	if tier == "" {
		writeError(w, http.StatusBadRequest, "no tier given")
		return
	}

	result, err := s.store.SetAccountTierByPrefix(r.Context(), prefix, tier)
	switch {
	case errors.Is(err, store.ErrNoSuchAccount):
		writeError(w, http.StatusNotFound, "no account starts with that")
		return
	case errors.Is(err, store.ErrAmbiguousAccount):
		// Refused rather than resolved. Granting a tier to the wrong account
		// is not a mistake that announces itself.
		writeError(w, http.StatusConflict, "that matches more than one account; give more characters")
		return
	case err != nil:
		s.log.Error("cannot change an account tier")
		writeError(w, http.StatusBadRequest,
			"the tier could not be set; it must be one the service knows")
		return
	}

	s.log.Info("account tier set", "account", result.AccountID, "tier", result.Tier)

	// The prefix back, not the identifier. The caller already knows what it
	// sent, and the answer travels into the same public log the request did.
	writeJSON(w, http.StatusOK, map[string]any{
		"prefix":       prefix,
		"tier":         result.Tier,
		"max_devices":  result.MaxDevices,
		"max_external": result.MaxExternal,
		"devices":      result.Devices,
	})
}
