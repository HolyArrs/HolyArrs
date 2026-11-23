package accounts

import (
	"fmt"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// Only the owner makes a link with no end date or no limit. An admin can
// still make one-time links and counted links that end.
// route: POST /api/v1/users/invites
func TestOnlyOwnerMakesStandingInvites(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	ad := join(t, f, invite(t, f, own, `{"role":"admin"}`), "ana")
	for body, want := range map[string]int{
		`{"max_uses":0}`:              403,
		`{"days":0}`:                  403,
		`{"max_uses":0,"days":0}`:     403,
		`{"max_uses":5,"days":0}`:     403,
		`{}`:                          201,
		`{"max_uses":5,"days":7}`:     201,
		`{"max_uses":1000,"days":30}`: 201,
	} {
		if resp, out := ad.do(t, f, "POST", "/api/v1/users/invites", body); resp.StatusCode != want {
			t.Errorf("admin %s: %d, want %d (%s)", body, resp.StatusCode, want, out)
		}
	}
	if resp, out := own.do(t, f, "POST", "/api/v1/users/invites", `{"max_uses":0,"days":0}`); resp.StatusCode != 201 {
		t.Fatalf("owner standing link: %d %s", resp.StatusCode, out)
	}
}

// Many people racing for the last place on a link: exactly one gets in.
// route: POST /api/v1/auth/invites/{token}
func TestInviteLastPlaceRace(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	tok := invite(t, f, own, `{"max_uses":2}`)
	join(t, f, tok, "first")
	const racers = 8
	var wg sync.WaitGroup
	codes := make([]int, racers)
	for i := range racers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes[i] = tryJoin(t, f, tok, fmt.Sprintf("racer%d", i))
		}()
	}
	wg.Wait()
	in := 0
	for _, c := range codes {
		if c == 201 {
			in++
		}
	}
	if in != 1 {
		t.Fatalf("%d racers got the last place, want 1: %v", in, codes)
	}
	var uses, joined, users int
	if err := f.s.Read().QueryRow(`SELECT uses, (SELECT count(*) FROM invite_uses), (SELECT count(*) FROM users WHERE name LIKE 'racer%') FROM invites`).Scan(&uses, &joined, &users); err != nil {
		t.Fatal(err)
	}
	if uses != 2 || joined != 2 || users != 1 {
		t.Fatalf("after the race: uses %d, joined %d, racer accounts %d", uses, joined, users)
	}
}

// A counted link taken back after some joins stays shut; a later try to
// take it back again or join through it fails.
// route: DELETE /api/v1/users/invites/{id}
func TestInviteReuseAfterRevoke(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	tok := invite(t, f, own, `{"max_uses":5,"days":7}`)
	join(t, f, tok, "ana")
	v := listed(t, f, own)
	if resp, _ := own.do(t, f, "DELETE", "/api/v1/users/invites/"+v.ID, ""); resp.StatusCode != 204 {
		t.Fatalf("take back: %d", resp.StatusCode)
	}
	for _, name := range []string{"ben", "cat"} {
		if got := tryJoin(t, f, tok, name); got != 404 {
			t.Fatalf("%s joined a taken-back link: %d", name, got)
		}
	}
	if v = listed(t, f, own); v.Uses != 1 || v.UsedAt == nil {
		t.Fatalf("taken-back link: %+v", v)
	}
}

// A counted link past its end date takes no one, even with places left.
// route: POST /api/v1/auth/invites/{token}
func TestInviteCountedLinkExpires(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	tok := invite(t, f, own, `{"max_uses":5,"days":1}`)
	join(t, f, tok, "ana")
	f.clock.add(25 * time.Hour)
	if got := tryJoin(t, f, tok, "ben"); got != 404 {
		t.Fatalf("an expired link took someone: %d", got)
	}
}

// Join attempts through one link are limited per minute.
// route: POST /api/v1/auth/invites/{token}
func TestInviteJoinsLimitedPerLink(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	tok := invite(t, f, own, `{"max_uses":0,"days":0}`)
	for i := range inviteJoinsPer {
		join(t, f, tok, fmt.Sprintf("p%d", i))
	}
	if got := tryJoin(t, f, tok, "toomany"); got != 429 {
		t.Fatalf("join over the limit: %d, want 429", got)
	}
	f.clock.add(inviteJoinsWindow)
	join(t, f, tok, "later")
}

// The limiter keeps links apart and never grows past its cap.
func TestInviteJoinLimitBounded(t *testing.T) {
	var l inviteJoinLimit
	now := time.Unix(0, 0)
	for i := range inviteJoinsKept {
		if !l.allow(httptest.NewRecorder(), fmt.Sprint(i), now) {
			t.Fatalf("link %d refused", i)
		}
	}
	if l.allow(httptest.NewRecorder(), "one-more", now) {
		t.Fatal("a full table took a new link")
	}
	if !l.allow(httptest.NewRecorder(), "one-more", now.Add(inviteJoinsWindow)) || len(l.seen) != 1 {
		t.Fatalf("old windows were not cleared: %d kept", len(l.seen))
	}
}
