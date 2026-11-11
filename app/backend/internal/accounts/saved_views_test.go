package accounts

import (
	"strings"
	"testing"
)

func myViews(t *testing.T, f *fixture, p person) map[string]any {
	t.Helper()
	resp, out := p.do(t, f, "GET", "/api/v1/my/views", "")
	if resp.StatusCode != 200 {
		t.Fatalf("get views: %d %s", resp.StatusCode, out)
	}
	var v map[string]any
	jsonOf(t, out, &v)
	return v
}

func TestMyViewsSaveAndRead(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	if v := myViews(t, f, own); len(v) != 0 {
		t.Fatalf("fresh views: %v", v)
	}
	resp, out := own.do(t, f, "PUT", "/api/v1/my/views", `{"library.view.movie":"posters","calendar.view":"list","calendar.follows":"everyone"}`)
	if resp.StatusCode != 200 || !strings.Contains(out, `"library.view.movie":"posters"`) {
		t.Fatalf("put: %d %s", resp.StatusCode, out)
	}
	v := myViews(t, f, own)
	if v["library.view.movie"] != "posters" || v["calendar.view"] != "list" || v["calendar.follows"] != "everyone" || len(v) != 3 {
		t.Fatalf("saved views: %v", v)
	}
	// A later pick replaces the earlier one; keys left out stay as they were.
	if resp, out := own.do(t, f, "PUT", "/api/v1/my/views", `{"library.view.movie":"table"}`); resp.StatusCode != 200 {
		t.Fatalf("second put: %d %s", resp.StatusCode, out)
	}
	if v := myViews(t, f, own); v["library.view.movie"] != "table" || v["calendar.view"] != "list" {
		t.Fatalf("after second put: %v", v)
	}
	// null forgets a pick, so the page opens on its default again.
	if resp, out := own.do(t, f, "PUT", "/api/v1/my/views", `{"calendar.view":null}`); resp.StatusCode != 200 {
		t.Fatalf("clear: %d %s", resp.StatusCode, out)
	}
	if v := myViews(t, f, own); v["calendar.view"] != nil || v["calendar.follows"] != "everyone" {
		t.Fatalf("after clear: %v", v)
	}
}

// The session answer carries the views, so a page opens on the saved one
// without a second ask.
func TestMyViewsComeWithTheSession(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	if _, out := own.do(t, f, "GET", "/api/v1/auth/me", ""); !strings.Contains(out, `"views":{}`) {
		t.Fatalf("me before any pick: %s", out)
	}
	own.do(t, f, "PUT", "/api/v1/my/views", `{"library.view.show":"overview"}`)
	if _, out := own.do(t, f, "GET", "/api/v1/auth/me", ""); !strings.Contains(out, `"views":{"library.view.show":"overview"}`) {
		t.Fatalf("me after a pick: %s", out)
	}
}

// Unknown keys and values that are too long are dropped quietly; the rest
// of the body is still saved.
func TestMyViewsIgnoresUnknownAndBad(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	long := `"` + strings.Repeat("x", 300) + `"`
	resp, out := own.do(t, f, "PUT", "/api/v1/my/views", `{"library.view.artist":"rows","nope":"x","library.view.other":"posters","calendar.view":`+long+`}`)
	if resp.StatusCode != 200 {
		t.Fatalf("put: %d %s", resp.StatusCode, out)
	}
	v := myViews(t, f, own)
	if v["library.view.artist"] != "rows" || len(v) != 1 {
		t.Fatalf("saved: %v", v)
	}
	for _, bad := range []string{`[1]`, `"posters"`, `{`} {
		if resp, _ := own.do(t, f, "PUT", "/api/v1/my/views", bad); resp.StatusCode != 400 {
			t.Fatalf("body %s took %d", bad, resp.StatusCode)
		}
	}
}

// Two people on one install keep their own picks.
func TestMyViewsArePerPerson(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	ana := join(t, f, invite(t, f, own, `{}`), "ana")
	own.do(t, f, "PUT", "/api/v1/my/views", `{"library.view.movie":"posters"}`)
	ana.do(t, f, "PUT", "/api/v1/my/views", `{"library.view.movie":"table","calendar.follows":"mine"}`)
	if v := myViews(t, f, own); v["library.view.movie"] != "posters" || len(v) != 1 {
		t.Fatalf("owner's views: %v", v)
	}
	if v := myViews(t, f, ana); v["library.view.movie"] != "table" || v["calendar.follows"] != "mine" {
		t.Fatalf("ana's views: %v", v)
	}
}
