package youtube

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

func playlistFollow(ytID string) Follow {
	return Follow{Kind: KindPlaylist, YTID: ytID, URL: "https://www.youtube.com/playlist?list=" + ytID, Title: "Test playlist"}
}

// listPlaylist makes the fake's playlist hold ids, saying it has count. A
// count below zero is the number of ids, which is what yt-dlp says when
// YouTube gave it no count: a listing cut short then looks whole.
func (r *rig) listPlaylist(count int, ids ...string) {
	r.t.Helper()
	if count < 0 {
		count = len(ids)
	}
	pl := tab{Type: "playlist", ID: "PLtestlist01", Title: "Test playlist", Count: &count, Entries: []tabEntry{}}
	for _, id := range ids {
		pl.Entries = append(pl.Entries, entry(id, "Video "+id, 0, ""))
	}
	r.putJSON("playlist.json", pl)
}

func (r *rig) mp4s() []string {
	r.t.Helper()
	var out []string
	for _, f := range r.files() {
		if strings.HasSuffix(f, ".mp4") {
			out = append(out, filepath.Base(f))
		}
	}
	slices.Sort(out)
	return out
}

func (r *rig) addByLink(userID int64, videoID string) Video {
	r.t.Helper()
	v, err := r.svc.AddVideo(context.Background(), userID, "https://www.youtube.com/watch?v="+videoID, 0)
	if err != nil {
		r.t.Fatal(err)
	}
	return v
}

// A video two follows list is one row and one file. Unfollowing one with
// its files deleted only unlinks what the other holds, and a video added by
// its own link outlives both.
func TestVideoInTwoFollowsIsStoredOnce(t *testing.T) {
	t.Parallel()
	r := newRig(t, "ok")
	ctx := context.Background()
	a := r.follow(anna, playlistFollow("PLtestlist01"))
	r.check(a.ID)
	if c := countStates(r.videos(anna)); c[StateDone] != 3 || len(c) != 1 {
		t.Fatalf("first follow %v", c)
	}
	downloads := len(r.jobs(KindDownload))

	b := r.follow(anna, playlistFollow("PLtestlist02"))
	r.check(b.ID)
	if n := len(r.jobs(KindDownload)); n != downloads {
		t.Fatalf("the second follow downloaded again: %d jobs, want %d", n, downloads)
	}
	if got := r.mp4s(); len(got) != 3 {
		t.Fatalf("files %v, want one per video", got)
	}
	for id, v := range r.videos(anna) {
		if !slices.Equal(v.FollowIDs, []int64{a.ID, b.ID}) || v.Manual {
			t.Fatalf("%s: follows %v manual %v", id, v.FollowIDs, v.Manual)
		}
	}
	for _, f := range []Follow{a, b} {
		if vs, err := VideosOf(ctx, r.s.Read(), anna, VideoFilter{FollowID: f.ID}); err != nil || len(vs) != 3 {
			t.Fatalf("filter by follow %d: %d videos, %v", f.ID, len(vs), err)
		}
	}
	// Someone else's follow never takes hold of Anna's video.
	other := r.follow(1, playlistFollow("PLtestlist03"))
	row := r.videos(anna)["aaaaaaaaaa1"]
	write(t, r.s, func(tx *sql.Tx) error { return LinkVideo(tx, anna, row.ID, other.ID, r.now) })
	write(t, r.s, func(tx *sql.Tx) error { return LinkVideo(tx, 1, row.ID, other.ID, r.now) })
	if got := r.videos(anna)["aaaaaaaaaa1"].FollowIDs; !slices.Equal(got, []int64{a.ID, b.ID}) {
		t.Fatalf("linked across people: %v", got)
	}

	if err := r.svc.Unfollow(ctx, anna, b.ID, true); err != nil {
		t.Fatal(err)
	}
	if got := r.mp4s(); len(got) != 3 {
		t.Fatalf("unfollowing one of two follows deleted files: %v", got)
	}
	for id, v := range r.videos(anna) {
		if !slices.Equal(v.FollowIDs, []int64{a.ID}) || v.State != StateDone {
			t.Fatalf("%s after unfollow: follows %v state %s", id, v.FollowIDs, v.State)
		}
	}

	// Added by its link: manual, not downloaded again.
	v := r.addByLink(anna, "aaaaaaaaaa1")
	if !v.Manual || v.State != StateDone || !slices.Equal(v.FollowIDs, []int64{a.ID}) {
		t.Fatalf("added by link: %+v", v)
	}
	if n := len(r.jobs(KindDownload)); n != downloads {
		t.Fatalf("adding a video already here queued a download: %d jobs", n)
	}
	if err := r.svc.Unfollow(ctx, anna, a.ID, true); err != nil {
		t.Fatal(err)
	}
	got := r.videos(anna)
	if len(got) != 1 || got["aaaaaaaaaa1"].State != StateDone || len(got["aaaaaaaaaa1"].FollowIDs) != 0 {
		t.Fatalf("after the last unfollow: %v", got)
	}
	if files := r.mp4s(); len(files) != 1 || !strings.Contains(files[0], "aaaaaaaaaa1") {
		t.Fatalf("files %v, want only the manual video", files)
	}
}

