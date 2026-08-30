package appupdate

import "testing"

func TestVerdictBoundaries(t *testing.T) {
	p := Policy{LatestVersionCode: 8, MinSupportedVersionCode: 5}
	cases := []struct {
		version int
		want    Status
	}{
		{4, Required},
		{5, Optional},
		{7, Optional},
		{8, Current},
		{9, Current},
	}
	for _, c := range cases {
		if got := p.Verdict(c.version); got != c.want {
			t.Errorf("version %d: got %s, want %s", c.version, got, c.want)
		}
	}
}

func TestPolicyValidation(t *testing.T) {
	valid := Policy{
		LatestVersionCode:       2,
		LatestVersionName:       "0.2.0",
		MinSupportedVersionCode: 1,
		Channels: map[string]Artifact{
			DirectAPK: {
				URL:    "https://simple-vpn.download/download/releases/0.2.0/simple-vpn-0.2.0.apk",
				SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			},
		},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid policy rejected: %v", err)
	}

	cases := []Policy{
		{LatestVersionCode: 1, LatestVersionName: "0.1.0", MinSupportedVersionCode: 2},
		{LatestVersionCode: 2, LatestVersionName: "", MinSupportedVersionCode: 1},
		{
			LatestVersionCode: 2, LatestVersionName: "0.2.0", MinSupportedVersionCode: 1,
			Channels: map[string]Artifact{DirectAPK: {URL: "http://example.test/a.apk", SHA256: valid.Channels[DirectAPK].SHA256}},
		},
		{
			LatestVersionCode: 2, LatestVersionName: "0.2.0", MinSupportedVersionCode: 1,
			Channels: map[string]Artifact{DirectAPK: {URL: valid.Channels[DirectAPK].URL, SHA256: "ABC"}},
		},
	}
	for i, policy := range cases {
		if err := policy.Validate(); err == nil {
			t.Errorf("invalid policy %d was accepted", i)
		}
	}
}
