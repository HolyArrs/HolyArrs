package sites

import (
	"bytes"
	"context"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/sites/fakesite"
)

// Made-up account details. None of them is a real account.
const (
	fakeKey  = "made-up-key-for-tests"
	fakeUser = "lab-person"
	fakePass = "made-up-word-xyz"
)

func osFake() *fakesite.OpenSubs {
	return &fakesite.OpenSubs{Key: fakeKey, User: fakeUser, Pass: fakePass, Quota: 2, Subs: []fakesite.Sub{
		{ID: "101", Kind: "movie", Language: "en", Title: "The Lantern", Year: 2010, Release: "The.Lantern.2010.1080p.BluRay.x264-GRP", ByID: true, Trusted: true, Uploader: "anna", Text: fakesite.Cue("made up", 1000)},
		{ID: "102", Kind: "movie", Language: "pt-BR", Title: "The Lantern", Year: 2010, Release: "The.Lantern.2010.720p", Text: fakesite.Cue("inventado", 1000)},
		{ID: "201", Kind: "episode", Language: "en", Title: "Quiet Harbour", Season: 1, Episode: 2, Release: "Quiet.Harbour.S01E02.WEB", ByID: true, HearingImpaired: true, Text: fakesite.Cue("made up", 1000)},
	}}
}

func osSite(t *testing.T, f *fakesite.OpenSubs, user, pass string) (Site, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	s, err := New("opensubtitles", Config{Name: "OS", URL: srv.URL + "/api/v1", Key: fakeKey, Username: user, Password: pass, GapMS: 1, AllowLocal: true})
	if err != nil {
		t.Fatal(err)
	}
	return s, srv
}

func kindOf(err error) string {
	var se *Error
	if errors.As(err, &se) {
		return se.Kind
	}
	return ""
}

func TestOpenSubtitlesSearchAndDownload(t *testing.T) {
	f := osFake()
	s, _ := osSite(t, f, fakeUser, fakePass)
	ctx := context.Background()
	got, err := s.Search(ctx, Query{Kind: "movie", Title: "The Lantern", Year: 2010, IMDb: "tt0000101", Languages: []string{"en", "pt"}, Hash: "00000000000000ff"})
	if err != nil || len(got) != 2 {
		t.Fatalf("%+v %v", got, err)
	}
	if got[0].Key != "101" || !got[0].MatchedByID || !got[0].Trusted || got[0].ReleaseNames[0] != "The.Lantern.2010.1080p.BluRay.x264-GRP" {
		t.Errorf("first %+v", got[0])
	}
	if got[1].Language != "pt-br" {
		t.Errorf("Brazilian file for a pt wish: %+v", got[1])
	}
	if f.Logins != 0 {
		t.Error("search signed in")
	}
	b, err := s.Download(ctx, got[0])
	if err != nil || !strings.Contains(string(b), "made up") || f.Logins != 1 {
		t.Fatalf("download %q %v logins %d", b, err, f.Logins)
	}
	ep, err := s.Search(ctx, Query{Kind: "episode", Title: "Quiet Harbour", Season: 1, Episode: 2, IMDb: "tt0555", Languages: []string{"en"}})
	if err != nil || len(ep) != 1 || !ep[0].HearingImpaired || ep[0].Season != 1 || ep[0].Episode != 2 || ep[0].Title != "Quiet Harbour" || !ep[0].MatchedByID {
		t.Fatalf("episode %+v %v", ep, err)
	}
}

