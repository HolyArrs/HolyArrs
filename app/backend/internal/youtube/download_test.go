package youtube

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// info is a video lookup as yt-dlp prints it.
func info(id, title, channel string, ts int64) map[string]any {
	return map[string]any{"_type": "video", "id": id, "title": title, "channel": channel, "channel_id": veritasiumUC,
		"duration": 61.0, "timestamp": ts, "live_status": "not_live"}
}

// download queues one row's download and runs it.
func (r *rig) download(userID, rowID int64) {
	r.t.Helper()
	r.svc.queueDownload(context.Background(), userID, rowID)
	r.drain()
}

func TestDownloadWritesLayoutAndRow(t *testing.T) {
	t.Parallel()
	r := newRig(t, "ok")
	r.putJSON("info-abcdefghijk.json", info("abcdefghijk", "Title", "Veritasium", 1709650800))
	f := r.follow(anna, veritasium())
	id := addVideo(t, r.s, Video{UserID: anna, FollowIDs: []int64{f.ID}, VideoID: "abcdefghijk", Title: "listed title"})
	r.download(anna, id)

	file := filepath.Join(r.root, "7-Anna", "Veritasium", "2024-03-05 Title [abcdefghijk].mp4")
	for _, p := range []string{file, strings.TrimSuffix(file, ".mp4") + ".jpg"} {
		if _, err := os.Stat(p); err != nil {
			t.Fatal(err)
		}
	}
	v := r.videos(anna)["abcdefghijk"]
	if v.State != StateDone || v.RelativePath != "7-Anna/Veritasium/2024-03-05 Title [abcdefghijk].mp4" || v.SizeBytes != int64(len("fake video")) ||
		v.Title != "Title" || v.ChannelName != "Veritasium" || v.UploadedAt == nil || v.UploadedAt.Unix() != 1709650800 || v.Error != "" {
		t.Fatalf("row %+v", v)
	}
	// The job asked for the row's quality cap.
	var args []string
	for _, c := range r.fake.Calls(t) {
		if len(c) > 0 && c[len(c)-1] == "https://www.youtube.com/watch?v=abcdefghijk" && strings.Contains(strings.Join(c, " "), "-o") {
			args = c
		}
	}
	if !strings.Contains(strings.Join(args, " "), FormatFor(1080)) {
		t.Fatalf("download args %q", args)
	}

	evs := r.live.all()
	if len(evs) < 3 {
		t.Fatalf("live events %+v", evs)
	}
	sawPercent := false
	for _, ev := range evs {
		if ev.User != anna || ev.Topic != LiveTopic || ev.Video.ID != id {
			t.Fatalf("event %+v went beyond Anna's video", ev)
		}
		if ev.Video.State == StateDownloading && ev.Video.Percent > 0 && ev.Video.Percent < 100 {
			sawPercent = true
		}
	}
	if evs[0].Video.State != StateDownloading || !sawPercent || evs[len(evs)-1].Video != (LiveVideo{ID: id, State: StateDone, Percent: 100}) {
		t.Fatalf("live events %+v", evs)
	}

	// A playlist's videos go in a folder named after the playlist; a video
	// of no follow in one named after its channel.
	pl := r.follow(anna, Follow{Kind: KindPlaylist, YTID: "PLtestlist01", URL: "https://www.youtube.com/playlist?list=PLtestlist01", Title: "Test: playlist"})
	r.putJSON("info-bbbbbbbbbbb.json", info("bbbbbbbbbbb", "In a list", "Someone", 1709650800))
	r.putJSON("info-ccccccccccc.json", info("ccccccccccc", "On its own", "Some Channel", 1709650800))
	inList := addVideo(t, r.s, Video{UserID: anna, FollowIDs: []int64{pl.ID}, VideoID: "bbbbbbbbbbb"})
	alone := addVideo(t, r.s, Video{UserID: anna, VideoID: "ccccccccccc"})
	r.download(anna, inList)
	r.download(anna, alone)
	got := r.videos(anna)
	if p := got["bbbbbbbbbbb"].RelativePath; p != "7-Anna/Test_ playlist/2024-03-05 In a list [bbbbbbbbbbb].mp4" {
		t.Fatalf("playlist video at %q", p)
	}
	if p := got["ccccccccccc"].RelativePath; p != "7-Anna/Some Channel/2024-03-05 On its own [ccccccccccc].mp4" {
		t.Fatalf("one-off video at %q", p)
	}
}

