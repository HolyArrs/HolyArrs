package sites

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/sites/fakesite"
)

func a7Fake() *fakesite.Addic7ed {
	return &fakesite.Addic7ed{User: fakeUser, Pass: fakePass, Shows: map[int]string{12: "Quiet Harbour", 13: "Quiet Harbour Nights"}, Subs: []fakesite.Sub{
		{ID: "301", Title: "Quiet Harbour", Season: 1, Episode: 2, Language: "English", Release: "WEB.KILLERS, WEB-DL", HearingImpaired: true, Text: fakesite.Cue("made up", 1000)},
		{ID: "302", Title: "Quiet Harbour", Season: 1, Episode: 2, Language: "French", Release: "WEB", Text: fakesite.Cue("invente", 1000)},
		{ID: "303", Title: "Quiet Harbour", Season: 1, Episode: 2, Language: "English", Release: "HDTV", Downloads: -1, Text: "x"},
		{ID: "304", Title: "Quiet Harbour", Season: 1, Episode: 3, Language: "English", Release: "WEB", Text: "x"},
	}}
}

func a7Site(t *testing.T, f *fakesite.Addic7ed, user, pass string) (Site, *httptest.Server) {
	t.Helper()
	old := addic7edMinGap
	addic7edMinGap = time.Millisecond
	t.Cleanup(func() { addic7edMinGap = old })
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	s, err := New("addic7ed", Config{Name: "A7", URL: srv.URL, Username: user, Password: pass, GapMS: 1, AllowLocal: true})
	if err != nil {
		t.Fatal(err)
	}
	return s, srv
}

var a7Query = Query{Kind: "episode", Title: "Quiet Harbour", Season: 1, Episode: 2, Languages: []string{"en"}}

func TestAddic7edSearchAndDownload(t *testing.T) {
	f := a7Fake()
	s, _ := a7Site(t, f, fakeUser, fakePass)
	ctx := context.Background()
	got, err := s.Search(ctx, a7Query)
	if err != nil || len(got) != 1 {
		t.Fatalf("%+v %v", got, err)
	}
	c := got[0]
	if c.Key != "301" || c.Language != "en" || !c.HearingImpaired || c.Season != 1 || c.Episode != 2 ||
		len(c.ReleaseNames) != 2 || c.ReleaseNames[0] != "WEB.KILLERS" || c.ReleaseNames[1] != "WEB-DL" {
		t.Fatalf("candidate %+v", c)
	}
	b, err := s.Download(ctx, c)
	if err != nil || !strings.Contains(string(b), "made up") {
		t.Fatalf("download %q %v", b, err)
	}
	if !strings.Contains(f.LastReferer, "/show/12") || f.Logins != 1 {
		t.Fatalf("referer %q logins %d", f.LastReferer, f.Logins)
	}
	// Movies are not offered.
	if got, err := s.Search(ctx, Query{Kind: "movie", Title: "Quiet Harbour", Languages: []string{"en"}}); err != nil || len(got) != 0 {
		t.Fatalf("movie %+v %v", got, err)
	}
	// Ask for French as well.
	q := a7Query
	q.Languages = []string{"en", "fr"}
	if got, _ := s.Search(ctx, q); len(got) != 2 {
		t.Fatalf("two languages %+v", got)
	}
}

func TestAddic7edLoginRefused(t *testing.T) {
	f := a7Fake()
	s, _ := a7Site(t, f, fakeUser, "wrong-word-xyz")
	_, err := s.Search(context.Background(), a7Query)
	if kindOf(err) != KindLoginRefused {
		t.Fatalf("err %v", err)
	}
	if strings.Contains(err.Error(), "wrong-word-xyz") || strings.Contains(err.Error(), fakeUser) {
		t.Fatalf("account in error: %v", err)
	}
	if k, d := PauseFor(err, 1, time.Now()); k != KindLoginRefused || d != 12*time.Hour {
		t.Fatalf("pause %s %v", k, d)
	}
	s2, _ := a7Site(t, f, "", "")
	if _, err := s2.Search(context.Background(), a7Query); kindOf(err) != KindLoginRefused {
		t.Fatalf("no account: %v", err)
	}
}

