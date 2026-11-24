package hooks

import (
	"strings"
	"testing"
)

func TestCheckCardTable(t *testing.T) {
	long := strings.Repeat("a", 201)
	for _, c := range []struct {
		name string
		in   string
		ok   bool
	}{
		{"plain card", `{"title":"Now on Trakt","rows":[{"text":"Dune","sub":"watched 2h ago","title_id":12}]}`, true},
		{"internal link", `{"title":"x","rows":[{"text":"a","link":"/library/movies"}]}`, true},
		{"api link", `{"title":"x","rows":[{"text":"a","link":"/api/v1/plugin-pages/x/call"}]}`, false},
		{"no rows", `{"title":"x","rows":[]}`, true},
		{"markup in text", `{"title":"x","rows":[{"text":"<img src=x onerror=alert(1)>"}]}`, false},
		{"markup in title", `{"title":"<b>x</b>","rows":[]}`, false},
		{"outside link", `{"title":"x","rows":[{"text":"a","link":"https://evil.example"}]}`, false},
		{"protocol-relative link", `{"title":"x","rows":[{"text":"a","link":"//evil.example"}]}`, false},
		{"script link", `{"title":"x","rows":[{"text":"a","link":"javascript:alert(1)"}]}`, false},
		{"backslash link", `{"title":"x","rows":[{"text":"a","link":"/\evil.example"}]}`, false},
		{"unknown field", `{"title":"x","html":"<p>","rows":[]}`, false},
		{"too long", `{"title":"` + long + `","rows":[]}`, false},
		{"empty title", `{"title":"","rows":[]}`, false},
		{"too many rows", `{"title":"x","rows":[` + strings.Repeat(`{"text":"a"},`, 20) + `{"text":"a"}]}`, false},
		{"not json", `<html>`, false},
	} {
		_, err := CheckCard([]byte(c.in))
		if (err == nil) != c.ok {
			t.Errorf("%s: err = %v, want ok %v", c.name, err, c.ok)
		}
	}
}

func TestCheckToolsTable(t *testing.T) {
	for _, c := range []struct {
		name string
		in   string
		ok   bool
	}{
		{"one read tool", `[{"name":"lyrics","about":"Find lyrics","scopes":["library.read"]}]`, true},
		{"no scopes", `[{"name":"ping","about":"x","scopes":[]}]`, false},
		{"bad name", `[{"name":"search titles","about":"x"}]`, false},
		{"dotted name", `[{"name":"holyarrs.search","about":"x"}]`, false},
		{"repeat", `[{"name":"a","about":"x"},{"name":"a","about":"y"}]`, false},
		{"unknown scope", `[{"name":"a","about":"x","scopes":["root.everything"]}]`, false},
		{"markup about", `[{"name":"a","about":"<script>"}]`, false},
	} {
		_, err := CheckTools([]byte(c.in), func(s string) bool { return strings.HasPrefix(s, "library.") || strings.HasPrefix(s, "requests.") })
		if (err == nil) != c.ok {
			t.Errorf("%s: err = %v, want ok %v", c.name, err, c.ok)
		}
	}
}

func TestCheckPagesTable(t *testing.T) {
	for _, c := range []struct {
		name string
		in   string
		ok   bool
	}{
		{"one page", `[{"id":"player","title":"Player","file":"index.html"}]`, true},
		{"sub folder", `[{"id":"p","title":"P","file":"app/index.html"}]`, true},
		{"climbs out", `[{"id":"p","title":"P","file":"../../etc/passwd"}]`, false},
		{"absolute", `[{"id":"p","title":"P","file":"/etc/passwd"}]`, false},
		{"not html", `[{"id":"p","title":"P","file":"x.js"}]`, false},
		{"bad id", `[{"id":"P Q","title":"P","file":"a.html"}]`, false},
		{"too many", `[` + strings.Repeat(`{"id":"a","title":"P","file":"a.html"},`, 8) + `{"id":"b","title":"P","file":"a.html"}]`, false},
	} {
		_, err := CheckPages([]byte(c.in))
		if (err == nil) != c.ok {
			t.Errorf("%s: err = %v, want ok %v", c.name, err, c.ok)
		}
	}
}
