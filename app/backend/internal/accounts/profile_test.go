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

// Each person edits only their own profile, and extra fields (role, name)
// in the body change nothing.
func TestProfileIsOwnOnly(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	ana := join(t, f, invite(t, f, own, `{}`), "ana")
	if resp, out := ana.do(t, f, "PUT", "/api/v1/my/profile", `{"display_name":"Ana","role":"owner","name":"root"}`); resp.StatusCode != 200 {
		t.Fatalf("put: %d %s", resp.StatusCode, out)
	}
	if _, out := own.do(t, f, "GET", "/api/v1/my/profile", ""); !strings.Contains(out, `"display_name":""`) {
		t.Fatalf("owner's profile changed: %s", out)
	}
	_, out := ana.do(t, f, "GET", "/api/v1/auth/me", "")
	if !strings.Contains(out, `"name":"ana"`) || strings.Contains(out, `"role":"owner"`) {
		t.Fatalf("profile edit changed name or role: %s", out)
	}
}

func TestMyLanguage(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	if resp, out := own.do(t, f, "PUT", "/api/v1/my/language", `{"language":"de"}`); resp.StatusCode != 200 || !strings.Contains(out, `"language":"de"`) {
		t.Fatalf("put: %d %s", resp.StatusCode, out)
	}
	for _, l := range []string{"zh-tw", "pt-pt", "nb", "he", "th"} {
		if resp, out := own.do(t, f, "PUT", "/api/v1/my/language", `{"language":"`+l+`"}`); resp.StatusCode != 200 || !strings.Contains(out, `"language":"`+l+`"`) {
			t.Fatalf("put %s: %d %s", l, resp.StatusCode, out)
		}
	}
	own.do(t, f, "PUT", "/api/v1/my/language", `{"language":"de"}`)
	if resp, _ := own.do(t, f, "PUT", "/api/v1/my/language", `{"language":"xx"}`); resp.StatusCode != 400 {
		t.Fatalf("bad language took %d", resp.StatusCode)
	}
	if _, out := own.do(t, f, "PUT", "/api/v1/my/profile", `{"display_name":"Lu"}`); !strings.Contains(out, `"language":"de"`) {
		t.Fatalf("name change lost language: %s", out)
	}
}

// The page the app opens on: Home until a person picks Explore, kept on
// their own row only, and sent with the session so the first page knows it.
func TestMyOpensOn(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	ana := join(t, f, invite(t, f, own, `{}`), "ana")
	for _, p := range []person{own, ana} {
		if _, out := p.do(t, f, "GET", "/api/v1/my/profile", ""); !strings.Contains(out, `"opens_on":"home"`) {
			t.Fatalf("fresh profile: %s", out)
		}
		if _, out := p.do(t, f, "GET", "/api/v1/auth/me", ""); !strings.Contains(out, `"opens_on":"home"`) {
			t.Fatalf("fresh session: %s", out)
		}
	}
	if resp, out := ana.do(t, f, "PUT", "/api/v1/my/opens-on", `{"opens_on":"explore","user_id":1}`); resp.StatusCode != 200 || !strings.Contains(out, `"opens_on":"explore"`) {
		t.Fatalf("put: %d %s", resp.StatusCode, out)
	}
	if _, out := ana.do(t, f, "GET", "/api/v1/auth/me", ""); !strings.Contains(out, `"opens_on":"explore"`) {
		t.Fatalf("session after the pick: %s", out)
	}
	if _, out := own.do(t, f, "GET", "/api/v1/auth/me", ""); !strings.Contains(out, `"opens_on":"home"`) {
		t.Fatalf("another person's pick changed the owner's: %s", out)
	}
	for _, bad := range []string{`{"opens_on":"downloads"}`, `{"opens_on":""}`, `{"opens_on":"/explore"}`, `{}`} {
		if resp, _ := ana.do(t, f, "PUT", "/api/v1/my/opens-on", bad); resp.StatusCode != 400 {
			t.Fatalf("bad pick %s took %d", bad, resp.StatusCode)
		}
	}
	// The name and the language keep the pick, and the pick keeps them.
	ana.do(t, f, "PUT", "/api/v1/my/profile", `{"display_name":"Ana"}`)
	ana.do(t, f, "PUT", "/api/v1/my/language", `{"language":"de"}`)
	if _, out := ana.do(t, f, "GET", "/api/v1/my/profile", ""); !strings.Contains(out, `"opens_on":"explore"`) {
		t.Fatalf("name or language change lost the pick: %s", out)
	}
	if _, out := ana.do(t, f, "PUT", "/api/v1/my/opens-on", `{"opens_on":"Home"}`); !strings.Contains(out, `"opens_on":"home"`) || !strings.Contains(out, `"display_name":"Ana"`) || !strings.Contains(out, `"language":"de"`) {
		t.Fatalf("back to Home: %s", out)
	}
}
