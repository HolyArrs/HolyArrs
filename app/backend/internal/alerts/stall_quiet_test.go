package alerts

import (
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// A Release that keeps stalling alerts on its first and second stall, then
// stays quiet while it waits.
func TestRepeatedStallsStayQuietAfterTheSecond(t *testing.T) {
	for _, c := range []struct {
		data map[string]any
		want string
	}{
		{nil, MomentDownloadFailed},
		{map[string]any{"stall_count": 2}, MomentDownloadFailed},
		{map[string]any{"stall_count": float64(3)}, ""},
		{map[string]any{"stall_count": 7}, ""},
	} {
		if got := MomentOf(domain.Event{Kind: "download_failed", Data: c.data}); got != c.want {
			t.Fatalf("%v: %q", c.data, got)
		}
	}
}
