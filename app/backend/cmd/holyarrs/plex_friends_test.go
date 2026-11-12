package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/requests"
)

// route: GET /api/v1/settings/asks/plex-friends
func TestPlexFriendsOverHTTP(t *testing.T) {
	t.Parallel()
	r := newAskRig(t)
	ctx := context.Background()
	linked := ""
	r.wl.Owner = func(context.Context) (string, string) { return linked, "" }
	type view struct {
		On          bool                  `json:"on"`
		OwnerLinked bool                  `json:"owner_linked"`
		People      []requests.PlexFriend `json:"people"`
	}
	get := func() view {
		var v view
		if err := json.Unmarshal(r.owner.call("GET", "/api/v1/settings/asks/plex-friends", "", 200), &v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	if v := get(); v.On || v.OwnerLinked || v.People == nil || len(v.People) != 0 {
		t.Fatalf("first view %+v", v)
	}
	// Members never see who else is read.
	r.ana.call("GET", "/api/v1/settings/asks/plex-friends", "", 403)
	if err := r.st.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec("INSERT INTO user_links (user_id, provider, external_id, external_name, created_at) VALUES (?, 'plex', '9001', 'anaplex', '2026-10-01T00:00:00Z')", r.ana.id); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE users SET max_rating = 'PG' WHERE id = ?", r.ana.id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	linked = "tok-owner"
	r.owner.call("PUT", "/api/v1/settings/asks", `{"movies_per_week":10,"shows_per_week":10,"music_per_week":10,"search_on_approve":true,"plex_watchlists":true,"plex_friends":true}`, 200)
	v := get()
	if !v.On || !v.OwnerLinked || len(v.People) != 1 {
		t.Fatalf("after turning on %+v", v)
	}
	if p := v.People[0]; p.UserID != r.ana.id || p.Name != "ana" || p.PlexName != "anaplex" || !p.Kid || p.CheckedAt != nil {
		t.Fatalf("ana's row %+v", p)
	}
	// My account says the owner's link reads ana's watchlist, and she can
	// turn it off.
	type mine struct {
		ReadByOwner bool `json:"read_by_owner"`
		CanRead     bool `json:"can_read"`
		Movies      bool `json:"movies"`
		Shows       bool `json:"shows"`
	}
	var m mine
	_ = json.Unmarshal(r.ana.call("GET", "/api/v1/my/plex-watchlist", "", 200), &m)
	if !m.ReadByOwner || m.CanRead || !m.Movies || !m.Shows {
		t.Fatalf("ana's card %+v", m)
	}
	_ = json.Unmarshal(r.ana.call("PUT", "/api/v1/my/plex-watchlist", `{"movies":false,"shows":false}`, 200), &m)
	if m.Movies || m.Shows {
		t.Fatalf("after turning off %+v", m)
	}
	r.ana.call("PUT", "/api/v1/my/plex-watchlist", `{"shows":true}`, 200)
	// The owner is never read as a friend.
	var o mine
	_ = json.Unmarshal(r.owner.call("GET", "/api/v1/my/plex-watchlist", "", 200), &o)
	if o.ReadByOwner {
		t.Fatalf("owner's card %+v", o)
	}
}
