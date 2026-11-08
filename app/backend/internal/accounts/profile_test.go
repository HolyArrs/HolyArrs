package accounts

import (
	"strings"
	"testing"
)

func TestProfileDisplayName(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	get := func() string {
		resp, out := own.do(t, f, "GET", "/api/v1/my/profile", "")
		if resp.StatusCode != 200 {
			t.Fatalf("get profile: %d %s", resp.StatusCode, out)
		}
		return out
	}
	if out := get(); !strings.Contains(out, `"display_name":""`) || !strings.Contains(out, `"username":"luis"`) {
		t.Fatalf("fresh profile: %s", out)
	}
	resp, out := own.do(t, f, "PUT", "/api/v1/my/profile", `{"display_name":"  Lu B.  "}`)
	if resp.StatusCode != 200 || !strings.Contains(out, `"display_name":"Lu B."`) {
		t.Fatalf("saved profile: %d %s", resp.StatusCode, out)
	}
	for _, bad := range []string{`{"display_name":"` + strings.Repeat("x", 65) + `"}`, `{"display_name":"a\u0007b"}`} {
		if resp, _ := own.do(t, f, "PUT", "/api/v1/my/profile", bad); resp.StatusCode != 400 {
			t.Fatalf("bad name %q: %d", bad, resp.StatusCode)
		}
	}
	if out := get(); !strings.Contains(out, `"display_name":"Lu B."`) {
		t.Fatalf("a refused name changed the profile: %s", out)
	}
	if _, out := own.do(t, f, "PUT", "/api/v1/my/profile", `{"display_name":""}`); !strings.Contains(out, `"display_name":""`) {
		t.Fatalf("cleared profile: %s", out)
	}
}
