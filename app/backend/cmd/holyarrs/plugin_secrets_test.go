package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// TestPluginReadsHandOutNoSecret seeds a server with a source key, a proxy
// password, a download client password and a media server token, gives a
// plugin every scope, and calls every route a plugin may GET: no answer may
// hold a secret, and every closed route refuses the token.
func TestPluginReadsHandOutNoSecret(t *testing.T) {
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctx := context.Background()
	if err := st.Write(ctx, insertFolder(t)); err != nil {
		t.Fatal(err)
	}
	v, err := secrets.Open(ctx, st, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	marks := []string{"SRCKEYsecret111", "PROXYPWsecret222", "DLPWsecret333", "MSTOKENsecret444", "PATHKEYsecret555"}
	ref := func(s string) *int64 {
		id, err := v.Put(ctx, "test", s)
		if err != nil {
			t.Fatal(err)
		}
		return &id
	}
	src, proxy, dl, ms, path := ref(marks[0]), ref(marks[1]), ref(marks[2]), ref(marks[3]), ref(marks[4])
	if err := st.Write(ctx, func(tx *sql.Tx) error {
		if _, err := database.Insert(tx, "sources", &domain.Source{Name: "S", Protocol: "torrent", Format: "torznab", URL: "http://x/api",
			SecretRef: src, ProxyURL: "http://proxy:8080", ProxySecretRef: proxy, PathSecretRef: path, Kinds: []string{"movie"},
			Categories: map[string][]int{}, State: "enabled", Extra: map[string]string{}}, domain.System); err != nil {
			return err
		}
		if _, err := database.Insert(tx, "downloaders", &domain.Downloader{Name: "Q", Adapter: "qbittorrent", Protocol: "torrent", URL: "http://q:8080",
			SecretRef: dl, Category: "holyarrs", AfterImport: "leave", State: "ok", PathMap: []domain.PathMap{}}, domain.System); err != nil {
			return err
		}
		_, err := database.Insert(tx, "media_servers", &domain.MediaServer{Name: "P", Kind: "plex", URL: "http://plex:32400", TokenSecretID: ms}, domain.System)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	au := accounts.New(st, accounts.Options{Vault: v})
	q := jobs.New(st, jobs.Options{Workers: 1})
	a := buildHandlers(services{Store: st, Queue: q, Metadata: metadata.Lab(), Version: "test", Auth: au, Secrets: v, Live: webserver.NewLiveHub(webserver.LiveOptions{})})
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)
	jar, _ := cookiejar.New(nil)
	owner := &signedIn{t: t, base: srv.URL, c: &http.Client{Jar: jar}}
	ownerMark := "OWNERPW" + "mark666"
	setupBody, _ := json.Marshal(map[string]string{"name": "luis", "password": ownerMark + " long enough"})
	owner.call("POST", "/api/v1/auth/setup", string(setupBody), 201)
	marks = append(marks, ownerMark)

	tok := accounts.IssuePluginToken("walker", accounts.PluginScopeList())
	t.Cleanup(func() { accounts.RevokePluginToken("walker") })
	param := regexp.MustCompile(`\{[^}]+\}`)
	get := func(path string) (int, string) {
		req, _ := http.NewRequest("GET", srv.URL+path, nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		return resp.StatusCode, string(b)
	}
	walked := 0
	for r := range accounts.PluginScopes {
		method, path, _ := strings.Cut(r, " ")
		if method != "GET" || strings.Contains(path, "/stream/") {
			continue
		}
		path = strings.TrimSuffix(param.ReplaceAllString(path, "1"), "...")
		_, body := get(path)
		walked++
		for _, m := range marks {
			if strings.Contains(body, m) {
				t.Errorf("%s hands a plugin a secret: %s", r, m)
			}
		}
	}
	if walked < 100 {
		t.Fatalf("walked only %d plugin GET routes", walked)
	}
	for r := range accounts.PluginClosed {
		method, path, _ := strings.Cut(r, " ")
		if method != "GET" || strings.Contains(path, "{") || strings.HasPrefix(path, "/sonarr") || strings.HasPrefix(path, "/radarr") {
			continue
		}
		if resp, err := http.Get(srv.URL + path); err == nil {
			resp.Body.Close()
			if resp.StatusCode < 400 {
				continue // public to everyone (sign-in page, first run)
			}
		}
		if code, _ := get(path); code < 400 {
			t.Errorf("closed %s answered a plugin with %d", r, code)
		}
	}
}
