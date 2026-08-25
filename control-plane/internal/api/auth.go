package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"

	"download.simplevpn/control-plane/internal/auth"
	"download.simplevpn/control-plane/internal/store"
)

type startRequest struct {
	DeviceID string `json:"device_id"`
	Email    string `json:"email"`
}

type startResponse struct {
	AttemptID    string `json:"attempt_id"`
	ResendAfterS int    `json:"resend_after_s"`
	ExpiresInS   int    `json:"expires_in_s"`
}

// authStart takes an address and sends a link to it.
//
// The answer is the same whether the address has an account, has none, is
// mistyped, or has just been rate limited. That is the requirement: the
// application must not become a way of asking "is this person a customer".
//
// It follows that a failure to send is also not reported. The person is told to
// look in their mailbox either way, and finds nothing there; that is worse for
// them than an error message and better for everyone whose membership would
// otherwise be discoverable by anyone with their address.
func (s *Server) authStart(w http.ResponseWriter, r *http.Request) {
	var req startRequest
	if err := decode(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "request could not be read")
		return
	}

	deviceID, err := uuid.Parse(req.DeviceID)
	if err != nil {
		// A malformed device identifier is the application's fault, not the
		// user's, so this one is reported honestly.
		writeError(w, http.StatusBadRequest, "device is not identified")
		return
	}

	email, err := auth.NormaliseEmail(req.Email)
	if err != nil {
		// Shape only. Whether the address exists is never revealed, but an
		// address with no at sign is a typo the person can fix, and refusing to
		// say so would be unhelpful rather than private.
		writeError(w, http.StatusBadRequest, "address does not look like an address")
		return
	}

	ctx := r.Context()
	attemptID := uuid.New()
	response := startResponse{
		AttemptID:    attemptID.String(),
		ResendAfterS: int(resendAfter.Seconds()),
		ExpiresInS:   int(linkLifetime.Seconds()),
	}

	recent, err := s.store.RecentAttempts(ctx, email, rateWindow)
	if err != nil {
		s.log.Error("cannot count attempts", "error", err)
		writeError(w, http.StatusInternalServerError, "cannot start sign-in")
		return
	}
	if recent >= maxAttemptsPerWindow {
		// Answered exactly like success. Someone using the form to post mail at
		// a stranger sees no difference and learns nothing, and the stranger
		// stops receiving messages.
		s.log.Info("attempt rate limit reached")
		// The identifier is one this device already has a link for, so the
		// screen goes on waiting for a message that really is in the mailbox.
		// When the requests came from somewhere else there is none, the random
		// identifier stands, and the wait ends in "expired" as before.
		if live, err := s.store.LiveAttempt(ctx, email, deviceID); err == nil {
			response.AttemptID = live.String()
		}
		writeJSON(w, http.StatusOK, response)
		return
	}

	token, hash, err := auth.NewToken()
	if err != nil {
		s.log.Error("cannot make a token", "error", err)
		writeError(w, http.StatusInternalServerError, "cannot start sign-in")
		return
	}

	attempt, err := s.store.CreateAttempt(ctx, email, deviceID, hash, linkLifetime)
	if err != nil {
		s.log.Error("cannot record the attempt", "error", err)
		writeError(w, http.StatusInternalServerError, "cannot start sign-in")
		return
	}
	response.AttemptID = attempt.ID.String()

	link := s.baseURL + "/a?t=" + token
	if _, err := s.mail.SendSignInLink(email, link, linkLifetime); err != nil {
		// Logged without the address. The person will press send again, and the
		// attempt already recorded simply goes unused.
		s.log.Error("could not send the sign-in link", "error", err)
	}

	writeJSON(w, http.StatusOK, response)
}

type pollRequest struct {
	AttemptID string `json:"attempt_id"`
	DeviceID  string `json:"device_id"`
}

