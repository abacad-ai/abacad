package discovery

import "testing"

func TestUnescapeDNS(t *testing.T) {
	cases := map[string]string{
		`Li\226\128\153s\ Mac\ mini`: "Li’s Mac mini",
		"plain-name":                 "plain-name",
		`a\256b`:                     `a\256b`,
	}
	for in, want := range cases {
		if got := unescapeDNS(in); got != want {
			t.Errorf("unescapeDNS(%q) = %q, want %q", in, got, want)
		}
	}
}
