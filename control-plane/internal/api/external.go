package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"

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

	connection, err := s.linkFor(r, added)
	if err != nil {
		s.log.Error("cannot build a link", "error", err)
		writeError(w, http.StatusInternalServerError, "cannot build the link")
		return
	}

	// The account, never the label. What somebody calls their television is
	// theirs, and a log is not the place for it.
	s.log.Info("external device added", "account", caller.AccountID, "device", added.ID)

	writeJSON(w, http.StatusCreated, map[string]any{
		"device_id": added.ID.String(),
		"label":     added.Label,
		"link":      connection,
	})
}

// linkFor builds the one address this device uses.
//
// One, not one per node. The first version handed back a link for every node
// in the fleet, reasoning that a client holding several survives one of them
// being retired. The Business Owner refused it, and the arithmetic is the
// argument: with a hundred nodes that is a hundred links for one television,
// and nobody pastes a hundred of anything. A person who wants a second
// connection adds a second device and names it.
//
// So the node is drawn, once, from the same weighted ranking that decides
// where a phone goes - by room and by quality, so a busy or unwell node is
// drawn rarely and never for everybody at once.
//
// Drawn from the credential rather than from the device, and that is the whole
// mechanism behind "new link". The draw is stable while the credential is, so
// a router keeps the address it was given and does not silently move; and
// replacing the credential draws again, which is exactly what somebody asking
// for a new link wants - a different node, because the reason they are asking
// is usually that the old one stopped answering.
func (s *Server) linkFor(r *http.Request, device store.DeviceSummary) (string, error) {
	// A link is built for an external device and for nothing else.
	//
	// Checked here rather than left to the callers, because it is the rule the
	// whole tier arrangement rests on: a link is a way to connect from
	// somewhere we did not build, and only VIP may have one. An account with
	// no external allowance can own no external device, so it can reach no
	// link - but that is a chain of three facts, and a chain is worth one
	// explicit link at the end of it.
	if device.Kind != "external" {
		return "", errors.New("only an external device has a link")
	}
	if device.Credential == nil {
		return "", errors.New("the device has no credential")
	}

	standings, err := s.store.NodeStandings(r.Context(), "", s.nodeCapacity)
	if err != nil {
		return "", err
	}

	now := time.Now().UTC()
	for _, standing := range store.Rank(standings, device.Credential.String(), now) {
		if !standing.Usable(now) {
			continue
		}
		built, err := link.For(standing.Node, device.Credential.String(), device.Label)
		if err != nil {
			// A transport with no link form is passed over rather than fatal.
			// The ranking has others behind it, and refusing everything
			// because the first node speaks something else would be the wrong
			// trade.
			continue
		}
		return built, nil
	}
	return "", errors.New("no node can be linked to right now")
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
		connection, err := s.linkFor(r, d)
		if err != nil {
			// Listed anyway, with no link. A device the service cannot
			// build an address for right now is the one its owner most
			// needs to see: it is the row they press "new link" on.
			s.log.Error("cannot build a link", "error", err)
			connection = ""
		}
		out = append(out, map[string]any{
			"device_id": d.ID.String(),
			"label":     d.Label,
			"link":      connection,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": out})
}

// rotateExternal replaces the link of one external device.
//
// For "the old one stopped working". The reasons differ - a node retired, a
// link pasted where it should not have been, a client that cached something
// broken - and the answer to all of them is the same: this television, working
// again, still called what its owner calls it.
//
// Deliberately not "delete and add again". That loses the name, loses the
// device's place in the list, and asks somebody to re-do the part they got
// right in order to fix the part they did not.
func (s *Server) rotateExternal(w http.ResponseWriter, r *http.Request) {
	caller, ok := s.device(w, r)
	if !ok {
		return
	}

	var body struct {
		DeviceID string `json:"device_id"`
	}
	if err := decode(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "request could not be read")
		return
	}

	deviceID, err := uuid.Parse(body.DeviceID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "that is not a device")
		return
	}

	// The account comes from the token, never from the request. Naming
	// somebody else's television has to fail on ownership rather than on
	// whether the caller guessed an identifier that exists.
	rotated, err := s.store.RotateExternalCredential(r.Context(), caller.AccountID, deviceID)
	switch {
	case errors.Is(err, store.ErrNotYours):
		writeError(w, http.StatusNotFound, "no such device on this account")
		return
	case err != nil:
		s.log.Error("cannot rotate an external credential", "error", err)
		writeError(w, http.StatusInternalServerError, "cannot replace the link")
		return
	}

	connection, err := s.linkFor(r, rotated)
	if err != nil {
		s.log.Error("cannot build a link", "error", err)
		writeError(w, http.StatusInternalServerError, "cannot build the link")
		return
	}

	s.log.Info("external link replaced", "account", caller.AccountID, "device", rotated.ID)

	writeJSON(w, http.StatusOK, map[string]any{
		"device_id": rotated.ID.String(),
		"label":     rotated.Label,
		"link":      connection,
	})
}
