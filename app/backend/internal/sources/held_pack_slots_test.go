package sources

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

const heldPack = "Copper.Valley.S01.1080p.WEB-DL.H.264-QUILL"

var heldPackEps = []string{"S01E01", "S01E02", "S01E03"}

// heldPackShow is a show whose feed first lists a season 1 pack that every
// episode of the season waits on, then only what feed returns.
func heldPackShow(t *testing.T, feed *string) (showFix, *Searcher) {
	t.Helper()
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch {
		case q.Get("t") == "caps":
			_, _ = w.Write([]byte(tvCaps))
		case q.Get("q") != "" || q.Get("season") != "" || q.Get("tvdbid") != "":
			_, _ = w.Write([]byte(feedOf()))
		default:
			_, _ = w.Write([]byte(feedOf(*feed)))
		}
	})
	f := seedShow(t, srv.URL+"/api", nil)
	setSetting(t, f.fixture, SettingPackUpgrade, "any")
	waitMinutes(t, f.fixture, 60)
	sr := &Searcher{Store: f.s}
	*feed = heldPackItem(heldPack)
	runKind(t, f.fixture, sr, jobs.KindFeedCheck, nil, nil)
	for _, ep := range heldPackEps {
		if st := f.state(t, ep); st != mediafiles.SlotHeld {
			t.Fatalf("%s after the first feed check: %s, want held", ep, st)
		}
	}
	return f, sr
}

func heldPackItem(name string) string {
	return fmt.Sprintf(`<item><title>%s</title><pubDate>%s</pubDate><enclosure url="http://src/%s.torrent" length="%d" type="application/x-bittorrent"/><torznab:attr name="seeders" value="50"/></item>`,
		name, time.Now().UTC().Format(time.RFC1123Z), name, int64(1200)<<20)
}

// endWait makes every wait over.
func endWait(t *testing.T, f fixture) {
	t.Helper()
	exec(t, f, "UPDATE slots SET next_try_at = '2000-01-01T00:00:00Z' WHERE state = 'held'")
	exec(t, f, "UPDATE releases SET hold_until = '2000-01-01T00:00:00Z' WHERE state = 'held'")
}

