package logs

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRedactPatterns(t *testing.T) {
	for in, want := range map[string]string{
		"GET http://user:XXXX@host:8080/api":                     "GET http://<H>@host:8080/api",
		"feed https://tracker.example/rss?passkey=XXXX_PASS&x=1": "feed https://tracker.example/rss?passkey=<H>&x=1",
		"sources: X-Api-Key: XXXX_KEY sent":                      "sources: X-Api-Key: <H> sent",
		`{"password":"XXXX_PW"}`:                                 `{"password":"<H>"}`,
		"Authorization: Bearer abcdefghijklmnop":                 "Authorization: <H> <H>",
		"key hak_abcDEF123_xyz made":                             "key hak_<H> made",
		"token=XXXX_TOKEN next":                                  "token=<H> next",
		"cookie: XXXX_COOKIE":                                    "cookie: <H>",
		"blob 9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822c":  "blob <H>",
		"Movie.Name.2026.1080p.WEB-DL.x264-GROUP imported":       "Movie.Name.2026.1080p.WEB-DL.x264-GROUP imported",
		"auth: user 1 changed their password":                    "auth: user 1 changed their password",
	} {
		if want = strings.ReplaceAll(want, "<H>", Hidden); RedactPatterns(in) != want {
			got := RedactPatterns(in)
			t.Errorf("RedactPatterns(%q)\n got  %q\n want %q", in, got, want)
		}
	}
	// A bearer token after the header name is hidden one way or another.
	if got := RedactPatterns("sent Bearer abcdefghijklmnopqrstu"); strings.Contains(got, "abcdefghijklmnopqrstu") {
		t.Fatalf("bearer shown: %q", got)
	}
}

func TestWriteMasksRotatesAndReads(t *testing.T) {
	dir := t.TempDir()
	l, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	l.maxFile = 2048
	l.SetSecrets([]string{"vault-secret-value-1", `{"url":"http://x","key":"json-inner-secret"}`, "abc"})
	lg := log.New(l, "", log.LstdFlags)
	lg.Printf("downloader said vault-secret-value-1 and json-inner-secret")
	lg.Printf("health: Downloader qbit failed: connection refused")
	lg.Printf("search: could not reach the Source")
	for i := range 60 {
		lg.Printf("line %d of filler text to rotate the file", i)
	}
	// A partial write is held until its line ends.
	_, _ = l.Write([]byte("half a "))
	_, _ = l.Write([]byte("line\n"))
	if _, err := os.Stat(filepath.Join(dir, OldName)); err != nil {
		t.Fatalf("not rotated: %v", err)
	}
	for _, name := range []string{FileName, OldName} {
		b, _ := os.ReadFile(filepath.Join(dir, name))
		if st, _ := os.Stat(filepath.Join(dir, name)); st.Size() > 2048 {
			t.Fatalf("%s is %d bytes", name, st.Size())
		}
		if strings.Contains(string(b), "vault-secret-value-1") || strings.Contains(string(b), "json-inner-secret") {
			t.Fatalf("secret on disk in %s", name)
		}
	}
	got, total := l.Read(Filter{Text: "HALF A"})
	if total != 1 || got[0].Text != "half a line" || got[0].Level != Info {
		t.Fatalf("text filter: %d %+v", total, got)
	}
	got, total = l.Read(Filter{Level: Warning})
	for _, ln := range got {
		if ln.Level == Info {
			t.Fatalf("info line under a warning filter: %+v", ln)
		}
	}
	got, _ = l.Read(Filter{Limit: 5, Offset: 2})
	if len(got) != 5 || !strings.Contains(got[0].Text, "line 58") {
		t.Fatalf("paging: %+v", got)
	}
	now := time.Now().UTC()
	if got, _ := l.Read(Filter{From: now.Add(time.Hour)}); len(got) != 0 {
		t.Fatalf("from the future: %d", len(got))
	}
	if got, _ := l.Read(Filter{From: now.Add(-time.Hour), To: now.Add(time.Hour), Limit: 1}); len(got) != 1 || got[0].At.IsZero() {
		t.Fatalf("time range: %+v", got)
	}
	tail := l.Tail(200)
	if len(tail) > 200 || !strings.HasSuffix(string(tail), "half a line\n") {
		t.Fatalf("tail: %q", tail)
	}
}

// A secret written before the vault value was known is masked when read.
func TestReadMasksAgain(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(fmt.Sprintf("2026/10/14 10:00:00 old line with later-known-secret\n")), 0o600); err != nil {
		t.Fatal(err)
	}
	l, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	l.SetSecrets([]string{"later-known-secret"})
	got, _ := l.Read(Filter{})
	if len(got) != 1 || strings.Contains(got[0].Text, "later-known-secret") || got[0].At.IsZero() {
		t.Fatalf("read: %+v", got)
	}
	if strings.Contains(string(l.Tail(1<<20)), "later-known-secret") {
		t.Fatal("tail shows the secret")
	}
}

func TestLevels(t *testing.T) {
	for text, want := range map[string]string{
		"api: something went wrong":                         Info,
		"health: check failed":                              Error,
		"supervisor: loop crashed: x":                       Error,
		"store: a write ran past 300ms and was rolled back": Warning,
		"holyarrs 0.0.0: listening":                         Info,
	} {
		if got := LevelOf(text); got != want {
			t.Errorf("LevelOf(%q) = %s, want %s", text, got, want)
		}
	}
}
