package shows

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// flakyGroups lists the orders and fails the read of one after a number
// of good reads; it notes whether a read asked past the cache.
type flakyGroups struct {
	fakeGroups
	listErr  error
	goodLeft int
	fresh    []bool
}

func (f *flakyGroups) EpisodeGroups(ctx context.Context, tmdbID string) ([]metadata.EpisodeGroupInfo, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.fakeGroups.EpisodeGroups(ctx, tmdbID)
}

func (f *flakyGroups) EpisodeGroup(ctx context.Context, id string) (metadata.EpisodeGroup, error) {
	f.fresh = append(f.fresh, strings.Contains(fmt.Sprint(ctx), "noCacheKey"))
	if f.goodLeft == 0 {
		return metadata.EpisodeGroup{}, errors.New("TMDB timed out")
	}
	f.goodLeft--
	return f.fakeGroups.EpisodeGroup(ctx, id)
}

func groupOf(t *testing.T, s *database.Store, id int64) string {
	t.Helper()
	var x mediafiles.Title
	if err := database.Get(context.Background(), s.Read(), "titles", id, &x); err != nil {
		t.Fatal(err)
	}
	if x.EpisodeGroupID == nil {
		return ""
	}
	return *x.EpisodeGroupID
}

func absOf(t *testing.T, s *database.Store, id int64) map[string]string {
	out := map[string]string{}
	for k, r := range episodes(t, s, id) {
		out[k] = "-"
		if r.Abs != nil {
			out[k] = fmt.Sprint(*r.Abs)
		}
	}
	return out
}

func TestEpisodeGroupReadFailures(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	src := &fakeShows{}
	sh := trappedShow()
	src.set(sh, "tmdb:64485")
	ti := seedShow(t, s, sh, library.SeasonsAll)
	groups := &flakyGroups{listErr: errors.New("TMDB down")}
	svc := showService(s, metadata.ShowSources{"tmdb": src})
	svc.EpisodeGroups = groups
	cut := trappedCut

	// An outage is not a bad order: it answers 502 and saves nothing.
	_, err := svc.Edit(ctx, ti.ID, library.TitleChange{EpisodeGroupID: &cut}, false, domain.System)
	if errors.Is(err, library.ErrBadEpisodeGroup) || !errors.Is(err, library.ErrEpisodeGroupUnread) {
		t.Fatalf("list outage: %v", err)
	}
	rec := httptest.NewRecorder()
	ShowError(rec, err, nil)
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "TMDB down") {
		t.Fatalf("list outage answered %d %s", rec.Code, rec.Body.String())
	}

	// The order itself cannot be read: nothing is saved.
	groups.listErr = nil
	if _, err := svc.Edit(ctx, ti.ID, library.TitleChange{EpisodeGroupID: &cut}, false, domain.System); !errors.Is(err, library.ErrEpisodeGroupUnread) {
		t.Fatalf("group outage: %v", err)
	}
	if g := groupOf(t, s, ti.ID); g != "" {
		t.Fatalf("saved %q although the order was not read", g)
	}

	// The order reads for the edit but not for the refresh: the order goes back.
	groups.goodLeft = 1
	if _, err := svc.Edit(ctx, ti.ID, library.TitleChange{EpisodeGroupID: &cut}, false, domain.System); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Regroup(ctx, ti.ID, nil, domain.System); err == nil {
		t.Fatal("a refresh that could not read the order succeeded")
	}
	if g := groupOf(t, s, ti.ID); g != "" {
		t.Fatalf("order kept after the refresh failed: %q", g)
	}

	// A good switch, read past the cache.
	groups.goodLeft, groups.fresh = 2, nil
	if _, err := svc.Edit(ctx, ti.ID, library.TitleChange{EpisodeGroupID: &cut}, false, domain.System); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Regroup(ctx, ti.ID, nil, domain.System); err != nil {
		t.Fatal(err)
	}
	if len(groups.fresh) != 2 || !groups.fresh[1] {
		t.Fatalf("the refresh read the order from the cache: %v", groups.fresh)
	}
	wantAbs := absOf(t, s, ti.ID)
	if wantAbs["S03E04"] != "3" || wantAbs["S03E07"] != "-" {
		t.Fatalf("group absolutes %v", wantAbs)
	}

	// A failed read on a grouped show keeps the stored absolute numbers too.
	if _, err := svc.RefreshShow(ctx, ti.ID, domain.System, nil); err != nil {
		t.Fatal(err)
	}
	if got := absOf(t, s, ti.ID); fmt.Sprint(got) != fmt.Sprint(wantAbs) {
		t.Fatalf("absolutes after a failed read %v, want %v", got, wantAbs)
	}
	if got := groupedRows(t, s, ti.ID); got[4].Alt != [2]int{3, 3} {
		t.Fatalf("numbers after a failed read %+v", got)
	}

	// Back to TMDB order while the anime numbers cannot be read: the
	// switch fails and the group and its numbers stay.
	stale := trappedShow()
	stale.AltStale = true
	src.set(stale, "tmdb:64485")
	none := ""
	if _, err := svc.Edit(ctx, ti.ID, library.TitleChange{EpisodeGroupID: &none}, false, domain.System); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Regroup(ctx, ti.ID, &cut, domain.System); err == nil {
		t.Fatal("switch back kept the group's numbers as anime numbers")
	}
	if g := groupOf(t, s, ti.ID); g != cut {
		t.Fatalf("group after the failed switch back: %q", g)
	}
	if got := groupedRows(t, s, ti.ID); got[4].Alt != [2]int{3, 3} {
		t.Fatalf("numbers after the failed switch back %+v", got)
	}

	// With the anime numbers readable, the switch back clears them.
	src.set(trappedShow(), "tmdb:64485")
	if _, err := svc.Edit(ctx, ti.ID, library.TitleChange{EpisodeGroupID: &none}, false, domain.System); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Regroup(ctx, ti.ID, &cut, domain.System); err != nil {
		t.Fatal(err)
	}
	if got := groupedRows(t, s, ti.ID); got[4].Alt != [2]int{} || groupOf(t, s, ti.ID) != "" {
		t.Fatalf("back on TMDB order %+v", got)
	}
}
