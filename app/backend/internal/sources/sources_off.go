package sources

import (
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
)

// SkippedSourcesOff is the result of a search Job that ended at once because
// the Sources part is off.
const SkippedSourcesOff = "Sources are off"

// on reports whether the Sources part is on (no switch wired means on).
func (s *Searcher) on() bool { return s.SourcesOn == nil || s.SourcesOn() }

// sourcesOff ends an automatic, scheduled or bulk search quietly while the
// Sources part is off, for people who add their downloads by hand: no
// Source is asked, nothing is written but the Job's result. A search a
// person asks for on one title (by_hand) still runs.
func (s *Searcher) sourcesOff(r *jobs.Run) bool {
	if s.on() {
		return false
	}
	if hand, _ := r.Job.Params["by_hand"].(bool); hand {
		return false
	}
	r.SetResult(map[string]any{"skipped": SkippedSourcesOff})
	return true
}
