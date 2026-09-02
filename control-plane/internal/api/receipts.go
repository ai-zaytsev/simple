package api

import (
	"net/http"
	"strings"
)

// adminReceipts is the manual way out.
//
// It exists because of one state the automatic path cannot resolve on its own:
// a receipt was asked for and the answer never arrived, so nobody knows whether
// «Мой налог» holds one. Creating another might duplicate it; creating none
// might leave income unreported. Only a person looking at the cabinet can say
// which, and until they do the queue stays blocked - and a blocked queue keeps
// selling closed.
//
// So this is not a convenience. Without it a single lost response would shut
// the shop permanently.
func (s *Server) adminReceipts(w http.ResponseWriter, r *http.Request) {
	if !s.admin(w, r) {
		return
	}

	var body struct {
		PaymentID string `json:"payment_id"`

		// The receipt found in «Мой налог», or empty to say there is none and
		// the queue should try again properly.
		ReceiptUUID string `json:"receipt_uuid"`

		// What that receipt is for, in minor units. Zero keeps the amount the
		// failed attempt was going to use.
		AmountMinor int64 `json:"amount_minor"`
	}
	if err := decode(w, r, &body); err != nil {
		return
	}

	paymentID := strings.TrimSpace(body.PaymentID)
	if paymentID == "" {
		writeError(w, http.StatusBadRequest, "payment_id is required")
		return
	}
	if body.AmountMinor < 0 {
		writeError(w, http.StatusBadRequest, "the amount cannot be negative")
		return
	}

	receiptUUID := strings.TrimSpace(body.ReceiptUUID)
	if err := s.store.SettleReceiptByHand(
		r.Context(), paymentID, receiptUUID, body.AmountMinor,
	); err != nil {
		s.log.Error("cannot settle a receipt by hand", "error", err)
		writeError(w, http.StatusInternalServerError, "cannot settle the receipt")
		return
	}

	// Said out loud without the receipt identifier: this is the kind of change
	// somebody will later want to date, and the identifier belongs in «Мой
	// налог» rather than in a log.
	s.log.Info("tax receipt settled by hand", "resolved", receiptUUID != "")

	writeJSON(w, http.StatusOK, map[string]any{
		"settled":  true,
		"receipt":  receiptUUID != "",
		"retrying": receiptUUID == "",
	})
}
