package imgstore

import (
	"context"
	"testing"
	"time"
)

// A miss is kept with its date; once its week is over it is listed for the
// weekly re-ask, and HasMiss holds until a picture is stored.
func TestMissesDueWeekly(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	s, err := Open(Options{Dir: t.TempDir(), Now: func() time.Time { return now }, Logf: func(string, ...any) {}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if _, err := s.db.Exec(`INSERT INTO misses(kind,mbid,until) VALUES(?,?,?)`, KindAlbum, albumID, now.Add(MissRecheck).Unix()); err != nil {
		t.Fatal(err)
	}
	if MissRecheck != 7*24*time.Hour {
		t.Fatalf("miss kept %v, want a week", MissRecheck)
	}
	if !s.HasMiss(ctx, KindAlbum, albumID) || len(s.MissesDue(ctx, 10)) != 0 {
		t.Fatal("fresh miss listed or not kept")
	}
	now = now.Add(MissRecheck + time.Minute)
	due := s.MissesDue(ctx, 10)
	if len(due) != 1 || due[0] != (Key{KindAlbum, albumID}) || !s.HasMiss(ctx, KindAlbum, albumID) {
		t.Fatalf("due %v", due)
	}
}
