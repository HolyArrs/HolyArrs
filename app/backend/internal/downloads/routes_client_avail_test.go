package downloads

import (
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// A downloading torrent carries the swarm's full copies; a finished one or
// a client that does not say carries none.
func TestClientRowAvailability(t *testing.T) {
	t.Parallel()
	q := domain.Downloader{ID: 1, Name: "Q", Adapter: "qbittorrent", Protocol: "torrent"}
	a := 0.4
	if r := clientRow(q, Job{ID: outHash, Progress: 0.5, Availability: &a}); r.Availability == nil || *r.Availability != 0.4 {
		t.Fatalf("downloading row %+v", r.Availability)
	}
	if r := clientRow(q, Job{ID: outHash, Progress: 1, Availability: &a}); r.Availability != nil {
		t.Fatalf("finished row %+v", r.Availability)
	}
	if r := clientRow(q, Job{ID: outHash, Progress: 0.5}); r.Availability != nil {
		t.Fatalf("unknown row %+v", r.Availability)
	}
}
