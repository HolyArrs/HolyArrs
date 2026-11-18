package accounts

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/startup"
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

// said is the line a JSON answer holds for a person: "message" or "error".
func said(t *testing.T, out string) string {
	t.Helper()
	var a struct{ Message, Error string }
	if err := json.Unmarshal([]byte(out), &a); err != nil {
		t.Fatalf("answer %q: %v", out, err)
	}
	return a.Message + a.Error
}

// The lines a Jellyfin or Emby address Test answers with are fixed lines the
// word file holds, so Settings shows them in the picked language.
func TestMediaServerTestLinesAreInTheWordFile(t *testing.T) {
	gone := httptest.NewServer(http.NotFoundHandler())
	goneURL := gone.URL
	gone.Close()
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "gone", 418) }))
	t.Cleanup(other.Close)
	f := gated(t, Options{})
	own := ownerOf(t, f)
	for kind, word := range map[string]string{"jellyfin": "Jellyfin", "emby": "Emby"} {
		test := func(u string) string {
			b, _ := json.Marshal(map[string]string{"url": u})
			_, out := own.do(t, f, "POST", "/api/v1/auth/media-server/"+kind+"/test", string(b))
			return said(t, out)
		}
		if got := test(goneURL); got != "The server did not answer at this address." || !wordFileKnows(t, got) {
			t.Fatalf("%s, nothing there: %q", kind, got)
		}
		if got := test(other.URL); got != "Something answered, but it does not look like "+word+"." || !wordFileKnows(t, got) {
			t.Fatalf("%s, something else there: %q", kind, got)
		}
		if got := test("gopher://x"); !wordFileKnows(t, got) {
			t.Fatalf("%s, odd address: %q", kind, got)
		}
	}
}

// The first-run refusals of a Jellyfin or Emby sign-in are fixed lines too.
func TestSetupMediaServerLinesAreInTheWordFile(t *testing.T) {
	t.Setenv("HOLYARRS_FLAGS", strings.Join(startup.HeldFeatures(), ","))
	gone := httptest.NewServer(http.NotFoundHandler())
	goneURL := gone.URL
	gone.Close()
	other := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(other.Close)
	for kind, word := range map[string]string{"jellyfin": "Jellyfin", "emby": "Emby"} {
		ms := fakeSetupServer(t)
		f := gated(t, Options{})
		try := func(u, name, pass string) string {
			b, _ := json.Marshal(map[string]string{"url": u, "name": name, "password": pass})
			_, out := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/setup/media-server/"+kind, string(b), nil)
			return said(t, out)
		}
		if got := try(other.URL, "boss", "right"); got != "that address did not answer like "+word+"; check it and try again" || !wordFileKnows(t, got) {
			t.Fatalf("%s, something else there: %q", kind, got)
		}
		for _, got := range []string{try(goneURL, "boss", "right"), try(ms.URL, "boss", "wrong"), try(ms.URL, "mum", "right")} {
			if !wordFileKnows(t, got) {
				t.Fatalf("%s: the word file lacks %q", kind, got)
			}
		}
	}
}
