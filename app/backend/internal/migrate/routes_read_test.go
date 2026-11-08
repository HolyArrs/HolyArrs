package migrate

import "testing"

func TestSameAddress(t *testing.T) {
	base := sameAddress("http://host:7878")
	if base == "" || sameAddress("HTTP://Host:7878/") != base {
		t.Fatalf("case or trailing slash changed the address: %q vs %q", sameAddress("HTTP://Host:7878/"), base)
	}
	for _, other := range []string{"http://host:7879", "http://host:7878/base", "http://host:7878?x=1", "https://host:7878"} {
		if sameAddress(other) == base {
			t.Errorf("%s matched %s", other, base)
		}
	}
	for _, bad := range []string{"host:7878", "", "   "} {
		if got := sameAddress(bad); got != "" {
			t.Errorf("%q gave %q", bad, got)
		}
	}
}
