package arrapi

import (
	"errors"
	"fmt"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

// A refused delete names its reason. Unknown errors stay generic.
func TestRemoveReason(t *testing.T) {
	cases := map[error]string{
		fmt.Errorf("x: %w", library.ErrProtectedCopy):                   library.ErrProtectedCopy.Error(),
		errors.New("moving a.mkv to the holding folder: access denied"): "a file could not be moved to the holding folder",
		errors.New("database is locked"):                                "this title could not be removed",
	}
	for err, want := range cases {
		if got := removeReason(err); got != want {
			t.Errorf("%v: got %q want %q", err, got, want)
		}
	}
}
