package library

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// An import stopped by a restart after its titles were added but before
// they were followed is run again after the restart: the folders it already
// added count as brought in, and the person follows every one of them.
func TestAdoptRunAgainAfterARestartFollowsEveryTitle(t *testing.T) {
	s := open(t)
	lf := addFolder(t, s)
	adoptTree(t, lf.Path)
	ctx := context.Background()
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO users(id, created_at, created_by, updated_at, updated_by, name, role, password_hash) VALUES (7, 't', '{}', 't', '{}', 'ana', 'owner', 'x')`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	svc, _, _ := runQueue(t, s, adoptTMDB)
	ana := domain.Actor{Kind: "user", ID: 7}
	choices := []AdoptChoice{{Folder: "Dune - Part Two (2024)", TMDB: "693134"}, {Folder: "Heat (1995) {tmdb-949}", TMDB: "949"}}
	// The first run added both titles, then the restart stopped it before the follows were written.
	if _, err := svc.Adopt(ctx, lf.ID, choices, ana); err != nil {
		t.Fatal(err)
	}
	// The run after the restart.
	if _, err := svc.AdoptLater(ctx, lf.ID, choices, true, ana); err != nil {
		t.Fatal(err)
	}
	js := waitJobs(t, s, 1)
	var follows, titles int
	_ = s.Read().QueryRow("SELECT count(*) FROM follows WHERE user_id = 7").Scan(&follows)
	_ = s.Read().QueryRow("SELECT count(*) FROM titles").Scan(&titles)
	if titles != 2 || follows != 2 {
		t.Fatalf("titles %d, follows %d: every title brought in must be followed", titles, follows)
	}
	// What the run again tells the page: both are in the Library, not two failures.
	var out struct {
		Adopted int           `json:"adopted"`
		Results []AdoptResult `json:"results"`
	}
	raw, _ := json.Marshal(js[len(js)-1].Result)
	_ = json.Unmarshal(raw, &out)
	if out.Adopted != 2 || len(out.Results) != 2 || out.Results[0].TitleID == 0 || out.Results[0].Error != "" {
		t.Fatalf("the run again says %s", raw)
	}
}

// An import reports how far it is folder by folder, so the page's number
// moves inside a part instead of only between parts.
func TestAdoptTellsItsProgress(t *testing.T) {
	s := open(t)
	lf := addFolder(t, s)
	adoptTree(t, lf.Path)
	svc := &Service{Store: s, Metadata: adoptTMDB, Now: func() time.Time { return now }}
	var seen [][2]int
	ctx := WithScanProgress(context.Background(), func(done, total int) { seen = append(seen, [2]int{done, total}) })
	if _, err := svc.Adopt(ctx, lf.ID, []AdoptChoice{{Folder: "Dune - Part Two (2024)", TMDB: "693134"}, {Folder: "Heat (1995) {tmdb-949}", TMDB: "949"}, {Folder: "Empty Movie (2001)", TMDB: "5001"}}, domain.System); err != nil {
		t.Fatal(err)
	}
	if len(seen) == 0 || seen[len(seen)-1] != [2]int{3, 3} || seen[0] != [2]int{0, 3} {
		t.Fatalf("progress %v", seen)
	}
}