// authPoll is how a phone learns that a link was opened somewhere else.
//
// The application asks; nothing is pushed to it. That is what makes following
// the link on a laptop work without the phone and the laptop knowing about each
// other.
func (s *Server) authPoll(w http.ResponseWriter, r *http.Request) {
	var req pollRequest
	if err := decode(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "request could not be read")
		return
	}

	attemptID, err1 := uuid.Parse(req.AttemptID)
	deviceID, err2 := uuid.Parse(req.DeviceID)
	if err1 != nil || err2 != nil {
		writeError(w, http.StatusBadRequest, "request is incomplete")
		return
	}

	outcome, err := s.store.PollAttempt(r.Context(), attemptID, deviceID)
	if errors.Is(err, store.ErrNoSuchAttempt) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "expired"})
		return
	}
	if err != nil {
		s.log.Error("cannot poll the attempt", "error", err)
		writeError(w, http.StatusInternalServerError, "cannot check sign-in")
		return
	}

	switch {
	case outcome.Confirmed:
		writeJSON(w, http.StatusOK, map[string]any{
			"status":     "confirmed",
			"account_id": outcome.AccountID.String(),
		})
	case outcome.Expired:
		writeJSON(w, http.StatusOK, map[string]any{"status": "expired"})
	default:
		writeJSON(w, http.StatusOK, map[string]any{"status": "pending"})
	}
}

// authConfirm is the page a person lands on from their mailbox.
//
// It answers in HTML because a browser opens it, and it says as little as
// possible: an unusable link and a link for somebody else's account produce the
// same page, since the person reading it can do nothing with the difference and
// an attacker could.
func (s *Server) authConfirm(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("t")
	if token == "" {
		writePage(w, http.StatusBadRequest, pageFailed)
		return
	}

	_, err := s.store.ConfirmAttempt(r.Context(), auth.HashToken(token))
	if errors.Is(err, store.ErrNoSuchAttempt) {
		writePage(w, http.StatusGone, pageFailed)
		return
	}
	if err != nil {
		s.log.Error("cannot confirm", "error", err)
		writePage(w, http.StatusInternalServerError, pageFailed)
		return
	}

	writePage(w, http.StatusOK, pageDone)
}

func writePage(w http.ResponseWriter, status int, body string) {
	w.Header().Set("content-type", "text/html; charset=utf-8")
	// Nothing about this page should be kept by anything in between: the URL
	// carries the secret, and a cached copy is a copy of it.
	w.Header().Set("cache-control", "no-store")
	w.Header().Set("referrer-policy", "no-referrer")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

const (
	// How long a link works. Long enough to walk to a computer, short enough
	// that a message sitting in a mailbox stops being a key.
	linkLifetime = 15 * time.Minute

	// How long before the application offers to send another. Shorter than the
	// time a message usually takes would train people to press it twice.
	resendAfter = 60 * time.Second

	rateWindow           = time.Hour
	maxAttemptsPerWindow = 5
)

const pageHead = `<!doctype html><meta charset="utf-8">` +
	`<meta name="viewport" content="width=device-width,initial-scale=1">` +
	`<title>Вход</title><style>` +
	`body{font:16px/1.6 system-ui,sans-serif;margin:0;padding:4rem 1.5rem;color:#243;background:#fbfbf9}` +
	`main{max-width:26rem;margin:0 auto;text-align:center}` +
	`h1{font-size:1.3rem;margin:0 0 .6rem}p{color:#465;margin:.6rem 0}` +
	`</style><main>`

const pageDone = pageHead +
	`<h1>Готово</h1><p>Вернитесь в приложение — оно уже знает, что вход подтверждён.</p>` +
	`<p>Эту страницу можно закрыть.</p></main>`

const pageFailed = pageHead +
	`<h1>Ссылка не сработала</h1>` +
	`<p>Она действует ограниченное время и срабатывает один раз.</p>` +
	`<p>Откройте приложение и запросите вход заново.</p></main>`
