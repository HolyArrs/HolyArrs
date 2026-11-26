package shows

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// TestAdoptReadsShowOriginalLanguage: an adopted episode file is judged
// with the show's original language, so with the imported "Not Original"
// format in the Pick rules an English file of an English show is good
// enough and an English file of a Japanese show is not.
func TestAdoptReadsShowOriginalLanguage(t *testing.T) {
	mediafiles.SetOwnTraits([]mediafiles.OwnTrait{{Meta: database.Meta{ID: 1}, Name: "Not Original", Source: "import", Specs: []mediafiles.OwnSpec{
		{Name: "Not Original", Kind: "language", Value: "!original", Required: true},
		{Name: "Original", Kind: "language", Value: "original", Negate: true, Required: true},
	}}})
	t.Cleanup(func() { mediafiles.SetOwnTraits(nil) })
	for lang, want := range map[string]bool{"en": true, "ja": false} {
		svc, s, tmdb, _ := addShowService(t)
		ctx := context.Background()
		sh := labShow("tmdb", "100")
		sh.OriginalLanguage = lang
		tmdb.set(sh, "tmdb:100", "tvdb:200", "imdb:tt0100")
		zero := 0
		err := s.Write(ctx, func(tx *sql.Tx) error {
			all, err := database.Select[mediafiles.PickRules](ctx, tx, "pick_rules", "1 = 1")
			if err != nil {
				return err
			}
			for _, pr := range all {
				pr.Body.Traits = append(pr.Body.Traits, mediafiles.TraitPoints{Trait: mediafiles.OwnTraitKey(1), Points: -999999})
				pr.Body.GoodEnoughPoints = &zero
				if err := database.Update(tx, "pick_rules", &pr, domain.System); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		lf := addShowFolder(t, s)
		writeFile(t, filepath.Join(lf.Path, "Lab Show (2025)", "Season 1", "Lab Show - S01E01 - Pilot (1080p AMZN WEB-DL H264 DDP 5.1 English - HONE).mkv"), strings.Repeat("a", 300))
		res, err := svc.Adopt(ctx, lf.ID, []library.AdoptChoice{{Folder: "Lab Show (2025)", ID: "tmdb:100"}}, domain.System)
		if err != nil || len(res) != 1 || res[0].Error != "" {
			t.Fatalf("%s: adopt %+v %v", lang, res, err)
		}
		var sl mediafiles.Slot
		if err := database.Get(ctx, s.Read(), "slots", episodes(t, s, res[0].TitleID)["S01E01"].SlotID, &sl); err != nil {
			t.Fatal(err)
		}
		if sl.GoodEnough != want {
			t.Errorf("%s-original show: English file good enough %v, want %v", lang, sl.GoodEnough, want)
		}
	}
}
