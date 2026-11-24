package sites

import (
	"archive/zip"
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"golang.org/x/text/encoding/charmap"
)

// n24Fake stands in for the CheckSubAgent endpoint. files maps "hash size" to
// the subtitle text it zips; status, when set, replaces every answer's status.
type n24Fake struct {
	mu     sync.Mutex
	files  map[string]string
	status string
	asked  []string
}

func (f *n24Fake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.URL.Path != "/run/CheckSubAgent.php" || r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	_ = r.ParseForm()
	p := r.PostForm
	f.asked = append(f.asked, strings.Join([]string{p.Get("postAction"), p.Get("ua"), p.Get("ap"), p.Get("fh"), p.Get("fs"), p.Get("fn"), p.Get("n24pref")}, " "))
	if f.status != "" {
		_, _ = w.Write([]byte(f.status))
		return
	}
	text, ok := f.files[p.Get("fh")+" "+p.Get("fs")]
	if !ok {
		_, _ = w.Write([]byte("OK-0||"))
		return
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	zf, _ := zw.Create("napis.srt")
	_, _ = zf.Write([]byte(text))
	_ = zw.Close()
	_, _ = w.Write(append([]byte("OK-2|res:1|fps:23.976|imdb:123456|napisId:42||"), buf.Bytes()...))
}

func TestNapisy24SearchAndDownload(t *testing.T) {
	srt, _ := charmap.Windows1250.NewEncoder().String("1\r\n00:00:01,000 --> 00:00:02,000\r\nŻółw śpi\r\n")
	f := &n24Fake{files: map[string]string{
		"00000000000000ff 1234567": srt,
		"00000000000000aa 7654321": "[10][20]Część|/druga\n",
	}}
	s := n24Site(t, f)
	ctx := context.Background()
	q := Query{Kind: "episode", Title: "Quiet Harbour", Season: 1, Episode: 2, ReleaseName: "Quiet.Harbour.S01E02.WEB", Hash: "00000000000000FF", Size: 1234567,
		File: "/media/Quiet.Harbour.S01E02.WEB.mkv", Languages: []string{"en", "pl"}}
	got, err := s.Search(ctx, q)
	if err != nil || len(got) != 1 {
		t.Fatalf("%+v %v", got, err)
	}
	c := got[0]
	if c.Key != "00000000000000ff-1234567" || c.Language != "pl" || !c.MatchedByID || c.Season != 1 || c.Episode != 2 || c.Format != "srt" ||
		len(c.ReleaseNames) != 1 || c.ReleaseNames[0] != "Quiet.Harbour.S01E02.WEB" {
		t.Fatalf("candidate %+v", c)
	}
	if want := "CheckSub " + fakeUser + " " + fakePass + " 00000000000000ff 1234567 Quiet.Harbour.S01E02.WEB.mkv 1"; f.asked[0] != want {
		t.Fatalf("asked %q", f.asked[0])
	}
	b, err := s.Download(ctx, c)
	if err != nil || !strings.Contains(string(b), "00:00:01,000 --> 00:00:02,000\nŻółw śpi") {
		t.Fatalf("download %q %v", b, err)
	}
	// An old tenth-of-a-second file comes back as SRT.
	q2 := q
	q2.Hash, q2.Size = "00000000000000aa", 7654321
	got, err = s.Search(ctx, q2)
	if err != nil || len(got) != 1 {
		t.Fatalf("mpl2 %+v %v", got, err)
	}
	if b, err := s.Download(ctx, got[0]); err != nil || !strings.Contains(string(b), "00:00:01,000 --> 00:00:02,000\nCzęść\n<i>druga</i>") {
		t.Fatalf("mpl2 download %q %v", b, err)
	}

	// No Polish asked, no hash or no size: nothing, and no call.
	n := len(f.asked)
	for _, bad := range []Query{
		{Kind: "movie", Hash: "00000000000000ff", Size: 1234567, Languages: []string{"en"}},
		{Kind: "movie", Size: 1234567, Languages: []string{"pl"}},
		{Kind: "movie", Hash: "00000000000000ff", Languages: []string{"pl"}},
		{Kind: "movie", Hash: "not-a-hash", Size: 1, Languages: []string{"pl"}},
	} {
		if got, err := s.Search(ctx, bad); err != nil || len(got) != 0 {
			t.Fatalf("%+v: %+v %v", bad, got, err)
		}
	}
	if len(f.asked) != n {
		t.Fatalf("calls made without need: %v", f.asked[n:])
	}
	// A key that is not a hash and size is refused before any call.
	if _, err := s.Download(ctx, Candidate{Key: "../x"}); kindOf(err) != KindUnreadable || len(f.asked) != n {
		t.Fatalf("bad key: %v", err)
	}
	// Gone from the site.
	delete(f.files, "00000000000000ff 1234567")
	if got, err := s.Search(ctx, q); err != nil || len(got) != 0 {
		t.Fatalf("not found: %+v %v", got, err)
	}
	if _, err := s.Download(ctx, c); kindOf(err) != KindUnreadable {
		t.Fatalf("gone: %v", err)
	}
}

func TestNapisy24Statuses(t *testing.T) {
	ctx := context.Background()
	q := Query{Kind: "movie", Hash: "00000000000000ff", Size: 99, Languages: []string{"pl"}}
	for status, want := range map[string]string{
		"OK-0||":                    "",
		"OK-1|imdb:1|ftitle:Film||": "",
		"OK-3|napisId:9||PK":        "",
		"login error":               KindLoginRefused,
		"<html>oops</html>":         KindUnreadable,
		"OK-2|napisId:1||":          KindUnreadable,
		"OK-2|napisId:1||not a zip": "",
	} {
		s := n24Site(t, &n24Fake{status: status})
		got, err := s.Search(ctx, q)
		if kindOf(err) != want || len(got) != 0 {
			t.Errorf("%q: %+v %v", status, got, err)
		}
	}
}

func n24Site(t *testing.T, h http.Handler) Site {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := cfg(srv.URL)
	c.Username, c.Password = fakeUser, fakePass
	s, err := New("napisy24", c)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// Without the owner's own account the site is never asked: HolyArrs ships no
// shared login.
func TestNapisy24NeedsOwnAccount(t *testing.T) {
	f := &n24Fake{}
	s := fakeSite(t, "napisy24", f)
	got, err := s.Search(context.Background(), Query{Kind: "movie", Title: "Film", Hash: "00000000000000ff", Size: 10, File: "/m/Film.mkv", Languages: []string{"pl"}})
	if kindOf(err) != KindLoginRefused || len(got) != 0 || len(f.asked) != 0 {
		t.Fatalf("%+v %v asked=%d", got, err, len(f.asked))
	}
}
