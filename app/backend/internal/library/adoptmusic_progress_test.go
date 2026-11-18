package library

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// A music scan tells the page how many artist folders of this part are
// read, as the movie and show scans do, so a long one is seen moving.
func TestMusicScanReportsProgress(t *testing.T) {
	_, s, _, lf := musicService(t)
	for i := 1; i <= 30; i++ {
		writeFile(t, filepath.Join(lf.Path, fmt.Sprintf("Artist %04d", i), fmt.Sprintf("Album %04d (2010)", i), "01 - Song 1.mp3"), "a")
	}
	cm := &countMusic{n: 30, tracks: 1}
	var seen [][2]int
	ctx := WithScanProgress(context.Background(), func(done, total int) { seen = append(seen, [2]int{done, total}) })
	p, err := PlanMusicAdoptFrom(ctx, s.Read(), cm, lf.ID, "Artist 0010")
	if err != nil || len(p.Folders) != 20 {
		t.Fatalf("%v %d", err, len(p.Folders))
	}
	if len(seen) < 20 {
		t.Fatalf("only %d progress reports: %v", len(seen), seen)
	}
	last := seen[len(seen)-1]
	if last != [2]int{20, 20} || seen[0][1] != 20 {
		t.Fatalf("progress %v", seen)
	}
	for i := 1; i < len(seen); i++ {
		if seen[i][0] < seen[i-1][0] {
			t.Fatalf("progress went back: %v", seen)
		}
	}
}

// One music part is small enough to finish while a person waits: each
// artist costs a few lookups paced seconds apart.
func TestMusicScanPartIsShort(t *testing.T) {
	if n := mediafiles.DefaultMusicAdoptScanLimit; n > 50 {
		t.Fatalf("music part of %d artists takes too long", n)
	}
}
