package downloads

import (
	"slices"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// A finished torrent held by a partly done import names what it
// waits for, only when its Downloader removes torrents after seeding.
func TestDownloadsViewCleanupWaitNamesWaitingEpisodes(t *testing.T) {
	t.Parallel()
	x := seed41(t, mediafiles.SlotWanted, "finished")
	exec41(t, x.s, `UPDATE slots SET active_download_id = NULL`)
	importRow(t, x, "partly_done")
	if r := ownRow(t, x); len(r.CleanupWait) != 0 {
		t.Fatalf("leave after import: %v", r.CleanupWait)
	}
	exec41(t, x.s, `UPDATE downloaders SET after_import = 'remove'`)
	if r := ownRow(t, x); !slices.Equal(r.CleanupWait, []string{"Salt Choir"}) {
		t.Fatalf("cleanup wait %v", r.CleanupWait)
	}
	exec41(t, x.s, `UPDATE slots SET state = 'in_library'`)
	if r := ownRow(t, x); len(r.CleanupWait) != 0 {
		t.Fatalf("after the episode came in: %v", r.CleanupWait)
	}
}