// Keep-last lets go of a video another holder keeps without deleting it,
// and never takes it back.
func TestKeepLastSparesHeldVideos(t *testing.T) {
	t.Parallel()
	r := newRig(t, "ok")
	ctx := context.Background()
	day := int64(24 * 3600)
	ups := map[string]int64{"aaaaaaaaaaa": 1700000000 + 2*day, "bbbbbbbbbbb": 1700000000, "ccccccccccc": 1700000000 + day}
	var es []tabEntry
	for id, ts := range ups {
		r.putJSON("info-"+id+".json", info(id, "Video "+id, "Veritasium", ts))
		es = append(es, entry(id, "Video "+id, ts, ""))
	}
	r.putJSON("channel_videos.json", channelTab(es...))
	r.putJSON("channel_streams.json", channelTab())
	oldest := addVideo(t, r.s, Video{UserID: anna, VideoID: "bbbbbbbbbbb", Manual: true})
	f := veritasium()
	f.KeepLast, f.Backfill = intp(2), intp(3)
	fl := r.follow(anna, f)
	r.check(fl.ID)

	got := r.videos(anna)
	if c := countStates(got); c[StateDone] != 3 || len(c) != 1 {
		t.Fatalf("states %v, want the held video kept", c)
	}
	if b := got["bbbbbbbbbbb"]; b.ID != oldest || len(b.FollowIDs) != 0 || b.RelativePath == "" {
		t.Fatalf("held video %+v, want let go by the follow with its file kept", b)
	}
	if vs, _ := VideosOf(ctx, r.s.Read(), anna, VideoFilter{FollowID: fl.ID}); len(vs) != 2 {
		t.Fatalf("the follow shows %d videos, want its newest 2", len(vs))
	}
	if n := len(r.mp4s()); n != 3 {
		t.Fatalf("%d files, want 3", n)
	}
	r.setNow(r.now.Add(6 * time.Hour))
	r.check(fl.ID)
	if b := r.videos(anna)["bbbbbbbbbbb"]; len(b.FollowIDs) != 0 || b.State != StateDone {
		t.Fatalf("the next check took the video back: %+v", b)
	}
	if err := r.svc.Unfollow(ctx, anna, fl.ID, true); err != nil {
		t.Fatal(err)
	}
	if got := r.videos(anna); len(got) != 1 || got["bbbbbbbbbbb"].State != StateDone || len(r.mp4s()) != 1 {
		t.Fatalf("after unfollow %v files %v", got, r.mp4s())
	}
}

