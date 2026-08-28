package api

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"download.simplevpn/control-plane/internal/auth"
	"download.simplevpn/control-plane/internal/certs"
)

type certificateRequest struct {
	// CSR is the node's signing request, PEM. The private half stays on the
	// node and is never asked for.
	CSR string `json:"csr"`

	// ExpiresAt is when the node's current certificate runs out, empty when it
	// has none. Sent so that a node which does not need one is not given one:
	// the authority counts issuances, and spending the count on a certificate
	// nobody needed is how a renewal that matters gets refused.
	ExpiresAt string `json:"expires_at,omitempty"`
}

// How much life left still counts as "does not need one yet". Thirty days is
// the usual renewal window: long enough that a week of failures can be
// survived, short enough that the authority's own advice is followed.
const renewWithin = 30 * 24 * time.Hour

// How many issuances for one name the authority allows in seven days. Kept one
// below the real limit of five, so that a genuine emergency has one left.
const issuesPerWeek = 4

// nodeCertificate signs a node's request without ever seeing its key.
//
// The name is taken from what the node was configured with, never from what it
// asks for. A node allowed to name its own domain could ask us to prove
// ownership of any domain we hold and hand it the result - which is the whole
// thing this arrangement exists to prevent.
func (s *Server) nodeCertificate(w http.ResponseWriter, r *http.Request) {
	if s.certs == nil {
		writeError(w, http.StatusServiceUnavailable, "certificates are not configured")
		return
	}

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

	var req certificateRequest
	if err := decode(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "request could not be read")
		return
	}

	ctx := r.Context()
	expected, err := s.store.CertificateName(ctx, alias)
	if err != nil {
		s.log.Error("cannot decide what to certify", "node", alias, "error", err)
		writeError(w, http.StatusInternalServerError, "cannot issue")
		return
	}

	if err := s.mayIssue(ctx, alias, expected, req); err != nil {
		s.store.RecordRefusal(ctx, expected, alias, err.Error())
		s.log.Warn("refusing to issue", "node", alias, "name", expected, "reason", err.Error())
		writeError(w, http.StatusConflict, err.Error())
		return
	}

	// Which authority to ask is a property of the node, not of this service.
	// A rehearsal machine is certified by the one whose certificates nobody
	// trusts, so that proving the whole path from nothing to a working node
	// costs none of the real allowance - five per name per week, and the thing
	// a genuine renewal depends on.
	issuer, authority := s.certs, "real"
	if rehearsal, err := s.store.NodeWantsTestCertificates(ctx, alias); err == nil && rehearsal {
		if s.certsTest == nil {
			writeError(w, http.StatusServiceUnavailable, "test certificates are not configured")
			return
		}
		issuer, authority = s.certsTest, "test"
	}

	chain, err := issuer.Issue(ctx, req.CSR)
	if err != nil {
		s.store.RecordRefusal(ctx, expected, alias, "authority refused")
		s.log.Error("the authority did not issue",
			"node", alias, "name", expected, "authority", authority, "error", err)
		writeError(w, http.StatusBadGateway, "the authority did not issue")
		return
	}

	expires := expiryOf(chain)
	if err := s.store.RecordIssue(ctx, expected, alias, expires); err != nil {
		// The certificate exists whatever happens to the note. Losing the note
		// costs accuracy in the count against the limit; refusing to hand over
		// a certificate already granted costs a working node.
		s.log.Error("cannot record the issue", "error", err)
	}

	s.log.Info("certificate issued",
		"node", alias, "name", expected, "expires", expires.Format(time.RFC3339))

	// The certificate is public by definition - it is meant to be shown to
	// everybody - so the answer needs no protection beyond being the right one.
	writeJSON(w, http.StatusOK, map[string]any{
		"certificate": chain,
		"expires_at":  expires.Format(time.RFC3339),
	})
}

// mayIssue decides whether this issuance should happen at all.
//
// Three refusals, and each of them protects something a node cannot get back
// on its own: the authority's weekly count, the meaning of the node's own
// name, and the guarantee that we only ever prove what we were asked about.
func (s *Server) mayIssue(ctx context.Context, alias, expected string, req certificateRequest) error {
	names, err := certs.NamesIn(req.CSR)
	if err != nil {
		return errors.New("the request is unreadable")
	}
	if len(names) != 1 || !strings.EqualFold(names[0], expected) {
		// Not "the node asked for the wrong thing" but "the node asked us to
		// prove something about a name it was not given". Refused before any
		// record is published.
		return fmt.Errorf("this node may only be certified for %s", expected)
	}

	if remaining := timeLeft(req.ExpiresAt); remaining > renewWithin {
		return fmt.Errorf(
			"the current certificate has %d days left; renewal starts at %d",
			int(remaining.Hours()/24), int(renewWithin.Hours()/24),
		)
	}

	issued, err := s.store.IssuesThisWeek(ctx, expected)
	if err != nil {
		return errors.New("cannot check the weekly count")
	}
	if issued >= issuesPerWeek {
		// The authority will refuse anyway, and its refusal costs a failed
		// order rather than a polite no. Better to stop here and leave one of
		// the week's allowance for an emergency.
		return fmt.Errorf("%s has been certified %d times this week", expected, issued)
	}

	return nil
}

// timeLeft reads how long the node says its certificate has.
//
// An unreadable or absent date means "no certificate", which is the answer that
// allows issuance. A node with no certificate is the case this exists for; a
// node that garbles the date is a node we would rather over-serve than strand.
func timeLeft(stated string) time.Duration {
	if stated == "" {
		return 0
	}
	at, err := time.Parse(time.RFC3339, stated)
	if err != nil {
		return 0
	}
	return time.Until(at)
}

// expiryOf reads the end date out of the chain that was issued.
//
// From the certificate itself rather than from what was asked for: the
// authority decides the life of what it signs, and recording our assumption
// instead would make the count drift from the thing it counts.
func expiryOf(chainPEM string) time.Time {
	rest := []byte(chainPEM)
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			return time.Time{}
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		parsed, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return time.Time{}
		}
		// The first certificate in a bundle is the one that was issued; the
		// rest are the chain to it.
		return parsed.NotAfter
	}
}
