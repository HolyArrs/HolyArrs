package migrate

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func zipWith(t *testing.T, add func(zw *zip.Writer)) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "backup.zip")
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add(zw)
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func entry(t *testing.T, zw *zip.Writer, fh *zip.FileHeader, data []byte) {
	t.Helper()
	if fh.Method == 0 {
		fh.Method = zip.Deflate
	}
	w, err := zw.CreateHeader(fh)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write(data)
}

// TestHostileZips: entries that leave the archive, links, too many entries,
// bombs and oversized files refuse the whole backup.
func TestHostileZips(t *testing.T) {
	db := dbBytes(t, radarrDB()...)
	cases := map[string]struct {
		add  func(zw *zip.Writer)
		lim  Limits
		want error
	}{
		"slip": {add: func(zw *zip.Writer) {
			entry(t, zw, &zip.FileHeader{Name: "radarr.db"}, db)
			entry(t, zw, &zip.FileHeader{Name: "../../evil.txt"}, []byte("x"))
		}, want: ErrUnsafeBackup},
		"absolute": {add: func(zw *zip.Writer) { entry(t, zw, &zip.FileHeader{Name: "/etc/radarr.db"}, db) }, want: ErrUnsafeBackup},
		"drive":    {add: func(zw *zip.Writer) { entry(t, zw, &zip.FileHeader{Name: `C:\radarr.db`}, db) }, want: ErrUnsafeBackup},
		"link": {add: func(zw *zip.Writer) {
			fh := &zip.FileHeader{Name: "radarr.db"}
			fh.SetMode(fs.ModeSymlink | 0o777)
			entry(t, zw, fh, []byte("/etc/passwd"))
		}, want: ErrUnsafeBackup},
		"too many entries": {add: func(zw *zip.Writer) {
			for i := 0; i < 6; i++ {
				entry(t, zw, &zip.FileHeader{Name: "junk" + string(rune('a'+i))}, []byte("x"))
			}
		}, lim: Limits{MaxEntries: 5}, want: ErrUnsafeBackup},
		"bomb": {add: func(zw *zip.Writer) {
			entry(t, zw, &zip.FileHeader{Name: "radarr.db"}, make([]byte, 8<<20))
		}, lim: Limits{MaxRatio: 100}, want: ErrUnsafeBackup},
		"too big": {add: func(zw *zip.Writer) { entry(t, zw, &zip.FileHeader{Name: "radarr.db"}, db) }, lim: Limits{MaxFile: 4096}, want: ErrTooBig},
		"no database": {add: func(zw *zip.Writer) {
			entry(t, zw, &zip.FileHeader{Name: "notes.txt"}, []byte("hello"))
		}, want: ErrNotBackup},
		"not sqlite": {add: func(zw *zip.Writer) {
			entry(t, zw, &zip.FileHeader{Name: "radarr.db"}, []byte("CREATE TABLE Movies (Id INTEGER); -- a SQL text, not a database"))
		}, want: ErrNotSQLite},
		"sqlite head then junk": {add: func(zw *zip.Writer) {
			entry(t, zw, &zip.FileHeader{Name: "radarr.db"}, append([]byte("SQLite format 3\x00"), bytes.Repeat([]byte{0xff}, 4096)...))
		}, want: ErrNotSQLite},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			p := zipWith(t, c.add)
			src := file(Radarr, p)
			src.Limits = c.lim
			if _, err := Read(context.Background(), src); !errors.Is(err, c.want) {
				t.Fatalf("got %v, want %v", err, c.want)
			}
			if _, err := Probe(context.Background(), src); !errors.Is(err, c.want) {
				t.Fatalf("probe got %v, want %v", err, c.want)
			}
		})
	}
	// Nothing is left behind in the temporary folder.
	left, _ := filepath.Glob(filepath.Join(os.TempDir(), "holyarrs-move-*"))
	for _, l := range left {
		if fi, err := os.Stat(l); err == nil && time.Since(fi.ModTime()) < time.Minute {
			t.Errorf("temporary folder left behind: %s", l)
		}
	}
}

