package sites

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"golang.org/x/text/encoding/charmap"
)

func TestNapiToken(t *testing.T) {
	// Values from the reference client's check-value function.
	for hash, want := range map[string]string{
		"0123456789abcdef0123456789abcdef": "e2308",
		"d41d8cd98f00b204e9800998ecf8427e": "8030b",
		"ffffffffffffffffffffffffffffffff": "eebcd",
	} {
		if got := napiToken(hash); got != want {
			t.Errorf("napiToken(%s) = %s, want %s", hash, got, want)
		}
	}
}

func TestNapiHash(t *testing.T) {
	dir := t.TempDir()
	small := filepath.Join(dir, "small.mkv")
	if err := os.WriteFile(small, make([]byte, 1000), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NapiHash(small); err == nil {
		t.Fatal("a tiny file must not hash")
	}
	// Only the first 10 MiB count: two files that differ after that hash alike.
	a, b := filepath.Join(dir, "a.mkv"), filepath.Join(dir, "b.mkv")
	data := make([]byte, napiHashBytes+100)
	if err := os.WriteFile(a, data, 0o600); err != nil {
		t.Fatal(err)
	}
	data[napiHashBytes+50] = 1
	if err := os.WriteFile(b, data, 0o600); err != nil {
		t.Fatal(err)
	}
	ha, err1 := NapiHash(a)
	hb, err2 := NapiHash(b)
	if err1 != nil || err2 != nil || ha != hb || !hex32.MatchString(ha) {
		t.Fatalf("%s %s %v %v", ha, hb, err1, err2)
	}
}

func TestNapiConvert(t *testing.T) {
	cp1250, _ := charmap.Windows1250.NewEncoder().String("[10][35]Zażółć gęślą jaźń|/druga linia\n[40][]Bez końca\n")
	for _, tc := range []struct {
		name, in, format string
		want             []string
	}{
		{"mpl2 in windows-1250", cp1250, "srt", []string{"00:00:01,000 --> 00:00:03,500\nZażółć gęślą jaźń\n<i>druga linia</i>", "00:00:04,000 --> 00:00:09,000\nBez końca"}},
		{"microdvd with rate", "{1}{1}25\r\n{25}{50}Raz\r\n{75}{100}Dwa\r\n", "srt", []string{"00:00:01,000 --> 00:00:02,000\nRaz", "00:00:03,000 --> 00:00:04,000\nDwa"}},
		{"microdvd without rate", "{25}{50}Raz\n", "sub", []string{"{25}{50}Raz"}},
		{"tmplayer", "00:00:01:Raz\n00:00:03:Dwa\n", "srt", []string{"00:00:01,000 --> 00:00:03,000\nRaz", "00:00:03,000 --> 00:00:08,000\nDwa"}},
		{"srt", "1\n00:00:01,000 --> 00:00:02,000\nRaz\n", "srt", []string{"Raz"}},
	} {
		out, format, ok := napiConvert([]byte(tc.in))
		if !ok || format != tc.format {
			t.Errorf("%s: ok=%v format=%s", tc.name, ok, format)
			continue
		}
		for _, w := range tc.want {
			if !strings.Contains(string(out), w) {
				t.Errorf("%s: %q lacks %q", tc.name, out, w)
			}
		}
	}
	if _, _, ok := napiConvert([]byte("<html>nope</html>")); ok {
		t.Error("an HTML page must not read as a subtitle")
	}
}

type napiFake struct {
	mu    sync.Mutex
	files map[string]string // hash to body
	asked []string
}

func (f *napiFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	q := r.URL.Query()
	f.asked = append(f.asked, r.URL.Path+" "+q.Get("l")+" "+q.Get("f")+" "+q.Get("t"))
	if body, ok := f.files[q.Get("f")]; ok {
		_, _ = w.Write([]byte(body))
		return
	}
	_, _ = w.Write([]byte(napiNotFound))
}

func TestNapiProjektSearchAndDownload(t *testing.T) {
	video := filepath.Join(t.TempDir(), "Film.2020.1080p.mkv")
	if err := os.WriteFile(video, make([]byte, 2<<20), 0o600); err != nil {
		t.Fatal(err)
	}
	hash, _ := NapiHash(video)
	f := &napiFake{files: map[string]string{hash: "[10][20]Cześć\n"}}
	s := fakeSite(t, "napiprojekt", f)
	ctx := context.Background()
	q := Query{Kind: "movie", Title: "Film", Year: 2020, ReleaseName: "Film.2020.1080p", File: video, Languages: []string{"en", "pl"}}
	got, err := s.Search(ctx, q)
	if err != nil || len(got) != 1 {
		t.Fatalf("%+v %v", got, err)
	}
	c := got[0]
	if c.Key != hash || c.Language != "pl" || !c.MatchedByID || c.Format != "srt" || len(c.ReleaseNames) != 1 {
		t.Fatalf("candidate %+v", c)
	}
	if want := "/unit_napisy/dl.php PL " + hash + " " + napiToken(hash); f.asked[0] != want {
		t.Fatalf("asked %q, want %q", f.asked[0], want)
	}
	b, err := s.Download(ctx, c)
	if err != nil || !strings.Contains(string(b), "00:00:01,000 --> 00:00:02,000\nCześć") {
		t.Fatalf("download %q %v", b, err)
	}

	// No Polish asked, no file, or not on the site: nothing, and no call for the first two.
	n := len(f.asked)
	for _, q2 := range []Query{
		{Kind: "movie", File: video, Languages: []string{"en"}},
		{Kind: "movie", Languages: []string{"pl"}},
	} {
		if got, err := s.Search(ctx, q2); err != nil || len(got) != 0 {
			t.Fatalf("%+v: %+v %v", q2, got, err)
		}
	}
	if len(f.asked) != n {
		t.Fatalf("calls made without need: %v", f.asked[n:])
	}
	delete(f.files, hash)
	if got, err := s.Search(ctx, q); err != nil || len(got) != 0 {
		t.Fatalf("not found: %+v %v", got, err)
	}
	if _, err := s.Download(ctx, c); kindOf(err) != KindUnreadable {
		t.Fatalf("gone: %v", err)
	}
}
