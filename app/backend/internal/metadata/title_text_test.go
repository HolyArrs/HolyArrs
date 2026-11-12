package metadata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestTextLanguage(t *testing.T) {
	cases := []struct{ header, accept, want string }{
		{"es", "", "es"},
		{"PT-BR", "", "pt-br"},
		{"pt-PT", "", "pt-pt"},
		{"pt", "", "pt-br"},
		{"zh-TW", "", "zh-tw"},
		{"zh-Hant-HK", "", "zh-tw"},
		{"zh", "", "zh-cn"},
		{"nn", "", "nb"},
		{"no", "", "nb"},
		{"en", "de-DE,de;q=0.9", "en"},
		// The header names what the page shows; an odd one falls back to the browser's.
		{"xx", "fr-CA,fr;q=0.9", "fr"},
		{"", "eu,es;q=0.9", "es"},
		// Every app language has its texts, old tags included.
		{"ca", "", "ca"},
		{"fil", "", "fil"},
		{"tl", "", "fil"},
		{"iw", "", "he"},
		{"in", "", "id"},
		{"sh", "", "sr"},
		{"", "fa-IR,fa;q=0.9", "fa"},
		{"", "de-DE,de;q=0.9", "de"},
		{"", "", "en"},
		{"", "*", "en"},
	}
	for _, c := range cases {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		if c.header != "" {
			r.Header.Set("X-HolyArrs-Language", c.header)
		}
		if c.accept != "" {
			r.Header.Set("Accept-Language", c.accept)
		}
		if got := TextLanguage(r); got != c.want {
			t.Errorf("header %q accept %q: %q, want %q", c.header, c.accept, got, c.want)
		}
	}
}

func TestTMDBLanguage(t *testing.T) {
	for lang, want := range map[string]string{"en": "en-US", "es": "es-ES", "pt-br": "pt-BR", "pt-pt": "pt-PT", "zh-cn": "zh-CN", "zh-tw": "zh-TW",
		"nb": "no-NO", "ar": "ar-SA", "ja": "ja-JP", "uk": "uk-UA", "he": "he-IL", "el": "el-GR", "cs": "cs-CZ", "da": "da-DK", "sv": "sv-SE",
		"vi": "vi-VN", "hi": "hi-IN", "ko": "ko-KR", "fi": "fi-FI", "de": "de-DE",
		"fil": "tl-PH", "ca": "ca-ES", "fa": "fa-IR", "bn": "bn-BD", "ta": "ta-IN", "ms": "ms-MY", "sr": "sr-RS", "et": "et-EE", "sk": "sk-SK", "is": "is-IS"} {
		if got := TMDBLanguage(lang); got != want {
			t.Errorf("%s: %q, want %q", lang, got, want)
		}
	}
}

