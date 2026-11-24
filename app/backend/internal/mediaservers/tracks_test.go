package mediaservers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles"
)

var testLangs = TrackLanguages{Norm: subtitles.NormLanguage, Match: subtitles.LangMatches, Three: subtitles.ThreeLetter}

func TestPickTrack(t *testing.T) {
	file := []track{{ID: "1", Lang: "en", Selected: true}, {ID: "2", Lang: "ja"}, {ID: "3", Lang: "fr", Forced: true}, {ID: "4", Lang: "fr"}, {ID: "5", Lang: "pt-br"}, {ID: "6", Lang: "de", Forced: true}}
	for _, c := range []struct {
		name string
		want []string
		id   string
		ok   bool
	}{
		{"first wanted language the file has", []string{"ja", "en"}, "2", true},
		{"falls back in order", []string{"ko", "it", "en"}, "1", true},
		{"full track before forced only", []string{"fr"}, "4", true},
		{"forced only when nothing else", []string{"de"}, "6", true},
		{"base language takes a regional track", []string{"pt"}, "5", true},
		{"off turns subtitles off", []string{"ko", SubtitlesOff}, "0", true},
		{"wanted before off wins", []string{"ja", SubtitlesOff}, "2", true},
		{"nothing wanted in the file: left alone", []string{"ko"}, "", false},
		{"empty list: left alone", nil, "", false},
	} {
		id, ok := pickTrack(testLangs, file, c.want)
		if id != c.id || ok != c.ok {
			t.Errorf("%s: got %q %v, want %q %v", c.name, id, ok, c.id, c.ok)
		}
	}
	if pickTrackNone, ok := pickTrack(testLangs, nil, []string{"en"}); ok || pickTrackNone != "" {
		t.Error("a file with no tracks is left alone")
	}
	if selectedTrack(file) != "1" || selectedTrack([]track{{ID: "9"}}) != "0" {
		t.Error("selected track")
	}
}

func TestCleanTrackLangs(t *testing.T) {
	got, err := CleanTrackLangs(testLangs, []string{" English ", "eng", "ja", "", "off", "fr"}, true)
	if err != nil || strings.Join(got, ",") != "en,ja,off" {
		t.Fatalf("got %v %v", got, err)
	}
	if _, err := CleanTrackLangs(testLangs, []string{"off"}, false); err == nil {
		t.Error("sound can't be off")
	}
	if _, err := CleanTrackLangs(testLangs, []string{"klingon"}, false); err == nil {
		t.Error("unknown language")
	}
	if _, err := CleanTrackLangs(testLangs, []string{"en", "fr", "de", "es", "it", "ja"}, false); err == nil {
		t.Error("too many")
	}
	if got, _ := CleanTrackLangs(testLangs, nil, true); len(got) != 0 {
		t.Error("empty stays empty")
	}
}

func TestParsePlexParts(t *testing.T) {
	b := []byte(`<MediaContainer><Video ratingKey="50"><Media><Part id="900">
<Stream id="1" streamType="1"/>
<Stream id="2" streamType="2" languageCode="eng" selected="1"/>
<Stream id="3" streamType="2" languageCode="jpn"/>
<Stream id="4" streamType="3" languageCode="fre" languageTag="fr" forced="1"/>
<Stream id="5" streamType="3" languageTag="pt-BR"/>
</Part><Part id="901"/></Media></Video></MediaContainer>`)
	ps, err := parsePlexParts(testLangs, b)
	if err != nil || len(ps) != 2 {
		t.Fatalf("%v %v", ps, err)
	}
	p := ps[0]
	if len(p.Audio) != 2 || p.Audio[0].Lang != "en" || !p.Audio[0].Selected || p.Audio[1].Lang != "ja" {
		t.Errorf("audio %+v", p.Audio)
	}
	if len(p.Subs) != 2 || p.Subs[0].Lang != "fr" || !p.Subs[0].Forced || p.Subs[1].Lang != "pt-br" {
		t.Errorf("subs %+v", p.Subs)
	}
	if _, err := parsePlexParts(testLangs, []byte("not xml <")); err == nil {
		t.Error("bad answer")
	}
}

