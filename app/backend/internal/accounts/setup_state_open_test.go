package accounts

import (
	"strings"
	"testing"
)

// The public state says setup_open only while the owner exists and first-run setup is unfinished.
func TestStateSetupOpenFlag(t *testing.T) {
	f := setup(t, Options{})
	c := client(t)
	url := f.srv.URL + "/api/v1/auth/"
	has := func() bool {
		t.Helper()
		_, body := call(t, c, "GET", url+"state", "", nil)
		return strings.Contains(body, `"setup_open":true`)
	}
	csrf := ""
	for _, tc := range []struct {
		name string
		step func()
		want bool
	}{
		{"no owner", func() {}, false},
		{"owner, setup unfinished", func() { csrf = setupOwner(t, f, c) }, true},
		{"setup done", func() {
			if resp, _ := call(t, c, "POST", url+"setup/done", "", map[string]string{CSRFHeader: csrf}); resp.StatusCode != 200 {
				t.Fatalf("done: %d", resp.StatusCode)
			}
		}, false},
	} {
		tc.step()
		if got := has(); got != tc.want {
			t.Fatalf("%s: setup_open %v, want %v", tc.name, got, tc.want)
		}
	}
}