func TestAddic7edSessionExpiry(t *testing.T) {
	f := a7Fake()
	s, _ := a7Site(t, f, fakeUser, fakePass)
	ctx := context.Background()
	got, err := s.Search(ctx, a7Query)
	if err != nil || len(got) != 1 {
		t.Fatal(err)
	}
	f.Expire = true
	b, err := s.Download(ctx, got[0])
	if err != nil || !strings.Contains(string(b), "made up") || f.Logins != 2 {
		t.Fatalf("after expiry: %q %v logins %d", b, err, f.Logins)
	}
}

func TestAddic7edCandidateYearNeedsExplicitSearchMatch(t *testing.T) {
	for _, tc := range []struct {
		name string
		show string
		year int
	}{
		{name: "title only", show: "Quiet Harbour"},
		{name: "title and year", show: "Quiet Harbour 2020", year: 2020},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := a7Fake()
			f.Shows = map[int]string{12: tc.show}
			f.Subs[0].Title = tc.show
			s, _ := a7Site(t, f, fakeUser, fakePass)
			q := a7Query
			q.Year = 2020
			got, err := s.Search(context.Background(), q)
			if err != nil || len(got) != 1 {
				t.Fatalf("%+v %v", got, err)
			}
			if got[0].Year != tc.year {
				t.Fatalf("year %d, want %d", got[0].Year, tc.year)
			}
		})
	}
}

func TestAddic7edGenericHTMLDownloadDoesNotSignInAgain(t *testing.T) {
	f := a7Fake()
	s, _ := a7Site(t, f, fakeUser, fakePass)
	got, err := s.Search(context.Background(), a7Query)
	if err != nil || len(got) != 1 {
		t.Fatalf("%+v %v", got, err)
	}
	f.DownloadHTML = `<main>temporarily unavailable</main>`
	_, err = s.Download(context.Background(), got[0])
	if kindOf(err) != KindUnreadable || f.Logins != 1 {
		t.Fatalf("err %v, logins %d", err, f.Logins)
	}
}

func TestAddic7edLoginFormVariants(t *testing.T) {
	for _, page := range [][]byte{
		[]byte(`<INPUT TYPE = 'PASSWORD'>`),
		[]byte(`<input NAME = 'PassWord' type=text>`),
	} {
		if !hasLoginForm(page) {
			t.Fatalf("missed %q", page)
		}
	}
	if hasLoginForm([]byte(`<input data-type="password"><p>password help</p>`)) {
		t.Fatal("accepted an attribute substring or plain text")
	}
}

func TestAddic7edLooksHTMLFragments(t *testing.T) {
	for _, page := range [][]byte{
		[]byte(`<FORM><INPUT TYPE = 'PASSWORD'></FORM>`),
		[]byte(`<main>maintenance</main>`),
	} {
		if !looksHTML(page) {
			t.Fatalf("missed %q", page)
		}
	}
	if looksHTML([]byte("1\n00:00:01,000 --> 00:00:02,000\n<i>hello</i>")) {
		t.Fatal("subtitle text looked like HTML")
	}
}

func TestAddic7edConcurrentFirstSearchSignsInOnce(t *testing.T) {
	f := a7Fake()
	f.LoginDelay = 30 * time.Millisecond
	s, _ := a7Site(t, f, fakeUser, fakePass)
	start := make(chan struct{})
	var wg sync.WaitGroup
	var failures atomic.Int32
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			got, err := s.Search(context.Background(), a7Query)
			if err != nil || len(got) != 1 {
				failures.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()
	if failures.Load() != 0 || f.Logins != 1 {
		t.Fatalf("failures %d, logins %d", failures.Load(), f.Logins)
	}
}

func TestAddic7edRowsPreferDownloadLink(t *testing.T) {
	f := a7Fake()
	f.Hostile = `<table><tr><td>1</td><td>2</td><td><a href="/episode/2">Episode</a></td><td>English</td><td>WEB</td><td>Completed</td><td></td><td><a href="/download/301">Download</a></td></tr></table>`
	s, _ := a7Site(t, f, fakeUser, fakePass)
	got, err := s.Search(context.Background(), a7Query)
	if err != nil || len(got) != 1 || got[0].Key != "301" {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestAddic7edHTMLReaderAttributeBoundaries(t *testing.T) {
	b := []byte(`<A data-href="/wrong" HREF = '/download/301?note=>'>ok</A><SCRIPTURE><a href=/wrong>bad</a></SCRIPTURE><ScRiPt><a href=/wrong>bad</a></sCrIpT>`)
	got := links(b)
	if len(got) != 2 || got[0].href != "/download/301?note=>" || got[0].text != "ok" || got[1].href != "/wrong" {
		t.Fatalf("links %+v", got)
	}
	for _, in := range []string{`data-href=/wrong`, `xhref=/wrong`, `hrefish=/wrong`} {
		if got := attr([]byte(in), "href"); got != "" {
			t.Fatalf("attr %q from %q", got, in)
		}
	}
}

func TestAddic7edRedirectsUseLimits(t *testing.T) {
	old := addic7edMinGap
	addic7edMinGap = 15 * time.Millisecond
	t.Cleanup(func() { addic7edMinGap = old })
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		switch r.URL.Path {
		case "/start":
			http.Redirect(w, r, "/middle", http.StatusFound)
		case "/middle":
			http.Redirect(w, r, "/done", http.StatusFound)
		default:
			fmt.Fprint(w, `<p>done</p>`)
		}
	}))
	defer srv.Close()
	c, err := newClient(Config{Name: "A7", URL: srv.URL, AllowLocal: true, GapMS: 1})
	if err != nil {
		t.Fatal(err)
	}
	a := newAddic7ed(c)
	started := time.Now()
	if _, err := a.page(context.Background(), "/start", nil, ""); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 3 || a.calls != 3 || time.Since(started) < 2*addic7edMinGap {
		t.Fatalf("requests %d, calls %d, elapsed %v", requests.Load(), a.calls, time.Since(started))
	}
}

