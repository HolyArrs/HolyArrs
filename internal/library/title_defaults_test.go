package library

import (
	"context"
	"errors"
	"testing"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/jobs"
	"github.com/CoderLuii/holyarrs/internal/store"
)

func TestTitleDefaultsBuiltIn(t *testing.T) {
	s := open(t)
	d, saved, err := ReadTitleDefaults(context.Background(), s.Read())
	if err != nil {
		t.Fatal(err)
	}
	want := TitleDefaults{Movie: KindDefaults{KeepUpToDate: true, SearchOnAdd: true}, Show: KindDefaults{KeepUpToDate: true}}
	if d != want || len(saved) != 0 {
		t.Fatalf("built in %+v saved %v", d, saved)
	}
}

func TestTitleDefaultsSaveAndValidate(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	movies := addFolder(t, s)
	shows := addShowFolder(t, s)
	ge := presetID(t, s, domain.GoodEverydayName)
	lossless := presetID(t, s, domain.LosslessFirstName)
	bad := []struct {
		d    TitleDefaults
		want error
	}{
		{TitleDefaults{Movie: KindDefaults{LibraryFolderID: shows.ID}}, ErrBadFolder},
		{TitleDefaults{Show: KindDefaults{LibraryFolderID: movies.ID}}, ErrBadFolder},
		{TitleDefaults{Movie: KindDefaults{LibraryFolderID: 999}}, ErrBadFolder},
		{TitleDefaults{Movie: KindDefaults{PickRulesID: 999}}, ErrNoPickRules},
		{TitleDefaults{Movie: KindDefaults{PickRulesID: lossless}}, ErrNotVideoRules},
		{TitleDefaults{Show: KindDefaults{PickRulesID: lossless}}, ErrNotVideoRules},
	}
	for _, b := range bad {
		if err := SetTitleDefaults(ctx, s, b.d, domain.System); !errors.Is(err, b.want) {
			t.Errorf("%+v: got %v want %v", b.d, err, b.want)
		}
	}
	if _, saved, _ := ReadTitleDefaults(ctx, s.Read()); len(saved) != 0 {
		t.Fatalf("a refused save stored %v", saved)
	}
	d := TitleDefaults{Movie: KindDefaults{LibraryFolderID: movies.ID, PickRulesID: ge, KeepUpToDate: false, SearchOnAdd: false},
		Show: KindDefaults{LibraryFolderID: shows.ID, PickRulesID: ge, KeepUpToDate: true, SearchOnAdd: true}}
	if err := SetTitleDefaults(ctx, s, d, domain.System); err != nil {
		t.Fatal(err)
	}
	got, saved, err := ReadTitleDefaults(ctx, s.Read())
	if err != nil || got != d || len(saved) != 8 || !saved["movie_search_on_add"] || !saved["show_pick_rules_id"] {
		t.Fatalf("read back %+v saved %v err %v", got, saved, err)
	}
}

// TestAddShowSearchOnAdd: a show added with search_on_add gets, after its
// folder scan, an automatic search of each season looking for a copy: no
// by_hand, so the release gate still holds back what is not out yet.
func TestAddShowSearchOnAdd(t *testing.T) {
	svc, s, _, _ := addShowService(t)
	ctx := context.Background()
	addShowFolder(t, s)
	id, jobID, err := svc.AddShow(ctx, "tmdb:100", ShowAddOptions{SearchOnAdd: true}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	var scan domain.Job
	if err := store.Get(ctx, s.Read(), "jobs", jobID, &scan); err != nil {
		t.Fatal(err)
	}
	if scan.Kind != KindScanTitle {
		t.Fatalf("first job %s", scan.Kind)
	}
	if n := countJobs(t, s, "search_season"); n != 0 {
		t.Fatalf("search before the scan: %d", n)
	}
	if err := svc.next(ctx, &jobs.Run{Job: scan}, id); err != nil {
		t.Fatal(err)
	}
	ss, err := store.Select[domain.Job](ctx, s.Read(), "jobs", "kind = 'search_season' ORDER BY id")
	if err != nil || len(ss) == 0 {
		t.Fatalf("season searches %v %v", ss, err)
	}
	for _, j := range ss {
		if _, hand := j.Params["by_hand"]; hand || j.Params["kept_only"] != nil || j.Params["version_id"] == nil || j.Subject == nil || j.Subject.Type != "part" {
			t.Fatalf("season search %+v", j)
		}
	}
	// Without search_on_add the scan is the last step.
	svc2, s2, _, _ := addShowService(t)
	addShowFolder(t, s2)
	id2, jobID2, err := svc2.AddShow(ctx, "tmdb:100", ShowAddOptions{}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	var scan2 domain.Job
	_ = store.Get(ctx, s2.Read(), "jobs", jobID2, &scan2)
	if err := svc2.next(ctx, &jobs.Run{Job: scan2}, id2); err != nil {
		t.Fatal(err)
	}
	if n := countJobs(t, s2, "search_season"); n != 0 {
		t.Fatalf("searched without search_on_add: %d", n)
	}
}

func countJobs(t *testing.T, s *store.Store, kind string) int {
	t.Helper()
	var n int
	if err := s.Read().QueryRow("SELECT count(*) FROM jobs WHERE kind = ?", kind).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
