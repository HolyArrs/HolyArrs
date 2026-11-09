package health

import (
	"strings"
	"testing"
)

func TestProxyWarning(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	if w := ProxyWarning(env(nil)); w != "" {
		t.Fatalf("no proxy: got %q", w)
	}
	if w := ProxyWarning(env(map[string]string{"HTTP_PROXY": "  "})); w != "" {
		t.Fatalf("blank proxy: got %q", w)
	}
	w := ProxyWarning(env(map[string]string{"HTTPS_PROXY": "http://user:secret@proxy:3128", "all_proxy": "socks5://p:1080"}))
	if !strings.Contains(w, "HTTPS_PROXY") || !strings.Contains(w, "all_proxy") || !strings.Contains(w, "does not use it") {
		t.Fatalf("missing names or wording: %q", w)
	}
	if strings.Contains(w, "secret") || strings.Contains(w, "proxy:3128") {
		t.Fatalf("value leaked: %q", w)
	}
}
