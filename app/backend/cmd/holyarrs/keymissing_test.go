package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
)

// A missing master key for a database with saved secrets: the app keeps
// listening and says what to do, instead of exiting into a restart loop
// that NAS users only see in the container log.
func TestMissingKeyKeepsAPlainPage(t *testing.T) {
	t.Setenv(secrets.EnvKeyFile, "")
	dir := t.TempDir()
	s, err := database.Open(dir, database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	key, err := secrets.LoadKey(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	v, err := secrets.Open(context.Background(), s, key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Put(context.Background(), "source_api_key", "lab-value"); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	if err := os.Remove(filepath.Join(dir, secrets.KeyFile)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "holyarrs.key.txt"), []byte("KEYCONTENT"), 0o600); err != nil {
		t.Fatal(err)
	}
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := l.Addr().String()
	_ = l.Close()
	c, err := parseFlags([]string{"-config", dir, "-addr", addr}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serve(ctx, c, runOpts{}) }()
	var resp *http.Response
	for i := 0; i < 100; i++ {
		select {
		case err := <-done:
			cancel()
			t.Fatalf("serve exited instead of keeping a page: %v", err)
		default:
		}
		if resp, err = http.Get("http://" + addr + "/healthz"); err == nil && resp.StatusCode == 503 {
			break
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil || resp.StatusCode != 503 {
		cancel()
		t.Fatalf("healthz: %v %v", resp, err)
	}
	resp.Body.Close()
	resp, err = http.Get("http://" + addr + "/")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	// The page is open to anyone: it points at the log and shows no path or
	// file name from the config folder, and never a key.
	if !strings.Contains(string(b), "master key file is missing") || !strings.Contains(string(b), "holyarrs: not started") ||
		strings.Contains(string(b), dir) || strings.Contains(string(b), "holyarrs.key.txt") || strings.Contains(string(b), "KEYCONTENT") {
		t.Fatalf("page %q", b)
	}
	// The config folder here is not /config, so the page does not name
	// /config; it stays as before.
	if strings.Contains(string(b), "/config") {
		t.Fatalf("page names /config for a config folder elsewhere: %q", b)
	}
	if _, err := os.Stat(filepath.Join(dir, secrets.KeyFile)); err == nil {
		t.Fatal("a new key was made")
	}
	cancel()
	if err := <-done; err == nil {
		t.Fatal("serve ended without the error")
	}
}

// keyPage is the page body the held handler shows for a missing key with
// the given config folder and browser language.
func keyPage(t *testing.T, dir, lang string) string {
	t.Helper()
	h := keyHeldHandler(fmt.Errorf("load key: %w", secrets.ErrKeyMissing), dir)
	req := httptest.NewRequest("GET", "/", nil)
	if lang != "" {
		req.Header.Set("Accept-Language", lang)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "master key file is missing") {
		t.Fatalf("%s %s: %d %q", dir, lang, rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

// The /config line is only true in Docker, where the config folder is
// /config; other installs (the Linux package, Synology, portable) keep the
// page as it was.
func TestMissingKeyLineOnlyForConfig(t *testing.T) {
	for _, dir := range []string{"/config", "/config/"} {
		if b := keyPage(t, dir, ""); !strings.Contains(b, "HolyArrs looked in /config and found no master key there.") {
			t.Fatalf("%s: page has no /config line: %q", dir, b)
		}
	}
	b := keyPage(t, "/config", "de-DE,de;q=0.9,en;q=0.8")
	if !strings.Contains(b, "dort keinen Hauptschlüssel gefunden") || strings.Contains(b, "HolyArrs looked in") {
		t.Fatalf("German page %q", b)
	}
	for _, dir := range []string{t.TempDir(), "/var/lib/holyarrs", "/srv/holyarrs/config", `C:\HolyArrs\config`, "/volume1/@appdata/holyarrs/var/config"} {
		if b := keyPage(t, dir, ""); strings.Contains(b, "/config") || strings.Contains(b, "HolyArrs looked") {
			t.Fatalf("%s: page names /config: %q", dir, b)
		}
	}
	if len(keyNotInConfig) != 44 {
		t.Fatalf("the /config line is in %d languages, want 44", len(keyNotInConfig))
	}
}

// The page is plain text, so a right-to-left line is wrapped in a
// right-to-left isolate to keep its pieces in order; others are left as is.
func TestMissingKeyLineRightToLeft(t *testing.T) {
	rtl := map[string]bool{"ar": true, "fa": true, "he": true}
	for lang, line := range keyNotInConfig {
		got := keyLine(lang)
		wrapped := "\u2067" + strings.ReplaceAll(line, "/config", "\u2066/config\u2069") + "\u2069"
		if rtl[lang] && got != wrapped {
			t.Fatalf("%s: not wrapped: %q", lang, got)
		}
		if !rtl[lang] && got != line {
			t.Fatalf("%s: changed: %q", lang, got)
		}
	}
	for _, lang := range []string{"ar", "fa", "he"} {
		if b := keyPage(t, "/config", lang); !strings.Contains(b, keyLine(lang)) || !strings.Contains(b, "\u2066/config\u2069") {
			t.Fatalf("%s page %q", lang, b)
		}
	}
	if b := keyPage(t, "/config", "en-US"); strings.ContainsAny(b, "\u2067\u2069") {
		t.Fatalf("English page has isolate marks: %q", b)
	}
}