// A refused token is replaced by one new sign-in. A refused sign-in is a
// login_refused error and the site rests 12 hours. Without account details
// there is no download.
func TestOpenSubtitlesSignIn(t *testing.T) {
	f := osFake()
	s, _ := osSite(t, f, fakeUser, fakePass)
	ctx := context.Background()
	c := Candidate{Key: "101", Link: "101"}
	if _, err := s.Download(ctx, c); err != nil {
		t.Fatal(err)
	}
	f.Expire = true
	if _, err := s.Download(ctx, c); err != nil || f.Logins != 2 {
		t.Fatalf("refresh: %v logins %d", err, f.Logins)
	}
	bad, _ := osSite(t, osFake(), fakeUser, "wrong-made-up")
	_, err := bad.Download(ctx, c)
	if kindOf(err) != KindLoginRefused || strings.Contains(err.Error(), "wrong-made-up") {
		t.Fatalf("bad password: %v", err)
	}
	if k, d := PauseFor(err, 1, time.Now()); k != KindLoginRefused || d != 12*time.Hour {
		t.Errorf("pause %s %v", k, d)
	}
	none, _ := osSite(t, osFake(), "", "")
	if _, err := none.Download(ctx, c); kindOf(err) != KindLoginRefused {
		t.Fatalf("no account: %v", err)
	}
	// A wrong API key is refused on search as well.
	f2 := osFake()
	srv := httptest.NewServer(f2)
	t.Cleanup(srv.Close)
	wk, _ := New("opensubtitles", Config{Name: "OS", URL: srv.URL, Key: "other-made-up", GapMS: 1, AllowLocal: true})
	if _, err := wk.Search(ctx, Query{Kind: "movie", Title: "x", Languages: []string{"en"}}); kindOf(err) != KindLoginRefused {
		t.Fatalf("wrong key: %v", err)
	}
}

// When the daily download count runs out, the site rests until the reset
// time it gave.
func TestOpenSubtitlesQuota(t *testing.T) {
	f := osFake()
	f.Quota = 0
	s, _ := osSite(t, f, fakeUser, fakePass)
	_, err := s.Download(context.Background(), Candidate{Key: "101", Link: "101"})
	var se *Error
	if !errors.As(err, &se) || se.Kind != KindDownloadLimit || se.RetryAfter < 2*time.Hour || se.RetryAfter > 3*time.Hour {
		t.Fatalf("quota: %+v", err)
	}
	if k, d := PauseFor(err, 1, time.Now()); k != KindDownloadLimit || d != se.RetryAfter {
		t.Errorf("pause %s %v", k, d)
	}
}

func TestOpenSubtitlesHostile(t *testing.T) {
	ctx := context.Background()
	q := Query{Kind: "movie", Title: "The Lantern", Languages: []string{"en"}}
	for _, body := range []string{
		`not json`,
		`{"data": [{"attributes": {"language": "en", "files": []}}, {"attributes": {"language": "en", "files": [{"file_id": -4}]}}, {"attributes": {"language": "de", "files": [{"file_id": 9}]}}, {"attributes": {"language": "en", "machine_translated": true, "files": [{"file_id": 9}]}}]}`,
		`{"data": "x"}`,
	} {
		f := osFake()
		f.Hostile = body
		s, _ := osSite(t, f, fakeUser, fakePass)
		got, err := s.Search(ctx, q)
		// A machine-made file may come back, but only flagged as such.
		if len(got) > 1 || (len(got) == 1 && got[0].MachineMade == "") || (err != nil && kindOf(err) != KindUnreadable) {
			t.Errorf("%s: %+v %v", body, got, err)
		}
	}
	// A download link on another host must use https.
	f := osFake()
	f.LinkHost = "http://192.0.2.1"
	s, _ := osSite(t, f, fakeUser, fakePass)
	if _, err := s.Download(ctx, Candidate{Link: "101"}); kindOf(err) != KindUnreadable {
		t.Fatalf("plain http link off the site: %v", err)
	}
	if _, err := s.Download(ctx, Candidate{Link: "../etc"}); kindOf(err) != KindUnreadable {
		t.Fatalf("bad file id: %v", err)
	}
}

func subdlFake() *fakesite.Subdl {
	return &fakesite.Subdl{Key: fakeKey, Quota: 1, Subs: []fakesite.Sub{
		{ID: "3197-3290", Kind: "movie", Language: "en", Title: "The Lantern", Year: 2010, Release: "The.Lantern.2010.1080p.BluRay.x264-GRP", Uploader: "anna", Text: fakesite.Cue("made up", 1000)},
		{ID: "3197-3291", Kind: "episode", Language: "en", Title: "Quiet Harbour", Season: 1, Episode: 2, Release: "Quiet.Harbour.S01E02.WEB", Text: fakesite.Cue("made up", 1000)},
		{ID: "3197-3292", Kind: "episode", Language: "en", Title: "Quiet Harbour", Season: 1, Episode: 3, Release: "Quiet.Harbour.S01E03.WEB", Text: fakesite.Cue("other", 1000)},
	}}
}

