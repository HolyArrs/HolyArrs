package sites

import (
	"context"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/sites/fakesite"
)

// OpenSubtitles.com has no plain zh, so Chinese asks for each kind, in order.
func TestOSAskLangsChinese(t *testing.T) {
	got := osAskLangs([]string{"en", "zh"})
	want := []string{"en", "ze", "zh-ca", "zh-cn", "zh-tw"}
	if !slices.Equal(got, want) {
		t.Fatalf("osAskLangs = %v, want %v", got, want)
	}
	if siteLang("zh-TW") != "zh-hant" || !langWanted([]string{"zh"}, siteLang("zh-cn")) {
		t.Fatal("a regional Chinese answer should read as a zh wish")
	}
}

// A Chinese wish finds Simplified, Traditional, Cantonese and bilingual files,
// each read back as its own kind, and nothing in another language.
func TestOpenSubtitles_Chinese(t *testing.T) {
	f := osFake()
	f.Subs = append(f.Subs,
		fakesite.Sub{ID: "201", Kind: "movie", Language: "zh-CN", Title: "The Lantern", Year: 2010, Text: fakesite.Cue("x", 1000)},
		fakesite.Sub{ID: "202", Kind: "movie", Language: "zh-TW", Title: "The Lantern", Year: 2010, Text: fakesite.Cue("x", 1000)},
		fakesite.Sub{ID: "203", Kind: "movie", Language: "zh-CA", Title: "The Lantern", Year: 2010, Text: fakesite.Cue("x", 1000)},
		fakesite.Sub{ID: "204", Kind: "movie", Language: "ze", Title: "The Lantern", Year: 2010, Text: fakesite.Cue("x", 1000)},
	)
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	s, err := New("opensubtitles", Config{Name: "OS", URL: srv.URL + "/api/v1", Key: fakeKey, GapMS: 1, AllowLocal: true})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Search(context.Background(), Query{Kind: "movie", Title: "The Lantern", Year: 2010, Languages: []string{"zh"}})
	if err != nil || len(got) != 4 {
		t.Fatalf("%+v %v", got, err)
	}
	kinds := map[string]string{"201": "zh-hans", "202": "zh-hant", "203": "zh-yue", "204": "zh-hans"}
	for _, c := range got {
		if c.Language != kinds[c.Key] {
			t.Errorf("%s read as %q, want %q", c.Key, c.Language, kinds[c.Key])
		}
	}
}
