package downloads

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// seedPack turns seed41's movie into a show whose pack Download covers
// S01E01 to S01E03: E02 is in the library, E01 and E03 need a decision.
func seedPack(t *testing.T) (*dl41, []int64) {
	t.Helper()
	x := seed41(t, mediafiles.SlotNeedsADecision, "finished")
	ids := []int64{x.slotID}
	err := x.s.Write(context.Background(), func(tx *sql.Tx) error {
		var titleID, verID, itemID int64
		if err := tx.QueryRow("SELECT i.title_id, s.version_id, s.item_id FROM slots s JOIN items i ON i.id = s.item_id WHERE s.id = ?", x.slotID).Scan(&titleID, &verID, &itemID); err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE titles SET kind = 'show', name = 'Copper Valley'"); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE items SET kind = 'episode', numbers = '{"season":1,"episode":1}', name = 'The Mill' WHERE id = ?`, itemID); err != nil {
			return err
		}
		// In episode order: ids[1] is episode 2 and ids[2] episode 3.
		for _, n := range []int{2, 3} {
			st := map[int]mediafiles.SlotState{2: mediafiles.SlotInLibrary, 3: mediafiles.SlotNeedsADecision}[n]
			season, ep, name := 1, n, map[int]string{2: "Flood", 3: "Ash"}[n]
			it := &mediafiles.Item{TitleID: titleID, Kind: "episode", Numbers: mediafiles.Numbers{Season: &season, Episode: &ep}, Name: &name, KeepUpToDate: true, SourceIDs: map[string]string{}}
			if _, err := database.Insert(tx, "items", it, domain.System); err != nil {
				return err
			}
			reason := "the file could not be placed: a file HolyArrs did not place is already at the target"
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

// 4.2.3: a pack row names the show and the episodes it covers, and says
// where each one is.
func TestDownloadsViewShowsEpisodes(t *testing.T) {
	t.Parallel()
	x, ids := seedPack(t)
	var out struct {
		Downloads []DownloadRow `json:"downloads"`
	}
	_ = json.Unmarshal(do(t, "GET", x.srv.URL+"/api/v1/downloads/view", "", 200), &out)
	if len(out.Downloads) != 1 {
		t.Fatalf("%+v", out)
	}
	r := out.Downloads[0]
	if r.Kind != "show" || r.Title != "Copper Valley" || r.Covers != "S01E01-E03, 3 episodes" || len(r.Episodes) != 3 || r.Episodes[1].Label != "S01E02" ||
		r.Episodes[1].Name != "Flood" || r.Episodes[0].SlotID != ids[0] {
		t.Fatalf("row %+v", r)
	}
	if r.Phase != "needs_a_decision" || r.Text != "1 of 3 episodes in the library; S01E01, S01E03 need a decision." {
		t.Fatalf("phase %s text %q", r.Phase, r.Text)
	}
	exec41(t, x.s, "UPDATE slots SET state = 'in_library'")
	_ = json.Unmarshal(do(t, "GET", x.srv.URL+"/api/v1/downloads/view", "", 200), &out)
	if r := out.Downloads[0]; r.Phase != "done" || r.Text != "3 of 3 episodes in the library." {
		t.Fatalf("done row %s %q", r.Phase, r.Text)
	}
	// One episode reads as its name.
	exec41(t, x.s, "UPDATE downloads SET slot_ids = ?", "["+itoa(ids[1])+"]")
	_ = json.Unmarshal(do(t, "GET", x.srv.URL+"/api/v1/downloads/view", "", 200), &out)
	if r := out.Downloads[0]; r.Covers != "S01E02 Flood" || r.Text != "In the library." {
		t.Fatalf("one episode %q %q", r.Covers, r.Text)
	}
}

// 4.2.4: stuck episodes list their episode and pack; a choice is made for
// one episode or for every stuck episode of the pack.
func TestNeedsDecisionEpisodesAndPacks(t *testing.T) {
	t.Parallel()
	x, ids := seedPack(t)
	var out struct {
		Items []DecisionView `json:"items"`
	}
	_ = json.Unmarshal(do(t, "GET", x.srv.URL+"/api/v1/needs-decision", "", 200), &out)
	if len(out.Items) != 2 {
		t.Fatalf("%+v", out)
	}
	for _, it := range out.Items {
		if it.SlotID == ids[2] && it.Why != "conflict" {
			t.Fatalf("a file that could not be placed is a conflict: %+v", it)
		}
		if it.Kind != "show" || it.Episode == nil || len(it.Pack) != 3 || len(it.PackSlotIDs) != 2 || it.DownloadID == nil || *it.DownloadID != x.dlID {
			t.Fatalf("item %+v", it)
		}
	}
	// Turn down for one episode: the Release stays for the others.
	do(t, "POST", x.srv.URL+"/api/v1/needs-decision/"+itoa(ids[0])+"/decide", `{"action":"turn_down"}`, 202)
	var verdict string
	_ = x.s.Read().QueryRow("SELECT verdict FROM releases WHERE id = ?", x.relID).Scan(&verdict)
	if !strings.Contains(verdict, `"turned_down_slot_ids":[`+itoa(ids[0])+`]`) {
		t.Fatalf("verdict %s", verdict)
	}
	// A movie is not a pack.
	y := seed41(t, mediafiles.SlotNeedsADecision, "failed")
	do(t, "POST", y.srv.URL+"/api/v1/needs-decision/"+itoa(y.slotID)+"/decide", `{"action":"stop","pack":true}`, 400)
	// Stop looking for the pack: every stuck episode stops, the show goes on.
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
	for _, id := range []int64{ids[0], ids[2]} {
		var st string
		_ = x.s.Read().QueryRow("SELECT state FROM slots WHERE id = ?", id).Scan(&st)
		if st != "wanted" {
			t.Fatalf("slot %d %s", id, st)
		}
	}
}