func TestDownloadFailsAfterThreeAttempts(t *testing.T) {
	t.Parallel()
	r := newRig(t, "fail")
	id := addVideo(t, r.s, Video{UserID: anna, VideoID: "abcdefghijk"})
	r.download(anna, id)

	var states []string
	for _, ev := range r.live.all() {
		states = append(states, ev.Video.State)
	}
	want := "downloading,queued,downloading,queued,downloading,failed"
	if strings.Join(states, ",") != want {
		t.Fatalf("states %v, want %s", states, want)
	}
	v := r.videos(anna)["abcdefghijk"]
	if v.State != StateFailed || v.Error != "ERROR: [youtube] x: Video unavailable" {
		t.Fatalf("row %+v", v)
	}
	js := r.jobs(KindDownload)
	if len(js) != 1 || js[0].State != "failed" || js[0].Attempt != 3 || js[0].Error == nil {
		t.Fatalf("job %+v", js)
	}
	if msg := js[0].Error.Message; strings.Contains(msg, "unavailable") || strings.Contains(msg, "A video") || !strings.Contains(msg, itoa(id)) {
		t.Fatalf("job error %q", msg)
	}
	// Nothing half-written stays.
	leftovers, _ := filepath.Glob(filepath.Join(r.root, "*", "*", "*"))
	if len(leftovers) != 0 {
		t.Fatalf("left behind %v", leftovers)
	}
}

func TestLowSpaceLeavesQueued(t *testing.T) {
	t.Parallel()
	r := newRig(t, "ok")
	r.svc.FreeSpace = func(string) int64 { return 1 << 30 }
	id := addVideo(t, r.s, Video{UserID: anna, VideoID: "abcdefghijk"})
	r.download(anna, id)
	if v := r.videos(anna)["abcdefghijk"]; v.State != StateQueued {
		t.Fatalf("row %q, want queued", v.State)
	}
	js := r.jobs(KindDownload)
	if len(js) != 1 || js[0].State != "done" || js[0].Result["paused"] != "low_space" {
		t.Fatalf("job %+v", js)
	}
	if !r.svc.Status(context.Background()).LowSpace {
		t.Fatal("status does not say low space")
	}
	if calls := r.fake.Calls(t); len(calls) != 0 {
		t.Fatalf("yt-dlp ran: %v", calls)
	}
}

func TestKeepLastRemovesOldest(t *testing.T) {
	t.Parallel()
	r := newRig(t, "ok")
	day := int64(24 * 3600)
	ups := map[string]int64{"aaaaaaaaaaa": 1700000000 + 2*day, "bbbbbbbbbbb": 1700000000, "ccccccccccc": 1700000000 + day}
	var es []tabEntry
	for id, ts := range ups {
		r.putJSON("info-"+id+".json", info(id, "Video "+id, "Veritasium", ts))
		es = append(es, entry(id, "Video "+id, ts, ""))
	}
	r.putJSON("channel_videos.json", channelTab(es...))
	r.putJSON("channel_streams.json", channelTab())
	f := veritasium()
	f.KeepLast, f.Backfill = intp(2), intp(3)
	fl := r.follow(anna, f)
	r.check(fl.ID)

	got := r.videos(anna)
	if got["aaaaaaaaaaa"].State != StateDone || got["ccccccccccc"].State != StateDone || got["bbbbbbbbbbb"].State != StateRemoved {
		t.Fatalf("states a %s b %s c %s", got["aaaaaaaaaaa"].State, got["bbbbbbbbbbb"].State, got["ccccccccccc"].State)
	}
	if b := got["bbbbbbbbbbb"]; b.RelativePath != "" || b.SizeBytes != 0 {
		t.Fatalf("removed row keeps its file %+v", b)
	}
	files, _ := filepath.Glob(filepath.Join(r.root, "7-Anna", "Veritasium", "*"))
	if len(files) != 4 {
		t.Fatalf("files %v, want two videos and two thumbnails", files)
	}
	for _, f := range files {
		if strings.Contains(f, "bbbbbbbbbbb") {
			t.Fatalf("oldest still on disk: %s", f)
		}
	}
	r.setNow(r.now.Add(6 * 3600e9))
	before := len(r.jobs(KindDownload))
	r.check(fl.ID)
	if got := r.videos(anna)["bbbbbbbbbbb"]; got.State != StateRemoved || len(r.jobs(KindDownload)) != before {
		t.Fatalf("the next check took the oldest again: %s", got.State)
	}
}