// textTMDB answers a movie (5) and a show (8) with their translations, and
// season 1 of the show in English and in Spanish.
func textTMDB(t *testing.T, hits *atomic.Int32, asked *atomic.Value) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		switch {
		case strings.HasPrefix(r.URL.Path, "/3/genre/"):
			_, _ = w.Write([]byte(`{"genres":[]}`))
		case r.URL.Path == "/3/movie/5":
			asked.Store(r.URL.Query().Get("append_to_response"))
			_, _ = w.Write([]byte(`{"id":5,"title":"Five","overview":"English story","tagline":"Tag","original_language":"en",
				"translations":{"translations":[
				{"iso_639_1":"en","iso_3166_1":"US","data":{"title":"Five","overview":"English story","tagline":"Tag"}},
				{"iso_639_1":"es","iso_3166_1":"MX","data":{"title":"Cinco MX","overview":"Historia","tagline":""}},
				{"iso_639_1":"es","iso_3166_1":"ES","data":{"title":"Cinco","overview":"","tagline":""}},
				{"iso_639_1":"pt","iso_3166_1":"BR","data":{"title":"Cinco BR","overview":"História BR"}},
				{"iso_639_1":"fr","iso_3166_1":"FR","data":{"title":"","overview":" Histoire "}},
				{"iso_639_1":"tl","iso_3166_1":"PH","data":{"title":"Lima","overview":"Kuwento"}},
				{"iso_639_1":"xx","iso_3166_1":"XX","data":{"title":"Unknown","overview":"Unknown"}}]}}`))
		case r.URL.Path == "/3/tv/8":
			_, _ = w.Write([]byte(`{"id":8,"name":"Eight","overview":"Show story","translations":{"translations":[
				{"iso_639_1":"de","iso_3166_1":"DE","data":{"name":"Acht","overview":"Geschichte"}}]}}`))
		case r.URL.Path == "/3/tv/8/season/1" && r.URL.Query().Get("language") == "es-ES":
			_, _ = w.Write([]byte(`{"episodes":[{"episode_number":1,"name":"Piloto","overview":"Empieza"},{"episode_number":2,"name":"Dos","overview":""}]}`))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestDetailsKeepTitleTexts(t *testing.T) {
	var hits atomic.Int32
	var asked atomic.Value
	disk := &memCache{}
	p := newTMDB(textTMDB(t, &hits, &asked), fakeCred)
	p.Disk = disk
	ctx := context.Background()
	if _, ok := p.KeptText(ctx, "movie", "5", "es"); ok {
		t.Fatal("text known before any lookup")
	}
	d, err := p.Details(ctx, "movie", "5")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(asked.Load().(string), "translations") {
		t.Fatalf("details did not ask for translations: %q", asked.Load())
	}
	// The record itself stays English: names, matching and file names use it.
	if d.Name != "Five" || d.Overview != "English story" || d.Tagline != "Tag" {
		t.Fatalf("details not English: %+v", d.DiscoverItem)
	}
	cases := map[string]TitleText{
		// es-ES first; its empty synopsis comes from es-MX.
		"es": {Name: "Cinco", Overview: "Historia"},
		// No French title: the English one is shown.
		"fr": {Overview: "Histoire"},
		// Portuguese from Portugal falls back to Brazil before English.
		"pt-pt": {Name: "Cinco BR", Overview: "História BR"},
		// Filipino is TMDB's tl.
		"fil": {Name: "Lima", Overview: "Kuwento"},
		// Kept, but nothing in German: English everywhere.
		"de": {},
	}
	for lang, want := range cases {
		got, ok := p.KeptText(ctx, "movie", "5", lang)
		if !ok || got != want {
			t.Errorf("%s: %+v %v, want %+v", lang, got, ok, want)
		}
	}
	// A restart: a new client over the same disk knows the texts without asking.
	q := newTMDB(textTMDB(t, &hits, &asked), fakeCred)
	q.Disk = disk
	if got, ok := q.KeptText(ctx, "movie", "5", "es"); !ok || got.Name != "Cinco" {
		t.Fatalf("after restart: %+v %v", got, ok)
	}
	if _, err := p.Details(ctx, "show", "8"); err != nil {
		t.Fatal(err)
	}
	if got, ok := p.KeptText(ctx, "show", "8", "de"); !ok || got != (TitleText{Name: "Acht", Overview: "Geschichte"}) {
		t.Fatalf("show: %+v %v", got, ok)
	}
	if _, ok := p.KeptText(ctx, "movie", "6", "es"); ok {
		t.Fatal("a title never looked up has kept text")
	}
}

func TestSeasonText(t *testing.T) {
	var hits atomic.Int32
	var asked atomic.Value
	p := newTMDB(textTMDB(t, &hits, &asked), fakeCred)
	ctx := context.Background()
	got, err := p.SeasonText(ctx, "8", 1, "es")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[1] != (TitleText{Name: "Piloto", Overview: "Empieza"}) || got[2] != (TitleText{Name: "Dos"}) {
		t.Fatalf("season text %+v", got)
	}
	before := hits.Load()
	if got, err := p.SeasonText(ctx, "8", 1, "en"); err != nil || got != nil || hits.Load() != before {
		t.Fatalf("English asked out: %+v %v", got, err)
	}
}

func TestTitleTextApply(t *testing.T) {
	it := DiscoverItem{Name: "Five", Overview: "English story"}
	TitleText{Overview: "Historia"}.Apply(&it)
	if it.Name != "Five" || it.Overview != "Historia" {
		t.Fatalf("%+v", it)
	}
}

// Every language the app is shown in (a messages folder) has its texts.
func TestTextLanguageCoversAppLanguages(t *testing.T) {
	dirs, err := os.ReadDir(filepath.Join("..", "..", "..", "frontend", "src", "lib", "core", "i18n", "messages"))
	if err != nil {
		t.Skip("no app words here:", err)
	}
	for _, d := range dirs {
		if d.IsDir() && appLanguage(d.Name()) != d.Name() {
			t.Errorf("app language %q has no texts (%q)", d.Name(), appLanguage(d.Name()))
		}
	}
}
