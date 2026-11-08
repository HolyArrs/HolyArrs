package sources

import (
	"context"
	"database/sql"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// slotPubs records the Slot ids that were published live.
type slotPubs struct {
	mu  sync.Mutex
	ids []int64
}

// TestGrabHeld_SlotMissingFromPickList checks that a held Slot whose Release lists other Slots is still chosen when its wait is over, not left held for ever.
func TestGrabHeld_SlotMissingFromPickList(t *testing.T) {
	f, rel := holdForFeed(t)
	exec(t, f, "UPDATE releases SET verdict = json_set(COALESCE(verdict, '{}'), '$.pick_slot_ids', json('[90001, 90002]')) WHERE id = ?", rel.ID)
	var chosen []int64
	sr := &Searcher{Store: f.s, OnChosen: func(_ context.Context, slotID, _ int64) { chosen = append(chosen, slotID) }}
	runKind(t, f, sr, jobs.KindGrabHeld, nil, nil)
	if sl := slotState(t, f); sl.State != mediafiles.SlotChosen || len(chosen) != 1 || chosen[0] != f.slotID {
		t.Fatalf("slot %s, chosen %v", sl.State, chosen)
	}
}

// TestSettleChosenMany_RefusesEmpty checks that an empty pick list is refused.
func TestSettleChosenMany_RefusesEmpty(t *testing.T) {
	f := seed(t, "http://unused/api")
	sr := &Searcher{Store: f.s}
	if err := sr.settleChosenMany(context.Background(), nil, 1, "x", domain.System); err == nil {
		t.Fatal("an empty pick list was settled")
	}
}

// TestSettleChosen_RecordsRelease checks that choosing a Release for one Slot records it on the Slot and as the Release's pick list.
func TestSettleChosen_RecordsRelease(t *testing.T) {
	f, rel := holdForFeed(t)
	sr := &Searcher{Store: f.s}
	runKind(t, f, sr, jobs.KindGrabHeld, nil, nil)
	var chosenFor sql.NullInt64
	_ = f.s.Read().QueryRow("SELECT chosen_release_id FROM slots WHERE id = ?", f.slotID).Scan(&chosenFor)
	if chosenFor.Int64 != rel.ID {
		t.Fatalf("chosen_release_id %v, want %d", chosenFor, rel.ID)
	}
	var picks string
	_ = f.s.Read().QueryRow("SELECT json_extract(verdict, '$.pick_slot_ids') FROM releases WHERE id = ?", rel.ID).Scan(&picks)
	if want := "[" + strconv.FormatInt(f.slotID, 10) + "]"; picks != want {
		t.Fatalf("pick_slot_ids %s, want %s", picks, want)
	}
}

// TestGrabHeld_PackTellsEverySlot checks that a season pack held for several Slots writes a chosen Event and a live update for every Slot when the wait is over.
func TestGrabHeld_PackTellsEverySlot(t *testing.T) {
	ts := &tvServer{answer: func(url.Values) []string { return nil }}
	f := seedShow(t, ts.start(t), nil)
	ctx := context.Background()
	ids := []int64{f.slots["S01E01"], f.slots["S01E02"]}
	var relID int64
	err := f.s.Write(ctx, func(tx *sql.Tx) error {
		rel := &domain.Release{SourceID: f.srcID, Name: "Copper.Valley.S01.1080p.WEB-DL.H.264-QUILL", LinkRef: "x", Protocol: "torrent",
			Flags: []string{}, Parsed: map[string]any{}, MatchedSlotIDs: ids, FoundBy: "feed", State: "held",
			Verdict: map[string]any{"pick_slot_ids": ids}}
		if _, err := database.Insert(tx, "releases", rel, domain.System); err != nil {
			return err
		}
		relID = rel.ID
		for _, id := range ids {
			if _, err := tx.Exec("UPDATE slots SET state = 'held', held_release_id = ?, next_try_at = ? WHERE id = ?", relID, database.FormatTime(time.Now().Add(-time.Minute)), id); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	p := &slotPubs{}
	sr := &Searcher{Store: f.s, Live: p}
	if n, err := sr.grabHeld(ctx, domain.System, nil, time.Now().UTC()); err != nil || n != 1 {
		t.Fatalf("grab held: %d %v", n, err)
	}
	for _, id := range ids {
		n := 0
		evs, _ := f.s.RecentEvents(ctx, 50)
		for _, e := range evs {
			if e.Kind == EventReleaseChosen && e.SlotID != nil && *e.SlotID == id {
				n++
			}
		}
		if n != 1 {
			t.Errorf("slot %d: %d chosen Events", id, n)
		}
		found := false
		p.mu.Lock()
		for _, x := range p.ids {
			found = found || x == id
		}
		p.mu.Unlock()
		if !found {
			t.Errorf("slot %d: no live update", id)
		}
	}
}

// TestHandPack_SkipsEpisodeThatWouldNotGain checks that a season pack chosen by hand leaves out an episode whose copy is already as good, with a warning that names it.
func TestHandPack_SkipsEpisodeThatWouldNotGain(t *testing.T) {
	ts := &tvServer{answer: func(url.Values) []string { return nil }}
	f := seedShow(t, ts.start(t), func(label string, sl *mediafiles.Slot) {
		if label == "S01E01" {
			sl.State = mediafiles.SlotInLibrary
		}
	})
	addFile(t, f, f.slots["S01E01"], "Copper.Valley.S01E01.1080p.WEB-DL.H.264-OTHER")
	ctx := context.Background()
	ids := []int64{f.slots["S01E01"], f.slots["S01E02"], f.slots["S01E03"]}
	var relID int64
	err := f.s.Write(ctx, func(tx *sql.Tx) error {
		rel := &domain.Release{SourceID: f.srcID, Name: "Copper.Valley.S01.720p.HDTV.x264-OXBOW", LinkRef: "x", Protocol: "torrent",
			Flags: []string{}, Parsed: map[string]any{}, MatchedSlotIDs: ids, FoundBy: "hand_search", State: "judged", Verdict: map[string]any{}}
		if _, err := database.Insert(tx, "releases", rel, domain.System); err != nil {
			return err
		}
		relID = rel.ID
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sr := &Searcher{Store: f.s}
	ch, err := sr.Choose(ctx, relID, 0, false, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if want := ids[1:]; !slices.Equal(ch.SlotIDs, want) {
		t.Fatalf("chosen for %v, want %v", ch.SlotIDs, want)
	}
	if f.state(t, "S01E01") != mediafiles.SlotInLibrary {
		t.Fatalf("S01E01 %s", f.state(t, "S01E01"))
	}
	if !slices.ContainsFunc(ch.Warnings, func(w string) bool { return strings.Contains(w, "S01E01") }) {
		t.Fatalf("warnings %v", ch.Warnings)
	}
}

// TestStoppedSearches_LeaveNoSlotSearching checks that a search cut off by a stop puts its Slots back from searching.
func TestStoppedSearches_LeaveNoSlotSearching(t *testing.T) {
	for _, c := range []struct {
		name   string
		kind   string
		labels []string
	}{
		{"season", jobs.KindSearchSeason, []string{"S01E01", "S01E02", "S01E03"}},
		{"episode", jobs.KindSearchSlot, []string{"S02E01"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			block := make(chan struct{})
			ts := &tvServer{answer: func(url.Values) []string {
				select {
				case <-block:
				case <-time.After(10 * time.Second):
				}
				return nil
			}}
			f := seedShow(t, ts.start(t), nil)
			defer close(block)
			q := jobs.New(f.s, jobs.Options{Workers: 1, BaseBackoff: time.Hour, PollInterval: 5 * time.Millisecond})
			sr := &Searcher{Store: f.s, Queue: q}
			sr.Register(q)
			ctx := context.Background()
			if err := q.Start(ctx); err != nil {
				t.Fatal(err)
			}
			subj := &domain.Subject{Type: "part", ID: f.parts[1]}
			params := map[string]any{"version_id": f.verID}
			if c.kind == jobs.KindSearchSlot {
				subj, params = &domain.Subject{Type: "slot", ID: f.slots["S02E01"]}, nil
			}
			if _, _, err := q.Enqueue(ctx, c.kind, subj, params, "", domain.System); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(5 * time.Second)
			for f.state(t, c.labels[0]) != mediafiles.SlotSearching {
				if time.Now().After(deadline) {
					t.Fatalf("never searching: %s", f.state(t, c.labels[0]))
				}
				time.Sleep(5 * time.Millisecond)
			}
			q.Stop()
			for _, l := range c.labels {
				if st := f.state(t, l); st != mediafiles.SlotWanted {
					t.Errorf("%s left %s", l, st)
				}
			}
		})
	}
}

// TestSweepStale_Searching checks that at start, Slots left searching longer than the limit go back.
func TestSweepStale_Searching(t *testing.T) {
	ts := &tvServer{answer: func(url.Values) []string { return nil }}
	f := seedShow(t, ts.start(t), nil)
	old := database.FormatTime(time.Now().Add(-2 * time.Hour))
	exec(t, f.fixture, "UPDATE slots SET state = 'searching', updated_at = ? WHERE id = ?", old, f.slots["S01E01"])
	exec(t, f.fixture, "UPDATE slots SET state = 'searching' WHERE id = ?", f.slots["S01E02"])
	sr := &Searcher{Store: f.s}
	n, err := sr.SweepSearching(context.Background(), 30*time.Minute)
	if err != nil || n != 1 {
		t.Fatalf("swept %d %v", n, err)
	}
	if f.state(t, "S01E01") != mediafiles.SlotWanted || f.state(t, "S01E02") != mediafiles.SlotSearching {
		t.Fatalf("%s %s", f.state(t, "S01E01"), f.state(t, "S01E02"))
	}
}

func (p *slotPubs) Publish(topic string, data any) uint64 {
	if m, ok := data.(map[string]any); ok && topic == "slot" {
		if id, ok := m["id"].(int64); ok {
			p.mu.Lock()
			p.ids = append(p.ids, id)
			p.mu.Unlock()
		}
	}
	return 0
}

// holdForFeed puts the fixture's Slot in held for tv720 with the wait over.
func holdForFeed(t *testing.T) (fixture, domain.Release) {
	t.Helper()
	lf := &liveFeed{}
	lf.set(feedOf(item(tv720, time.Now(), 50, "")))
	f := seed(t, lf.url(t))
	feedOn(t, f)
	waitMinutes(t, f, 60)
	runKind(t, f, &Searcher{Store: f.s}, jobs.KindFeedCheck, nil, nil)
	if sl := slotState(t, f); sl.State != mediafiles.SlotHeld {
		t.Fatalf("slot %s", sl.State)
	}
	exec(t, f, "UPDATE slots SET next_try_at = ?", database.FormatTime(time.Now().Add(-time.Minute)))
	return f, relByName(t, f, tv720)
}
