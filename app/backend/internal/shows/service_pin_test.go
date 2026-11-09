package shows

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// Pins today's show SQL (episode/season reads over items, slots, versions,
// parts, titles and downloads) before it moves into the catalog store.

func t10SeedLab(t *testing.T) (*database.Store, mediafiles.Title) {
	t.Helper()
	s := open(t)
	ti := seedShow(t, s, labShow("tmdb", "100"), "all")
	return s, ti
}

func TestT10PinSeasonsSummaryGolden(t *testing.T) {
	s, ti := t10SeedLab(t)
	ctx := context.Background()
	eps := episodes(t, s, ti.ID)
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		set := func(k string, state string) error {
			_, err := tx.Exec("UPDATE slots SET state = ? WHERE id = ?", state, eps[k].SlotID)
			return err
		}
		if err := set("S01E01", "in_library"); err != nil {
			return err
		}
		if err := set("S01E02", "better_wanted"); err != nil {
			return err
		}
		if err := set("S02E01", "downloading"); err != nil {
			return err
		}
		if err := set("S02E02", "failed"); err != nil {
			return err
		}
		// An episode with no slot for the main Version (NULL partner of the LEFT JOIN).
		if _, err := tx.Exec("DELETE FROM slots WHERE id = ?", eps["S02E03"].SlotID); err != nil {
			return err
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	got, err := Seasons(ctx, s.Read(), ti.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[2].Episodes != 3 || got[2].Aired != 1 || got[2].NextAirDate == nil {
		t.Fatalf("seasons %+v", got)
	}
	if got[1].InLibrary != 2 || got[1].BetterWanted != 1 || got[2].Downloading != 1 || got[2].Failed != 1 {
		t.Fatalf("counts %+v", got)
	}
	b, _ := json.MarshalIndent(got, "", "  ")
	t10Golden(t, "season_summary_pin.golden", b)
}

func TestT10PinSeasonsSummaryNoSeasons(t *testing.T) {
	s := open(t)
	got, err := Seasons(context.Background(), s.Read(), 424242, now)
	if err != nil || len(got) != 0 {
		t.Fatalf("got %+v err %v", got, err)
	}
}

func TestT10PinShowSeasonsToSearch(t *testing.T) {
	s, ti := t10SeedLab(t)
	ctx := context.Background()
	eps := episodes(t, s, ti.ID)
	// Every episode is keep+wanted or not_out_yet; make a few searchable states.
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		for _, k := range []string{"S01E01", "S01E02", "S02E01", "S02E02", "S02E03", "S00E01"} {
			if _, err := tx.Exec("UPDATE slots SET state='in_library' WHERE id = ?", eps[k].SlotID); err != nil {
				return err
			}
		}
		for k, st := range map[string]string{"S01E02": "failed", "S02E01": "stalled", "S02E02": "wanted"} {
			if _, err := tx.Exec("UPDATE slots SET state=? WHERE id = ?", st, eps[k].SlotID); err != nil {
				return err
			}
		}
		// S02E02 stops being kept up to date: it no longer counts, S02E01 still does.
		_, err := tx.Exec("UPDATE items SET keep_up_to_date = 0 WHERE id = ?", eps["S02E02"].ItemID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	got, err := ShowSeasonsToSearch(ctx, s.Read(), ti.ID)
	if err != nil || len(got) != 2 {
		t.Fatalf("got %+v err %v", got, err)
	}
	// Season 1 then season 2, both for the main Version.
	if got[0].VersionID != got[1].VersionID || got[0].PartID == got[1].PartID {
		t.Fatalf("got %+v", got)
	}
	// A show not kept up to date returns nothing.
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE titles SET keep_up_to_date = 0 WHERE id = ?", ti.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	got, err = ShowSeasonsToSearch(ctx, s.Read(), ti.ID)
	if err != nil || len(got) != 0 {
		t.Fatalf("not kept: %+v %v", got, err)
	}
	if got, err := ShowSeasonsToSearch(ctx, s.Read(), 99999); err != nil || len(got) != 0 {
		t.Fatalf("unknown show: %+v %v", got, err)
	}
}

func TestT10PinEpisodeLabel(t *testing.T) {
	s, ti := t10SeedLab(t)
	eps := episodes(t, s, ti.ID)
	ctx := context.Background()
	if got := library.EpisodeLabel(ctx, s.Read(), eps["S01E02"].SlotID); got != "S01E02" {
		t.Fatalf("label %q", got)
	}
	if got := library.EpisodeLabel(ctx, s.Read(), eps["S00E01"].SlotID); got != "S00E01" {
		t.Fatalf("specials label %q", got)
	}
	// No slot: both numbers read as NULL and print as zero.
	if got := library.EpisodeLabel(ctx, s.Read(), 424242); got != "S00E00" {
		t.Fatalf("unknown slot label %q", got)
	}
}

// Many shows: the episode gate query must touch only shows with an aired
// not_out_yet episode and leave removed shows alone.
func TestT10PinOpenEpisodeGatesManyShows(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	ids := []int64{}
	for i := 0; i < 12; i++ {
		sh := labShow("tmdb", fmt.Sprint(100+i))
		sh.Name = fmt.Sprintf("Lab Show %02d", i)
		sh.IDs["tvdb"] = fmt.Sprint(200 + i)
		sh.IDs["imdb"] = fmt.Sprintf("tt01%02d", i)
		ids = append(ids, seedShow(t, s, sh, "all").ID)
	}
	// Show 0 is removed; show 1 has an aired episode stuck in not_out_yet.
	eps1 := episodes(t, s, ids[1])
	eps0 := episodes(t, s, ids[0])
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec("UPDATE titles SET state='removed' WHERE id = ?", ids[0]); err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE slots SET state='not_out_yet' WHERE id IN (?, ?)", eps1["S01E01"].SlotID, eps0["S01E01"].SlotID); err != nil {
			return err
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	svc := showService(s, nil)
	svc.Now = func() time.Time { return now }
	n, err := svc.OpenEpisodeGates(ctx)
	if err != nil || n != 1 {
		t.Fatalf("opened %d err %v", n, err)
	}
	if st := episodes(t, s, ids[1])["S01E01"].State; st != mediafiles.SlotWanted {
		t.Fatalf("show 1 state %s", st)
	}
	if st := episodes(t, s, ids[0])["S01E01"].State; st != mediafiles.SlotNotOutYet {
		t.Fatalf("removed show state %s", st)
	}
}

// The delete preview counts active downloads through json_each(slot_ids)
// joined to slots and items, by scope.
func TestT10PinRemoveEpisodesCountsActiveDownloads(t *testing.T) {
	s, ti := t10SeedLab(t)
	ctx := context.Background()
	eps := episodes(t, s, ti.ID)
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		dr := domain.Downloader{Adapter: "sabnzbd", Name: "Lab", Protocol: "usenet", URL: "http://lab.invalid", PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok"}
		if _, err := database.Insert(tx, "downloaders", &dr, domain.System); err != nil {
			return err
		}
		for i, c := range []struct {
			state string
			slots []int64
		}{
			{"downloading", []int64{eps["S01E01"].SlotID, eps["S01E02"].SlotID}},
			{"paused", []int64{eps["S02E01"].SlotID}},
			{"finished", []int64{eps["S01E01"].SlotID}},
			{"downloading", []int64{}},
		} {
			d := domain.Download{DownloaderID: dr.ID, ExternalID: fmt.Sprint("x", i), SlotIDs: c.slots, State: c.state, FilesSeen: []domain.SeenFile{}, StartedBy: domain.System}
			if _, err := database.Insert(tx, "downloads", &d, domain.System); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	svc := showService(s, nil)
	season := 1
	p, err := svc.RemoveEpisodes(ctx, ti.ID, library.RemoveScope{Season: &season}, library.RemoveChoice{}, false, domain.System)
	if err != nil || p.ActiveDownloads != 1 {
		t.Fatalf("season 1: %d %v", p.ActiveDownloads, err)
	}
	p, err = svc.RemoveEpisodes(ctx, ti.ID, library.RemoveScope{EpisodeIDs: []int64{eps["S02E01"].ItemID}}, library.RemoveChoice{}, false, domain.System)
	if err != nil || p.ActiveDownloads != 1 {
		t.Fatalf("episode: %d %v", p.ActiveDownloads, err)
	}
	p, err = svc.RemoveEpisodes(ctx, ti.ID, library.RemoveScope{EpisodeIDs: []int64{eps["S02E02"].ItemID}}, library.RemoveChoice{}, false, domain.System)
	if err != nil || p.ActiveDownloads != 0 {
		t.Fatalf("untouched episode: %d %v", p.ActiveDownloads, err)
	}
	// Whole title: the delete.go variant of the same count.
	pw, err := svc.Remove(ctx, ti.ID, library.RemoveChoice{}, false, domain.System)
	if err != nil || pw.ActiveDownloads != 2 {
		t.Fatalf("whole show: %d %v", pw.ActiveDownloads, err)
	}
}

// A show with no seasons or episodes (provider gave none) keeps the reads empty.
func TestT10PinShowWithoutEpisodes(t *testing.T) {
	s := open(t)
	sh := labShow("tmdb", "100")
	sh.Seasons, sh.Episodes = nil, nil
	ti := seedShow(t, s, metadata.Show(sh), "all")
	ctx := context.Background()
	got, err := Seasons(ctx, s.Read(), ti.ID, now)
	if err != nil || len(got) != 0 {
		t.Fatalf("seasons %+v %v", got, err)
	}
	if got, err := ShowSeasonsToSearch(ctx, s.Read(), ti.ID); err != nil || len(got) != 0 {
		t.Fatalf("to search %+v %v", got, err)
	}
	svc := showService(s, nil)
	if n, err := svc.OpenEpisodeGates(ctx); err != nil || n != 0 {
		t.Fatalf("gates %d %v", n, err)
	}
}
