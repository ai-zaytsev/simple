package api

import "testing"

// Whether what a node serves came from the authority it is meant to use.
//
// The test authority signs with STAGING in the name, in capitals, and its
// organisation is still Let's Encrypt - so that word is the only thing
// separating the two. The first check written for this looked for "Staging"
// and read a test certificate as a real one, reporting that a phone would
// trust something no phone accepts.
//
// A check that says success on a failure is worse than no check: it ends the
// looking.
func TestTellingTheAuthoritiesApart(t *testing.T) {
	const staging = "C = US, O = Let's Encrypt, CN = (STAGING) Artificial Amaranth YE1"
	const real = "C = US, O = Let's Encrypt, CN = YE2"

	cases := []struct {
		name   string
		issuer string
		wanted string
		wrong  bool
	}{
		{"staging held, real wanted", staging, "real", true},
		{"real held, real wanted", real, "real", false},
		{"staging held, test wanted", staging, "test", false},
		{"real held, test wanted", real, "test", true},

		// Nothing served yet is not the wrong authority; it is no authority,
		// and the ordinary reasons to issue already cover it.
		{"nothing held", "", "real", false},
		{"nothing held, test wanted", "", "test", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := wrongAuthority(c.issuer, c.wanted); got != c.wrong {
				t.Errorf("got %v, want %v for %q wanting %s", got, c.wrong, c.issuer, c.wanted)
			}
		})
	}
}

// The word is matched whatever case it arrives in. Written separately because
// this is the specific thing that went wrong, and a case list can be edited
// without anybody noticing this row leaving it.
func TestStagingIsRecognisedInAnyCase(t *testing.T) {
	for _, issuer := range []string{
		"CN = (STAGING) Artificial Amaranth YE1",
		"CN = (staging) artificial amaranth",
		"CN = (Staging) Pretend Pear X1",
	} {
		if !wrongAuthority(issuer, "real") {
			t.Errorf("a test certificate was read as a real one: %q", issuer)
		}
	}
}
