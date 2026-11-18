package downloads

import (
	"context"
	"errors"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// twinEngine lists a twin of the job, or fails to list at all.
type twinEngine struct {
	removeSpy
	twin Job
	fail bool
}

func (e *twinEngine) List(context.Context, string) ([]Job, error) {
	if e.fail {
		return nil, errors.New("list failed")
	}
	return []Job{e.twin}, nil
}

func TestKeepFilesWhyUsenetSkipsTwinCheck(t *testing.T) {
	_, m := setup(t)
	j := Job{ID: "nzo_1", ContentPath: "/data/usenet/complete/Some.Movie.2024"}
	for _, e := range []*twinEngine{
		{fail: true},
		{twin: Job{ID: "nzo_2", ContentPath: j.ContentPath}},
	} {
		dr := domain.Downloader{Name: "SAB", Protocol: "usenet"}
		if why := m.keepFilesWhy(context.Background(), e, dr, j); why != "" {
			t.Fatalf("usenet job kept (fail=%v): %q", e.fail, why)
		}
		dr.Protocol = "torrent"
		if why := m.keepFilesWhy(context.Background(), e, dr, j); why == "" {
			t.Fatalf("torrent with the same client answer was not kept (fail=%v)", e.fail)
		}
	}
}
