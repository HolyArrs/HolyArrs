package shows

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// squidSource is a TMDB stand-in whose find by TVDB 82817 lands on TMDB
// 82817 (Go Go Squid!), a show that lists that same number as its TVDB id,
// while the real MasterChef: The Professionals is TMDB 24285.
func squidSource(tmdb *fakeShows) {
	squid := labShow("tmdb", "82817")
	squid.Name, squid.Year = "Go Go Squid!", 2019
	squid.IDs = map[string]string{"tmdb": "82817", "tvdb": "82817"}
	tmdb.set(squid, "tmdb:82817", "tvdb:82817")
	chef := labShow("tmdb", "24285")
	chef.Name, chef.Year = "MasterChef The Professionals", 2008
	chef.IDs = map[string]string{"tmdb": "24285", "tvdb": "82817", "imdb": "tt1281256"}
	tmdb.set(chef, "tmdb:24285", "imdb:tt1281256")
}

// Importing a show folder tagged with its TVDB id never takes the TMDB
// show whose own id is that number: the folder is matched by its name.
func TestImportTaggedFolderSkipsTMDBCopiedFromTVDB(t *testing.T) {
	svc, s, tmdb, _ := addShowService(t)
	squidSource(tmdb)
	ctx := context.Background()
	lf := addShowFolder(t, s)
	folder := "MasterChef The Professionals (2008) [tvdbid-82817]"
	writeFile(t, filepath.Join(lf.Path, folder, "Season 1", "MasterChef.The.Professionals.S01E01.mkv"), "a")
	p, err := PlanShowAdoptFrom(ctx, s.Read(), svc.Shows, lf.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	c := byFolder(p)[folder]
	if c.Match == nil || c.Match.ID != "tmdb:24285" {
		t.Fatalf("match %+v (%s: %s)", c.Match, c.Status, c.Why)
	}
	res, err := svc.Adopt(ctx, lf.ID, []library.AdoptChoice{{Folder: folder, ID: c.Match.ID}}, domain.System)
	if err != nil || res[0].TitleID == 0 {
		t.Fatalf("%+v %v", res, err)
	}
	if got := storedTMDB(t, svc, res[0].TitleID); got != "24285" {
		t.Fatalf("tmdb id %q, want 24285", got)
	}
}

// Adding by the TVDB id alone does not store the TMDB show whose own id is
// that number.
func TestAddByTVDBSkipsTMDBCopiedFromTVDB(t *testing.T) {
	svc, s, tmdb, _ := addShowService(t)
	squidSource(tmdb)
	addShowFolder(t, s)
	id, _, err := svc.AddShow(context.Background(), "tvdb:82817", ShowAddOptions{Source: "tmdb"}, domain.System)
	if err == nil && storedTMDB(t, svc, id) == "82817" {
		t.Fatal("stored TMDB 82817 (Go Go Squid!) for TVDB 82817")
	}
}

// Repairing a copied id never puts the same copied number back.
func TestFixCopiedTMDBKeepsOutCopiedAnswer(t *testing.T) {
	svc, _, tmdb, _ := addShowService(t)
	squidSource(tmdb)
	delete(tmdb.shows, "imdb:tt1281256")
	sh := mazeChef("82817")
	delete(sh.IDs, "imdb")
	svc.fixCopiedTMDB(context.Background(), &sh)
	if sh.IDs["tmdb"] == "82817" {
		t.Fatalf("ids %v", sh.IDs)
	}
}

// A real show whose TMDB id happens to equal its TVDB id is still found on
// a refresh: TMDB's answer by its IMDb id confirms the pairing.
func TestRefreshKeepsRealShowWithEqualTMDBAndTVDB(t *testing.T) {
	svc, _, tmdb, _ := addShowService(t)
	same := labShow("tmdb", "500")
	same.IDs = map[string]string{"tmdb": "500", "tvdb": "500", "imdb": "tt0500"}
	tmdb.set(same, "tmdb:500", "imdb:tt0500")
	sh, err := svc.lookupKnown(context.Background(), tmdb, "tmdb", "Lab Show", map[string]string{"tmdb": "500", "tvdb": "500", "imdb": "tt0500"})
	if err != nil || sh.IDs["tmdb"] != "500" {
		t.Fatalf("%v %v", sh.IDs, err)
	}
}

// A real show whose TMDB id equals its TVDB id and that has no IMDb id is
// still found: the stored TMDB id answers with the stored name.
func TestRefreshKeepsEqualIDShowWithoutIMDb(t *testing.T) {
	svc, _, tmdb, _ := addShowService(t)
	same := labShow("tmdb", "500")
	same.IDs = map[string]string{"tmdb": "500", "tvdb": "500"}
	tmdb.set(same, "tmdb:500")
	sh, err := svc.lookupKnown(context.Background(), tmdb, "tmdb", "Lab Show", map[string]string{"tmdb": "500", "tvdb": "500"})
	if err != nil || sh.IDs["tmdb"] != "500" {
		t.Fatalf("%v %v", sh.IDs, err)
	}
}

// When TMDB gives only the other show, the lookup fails without saying the
// show is gone, so a refresh keeps it as it is and never flags it dropped.
func TestRefreshCopiedIDOnlyOtherShowIsNotDropped(t *testing.T) {
	svc, _, tmdb, _ := addShowService(t)
	squidSource(tmdb)
	delete(tmdb.shows, "imdb:tt1281256")
	_, err := svc.lookupKnown(context.Background(), tmdb, "tmdb", "MasterChef: The Professionals", map[string]string{"tmdb": "82817", "tvdb": "82817"})
	if err == nil || errors.Is(err, metadata.ErrShowNotFound) {
		t.Fatalf("err %v, want a kept-as-is error", err)
	}
}
