package downloads

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// Once the Downloader reports the torrent's real size, the list row shows
// it (as the pane does), not the size the Release was listed with.
func TestDownloadsViewOwnRowTakesTheRealSize(t *testing.T) {
	t.Parallel()
	x := seed41(t, mediafiles.SlotDownloading, "downloading")
	pointQB(t, x, qbList(t, `[{"hash":"abcdabcdabcdabcdabcdabcdabcdabcdabcdabcd","name":"Salt.Choir","category":"holyarrs","size":31457280,"progress":0.3,"state":"downloading","added_on":1700000000}]`).URL)
	v := view(t, x.srv.URL+"/api/v1/downloads/view")
	if len(v.Downloads) == 0 || v.Downloads[0].Size != 1000 {
		t.Fatalf("before the client is read, the listed size: %+v", v.Downloads)
	}
	feedStore(t, x.dl)
	v = view(t, x.srv.URL+"/api/v1/downloads/view")
	if len(v.Downloads) == 0 || v.Downloads[0].Size != 31457280 {
		t.Fatalf("own row size %d, want the client's 31457280", v.Downloads[0].Size)
	}
}
