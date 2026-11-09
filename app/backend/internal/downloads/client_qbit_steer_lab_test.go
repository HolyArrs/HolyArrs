//go:build lab

package downloads

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLabQbitSteer(t *testing.T) {
	key := os.Getenv("HOLYARRS_LAB_QBIT_KEY")
	if key == "" {
		t.Skip("HOLYARRS_LAB_QBIT_KEY not set (run through labctl qbt-apikey)")
	}
	base := os.Getenv("HOLYARRS_LAB_QBIT")
	if base == "" {
		base = "http://127.0.0.1:18708"
	}
	ctx := context.Background()
	q := NewQbittorrentKey(base, key, nil)
	if v, _ := q.qbitAPI(ctx); !atLeast(v, 2, 12, 0) {
		t.Fatalf("lab API %v: expected 5.2", v)
	}
	const folder = "/data/downloads/incomplete/holyarrs-labtest"
	id, err := q.Add(ctx, AddRequest{URL: "http://fake-indexer/dune-part-two.torrent", Category: "holyarrs-labtest", SavePath: folder, InfoHash: hash})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := q.Remove(context.Background(), id, true); err != nil {
			t.Errorf("remove: %v", err)
		}
	}()
	wait := func(what string, ok func() bool) {
		t.Helper()
		deadline := time.Now().Add(90 * time.Second)
		for time.Now().Before(deadline) {
			if ok() {
				return
			}
			time.Sleep(500 * time.Millisecond)
		}
		t.Fatalf("never %s", what)
	}
	job := func() Job {
		all, err := q.ListAll(ctx)
		if err != nil {
			return Job{}
		}
		for _, j := range all {
			if j.ID == id {
				return j
			}
		}
		return Job{}
	}
	ids := []string{id}
	wait("listed the torrent", func() bool { return job().ID == id })

	if err := q.SetShareLimits(ctx, ids, ShareLimits{Ratio: 1.5, SeedingMinutes: 120, InactiveMinutes: 30, Action: "stop"}); err != nil {
		t.Fatal(err)
	}
	wait("kept the seeding rules", func() bool {
		l := job().Limits
		return l != nil && l.Ratio == 1.5 && l.SeedingMinutes == 120 && l.InactiveMinutes == 30 && l.Action == "stop"
	})

	if err := q.SetLocation(ctx, ids, folder+"/moved"); err != nil {
		t.Fatal(err)
	}
	wait("moved", func() bool { return strings.TrimRight(job().SavePath, "/") == folder+"/moved" })

	for _, a := range []string{BulkBottom, BulkUp, BulkDown, BulkTop} {
		if err := q.Bulk(ctx, a, ids); err != nil && !errors.Is(err, ErrQueueOff) {
			t.Fatalf("%s: %v", a, err)
		} else if err != nil {
			t.Logf("%s: queue off in the lab", a)
		}
	}
	if err := q.Bulk(ctx, BulkForce, ids); err != nil {
		t.Fatal(err)
	}
	wait("force started", func() bool { return strings.HasPrefix(job().Detail, "forced") || job().Detail == "uploading" })
	t.Logf("after force: %s", job().Detail)

	if err := q.SetTorrentLimits(ctx, ids, 200*1024, 50*1024); err != nil {
		t.Fatal(err)
	}
	wait("kept the caps", func() bool { j := job(); return j.LimitDown == 200*1024 && j.LimitUp == 50*1024 })
	if err := q.SetTorrentLimits(ctx, ids, 0, 0); err != nil {
		t.Fatal(err)
	}

	const tr = "udp://tracker.holyarrs-lab.invalid:6969/announce"
	if err := q.AddTrackers(ctx, id, []string{tr}); err != nil {
		t.Fatal(err)
	}
	ref := trackerRef(tr)
	var rows []Tracker
	wait("listed the tracker", func() bool {
		rows, err = q.Trackers(ctx, id)
		for _, x := range rows {
			if x.Ref == ref {
				return true
			}
		}
		return false
	})
	fixed := 0
	for _, x := range rows {
		if x.Fixed {
			fixed++
		}
	}
	if fixed != 3 {
		t.Errorf("fixed rows %d: %+v", fixed, rows)
	}
	tier := 2
	if err := q.EditTracker(ctx, id, ref, "", &tier); err != nil {
		t.Fatal(err)
	}
	if err := q.RemoveTrackers(ctx, id, []string{ref}); err != nil {
		t.Fatal(err)
	}
	wait("removed the tracker", func() bool {
		rows, err := q.Trackers(ctx, id)
		if err != nil {
			return false
		}
		for _, x := range rows {
			if x.Ref == ref {
				return false
			}
		}
		return true
	})

	var files []TorrentFile
	wait("listed its files", func() bool { files, err = q.TorrentFiles(ctx, id); return err == nil && len(files) > 0 })
	if err := q.SetFilePriority(ctx, id, []int{files[0].Index}, 7); err != nil {
		t.Fatal(err)
	}
	wait("set the file to maximum", func() bool {
		fs, err := q.TorrentFiles(ctx, id)
		return err == nil && len(fs) > 0 && fs[0].Priority == 7
	})

	st, err := q.Status(ctx)
	if err != nil || st.SlowMode == nil {
		t.Fatalf("status %+v %v", st, err)
	}
	was := *st.SlowMode
	if err := q.SetSlowMode(ctx, !was); err != nil {
		t.Fatal(err)
	}
	wait("flipped slow mode", func() bool { s, err := q.Status(ctx); return err == nil && s.SlowMode != nil && *s.SlowMode != was })
	if err := q.SetSlowMode(ctx, was); err != nil {
		t.Fatal(err)
	}
	t.Logf("connection %q", st.Connection)
}
