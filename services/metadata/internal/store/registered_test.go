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
	_, ok, _ := s.Registered(context.Background(), tok, 90*day, "", true)
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
	src, ok, err := s.Registered(ctx, "reg-one", time.Hour, "", true)
	if err != nil || !ok || src != "203.0.113.1" {
		t.Fatalf("registered: %q %v %v", src, ok, err)
	}
	setUsed(t, s, "reg-one", 2*time.Hour, 1)
	if _, ok, _ := s.Registered(ctx, "reg-one", time.Hour, "", true); ok {
		t.Fatal("idle credential accepted")
	}
	if err := s.RegisterToken(ctx, "reg-b", "203.0.113.2", "203.0.113.0/24", 2, time.Hour, 30*day); err != nil {
		t.Fatal(err)
	}
	if err := s.RevokeToken(ctx, "reg-b"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.Registered(ctx, "reg-b", time.Hour, "", true); ok {
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

func storedCountry(t *testing.T, s *Store, tok string) (string, int64) {
	t.Helper()
	var c string
	var at int64
	if err := s.registered.QueryRow("SELECT country,country_at FROM registered WHERE hash=?", tokenHash(tok)).Scan(&c, &at); err != nil {
		t.Fatal(err)
	}
	return c, at
}

// The country is stored on first use and changes at most once a day; an
// unknown country keeps the stored one.
func TestRegisteredCountryStoredAtMostDaily(t *testing.T) {
	ctx := context.Background()
	s, _ := openTest(t, Options{})
	if err := s.RegisterToken(ctx, "home", "203.0.113.7", "203.0.113.0/24", 10, 90*day, 30*day); err != nil {
		t.Fatal(err)
	}
	s.Registered(ctx, "home", 90*day, "DE", true)
	if c, at := storedCountry(t, s, "home"); c != "DE" || at == 0 {
		t.Fatalf("first use: %q %d", c, at)
	}
	s.Registered(ctx, "home", 90*day, "FR", true)
	if c, _ := storedCountry(t, s, "home"); c != "DE" {
		t.Fatalf("changed within the day: %q", c)
	}
	s.registered.Exec("UPDATE registered SET country_at=? WHERE hash=?", time.Now().Add(-25*time.Hour).Unix(), tokenHash("home"))
	s.Registered(ctx, "home", 90*day, "", true)
	if c, _ := storedCountry(t, s, "home"); c != "DE" {
		t.Fatalf("unknown country replaced the stored one: %q", c)
	}
	s.Registered(ctx, "home", 90*day, "FR", true)
	if c, _ := storedCountry(t, s, "home"); c != "FR" {
		t.Fatalf("not refreshed after a day: %q", c)
	}
}

// Switched off, the install's country is cleared and stays empty; switched
// on again, it is stored at once.
func TestRegisteredCountryOffClears(t *testing.T) {
	ctx := context.Background()
	s, _ := openTest(t, Options{})
	if err := s.RegisterToken(ctx, "home", "203.0.113.7", "203.0.113.0/24", 10, 90*day, 30*day); err != nil {
		t.Fatal(err)
	}
	s.Registered(ctx, "home", 90*day, "DE", true)
	if _, ok, err := s.Registered(ctx, "home", 90*day, "DE", false); !ok || err != nil {
		t.Fatalf("off refused the credential: %v %v", ok, err)
	}
	if c, at := storedCountry(t, s, "home"); c != "" || at != 0 {
		t.Fatalf("off kept the country: %q %d", c, at)
	}
	if n, _ := s.InstallCountries(ctx, time.Now().Add(-30*day)); len(n) != 0 {
		t.Fatalf("switched-off install counted: %v", n)
	}
	s.Registered(ctx, "home", 90*day, "NL", true)
	if c, _ := storedCountry(t, s, "home"); c != "NL" {
		t.Fatalf("on again: %q", c)
	}
}

// The count covers installs used in the window, never revoked or
// countryless ones, and returns counts only.
func TestInstallCountriesCountsRecentInstalls(t *testing.T) {
	ctx := context.Background()
	s, _ := openTest(t, Options{})
	for _, c := range []struct {
		tok, country string
		ago          time.Duration
	}{{"de-now", "DE", 0}, {"de-old", "DE", 31 * day}, {"fr-10", "FR", 10 * day}, {"us-gone", "US", 0}, {"none", "", 0}} {
		if err := s.RegisterToken(ctx, c.tok, "203.0.113.8", "203.0.113.0/24", 10, 90*day, 30*day); err != nil {
			t.Fatal(err)
		}
		s.Registered(ctx, c.tok, 90*day, c.country, true)
		setUsed(t, s, c.tok, c.ago, 1)
	}
	if err := s.RevokeToken(ctx, "us-gone"); err != nil {
		t.Fatal(err)
	}
	got, err := s.InstallCountries(ctx, time.Now().Add(-30*day))
	if err != nil || len(got) != 2 || got["DE"] != 1 || got["FR"] != 1 {
		t.Fatalf("counts %v %v", got, err)
	}
}

func TestRegisteredSchemaUpgradesFromV2(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "registered.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE registered (hash BLOB PRIMARY KEY CHECK(length(hash)=32), source TEXT NOT NULL, last_used INTEGER NOT NULL, revoked INTEGER NOT NULL CHECK(revoked IN (0,1)), network TEXT NOT NULL DEFAULT '', used INTEGER NOT NULL DEFAULT 1 CHECK(used IN (0,1))) WITHOUT ROWID; CREATE INDEX registered_used ON registered(revoked,last_used); PRAGMA user_version=2;`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO registered VALUES(?,?,?,0,?,1)", tokenHash("old"), "203.0.113.5", time.Now().Unix(), "203.0.113.0/24"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	s, err := Open(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, ok, _ := s.Registered(context.Background(), "old", 90*day, "SE", true); !ok {
		t.Fatal("v2 credential lost on upgrade")
	}
	if c, _ := storedCountry(t, s, "old"); c != "SE" {
		t.Fatalf("upgraded row country %q", c)
	}
}
