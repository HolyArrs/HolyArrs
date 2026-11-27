package library

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// a movie whose lookup failed under load is marked fetched (or
// flagged as dropped) with no poster; the scheduled refresh must pick it
// up again instead of leaving it posterless until re-added.
func TestRefreshPlanRetriesMoviesWithNoPoster(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	addFolder(t, s)
	svc := &Service{Store: s, Now: func() time.Time { return now }}
	add := func(tmdb string) int64 {
		id, err := AddMovie(ctx, s, metadata.Movie{Name: "M" + tmdb, Year: 2020, IDs: map[string]string{"tmdb": tmdb}, CinemaDate: d("2020-01-01")}, AddOptions{}, nil, now, domain.System)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	set := func(id int64, q string, args ...any) {
		err := s.Write(ctx, func(tx *sql.Tx) error {
			_, err := tx.Exec("UPDATE titles SET "+q+" WHERE id = ?", append(args, id)...)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	day := database.FormatTime(now.Add(-24 * time.Hour))
	recent := database.FormatTime(now.Add(-time.Hour))
	dropped := add("1")    // not found under load: flagged, fetched, no poster
	posterless := add("2") // fetched but the poster download failed
	withPoster := add("3") // fetched with a poster: the rules say no
	justNow := add("4")    // no poster but fetched an hour ago: wait
	set(dropped, "refreshed_at = ?, dropped_at = ?", day, day)
	set(posterless, "refreshed_at = ?", day)
	set(withPoster, `refreshed_at = ?, artwork = '["poster:x.jpg"]'`, day)
	set(justNow, "refreshed_at = ?", recent)

	for _, last := range []*time.Time{nil, ptrTime(now.Add(-48 * time.Hour))} {
		ids, why, err := svc.RefreshPlan(ctx, false, last)
		if err != nil {
			t.Fatal(err)
		}
		in := map[int64]bool{}
		for _, id := range ids {
			in[id] = true
		}
		if !in[dropped] || !in[posterless] || in[withPoster] || in[justNow] {
			t.Fatalf("last %v: plan %v %v", last, ids, why)
		}
		if why[posterless] != "no poster yet" {
			t.Fatalf("why %v", why)
		}
	}
}

func ptrTime(t time.Time) *time.Time { return &t }