// A mirrored playlist takes every video, whatever its backfill, and deletes
// what leaves the playlist: only on a listing that looks complete, only
// once two such listings in a row lack the video, and only what nothing
// else holds.
func TestMirrorRemovesWhatLeftThePlaylist(t *testing.T) {
	t.Parallel()
	r := newRig(t, "ok")
	ctx := context.Background()
	f := playlistFollow("PLtestlist01")
	f.Mirror, f.Backfill = true, intp(1)
	fl := r.follow(anna, f)
	all := []string{"aaaaaaaaaa1", "aaaaaaaaaa2", "aaaaaaaaaa3"}
	r.listPlaylist(-1, all...)
	r.check(fl.ID)
	if c := countStates(r.videos(anna)); c[StateDone] != 3 || len(c) != 1 {
		t.Fatalf("first check %v, want every video whatever the backfill", c)
	}
	unchanged := func(why string) {
		t.Helper()
		if c := countStates(r.videos(anna)); c[StateDone] != 3 || len(c) != 1 || len(r.mp4s()) != 3 {
			t.Fatalf("%s removed something: %v files %v", why, c, r.mp4s())
		}
	}
	twice := func() {
		t.Helper()
		r.check(fl.ID)
		r.check(fl.ID)
	}

	// A page of the listing failed: yt-dlp ends well, warns, and counts
	// only what it got. However often, nothing goes.
	r.put("list-warns", nil)
	r.listPlaylist(-1, all[:2]...)
	twice()
	r.check(fl.ID)
	unchanged("a listing with a warning")
	if err := os.Remove(filepath.Join(r.data, "list-warns")); err != nil {
		t.Fatal(err)
	}
	// YouTube says three, the listing has two.
	r.listPlaylist(3, all[:2]...)
	twice()
	unchanged("a listing shorter than its count")
	// An empty listing never empties the library.
	r.listPlaylist(-1)
	twice()
	unchanged("an empty listing")
	// A playlist gone private or deleted fails to list.
	r.put("list-fails", nil)
	twice()
	unchanged("a failed listing")
	if got, _ := FollowOf(ctx, r.s.Read(), anna, fl.ID); got.LastError == "" {
		t.Fatal("the failed listing left no error on the follow")
	}
	if err := os.Remove(filepath.Join(r.data, "list-fails")); err != nil {
		t.Fatal(err)
	}
	// Paused: not listed at all.
	if _, err := r.svc.UpdateFollow(ctx, anna, fl.ID, FollowChange{Enabled: ptr(false)}); err != nil {
		t.Fatal(err)
	}
	r.listPlaylist(-1, all[0])
	twice()
	unchanged("a paused follow")
	if _, err := r.svc.UpdateFollow(ctx, anna, fl.ID, FollowChange{Enabled: ptr(true)}); err != nil {
		t.Fatal(err)
	}

	// Cut short with no warning and a count to match: it looks complete.
	// One such listing removes nothing, and the full listing after it
	// forgets that the video was ever missing.
	r.listPlaylist(-1, all[:2]...)
	r.check(fl.ID)
	unchanged("one short listing that looked complete")
	r.listPlaylist(-1, all...)
	r.check(fl.ID)
	r.listPlaylist(-1, all[:2]...)
	r.check(fl.ID)
	unchanged("a short listing after the video was back")

	// Gone from a second listing in a row: file and row go.
	r.check(fl.ID)
	got := r.videos(anna)
	if _, still := got["aaaaaaaaaa3"]; still || len(got) != 2 {
		t.Fatalf("after the video left: %v", got)
	}
	if files := r.mp4s(); len(files) != 2 || strings.Contains(strings.Join(files, " "), "aaaaaaaaaa3") {
		t.Fatalf("files %v", files)
	}
	var removed bool
	for _, ev := range r.live.all() {
		removed = removed || (ev.User == anna && ev.Video.State == StateRemoved)
	}
	if !removed {
		t.Fatal("the owner was not told the video went")
	}

	// A manual video and one another follow holds only leave the mirror.
	r.addByLink(anna, "aaaaaaaaaa1")
	other := r.follow(anna, veritasium())
	second := r.videos(anna)["aaaaaaaaaa2"]
	write(t, r.s, func(tx *sql.Tx) error { return LinkVideo(tx, anna, second.ID, other.ID, r.now) })
	r.listPlaylist(-1, "aaaaaaaaaa4")
	twice()
	got = r.videos(anna)
	if v := got["aaaaaaaaaa1"]; v.State != StateDone || len(v.FollowIDs) != 0 || !v.Manual {
		t.Fatalf("manual video %+v", v)
	}
	if v := got["aaaaaaaaaa2"]; v.State != StateDone || !slices.Equal(v.FollowIDs, []int64{other.ID}) {
		t.Fatalf("video of another follow %+v", v)
	}
	if v := got["aaaaaaaaaa4"]; v.State != StateDone || !slices.Equal(v.FollowIDs, []int64{fl.ID}) {
		t.Fatalf("new video %+v", v)
	}
	if n := len(r.mp4s()); n != 3 {
		t.Fatalf("%d files, want 3", n)
	}
}