func TestDownloadSkipsGoneRowsAndCleansUp(t *testing.T) {
	t.Parallel()
	r := newRig(t, "ok")
	// Removed before the job ran: skipped, yt-dlp never asked.
	gone := addVideo(t, r.s, Video{UserID: anna, VideoID: "ggggggggggg", State: StateRemoved})
	r.download(anna, gone)
	if js := r.jobs(KindDownload); js[0].State != "done" || js[0].Result["skipped"] != StateRemoved || len(r.fake.Calls(t)) != 0 {
		t.Fatalf("job %+v", js[0])
	}
	// Removed while downloading: the new file and thumbnail go.
	id := addVideo(t, r.s, Video{UserID: anna, VideoID: "abcdefghijk"})
	r.live.onEvent = func(ev liveEvent) {
		if ev.Video.ID == id && ev.Video.State == StateDownloading && ev.Video.Percent == 0 {
			if err := r.s.Write(context.Background(), func(tx *sql.Tx) error { return MarkRemoved(tx, anna, id) }); err != nil {
				t.Error(err)
			}
		}
	}
	r.download(anna, id)
	if v := r.videos(anna)["abcdefghijk"]; v.State != StateRemoved || v.RelativePath != "" {
		t.Fatalf("row %+v", v)
	}
	if files, _ := filepath.Glob(filepath.Join(r.root, "*", "*", "*")); len(files) != 0 {
		t.Fatalf("files kept for a removed video: %v", files)
	}
	// A row deleted outright (unfollow with delete) is skipped too.
	r.live.onEvent = nil
	r.download(anna, 999)
	js := r.jobs(KindDownload)
	if last := js[len(js)-1]; last.State != "done" || last.Result["skipped"] != "gone" {
		t.Fatalf("job for a missing row %+v", last)
	}
}

func TestDownloadWaitsForAiring(t *testing.T) {
	t.Parallel()
	r := newRig(t, "live")
	id := addVideo(t, r.s, Video{UserID: anna, VideoID: "abcdefghijk"})
	r.download(anna, id)
	if v := r.videos(anna)["abcdefghijk"]; v.State != StateQueued {
		t.Fatalf("row %q, want queued", v.State)
	}
	if js := r.jobs(KindDownload); js[0].Result["paused"] != "not_aired" {
		t.Fatalf("job %+v", js[0])
	}
}

func TestJobsCarryNoTitles(t *testing.T) {
	t.Parallel()
	r := newRig(t, "ok")
	r.putJSON("channel_streams.json", channelTab())
	// vvvvvvvvvv1 downloads; vvvvvvvvvv2 is gone and fails three times.
	r.put("gone-vvvvvvvvvv2", nil)
	r.putJSON("info-vvvvvvvvvv1.json", info("vvvvvvvvvv1", "Newest video", "Veritasium", 1700000300))
	f := r.follow(anna, veritasium())
	r.check(0)
	got := r.videos(anna)
	if got["vvvvvvvvvv1"].State != StateDone || got["vvvvvvvvvv2"].State != StateFailed {
		t.Fatalf("a %s, b %s", got["vvvvvvvvvv1"].State, got["vvvvvvvvvv2"].State)
	}
	// The reason, with its video id, is on the owner's row.
	if e := got["vvvvvvvvvv2"].Error; !strings.Contains(e, "vvvvvvvvvv2") {
		t.Fatalf("row error %q", e)
	}
	banned := []string{"vvvvvvvvvv1", "vvvvvvvvvv2", "Newest video", "Veritasium", veritasiumUC, "youtube.com", "http", f.URL}
	type jobRow struct{ Kind, Params, Note, Err, Result, Key string }
	var rows []jobRow
	q, err := r.s.Read().QueryContext(context.Background(),
		"SELECT kind, params, COALESCE(progress_note, ''), COALESCE(error, ''), COALESCE(result, ''), COALESCE(dedupe_key, '') FROM jobs")
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	for q.Next() {
		var row jobRow
		if err := q.Scan(&row.Kind, &row.Params, &row.Note, &row.Err, &row.Result, &row.Key); err != nil {
			t.Fatal(err)
		}
		rows = append(rows, row)
	}
	if len(rows) < 3 {
		t.Fatalf("only %d jobs", len(rows))
	}
	sawErr := false
	for _, row := range rows {
		text, _ := json.Marshal(row)
		for _, b := range banned {
			if strings.Contains(string(text), b) {
				t.Errorf("job %s carries %q: %s", row.Kind, b, text)
			}
		}
		sawErr = sawErr || row.Err != ""
	}
	if !sawErr {
		t.Fatal("no job error was checked")
	}
}

// files lists every regular file under the YouTube folder.
func (r *rig) files() []string {
	r.t.Helper()
	var out []string
	err := filepath.WalkDir(r.root, func(p string, d os.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			out = append(out, p)
		}
		return err
	})
	if err != nil {
		r.t.Fatal(err)
	}
	return out
}

// runningJob is the id of the running job of a kind, for hooks that run
// off the test goroutine.
func (r *rig) runningJob(kind string) (int64, error) {
	var id int64
	err := r.s.Read().QueryRowContext(context.Background(), "SELECT id FROM jobs WHERE kind = ? AND state = 'running'", kind).Scan(&id)
	return id, err
}

