package store

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Columns that are allowed to hold a hostname, each with the reason.
//
// The point of naming them one by one is that adding another one has to be a
// decision somebody writes down here, rather than a column that appears in a
// migration and is never noticed. A schema is the only privacy guarantee that
// cannot be undone by a mistake in a handler.
var namedOnPurpose = map[string]string{
	// The addresses this service tells clients to try. Ours, published, and
	// the same for everybody.
	"bootstrap_entries.host":        "our own way in",
	"bootstrap_entries.server_name": "our own name in the handshake",

	// The name a certificate is issued for. Ours.
	"certificate_issues.name": "the domain we asked to have certified",

	// The addresses we test. The service refuses to store one that is not in
	// our own node list, so this cannot become a list of places people go.
	"endpoint_probes.target": "our own way in, being checked",
}

// Words that must never name a column.
//
// Not a stylistic rule: each of these is a category the stage forbids storing,
// and a column with one of these names is the only way such a thing gets kept
// by accident.
var forbidden = []string{
	"sni", "url", "uri", "dns", "query",
	"destination", "dest_", "_dest", "remote_addr", "client_ip",
	"source_ip", "src_ip", "peer_ip", "ip_address", "addr",
	"referer", "referrer", "user_agent", "visited", "browsing",
	"domain", "hostname", "host", "site", "target",
}

var columnLine = regexp.MustCompile(`^\s{2,}("?[a-z_][a-z0-9_]*"?)\s+[a-z]`)
var createTable = regexp.MustCompile(`create table if not exists ([a-z_.]+)`)

// TestSchemaNamesNothingItMustNotKeep reads every migration and refuses a
// column whose name belongs to a category this service is not allowed to keep.
func TestSchemaNamesNothingItMustNotKeep(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("migrations", "*.sql"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no migrations found: %v", err)
	}

	for _, file := range files {
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("cannot read %s: %v", file, err)
		}

		table := ""
		for _, line := range strings.Split(string(body), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "--") {
				continue
			}
			if found := createTable.FindStringSubmatch(strings.ToLower(trimmed)); found != nil {
				table = found[1]
				if dot := strings.LastIndex(table, "."); dot >= 0 {
					table = table[dot+1:]
				}
				continue
			}
			if table == "" {
				continue
			}
			if trimmed == ");" || trimmed == ")" {
				table = ""
				continue
			}

			found := columnLine.FindStringSubmatch(strings.ToLower(line))
			if found == nil {
				continue
			}
			column := strings.Trim(found[1], `"`)

			// Constraint keywords look like columns to a regular expression.
			switch column {
			case "primary", "unique", "check", "constraint", "foreign", "exclude":
				continue
			}

			qualified := table + "." + column
			if _, ok := namedOnPurpose[qualified]; ok {
				continue
			}
			for _, word := range forbidden {
				if strings.Contains(column, word) {
					t.Errorf(
						"%s declares %s, and %q is a category this service must not store.\n"+
							"If this column really holds one of our own addresses, say so in namedOnPurpose.",
						filepath.Base(file), qualified, word)
				}
			}
		}
	}
}

// TestTrafficAndUsersStayApart checks the one join that must never exist.
//
// Traffic by kind carries no user; usage by user carries no kind. Either alone
// answers a question the Business Owner asked. Together they would be a
// profile of what each person does, which is the thing this whole schema is
// arranged to make impossible.
func TestTrafficAndUsersStayApart(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("migrations", "0010_observability.sql"))
	if err != nil {
		t.Fatalf("cannot read the observability migration: %v", err)
	}

	classes := tableBody(string(body), "metrics.traffic_classes")
	if classes == "" {
		t.Fatal("metrics.traffic_classes is missing")
	}
	for _, word := range []string{"analytics_id", "account", "credential", "device", "user"} {
		if strings.Contains(classes, word) {
			t.Errorf("traffic by kind must not carry %q; that pair is a profile", word)
		}
	}

	usage := tableBody(string(body), "metrics.account_usage")
	if usage == "" {
		t.Fatal("metrics.account_usage is missing")
	}
	if strings.Contains(usage, "class") {
		t.Error("usage per user must not carry a class; that pair is a profile")
	}
	if strings.Contains(usage, "account_id") {
		t.Error("usage must be keyed by the epoch pseudonym, not by the account")
	}
}

func tableBody(sql, name string) string {
	start := strings.Index(sql, "create table if not exists "+name)
	if start < 0 {
		return ""
	}
	end := strings.Index(sql[start:], "\n);")
	if end < 0 {
		return ""
	}
	return sql[start : start+end]
}
