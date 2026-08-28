package link

import (
	"errors"
	"net/url"
	"strings"
	"testing"

	"download.simplevpn/control-plane/internal/document"
)

func node() document.Node {
	return document.Node{
		Alias: "n1",
		Host:  "203.0.113.10",
		Port:  443,
		Transport: document.Transport{
			Kind: "vless-ws-tls",
			Params: map[string]any{
				"server_name": "6047864.xyz",
				"host_header": "6047864.xyz",
				"path":        "/a7f3c1d9e2",
				"fingerprint": "chrome",
			},
		},
	}
}

// A link is only useful if somebody else's client can read it, and the only
// way to check that here is to read it back apart.
//
// Every parameter is compared, not just the shape of the string: a link that
// parses and carries the wrong path connects to a node that answers with the
// cover site, and the person sees a working website instead of an error.
func TestTheLinkSaysWhatTheNodeNeeds(t *testing.T) {
	built, err := For(node(), "8f14e45f-ceea-467a-9c7c-5e1a1b2c3d4e", "Телевизор")
	if err != nil {
		t.Fatalf("cannot build a link: %v", err)
	}

	parsed, err := url.Parse(built)
	if err != nil {
		t.Fatalf("the link does not parse: %v (%s)", err, built)
	}

	if parsed.Scheme != "vless" {
		t.Errorf("scheme is %q", parsed.Scheme)
	}
	if parsed.User.Username() != "8f14e45f-ceea-467a-9c7c-5e1a1b2c3d4e" {
		t.Errorf("the credential is %q", parsed.User.Username())
	}

	// The name, not the address. A pasted link outlives the node changing
	// address only if it names the node rather than points at it.
	if parsed.Host != "6047864.xyz:443" {
		t.Errorf("connects to %q, which is not the node's name", parsed.Host)
	}

	want := map[string]string{
		"encryption": "none",
		"security":   "tls",
		"sni":        "6047864.xyz",
		"fp":         "chrome",
		"type":       "ws",
		"host":       "6047864.xyz",
		"path":       "/a7f3c1d9e2",
	}
	got := parsed.Query()
	for key, value := range want {
		if got.Get(key) != value {
			t.Errorf("%s is %q, want %q", key, got.Get(key), value)
		}
	}

	if parsed.Fragment != "Телевизор" {
		t.Errorf("the label is %q", parsed.Fragment)
	}
}

// The path begins with a slash and the label may contain spaces and Cyrillic.
// Both have to survive the round trip, and both are where a hand-built string
// would have gone wrong.
func TestAwkwardValuesSurvive(t *testing.T) {
	n := node()
	n.Transport.Params["path"] = "/tunnel/deep path?x=1"

	built, err := For(n, "8f14e45f-ceea-467a-9c7c-5e1a1b2c3d4e", "Роутер в гостиной")
	if err != nil {
		t.Fatalf("cannot build a link: %v", err)
	}

	// The raw form must not carry a bare space or a second question mark:
	// either one truncates the link when it is pasted into a client.
	after := built[strings.Index(built, "?"):]
	if strings.Contains(after, " ") {
		t.Errorf("an unescaped space made it into the query: %s", built)
	}

	parsed, err := url.Parse(built)
	if err != nil {
		t.Fatalf("the link does not parse: %v (%s)", err, built)
	}
	if parsed.Query().Get("path") != "/tunnel/deep path?x=1" {
		t.Errorf("the path came back as %q", parsed.Query().Get("path"))
	}
	if parsed.Fragment != "Роутер в гостиной" {
		t.Errorf("the label came back as %q", parsed.Fragment)
	}
}

// A transport with no link form must say so rather than produce something
// plausible. A link that looks right and connects to nothing is worse than no
// link: nothing in the failure says the fault was ours.
func TestAnUnknownTransportIsRefused(t *testing.T) {
	n := node()
	n.Transport.Kind = "vless-reality"

	if _, err := For(n, "8f14e45f-ceea-467a-9c7c-5e1a1b2c3d4e", "TV"); !errors.Is(err, ErrUnsupported) {
		t.Errorf("got %v, want ErrUnsupported", err)
	}
}

func TestAnIncompleteNodeIsRefused(t *testing.T) {
	cases := []struct {
		name    string
		break_  func(*document.Node)
		orEmpty string
	}{
		{"no name to connect to", func(n *document.Node) {
			delete(n.Transport.Params, "server_name")
		}, ""},
		{"no path", func(n *document.Node) {
			delete(n.Transport.Params, "path")
		}, ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			n := node()
			c.break_(&n)
			if _, err := For(n, "8f14e45f-ceea-467a-9c7c-5e1a1b2c3d4e", "TV"); err == nil {
				t.Error("an unusable node produced a link anyway")
			}
		})
	}

	if _, err := For(node(), "", "TV"); err == nil {
		t.Error("a link was built with no credential in it")
	}
}

// Two devices must never be handed the same link, which is the whole of what
// "no shared VIP key" means where it can be checked.
func TestTwoDevicesGetTwoLinks(t *testing.T) {
	television, err := For(node(), "8f14e45f-ceea-467a-9c7c-5e1a1b2c3d4e", "Телевизор")
	if err != nil {
		t.Fatal(err)
	}
	router, err := For(node(), "1c8e7a52-31bd-4f6a-88a1-9d0f2e4b6c7a", "Роутер")
	if err != nil {
		t.Fatal(err)
	}
	if television == router {
		t.Error("two devices were handed the same link")
	}
}
