package webserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// /login gets the sign-in seed as a JSON data block in place of the mark;
// other pages and an empty seed leave the page as it is, and the script
// policy does not change.
func TestLoginSeedOnlyOnLogin(t *testing.T) {
	page := []byte("<head>" + LoginSeedMark + "<script>a()</script></head>")
	seed := []byte(`{"x":"\u003c/script\u003e"}`)
	SetLoginSeed(func(*http.Request) []byte { return seed })
	t.Cleanup(func() { SetLoginSeed(func(*http.Request) []byte { return nil }) })
	got := string(withLoginSeed(page, httptest.NewRequest("GET", "/holyarrs/login", nil)))
	if !strings.Contains(got, `<script type="application/json" id="holyarrs-signin-inline">`+string(seed)+`</script>`) || strings.Contains(got, LoginSeedMark) {
		t.Fatal(got)
	}
	if CSP([]byte(got)) != CSP(page) {
		t.Fatal("policy changed")
	}
	if got := string(withLoginSeed(page, httptest.NewRequest("GET", "/library", nil))); got != string(page) {
		t.Fatal(got)
	}
	SetLoginSeed(func(*http.Request) []byte { return nil })
	if got := string(withLoginSeed(page, httptest.NewRequest("GET", "/login", nil))); got != string(page) {
		t.Fatal(got)
	}
}
