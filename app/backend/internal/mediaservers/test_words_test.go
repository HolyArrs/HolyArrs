package mediaservers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// wordFileKnows reports whether line is one of the fixed English lines the
// web app can show in the picked language (a slot stands for any text).
func wordFileKnows(t *testing.T, line string) bool {
	t.Helper()
	raw, err := os.ReadFile("../../../frontend/src/lib/core/i18n/messages/en/server-lines.json")
	if err != nil {
		t.Fatal(err)
	}
	var words map[string]string
	if err := json.Unmarshal(raw, &words); err != nil {
		t.Fatal(err)
	}
	slot := regexp.MustCompile(`\\{\w+\\}`)
	for _, w := range words {
		if w == line || (strings.Contains(w, "{") && regexp.MustCompile("^"+slot.ReplaceAllString(regexp.QuoteMeta(w), "(.+?)")+"$").MatchString(line)) {
			return true
		}
	}
	return false
}

// What a failed Test of a Jellyfin or Emby server says is a fixed line the
// word file holds, so Settings shows it in the picked language.
func TestFailedTestLinesAreInTheWordFile(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	l := &Linker{Store: w.s, Vault: w.v}
	answer := func(h http.HandlerFunc) string {
		srv := httptest.NewServer(h)
		t.Cleanup(srv.Close)
		return srv.URL
	}
	gone := httptest.NewServer(http.NotFoundHandler())
	goneURL := gone.URL
	gone.Close()
	no := false
	for _, kind := range []string{"jellyfin", "emby"} {
		cases := []struct {
			name, want string
			in         Input
		}{
			{"gone", "Nothing answers on that port. Check the port, and that the app is running.", Input{URL: goneURL}},
			{"token", "Key or password refused. Check them, then test again.", Input{URL: answer(func(rw http.ResponseWriter, _ *http.Request) { rw.WriteHeader(http.StatusUnauthorized) })}},
			{"status", "the server answered 500", Input{URL: answer(func(rw http.ResponseWriter, _ *http.Request) { rw.WriteHeader(http.StatusInternalServerError) })}},
			{"big", "the server's answer was too big to read", Input{URL: answer(func(rw http.ResponseWriter, _ *http.Request) { _, _ = rw.Write(make([]byte, maxAnswer+1)) })}},
			{"cut", "reading the server answer failed", Input{URL: answer(func(rw http.ResponseWriter, _ *http.Request) {
				rw.Header().Set("Content-Length", "100")
				_, _ = rw.Write([]byte("short"))
				if c, _, err := rw.(http.Hijacker).Hijack(); err == nil {
					_ = c.Close()
				}
			})}},
			{"local", "Blocked: local network. Turn on \"Reach the local network\" if it runs at home.", Input{URL: answer(func(http.ResponseWriter, *http.Request) {}), AllowLocal: &no}},
		}
		for _, c := range cases {
			c.in.Name, c.in.Kind, c.in.Token = kind+" "+c.name, kind, "x"
			m, err := Add(ctx, w.s, w.v, c.in, domain.System)
			if err != nil {
				t.Fatalf("%s %s: add: %v", kind, c.name, err)
			}
			_, err = l.Test(ctx, m)
			if err == nil || err.Error() != c.want || !wordFileKnows(t, err.Error()) {
				t.Fatalf("%s %s: got %v, want the fixed line %q from the word file", kind, c.name, err, c.want)
			}
		}
		m, err := Add(ctx, w.s, w.v, Input{Name: kind + " keyless", Kind: kind, URL: goneURL, Token: "x"}, domain.System)
		if err != nil {
			t.Fatal(err)
		}
		m.TokenSecretID = nil
		if _, err := l.Test(ctx, m); err == nil || !wordFileKnows(t, err.Error()) {
			t.Fatalf("%s without a key: the word file lacks %v", kind, err)
		}
	}
}
