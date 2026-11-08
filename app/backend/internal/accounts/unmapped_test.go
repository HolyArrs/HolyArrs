package accounts

import (
	"strings"
	"testing"
)

// The setup page gets a yes/no field when /config is not mapped, so it can
// show a note before the owner is made. Nothing else (no volume id) is sent
// before sign-in.
func TestStateSaysConfigUnmapped(t *testing.T) {
	f := setup(t, Options{ConfigUnmapped: true})
	_, body := call(t, client(t), "GET", f.srv.URL+"/api/v1/auth/state", "", nil)
	if !strings.Contains(body, `"config_unmapped":true`) {
		t.Fatalf("state: %s", body)
	}
	f = setup(t, Options{})
	_, body = call(t, client(t), "GET", f.srv.URL+"/api/v1/auth/state", "", nil)
	if strings.Contains(body, `"config_unmapped":true`) {
		t.Fatalf("mapped: %s", body)
	}
}
