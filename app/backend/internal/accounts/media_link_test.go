package accounts

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

// mediaLinked wires one fake Jellyfin server in as the owner's connected one.
func mediaLinked(t *testing.T, f *fixture) *atomic.Int32 {
	var logouts, linked atomic.Int32
	ms := fakeMediaServer(t, &logouts)
	f.a.SetMediaLinks(MediaLinks{
		Servers: func(context.Context) ([]LinkServer, error) {
			return []LinkServer{{ID: 7, Name: "Den", Kind: "jellyfin", URL: ms.URL}}, nil
		},
		Accounts: func(context.Context, int64) ([]ServerAccount, error) {
			return []ServerAccount{{ID: "abc123", Name: "Mum"}, {ID: "kid1", Name: "Kid"}}, nil
		},
		Linked: func() { linked.Add(1) },
	})
	return &linked
}

// route: GET /api/v1/auth/media-link
// route: POST /api/v1/auth/media-link/{server}
// route: DELETE /api/v1/auth/media-link/{kind}
func TestMyMediaLink(t *testing.T) {
	f := gated(t, Options{})
	linked := mediaLinked(t, f)
	own := ownerOf(t, f)
	ana := join(t, f, invite(t, f, own, `{"role":"member"}`), "ana")
	resp, out := ana.do(t, f, "GET", "/api/v1/auth/media-link", "")
	if resp.StatusCode != 200 || !strings.Contains(out, `"name":"Den"`) || strings.Contains(out, "http") {
		t.Fatalf("list: %d %s", resp.StatusCode, out)
	}
	if resp, _ := ana.do(t, f, "POST", "/api/v1/auth/media-link/7", `{"name":"mum","password":"wrong"}`); resp.StatusCode != 401 {
		t.Fatalf("wrong password: %d", resp.StatusCode)
	}
	if resp, _ := ana.do(t, f, "POST", "/api/v1/auth/media-link/9", `{"name":"mum","password":"right"}`); resp.StatusCode != 404 {
		t.Fatalf("unknown server: %d", resp.StatusCode)
	}
	if resp, out := ana.do(t, f, "POST", "/api/v1/auth/media-link/7", `{"name":"mum","password":"right"}`); resp.StatusCode != 200 {
		t.Fatalf("link: %d %s", resp.StatusCode, out)
	}
	if linked.Load() != 1 {
		t.Fatal("watch read not asked for")
	}
	links, _ := LinksOfPerson(context.Background(), f.s.Read(), userID(t, f, "ana"))
	if len(links) != 1 || links[0].ExtID != "abc123" {
		t.Fatalf("links = %v", links)
	}
	if _, out := ana.do(t, f, "GET", "/api/v1/auth/media-link", ""); !strings.Contains(out, `"provider":"jellyfin","name":"Mum"`) {
		t.Fatalf("my links: %s", out)
	}
	// The same account cannot go to a second person.
	if resp, _ := own.do(t, f, "POST", "/api/v1/auth/media-link/7", `{"name":"mum","password":"right"}`); resp.StatusCode != 409 {
		t.Fatalf("taken: %d", resp.StatusCode)
	}
	if resp, _ := ana.do(t, f, "DELETE", "/api/v1/auth/media-link/jellyfin", ""); resp.StatusCode != 204 {
		t.Fatalf("unlink: %d", resp.StatusCode)
	}
	if resp, _ := ana.do(t, f, "DELETE", "/api/v1/auth/media-link/jellyfin", ""); resp.StatusCode != 404 {
		t.Fatalf("unlink again: %d", resp.StatusCode)
	}
}

// Relinking someone whose only way in is their sign-in link, to an account on
// a server that is not the sign-in one, would lock them out: refused.
func TestRelinkKeepsOnlyWayIn(t *testing.T) {
	f := gated(t, Options{})
	mediaLinked(t, f)
	own := ownerOf(t, f)
	join(t, f, invite(t, f, own, `{"role":"member"}`), "ana")
	uid := userID(t, f, "ana")
	ctx := context.Background()
	err := f.s.WriteCore(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "UPDATE users SET password_hash = '' WHERE id = ?", uid); err != nil {
			return err
		}
		return insertUserLink(ctx, tx, uid, "jellyfin", "signin1", "Ana", "2026-01-01T00:00:00Z")
	})
	if err != nil {
		t.Fatal(err)
	}
	id := strconv.FormatInt(uid, 10)
	if resp, out := own.do(t, f, "PUT", "/api/v1/users/"+id+"/media-link", `{"server":7,"account":"kid1"}`); resp.StatusCode != 409 {
		t.Fatalf("relink: %d %s", resp.StatusCode, out)
	}
	links, _ := LinksOfPerson(ctx, f.s.Read(), uid)
	if len(links) != 1 || links[0].ExtID != "signin1" {
		t.Fatalf("links = %v", links)
	}
}

// route: GET /api/v1/users/media-accounts
// route: PUT /api/v1/users/{id}/media-link
// route: DELETE /api/v1/users/{id}/media-link/{kind}
func TestOwnerLinksSomeone(t *testing.T) {
	f := gated(t, Options{})
	mediaLinked(t, f)
	own := ownerOf(t, f)
	ana := join(t, f, invite(t, f, own, `{"role":"member"}`), "ana")
	id := strconv.FormatInt(userID(t, f, "ana"), 10)
	// A member never lists other people's server accounts, nor links anyone.
	if resp, out := ana.do(t, f, "GET", "/api/v1/users/media-accounts", ""); resp.StatusCode != 403 || strings.Contains(out, "Mum") {
		t.Fatalf("member list: %d %s", resp.StatusCode, out)
	}
	if resp, _ := ana.do(t, f, "PUT", "/api/v1/users/"+id+"/media-link", `{"server":7,"account":"abc123"}`); resp.StatusCode != 403 {
		t.Fatalf("member link: %d", resp.StatusCode)
	}
	if resp, out := own.do(t, f, "GET", "/api/v1/users/media-accounts", ""); resp.StatusCode != 200 || !strings.Contains(out, `"id":"kid1"`) {
		t.Fatalf("owner list: %d %s", resp.StatusCode, out)
	}
	if resp, _ := own.do(t, f, "PUT", "/api/v1/users/"+id+"/media-link", `{"server":7,"account":"made-up"}`); resp.StatusCode != 404 {
		t.Fatalf("invented account: %d", resp.StatusCode)
	}
	if resp, out := own.do(t, f, "PUT", "/api/v1/users/"+id+"/media-link", `{"server":7,"account":"kid1"}`); resp.StatusCode != 200 {
		t.Fatalf("owner link: %d %s", resp.StatusCode, out)
	}
	// The picker shows which account is already someone's, and whose.
	if resp, out := own.do(t, f, "GET", "/api/v1/users/media-accounts", ""); resp.StatusCode != 200 ||
		!strings.Contains(out, `{"id":"kid1","name":"Kid","taken_by":`+id+`,"taken_name":"ana"}`) || !strings.Contains(out, `{"id":"abc123","name":"Mum"}`) {
		t.Fatalf("taken list: %d %s", resp.StatusCode, out)
	}
	if resp, _ := own.do(t, f, "DELETE", "/api/v1/users/"+id+"/media-link/jellyfin", ""); resp.StatusCode != 204 {
		t.Fatalf("owner unlink: %d", resp.StatusCode)
	}
}