// TestHostileFiles: a huge database, a SQL text named like a database and
// links on disk are refused.
func TestHostileFiles(t *testing.T) {
	dir := t.TempDir()
	big := filepath.Join(dir, "radarr.db")
	makeDB(t, big, radarrDB()...)
	if _, err := Read(context.Background(), Source{Kind: Radarr, Path: big, Limits: Limits{MaxFile: 1024}}); !errors.Is(err, ErrTooBig) {
		t.Fatalf("huge db: %v", err)
	}
	sqlText := filepath.Join(t.TempDir(), "radarr.db")
	_ = os.WriteFile(sqlText, []byte("CREATE TABLE Movies (Id INTEGER);"), 0o600)
	if _, err := Read(context.Background(), file(Radarr, sqlText)); !errors.Is(err, ErrNotSQLite) {
		t.Fatalf("sql text: %v", err)
	}
	if _, err := Read(context.Background(), file(Radarr, filepath.Join(dir, "missing.zip"))); !errors.Is(err, ErrNotBackup) {
		t.Fatalf("missing: %v", err)
	}
	link := filepath.Join(t.TempDir(), "radarr.db")
	if err := os.Symlink(big, link); err != nil {
		t.Logf("no symlinks here (%v); link checks skipped", err)
		return
	}
	if _, err := Read(context.Background(), file(Radarr, link)); !errors.Is(err, ErrUnsafeBackup) {
		t.Fatalf("linked file: %v", err)
	}
	if _, err := Read(context.Background(), file(Radarr, filepath.Dir(link))); !errors.Is(err, ErrUnsafeBackup) {
		t.Fatalf("folder holding a link: %v", err)
	}
}

