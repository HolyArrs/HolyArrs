package metadata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func lp(s string) *string { return &s }

func TestPickCardArtOrder(t *testing.T) {
	textless := CardImage{Path: "/w780/plain.jpg", Vote: 5}
	logoEN := CardImage{Path: "/w500/logo-en.png", Lang: lp("en"), Vote: 5}
	logoFR := CardImage{Path: "/w500/logo-fr.png", Lang: lp("fr"), Vote: 4}
	cases := []struct {
		name string
		im   CardImages
		lang string
		want CardArt
	}{
		{"textless art plus the logo, never the titled backdrop", CardImages{
			Backdrops: []CardImage{{Path: "/w780/titled-en.jpg", Lang: lp("en"), Vote: 9}, textless},
			Logos:     []CardImage{logoEN}}, "en", CardArt{Art: "/w780/plain.jpg", Logo: "/w500/logo-en.png"}},
		{"titled art only when nothing textless exists, and then no logo", CardImages{
			Backdrops: []CardImage{{Path: "/w780/titled-fr.jpg", Lang: lp("fr"), Vote: 1}},
			Logos:     []CardImage{logoFR}}, "fr", CardArt{Art: "fallback.jpg", Logo: "/w500/logo-fr.png"}},
		{"textless art plus the logo", CardImages{Backdrops: []CardImage{textless}, Logos: []CardImage{logoEN, logoFR}}, "fr",
			CardArt{Art: "/w780/plain.jpg", Logo: "/w500/logo-fr.png"}},
		{"logo must be a PNG", CardImages{Backdrops: []CardImage{textless}, Logos: []CardImage{{Path: "/w500/logo.svg", Lang: lp("en")}}}, "en",
			CardArt{Art: "/w780/plain.jpg"}},
		{"name only, best textless vote", CardImages{Backdrops: []CardImage{textless, {Path: "/w780/better.jpg", Vote: 8}}}, "en",
			CardArt{Art: "/w780/better.jpg"}},
		{"nothing listed keeps the known backdrop", CardImages{}, "en", CardArt{Art: "fallback.jpg"}},
	}
	for _, c := range cases {
		if got := PickCardArt(c.im, c.lang, "fallback.jpg"); got != c.want {
			t.Errorf("%s: got %+v, want %+v", c.name, got, c.want)
		}
	}
}

func TestTMDBCardImages(t *testing.T) {
	var asked []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.URL.Path+"?"+r.URL.RawQuery)
		_, _ = w.Write([]byte(`{"backdrops":[{"file_path":"/a.jpg","iso_639_1":"en","vote_average":5}],"logos":[{"file_path":"/l.png","iso_639_1":null}]}`))
	}))
	defer srv.Close()
	tm := &TMDB{BaseURL: srv.URL, Images: "https://img", Access: func(context.Context) (string, error) { return "x", nil }, Gap: time.Millisecond}
	im, err := tm.CardImages(context.Background(), "show", "42", "fr")
	if err != nil {
		t.Fatal(err)
	}
	if len(asked) != 1 || !strings.HasPrefix(asked[0], "/tv/42/images?") || !strings.Contains(asked[0], "fr%2Cen%2Cnull") {
		t.Fatalf("one image list call expected, got %v", asked)
	}
	if im.Backdrops[0].Path != "https://img/w780/a.jpg" || im.Logos[0].Path != "https://img/w500/l.png" {
		t.Fatalf("paths: %+v", im)
	}
}
