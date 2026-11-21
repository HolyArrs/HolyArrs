package youtube

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// openStore opens an empty store with two people: 1 (owner) and 2 (member).
func openStore(t *testing.T) *database.Store {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	write(t, s, func(tx *sql.Tx) error {
		for _, q := range []string{
			`INSERT INTO users(id, created_at, created_by, updated_at, updated_by, name, role, password_hash) VALUES (1, 't', '{}', 't', '{}', 'owner', 'owner', 'x')`,
			`INSERT INTO users(id, created_at, created_by, updated_at, updated_by, name, role, password_hash) VALUES (2, 't', '{}', 't', '{}', 'member', 'member', 'x')`,
		} {
			if _, err := tx.Exec(q); err != nil {
				return err
			}
		}
		return nil
	})
	return s
}

func write(t *testing.T, s *database.Store, fn func(*sql.Tx) error) {
	t.Helper()
	if err := s.Write(context.Background(), fn); err != nil {
		t.Fatal(err)
	}
}

func addVideo(t *testing.T, s *database.Store, v Video) int64 {
	t.Helper()
	if v.State == "" {
		v.State = StateQueued
	}
	if v.AddedAt.IsZero() {
		v.AddedAt = time.Now()
	}
	var id int64
	write(t, s, func(tx *sql.Tx) error {
		var err error
		id, _, err = InsertVideo(tx, v)
		return err
	})
	return id
}

// followsOf reads the follows that hold a video.
func followsOf(t *testing.T, s *database.Store, v Video) []int64 {
	t.Helper()
	one := []Video{v}
	if err := WithFollows(context.Background(), s.Read(), v.UserID, one); err != nil {
		t.Fatal(err)
	}
	return one[0].FollowIDs
}

func addFollow(t *testing.T, s *database.Store, f Follow) int64 {
	t.Helper()
	var id int64
	write(t, s, func(tx *sql.Tx) error {
		var err error
		id, err = InsertFollow(tx, f)
		return err
	})
	return id
}

func TestStoreKeepsPeopleApart(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	idA := addVideo(t, s, Video{UserID: 1, VideoID: "abcdefghijk", Title: "A's video"})

	if _, err := VideoOf(ctx, s.Read(), 2, idA); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("VideoOf another person's row: err = %v, want sql.ErrNoRows", err)
	}
	if v, err := VideoOf(ctx, s.Read(), 1, idA); err != nil || v.Title != "A's video" {
		t.Fatalf("VideoOf own row = %+v, %v", v, err)
	}
	if vs, err := VideosOf(ctx, s.Read(), 2, VideoFilter{}); err != nil || len(vs) != 0 {
		t.Fatalf("VideosOf(B) = %v, %v; want none", vs, err)
	}
	if known, err := KnownVideos(ctx, s.Read(), 2); err != nil || len(known) != 0 {
		t.Fatalf("KnownVideos(B) = %v, %v; want none", known, err)
	}

	var createdA, createdB, again bool
	write(t, s, func(tx *sql.Tx) error {
		var err error
		if _, createdA, err = InsertVideo(tx, Video{UserID: 1, VideoID: "same_video1", State: StateQueued, AddedAt: time.Now()}); err != nil {
			return err
		}
		if _, createdB, err = InsertVideo(tx, Video{UserID: 2, VideoID: "same_video1", State: StateQueued, AddedAt: time.Now()}); err != nil {
			return err
		}
		_, again, err = InsertVideo(tx, Video{UserID: 2, VideoID: "same_video1", State: StateQueued, AddedAt: time.Now()})
		return err
	})
	if !createdA || !createdB {
		t.Fatalf("same video for two people: created = %v, %v; want both", createdA, createdB)
	}
	if again {
		t.Fatal("the same person adding the same video twice made a second row")
	}
	if known, _ := KnownVideos(ctx, s.Read(), 2); known["same_video1"].ID == 0 || len(known) != 1 {
		t.Fatalf("KnownVideos(B) = %v", known)
	}

	fid := addFollow(t, s, Follow{UserID: 1, URL: "https://www.youtube.com/playlist?list=PL1", Kind: KindPlaylist, YTID: "PL1", Enabled: true, AddedAt: time.Now()})
	if _, err := FollowOf(ctx, s.Read(), 2, fid); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("FollowOf another person's follow: err = %v", err)
	}
	if fs, err := FollowsOf(ctx, s.Read(), 2); err != nil || len(fs) != 0 {
		t.Fatalf("FollowsOf(B) = %v, %v", fs, err)
	}
	if err := s.Write(ctx, func(tx *sql.Tx) error { return DeleteFollow(tx, 2, fid) }); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("DeleteFollow of another person's follow: err = %v", err)
	}
	one := 1
	keep := &one
	if err := s.Write(ctx, func(tx *sql.Tx) error { return UpdateFollow(tx, 2, fid, FollowChange{KeepLast: &keep}) }); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("UpdateFollow of another person's follow: err = %v", err)
	}
	if err := s.Write(ctx, func(tx *sql.Tx) error { return SavePosition(tx, 2, idA, 30, true, time.Now()) }); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("SavePosition on another person's video: err = %v", err)
	}
}

