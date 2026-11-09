package accounts

import (
	"context"
	"database/sql"
	"strconv"
	"testing"
	"time"
)

// fillFlood fills the table the way a made-up-name flood does: names up to
// their share, then address records past the real-name ceiling.
func fillFlood(a *Auth) {
	for i := 0; i < 3*maxTracked; i++ {
		a.noteFail("name:filler-" + strconv.Itoa(i))
	}
	for i := 0; i < 3*fullExtra; i++ {
		a.fails["ip:flood-"+strconv.Itoa(i)] = &failure{n: 1, last: a.now()}
	}
}

// Address records from a flood must not use up the room real names get once
// the table is full: the owner's name and a code ticket still get a record.
func TestFloodAddressesKeepRealNameRoom(t *testing.T) {
	f := setup(t, Options{})
	setupOwner(t, f, client(t))
	a := f.a
	a.mu.Lock()
	defer a.mu.Unlock()
	fillFlood(a)
	if len(a.fails) < 3*maxTracked+fullExtra {
		t.Fatalf("flood left only %d records", len(a.fails))
	}
	a.noteFail("ip:fresh-1", "name:luis", "2fa:7")
	if a.fails["name:luis"] == nil || a.fails["2fa:7"] == nil {
		t.Fatal("real name or code ticket got no record during a flood")
	}
}

// A Jellyfin account linked under another member name (or renamed) is still
// a real account while the table is full.
func TestFloodKeepsLinkedMediaName(t *testing.T) {
	f := setup(t, Options{})
	setupOwner(t, f, client(t))
	if err := f.s.WriteCore(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO user_links(user_id, provider, external_id, external_name, created_at)
			SELECT id, 'jellyfin', 'jf-1', 'Mum', '2026-01-01T00:00:00Z' FROM users WHERE name = 'luis'`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	a := f.a
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := 0; i < 3*maxTracked; i++ {
		a.noteFail("name:filler-" + strconv.Itoa(i))
	}
	a.noteFail("ip:fresh-1", "name:jellyfin:mum")
	if a.fails["name:jellyfin:mum"] == nil {
		t.Fatal("linked Jellyfin name got no record while the table was full")
	}
	if a.isRealName("name:emby:mum", f.clock.t) {
		t.Error("a Jellyfin link counted for Emby")
	}
}

// One typo a day at home must not build up into a wait for the right
// password: an address count whose wait ended long ago starts over.
func TestHomeTyposDoNotAccumulate(t *testing.T) {
	f := setup(t, Options{})
	setupOwner(t, f, client(t))
	a := f.a
	a.mu.Lock()
	defer a.mu.Unlock()
	for day := 0; day < 20; day++ {
		a.noteFail("ip:127.0.0.1")
		f.clock.add(25 * time.Hour)
	}
	if w := a.waitFor("ip:127.0.0.1", freeTries); w != 0 {
		t.Fatalf("home address waits %v after one typo a day", w)
	}
	if n := a.fails["ip:127.0.0.1"].n; n != 1 {
		t.Fatalf("home address count %d, want 1", n)
	}
	// A real account's count is still kept within accountKeep.
	a.fails["name:luis"] = &failure{n: 8, last: a.now()}
	f.clock.add(time.Hour)
	a.noteFail("name:luis")
	if n := a.fails["name:luis"].n; n != 9 {
		t.Fatalf("account count %d after an hour, want 9", n)
	}
}
