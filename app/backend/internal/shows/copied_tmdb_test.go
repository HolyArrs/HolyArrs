package shows

import (
	"context"
	"database/sql"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// mazeChef is a show as TVmaze (or TVDB) gives it, with a TMDB id that is
// really its own TVDB id: TMDB 82817 is another show (Go Go Squid!), the
// show itself is TMDB 24285.
func mazeChef(tmdb string) metadata.Show {
	sh := labShow("tvmaze", "7201")
	sh.Name = "MasterChef: The Professionals"
	sh.IDs = map[string]string{"tvmaze": "7201", "tvdb": "82817", "imdb": "tt1352423"}
	if tmdb != "" {
		sh.IDs["tmdb"] = tmdb
	}
	return sh
}

func storedTMDB(t *testing.T, svc *Service, id int64) string {
	t.Helper()
	var v string
	_ = svc.Store.Read().QueryRowContext(context.Background(), "SELECT COALESCE(json_extract(source_ids, '$.tmdb'), '') FROM titles WHERE id = ?", id).Scan(&v)
	return v
}

// A show whose details source lists its own TVDB id as its TMDB id is
// stored without that id, so its art is never asked for as TMDB 82817.
func TestAddDropsTMDBCopiedFromTVDB(t *testing.T) {
	svc, s, _, maze := addShowService(t)
	ctx := context.Background()
	addShowFolder(t, s)
	maze.set(mazeChef("82817"), "tvmaze:7201", "tvdb:82817")
	id, _, err := svc.AddShow(ctx, "tvmaze:7201", ShowAddOptions{Source: "tvmaze"}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if got := storedTMDB(t, svc, id); got == "82817" {
		t.Fatalf("tmdb id is the TVDB id: %q", got)
	}
}

// When TMDB knows the show by its TVDB id, the right TMDB id is stored.
func TestAddFindsRightTMDBForCopiedID(t *testing.T) {
	svc, s, tmdb, maze := addShowService(t)
	ctx := context.Background()
	addShowFolder(t, s)
	maze.set(mazeChef("82817"), "tvmaze:7201", "tvdb:82817")
	right := mazeChef("24285")
	right.Source = "tmdb"
	tmdb.set(right, "tvdb:82817", "tmdb:24285")
	id, _, err := svc.AddShow(ctx, "tvmaze:7201", ShowAddOptions{Source: "tvmaze"}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if got := storedTMDB(t, svc, id); got != "24285" {
		t.Fatalf("tmdb id %q, want 24285", got)
	}
}

// A TVmaze id copied into the TMDB field is dropped the same way.
func TestAddDropsTMDBCopiedFromTVmaze(t *testing.T) {
	svc, s, _, maze := addShowService(t)
	ctx := context.Background()
	addShowFolder(t, s)
	sh := mazeChef("7201")
	delete(sh.IDs, "tvdb")
	maze.set(sh, "tvmaze:7201")
	id, _, err := svc.AddShow(ctx, "tvmaze:7201", ShowAddOptions{Source: "tvmaze"}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if got := storedTMDB(t, svc, id); got != "" {
		t.Fatalf("tmdb id %q, want none", got)
	}
}

// A show already stored with the copied id heals on its next refresh: the
// wrong id goes, and the right one comes in when TMDB knows the show.
func TestRefreshHealsCopiedTMDB(t *testing.T) {
	for _, c := range []struct {
		name  string
		known bool
		want  string
	}{{"tmdb knows it", true, "24285"}, {"tmdb does not", false, ""}} {
		t.Run(c.name, func(t *testing.T) {
			svc, s, tmdb, maze := addShowService(t)
			ctx := context.Background()
			addShowFolder(t, s)
			maze.set(mazeChef(""), "tvmaze:7201", "tvdb:82817")
			id, _, err := svc.AddShow(ctx, "tvmaze:7201", ShowAddOptions{Source: "tvmaze"}, domain.System)
			if err != nil {
				t.Fatal(err)
			}
			// The row as an older version left it.
			if err := s.Write(ctx, func(tx *sql.Tx) error {
				_, err := tx.Exec("UPDATE titles SET source_ids = json_set(source_ids, '$.tmdb', '82817') WHERE id = ?", id)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			if c.known {
				right := mazeChef("24285")
				right.Source = "tmdb"
				tmdb.set(right, "tvdb:82817", "tmdb:24285")
			}
			if _, err := svc.RefreshShow(ctx, id, domain.System, nil); err != nil {
				t.Fatal(err)
			}
			if got := storedTMDB(t, svc, id); got != c.want {
				t.Fatalf("tmdb id after refresh %q, want %q", got, c.want)
			}
		})
	}
}

// A show that follows TMDB and was stored with a wrong TMDB id is never
// refreshed into that other show (its name, episodes and pictures): the
// refresh finds the show by its TVDB id instead and keeps the right one.
func TestRefreshOnTMDBIgnoresWrongStoredID(t *testing.T) {
	for _, wrong := range []string{"82817", "99999"} {
		t.Run(wrong, func(t *testing.T) {
			svc, s, tmdb, _ := addShowService(t)
			ctx := context.Background()
			addShowFolder(t, s)
			right := mazeChef("24285")
			right.Source = "tmdb"
			tmdb.set(right, "tmdb:24285", "tvdb:82817")
			id, _, err := svc.AddShow(ctx, "tvdb:82817", ShowAddOptions{Source: "tmdb"}, domain.System)
			if err != nil {
				t.Fatal(err)
			}
			other := labShow("tmdb", wrong)
			other.Name = "Go Go Squid!"
			other.IDs["tvdb"] = "360556"
			tmdb.set(other, "tmdb:"+wrong)
			if err := s.Write(ctx, func(tx *sql.Tx) error {
				_, err := tx.Exec("UPDATE titles SET source_ids = json_set(source_ids, '$.tmdb', ?) WHERE id = ?", wrong, id)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			if _, err := svc.RefreshShow(ctx, id, domain.System, nil); err != nil {
				t.Fatal(err)
			}
			var name, tvdb string
			_ = s.Read().QueryRowContext(ctx, "SELECT name, json_extract(source_ids, '$.tvdb') FROM titles WHERE id = ?", id).Scan(&name, &tvdb)
			if got := storedTMDB(t, svc, id); got != "24285" || name != "MasterChef: The Professionals" || tvdb != "82817" {
				t.Fatalf("after refresh: tmdb %q, name %q, tvdb %q", got, name, tvdb)
			}
		})
	}
}

// TMDB's own answer is never second-guessed: its id stays even when it
// happens to equal the TVDB id.
func TestTMDBAnswerKeepsItsOwnID(t *testing.T) {
	svc, s, tmdb, _ := addShowService(t)
	ctx := context.Background()
	addShowFolder(t, s)
	sh := labShow("tmdb", "555")
	sh.IDs["tvdb"] = "555"
	tmdb.set(sh, "tmdb:555")
	id, _, err := svc.AddShow(ctx, "tmdb:555", ShowAddOptions{Source: "tmdb"}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if got := storedTMDB(t, svc, id); got != "555" {
		t.Fatalf("tmdb id %q, want 555", got)
	}
}
