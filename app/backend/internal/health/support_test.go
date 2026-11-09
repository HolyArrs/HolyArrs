package health

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// route: GET /api/v1/support/bundle
func TestSupportBundleHidesSecrets(t *testing.T) {
	// The doctor's network check goes to a closed local port, never out.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	t.Setenv(metadata.EnvServiceURL, "http://"+addr)
	srv, v, lg := logServer(t)
	const vaultOnly = "VAULTONLYSECRETVALUE"
	if err := v.SetSetting(context.Background(), "links", "tmdb_token", vaultOnly, domain.System); err != nil {
		t.Fatal(err)
	}
	secrets := []string{vaultOnly, "SEEDKEY1", "SEEDTOKEN2", "SEEDPASS3", "SEEDBEARER4abcdef", "SEEDAUTH5", "SEEDQUERY6", "SEEDHDR7", "hak_SEEDHAK8"}
	l := log.New(lg, "", log.LstdFlags)
	l.Printf("tmdb: answered 401 for %s", vaultOnly)
	l.Printf("source: GET http://indexer:9696/api?t=search&apikey=SEEDQUERY6&q=x")
	l.Printf("downloader: login with api_key=SEEDKEY1 token: SEEDTOKEN2 password=\"SEEDPASS3\"")
	l.Printf("request headers: Authorization: Bearer SEEDBEARER4abcdef")
	l.Printf("request headers: Authorization: SEEDAUTH5")
	l.Printf("request headers: X-Api-Key: SEEDHDR7")
	l.Printf("key made: hak_SEEDHAK8")
	l.Printf("downloader: qBittorrent failed: connection refused")

	resp, err := http.Get(srv.URL + "/api/v1/support/bundle")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "application/zip" || resp.Header.Get("Cache-Control") != "no-store" ||
		!strings.Contains(resp.Header.Get("Content-Disposition"), `filename="holyarrs-support-`) {
		t.Fatalf("%d %v", resp.StatusCode, resp.Header)
	}
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(rc)
		rc.Close()
		files[f.Name] = string(b)
	}
	for _, name := range []string{"log.txt", "about.json", "doctor.txt", "health.json"} {
		if files[name] == "" {
			t.Fatalf("no %s in %v", name, zr.File)
		}
	}
	for name, text := range files {
		for _, s := range secrets {
			if strings.Contains(text, s) {
				t.Errorf("%s holds %q:\n%s", name, s, text)
			}
		}
	}
	if !strings.Contains(files["log.txt"], "connection refused") || !strings.Contains(files["doctor.txt"], "HolyArrs doctor") ||
		!strings.Contains(files["about.json"], `"version": "test"`) || !strings.Contains(files["health.json"], `"ok"`) {
		t.Fatalf("contents: %v", files)
	}
}

func TestSupportBundleRedactsAgain(t *testing.T) {
	for _, c := range []struct{ in, gone string }{
		{"Authorization: Basic dXNlcjpwYXNz", "dXNlcjpwYXNz"},
		{"proxy-authorization=whatever here", "whatever"},
		{`{"apiKey": "abc123"}`, "abc123"},
		{"x-plex-token: PLEXTOK", "PLEXTOK"},
		{"refresh_token=RTOK&x=1", "RTOK"},
	} {
		if got := RedactBundle(c.in); strings.Contains(got, c.gone) {
			t.Errorf("%q -> %q", c.in, got)
		}
	}
	if got := RedactBundle("folders: 4 ok\nfix: run chown"); got != "folders: 4 ok\nfix: run chown" {
		t.Fatalf("plain text changed: %q", got)
	}
}
