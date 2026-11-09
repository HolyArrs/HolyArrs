package updates

import (
	"context"
	"net/http"
	"net/http/httptest"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"testing"
)

// The "what is new" link is kept only on the project's own release pages.
func TestUpdateLinkPinnedToProjectReleases(t *testing.T) {
	for link, keep := range map[string]bool{
		"https://github.com/HolyArrs/HolyArrs/releases/tag/v1.2.0":          true,
		"https://github.com/HolyArrs/HolyArrs/releases/":                    true,
		"https://example.com/r/v1.2.0":                                      false,
		"http://github.com/HolyArrs/HolyArrs/releases/tag/v1":               false,
		"https://github.com.evil.example/HolyArrs/HolyArrs/releases/tag/v1": false,
		"https://github.com/HolyArrs/HolyArrs/releasesX/tag/v1":             false,
		"https://github.com/HolyArrs/HolyArrs/releases/../../evil":          false,
		"https://user@github.com/HolyArrs/HolyArrs/releases/tag/v1":         false,
		"https://github.com:444/HolyArrs/HolyArrs/releases/tag/v1":          false,
		"javascript:alert(1)": false,
	} {
		w := newWorld(t)
		f := newFeed(t)
		w.e.ReleaseFeed = f.srv.URL
		f.body = `{"tag_name": "v1.2.0", "html_url": "` + link + `"}`
		i := w.e.CheckUpdate(context.Background())
		if (i.Link != "") != keep || (keep && i.Link != link) {
			t.Errorf("%s: link %q", link, i.Link)
		}
	}
}

func TestNewerPreRelease(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"v1.0.0-rc1", "v1.0.0", false},
		{"v1.0.0", "v1.0.0-rc1", true},
		{"v1.0.1-rc1", "v1.0.0", true},
		{"v1.0.0-rc2", "v1.0.0-rc1", false},
		{"v1.1.0", "v1.0.0", true},
	} {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%s, %s) = %v", c.a, c.b, got)
		}
	}
}

// A pre-release never gives a notice unless the owner opts in.
func TestPreReleaseNoticeOnlyWhenOptedIn(t *testing.T) {
	w := newWorld(t)
	f := newFeed(t)
	w.e.ReleaseFeed = f.srv.URL
	f.body = `{"tag_name": "v99.0.0-anything"}`
	if c := w.run(t)["update"]; c.State != OK {
		t.Fatalf("pre-release gave a notice: %+v", c)
	}
	if i := w.e.Update(context.Background()); i.Newer {
		t.Fatalf("newer: %+v", i)
	}
	on := true
	st, err := ReadSettings(context.Background(), w.s)
	if err != nil {
		t.Fatal(err)
	}
	st.UpdatePrerelease = &on
	if err := SaveSettings(context.Background(), w.s, st, domain.System); err != nil {
		t.Fatal(err)
	}
	if st2, _ := ReadSettings(context.Background(), w.s); st2.UpdatePrerelease == nil || !*st2.UpdatePrerelease {
		t.Fatalf("not saved: %+v", st2)
	}
	if i := w.e.Update(context.Background()); !i.Newer {
		t.Fatalf("opted in, no notice: %+v", i)
	}
}

// Redirects: at most 3, never to another scheme.
func TestUpdateRedirects(t *testing.T) {
	hops := 0
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/loop":
			hops++
			http.Redirect(w, r, srv.URL+"/loop", http.StatusFound)
		case "/two":
			http.Redirect(w, r, srv.URL+"/one", http.StatusFound)
		case "/one":
			http.Redirect(w, r, srv.URL+"/feed", http.StatusFound)
		case "/scheme":
			http.Redirect(w, r, "https://"+r.Host+"/feed", http.StatusFound)
		default:
			_, _ = w.Write([]byte("v1.5.0"))
		}
	}))
	defer srv.Close()
	w := newWorld(t)
	w.e.ReleaseFeed = srv.URL + "/two"
	if i := w.e.CheckUpdate(context.Background()); i.Latest != "v1.5.0" {
		t.Fatalf("two hops: %+v", i)
	}
	w.e.ReleaseFeed = srv.URL + "/loop"
	if i := w.e.CheckUpdate(context.Background()); i.Problem == "" || hops > 4 {
		t.Fatalf("loop: %d %+v", hops, i)
	}
	w.e.ReleaseFeed = srv.URL + "/scheme"
	if i := w.e.CheckUpdate(context.Background()); i.Problem == "" {
		t.Fatalf("scheme change followed: %+v", i)
	}
}
