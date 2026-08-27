package api

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// Categories that must not appear as a field in anything this service accepts.
//
// The schema test guards what is stored; this guards what is even received. A
// field that exists on an ingest struct is a field somebody will eventually
// persist, so the shortest way to keep a destination out of the database is to
// have nowhere to put one on the way in.
var mustNotAppear = []string{
	"sni", "url", "uri", "dns", "domain", "hostname",
	"destination", "dest", "remote", "clientip", "sourceip", "srcip",
	"ipaddress", "referer", "useragent", "site", "visited", "query", "target",
	"packet", "payload", "content",
}

// Fields that name one of our own addresses, and why that is allowed.
var oursOnPurpose = map[string]string{
	"appReport.Probes.Target": "one of our own ways in, which the service checks against its own node list before storing",
}

func TestIngestStructsHaveNowhereToPutADestination(t *testing.T) {
	for _, subject := range []any{nodeSample{}, appReport{}} {
		walk(t, reflect.TypeOf(subject), reflect.TypeOf(subject).Name())
	}
}

func walk(t *testing.T, typ reflect.Type, path string) {
	t.Helper()

	for typ.Kind() == reflect.Ptr || typ.Kind() == reflect.Slice || typ.Kind() == reflect.Array {
		typ = typ.Elem()
	}
	if typ.Kind() == reflect.Map {
		walk(t, typ.Elem(), path+"[]")
		return
	}
	if typ.Kind() != reflect.Struct {
		return
	}

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		here := path + "." + field.Name

		if _, allowed := oursOnPurpose[here]; !allowed {
			flat := strings.ToLower(strings.ReplaceAll(field.Name, "_", ""))
			for _, word := range mustNotAppear {
				if strings.Contains(flat, word) {
					t.Errorf(
						"%s would let a node or a device send %q.\n"+
							"If this really is one of our own addresses, say so in oursOnPurpose.",
						here, word)
				}
			}
		}
		walk(t, field.Type, here)
	}
}

// TestUnknownFieldsAreIgnoredRatherThanKept makes sure that a node sending
// something it should not send does not get it stored by accident.
//
// The decoder drops what it does not recognise, which is the behaviour we
// want: a compromised or modified node can post whatever it likes and none of
// it reaches a column, because there is no column and no field.
func TestUnknownFieldsAreIgnoredRatherThanKept(t *testing.T) {
	raw := []byte(`{
		"at": 1756300000,
		"window_s": 60,
		"uplink_bytes": 10,
		"downlink_bytes": 20,
		"visited_domains": ["example.com"],
		"sni": "bank.example",
		"destination_ips": ["1.2.3.4"]
	}`)

	var sample nodeSample
	if err := json.Unmarshal(raw, &sample); err != nil {
		t.Fatalf("a node report with extra fields should still decode: %v", err)
	}
	if sample.UplinkBytes != 10 || sample.DownlinkBytes != 20 {
		t.Fatalf("the fields we do accept were not read: %+v", sample)
	}

	// Round-tripping shows what survived: if any of the forbidden names come
	// back out, they were kept somewhere.
	back, err := json.Marshal(sample)
	if err != nil {
		t.Fatalf("cannot re-encode: %v", err)
	}
	for _, word := range []string{"visited", "sni", "destination"} {
		if strings.Contains(strings.ToLower(string(back)), word) {
			t.Errorf("%q survived decoding; it should have been dropped", word)
		}
	}
}
