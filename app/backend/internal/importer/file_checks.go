package importer

import (
	"fmt"
	"slices"
	"strings"
)

// How far a movie's play length may fall short of the Title's runtime before
// it is held for Review: 15%, or 12 minutes for short ones. Longer is
// looser (50%, at least 30 minutes): extended and director's cuts run long,
// while a far longer file is often a Title matched to the wrong TMDB entry.
const (
	runtimeShare      = 15
	runtimeFloorSecs  = 12 * 60
	runtimeLongShare  = 50
	runtimeLongFloor  = 30 * 60
	runtimeCheckFloor = 20 // minutes; shorter Titles are skipped (shorts vary too much)
)

// heldWhy says why a movie file that passed the fake checks still needs a
// person's look before import: no video track, a play length far from the
// Title's, or a first audio track in a language the Title doesn't use.
// Like the old apps, such a file is not turned down; the download waits in
// Review, where it can be imported anyway or turned down.
// "" when nothing is off or when a fact is unknown.
func heldWhy(facts map[string]any, minutes int, original string, wanted []string) string {
	if facts == nil {
		return ""
	}
	if c, _ := facts["container"].(string); c != "" {
		if n, ok := facts["video_tracks"].(int); ok && n == 0 {
			return "the file holds no video track"
		}
	}
	if secs := factInt(facts, "runtime_seconds"); secs > 0 && minutes >= runtimeCheckFloor {
		want := minutes * 60
		short := max(want*runtimeShare/100, runtimeFloorSecs)
		long := max(want*runtimeLongShare/100, runtimeLongFloor)
		if secs < want-short || secs > want+long {
			return fmt.Sprintf("it plays %d min but the title runs %d min; check it is the right one", (secs+30)/60, minutes)
		}
	}
	first, _ := facts["first_audio_language"].(string)
	original = strings.ToLower(strings.TrimSpace(original))
	if first != "" && original != "" && first != original && !slices.Contains(wanted, first) {
		return fmt.Sprintf("the first audio track is %s, not the title's %s", strings.ToUpper(first), strings.ToUpper(original))
	}
	return ""
}