// A playlist that is not mirrored never removes anything, and a mirror
// turned on later takes the videos its backfill passed over. What was seen
// missing before a switch does not count after it.
func TestMirrorTurnedOnLater(t *testing.T) {
	t.Parallel()
	r := newRig(t, "ok")
	ctx := context.Background()
	f := playlistFollow("PLtestlist01")
	f.Backfill = intp(1)
	fl := r.follow(anna, f)
	r.listPlaylist(-1, "aaaaaaaaaa1", "aaaaaaaaaa2", "aaaaaaaaaa3")
	r.check(fl.ID)
	if c := countStates(r.videos(anna)); c[StateDone] != 1 || c[StateSkipped] != 2 {
		t.Fatalf("first check %v", c)
	}
	r.listPlaylist(-1, "aaaaaaaaaa2")
	r.check(fl.ID)
	r.check(fl.ID)
	if c := countStates(r.videos(anna)); c[StateDone] != 1 || c[StateSkipped] != 2 || len(r.mp4s()) != 1 {
		t.Fatalf("a playlist that is not mirrored removed something: %v", c)
	}
	mirror := func(on bool) {
		t.Helper()
		got, err := r.svc.UpdateFollow(ctx, anna, fl.ID, FollowChange{Mirror: ptr(on)})
		if err != nil || got.Mirror != on {
			t.Fatalf("mirror %v: %+v %v", on, got, err)
		}
		r.drain() // turning it on queues a check
	}
	mirror(true)
	if vs := r.videos(anna); len(vs) != 3 || vs["aaaaaaaaaa2"].State != StateDone {
		t.Fatalf("after the first mirrored check: %v, want the video in the playlist downloaded and nothing gone yet", countStates(vs))
	}
	// Off and on again: the first check after it starts counting afresh.
	mirror(false)
	mirror(true)
	if vs := r.videos(anna); len(vs) != 3 {
		t.Fatalf("a mark from before the switch removed videos: %v", countStates(vs))
	}
	r.check(fl.ID)
	vs := r.videos(anna)
	if len(vs) != 1 || vs["aaaaaaaaaa2"].State != StateDone {
		t.Fatalf("after mirroring: %v, want only the video still in the playlist, downloaded", vs)
	}
	if files := r.mp4s(); len(files) != 1 || !strings.Contains(files[0], "aaaaaaaaaa2") {
		t.Fatalf("files %v", files)
	}
}

// A playlist that kept only its newest and is then mirrored downloads again
// what keep-last deleted, and holds it.
func TestMirrorTakesBackWhatKeepLastLetGo(t *testing.T) {
	t.Parallel()
	r := newRig(t, "ok")
	ctx := context.Background()
	f := playlistFollow("PLtestlist01")
	f.KeepLast = intp(2)
	fl := r.follow(anna, f)
	r.listPlaylist(3, "aaaaaaaaaa1", "aaaaaaaaaa2", "aaaaaaaaaa3")
	r.check(fl.ID)
	if c := countStates(r.videos(anna)); c[StateDone] != 2 || c[StateRemoved] != 1 {
		t.Fatalf("with keep-last: %v", c)
	}
	if _, err := r.svc.UpdateFollow(ctx, anna, fl.ID, FollowChange{Mirror: ptr(true)}); err != nil {
		t.Fatal(err)
	}
	r.drain()
	for id, v := range r.videos(anna) {
		if v.State != StateDone || !slices.Equal(v.FollowIDs, []int64{fl.ID}) {
			t.Fatalf("%s after mirroring: state %s follows %v", id, v.State, v.FollowIDs)
		}
	}
	if n := len(r.mp4s()); n != 3 {
		t.Fatalf("%d files, want every video of the playlist", n)
	}
}

// A video whose files cannot be deleted is kept on the follow as its
// error; the check itself goes on.
func TestMirrorRemovalFailureStaysOnTheFollow(t *testing.T) {
	t.Parallel()
	r := newRig(t, "ok")
	ctx := context.Background()
	f := playlistFollow("PLtestlist01")
	f.Mirror = true
	fl := r.follow(anna, f)
	r.listPlaylist(-1, "aaaaaaaaaa1", "aaaaaaaaaa2")
	r.check(fl.ID)
	gone := r.videos(anna)["aaaaaaaaaa2"]
	// A row whose path leaves the YouTube folder is refused, not deleted.
	write(t, r.s, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE youtube_videos SET relative_path = '../outside.mp4' WHERE id = ?", gone.ID)
		return err
	})
	r.listPlaylist(-1, "aaaaaaaaaa1")
	r.check(fl.ID)
	r.check(fl.ID)
	if js := r.jobs(KindCheck); js[len(js)-1].State != "done" {
		t.Fatalf("check job %s", js[len(js)-1].State)
	}
	got, err := FollowOf(ctx, r.s.Read(), anna, fl.ID)
	if err != nil || !strings.Contains(got.LastError, "could not be removed") || strings.Contains(got.LastError, "outside") {
		t.Fatalf("follow error %q, %v", got.LastError, err)
	}
}

