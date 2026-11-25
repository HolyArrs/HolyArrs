package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/alerts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/plugins/hooks"
	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
)

// TestPluginArtworkNotifyRatingsEndToEnd: a plugin sets a poster, sends a
// message through the owner's Alerts and adds a rating, all over HTTP with
// its Bearer token; without each scope it gets 403.
func TestPluginArtworkNotifyRatingsEndToEnd(t *testing.T) {
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	v, err := secrets.Open(ctx, st, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	var tid int64
	if err := st.Write(ctx, func(tx *sql.Tx) error {
		ti := &mediafiles.Title{Kind: "movie", GateDriver: "none", Name: "Dune", SortName: "dune", Aliases: []string{}, SourceIDs: map[string]string{},
			MetadataSource: "local", NewPartsPolicy: "all", ReleaseGate: "released", MusicTypes: []string{}, WhyAdded: "by_hand", Artwork: []string{}, Status: "released", State: "active"}
		_, err := database.Insert(tx, "titles", ti, domain.System)
		tid = ti.ID
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var hooked []string
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		hooked = append(hooked, string(b))
		mu.Unlock()
	}))
	t.Cleanup(hook.Close)
	yes := true
	if _, err := alerts.Add(ctx, st, v, alerts.Input{Name: "Hook", Kind: "webhook", URL: hook.URL + "/in", AllowLocal: &yes, Moments: []string{alerts.MomentPlugin}}, domain.System); err != nil {
		t.Fatal(err)
	}
	reg := newPluginHooks()
	t.Cleanup(func() { sources.PluginTransport = nil })
	lib := &library.Service{Store: st, Artwork: &library.ArtworkCache{Dir: t.TempDir()}}
	sender := &alerts.Sender{Store: st, Vault: v, Backoff: []time.Duration{}}
	au := accounts.New(st, accounts.Options{Vault: v})
	a := buildHandlers(services{Store: st, Queue: jobs.New(st, jobs.Options{Workers: 1}), Metadata: metadata.Lab(), Version: "test", Auth: au, Secrets: v,
		Live: webserver.NewLiveHub(webserver.LiveOptions{}), PluginHooks: reg, Library: lib, Alerts: sender})
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)
	t.Cleanup(cancel) // stub plugins stop before the server closes

	full := accounts.IssuePluginToken("helper", []string{"library.artwork.set", "alerts.webhooks.send", "sources.metadata"})
	none := accounts.IssuePluginToken("reader", []string{"library.read"})
	t.Cleanup(func() { accounts.RevokePluginToken("helper"); accounts.RevokePluginToken("reader"); reg.Drop("helper") })
	p := stubPlugin{t: t, base: srv.URL, tok: full}
	q := stubPlugin{t: t, base: srv.URL, tok: none}

	img := image.NewRGBA(image.Rect(0, 0, 200, 300))
	img.Set(5, 5, color.RGBA{R: 200, A: 255})
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	art := map[string]any{"title_id": tid, "kind": "poster", "image": buf.Bytes()}
	if code, b := q.do("POST", "/api/v1/plugin-hooks/artwork", art); code != http.StatusForbidden {
		t.Fatalf("artwork without its scope: %d %s", code, b)
	}
	if code, b := p.do("POST", "/api/v1/plugin-hooks/artwork", art); code != http.StatusNoContent {
		t.Fatalf("artwork: %d %s", code, b)
	}
	var raw string
	_ = st.Read().QueryRow("SELECT artwork FROM titles WHERE id = ?", tid).Scan(&raw)
	if !strings.HasPrefix(raw, `["poster:`) {
		t.Fatalf("title artwork %s", raw)
	}
	var via *string
	_ = st.ReadEvents().QueryRow("SELECT via_plugin FROM events WHERE kind = 'title_artwork_set'").Scan(&via)
	if via == nil || *via != "helper" {
		t.Errorf("history does not name the plugin: %v", via)
	}

	msg := map[string]string{"subject": "Backup done", "text": "All 3 folders"}
	if code, _ := q.do("POST", "/api/v1/plugin-hooks/notify", msg); code != http.StatusForbidden {
		t.Fatalf("notify without its scope: %d", code)
	}
	if code, b := p.do("POST", "/api/v1/plugin-hooks/notify", msg); code != http.StatusNoContent {
		t.Fatalf("notify: %d %s", code, b)
	}
	mu.Lock()
	if len(hooked) != 1 || !strings.Contains(hooked[0], "From helper: Backup done") {
		t.Errorf("webhook got %v", hooked)
	}
	mu.Unlock()

	if code, _ := q.do("GET", "/api/v1/plugin-hooks/metadata", nil); code != http.StatusForbidden {
		t.Fatalf("metadata without its scope: %d", code)
	}
	p.serve(ctx, "metadata", func(j hooks.Job) any {
		return map[string]any{"scores": []metadata.ServerScore{{Source: "Letterboxd", Value: 4.2, Max: 5, Votes: 10}}}
	})
	waitProvider(t, reg, hooks.Metadata, "helper")
	got, err := reg.WithPluginRatings(nil)(ctx, "movie", "438631")
	if err != nil || len(got.Scores) != 1 || got.Scores[0].Source != "Letterboxd" {
		t.Fatalf("ratings %+v %v", got, err)
	}
	b, _ := json.Marshal(got)
	if !strings.Contains(string(b), "4.2") {
		t.Errorf("ratings %s", b)
	}
}
