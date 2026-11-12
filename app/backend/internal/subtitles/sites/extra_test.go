package sites

import (
	"archive/zip"
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/sites/fakesite"
)

func fakeSite(t *testing.T, kind string, h http.Handler) Site {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	s, err := New(kind, cfg(srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func isZip(b []byte) bool { return bytes.HasPrefix(b, []byte("PK\x03\x04")) }

func unzipOne(t *testing.T, b []byte) string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil || len(zr.File) != 1 {
		t.Fatalf("zip %v", err)
	}
	f, _ := zr.File[0].Open()
	defer f.Close()
	var sb strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := f.Read(buf)
		sb.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return sb.String()
}

func yifyFake() *fakesite.Yify {
	return &fakesite.Yify{Movies: map[string][]fakesite.YifyRow{
		"tt0000101": {
			{ID: "lantern-en-1", Rating: 3, Language: "English", Release: "The.Lantern.2010.720p.WEB-LAB", Uploader: "maker", Text: fakesite.Cue("made up", 1000)},
			{ID: "lantern-en-2", Rating: 9, Language: "English", Release: "The.Lantern.2010.1080p.BluRay.x264-LAB", HearingImpaired: true, Uploader: "other", Text: fakesite.Cue("also made up", 1000)},
			{ID: "lantern-br", Rating: 5, Language: "Brazilian Portuguese", Release: "The.Lantern.2010.WEB", Text: fakesite.Cue("inventado", 1000)},
			{ID: "lantern-xx", Rating: 5, Language: "Klingon", Release: "The.Lantern.2010.WEB", Text: "x"},
			{ID: "lantern-fa", Rating: 1, Language: "Farsi/Persian", Release: "The.Lantern.2010.WEB", Text: "x", NoLink: true},
			{ID: "lantern-nz", Rating: 1, Language: "French", Release: "The.Lantern.2010.WEB", Text: "notzip"},
		},
	}}
}

var yifyQuery = Query{Kind: "movie", Title: "The Lantern", Year: 2010, IMDb: "tt0000101", Languages: []string{"en"}}

func TestYifySearchAndDownload(t *testing.T) {
	f := yifyFake()
	s := fakeSite(t, "yifysubtitles", f)
	ctx := context.Background()
	got, err := s.Search(ctx, yifyQuery)
	if err != nil || len(got) != 2 {
		t.Fatalf("%+v %v", got, err)
	}
	best := got[0]
	if best.Key != "lantern-en-2" || !best.HearingImpaired || !best.MatchedByID || best.Language != "en" || best.Uploader != "other" ||
		len(best.ReleaseNames) != 1 || best.ReleaseNames[0] != "The.Lantern.2010.1080p.BluRay.x264-LAB" || best.Title != "The Lantern" || best.Year != 2010 {
		t.Fatalf("best %+v", best)
	}
	b, err := s.Download(ctx, best)
	if err != nil || !isZip(b) || !strings.Contains(unzipOne(t, b), "also made up") {
		t.Fatalf("download %q %v", b, err)
	}
	if last := f.Referers[len(f.Referers)-1]; !strings.HasSuffix(last, "/subtitles/lantern-en-2") {
		t.Fatalf("download referer %q", last)
	}
	if !strings.HasSuffix(f.Referers[0], "/") || !strings.Contains(f.Agents[0], "Mozilla") {
		t.Fatalf("search referer %q agent %q", f.Referers[0], f.Agents[0])
	}

	// Brazilian Portuguese answers a pt wish and Farsi/Persian maps to fa; an
	// unknown language name is skipped.
	q := yifyQuery
	q.Languages = []string{"pt", "fa"}
	got, err = s.Search(ctx, q)
	if err != nil || len(got) != 2 || got[0].Language != "pt-br" || got[1].Language != "fa" {
		t.Fatalf("pt/fa %+v %v", got, err)
	}
	// A page without a download link, or a reply that is not a zip, fails.
	if _, err := s.Download(ctx, got[1]); kindOf(err) != KindUnreadable {
		t.Fatalf("no link: %v", err)
	}
	q.Languages = []string{"fr"}
	got, _ = s.Search(ctx, q)
	if len(got) != 1 {
		t.Fatalf("fr %+v", got)
	}
	if _, err := s.Download(ctx, got[0]); kindOf(err) != KindUnreadable {
		t.Fatalf("not a zip: %v", err)
	}
}

func TestYifySkipsShowsUnknownMoviesAndHostilePages(t *testing.T) {
	f := yifyFake()
	s := fakeSite(t, "yifysubtitles", f)
	ctx := context.Background()
	for _, q := range []Query{
		{Kind: "episode", Title: "The Lantern", IMDb: "tt0000101", Season: 1, Episode: 1, Languages: []string{"en"}},
		{Kind: "movie", Title: "The Lantern", Languages: []string{"en"}},                    // no IMDb id
		{Kind: "movie", Title: "The Lantern", IMDb: "../etc", Languages: []string{"en"}},    // not an IMDb id
		{Kind: "movie", Title: "Unknown", IMDb: "tt0000999", Languages: []string{"en"}},     // 404 is no results
		{Kind: "movie", Title: "The Lantern", IMDb: "tt0000101", Languages: []string{"xx"}}, // nothing wanted
	} {
		if got, err := s.Search(ctx, q); err != nil || len(got) != 0 {
			t.Fatalf("%+v: %+v %v", q, got, err)
		}
	}
	if f.Requests != 2 {
		t.Fatalf("requests %d, want the unknown movie and the unknown language", f.Requests)
	}
	for _, hostile := range []string{
		`<table class="other-subs"><tr><td>1</td><td>English</td><td><a href="//evil.example/x">subtitle x</a></td><td></td><td>u</td></tr></table>`,
		`<table class="other-subs"><tr><td>1</td><td>English</td><td><a href="http://evil.example/subtitles/x">subtitle x</a></td><td></td><td>u</td></tr></table>`,
		`<table class="other-subs"><tr><td>1</td><td>English</td><td><a href="/subtitles/../../x">subtitle x</a></td><td></td><td>u</td></tr></table>`,
		`<table class="other-subs"><tr><td>1` + strings.Repeat("<td>", 5000),
		`<table class="other-subs"><tr><td>nope</td><td>English</td><td><a href="/subtitles/x">subtitle x</a></td><td></td><td>u</td></tr></table>`,
	} {
		f.Hostile = hostile
		if got, err := s.Search(ctx, yifyQuery); err != nil || len(got) != 0 {
			t.Fatalf("hostile %q: %+v %v", hostile[:40], got, err)
		}
	}
}

func tvFake() *fakesite.TVSubs {
	return &fakesite.TVSubs{
		Shows: map[int]string{12: "Quiet Harbour (2005-2013)", 13: "Quiet Harbour Nights (2015-2016)", 14: "Quiet Harbour (US) (2019-2020)"},
		Subs: map[int][]fakesite.TVSub{
			12: {
				{ID: 501, Season: 1, Episode: 2, Flag: "en", Release: "Quiet.Harbour.S01E02.WEB-KILLERS", Rip: "WEB-DL", Text: fakesite.Cue("made up", 1000)},
				{ID: 502, Season: 1, Episode: 2, Flag: "br", Release: "Quiet.Harbour.S01E02.HDTV", Rip: "HDTV", Text: fakesite.Cue("inventado", 1000)},
				{ID: 503, Season: 1, Episode: 2, Flag: "gr", Release: "Quiet.Harbour.S01E02.HDTV", Rip: "HDTV", Text: "x", TwoFiles: true},
				{ID: 504, Season: 1, Episode: 3, Flag: "en", Release: "Quiet.Harbour.S01E03.WEB", Rip: "WEB", Text: "x"},
				{ID: 505, Season: 1, Episode: 2, Flag: "xx", Release: "Quiet.Harbour.S01E02.WEB", Rip: "WEB", Text: "x"},
			},
			14: {{ID: 601, Season: 1, Episode: 2, Flag: "en", Release: "Quiet.Harbour.US.S01E02", Rip: "WEB", Text: "x"}},
		},
	}
}

var tvQuery = Query{Kind: "episode", Title: "Quiet Harbour", Year: 2005, Season: 1, Episode: 2, Languages: []string{"en"}}

func TestTVSubtitlesSearchAndDownload(t *testing.T) {
	f := tvFake()
	s := fakeSite(t, "tvsubtitles", f)
	ctx := context.Background()
	got, err := s.Search(ctx, tvQuery)
	if err != nil || len(got) != 1 {
		t.Fatalf("%+v %v", got, err)
	}
	c := got[0]
	if c.Key != "501" || c.Language != "en" || c.Season != 1 || c.Episode != 2 || c.Title != "Quiet Harbour" || c.Year != 2005 ||
		len(c.ReleaseNames) != 2 || c.ReleaseNames[0] != "Quiet.Harbour.S01E02.WEB-KILLERS" || c.ReleaseNames[1] != "WEB-DL" {
		t.Fatalf("candidate %+v", c)
	}
	if len(f.Searches) != 1 || f.Searches[0] != "Quiet Harbour" {
		t.Fatalf("searches %v", f.Searches)
	}
	b, err := s.Download(ctx, c)
	if err != nil || !isZip(b) || !strings.Contains(unzipOne(t, b), "made up") {
		t.Fatalf("download %q %v", b, err)
	}

	// The show and season pages are kept, so a second language costs only the
	// episode page.
	before := f.Requests
	q := tvQuery
	q.Languages = []string{"pt", "el"}
	got, err = s.Search(ctx, q)
	if err != nil || len(got) != 2 || got[0].Language != "pt-br" || got[1].Language != "el" {
		t.Fatalf("pt/el %+v %v", got, err)
	}
	if f.Requests-before != 1 {
		t.Fatalf("second search made %d calls, want 1", f.Requests-before)
	}
	// A zip with two files is a failed download.
	if _, err := s.Download(ctx, got[1]); kindOf(err) != KindUnreadable {
		t.Fatalf("two files: %v", err)
	}
	// The year picks between shows of the same name; (US) is dropped from the name.
	q = tvQuery
	q.Year = 2019
	got, err = s.Search(ctx, q)
	if err != nil || len(got) != 1 || got[0].Key != "601" {
		t.Fatalf("2019 %+v %v", got, err)
	}
}

func TestTVSubtitlesSkipsMoviesAndUnknownShows(t *testing.T) {
	f := tvFake()
	s := fakeSite(t, "tvsubtitles", f)
	ctx := context.Background()
	for _, q := range []Query{
		{Kind: "movie", Title: "Quiet Harbour", Languages: []string{"en"}},
		{Kind: "episode", Title: "Quiet Harbour", Season: 0, Episode: 2, Languages: []string{"en"}},
		{Kind: "episode", Title: "Loud Harbour", Season: 1, Episode: 2, Languages: []string{"en"}},
		{Kind: "episode", Title: "Quiet Harbour", Year: 1999, Season: 1, Episode: 2, Languages: []string{"en"}},
		{Kind: "episode", Title: "Quiet Harbour", Season: 1, Episode: 9, Languages: []string{"en"}},
	} {
		if got, err := s.Search(ctx, q); err != nil || len(got) != 0 {
			t.Fatalf("%+v: %+v %v", q, got, err)
		}
	}
	// A download key that is not a number is refused before any call.
	before := f.Requests
	if _, err := s.Download(ctx, Candidate{Key: "../x", Link: "../x"}); kindOf(err) != KindUnreadable || f.Requests != before {
		t.Fatalf("bad key: %v", err)
	}
}

func TestTVSubtitlesDownloadPath(t *testing.T) {
	for _, tc := range []struct {
		page string
		ok   bool
	}{
		{`<script>var s1= 'files/'; var s2= 'a.zip';</script>`, true},
		{`<script>var s1= 'http://evil.example/'; var s2= 'a.zip';</script>`, false},
		{`<script>var s1= '//evil.example/'; var s2= 'a.zip';</script>`, false},
		{`<script>var s1= 'files/../../'; var s2= 'a.zip';</script>`, false},
		{`<p>no script</p>`, false},
	} {
		p, ok := tvDownloadPath([]byte(tc.page))
		if ok != tc.ok {
			t.Errorf("%q: %q %v", tc.page, p, ok)
		}
		if ok && p != "/files/a.zip" {
			t.Errorf("path %q", p)
		}
	}
}

func bspFake() *fakesite.BSPlayer {
	return &fakesite.BSPlayer{Items: []fakesite.BSPItem{
		{ID: "71", Hash: "00000000000000ff", Size: 734003200, Lang: "eng", Name: "The.Lantern.2010.1080p.BluRay.x264-LAB.srt", Text: fakesite.Cue("made up", 1000)},
		{ID: "72", Hash: "00000000000000ff", Size: 734003200, Lang: "pob", Name: "The.Lantern.2010.WEB.srt", Text: fakesite.Cue("inventado", 1000)},
		{ID: "73", Hash: "00000000000000aa", Size: 734003200, Lang: "eng", Name: "Other.srt", Text: "x"},
	}}
}

var bspQuery = Query{Kind: "movie", Title: "The Lantern", Year: 2010, IMDb: "tt0000101", Hash: "00000000000000FF", Size: 734003200, Languages: []string{"en", "pt-br"}}

func TestBSPlayerSearchAndDownload(t *testing.T) {
	old := bspSearchWait
	bspSearchWait = 0
	t.Cleanup(func() { bspSearchWait = old })
	f := bspFake()
	s := fakeSite(t, "bsplayer", f)
	ctx := context.Background()
	got, err := s.Search(ctx, bspQuery)
	if err != nil || len(got) != 2 {
		t.Fatalf("%+v %v", got, err)
	}
	c := got[0]
	if c.Key != "71" || c.Language != "en" || !c.MatchedByID || c.Title != "The Lantern" || c.Year != 2010 ||
		len(c.ReleaseNames) != 1 || c.ReleaseNames[0] != "The.Lantern.2010.1080p.BluRay.x264-LAB" || c.Format != "srt" {
		t.Fatalf("candidate %+v", c)
	}
	if got[1].Language != "pt-br" {
		t.Fatalf("second %+v", got[1])
	}
	if strings.Join(f.Calls, ",") != "logIn,searchSubtitles,logOut" {
		t.Fatalf("calls %v", f.Calls)
	}
	if sr := f.Searches[0]; sr.Hash != "00000000000000ff" || sr.Size != "734003200" || sr.Langs != "eng,pob" || sr.IMDb != "0000101" {
		t.Fatalf("search %+v", sr)
	}
	if f.Handles[0] != "made-up-handle" || !strings.Contains(f.Agents[0], "BSPlayer") {
		t.Fatalf("handles %v agents %v", f.Handles, f.Agents)
	}
	b, err := s.Download(ctx, c)
	if err != nil || !strings.Contains(string(b), "made up") {
		t.Fatalf("download %q %v", b, err)
	}
	// Episodes work the same way, and an unknown IMDb id goes as *.
	q := bspQuery
	q.Kind, q.IMDb, q.Season, q.Episode = "episode", "", 1, 2
	if _, err := s.Search(ctx, q); err != nil || f.Searches[len(f.Searches)-1].IMDb != "*" {
		t.Fatalf("episode %v %+v", err, f.Searches)
	}
	f.Fail500 = true
	if _, err := s.Download(ctx, c); kindOf(err) != KindDown {
		t.Fatalf("500 body: %v", err)
	}
}

func TestBSPlayerNeedsHashAndHandlesBusySite(t *testing.T) {
	old := bspSearchWait
	bspSearchWait = 0
	t.Cleanup(func() { bspSearchWait = old })
	f := bspFake()
	s := fakeSite(t, "bsplayer", f)
	ctx := context.Background()
	for _, q := range []Query{
		{Kind: "movie", Title: "The Lantern", Languages: []string{"en"}},
		{Kind: "movie", Title: "The Lantern", Hash: "00000000000000ff", Languages: []string{"en"}}, // no size
		{Kind: "movie", Title: "The Lantern", Hash: "nothex", Size: 10, Languages: []string{"en"}},
		{Kind: "movie", Title: "The Lantern", Hash: "00000000000000ff", Size: 734003200, Languages: []string{"xx"}},
	} {
		if got, err := s.Search(ctx, q); err != nil || len(got) != 0 {
			t.Fatalf("%+v: %+v %v", q, got, err)
		}
	}
	if len(f.Calls) != 0 {
		t.Fatalf("calls %v", f.Calls)
	}
	f.NotXML = true
	if _, err := s.Search(ctx, bspQuery); kindOf(err) != KindDown || !strings.Contains(err.Error(), "busy") {
		t.Fatalf("not xml: %v", err)
	}
	if len(f.Calls) != bspTries {
		t.Fatalf("tries %d", len(f.Calls))
	}
}

func TestBSPlayerLinksStayOnItsHosts(t *testing.T) {
	b := &bsplayer{}
	for link, ok := range map[string]bool{
		"http://s3.api.bsplayer-subtitles.com/dl/1":       true,
		"https://s101.api.bsplayer-subtitles.com/dl/1":    true,
		"http://evil.example/dl/1":                        false,
		"http://api.bsplayer-subtitles.com.evil.example/": false,
		"http://user@s3.api.bsplayer-subtitles.com/dl/1":  false,
		"ftp://s3.api.bsplayer-subtitles.com/dl/1":        false,
	} {
		if got := b.linkOK(link, "s1.api.bsplayer-subtitles.com"); got != ok {
			t.Errorf("%s: %v", link, got)
		}
	}
}

func TestBSPlayerServerList(t *testing.T) {
	hosts := bspHosts()
	if len(hosts) != bspServers {
		t.Fatalf("%d hosts", len(hosts))
	}
	for _, h := range hosts {
		if !strings.HasSuffix(h, ".api.bsplayer-subtitles.com") {
			t.Fatalf("host %s", h)
		}
	}
}