// heldOn is the Release a Slot waits on, 0 for none.
func heldOn(t *testing.T, f fixture, slotID int64) int64 {
	t.Helper()
	var id sql.NullInt64
	if err := f.s.Read().QueryRow("SELECT held_release_id FROM slots WHERE id = ?", slotID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id.Int64
}

// A season pack that waits for three episodes is judged again by later feed
// checks. When the wait ends it is chosen for all three.
func TestHeldPack_JudgedAgainStillChosenForEverySlot(t *testing.T) {
	var feed string
	f, sr := heldPackShow(t, &feed)
	for i := 2; i <= 3; i++ {
		feed = heldPackItem(fmt.Sprintf("Copper.Valley.S02E01.1080p.WEB-DL.H.264-QUILL%d", i))
		runKind(t, f.fixture, sr, jobs.KindFeedCheck, nil, nil)
	}
	endWait(t, f.fixture)
	runKind(t, f.fixture, sr, jobs.KindGrabHeld, nil, nil)
	for _, ep := range heldPackEps {
		if st := f.state(t, ep); st != mediafiles.SlotChosen {
			t.Errorf("%s after the wait: %s, want chosen", ep, st)
		}
	}
	if got := PickSlots(relByName(t, f.fixture, heldPack)); len(got) != 3 {
		t.Errorf("the pack is picked for %v, want all three episodes", got)
	}
}

// An album that waits for five tracks is judged again by a feed check. When
// the wait ends it is chosen for every track.
func TestHeldAlbum_JudgedAgainStillChosenForEveryTrack(t *testing.T) {
	n := 0
	ms := &musicServer{answer: func(q url.Values) []string {
		if q.Get("q") == "" && q.Get("artist") == "" && q.Get("album") == "" {
			n++
			return []string{fmt.Sprintf("Velvet Cartography - Low Tide Atlas (2019) [MP3 V%d]|100", n)}
		}
		return []string{flac}
	}}
	f := seedArtist(t, ms.start(t))
	exec(t, f.fixture, "UPDATE pick_rules SET body = json_set(body, '$.protocol.wait_minutes_torrent', 100000) WHERE name = ?", mediafiles.LosslessFirstName)
	sr := &Searcher{Store: f.s}
	f.albumJob(t, sr, lta, nil)
	for _, l := range ltaTracks {
		if st := f.state(t, l); st != mediafiles.SlotHeld {
			t.Fatalf("%s after the album search: %s, want held", l, st)
		}
	}
	runKind(t, f.fixture, sr, jobs.KindFeedCheck, nil, nil)
	endWait(t, f.fixture)
	runKind(t, f.fixture, sr, jobs.KindGrabHeld, nil, nil)
	for _, l := range ltaTracks {
		if st := f.state(t, l); st != mediafiles.SlotChosen {
			t.Errorf("%s after the wait: %s, want chosen", l, st)
		}
	}
}

// While a pack waits, one of its episodes gets a copy as good. That episode
// goes back to looking with its file kept; the others keep waiting and are
// chosen when the wait ends.
func TestHeldPack_SlotItNoLongerFillsGoesBackToLooking(t *testing.T) {
	var feed string
	f, sr := heldPackShow(t, &feed)
	name := "Copper.Valley.S01E02.2160p.BluRay.REMUX.HEVC-BEST"
	err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		fl := &mediafiles.File{SlotIDs: []int64{f.slots["S01E02"]}, VersionID: f.verID, RelativePath: "S01E02.mkv", Size: 1, AddedAt: time.Now(),
			Languages: []string{}, MediaInfo: map[string]any{}, ReleaseName: &name, Origin: "imported", PlacedBy: "copy", Role: "main", State: "present"}
		if _, err := database.Insert(tx, "files", fl, domain.System); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE slots SET file_id = ? WHERE id = ?", fl.ID, f.slots["S01E02"])
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	feed = heldPackItem("Copper.Valley.S02E01.1080p.WEB-DL.H.264-QUILL2")
	runKind(t, f.fixture, sr, jobs.KindFeedCheck, nil, nil)
	if st := f.state(t, "S01E02"); st != mediafiles.SlotBetterWanted || heldOn(t, f.fixture, f.slots["S01E02"]) != 0 {
		t.Fatalf("S01E02 after the feed check: %s waiting on %d, want better wanted and nothing", st, heldOn(t, f.fixture, f.slots["S01E02"]))
	}
	for _, ep := range []string{"S01E01", "S01E03"} {
		if st := f.state(t, ep); st != mediafiles.SlotHeld {
			t.Fatalf("%s after the feed check: %s, want held", ep, st)
		}
	}
	endWait(t, f.fixture)
	runKind(t, f.fixture, sr, jobs.KindGrabHeld, nil, nil)
	for _, ep := range []string{"S01E01", "S01E03"} {
		if st := f.state(t, ep); st != mediafiles.SlotChosen {
			t.Errorf("%s after the wait: %s, want chosen", ep, st)
		}
	}
	if st := f.state(t, "S01E02"); st != mediafiles.SlotBetterWanted {
		t.Errorf("S01E02 after the wait: %s, want better wanted", st)
	}
}

// Slots left waiting on a Release that is no longer waiting (chosen for one
// of them, or passed over) go back to looking when their wait ends.
func TestGrabHeld_FreesSlotsOfAReleaseNoLongerWaiting(t *testing.T) {
	for _, state := range []string{"chosen", "passed_over", "refused"} {
		t.Run(state, func(t *testing.T) {
			var feed string
			f, sr := heldPackShow(t, &feed)
			endWait(t, f.fixture)
			exec(t, f.fixture, "UPDATE releases SET state = ?, hold_until = NULL WHERE name = ?", state, heldPack)
			runKind(t, f.fixture, sr, jobs.KindGrabHeld, nil, nil)
			for _, ep := range heldPackEps {
				if st := f.state(t, ep); st != mediafiles.SlotWanted || heldOn(t, f.fixture, f.slots[ep]) != 0 {
					t.Errorf("%s: %s waiting on %d, want wanted and nothing", ep, st, heldOn(t, f.fixture, f.slots[ep]))
				}
			}
			if got := relByName(t, f.fixture, heldPack).State; got != state {
				t.Errorf("the Release became %s, want it left %s", got, state)
			}
		})
	}
}

// wipePickList puts a waiting Release in the state an older build left it
// in: still held by its Slots, the list of Slots it was held for gone.
func wipePickList(t *testing.T, f fixture, name string) {
	t.Helper()
	exec(t, f, "UPDATE releases SET verdict = json_remove(verdict, '$.pick_slot_ids') WHERE name = ?", name)
	if rel := relByName(t, f, name); rel.State != "held" || rel.Verdict["pick_slot_ids"] != nil {
		t.Fatalf("%s: %s with pick list %v, want held with none", name, rel.State, rel.Verdict["pick_slot_ids"])
	}
}

// oldHeldSteps are the ways a wait can end for a Release with no pick list.
var oldHeldSteps = map[string][]string{
	"grab-held first":                   {"end", jobs.KindGrabHeld},
	"feed check first":                  {"end", jobs.KindFeedCheck},
	"feed check during, then grab-held": {jobs.KindFeedCheck, "end", jobs.KindGrabHeld},
}

// A pack left waiting with no pick list is still chosen for every episode
// of its Version when the wait ends, as one pick.
func TestHeldPack_NoPickListStillChosenForEverySlot(t *testing.T) {
	for name, steps := range oldHeldSteps {
		t.Run(name, func(t *testing.T) {
			var feed string
			f, sr := heldPackShow(t, &feed)
			sent := 0
			sr.OnChosen = func(_ context.Context, _, releaseID int64) {
				if releaseID == relByName(t, f.fixture, heldPack).ID {
					sent++
				}
			}
			wipePickList(t, f.fixture, heldPack)
			feed = heldPackItem("Copper.Valley.S02E01.1080p.WEB-DL.H.264-QUILL2")
			for _, step := range steps {
				if step == "end" {
					endWait(t, f.fixture)
					continue
				}
				runKind(t, f.fixture, sr, step, nil, nil)
			}
			for _, ep := range heldPackEps {
				if st := f.state(t, ep); st != mediafiles.SlotChosen {
					t.Errorf("%s after the wait: %s, want chosen", ep, st)
				}
			}
			if got := PickSlots(relByName(t, f.fixture, heldPack)); len(got) != 3 || sent != 1 {
				t.Errorf("the pack is picked for %v and sent %d times, want three episodes sent once", got, sent)
			}
		})
	}
}

// The same for an album and its tracks.
func TestHeldAlbum_NoPickListStillChosenForEveryTrack(t *testing.T) {
	for name, steps := range oldHeldSteps {
		t.Run(name, func(t *testing.T) {
			n := 0
			ms := &musicServer{answer: func(q url.Values) []string {
				if q.Get("q") == "" && q.Get("artist") == "" && q.Get("album") == "" {
					n++
					return []string{fmt.Sprintf("Velvet Cartography - Low Tide Atlas (2019) [MP3 V%d]|100", n)}
				}
				return []string{flac}
			}}
			f := seedArtist(t, ms.start(t))
			exec(t, f.fixture, "UPDATE pick_rules SET body = json_set(body, '$.protocol.wait_minutes_torrent', 100000) WHERE name = ?", mediafiles.LosslessFirstName)
			sent := 0
			sr := &Searcher{Store: f.s, OnChosen: func(context.Context, int64, int64) { sent++ }}
			f.albumJob(t, sr, lta, nil)
			wipePickList(t, f.fixture, flac)
			for _, step := range steps {
				if step == "end" {
					endWait(t, f.fixture)
					continue
				}
				runKind(t, f.fixture, sr, step, nil, nil)
			}
			for _, l := range ltaTracks {
				if st := f.state(t, l); st != mediafiles.SlotChosen {
					t.Errorf("%s after the wait: %s, want chosen", l, st)
				}
			}
			if got := PickSlots(relByName(t, f.fixture, flac)); len(got) != len(ltaTracks) || sent != 1 {
				t.Errorf("the album is picked for %v and sent %d times, want %d tracks sent once", got, sent, len(ltaTracks))
			}
		})
	}
}
