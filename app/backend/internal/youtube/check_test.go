package youtube

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/youtube/youtubetest"
)

// liveRec keeps every PublishOwn call. onEvent, when set, runs inside the
// call, before it returns.
type liveRec struct {
	mu      sync.Mutex
	events  []liveEvent
	onEvent func(liveEvent)
}

type liveEvent struct {
	Topic string
	Video LiveVideo
	User  int64
}

func (l *liveRec) PublishOwn(topic string, data any, userID int64) uint64 {
	ev := liveEvent{Topic: topic, Video: data.(LiveVideo), User: userID}
	l.mu.Lock()
	l.events = append(l.events, ev)
	hook := l.onEvent
	l.mu.Unlock()
	if hook != nil {
		hook(ev)
	}
	return 0
}

func (l *liveRec) all() []liveEvent {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]liveEvent(nil), l.events...)
}

// rig runs the check and download jobs over a fake yt-dlp whose canned JSON
// lives in data, so a test can change what YouTube lists between checks.
type rig struct {
	t    *testing.T
	s    *database.Store
	svc  *Service
	q    *jobs.Queue
	fake youtubetest.Fake
	data string // the fake's JSON folder
	root string // the YouTube folder
	live *liveRec
	mu   sync.Mutex
	now  time.Time
}

// The rig's people besides 1 (owner) and 2 (member without youtube).
const (
	anna    = 7 // member with youtube
	removed = 8 // had youtube, removed
	kid     = 9 // kid ticked for youtube
)

func newRig(t *testing.T, mode string) *rig {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake yt-dlp is a shell script")
	}
	s := openStore(t)
	write(t, s, func(tx *sql.Tx) error {
		for _, p := range []struct {
			id                   int64
			name, perms, rmd, mr string
		}{{anna, "Anna", `["youtube"]`, "", ""}, {removed, "Gone", `["youtube"]`, "2026-01-01", ""}, {kid, "Kid", `["youtube"]`, "", "PG"}} {
			if _, err := tx.Exec(`INSERT INTO users(id, created_at, created_by, updated_at, updated_by, name, role, password_hash, perms, removed_at, max_rating)
				VALUES (?, 't', '{}', 't', '{}', ?, 'member', 'x', ?, ?, ?)`, p.id, p.name, p.perms, p.rmd, p.mr); err != nil {
				return err
			}
		}
		return nil
	})
	r := &rig{t: t, s: s, fake: youtubetest.NewFake(t, mode), data: t.TempDir(), root: t.TempDir(), live: &liveRec{},
		now: time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)}
	_, here, _, _ := runtime.Caller(0)
	src := filepath.Join(filepath.Dir(here), "testdata")
	files, _ := filepath.Glob(filepath.Join(src, "*.json"))
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		r.put(filepath.Base(f), b)
	}
	// The wrapper points the fake at this rig's JSON, answers a video lookup
	// from info-<id>.json when there is one, fails anything about a video
	// with a gone-<id> file, fails -U when update-fails exists and every
	// listing when list-fails exists; while list-warns exists a listing
	// ends well but with a warning, as yt-dlp does when a page of it
	// failed. While hold-info exists a video lookup writes info-started
	// and waits.
	wrap := filepath.Join(r.fake.Dir, "yt-dlp-wrap")
	script := fmt.Sprintf(`#!/bin/sh
DATA=%[1]q
last=""; json=0; upd=0
for a in "$@"; do last="$a"; [ "$a" = -J ] && json=1; [ "$a" = -U ] && upd=1; done
if [ $upd = 1 ] && [ -e "$DATA/update-fails" ]; then
	printf '%%s\n' -U '=end=' >> %[3]q
	echo "ERROR: Unable to write to the yt-dlp program" >&2; exit 1
fi
case "$last" in
*/videos|*/streams|*/shorts|*"playlist?list="*)
	if [ -e "$DATA/list-fails" ]; then
		printf '%%s\n' "$@" '=end=' >> %[3]q
		echo "ERROR: [youtube:tab] listing broke" >&2; exit 1
	fi
	if [ -e "$DATA/list-warns" ]; then
		echo "WARNING: [youtube:tab] Incomplete data received. Giving up after 3 retries" >&2
	fi ;;
*"watch?v="*)
	id="${last##*watch?v=}"
	if [ -e "$DATA/gone-$id" ]; then
		printf '%%s\n' "$@" '=end=' >> %[3]q
		echo "ERROR: [youtube] $id: Video unavailable" >&2; exit 1
	fi
	if [ $json = 1 ] && [ -e "$DATA/hold-info" ]; then
		: > "$DATA/info-started"
		while [ -e "$DATA/hold-info" ]; do sleep 0.01; done
	fi
	if [ $json = 1 ] && [ -e "$DATA/info-$id.json" ]; then
		printf '%%s\n' "$@" '=end=' >> %[3]q
		cat "$DATA/info-$id.json"; exit 0
	fi ;;
esac
FAKE_YTDLP_DATA="$DATA" exec %[2]q "$@"
`, r.data, r.fake.Bin, r.fake.ArgsLog)
	if err := os.WriteFile(wrap, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	r.svc = &Service{Store: s, Live: r.live,
		Tools: func() Tools { return Tools{YTDLP: wrap, FFmpeg: r.fake.Bin} },
		Person: func(ctx context.Context, id int64) (accounts.PersonAccess, error) {
			return accounts.ReadPersonAccess(ctx, s.Read(), id)
		},
		CheckFolder: func(context.Context, string) error { return nil },
		FreeSpace:   func(string) int64 { return 50 << 30 },
		Now: func() time.Time {
			r.mu.Lock()
			defer r.mu.Unlock()
			return r.now
		},
	}
	r.q = jobs.New(s, jobs.Options{Workers: 1, BaseBackoff: time.Millisecond, MaxBackoff: time.Millisecond, PollInterval: 5 * time.Millisecond})
	r.q.Handle(KindCheck, r.svc.HandleCheck)
	r.q.Handle(KindDownload, r.svc.HandleDownload)
	r.svc.Queue = r.q
	write(t, s, func(tx *sql.Tx) error {
		return database.PutSetting(tx, settingsGroup, keyFolder, r.root, database.System, time.Now())
	})
	return r
}

