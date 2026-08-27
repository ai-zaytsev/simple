package api

import (
	_ "embed"
	"net/http"
)

//go:embed panel.html
var panelPage []byte

// panel serves the page that shows the numbers.
//
// The page itself carries nothing: it is markup and script, and it holds no
// data until somebody gives it the secret. That is why it can be served
// without a check while the numbers behind it cannot - and why losing the URL
// costs nothing.
func (s *Server) panel(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("content-type", "text/html; charset=utf-8")
	w.Header().Set("cache-control", "no-store")
	w.Header().Set("referrer-policy", "no-referrer")

	// The page talks to this service and nowhere else. Said out loud so that a
	// mistake in the script cannot turn an operator's browser into a way out
	// for anything it is showing.
	w.Header().Set("content-security-policy",
		"default-src 'none'; connect-src 'self'; style-src 'unsafe-inline'; script-src 'unsafe-inline'")
	_, _ = w.Write(panelPage)
}
