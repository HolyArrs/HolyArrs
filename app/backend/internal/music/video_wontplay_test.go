package music

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata/youtube"
)

// A music video is dropped from the artist's list only when YouTube will
// not play that video, not when its door fails.
func TestVideoWontPlay(t *testing.T) {
	for err, want := range map[error]bool{
		nil:                  false,
		metadata.ErrNoStream: false,
		&metadata.TrailerMissed{Reason: youtube.BotCheck}:      false,
		&metadata.TrailerMissed{Reason: youtube.Network}:       false,
		&metadata.TrailerMissed{Reason: youtube.Unavailable}:   true,
		&metadata.TrailerMissed{Reason: youtube.AgeRestricted}: true,
	} {
		if got := videoWontPlay(err); got != want {
			t.Fatalf("%v: %v", err, got)
		}
	}
}
