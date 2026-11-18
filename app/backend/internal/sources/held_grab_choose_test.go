package sources

import (
	"context"
	"errors"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// The end of a wait checks the year again with no feed check in between: a
// release whose year is no longer the show's is not sent and its Slot goes
// back to looking; one taken by the Source's id is sent.
func TestGrabHeld_ChecksTheYear(t *testing.T) {
	for _, c := range []struct {
		why  string
		rel  string
		tvdb int
		want mediafiles.SlotState
	}{
		{"the year is not the show's", "Copper.Valley.2015.S01E01.1080p.WEB-DL.H.264-OXBOW", 0, mediafiles.SlotWanted},
		{"taken by the Source's id", "Copper.Valley.2019.S01E01.1080p.WEB-DL.H.264-OXBOW", 4242, mediafiles.SlotChosen},
	} {
		t.Run(c.why, func(t *testing.T) {
			f := seedShow(t, heldServer(t, func() (string, int) { return c.rel, c.tvdb }), nil)
			waitMinutes(t, f.fixture, 60)
			sr := &Searcher{Store: f.s}
			runKind(t, f.fixture, sr, jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: f.slots["S01E01"]}, nil)
			if st := f.state(t, "S01E01"); st != mediafiles.SlotHeld {
				t.Fatalf("after search: slot %s, want held", st)
			}
			exec(t, f.fixture, `UPDATE titles SET year = 2016`)
			waitOver(t, f.fixture)
			runKind(t, f.fixture, sr, jobs.KindGrabHeld, nil, nil)
			st, rs := f.state(t, "S01E01"), relByName(t, f.fixture, c.rel).State
			if st != c.want || (c.want == mediafiles.SlotWanted) != (rs != "chosen" && rs != "held") {
				t.Fatalf("after the wait: slot %s, release %s, want slot %s", st, rs, c.want)
			}
		})
	}
}

// A season pack whose year is not the show's matches no episode, but a
// person can still force it from the season's search: it goes to the
// episodes its name covers. A release named for another show cannot.
func TestChoose_YearRefusedPackFromSeasonSearch(t *testing.T) {
	pack, other := "Copper.Valley.2019.S01.1080p.WEB-DL.H.264-QUILL", "Silver.Valley.S01.1080p.WEB-DL.H.264-QUILL"
	ts := &tvServer{answer: func(url.Values) []string { return []string{pack, other} }}
	f := seedShow(t, ts.start(t), nil)
	sr := &Searcher{Store: f.s}
	runKind(t, f.fixture, sr, jobs.KindSearchSeason, &domain.Subject{Type: "part", ID: f.parts[1]}, map[string]any{"version_id": f.verID, "choose": false, "by_hand": true})
	rel := relByName(t, f.fixture, pack)
	if len(rel.MatchedSlotIDs) != 0 || rel.State != "refused" {
		t.Fatalf("pack %s matched %v, want refused with no episode", rel.State, rel.MatchedSlotIDs)
	}
	ctx := context.Background()
	var nf *NeedsForceError
	if _, err := sr.Choose(ctx, rel.ID, 0, false, domain.System); err == nil || !asNeedsForce(err, &nf) {
		t.Fatalf("choose without force: %v", err)
	}
	ch, err := sr.Choose(ctx, rel.ID, 0, true, domain.System)
	if err != nil || !ch.Forced {
		t.Fatalf("forced choose: %+v %v", ch, err)
	}
	for _, l := range []string{"S01E01", "S01E02", "S01E03"} {
		if st := f.state(t, l); st != mediafiles.SlotChosen {
			t.Errorf("%s is %s, want chosen", l, st)
		}
	}
	if st := f.state(t, "S02E01"); st == mediafiles.SlotChosen {
		t.Errorf("S02E01 was chosen for a season 1 pack")
	}
	if _, err := sr.Choose(ctx, relByName(t, f.fixture, other).ID, 0, true, domain.System); !errors.Is(err, ErrNoSlot) {
		t.Fatalf("a release named for another show was forced: %v", err)
	}
}

// A waiting movie release that no longer passes at a feed check frees its Slot.
func TestFeed_FailingHeldMovieFreesSlot(t *testing.T) {
	lf := &liveFeed{}
	lf.set(feedOf(item(tv720, time.Now(), 50, "")))
	f := seed(t, lf.url(t))
	feedOn(t, f)
	waitMinutes(t, f, 60)
	sr := &Searcher{Store: f.s}
	runKind(t, f, sr, jobs.KindFeedCheck, nil, nil)
	if st := slotState(t, f).State; st != mediafiles.SlotHeld {
		t.Fatalf("after the first feed check: slot %s, want held", st)
	}
	exec(t, f, `UPDATE sources SET min_seeders = 500`)
	lf.set(feedOf(item(tv720, time.Now(), 50, ""), item(web1080, time.Now(), 50, "")))
	runKind(t, f, sr, jobs.KindFeedCheck, nil, nil)
	if st, rs := slotState(t, f).State, relByName(t, f, tv720).State; st != mediafiles.SlotWanted || rs == "held" {
		t.Fatalf("after the second feed check: slot %s, release %s, want the slot looking again", st, rs)
	}
}

