package youtube

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/youtube/youtubetest"
)

const veritasiumUC = "UCHnyfMqiRRG1u-2MsSQLbXA"

// addService is a Service over the fake yt-dlp in mode, with a queue that
// holds jobs without running them.
func addService(t *testing.T, mode string) (*Service, *database.Store) {
	t.Helper()
	s := openStore(t)
	f := youtubetest.NewFake(t, mode)
	svc := testService(s)
	svc.Queue = jobs.New(s, jobs.Options{})
	svc.Tools = func() Tools { return Tools{YTDLP: f.Bin, FFmpeg: f.Bin} }
	return svc, s
}

// queuedJobs reads the queued jobs of kind as dedupe key -> params.
func queuedJobs(t *testing.T, s *database.Store, kind string) map[string]map[string]any {
	t.Helper()
	list, err := database.Select[jobs.Job](context.Background(), s.Read(), "jobs", "kind = ? AND state = 'queued'", kind)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]map[string]any{}
	for _, j := range list {
		key := ""
		if j.DedupeKey != nil {
			key = *j.DedupeKey
		}
		out[key] = j.Params
	}
	return out
}

func intp(n int) *int { return &n }

func TestAddFollowDefaults(t *testing.T) {
	t.Parallel()
	svc, s := addService(t, "ok")
	ctx := context.Background()

	ch, err := svc.AddFollow(ctx, 2, AddFollow{URL: "https://www.youtube.com/@veritasium"})
	if err != nil {
		t.Fatal(err)
	}
	if ch.Kind != KindChannel || ch.YTID != veritasiumUC || ch.Title != "Veritasium" || ch.Backfill == nil || *ch.Backfill != 20 ||
		ch.MaxHeight != 1080 || ch.IncludeShorts || ch.KeepLast != nil || !ch.Enabled || ch.URL != "https://www.youtube.com/channel/"+veritasiumUC {
		t.Fatalf("channel defaults %+v", ch)
	}
	jobsNow := queuedJobs(t, s, KindCheck)
	key := "youtube:2:follow:" + itoa(ch.ID)
	if p, ok := jobsNow[key]; !ok || len(p) != 1 || p["follow_id"] != float64(ch.ID) {
		t.Fatalf("check job %v", jobsNow)
	}

	pl, err := svc.AddFollow(ctx, 2, AddFollow{URL: "https://www.youtube.com/playlist?list=PLtestlist01", IncludeShorts: true})
	if err != nil {
		t.Fatal(err)
	}
	if pl.Kind != KindPlaylist || pl.YTID != "PLtestlist01" || pl.Backfill != nil || pl.IncludeShorts || pl.Title != "Test playlist" {
		t.Fatalf("playlist defaults %+v", pl)
	}

	// The same channel by its UC id is a duplicate, pointing at the first.
	dup, err := svc.AddFollow(ctx, 2, AddFollow{URL: "https://www.youtube.com/channel/" + veritasiumUC + "/videos"})
	if !errors.Is(err, ErrDuplicateFollow) || dup.ID != ch.ID {
		t.Fatalf("duplicate: %+v %v", dup, err)
	}
	// Another person may follow it too.
	other, err := svc.AddFollow(ctx, 1, AddFollow{URL: "https://www.youtube.com/@veritasium", Backfill: intp(50), KeepLast: intp(10), MaxHeight: 720, IncludeShorts: true})
	if err != nil {
		t.Fatal(err)
	}
	// Never download what keep_last would delete at once.
	if *other.Backfill != 10 || *other.KeepLast != 10 || other.MaxHeight != 720 || !other.IncludeShorts {
		t.Fatalf("backfill capped by keep_last %+v", other)
	}
	// Keep-last caps "all" too.
	all, err := svc.AddFollow(ctx, 1, AddFollow{URL: "https://www.youtube.com/playlist?list=PLtestlist01", KeepLast: intp(3)})
	if err != nil || all.Backfill == nil || *all.Backfill != 3 {
		t.Fatalf("all capped by keep_last %+v %v", all, err)
	}

	for _, bad := range []AddFollow{
		{URL: "https://example.com/@veritasium"},
		{URL: "https://youtu.be/dQw4w9WgXcQ"},
		{URL: "https://www.youtube.com/@other", Backfill: intp(-1)},
		{URL: "https://www.youtube.com/@other", KeepLast: intp(0)},
		{URL: "https://www.youtube.com/@other", MaxHeight: 99999},
	} {
		if _, err := svc.AddFollow(ctx, 2, bad); err == nil || !IsRefusal(err) {
			t.Fatalf("%+v: %v, want a refusal", bad, err)
		}
	}
	got, err := FollowsOf(ctx, s.Read(), 2)
	if err != nil || len(got) != 2 {
		t.Fatalf("person 2 follows %d %v", len(got), err)
	}
}

