package downloads

import (
	"testing"
	"time"
)

func TestSabPostedAt(t *testing.T) {
	now := time.Date(2026, 11, 2, 15, 30, 20, 0, time.UTC)
	for _, c := range []struct {
		age  string
		want time.Time
	}{
		{"123d", time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC)},
		{"5h", time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)},
		{"40m", time.Date(2026, 11, 2, 14, 50, 0, 0, time.UTC)},
	} {
		if got := sabPostedAt(c.age, now); got != c.want.Unix() {
			t.Fatalf("%s: %v", c.age, time.Unix(got, 0).UTC())
		}
	}
	for _, bad := range []string{"", "d", "xd", "12y", "-3d"} {
		if got := sabPostedAt(bad, now); got != 0 {
			t.Fatalf("%q: %d", bad, got)
		}
	}
}

func TestNZBGetPostedAt(t *testing.T) {
	if j := (nzbgetRow{NZBName: "a", MinPostTime: 1700000000}).job(false); j.PostedAt != 1700000000 {
		t.Fatalf("queue %d", j.PostedAt)
	}
	if j := (nzbgetRow{NZBName: "a", MinPostTime: 1700000000, Status: "SUCCESS/ALL"}).job(true); j.PostedAt != 1700000000 {
		t.Fatalf("history %d", j.PostedAt)
	}
}