func TestSubdl(t *testing.T) {
	f := subdlFake()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	ctx := context.Background()
	s, _ := New("subdl", Config{Name: "subdl", URL: srv.URL + "/api/v1", Key: fakeKey, GapMS: 1, AllowLocal: true})
	got, err := s.Search(ctx, Query{Kind: "movie", Title: "The Lantern", Year: 2010, Languages: []string{"en"}})
	if err != nil || len(got) != 1 || got[0].MatchedByID || got[0].Key != "3197-3290" || got[0].Year != 2010 {
		t.Fatalf("%+v %v", got, err)
	}
	b, err := s.Download(ctx, got[0])
	if err != nil || !bytes.HasPrefix(b, []byte("PK")) {
		t.Fatalf("zip %v", err)
	}
	// With the quota used up, the download limit applies until midnight UTC.
	if _, err := s.Download(ctx, got[0]); kindOf(err) != KindRateLimited && kindOf(err) != KindDownloadLimit {
		t.Fatalf("quota: %v", err)
	}
	ep, err := s.Search(ctx, Query{Kind: "episode", Title: "Quiet Harbour", Season: 1, Episode: 2, TMDB: "77", Languages: []string{"en"}})
	if err != nil || len(ep) != 1 || ep[0].Episode != 2 || !ep[0].MatchedByID {
		t.Fatalf("episode %+v %v", ep, err)
	}
	// Nothing found is an empty result, not an error.
	if got, err := s.Search(ctx, Query{Kind: "movie", Title: "Nope", Languages: []string{"en"}}); err != nil || len(got) != 0 {
		t.Fatalf("none %+v %v", got, err)
	}
	// A wrong key is login_refused, and the key never appears in the error.
	w, _ := New("subdl", Config{Name: "subdl", URL: srv.URL + "/api/v1", Key: "other-made-up-key", GapMS: 1, AllowLocal: true})
	_, err = w.Search(ctx, Query{Kind: "movie", Title: "The Lantern", Languages: []string{"en"}})
	if kindOf(err) != KindLoginRefused || strings.Contains(err.Error(), "other-made-up-key") {
		t.Fatalf("wrong key: %v", err)
	}
	// Hostile answers and links.
	f.Hostile = `{"status": true, "subtitles": [{"url": "https://elsewhere.example/x.zip", "language": "EN"}, {"url": "//elsewhere.example/x.zip", "language": "EN"}, {"url": "/ok.zip", "language": "DE"}]}`
	if got, err := s.Search(ctx, Query{Kind: "movie", Title: "The Lantern", Languages: []string{"en"}}); err != nil || len(got) != 0 {
		t.Fatalf("hostile %+v %v", got, err)
	}
	if _, err := s.Download(ctx, Candidate{Link: "//elsewhere.example/x.zip"}); kindOf(err) != KindUnreadable {
		t.Fatalf("hostile link: %v", err)
	}
	f.Hostile = "{"
	if _, err := s.Search(ctx, Query{Kind: "movie", Title: "The Lantern", Languages: []string{"en"}}); kindOf(err) != KindUnreadable {
		t.Fatalf("bad json: %v", err)
	}
}

func TestFileHash(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "v.mkv")
	if err := os.WriteFile(p, make([]byte, 200<<10), 0o600); err != nil {
		t.Fatal(err)
	}
	h, err := FileHash(p)
	if err != nil || h != "0000000000032000" { // all zeros, so the hash equals the size
		t.Fatalf("%s %v", h, err)
	}
	small := filepath.Join(dir, "s.mkv")
	_ = os.WriteFile(small, []byte("x"), 0o600)
	if _, err := FileHash(small); err == nil {
		t.Error("small file hashed")
	}
}
