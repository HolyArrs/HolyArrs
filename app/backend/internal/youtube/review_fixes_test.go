package youtube

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// A listing is complete only without a warning: yt-dlp ends well when a
// page of the listing failed, and then counts only what it got. Listings
// run with warnings on; nothing else does.
func TestListingHearsWarnings(t *testing.T) {
	t.Parallel()
	pl := Link{Kind: KindPlaylist, ID: "PLtestlist01"}
	r, fake := fakeRunner(t, "ok")
	list, err := r.Listing(context.Background(), pl, false)
	if err != nil || !list.Complete || len(list.Entries) != 3 {
		t.Fatalf("clean listing: %+v, %v", list, err)
	}
	if _, err := r.Info(context.Background(), "aaaaaaaaaa1"); err != nil {
		t.Fatal(err)
	}
	calls := fake.Calls(t)
	if len(calls) != 2 || slices.Contains(calls[0], "--no-warnings") || !slices.Contains(calls[1], "--no-warnings") {
		t.Fatalf("warnings: listing %v lookup %v", calls[0], calls[1])
	}
	if !slices.Equal(calls[0][len(calls[0])-2:], []string{"--", "https://www.youtube.com/playlist?list=PLtestlist01"}) {
		t.Fatalf("listing arguments %v", calls[0])
	}

	warned, _ := fakeRunner(t, "warn")
	list, err = warned.Listing(context.Background(), pl, false)
	if err != nil || list.Complete || len(list.Entries) != 3 {
		t.Fatalf("listing with a warning: %+v, %v; want its entries but not complete", list, err)
	}
	// A channel's listing never says it is complete.
	ch, err := r.Listing(context.Background(), Link{Kind: KindChannel, ID: veritasiumUC}, false)
	if err != nil || ch.Complete {
		t.Fatalf("channel listing: complete %v, %v", ch.Complete, err)
	}
}

// Mixes and YouTube's own lists are never mirrored.
func TestMirrorRefusesMixesAndOwnLists(t *testing.T) {
	t.Parallel()
	svc, s := addService(t, "ok")
	ctx := context.Background()
	for _, id := range []string{"RDdQw4w9WgXcQ", "ULabcdefghijk", "LLabcdefghijk", "WLabcdefghijk"} {
		link := "https://www.youtube.com/playlist?list=" + id
		if _, err := svc.AddFollow(ctx, 1, AddFollow{URL: link, Mirror: true}); !errors.Is(err, ErrMirrorList) || !IsRefusal(err) {
			t.Fatalf("%s mirrored when followed: %v", id, err)
		}
		fid := addFollow(t, s, Follow{UserID: 1, Kind: KindPlaylist, YTID: id, URL: link, Enabled: true, AddedAt: time.Now()})
		if _, err := svc.UpdateFollow(ctx, 1, fid, FollowChange{Mirror: ptr(true)}); !errors.Is(err, ErrMirrorList) {
			t.Fatalf("%s mirrored later: %v", id, err)
		}
		if f, err := FollowOf(ctx, s.Read(), 1, fid); err != nil || f.Mirror {
			t.Fatalf("%s: %+v, %v", id, f, err)
		}
	}
	if _, err := svc.AddFollow(ctx, 1, AddFollow{URL: "https://www.youtube.com/playlist?list=PLRDtestlist1", Mirror: true}); err != nil {
		t.Fatalf("an ordinary playlist with RD inside its id: %v", err)
	}
}

