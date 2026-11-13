package migrate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

func TestSnapshotStore(t *testing.T) {
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	u, key := radarrAPI(t)
	snap := read(t, api(Radarr, u, key))
	saved, err := Save(ctx, st, 1, snap)
	if err != nil || saved.ID == 0 || saved.Kind != Radarr || saved.Via != ViaAPI || saved.Counts["titles"] != 2 || saved.Size == 0 {
		t.Fatalf("save %+v %v", saved, err)
	}
	var data string
	if err := st.Read().QueryRow(`SELECT data FROM migrate_snapshots WHERE id = ?`, saved.ID).Scan(&data); err != nil || strings.Contains(data, secretMark) {
		t.Fatalf("stored data holds a secret or is missing: %v", err)
	}
	list, err := List(ctx, st)
	if err != nil || len(list) != 1 || list[0].ID != saved.ID || list[0].AppVersion != "6.4.4" {
		t.Fatalf("list %+v %v", list, err)
	}
	got, s2, err := Get(ctx, st, saved.ID)
	if err != nil || got.OwnerID != 1 || len(s2.Titles) != 2 || s2.Titles[0].Name != snap.Titles[0].Name {
		t.Fatalf("get %+v %v", got, err)
	}
	if _, _, err := Get(ctx, st, 999); !errors.Is(err, ErrNoSnapshot) {
		t.Fatalf("missing: %v", err)
	}
	if err := Delete(ctx, st, saved.ID); err != nil {
		t.Fatal(err)
	}
	if err := Delete(ctx, st, saved.ID); !errors.Is(err, ErrNoSnapshot) {
		t.Fatalf("second delete: %v", err)
	}
	old := MaxSnapshot
	MaxSnapshot = 100
	defer func() { MaxSnapshot = old }()
	if _, err := Save(ctx, st, 1, snap); !errors.Is(err, ErrSnapshotTooBig) {
		t.Fatalf("over the cap: %v", err)
	}
	MaxSnapshot = old
	for i := 0; i < MaxSnapshots+3; i++ {
		if _, err := Save(ctx, st, 1, &Snapshot{Kind: Sonarr, Via: ViaBackup}); err != nil {
			t.Fatal(err)
		}
	}
	if list, _ := List(ctx, st); len(list) != MaxSnapshots {
		t.Fatalf("kept %d snapshots, want %d", len(list), MaxSnapshots)
	}
}

// A removed read's number is not given to a new read while an import names
// it, so the new read is never shown as imported by that import.
func TestSnapshotNumberNotReused(t *testing.T) {
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	first, err := Save(ctx, st, 1, &Snapshot{Kind: Sonarr, Via: ViaBackup})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("INSERT INTO moves (created_at, owner_id, snapshots, choices, state) VALUES ('2026-11-12T10:00:00Z', 1, ?, '{}', 'done')", fmt.Sprintf("[%d]", first.ID))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := Delete(ctx, st, first.ID); err != nil {
		t.Fatal(err)
	}
	next, err := Save(ctx, st, 1, &Snapshot{Kind: Sonarr, Via: ViaBackup})
	if err != nil || next.ID <= first.ID {
		t.Fatalf("new read %d (%v) reuses the number of %d", next.ID, err, first.ID)
	}
}
