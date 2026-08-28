package api

import (
	"errors"
	"net/http"
	"time"

	"download.simplevpn/control-plane/internal/link"
	"download.simplevpn/control-plane/internal/store"
)

// How long a label may be. Long enough for "роутер в гостиной", short enough
// that the column is not a place to put something else.
const maxLabel = 64

// addExternal connects a router, a television or a computer.
//
// Authorised by the caller's own device token, so an account can only add to
// itself. What comes back is the device and one link per node it can use.
func (s *Server) addExternal(w http.ResponseWriter, r *http.Request) {
	caller, ok := s.device(w, r)
	if !ok {
		return
	}

	var body struct {
		Label string `json:"label"`
	}
	if err := decode(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "request could not be read")
		return
	}

	label := trim(body.Label, maxLabel)
	if label == "" {
		writeError(w, http.StatusBadRequest, "the device needs a name")
		return
	}

	added, err := s.store.AddExternalDevice(r.Context(), caller.AccountID, label)
	switch {
	case errors.Is(err, store.ErrTooManyExternal):
		// The refusal says what happened without saying what tier the caller
		// is on: the same wording answers a FREE account, which may have none,
		// and a VIP account that has filled its allowance.
		writeError(w, http.StatusConflict, "no room for another device on this account")
		return
	case err != nil:
		s.log.Error("cannot add an external device", "error", err)
		writeError(w, http.StatusInternalServerError, "cannot add the device")
		return
	}

	links, err := s.linksFor(r, added)
	if err != nil {
		s.log.Error("cannot build links", "error", err)
		writeError(w, http.StatusInternalServerError, "cannot build the links")
		return
	}

	// The account, never the label. What somebody calls their television is
	// theirs, and a log is not the place for it.
	s.log.Info("external device added", "account", caller.AccountID, "device", added.ID)

	writeJSON(w, http.StatusCreated, map[string]any{
		"device_id": added.ID.String(),
		"label":     added.Label,
		"links":     links,
	})
}

// linksFor builds one address per node this device could use.
//
// One link per node rather than one link, because a pasted link lives exactly
// as long as the node it names. A list is what lets a router keep working
// through a node being retired, and third-party clients import several without
// complaint.
func (s *Server) linksFor(r *http.Request, device store.DeviceSummary) ([]string, error) {
	if device.Credential == nil {
		return nil, errors.New("the device has no credential")
	}

	standings, err := s.store.NodeStandings(r.Context(), "", s.nodeCapacity)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	links := []string{}
	for _, standing := range standings {
		if !standing.Usable(now) {
			continue
		}
		built, err := link.For(standing.Node, device.Credential.String(), device.Label)
		if err != nil {
			// A transport with no link form is skipped rather than fatal: the
			// others are still usable, and refusing everything because one
			// node speaks something else would be the wrong trade.
			continue
		}
		links = append(links, built)
	}
	return links, nil
}

// externalLinks hands back the links for devices the caller already has.
//
// Needed because a link is shown once when a device is added, and the person
// setting up a router is rarely holding the phone at the same moment.
func (s *Server) externalLinks(w http.ResponseWriter, r *http.Request) {
	caller, ok := s.device(w, r)
	if !ok {
		return
	}

	devices, err := s.store.DevicesOfAccount(r.Context(), caller.AccountID)
	if err != nil {
		s.log.Error("cannot list devices", "error", err)
		writeError(w, http.StatusInternalServerError, "cannot list devices")
		return
	}

	out := []map[string]any{}
	for _, d := range devices {
		if d.Kind != "external" || d.State != "ACTIVE" {
			continue
		}
		links, err := s.linksFor(r, d)
		if err != nil {
			s.log.Error("cannot build links", "error", err)
			continue
		}
		out = append(out, map[string]any{
			"device_id": d.ID.String(),
			"label":     d.Label,
			"links":     links,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": out})
}
