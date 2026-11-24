package requests

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// withFile adds a movie to the Library with one file of size bytes.
func withFile(t *testing.T, r *rig, name, tmdb string, size int64) int64 {
	t.Helper()
	ctx := context.Background()
	tid, _, err := r.svc.Library.Add(ctx, movie(name, 2024, tmdb), library.AddOptions{}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	vs, err := database.Select[mediafiles.Version](ctx, r.st.Read(), "versions", "title_id = ?", tid)
	if err != nil || len(vs) == 0 {
		t.Fatalf("versions %v", err)
	}
	if size == 0 {
		return tid
	}
	err = r.st.Write(ctx, func(tx *sql.Tx) error {
		f := &mediafiles.File{SlotIDs: []int64{}, VersionID: vs[0].ID, RelativePath: name + ".mkv", Size: size, Languages: []string{}, MediaInfo: map[string]any{},
			Origin: "imported", PlacedBy: "none", Role: "main", State: "present"}
		_, err := database.Insert(tx, "files", f, domain.System)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return tid
}

func TestCleanupListsOnlyTitlesOnMyServices(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	r.svc.CleanupWait = true
	big := withFile(t, r, "Dune: Part Two", "693134", 9000)
	small := withFile(t, r, "Paper Harbour", "900002", 100)
	withFile(t, r, "Quiet Orchard", "900003", 5000) // not on a service
	withFile(t, r, "Salt Choir", "900005", 0)       // no files
	on := map[string][]metadata.WatchService{"US": {netflix}}
	r.svc.Watch = func(_ context.Context, kind, id string) (map[string][]metadata.WatchService, error) {
		switch id {
		case "693134", "900002", "900005":
			return on, nil
		}
		return map[string][]metadata.WatchService{}, nil
	}
	for _, c := range []struct {
		name     string
		region   string
		services []int64
		err      error
	}{
		{"no country", "", []int64{8}, ErrStreamingRegion},
		{"no services", "US", nil, ErrNoMyServices},
	} {
		s, _ := ReadSettings(ctx, r.st.Read())
		s.StreamingRegion, s.StreamingServices = c.region, c.services
		_ = SaveSettings(ctx, r.st, s, domain.System)
		if _, err := r.svc.StartCleanup(ctx); !errors.Is(err, c.err) {
			t.Errorf("%s: %v", c.name, err)
		}
	}
	s, _ := ReadSettings(ctx, r.st.Read())
	s.StreamingRegion, s.StreamingServices = "US", []int64{8}
	_ = SaveSettings(ctx, r.st, s, domain.System)
	got, err := r.svc.StartCleanup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.Running || got.Total != 3 || got.Checked != 3 || len(got.Found) != 2 {
		t.Fatalf("check %+v", got)
	}
	if got.Found[0].ID != big || got.Found[1].ID != small || got.Found[0].Service != "Netflix" || got.Found[0].Bytes != 9000 {
		t.Fatalf("found %+v", got.Found)
	}
	// Only a suggestion: every title is still in the Library.
	if n, _ := database.Select[mediafiles.Title](ctx, r.st.Read(), "titles", "state != 'removed'"); len(n) != 4 {
		t.Fatalf("titles left %d", len(n))
	}
}

func TestCleanupCountsFailedReads(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	r.svc.CleanupWait = true
	withFile(t, r, "Dune: Part Two", "693134", 10)
	r.svc.Watch = func(context.Context, string, string) (map[string][]metadata.WatchService, error) {
		return nil, errors.New("down")
	}
	s, _ := ReadSettings(ctx, r.st.Read())
	s.StreamingRegion, s.StreamingServices = "US", []int64{8}
	_ = SaveSettings(ctx, r.st, s, domain.System)
	got, err := r.svc.StartCleanup(ctx)
	if err != nil || got.Failed != 1 || len(got.Found) != 0 {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestCleanupTenThousandTitles(t *testing.T) {
	r := newRig(t)
	titles := make([]mediafiles.TitleOnDisk, 10000)
	for i := range titles {
		titles[i] = mediafiles.TitleOnDisk{ID: int64(i + 1), Kind: "movie", TMDBID: "1", Bytes: int64(10000 - i)}
	}
	r.svc.Watch = func(_ context.Context, _, _ string) (map[string][]metadata.WatchService, error) {
		return map[string][]metadata.WatchService{"US": {netflix}}, nil
	}
	r.svc.cleanup.cur.Running = true
	r.svc.runCleanup(titles, Settings{StreamingRegion: "US", StreamingServices: []int64{8}})
	got := r.svc.CleanupState()
	if got.Running || got.Checked != 10000 || len(got.Found) != 10000 || got.Found[0].ID != 1 || got.Found[9999].ID != 10000 {
		t.Fatalf("checked %d found %d", got.Checked, len(got.Found))
	}
}
