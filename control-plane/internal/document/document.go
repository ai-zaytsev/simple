// Package document defines what the Control Plane says and the client
// believes. The shapes here are the contract from
// docs/architecture/remote-config.md, and changing one is a change to every
// installed application, not a refactor.
package document

// Transport is an envelope, not a set of fields.
//
// Everything specific to a protocol lives inside Params, so the plan schema
// knows nothing about any particular transport beyond the string naming it.
// That is what allows a second transport to be introduced later without
// changing the schema, which cannot be done retroactively: it would require
// every installation to update at the same moment.
type Transport struct {
	Kind   string         `json:"kind"`
	Params map[string]any `json:"params"`
}

// Node is what a client needs to reach one endpoint, and nothing more.
//
// Alias is opaque on purpose. Not an index, not a hostname, not a region: it
// exists to correlate telemetry and must not let anyone infer how large the
// fleet is or where it sits.
//
// Host is an address rather than a name, and that is a requirement rather than
// a preference. See ADR-028: a name would have to be resolved before the
// tunnel exists, which on a phone cannot be done, because the application
// excludes itself from its own tunnel while the system resolver points inside
// it.
type Node struct {
	Alias     string    `json:"alias"`
	Host      string    `json:"host"`
	Port      int       `json:"port"`
	Transport Transport `json:"transport"`
}

// DNS and Routing are given by the server, never chosen by the client.
type DNS struct {
	Mode    string   `json:"mode"`
	Servers []string `json:"servers"`
}

// Routing is the whole of what the client is told about where traffic goes.
//
// Five lists rather than one, because the product rule is an order and an
// order has to survive the journey. A single list of rules with a priority
// field would let one careless row put "everything direct" above the explicit
// ones; separate lists make the order a property of the schema, applied the
// same way by every client that ever reads it.
//
// DirectApps is not a route but an exclusion: those applications never enter
// the tunnel at all. It is the only rule Android can enforce per application,
// and it is the one banking applications need, because they check the address
// they are seen from rather than the names they talk to.
type Routing struct {
	Profile string `json:"profile"`

	DirectApps    []string `json:"direct_apps"`
	DirectDomains []string `json:"direct_domains"`
	DirectIPs     []string `json:"direct_ips"`
	ProxyDomains  []string `json:"proxy_domains"`
	ProxyIPs      []string `json:"proxy_ips"`

	// RussiaDirect decides what happens to everything no explicit rule
	// mentions: Russian addresses and names go straight out, the rest goes
	// through the tunnel. A switch rather than an assumption, because the day
	// this product serves somebody outside Russia it has to be turned off, and
	// that day must not need a release.
	RussiaDirect bool `json:"russia_direct"`
}

// Policy carries the numbers a client would otherwise invent for itself:
// timeouts, how many failures before moving to a reserve, how long to wait
// between attempts. A client that picks these is a client making decisions.
type Policy struct {
	ConnectTimeoutMS     int   `json:"connect_timeout_ms"`
	FailoverAfterFailure int   `json:"failover_after_failures"`
	ReconnectBackoffMS   []int `json:"reconnect_backoff_ms"`
	PlanRefreshAfterS    int   `json:"plan_refresh_after_s"`
	TelemetrySampling    float64 `json:"telemetry_sampling"`
	ProbeIntervalS       int   `json:"probe_interval_s"`
}

// Plan is issued to one account and device.
//
// Reserves are part of the shape rather than an optional extra: a plan with an
// empty reserve list leaves a user waiting for a human the moment one node
// stops answering.
type Plan struct {
	V           int      `json:"v"`
	PlanID      string   `json:"plan_id"`
	Seq         int64    `json:"seq"`
	IssuedAt    string   `json:"issued_at"`
	ExpiresAt   string   `json:"expires_at"`
	AccountTier string   `json:"account_tier"`
	Primary     Node     `json:"primary"`
	Reserves    []Node   `json:"reserves"`
	DNS         DNS      `json:"dns"`
	Routing     Routing  `json:"routing"`
	Policy      Policy   `json:"policy"`
}

// Config is issued to everyone, and is how the fleet is steered without
// reissuing individual plans.
type Config struct {
	V                     int            `json:"v"`
	Seq                   int64          `json:"seq"`
	IssuedAt              string         `json:"issued_at"`
	MinSupportedAppVersion int           `json:"min_supported_app_version"`
	KillSwitch            KillSwitch     `json:"kill_switch"`
	Features              map[string]bool `json:"features"`

	// RefreshAfterS is how long a client may go on using this document before
	// asking again. Shorter than a plan's life on purpose: this is the only
	// path by which a client can be told to stop, and a switch that takes half
	// a day to reach anybody is not much of a switch.
	RefreshAfterS int `json:"refresh_after_s"`
}

// KillSwitch exists so that an incident can stop clients connecting without
// waiting for an application update to reach anyone.
type KillSwitch struct {
	Enabled    bool   `json:"enabled"`
	MessageKey string `json:"message_key"`
}

// BootstrapEntry is one way of reaching the Control Plane.
// BootstrapEntry is one way of reaching this service.
//
// Several, because one is a single point of recovery and the whole purpose of
// this document is not to have one. They differ deliberately: by name, by
// address, by machine, by provider and by country, so that whatever takes one
// away is unlikely to take the next.
//
// The contents are public by construction - an adversary will read this
// document. Its value is not secrecy but the speed with which entries can be
// replaced, which is why it is signed rather than hidden.
type BootstrapEntry struct {
	// Kind is how to use Host.
	//
	//   https-direct  a name, resolved normally
	//   https-ip      an address, with ServerName carried in the handshake;
	//                 this one needs no resolver at all and so survives a
	//                 poisoned or blocked one
	//   https-edge    a node that forwards the API inward; a different
	//                 machine, provider, country and registrar from the
	//                 service itself
	Kind string `json:"kind"`

	Host string `json:"host"`
	Port int    `json:"port,omitempty"`

	// ServerName is the name to present in the handshake when Host is an
	// address, and the path prefix is where an edge forwards from. Both are
	// empty for a plain name.
	ServerName string `json:"server_name,omitempty"`
	PathPrefix string `json:"path_prefix,omitempty"`

	// Weight orders the attempts. Random within weights rather than fixed:
	// one order for every client is a signature, and it puts the whole
	// installed base on whichever entry happens to be first.
	Weight int `json:"weight"`
}

// Bootstrap is the list of entry points, and is the mechanism by which the
// Control Plane can move without an application update.
//
// Its contents are public by construction: an adversary will obtain it. The
// value is not secrecy but the speed with which entries can be replaced.
type Bootstrap struct {
	V            int              `json:"v"`
	Seq          int64            `json:"seq"`
	IssuedAt     string           `json:"issued_at"`
	ValidUntil   string           `json:"valid_until"`
	Entries      []BootstrapEntry `json:"entries"`
	RefreshAfterS int             `json:"refresh_after_s"`
}
