package sources

import (
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func TestParseItemsReadsInfoLink(t *testing.T) {
	body := []byte(`<rss><channel>
<item><title>A.2019.1080p.WEB-GRP</title><link>http://x/1</link><guid>http://site/guid/1</guid><comments>https://site/details/1</comments></item>
<item><title>B.2019.1080p.WEB-GRP</title><link>http://x/2</link><guid>https://site/details/2</guid></item>
<item><title>C.2019.1080p.WEB-GRP</title><link>http://x/3</link><guid>abc123</guid></item>
<item><title>D.2019.1080p.WEB-GRP</title><link>http://x/4</link><comments>javascript:alert(1)</comments><guid>JAVASCRIPT:alert(1)</guid></item>
</channel></rss>`)
	items, err := parseItems(body, "torrent")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"https://site/details/1", "https://site/details/2", "", ""}
	for i, w := range want {
		if items[i].InfoURL != w {
			t.Errorf("item %d info link = %q, want %q", i, items[i].InfoURL, w)
		}
	}
}

func TestWebLinkOnlyHTTP(t *testing.T) {
	for in, want := range map[string]string{
		"https://a.example/x?id=1": "https://a.example/x?id=1",
		" http://a.example/x ":     "http://a.example/x",
		"javascript:alert(1)":      "",
		"data:text/html,hi":        "",
		"//a.example/x":            "",
		"https:///nohost":          "",
		"ftp://a.example/x":        "",
		"":                         "",
	} {
		if got := webLink(in); got != want {
			t.Errorf("webLink(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestReleaseViewCarriesInfoLink(t *testing.T) {
	m := map[string]any{infoURLKey: "https://site/details/1"}
	if v := releaseView(domain.Release{Name: "x", Parsed: m}); v.InfoURL != "https://site/details/1" {
		t.Errorf("info link = %q", v.InfoURL)
	}
	bad := map[string]any{infoURLKey: "javascript:alert(1)"}
	if v := releaseView(domain.Release{Name: "x", Parsed: bad}); v.InfoURL != "" {
		t.Errorf("a non-web stored link must not reach the screen, got %q", v.InfoURL)
	}
}

func TestKeepInfoLinkDropsKeys(t *testing.T) {
	if got := keepInfoURL("https://site/details/1", []string{"SECRET"}); got != "https://site/details/1" {
		t.Errorf("got %q", got)
	}
	if got := keepInfoURL("https://site/details/1?apikey=SECRET", []string{"SECRET"}); got != "" {
		t.Errorf("a link holding the Source key must be dropped, got %q", got)
	}
}
