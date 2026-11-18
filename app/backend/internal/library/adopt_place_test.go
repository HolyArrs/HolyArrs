package library

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// A scan read in parts says how many folders the Library folder holds in
// all and how many sort before this part, so the page can say "folders 11
// to 13 of 13" instead of only the part's own count.
func TestPlanAdoptSaysWhereThePartSits(t *testing.T) {
	s := open(t)
	lf := addFolder(t, s)
	for i := range 13 {
		writeFile(t, filepath.Join(lf.Path, fmt.Sprintf("Movie %02d (2001)", i), "m.mkv"), "m")
	}
	writeFile(t, filepath.Join(lf.Path, ".hidden", "m.mkv"), "m")
	writeFile(t, filepath.Join(lf.Path, "loose.mkv"), "m")
	ns := mediafiles.NamingDefaults()
	ns.AdoptScanLimit = 10
	if err := mediafiles.PutNamingSettings(context.Background(), s, ns, domain.System); err != nil {
		t.Fatal(err)
	}
	p, err := PlanAdoptFrom(context.Background(), s.Read(), adoptTMDB, lf.ID, "")
	if err != nil || p.Total != 13 || p.Offset != 0 || len(p.Folders) != 10 {
		t.Fatalf("first part total %d offset %d folders %d %v", p.Total, p.Offset, len(p.Folders), err)
	}
	p, err = PlanAdoptFrom(context.Background(), s.Read(), adoptTMDB, lf.ID, p.Next)
	if err != nil || p.Total != 13 || p.Offset != 10 || len(p.Folders) != 3 || p.More {
		t.Fatalf("second part total %d offset %d folders %d more %v %v", p.Total, p.Offset, len(p.Folders), p.More, err)
	}
}

// Exactly the limit of folders is one part with nothing left; one more
// makes a second part of one.
func TestPlanAdoptAtTheLimit(t *testing.T) {
	s := open(t)
	lf := addFolder(t, s)
	for i := range 10 {
		writeFile(t, filepath.Join(lf.Path, fmt.Sprintf("Movie %02d (2001)", i), "m.mkv"), "m")
	}
	ns := mediafiles.NamingDefaults()
	ns.AdoptScanLimit = 10
	if err := mediafiles.PutNamingSettings(context.Background(), s, ns, domain.System); err != nil {
		t.Fatal(err)
	}
	p, err := PlanAdoptFrom(context.Background(), s.Read(), adoptTMDB, lf.ID, "")
	if err != nil || p.More || p.Total != 10 || len(p.Folders) != 10 {
		t.Fatalf("at the limit: more %v total %d folders %d %v", p.More, p.Total, len(p.Folders), err)
	}
	writeFile(t, filepath.Join(lf.Path, "Movie 10 (2001)", "m.mkv"), "m")
	p, _ = PlanAdoptFrom(context.Background(), s.Read(), adoptTMDB, lf.ID, "")
	if !p.More || p.Total != 11 {
		t.Fatalf("one over: more %v total %d", p.More, p.Total)
	}
	p, _ = PlanAdoptFrom(context.Background(), s.Read(), adoptTMDB, lf.ID, p.Next)
	if p.More || p.Offset != 10 || len(p.Folders) != 1 {
		t.Fatalf("the last one: more %v offset %d folders %d", p.More, p.Offset, len(p.Folders))
	}
}

// The music scan says the same for its artist folders.
func TestMusicPlanSaysWhereThePartSits(t *testing.T) {
	_, s, _, lf := musicService(t)
	thousandArtists(t, lf.Path, 1)
	cm := &countMusic{n: 1000, tracks: 1}
	p, err := PlanMusicAdoptFrom(context.Background(), s.Read(), cm, lf.ID, "Artist 0200")
	if err != nil || p.Total != 1000 || p.Offset != 200 {
		t.Fatalf("total %d offset %d %v", p.Total, p.Offset, err)
	}
}
