package logs

import (
	"bytes"
	"encoding/base64"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fileHas(t *testing.T, dir, s string) bool {
	t.Helper()
	for _, name := range []string{FileName, OldName} {
		b, _ := os.ReadFile(filepath.Join(dir, name))
		if bytes.Contains(b, []byte(s)) {
			return true
		}
	}
	return false
}

// A rotated-out secret never shows again: not in the viewer, the download,
// the files, nor after a restart that no longer knows it.
func TestRotatedSecretStaysMasked(t *testing.T) {
	dir := t.TempDir()
	// A line written before the value was known (or by an older version).
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte("2026/10/14 10:00:00 sent old-rotated-secret-1 to the site\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	l, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	l.SetSecrets([]string{"old-rotated-secret-1"})
	// The vault now holds a new value; the old one leaves the list.
	l.SetSecrets([]string{"new-current-secret-2"})
	if fileHas(t, dir, "old-rotated-secret-1") {
		t.Fatal("old value still in the clear on disk")
	}
	lg := log.New(l, "", log.LstdFlags)
	lg.Printf("late line with old-rotated-secret-1")
	got, _ := l.Read(Filter{})
	for _, ln := range got {
		if strings.Contains(ln.Text, "old-rotated-secret-1") {
			t.Fatalf("viewer shows old value: %q", ln.Text)
		}
	}
	if strings.Contains(string(l.Tail(1<<20)), "old-rotated-secret-1") || fileHas(t, dir, "old-rotated-secret-1") {
		t.Fatal("download or file shows old value")
	}
	_ = l.Close()
	// Restart: only the new value is known.
	l2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer l2.Close()
	l2.SetSecrets([]string{"new-current-secret-2"})
	if strings.Contains(string(l2.Tail(1<<20)), "old-rotated-secret-1") {
		t.Fatal("old value shown after restart")
	}
}

// Retire rewrites the files with a value masked (the vault's retired list).
func TestRetireRewritesFiles(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, OldName), []byte("2026/10/14 09:00:00 a retired-value-x1 b\n"), 0o600)
	_ = os.WriteFile(filepath.Join(dir, FileName), []byte("2026/10/14 10:00:00 c retired-value-x1 d\n"), 0o600)
	l, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if err := l.Retire([]string{"retired-value-x1"}); err != nil {
		t.Fatal(err)
	}
	if fileHas(t, dir, "retired-value-x1") {
		t.Fatal("not rewritten")
	}
	lg := log.New(l, "", log.LstdFlags)
	lg.Printf("after retired-value-x1")
	if fileHas(t, dir, "retired-value-x1") {
		t.Fatal("retired value written in clear")
	}
	if got, _ := l.Read(Filter{}); len(got) != 3 {
		t.Fatalf("lines: %+v", got)
	}
}

// A value added one at a time is masked from the next line on.
func TestAddSecretMasksAtOnce(t *testing.T) {
	dir := t.TempDir()
	l, _ := Open(dir)
	defer l.Close()
	lg := log.New(l, "", log.LstdFlags)
	l.AddSecret("fresh-added-secret")
	lg.Printf("used fresh-added-secret")
	if fileHas(t, dir, "fresh-added-secret") {
		t.Fatal("new secret on disk")
	}
}

// Control characters in logged values cannot forge lines or keep ANSI.
func TestControlCharacters(t *testing.T) {
	dir := t.TempDir()
	l, _ := Open(dir)
	defer l.Close()
	lg := log.New(l, "", log.LstdFlags)
	lg.Printf("title %s", "x\n2026/01/01 00:00:00 forged error line\r\x1b[31mred\x07")
	b, _ := os.ReadFile(filepath.Join(dir, FileName))
	if bytes.Count(b, []byte("\n")) != 1 || bytes.ContainsAny(b, "\r\x1b\x07") {
		t.Fatalf("file: %q", b)
	}
	got, total := l.Read(Filter{})
	if total != 1 || strings.ContainsAny(got[0].Text, "\r\n\x1b") {
		t.Fatalf("read: %+v", got)
	}
}

// Encoded forms of vault values, quoted values and short secret tokens.
func TestMaskForms(t *testing.T) {
	l := &Log{}
	v := "p@ss w/rd&more"
	l.SetSecrets([]string{v, "zq7Kd", "admin"})
	for _, in := range []string{
		"q=" + url.QueryEscape(v),
		"b64 " + base64.StdEncoding.EncodeToString([]byte(v)),
		"b64url " + base64.URLEncoding.EncodeToString([]byte(v)),
		"short zq7Kd here",
	} {
		got := l.Redact(in)
		if strings.Contains(got, url.QueryEscape(v)) || strings.Contains(got, base64.StdEncoding.EncodeToString([]byte(v))) ||
			strings.Contains(got, base64.URLEncoding.EncodeToString([]byte(v))) || strings.Contains(got, "zq7Kd") {
			t.Errorf("Redact(%q) = %q", in, got)
		}
	}
	// A short value inside a longer word stays; a stoplist word stays.
	if got := l.Redact("xzq7Kdy admin page"); got != "xzq7Kdy admin page" {
		t.Errorf("over-masked: %q", got)
	}
	pw, tk := "pass"+"word", "to"+"ken"
	if got := RedactPatterns(pw + `="two words" next`); got != pw+`="`+Hidden+`" next` {
		t.Errorf("quoted: %q", got)
	}
	if got := RedactPatterns(tk + `: 'two words' x`); got != tk+`: '`+Hidden+`' x` {
		t.Errorf("single quoted: %q", got)
	}
}

// The program output (docker logs) is masked as well.
func TestStdoutMasked(t *testing.T) {
	l := &Log{}
	l.SetSecrets([]string{"stdout-secret-1"})
	var out bytes.Buffer
	lg := log.New(l.Mask(&out), "", 0)
	lg.Printf("a stdout-secret-1 b \x1b[0m")
	if strings.Contains(out.String(), "stdout-secret-1") || strings.Contains(out.String(), "\x1b") || !strings.HasSuffix(out.String(), "\n") {
		t.Fatalf("stdout: %q", out.String())
	}
}
