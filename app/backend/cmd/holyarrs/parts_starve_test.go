package main

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// TestHeldJobsDoNotStarveTheirPool: 300 held Music Jobs (title-decided
// and Music's own kind) at the front of the I/O pool's queue do not keep a
// Movies Job in the same pool from running.
func TestHeldJobsDoNotStarveTheirPool(t *testing.T) {
	f := newFix(t)
	var music, movie int64
	err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		ti := &mediafiles.Title{Kind: "artist", GateDriver: "none", Name: "Low Bells", SortName: "low bells", Aliases: []string{}, SourceIDs: map[string]string{},
			MetadataSource: "local", NewPartsPolicy: "all", ReleaseGate: "released", MusicTypes: []string{}, WhyAdded: "by_hand", Artwork: []string{}, Status: "released", State: "active"}
		var err error
		if music, err = database.Insert(tx, "titles", ti, domain.System); err != nil {
			return err
		}
		return tx.QueryRow("SELECT id FROM titles WHERE kind = 'movie' LIMIT 1").Scan(&movie)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.c.Set(context.Background(), parts.Music, false, owner); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 150; i++ {
		f.enqueue(t, "search_slot", &domain.Subject{Type: "title", ID: music}, fmt.Sprintf("m%d", i))
		f.enqueue(t, "search_album", &domain.Subject{Type: "title", ID: music}, fmt.Sprintf("a%d", i))
	}
	id := f.enqueue(t, "search_slot", &domain.Subject{Type: "title", ID: movie}, "movie")
	f.waitJob(t, id, "done")
	if n := f.count("search_slot"); n != 1 {
		t.Fatalf("search_slot ran %d times, want only the Movies one", n)
	}
}
