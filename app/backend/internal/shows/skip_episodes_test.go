package shows

import (
	"context"
	"errors"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

// SkipEpisodes switches single episodes off and leaves the season and its
// other episodes as they were.
func TestSkipEpisodes(t *testing.T) {
	svc, s, _, _ := addShowService(t)
	ctx := context.Background()
	addShowFolder(t, s)
	id, _, err := svc.AddShow(ctx, "tmdb:100", ShowAddOptions{}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		season int
		eps    []int
		err    error
	}{
		{1, nil, nil}, {1, []int{2}, nil}, {2, []int{1, 99}, nil}, {7, []int{1}, library.ErrNoSuchSeason},
	}
	for _, c := range cases {
		if err := svc.SkipEpisodes(ctx, id, c.season, c.eps, domain.System); !errors.Is(err, c.err) {
			t.Fatalf("season %d %v: %v", c.season, c.eps, err)
		}
	}
	eps := episodes(t, s, id)
	if !eps["S01E01"].Keep || eps["S01E02"].Keep || eps["S02E01"].Keep {
		t.Fatalf("episodes %+v", eps)
	}
	ss, _ := Seasons(ctx, s.Read(), id, now)
	if !ss[1].KeepUpToDate || !ss[2].KeepUpToDate {
		t.Fatalf("seasons %+v", ss)
	}
}
