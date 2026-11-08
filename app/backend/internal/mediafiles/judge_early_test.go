package mediafiles

import (
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

func reasonOf(v Verdict, rule string) (Reason, bool) {
	for _, r := range v.Reasons {
		if r.Rule == rule {
			return r, true
		}
	}
	return Reason{}, false
}

// 4.3.5: a Release posted before its movie came out is refused, unless a
// person searched (then it warns); a grace covers time zones; off when the
// setting is off.
func TestPostedEarly(t *testing.T) {
	out := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	name := "The.Lantern.Keepers.2021.1080p.WEB-DL.H264-HOLYARRS"
	rel := ReleaseFacts{Name: name, Parse: releasename.Parse(name, releasename.Hints{}), Size: 4 * gb, Protocol: "torrent", Seeders: ptr(50),
		PublishedAt: out.Add(-72 * time.Hour)}
	set := setFor(t, rel, nil)
	set.OutSince, set.Checks = &out, Checks{Early: true, Grace: 24 * time.Hour, Tiny: true}
	v := Judge(rel, set, GoodEveryday())
	if r, ok := reasonOf(v, "safety:posted_early"); !ok || r.Effect != "fail" || v.Passed {
		t.Fatalf("early: %+v", v.Reasons)
	}
	hand := set
	hand.HandSearch = true
	if r, ok := reasonOf(Judge(rel, hand, GoodEveryday()), "safety:posted_early"); !ok || r.Effect != "warn" {
		t.Fatalf("hand search: %+v", r)
	}
	rel.PublishedAt = out.Add(-12 * time.Hour) // inside the grace
	if _, ok := reasonOf(Judge(rel, set, GoodEveryday()), "safety:posted_early"); ok {
		t.Fatal("refused inside the grace")
	}
	rel.PublishedAt = out.Add(-72 * time.Hour)
	set.Checks.Early = false
	if _, ok := reasonOf(Judge(rel, set, GoodEveryday()), "safety:posted_early"); ok {
		t.Fatal("refused with the check off")
	}
}

// 4.3.5: far too small for a movie's runtime, for an unknown length, and
// for an album's tracks.
func TestTooSmall(t *testing.T) {
	name := "The.Lantern.Keepers.2021.1080p.WEB-DL.H264-HOLYARRS"
	rel := ReleaseFacts{Name: name, Parse: releasename.Parse(name, releasename.Hints{}), Size: 30 * mb, Protocol: "torrent", Seeders: ptr(50)}
	set := setFor(t, rel, nil)
	set.Checks = Checks{Tiny: true}
	rules := GoodEveryday()
	rules.SizeLimits = nil
	if r, ok := reasonOf(Judge(rel, set, rules), "safety:too_small"); !ok || r.Effect != "fail" {
		t.Fatalf("30 MB for 118 minutes: %+v", r)
	}
	set.RuntimeMinutes = 0
	if _, ok := reasonOf(Judge(rel, set, rules), "safety:too_small"); ok {
		t.Fatal("30 MB of unknown length refused")
	}
	rel.Size = 0 // not known: never refused
	set.RuntimeMinutes = 118
	if _, ok := reasonOf(Judge(rel, set, rules), "safety:too_small"); ok {
		t.Fatal("an unknown size refused")
	}
	album := "Northern Lanterns - Harbour Songs (2024) [FLAC]"
	p := releasename.Parse(album, releasename.Hints{Music: true})
	mrel := ReleaseFacts{Name: album, Parse: p, Size: 2 << 20}
	mset := SlotSet{Match: MatchTitle(p, musicLib), SlotIDs: []int64{1, 2, 3, 4, 5, 6}, Out: true, Checks: Checks{Tiny: true}}
	if r, ok := reasonOf(Judge(mrel, mset, GoodEveryday()), "safety:too_small"); !ok || r.Effect != "fail" {
		t.Fatalf("2 MB for 6 tracks: %+v", r)
	}
}

// 4.3.5 review: unknown size or dates fail open with a warning line in
// automatic searches, or hold the Release for a person (HoldUnknown); a
// person's own search adds no line.
func TestUnknownSizeAndDate(t *testing.T) {
	name := "The.Lantern.Keepers.2021.1080p.WEB-DL.H264-HOLYARRS"
	rel := ReleaseFacts{Name: name, Parse: releasename.Parse(name, releasename.Hints{}), Protocol: "torrent", Seeders: ptr(50)}
	set := setFor(t, rel, nil)
	set.Checks = Checks{Early: true, Tiny: true}
	rules := GoodEveryday()
	rules.SizeLimits = nil
	v := Judge(rel, set, rules)
	for _, rule := range []string{"safety:size_unknown", "safety:date_unknown"} {
		if r, ok := reasonOf(v, rule); !ok || r.Effect != "warn" {
			t.Fatalf("%s: %+v", rule, v.Reasons)
		}
	}
	if r, _ := reasonOf(v, "safety:size_unknown"); !strings.Contains(r.Detail, "size unknown") {
		t.Fatalf("detail %q", r.Detail)
	}
	set.Checks.HoldUnknown = true
	v = Judge(rel, set, rules)
	if r, ok := reasonOf(v, "safety:size_unknown"); !ok || r.Effect != "fail" || v.Passed {
		t.Fatalf("held: %+v", v.Reasons)
	}
	set.HandSearch = true
	v = Judge(rel, set, rules)
	if _, ok := reasonOf(v, "safety:size_unknown"); ok {
		t.Fatalf("hand search: %+v", v.Reasons)
	}
}

// 4.3.5 review: a season pack whose episodes have no known length is
// floored at the default length per episode.
func TestPackFloorWithoutRuntimes(t *testing.T) {
	// Judged as one Release over unknown-length episodes: the floor only.
	name := "The.Lantern.Keepers.2021.1080p.WEB-DL.H264-HOLYARRS"
	rel := ReleaseFacts{Name: name, Parse: releasename.Parse(name, releasename.Hints{}), Size: 150 * mb, Protocol: "torrent", Seeders: ptr(50)}
	set := setFor(t, rel, nil)
	set.RuntimeMinutes, set.FloorEpisodes = 0, 10
	set.Checks = Checks{Tiny: true, EpisodeMinutes: 20}
	rules := GoodEveryday()
	rules.SizeLimits = nil
	if r, ok := reasonOf(Judge(rel, set, rules), "safety:too_small"); !ok || r.Effect != "fail" || !strings.Contains(r.Detail, "200 minutes") {
		t.Fatalf("150 MB for 10 episodes: %+v", r)
	}
	set.FloorEpisodes = 5
	if _, ok := reasonOf(Judge(rel, set, rules), "safety:too_small"); ok {
		t.Fatal("150 MB for 5 episodes of 20 minutes refused")
	}
}
