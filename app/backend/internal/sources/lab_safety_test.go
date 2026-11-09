package sources

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// labFeed serves the lab indexer's saved feed and caps, and skips the test when the files are missing. HOLYARRS_LAB_FEED overrides the path.
func labFeed(t *testing.T) string {
	t.Helper()
	path := os.Getenv("HOLYARRS_LAB_FEED")
	if path == "" {
		path = filepath.Join("..", "..", "..", "..", "..", "..", "work", "research", "lab", "fake-indexer", "results.xml")
	}
	feed, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("no lab feed: %v", err)
	}
	caps, err := os.ReadFile(filepath.Join(filepath.Dir(path), "caps.xml"))
	if err != nil {
		t.Skipf("no lab caps: %v", err)
	}
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("t") == "caps" {
			_, _ = w.Write(caps)
			return
		}
		_, _ = w.Write(feed)
	})
	return srv.URL + "/api"
}

// labExec runs one write statement and fails the test on error.
func labExec(t *testing.T, s *database.Store, q string, args ...any) {
	t.Helper()
	if err := s.Write(context.Background(), func(tx *sql.Tx) error { _, err := tx.Exec(q, args...); return err }); err != nil {
		t.Fatal(err)
	}
}

// day formats a calendar date the way the database stores times.
func day(s string) string {
	d, _ := time.Parse("2006-01-02", s)
	return database.FormatTime(d)
}

func TestLabFeed_MovieEarlyAndFake(t *testing.T) {
	f := seed(t, labFeed(t))
	labExec(t, f.s, "UPDATE titles SET cinema_date = ?", day("2024-03-01"))
	if j := runKind(t, f, &Searcher{Store: f.s}, jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: f.slotID}, nil); j.State != "done" {
		t.Fatalf("job %s %+v", j.State, j.Error)
	}
	wantRefused(t, f, "Dune.Part.Two.2024.2160p.WEB-DL.DV.H265-EARLY", "posted_early")
	wantRefused(t, f, "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS.mkv.exe", "executable")
	if r := relByName(t, f, "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS"); r.State != "chosen" {
		t.Fatalf("the lab's Dune is %s %v", r.State, reasonCodes(r))
	}
}

func TestLabFeed_EpisodeEarlyAndFake(t *testing.T) {
	f := seedShow(t, labFeed(t), nil)
	labExec(t, f.s, "UPDATE titles SET name = 'Arcane', sort_name = 'arcane', year = 2021, source_ids = '{\"tvdb\":\"371028\",\"imdb\":\"tt11126994\"}' WHERE id = ?", f.titleID)
	labExec(t, f.s, "UPDATE items SET air_date = ?, air_day = '2021-11-06', runtime = 20 WHERE id = ?", database.FormatTime(time.Date(2021, 11, 6, 19, 0, 0, 0, time.UTC)), f.items["S01E01"])
	if j := runKind(t, f.fixture, &Searcher{Store: f.s}, jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: f.slots["S01E01"]}, nil); j.State != "done" {
		t.Fatalf("job %s %+v", j.State, j.Error)
	}
	wantRefused(t, f.fixture, "Arcane.S01E01.1080p.WEB.H264-EARLY", "posted_early")
	wantRefused(t, f.fixture, "Arcane.S01E01.1080p.WEB.H264-FAKE", "too_small")
	if r := relByName(t, f.fixture, "Arcane.S01E01.1080p.WEB.H264-HOLYARRS"); r.State != "chosen" {
		t.Fatalf("the lab's Arcane is %s %v", r.State, reasonCodes(r))
	}
}

func TestLabFeed_AlbumEarlyAndFake(t *testing.T) {
	f := seedArtist(t, labFeed(t))
	labExec(t, f.s, "UPDATE titles SET name = 'Lab Lanterns', sort_name = 'lab lanterns' WHERE id = ?", f.titleID)
	labExec(t, f.s, "UPDATE parts SET name = 'Harbour Light', release_date = ? WHERE id = ?", day("2024-06-07"), f.albums[lta])
	if j := f.albumJob(t, &Searcher{Store: f.s}, lta, nil); j.State != "done" {
		t.Fatalf("job %s %+v", j.State, j.Error)
	}
	wantRefused(t, f.fixture, "Lab Lanterns - Harbour Light (2024) [FLAC] [WEB]", "posted_early")
	wantRefused(t, f.fixture, "Lab Lanterns - Harbour Light (2024) [FLAC] PASSWORDED", "password_protected")
	rs, _ := database.Select[domain.Release](context.Background(), f.s.Read(), "releases", "state = 'chosen'")
	if len(rs) != 1 || rs[0].Name != "Lab Lanterns - Harbour Light (2024) [FLAC]" {
		t.Fatalf("chosen %+v", rs)
	}
}
