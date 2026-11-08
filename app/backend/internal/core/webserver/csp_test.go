package webserver

import (
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"
)

// The policy allows the page's inline scripts by hash and nothing inline else.
func TestCSPHashesInlineScripts(t *testing.T) {
	page := []byte("<head><script>a()</script><script type=\"module\" src=\"/_app/x.js\"></script></head>")
	sum := sha256.Sum256([]byte("a()"))
	got := CSP(page)
	if !strings.Contains(got, "script-src 'self' 'sha256-"+base64.StdEncoding.EncodeToString(sum[:])+"'") {
		t.Fatal(got)
	}
	if strings.Contains(CSP(nil), "sha256-") || !strings.Contains(got, "frame-src https://www.youtube-nocookie.com") {
		t.Fatal(CSP(nil))
	}
}

// A CRLF checkout of app.html: the hash is of the text the browser sees (LF).
func TestCSPHashesNormalizedNewlines(t *testing.T) {
	sum := sha256.Sum256([]byte("a()\nb()\n"))
	if got := CSP([]byte("<script>a()\r\nb()\r</script>")); !strings.Contains(got, "'sha256-"+base64.StdEncoding.EncodeToString(sum[:])+"'") {
		t.Fatal(got)
	}
}

// Every inline script of the page gets its own hash, not just the first.
func TestCSPHashesEveryInlineScript(t *testing.T) {
	got := CSP([]byte("<head><script>a()</script></head><body><div id=\"hl\"></div><script>b()</script></body>"))
	for _, s := range []string{"a()", "b()"} {
		sum := sha256.Sum256([]byte(s))
		if !strings.Contains(got, "'sha256-"+base64.StdEncoding.EncodeToString(sum[:])+"'") {
			t.Fatal(s, got)
		}
	}
}