func TestAddic7edUsesDefaultGap(t *testing.T) {
	c, err := newClient(Config{Name: "A7", URL: "https://example.com", GapMS: 1})
	if err != nil {
		t.Fatal(err)
	}
	newAddic7ed(c)
	if got := time.Duration(c.cfg.GapMS) * time.Millisecond; got != addic7edMinGap {
		t.Fatalf("gap %v, want %v", got, addic7edMinGap)
	}
}

func TestAddic7edRedirectDailyCapKeepsRateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/next", http.StatusFound)
	}))
	defer srv.Close()
	c, err := newClient(Config{Name: "A7", URL: srv.URL, AllowLocal: true, GapMS: 1})
	if err != nil {
		t.Fatal(err)
	}
	a := newAddic7ed(c)
	a.day = time.Now().UTC().Format("2006-01-02")
	a.calls = addic7edDailyCap - 1
	_, err = a.page(context.Background(), "/start", nil, "")
	if kindOf(err) != KindRateLimited || a.calls != addic7edDailyCap {
		t.Fatalf("err %v, calls %d", err, a.calls)
	}
}

func TestAddic7edRedirectWaitKeepsContextError(t *testing.T) {
	old := addic7edMinGap
	addic7edMinGap = time.Second
	t.Cleanup(func() { addic7edMinGap = old })
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/next", http.StatusFound)
	}))
	defer srv.Close()
	c, err := newClient(Config{Name: "A7", URL: srv.URL, AllowLocal: true, GapMS: 1})
	if err != nil {
		t.Fatal(err)
	}
	a := newAddic7ed(c)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err = a.page(ctx, "/start", nil, "")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err %v", err)
	}
}

func TestAddic7edRedirectValidationAndCapRemain(t *testing.T) {
	var offHostHits atomic.Int32
	offHost := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { offHostHits.Add(1) }))
	defer offHost.Close()
	for _, tc := range []struct {
		name string
		next func(*http.Request) string
		want int32
	}{
		{name: "off host", next: func(*http.Request) string { return offHost.URL }, want: 1},
		{name: "redirect cap", next: func(r *http.Request) string { return r.URL.Path }, want: 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var hits atomic.Int32
			var srv *httptest.Server
			srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits.Add(1)
				http.Redirect(w, r, tc.next(r), http.StatusFound)
			}))
			defer srv.Close()
			c, err := newClient(Config{Name: "A7", URL: srv.URL, AllowLocal: true, GapMS: 1})
			if err != nil {
				t.Fatal(err)
			}
			a := newAddic7ed(c)
			if _, err := a.page(context.Background(), "/start", nil, ""); err == nil {
				t.Fatal("redirect accepted")
			}
			if hits.Load() != tc.want {
				t.Fatalf("hits %d, want %d", hits.Load(), tc.want)
			}
		})
	}
	if offHostHits.Load() != 0 {
		t.Fatalf("off-host server reached %d times", offHostHits.Load())
	}
}