func TestJellyGive(t *testing.T) {
	for _, c := range []struct {
		name         string
		cfg          map[string]any
		audio, subs  string
		pa, ps       string
		changed      bool
		wantA, wantS string
		wantM        string
		wantPlayDflt any
	}{
		{"fresh person gets both", map[string]any{"PlayDefaultAudioTrack": true}, "jpn", "eng", "", "", true, "jpn", "eng", "Always", false},
		{"own pick stays", map[string]any{"AudioLanguagePreference": "ger", "SubtitleLanguagePreference": "ger", "SubtitleMode": "Smart"}, "jpn", "eng", "", "", false, "ger", "ger", "Smart", nil},
		{"own mode stays", map[string]any{"SubtitleMode": "OnlyForced"}, "", "eng", "", "", false, "", "", "OnlyForced", nil},
		{"what we gave follows the new list", map[string]any{"AudioLanguagePreference": "jpn", "PlayDefaultAudioTrack": false, "SubtitleLanguagePreference": "eng", "SubtitleMode": "Always"}, "kor", "fre", "jpn", "eng", true, "kor", "fre", "Always", false},
		{"off", map[string]any{"SubtitleMode": "Default"}, "", SubtitlesOff, "", "", true, "", "", "None", nil},
		{"off back to a language", map[string]any{"SubtitleMode": "None"}, "", "eng", "", SubtitlesOff, true, "", "eng", "Always", nil},
		{"already set: no change", map[string]any{"AudioLanguagePreference": "jpn", "SubtitleLanguagePreference": "eng", "SubtitleMode": "Always"}, "jpn", "eng", "jpn", "eng", false, "jpn", "eng", "Always", nil},
	} {
		got := jellyGive(c.cfg, c.audio, c.subs, c.pa, c.ps)
		s := func(k string) string { v, _ := c.cfg[k].(string); return v }
		if got != c.changed || s("AudioLanguagePreference") != c.wantA || s("SubtitleLanguagePreference") != c.wantS || s("SubtitleMode") != c.wantM {
			t.Errorf("%s: changed %v cfg %v", c.name, got, c.cfg)
		}
		if c.wantPlayDflt != nil && c.cfg["PlayDefaultAudioTrack"] != c.wantPlayDflt {
			t.Errorf("%s: PlayDefaultAudioTrack %v", c.name, c.cfg["PlayDefaultAudioTrack"])
		}
	}
}

// plexTrackServer plays Plex with one movie (key 50, part 900) and one show
// (key 7) with one episode (key 70, part 700).
type plexTrackServer struct {
	*httptest.Server
	mu   sync.Mutex
	puts []string
}

func newPlexTrackServer(t *testing.T) *plexTrackServer {
	p := &plexTrackServer{}
	part := func(id string) string {
		return `<Media><Part id="` + id + `"><Stream id="` + id + `1" streamType="2" languageCode="eng" selected="1"/><Stream id="` + id + `2" streamType="2" languageCode="jpn"/><Stream id="` + id + `3" streamType="3" languageCode="eng"/></Part></Media>`
	}
	p.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Plex-Token") != tok {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		p.mu.Lock()
		defer p.mu.Unlock()
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/library/sections":
			_, _ = w.Write([]byte(`<MediaContainer><Directory key="1" type="movie" title="Movies"/><Directory key="2" type="show" title="TV"/></MediaContainer>`))
		case r.URL.Path == "/library/sections/1/all":
			_, _ = w.Write([]byte(`<MediaContainer><Video ratingKey="50"><Guid id="tmdb://603"/></Video></MediaContainer>`))
		case r.URL.Path == "/library/sections/2/all":
			_, _ = w.Write([]byte(`<MediaContainer><Directory ratingKey="7"><Guid id="tvdb://100"/></Directory></MediaContainer>`))
		case r.URL.Path == "/library/metadata/7/allLeaves":
			_, _ = w.Write([]byte(`<MediaContainer><Video ratingKey="70"/></MediaContainer>`))
		case r.URL.Path == "/library/metadata/50":
			_, _ = w.Write([]byte(`<MediaContainer><Video ratingKey="50">` + part("900") + `</Video></MediaContainer>`))
		case r.URL.Path == "/library/metadata/70":
			_, _ = w.Write([]byte(`<MediaContainer><Video ratingKey="70">` + part("700") + `</Video></MediaContainer>`))
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/library/parts/"):
			p.puts = append(p.puts, r.URL.Path+"?"+r.URL.RawQuery)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(p.Close)
	return p
}

