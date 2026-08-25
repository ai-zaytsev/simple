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

type Routing struct {
	Profile string `json:"profile"`
	Rules   []any  `json:"rules"`
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
}

// KillSwitch exists so that an incident can stop clients connecting without
// waiting for an application update to reach anyone.
type KillSwitch struct {
	Enabled    bool   `json:"enabled"`
	MessageKey string `json:"message_key"`
}

// BootstrapEntry is one way of reaching the Control Plane.
type BootstrapEntry struct {
	Kind   string `json:"kind"`
	Host   string `json:"host"`
	Port   int    `json:"port,omitempty"`
	Weight int    `json:"weight"`
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
