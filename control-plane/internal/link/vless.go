// Package link turns a node and a credential into the address a third-party
// client understands.
//
// One format, written once, in a package with no database and no network: what
// a link says has to be checkable by reading it back, and that is only
// possible while it is a pure function of its inputs.
package link

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"

	"download.simplevpn/control-plane/internal/document"
)

// ErrUnsupported is returned for a transport that has no link form here.
//
// Returned rather than guessed. A link that looks right and connects to
// nothing is worse than no link: the person pastes it, it fails, and there is
// nothing in the failure that says the fault was ours.
var ErrUnsupported = errors.New("this transport has no link form")

// For builds the vless:// address for one credential on one node.
//
// The connect address is the node's own name rather than its address, and that
// differs on purpose from what the application is given. The application must
// be handed an address, because a phone cannot resolve a name before its own
// tunnel exists (ADR-028). A router has no such problem, and a name survives
// the node changing address, which a pasted link otherwise would not.
func For(node document.Node, credential, label string) (string, error) {
	if node.Transport.Kind != "vless-ws-tls" {
		return "", fmt.Errorf("%w: %s", ErrUnsupported, node.Transport.Kind)
	}
	if credential == "" {
		return "", errors.New("no credential to put in the link")
	}

	serverName := text(node.Transport.Params, "server_name")
	if serverName == "" {
		return "", errors.New("the node has no name to connect to")
	}

	host := text(node.Transport.Params, "host_header")
	if host == "" {
		host = serverName
	}

	path := text(node.Transport.Params, "path")
	if path == "" {
		return "", errors.New("the node has no path for the tunnel")
	}

	fingerprint := text(node.Transport.Params, "fingerprint")
	if fingerprint == "" {
		fingerprint = "chrome"
	}

	port := node.Port
	if port == 0 {
		port = 443
	}

	query := url.Values{}
	// Encryption is none for VLESS and has to be said anyway: the protocol
	// carries the field, and clients reject a link that omits it.
	query.Set("encryption", "none")
	query.Set("security", "tls")
	query.Set("sni", serverName)
	query.Set("fp", fingerprint)
	query.Set("type", "ws")
	query.Set("host", host)
	query.Set("path", path)

	address := url.URL{
		Scheme:   "vless",
		User:     url.User(credential),
		Host:     serverName + ":" + strconv.Itoa(port),
		RawQuery: query.Encode(),
		Fragment: label,
	}
	return address.String(), nil
}

func text(params map[string]any, key string) string {
	if params == nil {
		return ""
	}
	value, ok := params[key].(string)
	if !ok {
		return ""
	}
	return value
}
