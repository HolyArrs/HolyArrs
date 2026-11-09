package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/requests"
)

// route: GET /api/v1/my/plex-watchlist
// route: PUT /api/v1/my/plex-watchlist
// route: POST /api/v1/my/plex-watchlist/check
func TestPlexWatchlistOverHTTP(t *testing.T) {
	t.Parallel()
	r := newAskRig(t)
	ctx := context.Background()
	plex := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("X-Plex-Token") != "tok-ana" {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"MediaContainer":{"size":1,"Metadata":[{"ratingKey":"k1","type":"movie","title":"Quiet Orchard","year":2022,"Guid":[{"id":"tmdb://900003"}]}]}}`))
	}))
	defer plex.Close()
	wl := &requests.Watchlists{Asks: r.asks, Base: plex.URL, Token: func(_ context.Context, id int64) (string, error) {
		if id == r.ana.id {
			return "tok-ana", nil
		}
		return "", nil
	}}
	*r.wl = *wl
	type view struct {
		On, Linked    bool
		Movies, Shows bool
		MayAsk        bool `json:"may_ask"`
		CanReadJSON   bool `json:"can_read"`
		Items         []requests.WatchlistItem
	}
	get := func() view {
		var v view
		_ = json.Unmarshal(r.ana.call("GET", "/api/v1/my/plex-watchlist", "", 200), &v)
		return v
	}
	if v := get(); v.On || v.Linked || v.CanReadJSON || !v.MayAsk {
		t.Fatalf("first view %+v", v)
	}
	// Off in Request rules: a person cannot turn it on.
	r.ana.call("PUT", "/api/v1/my/plex-watchlist", `{"movies":true}`, 409)
	r.owner.call("PUT", "/api/v1/settings/asks", `{"movies_per_week":10,"shows_per_week":10,"music_per_week":10,"search_on_approve":true,"plex_watchlists":true}`, 200)
	if !strings.Contains(string(r.owner.call("GET", "/api/v1/settings/asks", "", 200)), `"plex_watchlists":true`) {
		t.Fatal("owner switch not kept")
	}
	// No Plex account linked yet.
	if b := r.ana.call("PUT", "/api/v1/my/plex-watchlist", `{"movies":true}`, 409); !strings.Contains(string(b), "link your Plex") {
		t.Fatalf("no link: %s", b)
	}
	r.ana.call("POST", "/api/v1/my/plex-watchlist/check", "", 409)
	if err := r.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("INSERT INTO user_links (user_id, provider, external_id, external_name, created_at) VALUES (?, 'plex', '9001', 'anaplex', '2026-10-01T00:00:00Z')", r.ana.id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	r.ana.call("PUT", "/api/v1/my/plex-watchlist", `{"movies":true}`, 200)
	if v := get(); !v.On || !v.Linked || !v.CanReadJSON || !v.Movies || v.Shows {
		t.Fatalf("after turning on %+v", v)
	}
	r.ana.call("POST", "/api/v1/my/plex-watchlist/check", "", 202)
	// bo turns nothing on and is never read; turning off is always allowed.
	r.bo.call("PUT", "/api/v1/my/plex-watchlist", `{"movies":false,"shows":false}`, 200)
	if _, err := wl.Pass(ctx, r.ana.id); err != nil {
		t.Fatal(err)
	}
	v := get()
	if len(v.Items) != 1 || v.Items[0].State != requests.WatchAsked || v.Items[0].AskID == nil {
		t.Fatalf("items %+v", v.Items)
	}
	a := askOf(t, []byte(`{"ask":`+string(r.ana.call("GET", "/api/v1/asks/"+itoa(*v.Items[0].AskID), "", 200))+`}`))
	if len(a.People) != 1 || !a.People[0].FromWatchlist {
		t.Fatalf("ask people %+v", a.People)
	}
}