// Mirroring or keep-last switched off, or the follow paused or gone, while
// a long check or download ran: nothing more is marked, let go or deleted.
func TestLettingGoStopsWhenTheFollowChanged(t *testing.T) {
	t.Parallel()
	r := newRig(t, "ok")
	ctx := context.Background()
	f := playlistFollow("PLtestlist01")
	f.Mirror = true
	fl := r.follow(anna, f)
	r.listPlaylist(-1, "aaaaaaaaaa1", "aaaaaaaaaa2")
	r.check(fl.ID)
	// Seen missing once, so the next look would delete.
	r.listPlaylist(-1, "aaaaaaaaaa1")
	r.check(fl.ID)
	listed := map[string]bool{"aaaaaaaaaa1": true}
	intact := func(why string) {
		t.Helper()
		if vs := r.videos(anna); len(vs) != 2 || len(r.mp4s()) != 2 || !slices.Equal(vs["aaaaaaaaaa2"].FollowIDs, []int64{fl.ID}) {
			t.Fatalf("%s: videos %v files %v", why, vs, r.mp4s())
		}
	}
	// The follow as the check loaded it still says mirror; the row no
	// longer does.
	write(t, r.s, func(tx *sql.Tx) error { return UpdateFollow(tx, anna, fl.ID, FollowChange{Enabled: ptr(false)}) })
	if err := r.svc.mirror(ctx, anna, fl.ID, listed); err != nil {
		t.Fatal(err)
	}
	intact("paused meanwhile")
	write(t, r.s, func(tx *sql.Tx) error {
		return UpdateFollow(tx, anna, fl.ID, FollowChange{Enabled: ptr(true), Mirror: ptr(false)})
	})
	if err := r.svc.mirror(ctx, anna, fl.ID, listed); err != nil {
		t.Fatal(err)
	}
	intact("mirroring switched off meanwhile")

	// Keep-last switched off between reading what is beyond it and
	// letting it go.
	v := r.videos(anna)["aaaaaaaaaa2"]
	if stopped, err := r.svc.letGo(ctx, r.root, v, fl.ID, false, false); err != nil || !stopped {
		t.Fatalf("keep-last off: stopped %v, %v", stopped, err)
	}
	intact("keep-last off")
	// Unfollowed meanwhile.
	if stopped, err := r.svc.letGo(ctx, r.root, v, fl.ID+99, true, false); err != nil || !stopped {
		t.Fatalf("no such follow: stopped %v, %v", stopped, err)
	}
	intact("follow gone")

	// Still on, but a listing the video was back in cleared its mark:
	// it is not let go.
	write(t, r.s, func(tx *sql.Tx) error {
		if err := UpdateFollow(tx, anna, fl.ID, FollowChange{Mirror: ptr(true)}); err != nil {
			return err
		}
		return MarkMissing(tx, anna, v.ID, fl.ID, false)
	})
	if stopped, err := r.svc.letGo(ctx, r.root, v, fl.ID, true, false); err != nil || stopped {
		t.Fatalf("not missing any more: stopped %v, %v", stopped, err)
	}
	intact("no longer marked missing")

	// Marked again: a row read before its download finished has no path,
	// yet the file that is there now goes with the row.
	write(t, r.s, func(tx *sql.Tx) error { return MarkMissing(tx, anna, v.ID, fl.ID, true) })
	stale := Video{ID: v.ID, UserID: anna, VideoID: v.VideoID, State: StateDownloading}
	if stopped, err := r.svc.letGo(ctx, r.root, stale, fl.ID, true, false); err != nil || stopped {
		t.Fatalf("letting go: stopped %v, %v", stopped, err)
	}
	if vs := r.videos(anna); len(vs) != 1 || len(r.mp4s()) != 1 || strings.Contains(r.mp4s()[0], "aaaaaaaaaa2") {
		t.Fatalf("after letting go: %v files %v", vs, r.mp4s())
	}
}

// Unfollowed while yt-dlp listed the follow: the check records nothing, so
// no row is left that belongs to no follow.
func TestCheckOfAnUnfollowedFollowAddsNothing(t *testing.T) {
	t.Parallel()
	r := newRig(t, "ok")
	ctx := context.Background()
	fl := r.follow(anna, playlistFollow("PLtestlist01"))
	if err := r.svc.Unfollow(ctx, anna, fl.ID, false); err != nil {
		t.Fatal(err)
	}
	n, err := r.svc.checkFollow(ctx, r.svc.runner(), fl)
	if err != nil || n != 0 {
		t.Fatalf("check of a follow that is gone: %d, %v", n, err)
	}
	if vs := r.videos(anna); len(vs) != 0 {
		t.Fatalf("rows left with no follow: %v", vs)
	}
}

// Adding a video again while a job for it is live queues a second job
// behind the first, so a first job already past its last look does not
// leave the video waiting for the next check.
func TestReAddQueuesBehindALiveJob(t *testing.T) {
	t.Parallel()
	svc, s := addService(t, "ok")
	ctx := context.Background()
	id := addVideo(t, s, Video{UserID: 2, VideoID: "abcdefghijk"})
	svc.queueDownloadNow(ctx, 2, id)
	if js := queuedJobs(t, s, KindDownload); len(js) != 1 {
		t.Fatalf("first add: %v", js)
	}
	svc.queueDownloadNow(ctx, 2, id)
	svc.queueDownloadNow(ctx, 2, id)
	js := queuedJobs(t, s, KindDownload)
	after := js["youtube:2:"+itoa(id)+":after"]
	if len(js) != 2 || after["after"] != true || after["row_id"] != float64(id) {
		t.Fatalf("jobs %v, want one more that waits behind the first", js)
	}
	// The scheduled check never adds one.
	svc.queueDownload(ctx, 2, id)
	if js := queuedJobs(t, s, KindDownload); len(js) != 2 {
		t.Fatalf("jobs after a check %v", js)
	}
}

// The second job downloads a video the first left queued, and leaves one
// that is settled alone.
func TestSecondJobTakesWhatTheFirstLeftQueued(t *testing.T) {
	t.Parallel()
	r := newRig(t, "ok")
	ctx := context.Background()
	id := addVideo(t, r.s, Video{UserID: anna, VideoID: "abcdefghijk"})
	enqueue := func() {
		t.Helper()
		if _, _, err := r.q.Enqueue(ctx, KindDownload, nil, map[string]any{"row_id": id, "after": true}, "", database.System); err != nil {
			t.Fatal(err)
		}
		r.drain()
	}
	enqueue()
	if v := r.videos(anna)["abcdefghijk"]; v.State != StateDone {
		t.Fatalf("a video left queued: %s", v.State)
	}
	enqueue()
	js := r.jobs(KindDownload)
	if js[1].Result["skipped"] != "settled" || len(r.mp4s()) != 1 {
		t.Fatalf("second run %+v", js[1])
	}
}
