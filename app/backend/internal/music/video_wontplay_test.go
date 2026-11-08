package music

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// A music video is dropped from the artist's list only when YouTube will
// not play that video, not when its door fails.
func TestVideoWontPlay(t *testing.T) {
	for err, want := range map[error]bool{
		nil:                  false,
		metadata.ErrNoStream: false,
		&metadata.TrailerMissed{Reason: metadata.VideoBotCheck}:      false,
		&metadata.TrailerMissed{Reason: metadata.VideoNetwork}:       false,
		&metadata.TrailerMissed{Reason: metadata.VideoUnavailable}:   true,
		&metadata.TrailerMissed{Reason: metadata.VideoAgeRestricted}: true,
	} {
		if got := videoWontPlay(err); got != want {
			t.Fatalf("%v: %v", err, got)
		}
	}
}