func TestAddFollowNeedsTools(t *testing.T) {
	t.Parallel()
	svc, _ := addService(t, "ok")
	svc.Tools = func() Tools { return Tools{YTDLP: "/bin/yt-dlp"} }
	if _, err := svc.AddFollow(context.Background(), 2, AddFollow{URL: "https://www.youtube.com/@veritasium"}); !errors.Is(err, ErrNeedsTools) {
		t.Fatalf("no ffmpeg: %v", err)
	}
	if _, err := svc.Probe(context.Background(), "https://www.youtube.com/@veritasium"); !errors.Is(err, ErrNeedsTools) {
		t.Fatalf("probe without ffmpeg: %v", err)
	}
}

func TestAddVideo(t *testing.T) {
	t.Parallel()
	svc, s := addService(t, "ok")
	ctx := context.Background()
	v, err := svc.AddVideo(ctx, 2, "https://youtu.be/dQw4w9WgXcQ?si=x", 0)
	if err != nil {
		t.Fatal(err)
	}
	if v.VideoID != "dQw4w9WgXcQ" || v.Title != "Never Gonna Give You Up" || v.ChannelName != "Rick Astley" || v.State != StateQueued ||
		v.MaxHeight != 1080 || len(v.FollowIDs) != 0 || !v.Manual || v.DurationSeconds != 212 {
		t.Fatalf("video %+v", v)
	}
	key := "youtube:2:" + itoa(v.ID)
	if p := queuedJobs(t, s, KindDownload)[key]; len(p) != 1 || p["row_id"] != float64(v.ID) {
		t.Fatalf("download job %v", queuedJobs(t, s, KindDownload))
	}
	// Adding it again gives the same row; a removed one comes back queued.
	again, err := svc.AddVideo(ctx, 2, "https://www.youtube.com/watch?v=dQw4w9WgXcQ", 720)
	if err != nil || again.ID != v.ID {
		t.Fatalf("again %+v %v", again, err)
	}
	if err := svc.RemoveVideo(ctx, 2, v.ID); err != nil {
		t.Fatal(err)
	}
	back, err := svc.AddVideo(ctx, 2, "https://www.youtube.com/watch?v=dQw4w9WgXcQ", 720)
	if err != nil || back.ID != v.ID || back.State != StateQueued || back.MaxHeight != 720 {
		t.Fatalf("back %+v %v", back, err)
	}
	if _, err := svc.AddVideo(ctx, 2, "https://www.youtube.com/@veritasium", 0); !errors.Is(err, ErrNotAVideo) {
		t.Fatalf("channel as a video: %v", err)
	}
	if _, err := svc.AddVideo(ctx, 2, "https://evil.example/watch?v=dQw4w9WgXcQ", 0); !errors.Is(err, ErrNotYouTube) {
		t.Fatalf("bad link: %v", err)
	}
}

func TestAddVideoRefusesUpcoming(t *testing.T) {
	t.Parallel()
	svc, s := addService(t, "live")
	_, err := svc.AddVideo(context.Background(), 2, "https://www.youtube.com/watch?v=xxxxxxxxxx1", 0)
	if err == nil || err.Error() != "live streams and premieres can be added once they have aired" {
		t.Fatalf("upcoming: %v", err)
	}
	if got, _ := VideosOf(context.Background(), s.Read(), 2, VideoFilter{}); len(got) != 0 {
		t.Fatalf("a row was added: %+v", got)
	}
	if n := len(queuedJobs(t, s, KindDownload)); n != 0 {
		t.Fatalf("%d download jobs", n)
	}
}

