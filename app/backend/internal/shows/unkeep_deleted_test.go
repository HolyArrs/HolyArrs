package shows

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

// With the switch on, an episode whose file is deleted from disk loses Keep
// up to date, as Sonarr's "unmonitor deleted episodes" does; the show and
// its other episodes keep theirs.
func TestRescanUnkeepsDeletedEpisode(t *testing.T) {
	svc, s, _, _ := addShowService(t)
	ctx := context.Background()
	lf := addShowFolder(t, s)
	showTree(t, lf.Path)
	res, err := svc.Adopt(ctx, lf.ID, []library.AdoptChoice{{Folder: "Lab Show (2025)", ID: "tmdb:100"}}, domain.System)
	if err != nil || res[0].Error != "" {
		t.Fatalf("adopt: %+v %v", res, err)
	}
	id := res[0].TitleID
	if !episodes(t, s, id)["S01E01"].Keep || !episodes(t, s, id)["S01E02"].Keep {
		t.Fatal("episodes should start kept up to date")
	}
	rules := importer.DefaultFileRules()
	rules.UnkeepDeletedEpisodes = true
	if err := importer.SaveFileRules(ctx, s, rules, domain.System); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(lf.Path, "Lab Show (2025)", "Season 1", "Lab.Show.S01E01.1080p.WEB-DL.H264-GRP.mkv")); err != nil {
		t.Fatal(err)
	}
	if r, err := svc.Rescan(ctx, domain.System); err != nil || r.Missing != 1 {
		t.Fatalf("rescan %+v %v", r, err)
	}
	eps := episodes(t, s, id)
	if eps["S01E01"].Keep || !eps["S01E02"].Keep {
		t.Fatalf("S01E01 keep %v, S01E02 keep %v", eps["S01E01"].Keep, eps["S01E02"].Keep)
	}
	var show bool
	_ = s.Read().QueryRow("SELECT keep_up_to_date FROM titles WHERE id = ?", id).Scan(&show)
	if !show {
		t.Fatal("the show lost Keep up to date")
	}
	evs, _ := s.RecentEvents(ctx, 5)
	found := false
	for _, e := range evs {
		found = found || e.Kind == domain.EventSeasonChanged && e.Summary == "Lab Show S01E01: Keep up to date off, its file was deleted from disk"
	}
	if !found {
		t.Fatalf("no episode event: %+v", evs)
	}
}