func TestDefaultTracksPlex(t *testing.T) {
	w := newWorld(t)
	watchTitles(t, w)
	p := newPlexTrackServer(t)
	if _, err := Add(context.Background(), w.s, w.v, Input{Name: "Plex", Kind: "plex", URL: p.URL, Token: tok}, domain.System); err != nil {
		t.Fatal(err)
	}
	l := &Linker{Store: w.s, Vault: w.v, Langs: testLangs}
	ctx := context.Background()
	// Off: nothing happens.
	if n, err := l.SetDefaultTracks(ctx); err != nil || n != 0 || len(p.puts) != 0 {
		t.Fatalf("off: %d %v %v", n, err, p.puts)
	}
	set := TrackSettings{Enabled: true, Audio: []string{"ja", "en"}, Subtitles: []string{"en"}}
	if err := SaveTrackSettings(ctx, w.s, set, domain.System); err != nil {
		t.Fatal(err)
	}
	n, err := l.SetDefaultTracks(ctx)
	if err != nil || n != 2 || len(p.puts) != 2 {
		t.Fatalf("on: %d %v %v", n, err, p.puts)
	}
	for _, put := range p.puts {
		u, _ := url.Parse(put)
		id := strings.TrimPrefix(u.Path, "/library/parts/")
		q := u.Query()
		if q.Get("audioStreamID") != id+"2" || q.Get("subtitleStreamID") != id+"3" || q.Get("allParts") != "1" {
			t.Errorf("put %s", put)
		}
	}
	// Twice: every file is set once per list, so hand changes stay.
	if n, err := l.SetDefaultTracks(ctx); err != nil || n != 0 || len(p.puts) != 2 {
		t.Fatalf("again: %d %v %v", n, err, p.puts)
	}
	// A new list sets the files again; no subtitle is on, so off changes
	// only the sound.
	set.Audio, set.Subtitles = []string{"ja"}, []string{SubtitlesOff}
	if err := SaveTrackSettings(ctx, w.s, set, domain.System); err != nil {
		t.Fatal(err)
	}
	if n, err := l.SetDefaultTracks(ctx); err != nil || n != 2 || len(p.puts) != 4 {
		t.Fatalf("new list: %d %v %v", n, err, p.puts)
	}
	if q, _ := url.ParseQuery(strings.SplitN(p.puts[3], "?", 2)[1]); q.Get("audioStreamID") == "" || q.Has("subtitleStreamID") {
		t.Errorf("off put %s", p.puts[3])
	}
}

func TestDefaultTracksJellyfin(t *testing.T) {
	w := newWorld(t)
	var mu sync.Mutex
	cfgs := map[string]map[string]any{
		"a": {"PlayDefaultAudioTrack": true, "Other": "kept"},
		"b": {"AudioLanguagePreference": "ger", "SubtitleMode": "Smart"},
	}
	posts := 0
	js := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/Users":
			_, _ = rw.Write([]byte(`[{"Id":"a"},{"Id":"b"}]`))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/Users/"):
			b, _ := json.Marshal(map[string]any{"Id": "x", "Configuration": cfgs[strings.TrimPrefix(r.URL.Path, "/Users/")]})
			_, _ = rw.Write(b)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/Configuration"):
			id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/Users/"), "/Configuration")
			b, _ := io.ReadAll(r.Body)
			var c map[string]any
			_ = json.Unmarshal(b, &c)
			cfgs[id] = c
			posts++
			rw.WriteHeader(http.StatusNoContent)
		default:
			rw.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(js.Close)
	if _, err := Add(context.Background(), w.s, w.v, Input{Name: "Jelly", Kind: "jellyfin", URL: js.URL, Token: tok}, domain.System); err != nil {
		t.Fatal(err)
	}
	l := &Linker{Store: w.s, Vault: w.v, Langs: testLangs}
	ctx := context.Background()
	set := TrackSettings{Enabled: true, Audio: []string{"ja", "en"}, Subtitles: []string{"en"}}
	if err := SaveTrackSettings(ctx, w.s, set, domain.System); err != nil {
		t.Fatal(err)
	}
	if n, err := l.SetDefaultTracks(ctx); err != nil || n != 1 {
		t.Fatalf("%d %v", n, err)
	}
	a := cfgs["a"]
	if a["AudioLanguagePreference"] != "jpn" || a["SubtitleLanguagePreference"] != "eng" || a["SubtitleMode"] != "Always" || a["PlayDefaultAudioTrack"] != false || a["Other"] != "kept" {
		t.Errorf("a: %v", a)
	}
	if cfgs["b"]["AudioLanguagePreference"] != "ger" || cfgs["b"]["SubtitleMode"] != "Smart" {
		t.Errorf("b's own pick changed: %v", cfgs["b"])
	}
	// The owner changes the list: what HolyArrs gave follows it.
	set.Audio = []string{"ko"}
	if err := SaveTrackSettings(ctx, w.s, set, domain.System); err != nil {
		t.Fatal(err)
	}
	if n, err := l.SetDefaultTracks(ctx); err != nil || n != 1 || cfgs["a"]["AudioLanguagePreference"] != "kor" {
		t.Fatalf("%d %v %v", n, err, cfgs["a"])
	}
	if n, _ := l.SetDefaultTracks(ctx); n != 0 || posts != 2 {
		t.Errorf("again: %d posts %d", n, posts)
	}
}
