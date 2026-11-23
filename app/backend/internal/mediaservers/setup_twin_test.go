package mediaservers

import (
	"context"
	"database/sql"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

func ownerMux(h *Handlers) http.Handler {
	mux := http.NewServeMux()
	h.Routes(mux)
	c := webserver.Caller{UserID: 1, Via: "session", Role: webserver.RoleOwner}
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		mux.ServeHTTP(rw, r.WithContext(webserver.ContextWith(r.Context(), c)))
	})
}

// During first-run setup, saving a media server that the link step already
// saved (same kind and name) updates it; after setup the name check stays.
func TestSetupSaveReusesSameKind(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	h := ownerMux(&Handlers{Store: w.s, Vault: w.v, Linker: &Linker{Store: w.s, Vault: w.v}})
	post := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/media-servers", strings.NewReader(body)))
		return rec
	}
	if rec := post(`{"name":"Jelly","kind":"jellyfin","url":"http://10.0.0.5:8096","token":"a"}`); rec.Code != http.StatusCreated {
		t.Fatalf("first: %d %s", rec.Code, rec.Body)
	}
	if rec := post(`{"name":"jelly","kind":"jellyfin","url":"http://10.0.0.6:8096","token":"b"}`); rec.Code != http.StatusConflict {
		t.Fatalf("no setup link: %d %s", rec.Code, rec.Body)
	}
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		return database.PutSetting(tx, "this_app", "sign_in_jellyfin_on", "true", database.System, time.Now())
	}); err != nil {
		t.Fatal(err)
	}
	if rec := post(`{"name":"jelly","kind":"jellyfin","url":"http://10.0.0.6:8096","token":"b"}`); rec.Code != http.StatusOK {
		t.Fatalf("again in setup: %d %s", rec.Code, rec.Body)
	}
	ms, _ := listServers(ctx, w.s.Read())
	if len(ms) != 1 || ms[0].URL != "http://10.0.0.6:8096" {
		t.Fatalf("servers: %+v", ms)
	}
	if rec := post(`{"name":"Jelly","kind":"emby","url":"http://10.0.0.7:8096","token":"c"}`); rec.Code != http.StatusConflict {
		t.Fatalf("other kind: %d %s", rec.Code, rec.Body)
	}
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		return database.PutSetting(tx, "this_app", "first_run_done", "1", database.System, time.Now())
	}); err != nil {
		t.Fatal(err)
	}
	if rec := post(`{"name":"Jelly","kind":"jellyfin","url":"http://10.0.0.8:8096","token":"d"}`); rec.Code != http.StatusConflict {
		t.Fatalf("after setup: %d %s", rec.Code, rec.Body)
	}
}

// The Plex test answer gets the network spelling of the server name, like
// Jellyfin and Emby; the libraries answer is left alone.
func TestPlexTestNameCase(t *testing.T) {
	plex := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/library/sections":
			fmt.Fprint(rw, `<MediaContainer><Directory key="1" type="movie" title="Movies"/></MediaContainer>`)
		case "/":
			b, _ := xml.Marshal(struct {
				XMLName xml.Name `xml:"MediaContainer"`
				Name    string   `xml:"friendlyName,attr"`
				Version string   `xml:"version,attr"`
			}{Name: "KINGSTREAMINGPC", Version: "1.40.1.8227-abc"})
			rw.Write(b)
		}
	}))
	t.Cleanup(plex.Close)
	w := newWorld(t)
	ctx := context.Background()
	m, err := Add(ctx, w.s, w.v, Input{Name: "Plex", Kind: "plex", URL: plex.URL, Token: "t"}, database.System)
	if err != nil {
		t.Fatal(err)
	}
	h := ownerMux(&Handlers{Store: w.s, Vault: w.v, Linker: &Linker{Store: w.s, Vault: w.v},
		NameCase: func(_ context.Context, name, _ string) string {
			if strings.EqualFold(name, "KingStreamingPC") {
				return "KingStreamingPC"
			}
			return name
		}})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", fmt.Sprintf("/api/v1/media-servers/%d/test", m.ID), nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"found":"KingStreamingPC 1.40.1.8227"`) {
		t.Fatalf("test: %d %s", rec.Code, rec.Body)
	}
}
