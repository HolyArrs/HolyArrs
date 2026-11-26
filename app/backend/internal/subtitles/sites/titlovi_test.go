package sites

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The pages below have the shape of Titlovi's search page (one
// <li class="subtitleContainer"> per subtitle: heading with data-id, release in
// <h4>, a flag image named after the country). The titles and releases are made
// up, and no subtitle text is real.

func tlLi(id, title, year, sEp, release, fps, flag string) string {
	s := `<li class="subtitleContainer canEdit"><h3 data-id="` + id + `"><a href="/titlovi/x-` + id + `/">` + title + `</a> <i>(` + year + `)</i>`
	if sEp != "" {
		s += `<span class="s0xe0y">` + sEp + `</span>`
	}
	s += `<div class="moviePopup"></div></h3><h4>` + release
	if fps != "" {
		s += `<span class="fps">fps: ` + fps + `</span>`
	}
	s += `</h4><div class="icons"><img class="lang" src="https://static.titlovi.com/content/images/flags/` + flag + `" alt="7" />` +
		`<div class="autorski" title="x"><div class="icon"></div></div></div><ul class="rating"><li class="full"></li><li class="full"></li></ul></li>`
	return s
}

func tlPage(items ...string) string {
	return `<html><body><ul class="titlovi">` + strings.Join(items, "\n") + `</ul></body></html>`
}

func tlZip(t *testing.T, files map[string]string, order ...string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, n := range order {
		w, err := zw.Create(n)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(files[n]))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func tlZipNames(t *testing.T, b []byte) []string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, f := range zr.File {
		out = append(out, f.Name)
	}
	return out
}

func TestTitloviLang(t *testing.T) {
	for src, want := range map[string]string{
		"https://static.titlovi.com/x/flags/hr3.png": "hr",
		"/flags/rs3.png":          "sr",
		"/flags/ba3.png":          "bs",
		"/flags/mk3.png":          "mk",
		"/flags/gb3.png":          "en",
		"/flags/rs_cirilica3.png": "", // Cyrillic Serbian is left out
		"/flags/zz3.png":          "",
		"":                        "",
	} {
		if got := tlLang(src); got != want {
			t.Errorf("tlLang(%q) = %q, want %q", src, got, want)
		}
	}
}

func TestTitloviWho(t *testing.T) {
	for in, want := range map[string][3]int{
		"Show.S02E05.720p.WEB":     {2, 5, 1},
		"show s2 e05":              {2, 5, 1},
		"Show.2x05.HDTV":           {2, 5, 1},
		"Show.S03.COMPLETE.WEB":    {3, 0, 1},
		"Show Season 4 Complete":   {4, 0, 1},
		"Sintel.2010.1080p.BluRay": {0, 0, 0},
		"Movie.1280x720.x264":      {0, 0, 0},
	} {
		s, e, ok := tlWho(in)
		got := [3]int{s, e, 0}
		if ok {
			got[2] = 1
		}
		if got != want {
			t.Errorf("tlWho(%q) = %v, want %v", in, got, want)
		}
	}
}

