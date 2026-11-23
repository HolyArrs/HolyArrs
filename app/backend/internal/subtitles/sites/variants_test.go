package sites

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/sites/fakesite"
)

// Each kind goes out as OpenSubtitles.com's own codes.
func TestOSAskLangsVariants(t *testing.T) {
	for _, c := range []struct {
		in   []string
		want string
	}{
		{[]string{"zh"}, "ze,zh-ca,zh-cn,zh-tw"},
		{[]string{"zh-hans"}, "ze,zh-cn"},
		{[]string{"zh-hant"}, "zh-tw"},
		{[]string{"zh-yue"}, "zh-ca"},
		{[]string{"zh-hans", "zh-hant"}, "ze,zh-cn,zh-tw"},
		{[]string{"pt"}, "pt-br,pt-pt"},
		{[]string{"pt-br"}, "pt-br"},
		{[]string{"pt-pt"}, "pt-pt"},
		{[]string{"es"}, "ea,es"},
		{[]string{"es-419"}, "ea"},
		{[]string{"fr-ca", "fr"}, "fr"},
	} {
		if got := strings.Join(osAskLangs(c.in), ","); got != c.want {
			t.Errorf("%v: %s, want %s", c.in, got, c.want)
		}
	}
	for in, want := range map[string]string{"ze": "zh-hans", "zh-CN": "zh-hans", "zh-TW": "zh-hant", "zh-CA": "zh-yue", "ea": "es-419", "es": "es", "pt-PT": "pt-pt", "pt-BR": "pt-br", "en": "en"} {
		if got := osLang(in); got != want {
			t.Errorf("osLang(%q) = %q, want %q", in, got, want)
		}
	}
}

// A Traditional wish on OpenSubtitles.com takes only Traditional files, and
// a Latin American Spanish wish only the site's "ea" files.
func TestOpenSubtitles_VariantWishes(t *testing.T) {
	f := osFake()
	for _, s := range []fakesite.Sub{
		{ID: "301", Language: "zh-CN"}, {ID: "302", Language: "zh-TW"}, {ID: "303", Language: "ea"}, {ID: "304", Language: "es"},
	} {
		s.Kind, s.Title, s.Year, s.Text = "movie", "The Lantern", 2010, fakesite.Cue("x", 1000)
		f.Subs = append(f.Subs, s)
	}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	s, err := New("opensubtitles", Config{Name: "OS", URL: srv.URL + "/api/v1", Key: fakeKey, GapMS: 1, AllowLocal: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		langs []string
		keys  string
	}{
		{[]string{"zh-hant"}, "302"}, {[]string{"zh-hans"}, "301"}, {[]string{"es-419"}, "303"}, {[]string{"es"}, "303,304"},
	} {
		got, err := s.Search(context.Background(), Query{Kind: "movie", Title: "The Lantern", Year: 2010, Languages: c.langs})
		if err != nil {
			t.Fatal(err)
		}
		var keys []string
		for _, x := range got {
			keys = append(keys, x.Key)
		}
		slices.Sort(keys)
		if strings.Join(keys, ",") != c.keys {
			t.Errorf("%v: got %v, want %s", c.langs, keys, c.keys)
		}
	}
}

// subdl is asked BR_PT for Brazil and ZH_BG for Big5, and its answers read back.
func TestSubdl_VariantCodes(t *testing.T) {
	for _, c := range []struct {
		code string
		want []string
	}{{"pt-br", []string{"BR_PT"}}, {"pt", []string{"BR_PT", "PT"}}, {"pt-pt", []string{"PT"}}, {"zh", []string{"ZH", "ZH_BG"}}, {"zh-hant", []string{"ZH_BG"}}, {"zh-hans", nil}, {"en", []string{"EN"}}} {
		if got := sdAsk(c.code); !slices.Equal(got, c.want) {
			t.Errorf("sdAsk(%q) = %v, want %v", c.code, got, c.want)
		}
	}
	for in, want := range map[string]string{"BR_PT": "pt-br", "PT": "pt-pt", "ZH_BG": "zh-hant", "ZH": "zh", "EN": "en"} {
		if got := sdLang(in); got != want {
			t.Errorf("sdLang(%q) = %q, want %q", in, got, want)
		}
	}
	var mu sync.Mutex
	var asked []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		asked = append(asked, r.URL.Query().Get("languages"))
		mu.Unlock()
		_, _ = w.Write([]byte(`{"status":true,"results":[{"name":"The Lantern","year":2010}],"subtitles":[` +
			`{"release_name":"a","language":"BR_PT","url":"/subtitle/1.zip"},{"release_name":"b","language":"PT","url":"/subtitle/2.zip"},` +
			`{"release_name":"c","language":"ZH_BG","url":"/subtitle/3.zip"},{"release_name":"d","language":"ZH","url":"/subtitle/4.zip"}]}`))
	}))
	t.Cleanup(srv.Close)
	s, err := New("subdl", Config{Name: "SD", URL: srv.URL + "/api/v1", Key: fakeKey, GapMS: 1, AllowLocal: true})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Search(context.Background(), Query{Kind: "movie", Title: "The Lantern", Year: 2010, IMDb: "tt1", Languages: []string{"pt-br", "zh-hant"}})
	if err != nil {
		t.Fatal(err)
	}
	var langs []string
	for _, c := range got {
		langs = append(langs, c.Language)
	}
	if !slices.Equal(langs, []string{"pt-br", "zh-hant"}) || len(asked) != 1 || asked[0] != "BR_PT,ZH_BG" {
		t.Fatalf("asked %v, got %v", asked, langs)
	}
}

// Addic7ed and Gestdown names read as their own kinds, and a plain Chinese
// wish asks Gestdown for both kinds.
func TestNamedSiteVariants(t *testing.T) {
	for name, want := range map[string]string{"Chinese (Simplified)": "zh-hans", "Chinese (Traditional)": "zh-hant", "Spanish (Latin America)": "es-419",
		"Spanish (Spain)": "es", "Portuguese": "pt-pt", "Portuguese (Brazilian)": "pt-br", "French (Canadian)": "fr-ca", "French": "fr"} {
		if got := langCode(name); got != want {
			t.Errorf("Addic7ed %q = %q, want %q", name, got, want)
		}
	}
	if got := gdNames("zh"); !slices.Equal(got, []string{"Chinese (Simplified)", "Chinese (Traditional)"}) {
		t.Errorf("gdNames(zh) = %v", got)
	}
	if got := gdNames("zh-hant"); !slices.Equal(got, []string{"Chinese (Traditional)"}) {
		t.Errorf("gdNames(zh-hant) = %v", got)
	}
	if gdLang("Portuguese") != "pt-pt" || gdLang("Chinese (Traditional)") != "zh-hant" || gdLang("English") != "en" {
		t.Error("gdLang")
	}
	if got := plainLangs([]string{"zh-hant", "zh", "pt-br", "es-419", "en"}, "pt-br"); !slices.Equal(got, []string{"zh", "pt-br", "en"}) {
		t.Errorf("plainLangs = %v", got)
	}
}

func TestGestdownPortugalWish(t *testing.T) {
	if got := gdNames("pt-pt"); len(got) != 1 || got[0] != "Portuguese" || gdLang(got[0]) != "pt-pt" {
		t.Errorf("gdNames(pt-pt) = %v", got)
	}
}