func ptr[T any](v T) *T { return &v }

func TestMirrorRules(t *testing.T) {
	t.Parallel()
	svc, _ := addService(t, "ok")
	ctx := context.Background()
	if _, err := svc.AddFollow(ctx, 1, AddFollow{URL: "https://www.youtube.com/@Veritasium", Mirror: true}); !errors.Is(err, ErrMirrorKind) || !IsRefusal(err) {
		t.Fatalf("mirrored channel: %v", err)
	}
	pl := "https://www.youtube.com/playlist?list=PLtestlist01"
	if _, err := svc.AddFollow(ctx, 1, AddFollow{URL: pl, Mirror: true, KeepLast: intp(5)}); !errors.Is(err, ErrMirrorKeep) || !IsRefusal(err) {
		t.Fatalf("mirror with keep-last: %v", err)
	}
	f, err := svc.AddFollow(ctx, 1, AddFollow{URL: pl, Mirror: true, Backfill: intp(2)})
	if err != nil || !f.Mirror || f.Backfill != nil || f.KeepLast != nil {
		t.Fatalf("mirrored playlist %+v, %v", f, err)
	}
	if _, err := svc.UpdateFollow(ctx, 1, f.ID, FollowChange{KeepLast: ptr(intp(3))}); !errors.Is(err, ErrMirrorKeep) {
		t.Fatalf("keep-last on a mirror: %v", err)
	}
	if got, err := svc.UpdateFollow(ctx, 1, f.ID, FollowChange{Mirror: ptr(false), KeepLast: ptr(intp(3))}); err != nil || got.Mirror || got.KeepLast == nil || *got.KeepLast != 3 {
		t.Fatalf("mirror off with keep-last: %+v, %v", got, err)
	}
	got, err := svc.UpdateFollow(ctx, 1, f.ID, FollowChange{Mirror: ptr(true)})
	if err != nil || !got.Mirror || got.KeepLast != nil {
		t.Fatalf("mirror on: %+v, %v; want keep-last off", got, err)
	}
	if _, err := svc.UpdateFollow(ctx, 2, f.ID, FollowChange{Mirror: ptr(false)}); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("someone else's follow: %v", err)
	}
	ch, err := svc.AddFollow(ctx, 1, AddFollow{URL: "https://www.youtube.com/@Veritasium"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateFollow(ctx, 1, ch.ID, FollowChange{Mirror: ptr(true)}); !errors.Is(err, ErrMirrorKind) {
		t.Fatalf("mirroring a channel: %v", err)
	}
}

// Unfollowing while keeping the files makes the videos manual, so a mirror
// that also holds them never deletes them.
func TestUnfollowKeepingFilesMakesVideosManual(t *testing.T) {
	t.Parallel()
	svc, s := addService(t, "ok")
	ctx := context.Background()
	root := t.TempDir()
	if _, err := svc.SaveSettings(ctx, Settings{Folder: root, MinFreeGB: 1}, database.System); err != nil {
		t.Fatal(err)
	}
	fid := addFollow(t, s, Follow{UserID: 2, Kind: KindPlaylist, YTID: "PLkeepkeep01", Enabled: true, AddedAt: time.Now()})
	done, _ := doneVideo(t, s, root, 2, &fid, "kkkkkkkkkk1")
	skipped := addVideo(t, s, Video{UserID: 2, FollowIDs: []int64{fid}, VideoID: "kkkkkkkkkk2", State: StateSkipped})
	if err := svc.Unfollow(ctx, 2, fid, false); err != nil {
		t.Fatal(err)
	}
	if v, err := VideoOf(ctx, s.Read(), 2, done); err != nil || !v.Manual {
		t.Fatalf("kept video %+v, %v", v, err)
	}
	if v, err := VideoOf(ctx, s.Read(), 2, skipped); err != nil || v.Manual {
		t.Fatalf("a video never downloaded became manual: %+v, %v", v, err)
	}
}

// Removed and added again while its download ran: the same job starts the
// video over instead of leaving it queued until the next check.
func TestReAddedWhileDownloadingIsPickedUp(t *testing.T) {
	t.Parallel()
	r := newRig(t, "ok")
	id := addVideo(t, r.s, Video{UserID: anna, VideoID: "abcdefghijk"})
	var once sync.Once
	r.live.onEvent = func(ev liveEvent) {
		if ev.Video.ID != id || ev.Video.State != StateDownloading || ev.Video.Percent == 0 {
			return
		}
		once.Do(func() {
			err := r.s.Write(context.Background(), func(tx *sql.Tx) error {
				if err := MarkRemoved(tx, anna, id); err != nil {
					return err
				}
				return RequeueVideo(tx, anna, id, 720)
			})
			if err != nil {
				t.Error(err)
			}
			// The new job is the running one: nothing new is queued.
			r.svc.queueDownloadNow(context.Background(), anna, id)
		})
	}
	r.download(anna, id)
	// The running job starts the video over. A second job waits behind it
	// in case the first was already past its last look; here it finds the
	// video settled and does nothing.
	js := r.jobs(KindDownload)
	if len(js) != 2 || js[0].State != "done" || js[1].State != "done" || js[1].Result["skipped"] != "settled" {
		t.Fatalf("jobs %+v, want the first done and a second that had nothing to do", js)
	}
	v := r.videos(anna)["abcdefghijk"]
	if v.State != StateDone || v.MaxHeight != 720 || v.RelativePath == "" {
		t.Fatalf("row %+v, want downloaded at the new quality", v)
	}
	if files := r.mp4s(); len(files) != 1 {
		t.Fatalf("files %v", files)
	}
	downloads := 0
	for _, c := range r.fake.Calls(t) {
		if slices.Contains(c, "-o") {
			downloads++
		}
	}
	if downloads != 2 {
		t.Fatalf("yt-dlp downloaded %d times, want 2", downloads)
	}
}

// Removing the last video of a folder removes the folder, but never the
// person's own folder or the YouTube folder.
func TestRemovingTheLastVideoRemovesItsFolder(t *testing.T) {
	t.Parallel()
	svc, s := addService(t, "ok")
	ctx := context.Background()
	root := t.TempDir()
	if _, err := svc.SaveSettings(ctx, Settings{Folder: root, MinFreeGB: 1}, database.System); err != nil {
		t.Fatal(err)
	}
	a, afile := doneVideo(t, s, root, 2, nil, "aaaaaaaaaaa")
	b, _ := doneVideo(t, s, root, 2, nil, "bbbbbbbbbbb")
	folder := filepath.Dir(afile)
	if err := svc.RemoveVideo(ctx, 2, a); err != nil {
		t.Fatal(err)
	}
	if !exists(folder) {
		t.Fatal("the folder went while a video was still in it")
	}
	// What a NAS or a file browser leaves does not keep a folder.
	if err := os.MkdirAll(filepath.Join(folder, "@eaDir", "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, ".DS_Store"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := svc.RemoveVideo(ctx, 2, b); err != nil {
		t.Fatal(err)
	}
	if exists(folder) {
		t.Fatal("the empty folder stayed")
	}
	if !exists(filepath.Dir(folder)) || !exists(root) {
		t.Fatal("the person's folder or the YouTube folder went")
	}

	// A file kept by hand keeps its folder; a video right in the person's
	// folder never takes that folder with it.
	c, cfile := doneVideo(t, s, root, 2, nil, "ccccccccccc")
	if err := os.WriteFile(filepath.Join(filepath.Dir(cfile), "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := svc.RemoveVideo(ctx, 2, c); err != nil {
		t.Fatal(err)
	}
	if !exists(filepath.Dir(cfile)) {
		t.Fatal("a folder with someone's own file in it went")
	}
	if err := os.RemoveAll(filepath.Dir(cfile)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "2-p", "loose.mp4"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := removeFiles(root, "2-p/loose.mp4"); err != nil || !exists(filepath.Join(root, "2-p")) {
		t.Fatalf("the person's folder went: %v", err)
	}
	// A linked folder is never followed or removed.
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "2-p", "link")); err != nil {
		t.Fatal(err)
	}
	dir, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	pruneEmpty(dir, "2-p/link")
	if !exists(outside) || !exists(filepath.Join(root, "2-p", "link")) {
		t.Fatal("a link was removed or followed")
	}
}

// A job cancelled while it wrote the finished download leaves no file
// behind; a write that landed after all keeps it.
func TestCancelDuringTheFinalWriteCleansUp(t *testing.T) {
	t.Parallel()
	r := newRig(t, "ok")
	id := addVideo(t, r.s, Video{UserID: anna, VideoID: "abcdefghijk", State: StateDownloading})
	v, err := VideoByID(context.Background(), r.s.Read(), id)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(r.root, "7-Anna", "Chan", "2024-01-01 T [abcdefghijk].mp4")
	put := func() {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		for _, f := range []string{file, thumbOf(file)} {
			if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	put()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := r.svc.finish(ctx, r.root, v, file, Meta{Title: "T"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("finish on a cancelled job: %v", err)
	}
	if files := r.files(); len(files) != 0 || exists(filepath.Dir(file)) {
		t.Fatalf("left behind: %v", files)
	}
	if got := r.videos(anna)["abcdefghijk"]; got.State != StateDownloading || got.RelativePath != "" {
		t.Fatalf("row %+v", got)
	}

	put()
	if err := r.svc.finish(context.Background(), r.root, v, file, Meta{Title: "T"}); err != nil {
		t.Fatal(err)
	}
	r.svc.dropUnlessKept(r.root, id, file)
	if !exists(file) || !exists(thumbOf(file)) {
		t.Fatal("the file of a downloaded row was deleted")
	}
}

// With playlists, a video in two folders is copied once and joins both
// follows; without one for its folder the second copy is still skipped.
func TestAdoptLinksAVideoInTwoFolders(t *testing.T) {
	t.Parallel()
	r, _ := adoptRig(t)
	src := t.TempDir()
	for name, body := range map[string]string{
		"Course/01 - First.mp4":  "first video",
		"Course/01 - First.nfo":  nfo("aaaaaaaaaa1", "First", "Some Owner", "2020-01-01 00:00:00Z"),
		"Faves/First again.mp4":  "first video",
		"Faves/First again.nfo":  nfo("aaaaaaaaaa1", "First", "Some Owner", "2020-01-01 00:00:00Z"),
		"Loose/First thrice.mp4": "first video",
		"Loose/First thrice.nfo": nfo("aaaaaaaaaa1", "First", "Some Owner", "2020-01-01 00:00:00Z"),
	} {
		p := filepath.Join(src, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	in := AdoptRequest{UserID: anna, Path: src, Playlists: map[string]string{
		"Course": "https://www.youtube.com/playlist?list=PLtestlist01",
		"Faves":  "https://www.youtube.com/playlist?list=PLtestlist02",
	}}
	items, err := r.svc.PlanAdopt(context.Background(), in)
	if err != nil || len(items) != 3 {
		t.Fatalf("plan %v, %v", items, err)
	}
	if items[0].Reason != "" || items[1].Reason != "" || items[1].To != items[0].To || items[2].Reason != reasonTwice {
		t.Fatalf("plan %+v", items)
	}
	res := r.adopt(in)
	if res["copied"] != float64(1) || res["linked"] != float64(1) || res["skipped"] != float64(1) || res["follows"] != float64(2) || res["failed"] != float64(0) {
		t.Fatalf("result %v", res)
	}
	fs, err := FollowsOf(context.Background(), r.s.Read(), anna)
	if err != nil || len(fs) != 2 {
		t.Fatalf("follows %v, %v", fs, err)
	}
	v := r.videos(anna)["aaaaaaaaaa1"]
	if v.State != StateDone || v.Manual || !slices.Equal(v.FollowIDs, []int64{fs[0].ID, fs[1].ID}) {
		t.Fatalf("row %+v", v)
	}
	if files := r.mp4s(); len(files) != 1 {
		t.Fatalf("files %v, want one copy", files)
	}
	// Unfollowing one playlist with its files deleted keeps the copy.
	if err := r.svc.Unfollow(context.Background(), anna, fs[0].ID, true); err != nil {
		t.Fatal(err)
	}
	if files := r.mp4s(); len(files) != 1 {
		t.Fatalf("files %v after unfollowing one of the two", files)
	}
}
