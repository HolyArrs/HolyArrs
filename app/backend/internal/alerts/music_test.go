package alerts

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// musicWorld writes Lab Lanterns with the album Small Hours (2025) of n
// tracks (one Slot each, in a main Version).
func musicWorld(t *testing.T, s *database.Store, n int) (titleID int64, slots []int64) {
	t.Helper()
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
		ti := &mediafiles.Title{Kind: "artist", GateDriver: "none", Name: "Lab Lanterns", SortName: "lab lanterns", MetadataSource: "musicbrainz", NewPartsPolicy: "all",
			ReleaseGate: "released", WhyAdded: "by_hand", Status: "running", State: "active", SourceIDs: map[string]string{"musicbrainz": "a0000000-0000-0000-0000-000000000009"},
			Aliases: []string{}, MusicTypes: []string{"album"}, MusicSecondary: []string{"studio"}, MusicStatuses: []string{"official"}, Artwork: []string{}}
		var err error
		if titleID, err = database.Insert(tx, "titles", ti, domain.System); err != nil {
			return err
		}
		var pr int64
		if err := tx.QueryRow("SELECT id FROM pick_rules LIMIT 1").Scan(&pr); err != nil {
			return err
		}
		res, err := tx.Exec(`INSERT INTO library_folders (path, kind, label, created_at, created_by, updated_at, updated_by) VALUES ('/music', 'music', 'Music', '', '{}', '', '{}')`)
		if err != nil {
			return err
		}
		lf, _ := res.LastInsertId()
		v := &mediafiles.Version{TitleID: titleID, Label: "Main", PickRulesID: pr, LibraryFolderID: lf, FolderName: "Lab Lanterns", KeepUpToDate: true, IsMain: true}
		if _, err := database.Insert(tx, "versions", v, domain.System); err != nil {
			return err
		}
		rel := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
		kind := "album"
		p := &mediafiles.Part{TitleID: titleID, Kind: "album", Name: "Small Hours", ReleaseType: &kind, ReleaseDate: &rel, KeepUpToDate: true, Included: true,
			SourceIDs: map[string]string{"musicbrainz": "10000000-0000-0000-0000-000000000009"}, Pressings: []mediafiles.Pressing{}, SecondaryTypes: []string{}}
		if _, err := database.Insert(tx, "parts", p, domain.System); err != nil {
			return err
		}
		for i := 1; i <= n; i++ {
			disc, num, nm := 1, i, fmt.Sprintf("Song %d", i)
			it := &mediafiles.Item{TitleID: titleID, PartID: &p.ID, Kind: "track", Numbers: mediafiles.Numbers{Disc: &disc, Track: &num}, Name: &nm, KeepUpToDate: true, SourceIDs: map[string]string{}}
			if _, err := database.Insert(tx, "items", it, domain.System); err != nil {
				return err
			}
			sl := &mediafiles.Slot{ItemID: it.ID, VersionID: v.ID, State: mediafiles.SlotDownloading}
			if _, err := database.Insert(tx, "slots", sl, domain.System); err != nil {
				return err
			}
			slots = append(slots, sl.ID)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return titleID, slots
}

func floats(ids []int64) []any {
	out := make([]any, len(ids))
	for i, id := range ids {
		out[i] = float64(id)
	}
	return out
}

// 4.15.5: an album import is one Alert naming the artist and album with its
// tracks; with pack_alerts each it is one Alert per track, at most
// each_limit, and the rest in one summary.
func TestAlbumImportAlerts(t *testing.T) {
	s, v := open(t)
	ctx := context.Background()
	snd := &Sender{Store: s, Vault: v, Backoff: []time.Duration{}}
	tid, slots := musicWorld(t, s, 4)
	dl := int64(3)
	imp := domain.Event{Kind: "import_done", TitleID: &tid, DownloadID: &dl, Summary: "Import done: 4 placed",
		Data: map[string]any{"slot_ids": floats(slots), "placed_slot_ids": floats(slots)}}
	ms := snd.MessagesFor(ctx, imp)
	if len(ms) != 1 || ms[0].Subject != "In the Library: Lab Lanterns – Small Hours (2025)" || ms[0].Artist == nil || len(ms[0].Tracks) != 4 ||
		!strings.Contains(ms[0].Text, "track 02 Song 2\ntrack 03 Song 3") || ms[0].Movie != nil || ms[0].Show != nil {
		t.Fatalf("album %+v", ms)
	}
	one := imp
	one.Data = map[string]any{"placed_slot_ids": floats(slots[1:2])}
	if m, _ := snd.MessageFor(ctx, one); m.Subject != "In the Library: Lab Lanterns – Small Hours (2025) track 02" {
		t.Fatalf("one track %q", m.Subject)
	}
	if err := SaveSettings(ctx, s, Settings{PackAlerts: PackEach, EachLimit: 3}, domain.System); err != nil {
		t.Fatal(err)
	}
	ms = snd.MessagesFor(ctx, imp)
	if len(ms) != 4 || ms[0].Subject != "In the Library: Lab Lanterns – Small Hours (2025) track 01" ||
		ms[3].Subject != "In the Library: Lab Lanterns – Small Hours (2025) track 04" || !strings.HasPrefix(ms[3].Text, "1 more tracks") {
		for _, m := range ms {
			t.Log(m.Subject, "|", m.Text)
		}
		t.Fatal("per track")
	}
	del := domain.Event{Kind: domain.EventEpisodesRemoved, TitleID: &tid, Summary: "Deleted Lab Lanterns – Small Hours from the Library",
		Data: map[string]any{"album": "Small Hours", "delete_files": true}}
	if m, ok := snd.MessageFor(ctx, del); !ok || m.Moment != MomentDelete || m.Subject != "Deleted: Lab Lanterns – Small Hours" {
		t.Fatalf("delete %+v", m)
	}
	if ms[0].Artist.MusicBrainz == "" || len(ms[0].Tracks) != 1 {
		t.Fatalf("artist ids %+v", ms[0])
	}
}
