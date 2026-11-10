package accounts

import (
	"encoding/json"
	"testing"
)

// The public auth state names a host saves are refused on, so the app can
// say so on arrival; an allowed name gets no such field.
func TestStateNamesARefusedHost(t *testing.T) {
	f := gated(t, Options{})
	_ = ownerOf(t, f)
	read := func(host string) map[string]any {
		resp, body := hostDo(t, client(t), "GET", f.srv.URL+"/api/v1/auth/state", host, "", nil)
		if resp.StatusCode != 200 {
			t.Fatalf("state on %s: %d %s", host, resp.StatusCode, body)
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(body), &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	if got := read("media.example.com")["host_refused"]; got != "media.example.com" {
		t.Fatalf("refused name: %v", got)
	}
	if got, ok := read("127.0.0.1")["host_refused"]; ok {
		t.Fatalf("an IP address is allowed, got %v", got)
	}
}
