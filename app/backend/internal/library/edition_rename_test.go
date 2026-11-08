package library

import (
	"context"
	"database/sql"
	"path"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func TestRenameUsesCheckedEditionOnceAndKeepsVersionsSeparate(t *testing.T) {
	svc, s, lf, titleID := renameWorld(t)
	ctx := context.Background()

	main := putFile(t, s, lf, titleID, "main.mkv", "Dune.Part.Two.2024.Directors.Cut.1080p.BluRay-HOLYARRS", "imported")
	checkedMain := "Theatrical"
	if _, err := svc.SetFileChoice(ctx, titleID, main.ID, FileChoice{Edition: &checkedMain}, domain.System); err != nil {
		t.Fatal(err)
	}

	otherVersion, err := AddVersion(ctx, s, titleID, VersionInput{Label: "4K Director", PickRulesID: fourKRules(t, s), LibraryFolderID: lf.ID, Edition: "Director's Cut"}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	other := putFile(t, s, lf, titleID, "other.mkv", "Dune.Part.Two.2024.Theatrical.Cut.2160p.WEB-DL-HOLYARRS", "imported")
	otherSlots, err := database.Select[mediafiles.Slot](ctx, s.Read(), "slots", "version_id = ? ORDER BY id", otherVersion.ID)
	if err != nil || len(otherSlots) != 1 {
		t.Fatalf("other Version slots: %+v, %v", otherSlots, err)
	}
	other.VersionID = otherVersion.ID
	other.SlotIDs = []int64{otherSlots[0].ID}
	other.MediaInfo["step"] = "2160p_web"
	if err := s.Write(ctx, func(tx *sql.Tx) error { return database.Update(tx, "files", &other, domain.System) }); err != nil {
		t.Fatal(err)
	}
	checkedOther := "Director's Cut"
	if _, err := svc.SetFileChoice(ctx, titleID, other.ID, FileChoice{Edition: &checkedOther}, domain.System); err != nil {
		t.Fatal(err)
	}

	plan, err := PlanRename(ctx, s.Read(), []int64{titleID})
	if err != nil {
		t.Fatal(err)
	}
	steps := map[int64]RenameStep{}
	for _, step := range plan {
		steps[step.FileID] = step
	}
	wantMain := "Dune - Part Two (2024)/Dune - Part Two (2024) 1080p BluRay {edition-Theatrical}.mkv"
	wantOther := "Dune - Part Two (2024) [4K Director]/Dune - Part Two (2024) 2160p WEB {edition-Director's Cut}.mkv"
	if got := steps[main.ID]; got.To != wantMain || got.Skip != "" {
		t.Fatalf("checked edition did not override parsed release: %+v", got)
	}
	if got := steps[other.ID]; got.To != wantOther || got.Skip != "" {
		t.Fatalf("other Version destination: %+v", got)
	}
	for _, step := range []RenameStep{steps[main.ID], steps[other.ID]} {
		if strings.Count(path.Base(step.To), "{edition-") != 1 {
			t.Fatalf("edition tag count in %q", step.To)
		}
	}
	if path.Dir(steps[main.ID].To) == path.Dir(steps[other.ID].To) {
		t.Fatalf("Versions share a destination: %+v", plan)
	}

	if n, err := svc.ApplyRename(ctx, plan, 0, domain.System); err != nil || n != 2 {
		t.Fatalf("renamed %d: %v", n, err)
	}
	if got := fileRow(t, s, main.ID).RelativePath; got != wantMain {
		t.Fatalf("main stored path %q", got)
	}
	if got := fileRow(t, s, other.ID).RelativePath; got != wantOther {
		t.Fatalf("other stored path %q", got)
	}
	again, err := PlanRename(ctx, s.Read(), []int64{titleID})
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 0 {
		t.Fatalf("second rename was not a no-op: %+v", again)
	}
}

func TestRenameDoesNotInventTheatricalForOldUnknownEdition(t *testing.T) {
	_, s, lf, titleID := renameWorld(t)
	f := putFile(t, s, lf, titleID, "old.mkv", "Dune.Part.Two.2024.1080p.BluRay-HOLYARRS", "imported")

	plan, err := PlanRename(context.Background(), s.Read(), []int64{titleID})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan) != 1 || plan[0].FileID != f.ID {
		t.Fatalf("rename plan: %+v", plan)
	}
	if strings.Contains(plan[0].To, "{edition-") || strings.Contains(strings.ToLower(plan[0].To), "theatrical") {
		t.Fatalf("unknown edition was relabeled: %q", plan[0].To)
	}
}