// TestHostileAnswers: JSON bombs, huge answers and redirects off the host.
func TestHostileAnswers(t *testing.T) {
	status := map[string]any{"appName": "Radarr", "version": "6"}
	deep := strings.Repeat("[", 200) + strings.Repeat("]", 200)
	srv := fakeAPI(t, "X-Api-Key", "k", map[string]any{"/api/v3/system/status": status, "/api/v3/movie": deep})
	if _, err := Read(context.Background(), api(Radarr, srv.URL, "k")); !errors.Is(err, ErrBadAnswer) {
		t.Fatalf("deep json: %v", err)
	}
	huge := "[" + strings.Repeat(`{"id":1},`, 2000) + `{"id":1}]`
	srv = fakeAPI(t, "X-Api-Key", "k", map[string]any{"/api/v3/system/status": status, "/api/v3/movie": huge})
	src := api(Radarr, srv.URL, "k")
	src.Limits = Limits{MaxAnswer: 4096}
	if _, err := Read(context.Background(), src); !errors.Is(err, ErrTooBig) {
		t.Fatalf("huge answer: %v", err)
	}
	src.Limits = Limits{MaxRows: 10}
	if _, err := Read(context.Background(), src); !errors.Is(err, ErrTooBig) {
		t.Fatalf("too many rows: %v", err)
	}
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"appName":"Radarr","version":"6"}`))
	}))
	defer other.Close()
	away := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, strings.Replace(other.URL, "127.0.0.1", "localhost", 1)+r.URL.Path, http.StatusFound)
	}))
	defer away.Close()
	_, err := Probe(context.Background(), api(Radarr, away.URL, "k"))
	if !errors.Is(err, ErrUnreachable) || !strings.Contains(err.Error(), "another address") {
		t.Fatalf("redirect: %v", err)
	}
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(2 * time.Second):
		case <-r.Context().Done():
		}
	}))
	defer slow.Close()
	src = api(Radarr, slow.URL, "k")
	src.Limits = Limits{Timeout: 100 * time.Millisecond}
	if _, err := Probe(context.Background(), src); !errors.Is(err, ErrUnreachable) || !strings.Contains(err.Error(), "too long") {
		t.Fatalf("slow: %v", err)
	}
}

// TestNetworkPolicy: without the per-read local switch only public
// addresses are dialled; link-local and cloud metadata addresses are
// refused even with it; addresses with a user or query are refused.
func TestNetworkPolicy(t *testing.T) {
	u, key := radarrAPI(t)
	src := api(Radarr, u, key)
	src.AllowLocal = false
	_, err := Probe(context.Background(), src)
	if !errors.Is(err, ErrUnreachable) || !strings.Contains(err.Error(), "local network") {
		t.Fatalf("local without the switch: %v", err)
	}
	for _, bad := range []string{"http://169.254.169.254/", "http://metadata.google.internal/", "http://[fe80::1]/"} {
		_, err := Probe(context.Background(), api(Radarr, bad, key))
		if !errors.Is(err, ErrUnreachable) {
			t.Errorf("%s: %v", bad, err)
		}
	}
	for _, bad := range []string{"ftp://x/", "http://user:pw@10.0.0.1/", "http://10.0.0.1/?apikey=x", "nohost"} {
		if _, err := Probe(context.Background(), api(Radarr, bad, key)); err == nil || errors.Is(err, ErrUnreachable) {
			t.Errorf("%s: %v", bad, err)
		}
	}
	if _, err := Probe(context.Background(), Source{Kind: Radarr}); !errors.Is(err, ErrBadSource) {
		t.Fatalf("empty source: %v", err)
	}
	if _, err := Probe(context.Background(), Source{Kind: Radarr, URL: u, Path: "/x"}); !errors.Is(err, ErrBadSource) {
		t.Fatalf("both: %v", err)
	}
	if _, err := Probe(context.Background(), Source{Kind: "readarr", URL: u}); err == nil {
		t.Fatal("unknown kind read")
	}
}

// TestTypedKeyNeverKept: a Snapshot that would hold the typed key (here
// an app echoing it as a title) is refused whole.
func TestTypedKeyNeverKept(t *testing.T) {
	key := "typed-key-1234"
	srv := fakeAPI(t, "X-Api-Key", key, map[string]any{
		"/api/v3/system/status": map[string]any{"appName": "Radarr", "version": "6"},
		"/api/v3/movie":         []any{map[string]any{"id": 1, "title": "echo " + key}},
	})
	if _, err := Read(context.Background(), api(Radarr, srv.URL, key)); !errors.Is(err, ErrHeldKey) {
		t.Fatalf("echoed key: %v", err)
	}
}

func TestShortTypedKeyNeverKept(t *testing.T) {
	key := "abc"
	srv := fakeAPI(t, "X-Api-Key", key, map[string]any{
		"/api/v3/system/status": map[string]any{"appName": "Radarr", "version": "6"},
		"/api/v3/movie":         []any{map[string]any{"id": 1, "title": "echo " + key}},
	})
	if _, err := Read(context.Background(), api(Radarr, srv.URL, key)); !errors.Is(err, ErrHeldKey) {
		t.Fatalf("echoed short key: %v", err)
	}
}

func TestSecretNames(t *testing.T) {
	for _, n := range []string{"apiKey", "password", "authPassword", "authUsername", "accessToken", "cookie", "webHookUrl", "baseSettings.apiKey", "PostgresPassword", "pgpKey", "x_api_key", "secretToken"} {
		if !IsSecretName(n) {
			t.Errorf("%s not secret", n)
		}
	}
	for _, n := range []string{"baseUrl", "host", "port", "username", "categories", "keyword", "name", "seedRatio"} {
		if IsSecretName(n) {
			t.Errorf("%s secret", n)
		}
	}
	set, gone := plainSettings(map[string]any{"url": "http://u:S3CR3T@h:1/p?apikey=S3CR3T&cat=2#f", "link": "pushover://S3CR3T@x", "nested": map[string]any{"a": 1}})
	if strings.Contains(set["url"].(string), secretMark) || set["url"] != "http://h:1/p?cat=2" || set["nested"] != nil || len(gone) != 1 || gone[0] != "link" {
		t.Fatalf("%+v %v", set, gone)
	}
	// An on/off switch is never a secret, even when its name holds a word like hash.
	set, gone = plainSettings(map[string]any{"rejectBlocklistedTorrentHashesWhileGrabbing": true, "seriesMetadataEpisodeGuide": false, "pinCode": "1234"})
	if set["rejectBlocklistedTorrentHashesWhileGrabbing"] != true || set["seriesMetadataEpisodeGuide"] != false || len(gone) != 1 || gone[0] != "pinCode" {
		t.Fatalf("switches: %+v %v", set, gone)
	}
}

func TestINIAndYAML(t *testing.T) {
	f := parseINI([]byte("__version__ = 19\n[misc]\nport = 8080\n[servers]\n[[news.example.com]]\nhost = news.example.com\n[categories]\n[[tv]]\ndir = tv\n"))
	if f[""]["__version__"] != "19" || f["misc"]["port"] != "8080" || f["servers/news.example.com"]["host"] != "news.example.com" || len(f.subsections("categories")) != 1 {
		t.Fatalf("%+v", f)
	}
	y := parseYAML([]byte("general:\n  enabled_providers:\n  - opensubtitlescom\n  - podnapisi\n  serie_default_profile: 1\nopensubtitlescom:\n  username: bob\n  password: 'S3CR3T'\naddic7ed:\n  cookies: ''\nlist: [a, b]\n"))
	if l, _ := y["general.enabled_providers"].([]any); len(l) != 2 || y["general.serie_default_profile"] != int64(1) || y["opensubtitlescom.password"] != "S3CR3T" || len(y["list"].([]any)) != 2 {
		t.Fatalf("%+v", y)
	}
}