// A waiting album release that no longer passes at a feed check frees its tracks.
func TestFeed_FailingHeldAlbumFreesSlots(t *testing.T) {
	ms := &musicServer{answer: func(url.Values) []string { return []string{flac, mp3} }}
	f := seedArtist(t, ms.start(t))
	exec(t, f.fixture, `UPDATE pick_rules SET body = json_set(body, '$.protocol.wait_minutes_torrent', 100000) WHERE name = ?`, mediafiles.LosslessFirstName)
	sr := &Searcher{Store: f.s}
	f.albumJob(t, sr, lta, nil)
	for _, l := range ltaTracks {
		if st := f.state(t, l); st != mediafiles.SlotHeld {
			t.Fatalf("%s is %s, want held", l, st)
		}
	}
	exec(t, f.fixture, `UPDATE sources SET min_seeders = 100000`)
	exec(t, f.fixture, `DELETE FROM releases WHERE state != 'held'`)
	runKind(t, f.fixture, sr, jobs.KindFeedCheck, nil, nil)
	for _, l := range ltaTracks {
		if st := f.state(t, l); st != mediafiles.SlotWanted {
			t.Errorf("%s is %s after the feed check, want looking again", l, st)
		}
	}
}

// seasonSearchRel runs a by-hand search of season 1 that finds name and
// returns the stored Release.
func seasonSearchRel(t *testing.T, name string, prep func(showFix)) (showFix, *Searcher, domain.Release) {
	t.Helper()
	ts := &tvServer{answer: func(url.Values) []string { return []string{name} }}
	f := seedShow(t, ts.start(t), nil)
	if prep != nil {
		prep(f)
	}
	sr := &Searcher{Store: f.s}
	runKind(t, f.fixture, sr, jobs.KindSearchSeason, &domain.Subject{Type: "part", ID: f.parts[1]}, map[string]any{"version_id": f.verID, "choose": false, "by_hand": true})
	return f, sr, relByName(t, f.fixture, name)
}

// A forced pack whose year is not the show's leaves out an episode that
// already has a copy as good, and says so.
func TestChoose_YearRefusedPackKeepsBetterCopy(t *testing.T) {
	f, sr, rel := seasonSearchRel(t, "Copper.Valley.2019.S01.720p.HDTV.x264-OXBOW", func(f showFix) {
		addFile(t, f, f.slots["S01E01"], "Copper.Valley.S01E01.2160p.BluRay.REMUX.HEVC-GRP")
		exec(t, f.fixture, "UPDATE slots SET state = 'better_wanted' WHERE id = ?", f.slots["S01E01"])
	})
	ch, err := sr.Choose(context.Background(), rel.ID, 0, true, domain.System)
	if err != nil || !ch.Forced {
		t.Fatalf("forced choose: %+v %v", ch, err)
	}
	if st := f.state(t, "S01E01"); st == mediafiles.SlotChosen {
		t.Errorf("S01E01 with a better copy was taken")
	}
	for _, l := range []string{"S01E02", "S01E03"} {
		if st := f.state(t, l); st != mediafiles.SlotChosen {
			t.Errorf("%s is %s, want chosen", l, st)
		}
	}
	if !slices.ContainsFunc(ch.Warnings, func(w string) bool { return strings.Contains(w, "S01E01 already has a copy as good") }) {
		t.Errorf("the left-out episode is not named: %v", ch.Warnings)
	}
}

// Only a year miss opens the forced season pick: a release that matched
// the show and was refused for something else, or a pack of another
// season, cannot be forced this way.
func TestChoose_SeasonPickOnlyForAYearMiss(t *testing.T) {
	for _, name := range []string{
		"Copper.Valley.S01-S02.1080p.WEB-DL.H.264-QUILL",
		"Copper.Valley.2019.S02.1080p.WEB-DL.H.264-QUILL",
	} {
		_, sr, rel := seasonSearchRel(t, name, nil)
		if len(rel.MatchedSlotIDs) != 0 {
			t.Fatalf("%s matched %v, want none", name, rel.MatchedSlotIDs)
		}
		if _, err := sr.Choose(context.Background(), rel.ID, 0, true, domain.System); !errors.Is(err, ErrNoSlot) {
			t.Errorf("%s was forced from the season search: %v", name, err)
		}
	}
}

// A forced year-miss pack where every episode already has a copy as good
// sends nothing and says why, with or without force.
func TestChoose_YearRefusedPackNothingGains(t *testing.T) {
	labels := []string{"S01E01", "S01E02", "S01E03"}
	f, sr, rel := seasonSearchRel(t, "Copper.Valley.2019.S01.720p.HDTV.x264-OXBOW", func(f showFix) {
		for _, l := range labels {
			addFile(t, f, f.slots[l], "Copper.Valley."+l+".2160p.BluRay.REMUX.HEVC-GRP")
			exec(t, f.fixture, "UPDATE slots SET state = 'better_wanted' WHERE id = ?", f.slots[l])
		}
	})
	for _, force := range []bool{false, true} {
		_, err := sr.Choose(context.Background(), rel.ID, 0, force, domain.System)
		var nf *NeedsForceError
		if !errors.As(err, &nf) || !strings.Contains(strings.Join(nf.Reasons, " "), "nothing would gain") {
			t.Fatalf("force=%v: %v, want nothing would gain", force, err)
		}
	}
	for _, l := range labels {
		if st := f.state(t, l); st == mediafiles.SlotChosen {
			t.Errorf("%s was taken although its copy is as good", l)
		}
	}
}

// A year-miss pack that also holds another season gets a line saying so.
func TestChoose_YearRefusedPackOfTwoSeasons(t *testing.T) {
	_, sr, rel := seasonSearchRel(t, "Copper.Valley.2019.S01-S02.720p.HDTV.x264-OXBOW", nil)
	for _, force := range []bool{false, true} {
		_, err := sr.Choose(context.Background(), rel.ID, 0, force, domain.System)
		var nf *NeedsForceError
		if !errors.As(err, &nf) || !strings.Contains(strings.Join(nf.Reasons, " "), "more than this season") {
			t.Fatalf("force=%v: %v, want the whole-show line", force, err)
		}
	}
}
