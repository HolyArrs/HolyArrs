package downloads

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func setKindCategories(t *testing.T, w *world, movies, shows, music string) {
	t.Helper()
	if err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE downloaders SET category_movies = ?, category_shows = ?, category_music = ?", movies, shows, music)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestGrabUsesTheCategoryForTheTitlesKind(t *testing.T) {
	w, m := setup(t)
	setKindCategories(t, w, "films", "tv", "")
	if _, err := m.Grab(context.Background(), w.slotID, w.relID, domain.System); err != nil {
		t.Fatal(err)
	}
	if len(w.fake.added) != 1 || w.fake.added[0]["category"] != "films" {
		t.Fatalf("added %+v, want category films", w.fake.added)
	}
}

func TestGrabOfAShowUsesTheShowCategory(t *testing.T) {
	w, m := setup(t)
	setKindCategories(t, w, "films", "tv", "")
	if err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE titles SET kind = 'show'")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Grab(context.Background(), w.slotID, w.relID, domain.System); err != nil {
		t.Fatal(err)
	}
	if len(w.fake.added) != 1 || w.fake.added[0]["category"] != "tv" {
		t.Fatalf("added %+v, want category tv", w.fake.added)
	}
}

func TestGrabWithEmptyKindCategoryUsesTheMainOne(t *testing.T) {
	w, m := setup(t)
	setKindCategories(t, w, "", "tv", "songs")
	if _, err := m.Grab(context.Background(), w.slotID, w.relID, domain.System); err != nil {
		t.Fatal(err)
	}
	if len(w.fake.added) != 1 || w.fake.added[0]["category"] != "holyarrs" {
		t.Fatalf("added %+v, want the main category", w.fake.added)
	}
}

func TestSyncFollowsAJobInAKindCategory(t *testing.T) {
	fastTimers(t)
	j := own(0.4)
	j.Category = "films"
	r := newRig(t, j)
	if err := r.db.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE downloaders SET category_movies = 'films'")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	r.m.Active = 200 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { r.s.Around(ctx, r.m.Run); close(done) }()
	waitUntil(t, "the job in the movie category to reach the row", func() bool { st, p := r.state(t); return st == "downloading" && p == 0.4 })
	cancel()
	<-done
}

func TestJobFiltersAcceptEveryCategory(t *testing.T) {
	d := domain.Downloader{Category: "holyarrs", CategoryMovies: "films", CategoryShows: "holyarrs", CategoryMusic: "songs"}
	if got := d.Categories(); len(got) != 3 || got[0] != "holyarrs" || got[1] != "films" || got[2] != "songs" {
		t.Fatalf("categories %v", got)
	}
	if d.CategoryFor("movie") != "films" || d.CategoryFor("show") != "holyarrs" || d.CategoryFor("artist") != "songs" || d.CategoryFor("") != "holyarrs" {
		t.Fatal("CategoryFor picked the wrong one")
	}
	jobs := []Job{{ID: "a", Category: "holyarrs"}, {ID: "b", Category: "films"}, {ID: "c", Category: "other"}, {ID: "d", Category: "songs"}}
	if got := inCategory(jobs, d.Categories()); len(got) != 3 {
		t.Fatalf("inCategory kept %v", got)
	}
	v := SyncView{Items: map[string]Job{}}
	for _, j := range jobs {
		v.Items[j.ID] = j
	}
	if got := snapshotJobs(v, d.Categories()); len(got) != 3 {
		t.Fatalf("snapshotJobs kept %v", got)
	}
	c := newFakeClient(jobs...)
	got, err := listCategories(context.Background(), c, d.Categories())
	if err != nil || len(got) != 3 {
		t.Fatalf("listCategories %v %v", got, err)
	}
}

func TestKindCategoryValidation(t *testing.T) {
	str := func(s string) *string { return &s }
	base := func() Input {
		return Input{Name: "q", Adapter: "qbittorrent", URL: "http://10.0.0.7:8080"}
	}
	var d domain.Downloader
	in := base()
	in.CategoryMovies, in.CategoryShows, in.CategoryMusic = str(" films "), str(""), str("songs")
	if err := fill(in, &d, true); err != nil {
		t.Fatal(err)
	}
	if d.Category != "holyarrs" || d.CategoryMovies != "films" || d.CategoryShows != "" || d.CategoryMusic != "songs" {
		t.Fatalf("saved %+v", d)
	}
	for _, bad := range []string{"a/b", `a\b`, strings.Repeat("x", 65)} {
		in := base()
		in.CategoryShows = str(bad)
		var d domain.Downloader
		if err := fill(in, &d, true); !errors.Is(err, ErrCategory) {
			t.Fatalf("%q: %v, want ErrCategory", bad, err)
		}
	}
	in = Input{Name: "d", Adapter: "deluge", URL: "http://10.0.0.7:8112", CategoryMusic: str("Music")}
	if err := fill(in, &d, true); !errors.Is(err, ErrDelugeLabel) {
		t.Fatalf("deluge: %v, want ErrDelugeLabel", err)
	}
}
