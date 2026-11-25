package metadata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

func TestLogoAskIsShortAndOrdered(t *testing.T) {
	cases := []struct{ lang, orig, want string }{
		{"de", "ko", "de,ko,en,null"},
		{"ja", "ja", "ja,en,null"},
		{"en", "es", "en,es,null"},
		{"pt-br", "", "pt,en,null"},
		{"fil", "tl", "tl,en,null"},
		{"de", "EN", "de,en,null"},
		{"de", "bad!", "de,en,null"},
	}
	for _, c := range cases {
		if got := strings.Join(logoAsk(c.lang, c.orig), ","); got != c.want {
			t.Errorf("%s/%s: got %q want %q", c.lang, c.orig, got, c.want)
		}
	}
}

func TestPickTitleLogoOrder(t *testing.T) {
	s := func(v string) *string { return &v }
	all := []tmdbLogo{
		{FilePath: "/en.png", Vote: 9, Language: s("en")},
		{FilePath: "/ko.png", Vote: 3, Language: s("ko")},
		{FilePath: "/de.svg", Vote: 9, Language: s("de")},
		{FilePath: "/none.png", Vote: 9},
	}
	cases := []struct {
		name, lang, orig string
		logos            []tmdbLogo
		want             string
	}{
		{"reader's language first", "ko", "ja", all, "/ko.png"},
		{"original when reader has none", "de", "ko", all, "/ko.png"},
		{"none in either: English stays", "fr", "ja", all, ""},
		{"PNG only", "de", "", all, ""},
		{"region first", "pt-br", "", []tmdbLogo{{FilePath: "/pt.png", Vote: 9, Language: s("pt"), Country: s("PT")}, {FilePath: "/br.png", Vote: 1, Language: s("pt"), Country: s("BR")}}, "/br.png"},
	}
	for _, c := range cases {
		got := pickTitleLogo(c.logos, c.lang, c.orig, "")
		if want := c.want; want != "" {
			c.want = "/w500" + want
		}
		if got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

func TestTitleLogoAsksTheShortList(t *testing.T) {
	var asked []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.URL.Path+"?"+r.URL.Query().Get("include_image_language"))
		_, _ = w.Write([]byte(`{"logos":[{"file_path":"/ja.png","iso_639_1":"ja","vote_average":5}]}`))
	}))
	defer srv.Close()
	got, err := newTMDB(srv, fakeCred).TitleLogo(context.Background(), "movie", "129", "de", "ja")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(got, "/w500/ja.png") || !slices.ContainsFunc(asked, func(a string) bool { return strings.HasSuffix(a, "/movie/129/images?de,ja,en,null") }) {
		t.Fatalf("logo %q asked %v", got, asked)
	}
}
