package sources

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// A show named only in a non-Latin script (TVDB's original name) is
// found on a word-only or id-ignoring site under its English other name.
func TestNonLatinTitleSearchesEnglishOtherName(t *testing.T) {
	t.Parallel()
	cases := []struct{ title, alias, rel string }{
		{"夜桜さんちの大作戦", "Mission: Yozakura Family", "Mission.Yozakura.Family.S01E02.1080p.WEB.h264-GRP"},
		{"오징어 게임", "Squid Game", "Squid.Game.S01E02.1080p.WEB.h264-GRP"},
		{"Слово пацана", "The Boy's Word", "The.Boys.Word.S01E02.1080p.WEB.h264-GRP"},
	}
	for _, tc := range cases {
		for _, caps := range []string{aliasCapsText, aliasCapsIDs} {
			site := &aliasSite{}
			q := Query{Title: tc.title, TV: true, Season: 1, Episode: "2", TVDB: 424019, Aliases: []string{tc.title, tc.alias}}
			got := searchWithOtherNames(t, site, []string{tc.rel}, caps, q)
			if len(got) != 1 {
				t.Fatalf("%s (%s): want the release under %q, got %d from %v", tc.title, caps, tc.alias, len(got), site.asks)
			}
		}
	}
}

// With no other name only the own name is asked: nothing to add, no error.
func TestNonLatinTitleWithoutOtherNames(t *testing.T) {
	t.Parallel()
	site := &aliasSite{}
	got := searchWithOtherNames(t, site, []string{"Mission.Yozakura.Family.S01E02.1080p.WEB.h264-GRP"}, aliasCapsText,
		Query{Title: "夜桜さんちの大作戦", TV: true, Season: 1, Episode: "2", TVDB: 424019})
	if len(got) != 0 || len(site.asks) != 1 {
		t.Fatalf("want one search and nothing found, got %d from %d searches", len(got), len(site.asks))
	}
}

// A site that ignores the id and sends its newest releases (The Pirate Bay
// in the report) is still asked by the English other name.
func TestNonLatinTitleIDIgnoredAsksOtherName(t *testing.T) {
	t.Parallel()
	var qs []string
	srv := serve(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v := r.URL.Query()
		if v.Get("t") == "caps" {
			_, _ = w.Write([]byte(aliasCapsIDs))
			return
		}
		qs = append(qs, v.Get("q"))
		rel := "Some.Other.Show.S03E07.1080p.WEB.h264-GRP"
		if strings.Contains(strings.ToLower(v.Get("q")), "yozakura") {
			rel = "Mission.Yozakura.Family.S01E02.1080p.WEB.h264-GRP"
		}
		fmt.Fprintf(w, `<rss><channel><item><title>%s</title><guid>g</guid><link>magnet:?xt=urn:btih:%040d</link></item></channel></rss>`, rel, 1)
	}))
	c := New(Config{URL: srv.URL, Protocol: "torrent", AllowLocal: true})
	items, _, err := c.searchNamed(context.Background(), Query{Title: "夜桜さんちの大作戦", TV: true, Season: 1, Episode: "2", TVDB: 424019,
		Aliases: []string{"Mission: Yozakura Family"}}, maxNameSearches)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, it := range items {
		found = found || strings.HasPrefix(it.Title, "Mission.Yozakura")
	}
	if !found {
		t.Fatalf("want the release found by the other name, asked %q, got %+v", qs, items)
	}
}
