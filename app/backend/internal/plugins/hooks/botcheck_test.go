package hooks

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// TestSolveBotcheckTable: a solver plugin gets the page and the proxy, and
// an answer of the wrong shape is refused or trimmed.
func TestSolveBotcheckTable(t *testing.T) {
	for _, c := range []struct {
		name   string
		answer string
		ok     bool
		check  func(BotcheckAnswer) bool
	}{
		{"page", `{"url":"https://site.example/a","status":200,"body":"<rss/>","cookies":[{"name":"cf_clearance","value":"ok"}],"user_agent":"Browser/1"}`, true,
			func(a BotcheckAnswer) bool {
				return a.Body == "<rss/>" && a.UserAgent == "Browser/1" && len(a.Cookies) == 1
			}},
		{"no status", `{"body":"x"}`, true, func(a BotcheckAnswer) bool { return a.Status == 0 }},
		{"bad status", `{"status":42}`, false, nil},
		{"status too high", `{"status":700}`, false, nil},
		{"not json", `"nope"`, false, nil},
		{"user agent with a line break", `{"status":200,"user_agent":"B\r\nX-Evil: 1"}`, true, func(a BotcheckAnswer) bool { return a.UserAgent == "" }},
		{"too many cookies", `{"status":200,"cookies":[` + strings.TrimSuffix(strings.Repeat(`{"name":"a","value":"1"},`, 80), ",") + `]}`, true,
			func(a BotcheckAnswer) bool { return len(a.Cookies) == botcheckCookies }},
	} {
		t.Run(c.name, func(t *testing.T) {
			b := newTestBroker(map[string][]string{"fs": {"sources.botcheck"}})
			var got BotcheckAsk
			stub(t, b, "fs", Botcheck, func(j Job) Answer {
				if j.Op != "open" {
					t.Errorf("op %q", j.Op)
				}
				_ = json.Unmarshal(j.Payload, &got)
				return Answer{Data: json.RawMessage(c.answer)}
			})
			a, err := SolveBotcheck(context.Background(), b, "fs", BotcheckAsk{URL: "https://site.example/a", Proxy: &BotcheckProxy{URL: "socks5://vpn:1080", Username: "me", Password: "pw"}})
			if (err == nil) != c.ok {
				t.Fatalf("err %v", err)
			}
			if got.URL != "https://site.example/a" || got.Proxy == nil || got.Proxy.Password != "pw" {
				t.Errorf("plugin got %+v", got)
			}
			if c.ok && !c.check(a) {
				t.Errorf("answer %+v", a)
			}
		})
	}
}

// TestSolveBotcheck_NeedsItsScope: a plugin without sources.botcheck is
// never handed a page.
func TestSolveBotcheck_NeedsItsScope(t *testing.T) {
	b := newTestBroker(map[string][]string{"fs": {"sources.releases"}})
	if _, err := SolveBotcheck(context.Background(), b, "fs", BotcheckAsk{URL: "https://site.example/a"}); err != ErrNoProvider {
		t.Fatalf("got %v", err)
	}
	if Scopes[Botcheck] != "sources.botcheck" {
		t.Fatalf("scope %q", Scopes[Botcheck])
	}
}
