package library

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

// TestHealServerPlaceholders: pictures stored before placeholders were
// refused (the metadata server's grey stand-in) are removed from the cache
// and from every title, so the real picture is fetched again; real pictures
// stay, and the clean-up runs once.
func TestHealServerPlaceholders(t *testing.T) {
	ctx := context.Background()
	svc, s, _, _ := musicService(t)
	dir := filepath.Join(t.TempDir(), "art")
	svc.Artwork = &ArtworkCache{Dir: dir}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	grey, err := os.ReadFile(filepath.Join("testdata", "server-placeholder.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	bad := ArtworkName("https://x/v1/img/artist/a/500.jpg")
	good := ArtworkName("https://x/v1/img/artist/b/500.jpg")
	for name, b := range map[string][]byte{bad: grey, good: []byte("\xff\xd8\xff real picture"), checkName(bad): []byte(`"e"`)} {
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o640); err != nil {
			t.Fatal(err)
		}
	}
	a := addBareArtist(t, svc)
	err = s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE titles SET artwork = ?, backdrop = ?, logo = ? WHERE id = ?",
			`["poster:`+bad+`","fanart:`+good+`"]`, "/api/v1/artwork/"+bad, "/api/v1/artwork/"+good, a)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	n, err := svc.HealServerPlaceholders(ctx)
	if err != nil || n != 1 {
		t.Fatalf("healed %d, %v; want 1", n, err)
	}
	if _, err := os.Stat(filepath.Join(dir, bad)); !os.IsNotExist(err) {
		t.Fatal("placeholder file kept")
	}
	if _, err := os.Stat(filepath.Join(dir, checkName(bad))); !os.IsNotExist(err) {
		t.Fatal("placeholder check file kept")
	}
	if _, err := os.Stat(filepath.Join(dir, good)); err != nil {
		t.Fatal("real picture removed")
	}
	ta := titleOf(t, s, a)
	if len(ta.Artwork) != 1 || ta.Artwork[0] != "fanart:"+good || ta.Backdrop != "" || ta.Logo != "/api/v1/artwork/"+good {
		t.Fatalf("artwork %v backdrop %q logo %q", ta.Artwork, ta.Backdrop, ta.Logo)
	}
	// Recorded: a second start does nothing, even with a new grey file.
	if err := os.WriteFile(filepath.Join(dir, bad), grey, 0o640); err != nil {
		t.Fatal(err)
	}
	if n, err := svc.HealServerPlaceholders(ctx); err != nil || n != 0 {
		t.Fatalf("second run healed %d, %v", n, err)
	}
}
