package library

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// TestSavingPickRulesRechecksGoodEnough: saving a Pick rules set re-checks
// the files of every title that uses it, the way a title edit does. Raising
// good enough makes a file want a better copy, lowering it (or clearing the
// points) puts the file back in the Library.
func TestSavingPickRulesRechecksGoodEnough(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	addFolder(t, s)
	svc, _, _ := runQueue(t, s, &metadata.Static{Movies: []metadata.Movie{dune}})
	id, err := AddMovie(ctx, s, dune, AddOptions{}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	sl := slotsOf(t, s, id)[0]
	err = s.Write(ctx, func(tx *sql.Tx) error {
		name := "Dune.Part.Two.2024.1080p.WEB-DL.DDP5.1.H.264-GRP"
		f := &mediafiles.File{SlotIDs: []int64{sl.ID}, VersionID: sl.VersionID, RelativePath: name + ".mkv", Size: 1, AddedAt: now,
			Languages: []string{}, MediaInfo: map[string]any{}, ReleaseName: &name, Origin: "imported", PlacedBy: "hardlink", Role: "main", State: "present"}
		if _, err := database.Insert(tx, "files", f, domain.System); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE slots SET state = 'in_library', file_id = ?, good_enough = 1 WHERE id = ?", f.ID, sl.ID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	in := PickRulesInput{Name: "Mine", Kinds: []string{"movie"}, Body: mediafiles.GoodEveryday()}
	p, err := CreatePickRules(ctx, s, in, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Edit(ctx, id, TitleChange{PickRulesID: &p.ID}, false, domain.System); err != nil {
		t.Fatal(err)
	}
	waitSlot := func(state mediafiles.SlotState, ge bool) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if got := slotsOf(t, s, id)[0]; got.State == state && got.GoodEnough == ge {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("slot %+v, want %s good enough %v", slotsOf(t, s, id)[0], state, ge)
	}
	save := func(b mediafiles.PickRulesBody) {
		t.Helper()
		in.Body = b
		p, err = UpdatePickRules(ctx, s, p.ID, in, domain.System)
		if err != nil {
			t.Fatal(err)
		}
		if err := svc.QueueRecheckPickRules(ctx, p, domain.System); err != nil {
			t.Fatal(err)
		}
	}
	waitSlot(mediafiles.SlotInLibrary, true)

	// Raise good enough to 4K web: the 1080p copy now wants a better one.
	higher := mediafiles.GoodEveryday()
	for i := range higher.Steps {
		if higher.Steps[i].Quality == "2160p_web" {
			higher.Steps[i].Allowed = true
		}
	}
	higher.GoodEnoughAt = "2160p_web"
	save(higher)
	waitSlot(mediafiles.SlotBetterWanted, false)

	// Lower it again: back in the Library.
	save(mediafiles.GoodEveryday())
	waitSlot(mediafiles.SlotInLibrary, true)

	// Good enough points out of reach (a TRaSH cutoff), then cleared.
	points := 10000
	far := mediafiles.GoodEveryday()
	far.GoodEnoughPoints = &points
	save(far)
	waitSlot(mediafiles.SlotBetterWanted, false)
	save(mediafiles.GoodEveryday())
	waitSlot(mediafiles.SlotInLibrary, true)
}
