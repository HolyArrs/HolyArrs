package downloads

import (
	"context"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// Every state word qBittorrent 4.x (paused*) and 5.x (stopped*) sends, and
// whether a torrent in it is still followed. None may end as removed.
func TestQbitEveryStateKeepsTheDownload(t *testing.T) {
	cases := []struct{ state, want string }{
		{"error", StatePaused}, {"missingFiles", StatePaused}, {"unknown", StatePaused},
		{"uploading", StateFinished}, {"stalledUP", StateFinished}, {"queuedUP", StateFinished}, {"checkingUP", StateFinished}, {"forcedUP", StateFinished},
		{"pausedUP", StateFinished}, {"stoppedUP", StateFinished},
		{"pausedDL", StatePaused}, {"stoppedDL", StatePaused},
		{"queuedDL", StateQueued}, {"checkingDL", StateQueued}, {"allocating", StateQueued}, {"checkingResumeData", StateQueued}, {"moving", StateQueued},
		{"downloading", StateDownloading}, {"metaDL", StateDownloading}, {"forcedMetaDL", StateDownloading}, {"stalledDL", StateDownloading}, {"forcedDL", StateDownloading},
	}
	for _, c := range cases {
		t.Run(c.state, func(t *testing.T) {
			if got := qbitState(c.state); got != c.want {
				t.Fatalf("qbitState(%s) = %s, want %s", c.state, got, c.want)
			}
			w, m := setup(t)
			id := grabbed(t, w, m)
			w.fake.set(hash, map[string]any{"state": c.state})
			for range missedSyncs + 2 {
				w.advance(missWindow)
				_, _ = m.SyncOnce(context.Background())
			}
			if d := w.download(t, id); d.State == "vanished" || d.State == "cleaned" {
				t.Fatalf("%s: download ended as %s (%v)", c.state, d.State, d.StateReason)
			}
		})
	}
}

// A hybrid (v1+v2) torrent is listed by qBittorrent 4.4+/5.x under the first 40
// characters of its v2 hash, not the v1 hash HolyArrs sent it with. It must be
// followed through infohash_v1, never called removed.
func TestQbitHybridTorrentIsFollowedByV1Hash(t *testing.T) {
	const qbitID = "2e21e95b00000000000000000000000000000000"
	w, m := setup(t)
	id, err := m.Grab(context.Background(), w.slotID, w.relID, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	w.fake.set(qbitID, map[string]any{"infohash_v1": hash, "infohash_v2": qbitID + "000000000000000000000000",
		"name": "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS", "category": "holyarrs", "progress": 0.2, "state": "downloading"})
	for range missedSyncs + 2 {
		w.advance(missWindow)
		_, _ = m.SyncOnce(context.Background())
	}
	d := w.download(t, id)
	if d.State == "vanished" || d.State == "cleaned" {
		t.Fatalf("hybrid torrent called %s (%v)", d.State, d.StateReason)
	}
	if d.ExternalID != qbitID {
		t.Fatalf("external id %s, want qBittorrent's id %s", d.ExternalID, qbitID)
	}
	w.fake.set(qbitID, map[string]any{"state": "stoppedDL"})
	w.advance(missWindow)
	_, _ = m.SyncOnce(context.Background())
	if d := w.download(t, id); d.State != "paused" {
		t.Fatalf("stopped hybrid torrent: %s", d.State)
	}
}

func TestQbitJobKeepsV1HashAsAltID(t *testing.T) {
	j := jobFromTorrent(qbitTorrent{Hash: "BBBB", InfohashV1: "AAAA", State: "downloading"})
	if j.ID != "bbbb" || j.AltID != "aaaa" {
		t.Fatalf("%+v", j)
	}
	if j := jobFromTorrent(qbitTorrent{Hash: "AAAA", InfohashV1: "aaaa"}); j.AltID != "" {
		t.Fatalf("alt id for a v1 torrent: %q", j.AltID)
	}
}
