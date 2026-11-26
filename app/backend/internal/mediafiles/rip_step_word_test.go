package mediafiles

import (
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// A Blu-ray rip at SD size lands on the SD disc step; the step's name in a
// reason must not read as DVD only, or a refused BRRip looks mislabelled.
func TestSDRipStepWordNamesBluRay(t *testing.T) {
	for _, name := range []string{"Movie.2020.480p.BRRip.x264-GRP", "Birdman.2014.FRENCH.BRRip.x264-GHZ", "Movie.2020.DVDRip.XviD-GRP"} {
		w := StepWord(StepFor(releasename.Parse(name, releasename.Hints{})))
		if !strings.Contains(w, "Blu-ray") || !strings.Contains(w, "DVD") {
			t.Errorf("%s: step word %q, want it to name DVD and Blu-ray", name, w)
		}
	}
}
