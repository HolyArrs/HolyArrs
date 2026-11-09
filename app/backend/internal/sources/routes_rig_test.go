package sources

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// Test rig for this module's routes on their own router, without the app.

const sourceCaps = `<?xml version="1.0"?><caps><limits max="50"/><searching><search available="yes" supportedParams="q"/>
<movie-search available="yes" supportedParams="q,imdbid"/></searching><categories><category id="2000"/></categories></caps>`

type rigCatalog struct{}

func (rigCatalog) TitleFromPath(w http.ResponseWriter, r *http.Request) (mediafiles.Title, bool) {
	http.NotFound(w, r)
	return mediafiles.Title{}, false
}
func (rigCatalog) EpisodeLabels(context.Context, []int64) string { return "" }
func (rigCatalog) AlbumLabels(context.Context, []int64) string   { return "" }
func (rigCatalog) PartLabel(context.Context, int64) (int64, string, string, string) {
	return 0, "", "", ""
}

type rigOldApps struct{}

func (rigOldApps) CanMakeKeys() bool                                 { return false }
func (rigOldApps) KeySides(context.Context) (map[string]bool, error) { return map[string]bool{}, nil }
func (rigOldApps) MakeKey(*http.Request, string) (string, error)     { return "", nil }
func (rigOldApps) DeleteKey(*http.Request, string) error             { return nil }
func (rigOldApps) On(context.Context) bool                           { return false }
func (rigOldApps) SetOn(context.Context, domain.Actor, bool) error   { return nil }

// routesServer serves this module's routes over a store and an optional vault.
func routesServer(t *testing.T, st *database.Store, v *secrets.Vault) *httptest.Server {
	t.Helper()
	sr := &Searcher{Store: st, Secrets: v}
	h := &Handlers{Store: st, Vault: v, Search: func() *Searcher { return sr }, Catalog: rigCatalog{}, OldApps: rigOldApps{},
		PartOn: func(parts.Part) bool { return true }}
	rt := webserver.NewRouter()
	h.Routes(rt)
	srv := httptest.NewServer(rt)
	t.Cleanup(srv.Close)
	return srv
}

// sourceServer opens a fresh store and vault and serves the routes over them.
func sourceServer(t *testing.T) (*httptest.Server, *secrets.Vault) {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	v, err := secrets.Open(context.Background(), s, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	return routesServer(t, s, v), v
}

func do(t *testing.T, method, url, body string, want int) []byte {
	t.Helper()
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != want {
		t.Fatalf("%s %s: %d, want %d: %s", method, url, resp.StatusCode, want, b)
	}
	return b
}
