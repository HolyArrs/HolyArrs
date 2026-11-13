package shows

import (
	"context"
	"database/sql"
	"testing"
)

// A Release chosen for an episode is not a download until a Downloader takes
// it, so the season shows the episode as missing, never as downloading.
func TestSeasonsCountChosenAsMissingNotDownloading(t *testing.T) {
	s, ti := t10SeedLab(t)
	ctx := context.Background()
	eps := episodes(t, s, ti.ID)
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE slots SET state = 'chosen' WHERE id = ?", eps["S01E01"].SlotID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	before, err := Seasons(ctx, s.Read(), ti.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	if before[1].Downloading != 0 {
		t.Fatalf("a chosen episode counted as downloading: %+v", before[1])
	}
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE slots SET state = 'wanted' WHERE id = ?", eps["S01E01"].SlotID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	wanted, _ := Seasons(ctx, s.Read(), ti.ID, now)
	if before[1].Missing != wanted[1].Missing {
		t.Fatalf("chosen %d missing, wanted %d missing", before[1].Missing, wanted[1].Missing)
	}
}
