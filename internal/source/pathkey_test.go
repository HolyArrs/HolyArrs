package source

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/secrets"
	"github.com/CoderLuii/holyarrs/internal/store"
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