func TestDownloadRemovedWhileLookingUpStaysRemoved(t *testing.T) {
	t.Parallel()
	r := newRig(t, "ok")
	r.putJSON("info-abcdefghijk.json", info("abcdefghijk", "Title", "Veritasium", 1709650800))
	f := r.follow(anna, veritasium())
	id := addVideo(t, r.s, Video{UserID: anna, FollowIDs: []int64{f.ID}, VideoID: "abcdefghijk"})
	hold := filepath.Join(r.data, "hold-info")
	r.put("hold-info", nil)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer os.Remove(hold)
		for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
			if _, err := os.Stat(filepath.Join(r.data, "info-started")); err == nil {
				if err := r.s.Write(context.Background(), func(tx *sql.Tx) error { return MarkRemoved(tx, anna, id) }); err != nil {
					t.Error(err)
				}
				return
			}
		}
		t.Error("the video was never looked up")
	}()
	r.download(anna, id)
	<-done

	if v := r.videos(anna)["abcdefghijk"]; v.State != StateRemoved || v.RelativePath != "" {
		t.Fatalf("row %+v, want it to stay removed", v)
	}
	if files := r.files(); len(files) != 0 {
		t.Fatalf("files kept for a removed video: %v", files)
	}
	if js := r.jobs(KindDownload); len(js) != 1 || js[0].State != "done" || js[0].Result["skipped"] != "gone" {
		t.Fatalf("job %+v", js)
	}
	for _, c := range r.fake.Calls(t) {
		if strings.Contains(strings.Join(c, " "), " -o ") {
			t.Fatalf("yt-dlp downloaded a removed video: %q", c)
		}
	}
	for _, ev := range r.live.all() {
		if ev.Video.State != StateRemoved {
			t.Fatalf("live event %+v for a removed video", ev)
		}
	}
}

func TestDownloadFailingAfterRemoveStaysRemoved(t *testing.T) {
	t.Parallel()
	r := newRig(t, "fail")
	id := addVideo(t, r.s, Video{UserID: anna, VideoID: "abcdefghijk"})
	r.live.onEvent = func(ev liveEvent) {
		if ev.Video.ID == id && ev.Video.State == StateDownloading {
			if err := r.s.Write(context.Background(), func(tx *sql.Tx) error { return MarkRemoved(tx, anna, id) }); err != nil {
				t.Error(err)
			}
		}
	}
	r.download(anna, id)
	if v := r.videos(anna)["abcdefghijk"]; v.State != StateRemoved {
		t.Fatalf("row %q, want it to stay removed", v.State)
	}
	// One attempt only: the job does not try a removed video again.
	if js := r.jobs(KindDownload); len(js) != 1 || js[0].State != "done" || js[0].Attempt != 1 || js[0].Result["skipped"] != "gone" {
		t.Fatalf("job %+v", js)
	}
	if files := r.files(); len(files) != 0 {
		t.Fatalf("files kept for a removed video: %v", files)
	}
}

// cancelMidDownload cancels the download job once yt-dlp reports progress,
// first removing the video when remove is set.
func cancelMidDownload(t *testing.T, remove bool) (*rig, int64) {
	r := newRig(t, "hang")
	id := addVideo(t, r.s, Video{UserID: anna, VideoID: "abcdefghijk"})
	var once sync.Once
	r.live.onEvent = func(ev liveEvent) {
		if ev.Video.ID != id || ev.Video.State != StateDownloading || ev.Video.Percent == 0 {
			return
		}
		once.Do(func() {
			if remove {
				if err := r.s.Write(context.Background(), func(tx *sql.Tx) error { return MarkRemoved(tx, anna, id) }); err != nil {
					t.Error(err)
				}
			}
			job, err := r.runningJob(KindDownload)
			if err == nil {
				err = r.q.Cancel(context.Background(), job)
			}
			if err != nil {
				t.Error(err)
			}
		})
	}
	r.download(anna, id)
	if js := r.jobs(KindDownload); len(js) != 1 || js[0].State != "cancelled" {
		t.Fatalf("job %+v", js)
	}
	if files := r.files(); len(files) != 0 {
		t.Fatalf("files kept after a cancel: %v", files)
	}
	return r, id
}

func TestDownloadCancelledGoesBackInTheQueue(t *testing.T) {
	t.Parallel()
	r, _ := cancelMidDownload(t, false)
	if v := r.videos(anna)["abcdefghijk"]; v.State != StateQueued {
		t.Fatalf("row %q, want queued for the next run", v.State)
	}
}

func TestDownloadCancelledAfterRemoveStaysRemoved(t *testing.T) {
	t.Parallel()
	r, _ := cancelMidDownload(t, true)
	if v := r.videos(anna)["abcdefghijk"]; v.State != StateRemoved {
		t.Fatalf("row %q, want it to stay removed", v.State)
	}
}
