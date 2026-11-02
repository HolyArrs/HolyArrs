package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/model"
	_ "modernc.org/sqlite"
)

const day = 24 * time.Hour

func setUsed(t *testing.T, s *Store, tok string, ago time.Duration, used int) {
	t.Helper()
	if _, err := s.registered.Exec("UPDATE registered SET last_used=?, used=? WHERE hash=?", time.Now().Add(-ago).Unix(), used, tokenHash(tok)); err != nil {
		t.Fatal(err)
	}
}

func active(s *Store, tok string) bool {
	_, ok, _ := s.Registered(context.Background(), tok, 90*day)
	return ok
}

func TestRegisteredCredentialsSeparateExpiringAndRevocable(t *testing.T) {
	ctx := context.Background()
	s, _ := openTest(t, Options{})
	if err := s.RegisterToken(ctx, "reg-one", "203.0.113.1", "203.0.113.0/24", 2, time.Hour, 30*day); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.Authorized(ctx, "reg-one"); ok {
		t.Fatal("registered credential mixed with operator credentials")
	}
	src, ok, err := s.Registered(ctx, "reg-one", time.Hour)
	if err != nil || !ok || src != "203.0.113.1" {
		t.Fatalf("registered: %q %v %v", src, ok, err)
	}
	setUsed(t, s, "reg-one", 2*time.Hour, 1)
	if _, ok, _ := s.Registered(ctx, "reg-one", time.Hour); ok {
		t.Fatal("idle credential accepted")
	}
	if err := s.RegisterToken(ctx, "reg-b", "203.0.113.2", "203.0.113.0/24", 2, time.Hour, 30*day); err != nil {
		t.Fatal(err)
	}
	if err := s.RevokeToken(ctx, "reg-b"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.Registered(ctx, "reg-b", time.Hour); ok {
		t.Fatal("revoked registered credential accepted")
	}
}

func TestFirstUseMarksUsed(t *testing.T) {
	ctx := context.Background()
	s, _ := openTest(t, Options{})
	if err := s.RegisterToken(ctx, "fresh", "198.51.100.1", "198.51.100.0/24", 10, 90*day, 30*day); err != nil {
		t.Fatal(err)
	}
	var used int
	s.registered.QueryRow("SELECT used FROM registered WHERE hash=?", tokenHash("fresh")).Scan(&used)
	if used != 0 {
		t.Fatal("new credential marked used")
	}
	active(s, "fresh")
	s.registered.QueryRow("SELECT used FROM registered WHERE hash=?", tokenHash("fresh")).Scan(&used)
	if used != 1 {
		t.Fatal("first use not recorded")
	}
}

// A flood from one network may only evict its own never-used credentials
// or credentials idle past the protection window; active installs stay.
func TestFloodNeverEvictsRecentlyUsedCredentials(t *testing.T) {
	ctx := context.Background()
	s, _ := openTest(t, Options{})
	reg := func(tok, network string) error {
		return s.RegisterToken(ctx, tok, network+"-src", network, 3, 90*day, 30*day)
	}
	// Three real installs: used 1 day, 10 days and 29 days ago.
	for i, tok := range []string{"real-1", "real-10", "real-29"} {
		if err := reg(tok, "home"); err != nil {
			t.Fatal(err)
		}
		setUsed(t, s, tok, []time.Duration{day, 10 * day, 29 * day}[i], 1)
	}
	// The table is full of protected credentials: the flood is refused.
	if err := reg("flood-1", "attacker"); !errors.Is(err, model.ErrRegistryFull) {
		t.Fatalf("full of protected: %v", err)
	}
	if active(s, "flood-1") {
		t.Fatal("refused credential stored")
	}
	// One real install goes quiet past the window: it may be replaced.
	setUsed(t, s, "real-29", 31*day, 1)
	if err := reg("flood-1", "attacker"); err != nil {
		t.Fatal(err)
	}
	// The flood may now only replace its own never-used credential.
	if err := reg("flood-2", "attacker"); err != nil {
		t.Fatal(err)
	}
	if active(s, "flood-1") || !active(s, "flood-2") {
		t.Fatal("flood did not recycle its own unused credential")
	}
	for _, tok := range []string{"real-1", "real-10"} {
		if !active(s, tok) {
			t.Fatalf("%s evicted by the flood", tok)
		}
	}
	// Another network's never-used, recent credential is protected too.
	if err := reg("other-1", "neighbour"); !errors.Is(err, model.ErrRegistryFull) {
		t.Fatalf("evicted another network's credential: %v", err)
	}
}

func TestRegisteredSchemaUpgradesFromV1(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "registered.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE registered (hash BLOB PRIMARY KEY CHECK(length(hash)=32), source TEXT NOT NULL, last_used INTEGER NOT NULL, revoked INTEGER NOT NULL CHECK(revoked IN (0,1))) WITHOUT ROWID; CREATE INDEX registered_used ON registered(revoked,last_used); PRAGMA user_version=1;`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO registered VALUES(?,?,?,0)", tokenHash("old"), "203.0.113.5", time.Now().Unix()); err != nil {
		t.Fatal(err)
	}
	db.Close()
	s, err := Open(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if !active(s, "old") {
		t.Fatal("v1 credential lost on upgrade")
	}
	var used int
	if err := s.registered.QueryRow("SELECT used FROM registered WHERE hash=?", tokenHash("old")).Scan(&used); err != nil || used != 1 {
		t.Fatalf("upgrade: used=%d %v", used, err)
	}
}
