package sources

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// protectSlotCopy gives the Slot a protected file and returns its ID.
func protectSlotCopy(t *testing.T, f fixture, slotID, versionID int64) int64 {
	t.Helper()
	var fileID int64
	err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		fl := &mediafiles.File{Protected: true, SlotIDs: []int64{slotID}, VersionID: versionID, RelativePath: "protected.mkv",
			Size: 1, AddedAt: time.Now(), Languages: []string{}, MediaInfo: map[string]any{}, Origin: "imported", PlacedBy: "move", Role: "main", State: "present"}
		if _, err := database.Insert(tx, "files", fl, domain.System); err != nil {
			return err
		}
		fileID = fl.ID
		_, err := tx.Exec("UPDATE slots SET file_id = ?, state = ? WHERE id = ?", fl.ID, mediafiles.SlotBetterWanted, slotID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return fileID
}

func TestChoose_CannotForceProtectedCopy(t *testing.T) {
	f := seed(t, "http://127.0.0.1:1/api")
	var versionID int64
	if err := f.s.Read().QueryRow("SELECT version_id FROM slots WHERE id = ?", f.slotID).Scan(&versionID); err != nil {
		t.Fatal(err)
	}
	protectSlotCopy(t, f, f.slotID, versionID)
	releaseID := addRelease(t, f, "Dune.Part.Two.2024.2160p.WEB.H265-GROUP", "refused")

	_, err := (&Searcher{Store: f.s}).Choose(context.Background(), releaseID, f.slotID, true, domain.System)
	if !errors.Is(err, ErrBusy) || !strings.Contains(err.Error(), "protected") {
		t.Fatalf("forced protected choice: %v", err)
	}
	if got := slotState(t, f).State; got != mediafiles.SlotBetterWanted {
		t.Fatalf("slot moved to %s", got)
	}
}

func TestChoose_AutomaticRechecksProtectionInsideWrite(t *testing.T) {
	f := seed(t, "http://127.0.0.1:1/api")
	releaseID := addRelease(t, f, "Dune.Part.Two.2024.2160p.WEB.H265-GROUP", "judged")
	var versionID int64
	if err := f.s.Read().QueryRow("SELECT version_id FROM slots WHERE id = ?", f.slotID).Scan(&versionID); err != nil {
		t.Fatal(err)
	}
	protectSlotCopy(t, f, f.slotID, versionID)

	err := (&Searcher{Store: f.s}).settleChosen(context.Background(), f.slotID, releaseID, "new copy", domain.System)
	if !errors.Is(err, ErrBusy) || !strings.Contains(err.Error(), "protected") {
		t.Fatalf("stale automatic choice: %v", err)
	}
	var state string
	if err := f.s.Read().QueryRow("SELECT state FROM releases WHERE id = ?", releaseID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state == "chosen" {
		t.Fatal("protected Slot accepted the stale automatic choice")
	}
}

func TestChoose_MovieEditionConstrainsEachVersion(t *testing.T) {
	f := seed(t, "http://127.0.0.1:1/api")
	c, err := (&Searcher{Store: f.s}).loadSlot(context.Background(), f.slotID)
	if err != nil {
		t.Fatal(err)
	}
	all := []found{
		{src: domain.Source{ID: f.srcID}, item: Item{Title: "Dune.Part.Two.2024.Directors.Cut.1080p.WEB.H264-GROUP", Protocol: "torrent"}},
		{src: domain.Source{ID: f.srcID}, item: Item{Title: "Dune.Part.Two.2024.Theatrical.Cut.1080p.WEB.H264-GROUP", Protocol: "torrent"}},
		{src: domain.Source{ID: f.srcID}, item: Item{Title: "Dune.Part.Two.2024.1080p.WEB.H264-GROUP", Protocol: "torrent"}},
		{src: domain.Source{ID: f.srcID}, item: Item{Title: "Dune.Part.Two.2024.Directors.Cut.Extended.1080p.WEB.H264-GROUP", Protocol: "torrent"}},
	}
	c.ver.Edition = "Director's Cut"
	_, verdicts, err := (&Searcher{Store: f.s}).judgeAll(context.Background(), c, all, true, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if !verdicts[0].Passed || verdicts[1].Passed || verdicts[2].Passed || verdicts[3].Passed {
		t.Fatalf("director version verdicts: %+v", verdicts)
	}
	for i, word := range []string{"", "does not match", "unknown", "ambiguous"} {
		if word != "" && !verdictHas(verdicts[i], word) {
			t.Errorf("verdict %d lacks %q: %+v", i, word, verdicts[i].Reasons)
		}
	}

	c.ver.Edition = "Theatrical"
	_, verdicts, err = (&Searcher{Store: f.s}).judgeAll(context.Background(), c, all[:2], true, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if verdicts[0].Passed || !verdicts[1].Passed {
		t.Fatalf("theatrical version verdicts: %+v", verdicts)
	}
	c.ver.Edition = ""
	_, verdicts, err = (&Searcher{Store: f.s}).judgeAll(context.Background(), c, all[2:3], true, true, false)
	if err != nil || !verdicts[0].Passed {
		t.Fatalf("unconstrained version changed quality-only behavior: %+v %v", verdicts, err)
	}

	c.ver.Edition = "Directors Cut Extended"
	_, verdicts, err = (&Searcher{Store: f.s}).judgeAll(context.Background(), c, all[3:4], true, true, false)
	if err != nil || verdicts[0].Passed || !verdictHas(verdicts[0], "ambiguous") {
		t.Fatalf("joined ambiguous edition was accepted: %+v %v", verdicts, err)
	}
}

// verdictHas reports whether any reason detail contains the word.
func verdictHas(v mediafiles.Verdict, word string) bool {
	for _, reason := range v.Reasons {
		if strings.Contains(reason.Detail, word) {
			return true
		}
	}
	return false
}

func TestChoose_ShowPackLeavesProtectedEpisode(t *testing.T) {
	ts := &tvServer{answer: func(_ url.Values) []string { return []string{"Copper.Valley.S01.720p.HDTV.x264-OXBOW"} }}
	f := seedShow(t, ts.start(t), nil)
	addFile(t, f, f.slots["S01E02"], "protected-episode")
	exec(t, f.fixture, `UPDATE files SET protected = 1 WHERE id = (SELECT file_id FROM slots WHERE id = ?)`, f.slots["S01E02"])
	exec(t, f.fixture, `UPDATE slots SET state = ? WHERE id = ?`, mediafiles.SlotInLibrary, f.slots["S01E02"])
	sr := &Searcher{Store: f.s}
	runKind(t, f.fixture, sr, jobs.KindSearchSeason, &domain.Subject{Type: "part", ID: f.parts[1]}, map[string]any{"version_id": f.verID, "choose": false, "by_hand": true})
	pack := relByName(t, f.fixture, "Copper.Valley.S01.720p.HDTV.x264-OXBOW")

	choice, err := sr.Choose(context.Background(), pack.ID, 0, false, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if len(choice.SlotIDs) != 2 || !strings.Contains(strings.Join(choice.Warnings, " "), "protected") {
		t.Fatalf("choice did not leave out protected episode: %+v", choice)
	}
	if got := f.state(t, "S01E02"); got != mediafiles.SlotInLibrary {
		t.Fatalf("protected episode moved to %s", got)
	}
	var protected bool
	if err := f.s.Read().QueryRow(`SELECT f.protected FROM files f JOIN slots s ON s.file_id = f.id WHERE s.id = ?`, f.slots["S01E02"]).Scan(&protected); err != nil || !protected {
		t.Fatalf("protected file detached or changed: protected=%v err=%v", protected, err)
	}
}