func TestDuplicateFollowRefused(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	f := Follow{UserID: 1, URL: "https://www.youtube.com/@x", Kind: KindChannel, YTID: "UCx", Enabled: true, AddedAt: time.Now()}
	addFollow(t, s, f)
	err := s.Write(ctx, func(tx *sql.Tx) error { _, err := InsertFollow(tx, f); return err })
	if !errors.Is(err, ErrDuplicateFollow) {
		t.Fatalf("second follow: err = %v, want ErrDuplicateFollow", err)
	}
	f.UserID = 2
	addFollow(t, s, f)
	f.UserID, f.Kind = 1, KindPlaylist
	addFollow(t, s, f)
}

func TestFollowRoundTripAndChange(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	twenty := 20
	added := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	id := addFollow(t, s, Follow{UserID: 1, URL: "https://www.youtube.com/@x", Kind: KindChannel, YTID: "UCx", Title: "X",
		MaxHeight: 720, Backfill: &twenty, Enabled: true, AddedAt: added})
	f, err := FollowOf(ctx, s.Read(), 1, id)
	if err != nil || f.MaxHeight != 720 || f.Backfill == nil || *f.Backfill != 20 || f.KeepLast != nil || !f.Enabled || !f.AddedAt.Equal(added) || f.LastCheckedAt != nil {
		t.Fatalf("FollowOf = %+v, %v", f, err)
	}

	five, no := 5, false
	keep := &five
	write(t, s, func(tx *sql.Tx) error {
		return UpdateFollow(tx, 1, id, FollowChange{KeepLast: &keep, Enabled: &no})
	})
	f, _ = FollowOf(ctx, s.Read(), 1, id)
	if f.KeepLast == nil || *f.KeepLast != 5 || f.Enabled || f.MaxHeight != 720 {
		t.Fatalf("after change = %+v", f)
	}
	if fs, _ := EnabledFollows(ctx, s.Read()); len(fs) != 0 {
		t.Fatalf("EnabledFollows lists a turned-off follow: %v", fs)
	}
	var none *int
	write(t, s, func(tx *sql.Tx) error { return UpdateFollow(tx, 1, id, FollowChange{KeepLast: &none}) })
	f, _ = FollowOf(ctx, s.Read(), 1, id)
	if f.KeepLast != nil {
		t.Fatalf("keep_last not cleared: %v", *f.KeepLast)
	}

	at := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	write(t, s, func(tx *sql.Tx) error { return MarkChecked(tx, id, at, "boom") })
	f, _ = FollowOf(ctx, s.Read(), 1, id)
	if f.LastCheckedAt == nil || !f.LastCheckedAt.Equal(at) || f.LastError != "boom" {
		t.Fatalf("after MarkChecked = %+v", f)
	}
}