// doneVideo adds a downloaded video of a follow with its file and thumbnail
// under root, and returns the row id and the file path.
func doneVideo(t *testing.T, s *database.Store, root string, userID int64, followID *int64, vid string) (int64, string) {
	t.Helper()
	v := Video{UserID: userID, VideoID: vid}
	if followID != nil {
		v.FollowIDs = []int64{*followID}
	}
	id := addVideo(t, s, v)
	rel := itoa(userID) + "-p/Chan/2024-01-01 T [" + vid + "].mp4"
	file := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{file, thumbOf(file)} {
		if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(t, s, func(tx *sql.Tx) error { return FinishVideo(tx, id, rel, 1, Meta{Title: "T"}) })
	return id, file
}

func exists(f string) bool {
	_, err := os.Stat(f)
	return err == nil
}

func TestUnfollowKeepOrDeleteFiles(t *testing.T) {
	t.Parallel()
	svc, s := addService(t, "ok")
	ctx := context.Background()
	root := t.TempDir()
	if _, err := svc.SaveSettings(ctx, Settings{Folder: root, MinFreeGB: 1}, database.System); err != nil {
		t.Fatal(err)
	}
	follow := func(userID int64, ytID string) int64 {
		var id int64
		write(t, s, func(tx *sql.Tx) error {
			var err error
			id, err = InsertFollow(tx, Follow{UserID: userID, Kind: KindPlaylist, YTID: ytID, Enabled: true})
			return err
		})
		return id
	}

	keep := follow(2, "PLkeepkeep01")
	kv, kfile := doneVideo(t, s, root, 2, &keep, "kkkkkkkkkk1")
	if err := svc.Unfollow(ctx, 2, keep, false); err != nil {
		t.Fatal(err)
	}
	v, err := VideoOf(ctx, s.Read(), 2, kv)
	if err != nil || len(followsOf(t, s, v)) != 0 || !v.Manual || v.State != StateDone || !exists(kfile) || !exists(thumbOf(kfile)) {
		t.Fatalf("keep: %+v %v files %v", v, err, exists(kfile))
	}
	if _, err := FollowOf(ctx, s.Read(), 2, keep); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("follow kept: %v", err)
	}

	del := follow(2, "PLdeldel0001")
	dv, dfile := doneVideo(t, s, root, 2, &del, "ddddddddddd")
	qv := addVideo(t, s, Video{UserID: 2, FollowIDs: []int64{del}, VideoID: "qqqqqqqqqqq"})
	oneOff, ofile := doneVideo(t, s, root, 2, nil, "ooooooooooo")
	// Someone else's follow of their own: untouched, and not theirs to drop.
	theirs := follow(1, "PLdeldel0001")
	tv, tfile := doneVideo(t, s, root, 1, &theirs, "ddddddddddd")
	if err := svc.Unfollow(ctx, 2, theirs, true); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("someone else's follow: %v", err)
	}

	if err := svc.Unfollow(ctx, 2, del, true); err != nil {
		t.Fatal(err)
	}
	if exists(dfile) || exists(thumbOf(dfile)) {
		t.Fatal("delete left the file or thumbnail")
	}
	for _, id := range []int64{dv, qv} {
		if _, err := VideoOf(ctx, s.Read(), 2, id); !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("row %d stayed: %v", id, err)
		}
	}
	if _, err := VideoOf(ctx, s.Read(), 2, oneOff); err != nil || !exists(ofile) {
		t.Fatalf("one-off video went too: %v", err)
	}
	if _, err := VideoOf(ctx, s.Read(), 1, tv); err != nil || !exists(tfile) {
		t.Fatalf("other person's video went: %v", err)
	}
}

func TestRemoveVideo(t *testing.T) {
	t.Parallel()
	svc, s := addService(t, "ok")
	ctx := context.Background()
	root := t.TempDir()
	if _, err := svc.SaveSettings(ctx, Settings{Folder: root, MinFreeGB: 1}, database.System); err != nil {
		t.Fatal(err)
	}
	id, file := doneVideo(t, s, root, 2, nil, "rrrrrrrrrrr")
	if err := svc.RemoveVideo(ctx, 1, id); !errors.Is(err, sql.ErrNoRows) || !exists(file) {
		t.Fatalf("someone else removed it: %v", err)
	}
	if err := svc.RemoveVideo(ctx, 2, id); err != nil {
		t.Fatal(err)
	}
	v, err := VideoOf(ctx, s.Read(), 2, id)
	if err != nil || v.State != StateRemoved || v.RelativePath != "" || v.SizeBytes != 0 || exists(file) || exists(thumbOf(file)) {
		t.Fatalf("removed %+v %v", v, err)
	}
	// A second remove is fine.
	if err := svc.RemoveVideo(ctx, 2, id); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateFollow(t *testing.T) {
	t.Parallel()
	svc, _ := addService(t, "ok")
	ctx := context.Background()
	f, err := svc.AddFollow(ctx, 2, AddFollow{URL: "https://www.youtube.com/@veritasium"})
	if err != nil {
		t.Fatal(err)
	}
	off, kl := false, intp(5)
	got, err := svc.UpdateFollow(ctx, 2, f.ID, FollowChange{Enabled: &off, KeepLast: &kl, MaxHeight: intp(480)})
	if err != nil || got.Enabled || *got.KeepLast != 5 || got.MaxHeight != 480 {
		t.Fatalf("update %+v %v", got, err)
	}
	if _, err := svc.UpdateFollow(ctx, 1, f.ID, FollowChange{Enabled: &off}); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("someone else's follow: %v", err)
	}
	zero := intp(0)
	if _, err := svc.UpdateFollow(ctx, 2, f.ID, FollowChange{KeepLast: &zero}); !IsRefusal(err) {
		t.Fatalf("keep_last 0: %v", err)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
