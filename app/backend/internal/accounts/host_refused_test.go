package accounts

import (
	"strings"
	"testing"
)

// A save refused for an unknown host name says why and where to fix it,
// and the block itself stays.
func TestUnknownHostRefusalSaysCauseAndFix(t *testing.T) {
	f := gated(t, Options{AllowedHosts: []string{"holyarrs.example"}})
	ownerOf(t, f)
	resp, out := loginOn(t, f, client(t), "media.unknown.example", nil)
	msg := errorOf(out)
	if resp.StatusCode != 403 {
		t.Fatalf("unknown host write: %d, want 403", resp.StatusCode)
	}
	want := "Saving is blocked because HolyArrs doesn't know the name media.unknown.example. Open HolyArrs at its local address and add this name under Settings > Sign-in and security > Allowed host names, or add it to HOLYARRS_ALLOWED_HOSTS."
	if msg != want {
		t.Errorf("message = %q\nwant      %q", msg, want)
	}
}

func TestHostRefusedMsgEchoesSafeName(t *testing.T) {
	if m := hostRefusedMsg("Evil<b>.example:443"); !strings.Contains(m, "the name evil. Open") {
		t.Errorf("unsafe name not cut: %q", m)
	}
}
