package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// seedAlbum turns seed41's movie into an artist whose album Download covers
// three tracks of First Light (2020): track 2 is in the library, tracks 1
// and 3 need a decision.
func seedAlbum(t *testing.T) (*dl41, []int64) {
	t.Helper()
	x := seed41(t, mediafiles.SlotNeedsADecision, "finished")
	ids := []int64{x.slotID}
	err := x.s.Write(context.Background(), func(tx *sql.Tx) error {
		var titleID, verID, itemID int64
		if err := tx.QueryRow("SELECT i.title_id, s.version_id, s.item_id FROM slots s JOIN items i ON i.id = s.item_id WHERE s.id = ?", x.slotID).Scan(&titleID, &verID, &itemID); err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE titles SET kind = 'artist', name = 'Lab Band', year = NULL"); err != nil {
			return err
		}
		rd := time.Date(2020, 5, 1, 0, 0, 0, 0, time.UTC)
		p := &mediafiles.Part{TitleID: titleID, Kind: "album", Name: "First Light", ReleaseDate: &rd, KeepUpToDate: true, SourceIDs: map[string]string{},
			Pressings: []mediafiles.Pressing{}, SecondaryTypes: []string{}, Included: true}
		if _, err := database.Insert(tx, "parts", p, domain.System); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE items SET kind = 'track', part_id = ?, numbers = '{"disc":1,"track":1}', name = 'Dawn' WHERE id = ?`, p.ID, itemID); err != nil {
			return err
		}
		for _, n := range []int{2, 3} {
			st := map[int]mediafiles.SlotState{2: mediafiles.SlotInLibrary, 3: mediafiles.SlotNeedsADecision}[n]
			disc, num, name := 1, n, map[int]string{2: "Glass River", 3: "Night Train"}[n]
			it := &mediafiles.Item{TitleID: titleID, PartID: &p.ID, Kind: "track", Numbers: mediafiles.Numbers{Disc: &disc, Track: &num}, Name: &name, KeepUpToDate: true, SourceIDs: map[string]string{}}
			if _, err := database.Insert(tx, "items", it, domain.System); err != nil {
				return err
			}
			reason := "the file could not be placed: not enough free space for the whole album"
			sl := &mediafiles.Slot{ItemID: it.ID, VersionID: verID, State: st, LastReason: &reason}
			if _, err := database.Insert(tx, "slots", sl, domain.System); err != nil {
				return err
			}
			ids = append(ids, sl.ID)
		}
		_, err := tx.Exec("UPDATE downloads SET slot_ids = ?, progress = 1 WHERE id = ?", "["+itoa(ids[0])+","+itoa(ids[1])+","+itoa(ids[2])+"]", x.dlID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return x, ids
}

// 4.14.2: an album row names the artist and album and the tracks it
// covers, and says where each one is.
func TestDownloadsViewShowsAlbumTracks(t *testing.T) {
	t.Parallel()
	x, ids := seedAlbum(t)
	var out struct {
		Downloads []downloads.DownloadRow `json:"downloads"`
	}
	_ = json.Unmarshal(do(t, "GET", x.srv.URL+"/api/v1/downloads/view", "", 200), &out)
	if len(out.Downloads) != 1 {
		t.Fatalf("%+v", out)
	}
	r := out.Downloads[0]
	if r.Kind != "artist" || r.Title != "Lab Band" || r.Album != "First Light" || r.Year == nil || *r.Year != 2020 || r.Covers != "3 tracks" ||
		len(r.Episodes) != 3 || r.Episodes[1].Label != "Track 02" || r.Episodes[1].Name != "Glass River" || !r.Episodes[0].Track || r.Episodes[0].SlotID != ids[0] {
		t.Fatalf("row %+v", r)
	}
	if r.Phase != "needs_a_decision" || r.Text != "1 of 3 tracks in the library; Track 01, Track 03 need a decision." {
		t.Fatalf("phase %s text %q", r.Phase, r.Text)
	}
}

// 4.14.2: stuck tracks list their track and album download; a choice is
// made for one track or for every stuck track of the album.
func TestNeedsDecisionAlbumTracks(t *testing.T) {
	t.Parallel()
	x, ids := seedAlbum(t)
	var out struct {
		Items []downloads.DecisionView `json:"items"`
	}
	_ = json.Unmarshal(do(t, "GET", x.srv.URL+"/api/v1/needs-decision", "", 200), &out)
	if len(out.Items) != 2 {
		t.Fatalf("%+v", out)
	}
	for _, it := range out.Items {
		if it.Kind != "artist" || it.Album != "First Light" || it.Episode == nil || !it.Episode.Track || len(it.Pack) != 3 || len(it.PackSlotIDs) != 2 {
			t.Fatalf("item %+v", it)
		}
	}
	// Turn down for one track: the Release stays for the others.
	do(t, "POST", x.srv.URL+"/api/v1/needs-decision/"+itoa(ids[0])+"/decide", `{"action":"turn_down"}`, 202)
	var verdict string
	_ = x.s.Read().QueryRow("SELECT verdict FROM releases WHERE id = ?", x.relID).Scan(&verdict)
	if !strings.Contains(verdict, `"turned_down_slot_ids":[`+itoa(ids[0])+`]`) {
		t.Fatalf("verdict %s", verdict)
	}
	// Stop looking for the album's stuck tracks: each track stops, the artist goes on.
	exec41(t, x.s, "UPDATE slots SET state = 'needs_a_decision' WHERE id = ?", ids[0])
	b := string(do(t, "POST", x.srv.URL+"/api/v1/needs-decision/"+itoa(ids[2])+"/decide", `{"action":"stop","pack":true}`, 200))
	if !strings.Contains(b, `"stopped":2`) {
		t.Fatalf("stop %s", b)
	}
	var kept, verKept int
	_ = x.s.Read().QueryRow("SELECT count(*) FROM items WHERE keep_up_to_date = 1").Scan(&kept)
	_ = x.s.Read().QueryRow("SELECT keep_up_to_date FROM versions").Scan(&verKept)
	if kept != 1 || verKept != 1 {
		t.Fatalf("items kept %d, version kept %d", kept, verKept)
	}
}
