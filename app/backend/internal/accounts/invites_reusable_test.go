package accounts

import (
	"net/http"
	"testing"
	"time"
)

// listed returns the one invite in the list.
func listed(t *testing.T, f *fixture, by person) InviteView {
	t.Helper()
	resp, out := by.do(t, f, "GET", "/api/v1/users/invites", "")
	if resp.StatusCode != 200 {
		t.Fatalf("list: %d %s", resp.StatusCode, out)
	}
	var v struct {
		Invites []InviteView `json:"invites"`
	}
	jsonOf(t, out, &v)
	if len(v.Invites) != 1 {
		t.Fatalf("want one invite, got %d: %s", len(v.Invites), out)
	}
	return v.Invites[0]
}

func tryJoin(t *testing.T, f *fixture, tok, name string) int {
	t.Helper()
	resp, _ := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/invites/"+tok, `{"name":"`+name+`","password":"`+pw+`"}`, nil)
	return resp.StatusCode
}

// A link for a set number of people takes exactly that many, names each of
// them in the list, and then closes like a one-time link.
// route: POST /api/v1/users/invites
func TestInviteForSeveralPeople(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	tok := invite(t, f, own, `{"max_uses":2,"note":"family"}`)
	join(t, f, tok, "ana")
	v := listed(t, f, own)
	if v.MaxUses != 2 || v.Uses != 1 || v.UsedAt != nil || len(v.Joined) != 1 || v.Joined[0] != "ana" {
		t.Fatalf("after one join: %+v", v)
	}
	join(t, f, tok, "ben")
	v = listed(t, f, own)
	if v.Uses != 2 || v.UsedAt == nil || len(v.Joined) != 2 || v.Joined[1] != "ben" {
		t.Fatalf("after two joins: %+v", v)
	}
	if got := tryJoin(t, f, tok, "cat"); got != 404 {
		t.Fatalf("a full link took a third person: %d", got)
	}
}

// A link with no limit and no end date keeps working long after 30 days.
// route: POST /api/v1/auth/invites/{token}
func TestInviteThatNeverEnds(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	tok := invite(t, f, own, `{"max_uses":0,"days":0}`)
	join(t, f, tok, "ana")
	f.clock.add(400 * 24 * time.Hour)
	join(t, f, tok, "ben")
	join(t, f, tok, "cat")
	// The owner's session is long gone by now, so read the row itself.
	var uses, joined int
	var expires string
	var usedAt *string
	if err := f.s.Read().QueryRow(`SELECT uses, expires_at, used_at, (SELECT count(*) FROM invite_uses) FROM invites`).Scan(&uses, &expires, &usedAt, &joined); err != nil {
		t.Fatal(err)
	}
	if uses != 3 || joined != 3 || usedAt != nil || expires != noEnd {
		t.Fatalf("standing link: uses %d, joined %d, used_at %v, expires %s", uses, joined, usedAt, expires)
	}
}

// The list says a link has no end and no limit.
// route: GET /api/v1/users/invites
func TestInviteListSaysNoEnd(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	invite(t, f, own, `{"max_uses":0,"days":0}`)
	if v := listed(t, f, own); !v.NeverExpires || v.MaxUses != 0 || v.Uses != 0 || len(v.Joined) != 0 {
		t.Fatalf("standing link: %+v", v)
	}
}

// Taking back a link people joined through ends it and keeps their names;
// taking back one nobody used removes it.
// route: DELETE /api/v1/users/invites/{id}
func TestTakingBackAStandingInvite(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	tok := invite(t, f, own, `{"max_uses":0,"days":0}`)
	join(t, f, tok, "ana")
	v := listed(t, f, own)
	if resp, out := own.do(t, f, "DELETE", "/api/v1/users/invites/"+v.ID, ""); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("take back: %d %s", resp.StatusCode, out)
	}
	v = listed(t, f, own)
	if v.UsedAt == nil || len(v.Joined) != 1 || v.Joined[0] != "ana" {
		t.Fatalf("an ended link should stay listed with who joined: %+v", v)
	}
	if got := tryJoin(t, f, tok, "ben"); got != 404 {
		t.Fatalf("an ended link still worked: %d", got)
	}
	if resp, _ := own.do(t, f, "DELETE", "/api/v1/users/invites/"+v.ID, ""); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("an ended link was taken back twice: %d", resp.StatusCode)
	}

	unused := invite(t, f, own, `{"max_uses":5}`)
	resp, out := own.do(t, f, "GET", "/api/v1/users/invites", "")
	var all struct {
		Invites []InviteView `json:"invites"`
	}
	jsonOf(t, out, &all)
	if resp.StatusCode != 200 || len(all.Invites) != 2 {
		t.Fatalf("list: %d %s", resp.StatusCode, out)
	}
	id := all.Invites[0].ID
	if id == v.ID {
		id = all.Invites[1].ID
	}
	if resp, _ := own.do(t, f, "DELETE", "/api/v1/users/invites/"+id, ""); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("take back an unused link: %d", resp.StatusCode)
	}
	if got := listed(t, f, own); got.ID != v.ID {
		t.Fatalf("the unused link should be gone: %+v", got)
	}
	if got := tryJoin(t, f, unused, "cat"); got != 404 {
		t.Fatalf("a removed link still worked: %d", got)
	}
}

// The link is one person and 1 to 30 days unless asked otherwise, and a
// standing link never hands out the admin role.
// route: POST /api/v1/users/invites
func TestInviteUseAndEndRules(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	for body, want := range map[string]int{
		`{"max_uses":-1}`:                400,
		`{"max_uses":1001}`:              400,
		`{"role":"admin","max_uses":2}`:  400,
		`{"role":"admin","max_uses":0}`:  400,
		`{"role":"admin","days":0}`:      400,
		`{"role":"admin","max_uses":1}`:  201,
		`{"days":0}`:                     201,
		`{"days":-1}`:                    400,
		`{"max_uses":1000,"days":30}`:    201,
		`{"max_uses":0,"days":31}`:       400,
		`{"role":"member","max_uses":0}`: 201,
		`{"role":"member","days":30}`:    201,
	} {
		if resp, out := own.do(t, f, "POST", "/api/v1/users/invites", body); resp.StatusCode != want {
			t.Errorf("%s: %d, want %d (%s)", body, resp.StatusCode, want, out)
		}
	}
	// With nothing asked, a link still works once.
	tok := invite(t, f, own, `{}`)
	join(t, f, tok, "ana")
	if got := tryJoin(t, f, tok, "ben"); got != 404 {
		t.Fatalf("a plain link worked twice: %d", got)
	}
}
