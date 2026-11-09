package sources

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// JSON golden of the Source and Release records (they move to
// sources/types.go). Both are stored and read back through the database, so
// the golden pins the column mapping and the field order, not only the
// struct. Row timestamps are zeroed: they come from the clock.
func TestPinSourceAndReleaseRecordGolden(t *testing.T) {
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	seeders, peers, minSeed := 40, 3, 5
	hash := "0123456789abcdef0123456789abcdef01234567"
	var srcID, relID int64
	err = s.Write(context.Background(), func(tx *sql.Tx) error {
		src := &domain.Source{Name: "Lab", Protocol: "torrent", Format: "torznab", URL: "http://idx.example/api",
			UseFor: domain.SourceUse{Feed: true, AutoSearch: true, HandSearch: true}, Kinds: []string{"movie", "tv"},
			Categories: map[string][]int{"movie": {2000}, "tv": {5000}}, MinSeeders: &minSeed, Priority: 10, State: "enabled",
			AllowLocal: true, Extra: map[string]string{"k": "v"}}
		if _, err := database.Insert(tx, "sources", src, domain.System); err != nil {
			return err
		}
		srcID = src.ID
		r := &domain.Release{SourceID: src.ID, Name: "Dune.Part.Two.2024.1080p.WEB-GRP", LinkRef: "http://idx.example/dl/1", Protocol: "torrent",
			Size: 123456789, PublishedAt: &at, Seeders: &seeders, Peers: &peers, InfoHash: &hash, Flags: []string{"freeleech"},
			Parsed: map[string]any{"title": "Dune Part Two", "year": float64(2024)}, MatchedSlotIDs: []int64{1, 2},
			Verdict: map[string]any{"outcome": "passed"}, FoundBy: "auto_search", State: "held", HoldUntil: &at}
		if _, err := database.Insert(tx, "releases", r, domain.System); err != nil {
			return err
		}
		relID = r.ID
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var src domain.Source
	var rel domain.Release
	if err := database.Get(context.Background(), s.Read(), "sources", srcID, &src); err != nil {
		t.Fatal(err)
	}
	if err := database.Get(context.Background(), s.Read(), "releases", relID, &rel); err != nil {
		t.Fatal(err)
	}
	src.CreatedAt, src.UpdatedAt, rel.CreatedAt, rel.UpdatedAt = time.Time{}, time.Time{}, time.Time{}, time.Time{}
	got, err := json.MarshalIndent(map[string]any{"source": src, "release": rel}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	path := filepath.Join("testdata", "pin_records.golden.json")
	if os.Getenv("PIN_UPDATE") == "1" {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.ReplaceAll(string(want), "\r\n", "\n") != string(got) {
		t.Fatalf("records changed:\n%s", got)
	}
}
