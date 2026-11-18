package downloads

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// qBittorrent reports a torrent it still shares as uploading or stalledUP,
// which maps to finished: the row still says it is seeding, so Remove says
// it stops the seeding, and its path is the torrent's own file as the client
// holds it now, not the save folder HolyArrs sent it to.
func TestDownloadsViewOwnFinishedRowSaysStillSeeding(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		state   string
		seeding bool
	}{{"uploading", true}, {"stalledUP", true}, {"forcedUP", true}, {"stoppedUP", false}} {
		x := seed41(t, mediafiles.SlotInLibrary, "finished")
		pointQB(t, x, qbList(t, `[{"hash":"abcdabcdabcdabcdabcdabcdabcdabcdabcdabcd","name":"Salt.Choir","category":"holyarrs","size":1000,"progress":1,"state":"`+c.state+`","save_path":"/moved/complete","content_path":"/moved/complete/Salt.Choir.mkv","added_on":1700000000}]`).URL)
		feedStore(t, x.dl)
		v := view(t, x.srv.URL+"/api/v1/downloads/view")
		if len(v.Downloads) == 0 {
			t.Fatalf("%s: no own row", c.state)
		}
		r := v.Downloads[0]
		if r.Seeding != c.seeding {
			t.Errorf("%s: seeding %v, want %v", c.state, r.Seeding, c.seeding)
		}
		if r.Path != "/moved/complete/Salt.Choir.mkv" {
			t.Errorf("%s: path %q, want the torrent's own file where the client holds it now", c.state, r.Path)
		}
	}
}
