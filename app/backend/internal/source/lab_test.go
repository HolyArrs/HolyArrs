//go:build lab

package source

import (
	"context"
	"os"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
)

// Runs inside the lab network: go test -tags lab ./internal/source/
// HOLYARRS_LAB_SOURCE defaults to the lab's fake indexer.
func labURL() string {
	if u := os.Getenv("HOLYARRS_LAB_SOURCE"); u != "" {
		return u
	}
	return "http://fake-indexer/api"
}

func TestLabFakeIndexer(t *testing.T) {
	c := New(Config{URL: labURL(), Protocol: "torrent", AllowLocal: true})
	caps, err := c.Caps(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !caps.MovieSearch {
		t.Fatalf("fake indexer should offer movie search: %+v", caps)
	}
	f := seed(t, labURL())
	j := runSearch(t, f, &Searcher{Store: f.s})
	if j.State != "done" {
		t.Fatalf("job %s %+v", j.State, j.Error)
	}
	var sl domain.Slot
	_ = database.Get(context.Background(), f.s.Read(), "slots", f.slotID, &sl)
	if sl.State != domain.SlotChosen {
		t.Fatalf("slot %s %v", sl.State, sl.LastReason)
	}
	rels, _ := database.Select[domain.Release](context.Background(), f.s.Read(), "releases", "state = 'chosen'")
	if len(rels) != 1 || rels[0].Name != "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS" {
		t.Fatalf("chosen %+v", rels)
	}
	t.Logf("chosen %s from %s", rels[0].Name, rels[0].LinkRef)
}