func TestVideoLifecycle(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	fid := addFollow(t, s, Follow{UserID: 1, URL: "u", Kind: KindPlaylist, YTID: "PL1", Enabled: true, AddedAt: time.Now()})
	id := addVideo(t, s, Video{UserID: 1, FollowIDs: []int64{fid}, VideoID: "vid00000001", MaxHeight: 1080})
	if q, _ := QueuedVideos(ctx, s.Read()); len(q) != 1 || q[0].ID != id {
		t.Fatalf("QueuedVideos = %v", q)
	}
	write(t, s, func(tx *sql.Tx) error { return SetVideoState(tx, id, StateDownloading, "") })
	if q, _ := QueuedVideos(ctx, s.Read()); len(q) != 0 {
		t.Fatalf("QueuedVideos after start = %v", q)
	}
	up := time.Date(2025, 5, 6, 0, 0, 0, 0, time.UTC)
	write(t, s, func(tx *sql.Tx) error {
		return FinishVideo(tx, id, "1-owner/PL/2025-05-06 T [vid00000001].mp4", 1234, Meta{Title: "T", ChannelName: "C", ChannelID: "UCc", UploadedAt: &up, DurationSeconds: 600})
	})
	v, err := VideoByID(ctx, s.Read(), id)
	if err != nil || v.State != StateDone || v.Error != "" || v.SizeBytes != 1234 || v.Title != "T" || v.ChannelID != "UCc" ||
		v.UploadedAt == nil || !v.UploadedAt.Equal(up) || v.DurationSeconds != 600 || !slices.Equal(followsOf(t, s, v), []int64{fid}) {
		t.Fatalf("after FinishVideo = %+v, %v", v, err)
	}

	write(t, s, func(tx *sql.Tx) error { return SavePosition(tx, 1, id, 100, false, time.Now()) })
	if vs, _ := VideosOf(ctx, s.Read(), 1, VideoFilter{Unwatched: true}); len(vs) != 1 {
		t.Fatalf("unwatched filter = %v", vs)
	}
	at := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	write(t, s, func(tx *sql.Tx) error { return SavePosition(tx, 1, id, 580, true, at) })
	v, _ = VideoOf(ctx, s.Read(), 1, id)
	if v.PositionSeconds != 580 || v.WatchedAt == nil || !v.WatchedAt.Equal(at) {
		t.Fatalf("after SavePosition = %+v", v)
	}
	if vs, _ := VideosOf(ctx, s.Read(), 1, VideoFilter{Unwatched: true}); len(vs) != 0 {
		t.Fatalf("unwatched filter lists a watched video: %v", vs)
	}
	if vs, _ := VideosOf(ctx, s.Read(), 1, VideoFilter{ChannelID: "UCc", FollowID: fid}); len(vs) != 1 {
		t.Fatalf("channel+follow filter = %v", vs)
	}
	if vs, _ := VideosOf(ctx, s.Read(), 1, VideoFilter{ChannelID: "other"}); len(vs) != 0 {
		t.Fatalf("channel filter = %v", vs)
	}

	write(t, s, func(tx *sql.Tx) error { return SetVideoState(tx, id, StateRemoved, "") })
	if vs, _ := VideosOf(ctx, s.Read(), 1, VideoFilter{}); len(vs) != 0 {
		t.Fatalf("VideosOf lists a removed video: %v", vs)
	}
	if vs, _ := VideosOfFollow(ctx, s.Read(), 1, fid); len(vs) != 1 {
		t.Fatalf("VideosOfFollow = %v", vs)
	}
	if vs, _ := VideosOfFollow(ctx, s.Read(), 2, fid); len(vs) != 0 {
		t.Fatalf("VideosOfFollow for another person = %v", vs)
	}
}

func TestVideosOfNewestFirstAndPaged(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i, vid := range []string{"v1", "v2", "v3"} {
		addVideo(t, s, Video{UserID: 1, VideoID: vid, AddedAt: base.Add(time.Duration(i) * time.Hour)})
	}
	addVideo(t, s, Video{UserID: 1, VideoID: "skip", State: StateSkipped, AddedAt: base.Add(10 * time.Hour)})
	vs, err := VideosOf(ctx, s.Read(), 1, VideoFilter{Limit: 2})
	if err != nil || len(vs) != 2 || vs[0].VideoID != "v3" || vs[1].VideoID != "v2" {
		t.Fatalf("first page = %v, %v", vs, err)
	}
	vs, _ = VideosOf(ctx, s.Read(), 1, VideoFilter{Limit: 2, Offset: 2})
	if len(vs) != 1 || vs[0].VideoID != "v1" {
		t.Fatalf("second page = %v", vs)
	}
}

func TestKeepLastExcessByUploadDate(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	fid := addFollow(t, s, Follow{UserID: 1, URL: "u", Kind: KindChannel, YTID: "UC1", Enabled: true, AddedAt: time.Now()})
	day := func(d int) *time.Time { t := time.Date(2026, 1, d, 0, 0, 0, 0, time.UTC); return &t }
	ids := map[int]int64{}
	// Added in a shuffled order so added_at and id do not match upload order.
	for _, d := range []int{3, 1, 5, 2, 4} {
		ids[d] = addVideo(t, s, Video{UserID: 1, FollowIDs: []int64{fid}, VideoID: string(rune('a'+d)) + "_done", State: StateDone, UploadedAt: day(d)})
	}
	addVideo(t, s, Video{UserID: 1, FollowIDs: []int64{fid}, VideoID: "old_queued", State: StateQueued, UploadedAt: day(1)})
	got, err := KeepLastExcess(ctx, s.Read(), 1, fid, 2)
	if err != nil || len(got) != 3 {
		t.Fatalf("KeepLastExcess = %v, %v", got, err)
	}
	for i, d := range []int{3, 2, 1} {
		if got[i].ID != ids[d] {
			t.Fatalf("KeepLastExcess[%d] = %d (%v), want day %d row %d", i, got[i].ID, got[i].UploadedAt, d, ids[d])
		}
	}
	if got, err := KeepLastExcess(ctx, s.Read(), 2, fid, 2); err != nil || len(got) != 0 {
		t.Fatalf("KeepLastExcess for another person's follow = %v, %v; want none", got, err)
	}
	if got, _ := KeepLastExcess(ctx, s.Read(), 1, fid, 5); len(got) != 0 {
		t.Fatalf("KeepLastExcess with room = %v", got)
	}
}