func TestAddic7edChallengeRests(t *testing.T) {
	f := a7Fake()
	f.Challenge = true
	s, _ := a7Site(t, f, fakeUser, fakePass)
	_, err := s.Search(context.Background(), a7Query)
	if kindOf(err) != KindBlocked || !strings.Contains(err.Error(), "anti-bot") {
		t.Fatalf("err %v", err)
	}
	if f.Requests != 1 {
		t.Fatalf("tried again: %d requests", f.Requests)
	}
}

func TestAddic7edLinksPinned(t *testing.T) {
	f := a7Fake()
	f.LinkHost = "https://elsewhere.example"
	s, _ := a7Site(t, f, fakeUser, fakePass)
	got, err := s.Search(context.Background(), a7Query)
	if err != nil || len(got) != 0 {
		t.Fatalf("off-host link kept: %+v %v", got, err)
	}
	// A candidate that points elsewhere is refused at download time too.
	for _, link := range []string{"https://elsewhere.example/download/301\n/show/12", "//elsewhere.example/x\n/show/12", "/download/301\nhttps://evil.example/show/1"} {
		if _, err := s.Download(context.Background(), Candidate{Key: "301", Link: link}); err == nil {
			t.Fatalf("followed %q", link)
		}
	}
}

func TestAddic7edHostilePages(t *testing.T) {
	row := `<tr><td>1</td><td>2</td><td>t</td><td>English</td><td>WEB</td><td>Completed</td><td></td><td><a href="/download/301">D</a></td></tr>`
	for name, body := range map[string]string{
		"huge":    strings.Repeat("<div>", 300000) + row,
		"nested":  strings.Repeat("<table><tr><td>", 50000) + row,
		"script":  `<script>document.write('` + row + `')</script><!-- ` + row + ` -->` + row,
		"unicode": "<tr><td>1</td><td>2</td><td>‮\u0000\xff\xfe</td><td>English</td><td>WEB ☃ \U0001F600</td><td>Completed</td><td></td><td><a href=\"/download/301\">D</a></td></tr>",
		"broken":  `<tr><td>1<td>2<td>t<td>English<td>WEB<td>Completed<td><td><a href=/download/301>D` + `<a href='`,
		"toolong": strings.Repeat("a", 5<<20),
	} {
		f := a7Fake()
		f.Hostile = body
		s, _ := a7Site(t, f, fakeUser, fakePass)
		done := make(chan struct{})
		var got []Candidate
		var err error
		go func() { got, err = s.Search(context.Background(), a7Query); close(done) }()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Fatalf("%s: too slow", name)
		}
		switch name {
		case "toolong":
			if kindOf(err) != KindUnreadable {
				t.Errorf("%s: %v", name, err)
			}
		case "script":
			if err != nil || len(got) != 1 {
				t.Errorf("%s: script or comment rows counted: %d %v", name, len(got), err)
			}
		default:
			if err != nil || len(got) > 1 {
				t.Errorf("%s: %d %v", name, len(got), err)
			}
		}
	}
}

func TestAddic7edListed(t *testing.T) {
	k, ok := KindOf("addic7ed")
	if !ok || k.Later || k.Movies || !k.Shows || !k.NeedsAccount || k.DefaultURL == "" {
		t.Fatalf("%+v", k)
	}
}

// The lab serves the fake under a path, and links and cookies follow it.
func TestAddic7edUnderAPath(t *testing.T) {
	old := addic7edMinGap
	addic7edMinGap = time.Millisecond
	defer func() { addic7edMinGap = old }()
	f := a7Fake()
	f.Base = "/addic7ed"
	mux := http.NewServeMux()
	mux.Handle("/addic7ed/", http.StripPrefix("/addic7ed", f))
	srv := httptest.NewServer(mux)
	defer srv.Close()
	s, err := New("addic7ed", Config{Name: "A7", URL: srv.URL + "/addic7ed", Username: fakeUser, Password: fakePass, GapMS: 1, AllowLocal: true})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Search(context.Background(), a7Query)
	if err != nil || len(got) != 1 {
		t.Fatalf("%+v %v", got, err)
	}
	if b, err := s.Download(context.Background(), got[0]); err != nil || !strings.Contains(string(b), "made up") {
		t.Fatalf("%q %v", b, err)
	}
}
