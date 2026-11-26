package downloads

import (
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// Every seeding-goal case: a torrent paused before the Downloader's
// own goal stays; a met goal or a client stop with no goal removes it.
func TestSeedingDoneMatrix(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	n := func(v int) *int { return &v }
	type goal struct {
		r *float64
		m *int
	}
	none, ratio2, min60, both := goal{}, goal{r: f(2)}, goal{m: n(60)}, goal{r: f(2), m: n(60)}
	cases := []struct {
		name    string
		adapter string
		g       goal
		j       Job
		done    bool
	}{
		// No goal: the client's stop decides (hand pause or its own limit look alike).
		{"none qbit4 pausedUP", "qbittorrent", none, Job{Detail: "pausedUP", Ratio: 0.3}, true},
		{"none qbit5 stoppedUP", "qbittorrent", none, Job{Detail: "stoppedUP"}, true},
		{"none qbit uploading", "qbittorrent", none, Job{Detail: "uploading", Ratio: 9}, false},
		{"none qbit stalledUP", "qbittorrent", none, Job{Detail: "stalledUP"}, false},
		{"none qbit queuedUP", "qbittorrent", none, Job{Detail: "queuedUP"}, false},
		{"none qbit forcedUP", "qbittorrent", none, Job{Detail: "forcedUP"}, false},
		{"none qbit pausedDL", "qbittorrent", none, Job{Detail: "pausedDL", Progress: 0.5}, false},
		{"none transmission stopped", "transmission", none, Job{Detail: "0"}, true},
		{"none transmission seeding", "transmission", none, Job{Detail: "6"}, false},
		{"none deluge paused", "deluge", none, Job{Detail: "Paused"}, true},
		{"none deluge seeding", "deluge", none, Job{Detail: "Seeding"}, false},
		{"none rtorrent complete", "rtorrent", none, Job{Detail: "complete"}, true},
		{"none rtorrent seeding", "rtorrent", none, Job{Detail: "seeding"}, false},
		{"none errored", "qbittorrent", none, Job{Detail: "error"}, false},
		{"none missingFiles", "qbittorrent", none, Job{Detail: "missingFiles"}, false},

		// Ratio goal 2. Client limit lower (paused at 1): stays, by hand or by limit.
		{"ratio limit lower qbit4", "qbittorrent", ratio2, Job{Detail: "pausedUP", Ratio: 1}, false},
		{"ratio limit lower qbit5", "qbittorrent", ratio2, Job{Detail: "stoppedUP", Ratio: 1}, false},
		{"ratio paused by hand early", "qbittorrent", ratio2, Job{Detail: "pausedUP", Ratio: 0.2, SeedingTime: 99999}, false},
		{"ratio limit equal qbit4", "qbittorrent", ratio2, Job{Detail: "pausedUP", Ratio: 2}, true},
		{"ratio limit equal qbit5", "qbittorrent", ratio2, Job{Detail: "stoppedUP", Ratio: 2}, true},
		{"ratio just under", "qbittorrent", ratio2, Job{Detail: "uploading", Ratio: 1.999}, false},
		{"ratio limit higher, still seeding past goal", "qbittorrent", ratio2, Job{Detail: "uploading", Ratio: 2.5}, true},
		{"ratio limit higher, below goal", "qbittorrent", ratio2, Job{Detail: "stalledUP", Ratio: 1.5}, false},
		{"ratio unknown 0", "qbittorrent", ratio2, Job{Detail: "pausedUP"}, false},
		{"ratio unknown -1", "qbittorrent", ratio2, Job{Detail: "pausedUP", Ratio: -1}, false},
		{"ratio met but errored", "qbittorrent", ratio2, Job{Detail: "error", Ratio: 3}, false},
		{"ratio rtorrent met", "rtorrent", ratio2, Job{Detail: "seeding", Ratio: 2}, true},
		{"ratio rtorrent complete early", "rtorrent", ratio2, Job{Detail: "complete", Ratio: 1}, false},

		// Minutes goal 60.
		{"min limit lower qbit4", "qbittorrent", min60, Job{Detail: "pausedUP", SeedingTime: 1800}, false},
		{"min limit lower qbit5", "qbittorrent", min60, Job{Detail: "stoppedUP", SeedingTime: 1800}, false},
		{"min equal", "qbittorrent", min60, Job{Detail: "stoppedUP", SeedingTime: 3600}, true},
		{"min one second short", "qbittorrent", min60, Job{Detail: "uploading", SeedingTime: 3599}, false},
		{"min limit higher past goal", "qbittorrent", min60, Job{Detail: "uploading", SeedingTime: 7200}, true},
		{"min seeding time missing", "qbittorrent", min60, Job{Detail: "pausedUP", Ratio: 50}, false},
		{"min seeding time negative", "qbittorrent", min60, Job{Detail: "pausedUP", SeedingTime: -1}, false},

		// Both goals: either one met is enough.
		{"both ratio met", "qbittorrent", both, Job{Detail: "uploading", Ratio: 2, SeedingTime: 10}, true},
		{"both time met", "qbittorrent", both, Job{Detail: "pausedUP", Ratio: 0.1, SeedingTime: 3600}, true},
		{"both neither met, paused", "qbittorrent", both, Job{Detail: "stoppedUP", Ratio: 1, SeedingTime: 600}, false},
		{"both unknown", "qbittorrent", both, Job{Detail: "stoppedUP", Ratio: -1}, false},

		// A stored goal on a client that cannot report it is ignored: the stop decides.
		{"goal ignored transmission stopped", "transmission", ratio2, Job{Detail: "0"}, true},
		{"goal ignored deluge seeding", "deluge", ratio2, Job{Detail: "Seeding", Ratio: 5}, false},

		// Not finished is never done.
		{"not finished", "qbittorrent", ratio2, Job{State: StateDownloading, Detail: "downloading", Ratio: 5, Progress: 0.9}, false},
	}
	for _, c := range cases {
		j := c.j
		if j.State == "" {
			j.State = StateFinished
		}
		if j.Progress == 0 {
			j.Progress = 1
		}
		dr := domain.Downloader{Name: "C", Adapter: c.adapter, SeedGoal: domain.SeedGoal{Ratio: c.g.r, Minutes: c.g.m}}
		if got := seedingDone(dr, j) != ""; got != c.done {
			t.Errorf("%s: done=%v, want %v (%q)", c.name, got, c.done, seedingDone(dr, j))
		}
	}
}

func TestPackStillOpenMatrix(t *testing.T) {
	if packStillOpen(domain.Download{FilesSeen: []domain.SeenFile{{Priority: 0}}}) {
		t.Fatal("whole torrent counted as an open pack")
	}
	open := domain.Download{PackFiles: []domain.PackFile{}, FilesSeen: []domain.SeenFile{{Priority: 1}, {Priority: 0}}}
	if !packStillOpen(open) {
		t.Fatal("pack with a skipped file not open")
	}
	open.FilesSeen[1].Priority = 1
	if packStillOpen(open) {
		t.Fatal("fully wanted pack still open")
	}
}