func TestDeletePersonRows(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	for _, u := range []int64{1, 2} {
		fid := addFollow(t, s, Follow{UserID: u, URL: "u", Kind: KindPlaylist, YTID: "PL", Enabled: true, AddedAt: time.Now()})
		addVideo(t, s, Video{UserID: u, FollowIDs: []int64{fid}, VideoID: "followed", State: StateDone})
		addVideo(t, s, Video{UserID: u, VideoID: "one_off", State: StateDone})
	}
	write(t, s, func(tx *sql.Tx) error { return DeletePersonRows(tx, 1) })
	if fs, _ := FollowsOf(ctx, s.Read(), 1); len(fs) != 0 {
		t.Fatalf("A's follows left: %v", fs)
	}
	if known, _ := KnownVideos(ctx, s.Read(), 1); len(known) != 0 {
		t.Fatalf("A's videos left: %v", known)
	}
	if fs, _ := FollowsOf(ctx, s.Read(), 2); len(fs) != 1 {
		t.Fatalf("B's follows = %v", fs)
	}
	if known, _ := KnownVideos(ctx, s.Read(), 2); len(known) != 2 {
		t.Fatalf("B's videos = %v", known)
	}
}

func TestDeleteFollowKeepsOrDropsVideos(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	keep := addFollow(t, s, Follow{UserID: 1, URL: "u", Kind: KindPlaylist, YTID: "PLkeep", Enabled: true, AddedAt: time.Now()})
	drop := addFollow(t, s, Follow{UserID: 1, URL: "u", Kind: KindPlaylist, YTID: "PLdrop", Enabled: true, AddedAt: time.Now()})
	kept := addVideo(t, s, Video{UserID: 1, FollowIDs: []int64{keep}, VideoID: "kept", State: StateDone})
	addVideo(t, s, Video{UserID: 1, FollowIDs: []int64{drop}, VideoID: "dropped", State: StateDone})

	write(t, s, func(tx *sql.Tx) error { return DeleteFollow(tx, 1, keep) })
	v, err := VideoOf(ctx, s.Read(), 1, kept)
	if err != nil || len(followsOf(t, s, v)) != 0 {
		t.Fatalf("video of an unfollowed follow = %+v, %v; want kept with no follow", v, err)
	}

	if err := s.Write(ctx, func(tx *sql.Tx) error { return DeleteOnlyOfFollow(tx, 2, drop) }); err != nil {
		t.Fatal(err)
	}
	if vs, _ := VideosOfFollow(ctx, s.Read(), 1, drop); len(vs) != 1 {
		t.Fatalf("another person deleted A's videos: %v", vs)
	}
	write(t, s, func(tx *sql.Tx) error {
		if err := DeleteOnlyOfFollow(tx, 1, drop); err != nil {
			return err
		}
		return DeleteFollow(tx, 1, drop)
	})
	if known, _ := KnownVideos(ctx, s.Read(), 1); len(known) != 1 || known["kept"].ID == 0 {
		t.Fatalf("videos after delete = %v", known)
	}
}

func TestTotalsCountDoneOnly(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	for i, size := range []int64{10, 20} {
		id := addVideo(t, s, Video{UserID: 1, VideoID: string(rune('a' + i))})
		write(t, s, func(tx *sql.Tx) error { return FinishVideo(tx, id, "x.mp4", size, Meta{}) })
	}
	failed := addVideo(t, s, Video{UserID: 2, VideoID: "f"})
	write(t, s, func(tx *sql.Tx) error { return SetVideoState(tx, failed, StateFailed, "nope") })
	got, err := ReadTotals(ctx, s.Read())
	if err != nil || got != (Totals{Videos: 2, Bytes: 30, People: 1}) {
		t.Fatalf("ReadTotals = %+v, %v", got, err)
	}
}