func titloviServer(t *testing.T, searches map[string]string, downloads map[string][]byte, hits *[]string) Site {
	t.Helper()
	return fakeSite(t, "titlovi", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits != nil {
			*hits = append(*hits, r.URL.RequestURI())
		}
		switch r.URL.Path {
		case "/titlovi/":
			q := r.URL.Query()
			key := q.Get("t") + "|" + q.Get("prijevod") + "|" + q.Get("s") + "|" + q.Get("e")
			if page, ok := searches[key]; ok {
				_, _ = w.Write([]byte(page))
				return
			}
			_, _ = w.Write([]byte(tlPage()))
		case "/download/":
			if b, ok := downloads[r.URL.Query().Get("mediaid")]; ok {
				w.Header().Set("Content-Type", "application/x-.zip-compressed")
				_, _ = w.Write(b)
				return
			}
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestTitloviMovie(t *testing.T) {
	page := tlPage(
		tlLi("101", "The Lantern", "2010", "", "720p.WEB-LAB &amp; Co", "23.976", "hr3.png"),
		tlLi("102", "The Lantern", "2010", "", "1080p.BluRay-LAB", "", "rs3.png"),
		tlLi("103", "The Lantern", "2010", "", "WEB", "", "rs_cirilica3.png"),
		tlLi("104", "The Lantern", "2010", "", "WEB", "", "mk3.png"),
		tlLi("105", "The Lantern", "2010", "", "WEB", "", "zz3.png"),
	)
	var hits []string
	s := titloviServer(t, map[string]string{"1|tt0000101||": page}, nil, &hits)
	got, err := s.Search(context.Background(), Query{Kind: "movie", Title: "The Lantern", Year: 2010, IMDb: "tt0000101", Languages: []string{"hr", "sr"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 (hr and sr; Cyrillic, mk and unknown left out), got %+v", got)
	}
	if got[0].Key != "101" || got[0].Language != "hr" || got[0].Title != "The Lantern" || got[0].Year != 2010 || got[0].FPS != 23.976 {
		t.Errorf("first: %+v", got[0])
	}
	if len(got[0].ReleaseNames) != 1 || got[0].ReleaseNames[0] != "720p.WEB-LAB & Co" {
		t.Errorf("release (fps text must not stick to it, entities decoded): %v", got[0].ReleaseNames)
	}
	if got[1].Key != "102" || got[1].Language != "sr" {
		t.Errorf("second: %+v", got[1])
	}
	if len(hits) != 1 || !strings.Contains(hits[0], "prijevod=tt0000101") || !strings.Contains(hits[0], "t=1") {
		t.Errorf("one search by the IMDb id first: %v", hits)
	}
}

// The last result is not closed before the sidebar, whose headings must not
// end up in its release name or heading.
func TestTitloviSidebarAfterLastItem(t *testing.T) {
	sidebar := `<div class="side"><h3 data-id="999">Elsewhere</h3><h4>Godina:</h4><h4>Žanr:</h4>` +
		`<h4>U zadnja 24 sata bilo je oko <b>291.724</b> pretraživanja.</h4></div>`
	last := strings.TrimSuffix(tlLi("601", "The Lantern", "2010", "", "WEB-LAB", "24", "hr3.png"), "</li>")
	items := tlParse([]byte(`<ul>` + last + sidebar))
	if len(items) != 1 {
		t.Fatalf("one item, got %+v", items)
	}
	if it := items[0]; it.id != "601" || it.release != "WEB-LAB" || it.title != "The Lantern" || it.year != 2010 || it.fps != 24 {
		t.Errorf("sidebar text leaked into the item: %+v", it)
	}
}

func TestTitloviMovieFallsBackToTitle(t *testing.T) {
	var hits []string
	s := titloviServer(t, map[string]string{
		"1|The Lantern||": tlPage(tlLi("201", "The Lantern", "2010", "", "WEB", "", "hr3.png")),
	}, nil, &hits)
	got, err := s.Search(context.Background(), Query{Kind: "movie", Title: "The Lantern", IMDb: "tt0000101", Languages: []string{"hr"}})
	if err != nil || len(got) != 1 || got[0].Key != "201" {
		t.Fatalf("got %+v, %v", got, err)
	}
	if len(hits) != 2 {
		t.Errorf("IMDb search, then title: %v", hits)
	}
}

func TestTitloviNothingToAsk(t *testing.T) {
	s := titloviServer(t, nil, nil, nil)
	for _, q := range []Query{
		{Kind: "movie", Title: "The Lantern"}, // no language
		{Kind: "movie", Languages: []string{"hr"}},
		{Kind: "movie", Title: "The Lantern", Languages: []string{"zz"}},
	} {
		if got, err := s.Search(context.Background(), q); err != nil || len(got) != 0 {
			t.Errorf("%+v: %v, %v", q, got, err)
		}
	}
}

func TestTitloviEpisode(t *testing.T) {
	page := tlPage(
		tlLi("301", "The Show", "2020", "Sezona 2 Epizoda 5", "WEB", "", "hr3.png"),
		tlLi("302", "The Show", "2020", "Sezona 2 Epizoda 6", "WEB", "", "hr3.png"), // another episode
		tlLi("303", "The Show", "2020", "Sezona 3 Epizoda 5", "WEB", "", "hr3.png"), // another season
		tlLi("304", "The Show", "2020", "", "The.Show.S02E05.720p", "", "hr3.png"),  // no heading: the release decides
		tlLi("305", "The Show", "2020", "", "The.Show.S02E07.720p", "", "hr3.png"),  // release names another episode
	)
	var hits []string
	s := titloviServer(t, map[string]string{"2|The Show|2|5": page}, nil, &hits)
	got, err := s.Search(context.Background(), Query{Kind: "episode", Title: "The Show", Season: 2, Episode: 5, Languages: []string{"hr"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Key != "301" || got[1].Key != "304" {
		t.Fatalf("want 301 and 304, got %+v", got)
	}
	for _, c := range got {
		if c.Season != 2 || c.Episode != 5 || strings.Contains(c.Key, ":") {
			t.Errorf("single episodes, not packs: %+v", c)
		}
	}
	if len(hits) != 1 || !strings.Contains(hits[0], "t=2") || !strings.Contains(hits[0], "s=2") || !strings.Contains(hits[0], "e=5") {
		t.Errorf("hits %v", hits)
	}
}

func TestTitloviSeasonPackFallback(t *testing.T) {
	var hits []string
	s := titloviServer(t, map[string]string{
		// No exact episode, but a pack for the season.
		"2|The Show|2|": tlPage(
			tlLi("401", "The Show", "2020", "Sezona 2", "The.Show.S02.COMPLETE", "", "hr3.png"),
			tlLi("402", "The Show", "2020", "Sezona 3", "The.Show.S03.COMPLETE", "", "hr3.png"),
		),
	}, nil, &hits)
	got, err := s.Search(context.Background(), Query{Kind: "episode", Title: "The Show", Season: 2, Episode: 5, Languages: []string{"hr"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Key != "401:e5" || got[0].Season != 2 || got[0].Episode != 0 {
		t.Fatalf("a pack carries the wanted episode in its key: %+v", got)
	}
	if len(hits) != 2 || strings.Contains(hits[1], "e=") {
		t.Errorf("exact search, then a season search without an episode: %v", hits)
	}
}

func TestTitloviDownload(t *testing.T) {
	cue := "1\n00:00:01,000 --> 00:00:02,000\nmade up\n"
	single := tlZip(t, map[string]string{"x.srt": cue}, "x.srt")
	pack := tlZip(t, map[string]string{
		"The.Show.S02E04.srt": "four\n", "The.Show.S02E05.srt": cue, "The.Show.S02E06.srt": "six\n", "readme.txt": "hi",
	}, "The.Show.S02E04.srt", "The.Show.S02E05.srt", "The.Show.S02E06.srt", "readme.txt")
	noEp := tlZip(t, map[string]string{"a.srt": "a\n", "b.srt": "b\n"}, "a.srt", "b.srt")
	s := titloviServer(t, nil, map[string][]byte{"501": single, "502": pack, "503": noEp, "504": []byte("<html>not an archive</html>")}, nil)
	ctx := context.Background()

	b, err := s.Download(ctx, Candidate{Key: "501"})
	if err != nil || !bytes.Equal(b, single) {
		t.Fatalf("a plain zip passes through: %v", err)
	}
	b, err = s.Download(ctx, Candidate{Key: "502:e5", Season: 2})
	if err != nil {
		t.Fatal(err)
	}
	if names := tlZipNames(t, b); len(names) != 1 || names[0] != "The.Show.S02E05.srt" || unzipOne(t, b) != cue {
		t.Errorf("the pack gives only episode 5: %v", tlZipNames(t, b))
	}
	if _, err = s.Download(ctx, Candidate{Key: "502:e9", Season: 2}); err == nil {
		t.Error("a pack with no file for the episode is unreadable")
	}
	if b, err = s.Download(ctx, Candidate{Key: "503"}); err != nil || !bytes.Equal(b, noEp) {
		t.Errorf("without an episode a zip passes through: %v", err)
	}
	var se *Error
	if _, err = s.Download(ctx, Candidate{Key: "504"}); !errors.As(err, &se) || se.Kind != KindUnreadable {
		t.Errorf("a page instead of an archive is unreadable: %v", err)
	}
	for _, bad := range []string{"", "x", "../1", "12:e", "12:e0", "12:eX", "99999999999"} {
		if _, err = s.Download(ctx, Candidate{Key: bad}); !errors.As(err, &se) || se.Kind != KindUnreadable {
			t.Errorf("key %q must be refused: %v", bad, err)
		}
	}
}

func TestTitloviBlockedIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "challenge", http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)
	s, err := New("titlovi", cfg(srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	// A block must not read as "no subtitles": the site has to rest.
	got, err := s.Search(context.Background(), Query{Kind: "movie", Title: "The Lantern", IMDb: "tt0000101", Languages: []string{"hr"}})
	if err == nil || len(got) != 0 {
		t.Fatalf("want an error, got %v, %v", got, err)
	}
}

func TestTitloviKindListed(t *testing.T) {
	k, ok := KindOf("titlovi")
	if !ok || !k.Movies || !k.Shows || k.NeedsAccount || k.DefaultOff || k.Lab || k.Later {
		t.Errorf("kind: %+v %v", k, ok)
	}
}

// A search by IMDb id that finds the film is not repeated by title: the title
// search would list the same subtitles and only spend a request of the site's
// hourly limit.
func TestTitloviStopsAfterTheIDSearchFoundTheFilm(t *testing.T) {
	other := tlPage(tlLi("701", "The Lantern", "2010", "", "WEB", "", "mk3.png")) // a language nobody asked for
	var hits []string
	s := titloviServer(t, map[string]string{
		"1|tt0000101||":   other,
		"1|The Lantern||": other,
	}, nil, &hits)
	got, err := s.Search(context.Background(), Query{Kind: "movie", Title: "The Lantern", IMDb: "tt0000101", Languages: []string{"hr"}})
	if err != nil || len(got) != 0 {
		t.Fatalf("nothing in Croatian: %+v %v", got, err)
	}
	if len(hits) != 1 || !strings.Contains(hits[0], "prijevod=tt0000101") {
		t.Errorf("only the IMDb search: %v", hits)
	}
}

// For an episode that is two requests at most (the exact episode, then the
// season) when the id finds the show, not four.
func TestTitloviEpisodeCostsTwoRequestsWhenTheIDFindsTheShow(t *testing.T) {
	other := tlPage(tlLi("711", "The Show", "2020", "Sezona 2 Epizoda 5", "WEB", "", "mk3.png"))
	var hits []string
	s := titloviServer(t, map[string]string{
		"2|tt0000202|2|5": other,
		"2|tt0000202|2|":  other,
	}, nil, &hits)
	got, err := s.Search(context.Background(), Query{Kind: "episode", Title: "The Show", IMDb: "tt0000202", Season: 2, Episode: 5, Languages: []string{"hr"}})
	if err != nil || len(got) != 0 {
		t.Fatalf("%+v %v", got, err)
	}
	if len(hits) != 2 {
		t.Fatalf("want 2 requests, got %v", hits)
	}
	for _, h := range hits {
		if strings.Contains(h, "The+Show") {
			t.Errorf("no search by title: %v", hits)
		}
	}
}

// When the id finds nothing at all, the title is tried.
func TestTitloviTriesTheTitleWhenTheIDFindsNothing(t *testing.T) {
	var hits []string
	s := titloviServer(t, map[string]string{
		"1|The Lantern||": tlPage(tlLi("721", "The Lantern", "2010", "", "WEB", "", "hr3.png")),
	}, nil, &hits)
	got, err := s.Search(context.Background(), Query{Kind: "movie", Title: "The Lantern", IMDb: "tt0000101", Languages: []string{"hr"}})
	if err != nil || len(got) != 1 || got[0].Key != "721" || len(hits) != 2 {
		t.Fatalf("%+v %v %v", got, err, hits)
	}
}

// A result of the IMDb id search is marked as found by the title's own id, so a
// film the site names differently is not refused as a wrong title; a result of
// the title search is not.
func TestTitloviMarksResultsOfTheIDSearch(t *testing.T) {
	byID := tlPage(tlLi("731", "The Lantern: The Movie", "2010", "", "WEB", "", "hr3.png"))
	byName := tlPage(tlLi("732", "The Lantern", "2010", "", "WEB", "", "hr3.png"))
	s := titloviServer(t, map[string]string{"1|tt0000101||": byID, "1|The Lantern||": byName}, nil, nil)
	got, err := s.Search(context.Background(), Query{Kind: "movie", Title: "The Lantern", IMDb: "tt0000101", Languages: []string{"hr"}})
	if err != nil || len(got) != 1 || got[0].Key != "731" || !got[0].MatchedByID {
		t.Fatalf("a result of the id search is marked: %+v %v", got, err)
	}
	// Without an IMDb id only the title is searched, and nothing is marked.
	got, err = s.Search(context.Background(), Query{Kind: "movie", Title: "The Lantern", Languages: []string{"hr"}})
	if err != nil || len(got) != 1 || got[0].Key != "732" || got[0].MatchedByID {
		t.Fatalf("a result of the title search is not: %+v %v", got, err)
	}
	// The same for an episode found by the show's IMDb id.
	ep := titloviServer(t, map[string]string{"2|tt0000202|2|5": tlPage(tlLi("741", "The Show", "2020", "Sezona 2 Epizoda 5", "WEB", "", "hr3.png"))}, nil, nil)
	got, err = ep.Search(context.Background(), Query{Kind: "episode", Title: "The Show", IMDb: "tt0000202", Season: 2, Episode: 5, Languages: []string{"hr"}})
	if err != nil || len(got) != 1 || !got[0].MatchedByID {
		t.Fatalf("%+v %v", got, err)
	}
}
