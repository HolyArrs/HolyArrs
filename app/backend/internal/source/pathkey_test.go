package source

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/store"
)

const tlPasskey = "0f3a9c2b7d1e4f5a8b6c0d9e2f1a3b4c"

func storedURL(t *testing.T, s *store.Store, id int64) string {
	t.Helper()
	var u string
	if err := s.Read().QueryRow("SELECT url FROM sources WHERE id = ?", id).Scan(&u); err != nil {
		t.Fatal(err)
	}
	return u
}

func TestLooksSecret(t *testing.T) {
	for _, p := range []string{tlPasskey, "aZ3kQ9xW2mN7pL4vB8cR1tY6", "123e4567-e89b-12d3-a456-426614174000"} {
		if !looksSecret(p) {
			t.Errorf("%q should look like a passkey", p)
		}
	}
	for _, p := range []string{"rss", "movies", "new-movies-2024-hd-collection", "abcdefghijklmnopqrstuvwxyz", "12345678901234567890123", "short1a", "feed.php?x=1", ""} {
		if looksSecret(p) {
			t.Errorf("%q should not look like a passkey", p)
		}
	}
}

// A passkey in an rss address path goes to the vault: the stored address
// has a marker, the API sees it masked, and ConfigFor gives the real one.
func TestPathPasskeyKeptInTheVault(t *testing.T) {
	ctx := context.Background()
	s, v := openVault(t)
	real := "https://rss24h.torrentleech.org/" + tlPasskey
	src, err := Add(ctx, s, v, Input{Name: "TL", Format: "rss", URL: real}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if got := storedURL(t, s, src.ID); strings.Contains(got, tlPasskey) || src.PathSecretRef == nil {
		t.Fatalf("passkey stored in the clear: %q ref %v", got, src.PathSecretRef)
	}
	if got := MaskURL(src.URL); got != "https://rss24h.torrentleech.org/"+secrets.Masked {
		t.Fatalf("masked %q", got)
	}
	cfg, err := ConfigFor(ctx, src, v)
	if err != nil || cfg.URL != real {
		t.Fatalf("config %q %v", cfg.URL, err)
	}
	// Download links carrying the passkey are stored without it and put back on a grab.
	link := "https://www.torrentleech.org/rss/download/123/" + tlPasskey + "/Some.Show.torrent"
	clean := cleanLink(link, secretValues(cfg))
	if strings.Contains(clean, tlPasskey) {
		t.Fatalf("link kept the passkey: %q", clean)
	}
	back, err := GrabLink(ctx, s, v, domain.Release{SourceID: src.ID, LinkRef: clean})
	if err != nil || back != link {
		t.Fatalf("grab link %q %v", back, err)
	}
	if msg := New(cfg).scrub("Get " + real + ": refused"); strings.Contains(msg, tlPasskey) {
		t.Fatalf("message kept the passkey: %q", msg)
	}
	// Remove drops the path secret too.
	ref := *src.PathSecretRef
	if err := Remove(ctx, s, src.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Get(ctx, ref); err == nil {
		t.Fatal("path secret left behind")
	}
}

func TestOrdinaryRSSPathUntouched(t *testing.T) {
	ctx := context.Background()
	s, v := openVault(t)
	for i, u := range []string{"https://feed.example/rss/movies", "https://feed.example/rss/new-movies-2024-hd-collection/", "https://feed.example"} {
		src, err := Add(ctx, s, v, Input{Name: "F" + string(rune('a'+i)), Format: "rss", URL: u}, domain.System)
		if err != nil {
			t.Fatal(err)
		}
		if got := storedURL(t, s, src.ID); got != u || src.PathSecretRef != nil || MaskURL(got) != u {
			t.Fatalf("%q stored as %q ref %v", u, got, src.PathSecretRef)
		}
		cfg, err := ConfigFor(ctx, src, v)
		if err != nil || cfg.URL != u {
			t.Fatalf("config %q %v", cfg.URL, err)
		}
	}
}

// Sending the masked address back keeps the passkey; on another host it
// must be typed again; a new address without one drops it.
func TestPathPasskeyRoundTripThroughChange(t *testing.T) {
	ctx := context.Background()
	s, v := openVault(t)
	real := "https://rss24h.torrentleech.org/" + tlPasskey
	src, err := Add(ctx, s, v, Input{Name: "TL", Format: "rss", URL: real + "?extra=1"}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	ref := *src.PathSecretRef
	got, err := Change(ctx, s, v, src.ID, Input{Name: "TL 2", URL: MaskURL(src.URL)}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if got.PathSecretRef == nil || *got.PathSecretRef != ref || strings.Contains(storedURL(t, s, src.ID), tlPasskey) {
		t.Fatalf("after change %+v", got)
	}
	cfg, err := ConfigFor(ctx, got, v)
	if err != nil || cfg.URL != real || cfg.APIKey != "extra=1" {
		t.Fatalf("config %q %q %v", cfg.URL, cfg.APIKey, err)
	}
	if _, err := Change(ctx, s, v, src.ID, Input{Name: "TL 2", URL: "https://evil.example/" + secrets.Masked}, domain.System); !errors.Is(err, ErrPathKey) {
		t.Fatalf("masked passkey on a new host: %v", err)
	}
	got, err = Change(ctx, s, v, src.ID, Input{Name: "TL 2", URL: "https://rss24h.torrentleech.org/rss"}, domain.System)
	if err != nil || got.PathSecretRef != nil {
		t.Fatalf("plain address %+v %v", got, err)
	}
	if _, err := v.Get(ctx, ref); err == nil {
		t.Fatal("old path secret left behind")
	}
}

// A marker typed into the address is broken, so the passkey is never
// filled into a part of the address the person chose.
func TestPlantedPathMarkerIsNotFilled(t *testing.T) {
	ctx := context.Background()
	s, v := openVault(t)
	src, err := Add(ctx, s, v, Input{Name: "P", Format: "rss", URL: "https://feed.example/" + pathMark + "0/" + tlPasskey}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := ConfigFor(ctx, src, v)
	if err != nil || cfg.URL != "https://feed.example/HOLYARRS-PATHKEY0/"+tlPasskey {
		t.Fatalf("config %q %v", cfg.URL, err)
	}
	// A stored marker with no path secret is never fetched as it is.
	src.PathSecretRef = nil
	if _, err := ConfigFor(ctx, src, v); err == nil {
		t.Fatal("a marker without its passkey should fail")
	}
}

// The startup step moves a passkey an older save left in the clear.
func TestHidePathKeysMovesOldRows(t *testing.T) {
	ctx := context.Background()
	s, v := openVault(t)
	real := "https://rss.torrentleech.org/" + tlPasskey
	src := &domain.Source{Name: "Old", Protocol: "torrent", Format: "rss", URL: real, UseFor: domain.SourceUse{Feed: true},
		Kinds: []string{"tv"}, Categories: map[string][]int{}, Priority: 25, State: "enabled", Extra: map[string]string{}}
	plain := &domain.Source{Name: "Plain", Protocol: "torrent", Format: "rss", URL: "https://feed.example/rss/movies", UseFor: domain.SourceUse{Feed: true},
		Kinds: []string{"tv"}, Categories: map[string][]int{}, Priority: 25, State: "enabled", Extra: map[string]string{}}
	err := s.Write(ctx, func(tx *sql.Tx) error {
		for _, x := range []*domain.Source{src, plain} {
			if _, err := store.Insert(tx, "sources", x, domain.System); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n, err := HidePathKeys(ctx, s, nil); n != 0 || err != nil {
		t.Fatalf("no vault: %d %v", n, err)
	}
	if n, err := HidePathKeys(ctx, s, v); n != 1 || err != nil {
		t.Fatalf("moved %d %v", n, err)
	}
	if n, err := HidePathKeys(ctx, s, v); n != 0 || err != nil {
		t.Fatalf("second run moved %d %v", n, err)
	}
	if got := storedURL(t, s, src.ID); strings.Contains(got, tlPasskey) {
		t.Fatalf("still in the clear: %q", got)
	}
	if got := storedURL(t, s, plain.ID); got != plain.URL {
		t.Fatalf("plain address changed: %q", got)
	}
	var moved domain.Source
	if err := store.Get(ctx, s.Read(), "sources", src.ID, &moved); err != nil {
		t.Fatal(err)
	}
	cfg, err := ConfigFor(ctx, moved, v)
	if err != nil || cfg.URL != real {
		t.Fatalf("config %q %v", cfg.URL, err)
	}
}

// Release links stored before the passkey moved keep it in the clear; the
// startup step takes it out and keeps the markers they already hold.
func TestHidePathKeysCleansOldReleaseLinks(t *testing.T) {
	ctx := context.Background()
	s, v := openVault(t)
	qkey := "q" + "key12345"
	src := &domain.Source{Name: "Old", Protocol: "torrent", Format: "rss", URL: "https://rss.torrentleech.org/" + tlPasskey, UseFor: domain.SourceUse{Feed: true},
		Kinds: []string{"tv"}, Categories: map[string][]int{}, Priority: 25, State: "enabled", Extra: map[string]string{}}
	withKey := "https://www.torrentleech.org/rss/download/1/" + tlPasskey + "/A.torrent?tp=" + qkey
	plainLink := "https://www.torrentleech.org/rss/download/2/x/B.torrent"
	var rels []*domain.Release
	err := s.Write(ctx, func(tx *sql.Tx) error {
		if err := putSecret(tx, v, &src.SecretRef, "source_api_key", "tp="+qkey, domain.System); err != nil {
			return err
		}
		if _, err := store.Insert(tx, "sources", src, domain.System); err != nil {
			return err
		}
		// As an older cleanLink stored them: the query key hidden, the path one not.
		for _, l := range []string{cleanLink(withKey, []string{qkey}), plainLink} {
			r := &domain.Release{SourceID: src.ID, Name: "R", LinkRef: l, Protocol: "torrent", Flags: []string{}, Parsed: map[string]any{},
				MatchedSlotIDs: []int64{}, FoundBy: "feed", State: "seen"}
			if _, err := store.Insert(tx, "releases", r, domain.System); err != nil {
				return err
			}
			rels = append(rels, r)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n, err := HidePathKeys(ctx, s, v); n != 1 || err != nil {
		t.Fatalf("moved %d %v", n, err)
	}
	var stored string
	if err := s.Read().QueryRow("SELECT link_ref FROM releases WHERE id = ?", rels[0].ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored, tlPasskey) || strings.Contains(stored, qkey) {
		t.Fatalf("release link kept a key: %q", stored)
	}
	back, err := GrabLink(ctx, s, v, domain.Release{SourceID: src.ID, LinkRef: stored})
	if err != nil || back != withKey {
		t.Fatalf("grab link %q %v", back, err)
	}
	if err := s.Read().QueryRow("SELECT link_ref FROM releases WHERE id = ?", rels[1].ID).Scan(&stored); err != nil || stored != plainLink {
		t.Fatalf("plain link changed: %q %v", stored, err)
	}
}

// A masked address that cannot keep its passkey says why, and leaves the
// stored passkey as it was.
func TestMaskedPathKeyErrorsSayWhy(t *testing.T) {
	ctx := context.Background()
	s, v := openVault(t)
	src, err := Add(ctx, s, v, Input{Name: "TL", Format: "rss", URL: "https://rss24h.torrentleech.org/" + tlPasskey}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	masked := MaskURL(src.URL)
	if _, err := Change(ctx, s, v, src.ID, Input{Name: "TL", Format: "torznab", URL: masked}, domain.System); !errors.Is(err, ErrPathKeyRSS) {
		t.Fatalf("other format: %v", err)
	}
	if _, err := Change(ctx, s, v, src.ID, Input{Name: "TL", URL: masked + "/" + secrets.Masked}, domain.System); !errors.Is(err, ErrPathKeyNone) {
		t.Fatalf("two masked parts, one passkey: %v", err)
	}
	plain, err := Add(ctx, s, v, Input{Name: "Plain", Format: "rss", URL: "https://feed.example/rss"}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Change(ctx, s, v, plain.ID, Input{Name: "Plain", URL: "https://feed.example/" + secrets.Masked}, domain.System); !errors.Is(err, ErrPathKeyNone) {
		t.Fatalf("no passkey stored: %v", err)
	}
	var now domain.Source
	if err := store.Get(ctx, s.Read(), "sources", src.ID, &now); err != nil {
		t.Fatal(err)
	}
	cfg, err := ConfigFor(ctx, now, v)
	if err != nil || cfg.URL != "https://rss24h.torrentleech.org/"+tlPasskey || now.Format != "rss" {
		t.Fatalf("after refused changes %q %q %v", now.Format, cfg.URL, err)
	}
}
