package mediafiles

import (
	"fmt"
	"time"
)

// Plausible least sizes (4.3.5): a Release smaller than these cannot hold
// what its name says, whatever the Pick rules' size limits are. The
// importer applies the same floors to the files it places.
const (
	MinVideoPerMinute = 1 << 20  // 1 MB a minute of video, far below any real copy
	MinVideo          = 20 << 20 // a video of unknown length
	MinTrack          = 512 << 10
)

// earlyAndTiny refuses a Release posted before its Items came out, and one
// far too small for what it claims to hold (settings in Checks). When the
// day posted, the day out or the size is not known the check cannot run:
// an automatic search says so in a warning line, and holds the Release for
// a person's choice when Checks.HoldUnknown is on (fail-open by default).
func (v *Verdict) earlyAndTiny(rel ReleaseFacts, set SlotSet) {
	c := set.Checks
	unknown := func(rule, detail string) {
		if set.HandSearch {
			return
		}
		if c.HoldUnknown {
			v.add("hard", rule, "fail", detail+"; held for a person to choose (setting)", 0)
			return
		}
		v.add("hard", rule, "warn", detail, 0)
	}
	if c.Early && (set.OutSince == nil || rel.PublishedAt.IsZero()) {
		what := "when it came out"
		if rel.PublishedAt.IsZero() {
			what = "when it was posted"
		}
		unknown("safety:date_unknown", "date unknown: "+what+" is not known, so the early check did not run")
	}
	if c.Early && set.OutSince != nil && !rel.PublishedAt.IsZero() && rel.PublishedAt.Add(c.Grace).Before(*set.OutSince) {
		detail := fmt.Sprintf("posted %s, before it came out (%s)", day(rel.PublishedAt), day(*set.OutSince))
		if set.HandSearch {
			// A person searched for it: said plainly, the choice is theirs.
			v.add("hard", "safety:posted_early", "warn", detail+"; it may be a fake, shown because a person searched for it", 0)
		} else {
			v.add("hard", "safety:posted_early", "fail", detail+"; likely a fake", 0)
		}
	}
	if !c.Tiny {
		return
	}
	if rel.Size <= 0 {
		unknown("safety:size_unknown", "size unknown: the source did not say how big it is, so the size checks did not run")
		return
	}
	var least int64
	what := "a video"
	minutes := set.RuntimeMinutes
	if minutes <= 0 && set.FloorEpisodes > 0 {
		// Episodes of unknown length: the default length each (setting).
		minutes = set.FloorEpisodes * max(c.EpisodeMinutes, 1)
	}
	switch {
	case rel.Parse.Music != nil:
		least, what = int64(max(len(set.SlotIDs), 1))*MinTrack, fmt.Sprintf("%d tracks", max(len(set.SlotIDs), 1))
	case minutes > 0:
		least, what = int64(minutes)*MinVideoPerMinute, fmt.Sprintf("%d minutes of video", minutes)
	default:
		least = MinVideo
	}
	if rel.Size < least {
		v.add("hard", "safety:too_small", "fail", fmt.Sprintf("%s is too small for %s; likely a fake", sizeWord(rel.Size), what), 0)
	}
}

func day(t time.Time) string { return t.UTC().Format("2006-01-02 15:04 UTC") }

func sizeWord(n int64) string {
	if n < 1<<20 {
		return fmt.Sprintf("%d KB", n>>10)
	}
	return fmt.Sprintf("%.0f MB", float64(n)/(1<<20))
}