func (r *rig) put(name string, b []byte) {
	r.t.Helper()
	if err := os.WriteFile(filepath.Join(r.data, name), b, 0o644); err != nil {
		r.t.Fatal(err)
	}
}

func (r *rig) putJSON(name string, v any) {
	r.t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		r.t.Fatal(err)
	}
	r.put(name, b)
}

func (r *rig) setNow(t time.Time) {
	r.mu.Lock()
	r.now = t
	r.mu.Unlock()
}

// check queues a check (of one follow when id > 0) and runs every job until
// none is left.
func (r *rig) check(followID int64) {
	r.t.Helper()
	params := map[string]any{}
	if followID > 0 {
		params["follow_id"] = followID
	}
	if _, _, err := r.q.Enqueue(context.Background(), KindCheck, nil, params, "", database.System); err != nil {
		r.t.Fatal(err)
	}
	r.drain()
}

// drain runs the queue until no job is queued or running.
func (r *rig) drain() {
	r.t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := r.q.Start(ctx); err != nil {
		r.t.Fatal(err)
	}
	defer r.q.Stop()
	deadline := time.Now().Add(30 * time.Second)
	for {
		var n int
		if err := r.s.Read().QueryRowContext(ctx, "SELECT count(*) FROM jobs WHERE state IN ('queued', 'running')").Scan(&n); err != nil {
			r.t.Fatal(err)
		}
		if n == 0 {
			return
		}
		if time.Now().After(deadline) {
			r.t.Fatalf("%d jobs still waiting", n)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (r *rig) jobs(kind string) []jobs.Job {
	r.t.Helper()
	list, err := database.Select[jobs.Job](context.Background(), r.s.Read(), "jobs", "kind = ? ORDER BY id", kind)
	if err != nil {
		r.t.Fatal(err)
	}
	return list
}

// videos reads a person's rows by YouTube id, every state included.
func (r *rig) videos(userID int64) map[string]Video {
	r.t.Helper()
	list, err := database.Select[Video](context.Background(), r.s.Read(), "youtube_videos", "user_id = ?", userID)
	if err != nil {
		r.t.Fatal(err)
	}
	if err := WithFollows(context.Background(), r.s.Read(), userID, list); err != nil {
		r.t.Fatal(err)
	}
	out := map[string]Video{}
	for _, v := range list {
		out[v.VideoID] = v
	}
	return out
}

func (r *rig) follow(userID int64, f Follow) Follow {
	r.t.Helper()
	f.UserID = userID
	if f.AddedAt.IsZero() {
		f.AddedAt = r.now
	}
	f.Enabled = true
	id := addFollow(r.t, r.s, f)
	got, err := FollowOf(context.Background(), r.s.Read(), userID, id)
	if err != nil {
		r.t.Fatal(err)
	}
	return got
}

// tab is a flat channel or playlist listing as yt-dlp prints it.
type tab struct {
	Type      string     `json:"_type"`
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	Channel   string     `json:"channel"`
	ChannelID string     `json:"channel_id"`
	Count     *int       `json:"playlist_count,omitempty"`
	Entries   []tabEntry `json:"entries"`
}

type tabEntry struct {
	ID         string  `json:"id"`
	URL        string  `json:"url"`
	Title      string  `json:"title"`
	Timestamp  *int64  `json:"timestamp"`
	LiveStatus *string `json:"live_status"`
}

func entry(id, title string, ts int64, live string) tabEntry {
	e := tabEntry{ID: id, URL: "https://www.youtube.com/watch?v=" + id, Title: title}
	if ts != 0 {
		e.Timestamp = &ts
	}
	if live != "" {
		e.LiveStatus = &live
	}
	return e
}

func channelTab(es ...tabEntry) tab {
	return tab{Type: "playlist", ID: veritasiumUC, Title: "Veritasium - Videos", Channel: "Veritasium", ChannelID: veritasiumUC, Entries: es}
}

func vid(prefix byte, n int) string { return fmt.Sprintf("%c%010d", prefix, n) }

func veritasium() Follow {
	return Follow{Kind: KindChannel, YTID: veritasiumUC, URL: "https://www.youtube.com/channel/" + veritasiumUC, Title: "Veritasium", Backfill: intp(20)}
}

// countStates counts a person's rows by state.
func countStates(vs map[string]Video) map[string]int {
	out := map[string]int{}
	for _, v := range vs {
		out[v.State]++
	}
	return out
}

func TestFirstChannelCheckTakesBackfillOnly(t *testing.T) {
	t.Parallel()
	r := newRig(t, "ok")
	r.svc.FreeSpace = func(string) int64 { return 1 << 30 } // downloads wait: the check alone is under test
	base := r.now.Add(-400 * 24 * time.Hour).Unix()
	var videos, streams, shorts []tabEntry
	for i := range 30 {
		videos = append(videos, entry(vid('v', i), "Video", base+int64(i)*1000, ""))
	}
	streams = append(streams, entry(vid('s', 0), "Stream", base+29500, "was_live"), entry(vid('s', 1), "Stream", base+28500, "was_live"),
		entry(vid('s', 2), "Upcoming", r.now.Add(48*time.Hour).Unix(), "is_upcoming"))
	for i := range 5 {
		shorts = append(shorts, entry(vid('h', i), "Short", base+40000, ""))
	}
	r.putJSON("channel_videos.json", channelTab(videos...))
	r.putJSON("channel_streams.json", channelTab(streams...))
	r.putJSON("channel_shorts.json", channelTab(shorts...))
	f := r.follow(anna, veritasium())

	r.check(f.ID)
	got := r.videos(anna)
	if c := countStates(got); c[StateQueued] != 20 || c[StateSkipped] != 12 || len(got) != 32 {
		t.Fatalf("states %v of %d rows", c, len(got))
	}
	// The newest 20 across Videos and Live: s0, v29, s1, v28 ... v12.
	for _, id := range []string{vid('v', 29), vid('s', 0), vid('s', 1), vid('v', 12)} {
		if got[id].State != StateQueued {
			t.Errorf("%s: %s, want queued", id, got[id].State)
		}
	}
	if got[vid('v', 11)].State != StateSkipped {
		t.Errorf("v11: %s, want skipped", got[vid('v', 11)].State)
	}
	if _, ok := got[vid('s', 2)]; ok {
		t.Error("an upcoming stream was recorded")
	}
	for id := range got {
		if id[0] == 'h' {
			t.Errorf("short %s recorded with shorts off", id)
		}
	}
	v := got[vid('v', 29)]
	if !slices.Equal(v.FollowIDs, []int64{f.ID}) || v.Manual || v.ChannelName != "Veritasium" || v.ChannelID != veritasiumUC ||
		v.UploadedAt == nil || v.UploadedAt.Unix() != base+29000 || v.MaxHeight != 1080 {
		t.Fatalf("row %+v", v)
	}
	ff, _ := FollowOf(context.Background(), r.s.Read(), anna, f.ID)
	if ff.LastCheckedAt == nil || !ff.LastCheckedAt.Equal(r.now) || ff.LastError != "" {
		t.Fatalf("follow after check %+v", ff)
	}
	if n := len(r.jobs(KindDownload)); n != 20 {
		t.Fatalf("%d download jobs, want 20", n)
	}
}

func TestLaterChannelCheckSkipsBackCatalogue(t *testing.T) {
	t.Parallel()
	r := newRig(t, "ok")
	r.svc.FreeSpace = func(string) int64 { return 1 << 30 }
	t0 := r.now
	r.putJSON("channel_videos.json", channelTab(entry(vid('v', 1), "Old", t0.Add(-30*24*time.Hour).Unix(), "")))
	r.putJSON("channel_streams.json", channelTab())
	f := r.follow(anna, veritasium())
	r.check(f.ID)

	// A day later: one new video, one old video seen for the first time.
	t1 := t0.Add(24 * time.Hour)
	r.setNow(t1)
	r.putJSON("channel_videos.json", channelTab(
		entry(vid('v', 2), "New", t1.Unix(), ""),
		entry(vid('v', 3), "Late find", t0.Add(-3*24*time.Hour).Unix(), ""),
		entry(vid('v', 4), "Back catalogue", t0.Add(-60*24*time.Hour).Unix(), ""),
		entry(vid('v', 5), "No date", 0, ""),
		entry(vid('v', 1), "Old", t0.Add(-30*24*time.Hour).Unix(), "")))
	r.check(f.ID)
	got := r.videos(anna)
	for id, want := range map[string]string{vid('v', 1): StateQueued, vid('v', 2): StateQueued, vid('v', 3): StateQueued, vid('v', 4): StateSkipped, vid('v', 5): StateQueued} {
		if got[id].State != want {
			t.Errorf("%s: %q, want %s", id, got[id].State, want)
		}
	}

	// Shorts turned on: those dated more than 7 days before the previous
	// check are the back catalogue.
	shorts := true
	write(t, r.s, func(tx *sql.Tx) error { return UpdateFollow(tx, anna, f.ID, FollowChange{IncludeShorts: &shorts}) })
	r.setNow(t1.Add(24 * time.Hour))
	r.putJSON("channel_shorts.json", channelTab(
		entry(vid('h', 1), "Old short", t1.Add(-8*24*time.Hour).Unix(), ""),
		entry(vid('h', 2), "Recent short", t1.Add(-6*24*time.Hour).Unix(), "")))
	r.check(f.ID)
	got = r.videos(anna)
	if got[vid('h', 1)].State != StateSkipped || got[vid('h', 2)].State != StateQueued {
		t.Fatalf("shorts: old %q, recent %q", got[vid('h', 1)].State, got[vid('h', 2)].State)
	}
}

func TestPlaylistCheckQueuesEveryUnknownEntry(t *testing.T) {
	t.Parallel()
	r := newRig(t, "ok")
	r.svc.FreeSpace = func(string) int64 { return 1 << 30 }
	f := r.follow(anna, Follow{Kind: KindPlaylist, YTID: "PLtestlist01", URL: "https://www.youtube.com/playlist?list=PLtestlist01", Title: "Test playlist"})
	r.check(f.ID)
	if c := countStates(r.videos(anna)); c[StateQueued] != 3 || len(c) != 1 {
		t.Fatalf("first check %v", c)
	}
	r.setNow(r.now.Add(48 * time.Hour))
	pl := tab{Type: "playlist", ID: "PLtestlist01", Title: "Test playlist", Entries: []tabEntry{
		entry("aaaaaaaaaa1", "First", 0, ""), entry("aaaaaaaaaa2", "Second", 0, ""), entry("aaaaaaaaaa3", "Third", 0, ""),
		entry("ooooooooooo", "Ten years old", r.now.Add(-3650*24*time.Hour).Unix(), ""),
	}}
	r.putJSON("playlist.json", pl)
	r.check(f.ID)
	if v := r.videos(anna)["ooooooooooo"]; v.State != StateQueued {
		t.Fatalf("old entry newly in the playlist: %q, want queued", v.State)
	}
}

func TestCheckSkipsPeopleWithoutYouTube(t *testing.T) {
	t.Parallel()
	r := newRig(t, "ok")
	r.svc.FreeSpace = func(string) int64 { return 1 << 30 }
	var others []Follow
	for _, uid := range []int64{2, removed, kid} {
		others = append(others, r.follow(uid, veritasium()))
		addVideo(t, r.s, Video{UserID: uid, VideoID: "qqqqqqqqqq" + itoa(uid), State: StateQueued})
	}
	mine := r.follow(anna, veritasium())
	r.check(0)
	for _, f := range others {
		got, _ := FollowOf(context.Background(), r.s.Read(), f.UserID, f.ID)
		if got.LastCheckedAt != nil {
			t.Errorf("person %d's follow was listed", f.UserID)
		}
		if vs := r.videos(f.UserID); len(vs) != 1 {
			t.Errorf("person %d has %d rows, want only the queued one", f.UserID, len(vs))
		}
	}
	for _, j := range r.jobs(KindDownload) {
		if !strings.HasPrefix(*j.DedupeKey, fmt.Sprintf("youtube:%d:", anna)) {
			t.Errorf("download queued for someone without youtube: %s", *j.DedupeKey)
		}
	}
	if got, _ := FollowOf(context.Background(), r.s.Read(), anna, mine.ID); got.LastCheckedAt == nil {
		t.Fatal("Anna's follow was not listed")
	}
	// Taking youtube away from Anna stops her checks too.
	write(t, r.s, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE users SET perms = '[]' WHERE id = ?", anna)
		return err
	})
	r.setNow(r.now.Add(time.Hour))
	r.check(mine.ID)
	if got, _ := FollowOf(context.Background(), r.s.Read(), anna, mine.ID); !got.LastCheckedAt.Equal(r.now.Add(-time.Hour)) {
		t.Fatalf("listed after youtube was taken away: %v", got.LastCheckedAt)
	}
}

func TestCheckSelfUpdatesOncePerDay(t *testing.T) {
	t.Parallel()
	r := newRig(t, "ok")
	updates := func() int {
		n := 0
		for _, c := range r.fake.Calls(t) {
			for _, a := range c {
				if a == "-U" {
					n++
				}
			}
		}
		return n
	}
	r.check(0)
	r.setNow(r.now.Add(23 * time.Hour))
	r.check(0)
	if n := updates(); n != 1 {
		t.Fatalf("%d updates in one day, want 1", n)
	}
	set := LoadSettings(context.Background(), r.s.Read())
	if set.UpdatedAt != database.FormatTime(r.now.Add(-23*time.Hour)) || set.UpdateError != "" {
		t.Fatalf("after update %+v", set)
	}
	r.put("update-fails", nil)
	r.setNow(r.now.Add(2 * time.Hour))
	r.check(0)
	set = LoadSettings(context.Background(), r.s.Read())
	if n := updates(); n != 2 || set.UpdateError != "ERROR: Unable to write to the yt-dlp program" || set.UpdatedAt != database.FormatTime(r.now) {
		t.Fatalf("%d updates, settings %+v", n, set)
	}
	// A failed update does not stop the check.
	if j := r.jobs(KindCheck); j[len(j)-1].State != "done" {
		t.Fatalf("check after a failed update: %s", j[len(j)-1].State)
	}
}

func TestCheckRequeuesQueuedRows(t *testing.T) {
	t.Parallel()
	r := newRig(t, "ok")
	id := addVideo(t, r.s, Video{UserID: anna, VideoID: "abcdefghijk", State: StateQueued})
	r.check(0)
	js := r.jobs(KindDownload)
	if len(js) != 1 || *js[0].DedupeKey != fmt.Sprintf("youtube:%d:%d", anna, id) || js[0].State != "done" {
		t.Fatalf("download jobs %+v", js)
	}
	if v := r.videos(anna)["abcdefghijk"]; v.State != StateDone {
		t.Fatalf("row %q, want done", v.State)
	}
}

func TestPlanBackfillAndLateGrace(t *testing.T) {
	t.Parallel()
	es := []Entry{{ID: "a", Timestamp: 10}, {ID: "b", Timestamp: 30}, {ID: "c"}, {ID: "d", Timestamp: 20}}
	got := plan(Follow{Kind: KindPlaylist, Backfill: intp(2)}, es)
	if order := es[0].ID + es[1].ID + es[2].ID + es[3].ID; order != "bdac" || strings.Join(got, ",") != "queued,queued,skipped,skipped" {
		t.Fatalf("first check: %s %v", order, got)
	}
	if got := plan(Follow{Kind: KindChannel, Backfill: intp(0)}, es); got[0] != StateSkipped {
		t.Fatalf("backfill 0: %v", got)
	}
	if got := plan(Follow{Kind: KindChannel}, es); strings.Count(strings.Join(got, ","), StateQueued) != 4 {
		t.Fatalf("backfill nil takes all: %v", got)
	}
}

func TestFailedFirstCheckStillTakesBackfill(t *testing.T) {
	t.Parallel()
	r := newRig(t, "ok")
	r.svc.FreeSpace = func(string) int64 { return 1 << 30 }
	base := r.now.Add(-400 * 24 * time.Hour).Unix()
	var videos []tabEntry
	for i := range 30 {
		videos = append(videos, entry(vid('v', i), "Video", base+int64(i)*1000, ""))
	}
	r.putJSON("channel_videos.json", channelTab(videos...))
	r.putJSON("channel_streams.json", channelTab())
	f := r.follow(anna, veritasium())

	r.put("list-fails", nil)
	r.check(f.ID)
	got, _ := FollowOf(context.Background(), r.s.Read(), anna, f.ID)
	if got.LastCheckedAt != nil || got.LastError != "ERROR: [youtube:tab] listing broke" || len(r.videos(anna)) != 0 {
		t.Fatalf("after a failed first listing: %+v, %d rows", got, len(r.videos(anna)))
	}
	if err := os.Remove(filepath.Join(r.data, "list-fails")); err != nil {
		t.Fatal(err)
	}
	r.setNow(r.now.Add(6 * time.Hour))
	r.check(f.ID)
	if c := countStates(r.videos(anna)); c[StateQueued] != 20 || c[StateSkipped] != 10 {
		t.Fatalf("first good listing after a failed one: %v", c)
	}
	if got, _ := FollowOf(context.Background(), r.s.Read(), anna, f.ID); got.LastError != "" || got.LastCheckedAt == nil {
		t.Fatalf("follow after a good listing %+v", got)
	}
}

func TestLongOutageKeepsUploadsFromIt(t *testing.T) {
	t.Parallel()
	r := newRig(t, "ok")
	r.svc.FreeSpace = func(string) int64 { return 1 << 30 }
	t0 := r.now
	day := 24 * time.Hour
	r.putJSON("channel_videos.json", channelTab(entry(vid('v', 1), "Before", t0.Add(-day).Unix(), "")))
	r.putJSON("channel_streams.json", channelTab())
	f := r.follow(anna, veritasium())
	r.check(f.ID)

	// yt-dlp is broken for ten days; checks keep failing.
	r.put("list-fails", nil)
	for _, d := range []time.Duration{3, 6, 10} {
		r.setNow(t0.Add(d * day))
		r.check(f.ID)
	}
	if got, _ := FollowOf(context.Background(), r.s.Read(), anna, f.ID); !got.LastCheckedAt.Equal(t0) || got.LastError == "" {
		t.Fatalf("during the outage %+v", got)
	}
	if err := os.Remove(filepath.Join(r.data, "list-fails")); err != nil {
		t.Fatal(err)
	}
	r.setNow(t0.Add(12 * day))
	r.putJSON("channel_videos.json", channelTab(
		entry(vid('v', 2), "Early in the outage", t0.Add(day).Unix(), ""),
		entry(vid('v', 3), "Late in the outage", t0.Add(9*day).Unix(), ""),
		entry(vid('v', 4), "Back catalogue", t0.Add(-30*day).Unix(), ""),
		entry(vid('v', 1), "Before", t0.Add(-day).Unix(), "")))
	r.check(f.ID)
	got := r.videos(anna)
	for id, want := range map[string]string{vid('v', 2): StateQueued, vid('v', 3): StateQueued, vid('v', 4): StateSkipped} {
		if got[id].State != want {
			t.Errorf("%s: %q, want %s", id, got[id].State, want)
		}
	}
}
