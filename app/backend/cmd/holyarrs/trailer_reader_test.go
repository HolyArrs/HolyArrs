package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/health"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata/youtube"
)

// A trailer YouTube will not play here (age) is marked bad for 7 days and
// the title's next trailer plays; the bad one is not asked for again, even
// for another language or after a restart. The picked one stays the
// title's key, so it plays again once its bad mark is over.
func TestTrailerNextOnVideoReason(t *testing.T) {
	yt := fakeReader(t, "age", "ageGated001")
	yt.set("secondKey01", "ok")
	_, st := trDiscoverRig(t, trailerTMDB(t))
	a := buildHandlers(services{Store: st, Metadata: trailerTMDB(t), Version: "test"})
	ctx := context.Background()
	s, err := a.Trailers().ResolveTitle(ctx, "movie", "13", "en")
	if err != nil || s.Key != "secondKey01" || s.Type != "hls" {
		t.Fatalf("%+v %v", s, err)
	}
	if k := a.Trailers().KnownKey(ctx, "movie", "13", "en"); k != "ageGated001" {
		t.Fatalf("kept key %q", k)
	}
	if st := a.Trailers().ReaderState(); st.Failed != 0 || st.LastGood.IsZero() {
		t.Fatalf("a video reason is not a failure: %+v", st)
	}
	if s, err := a.Trailers().ResolveTitle(ctx, "movie", "13", "de"); err != nil || s.Key != "secondKey01" {
		t.Fatalf("de %+v %v", s, err)
	}
	if n := yt.count("ageGated001"); n != 1 {
		t.Fatalf("the bad key was asked %d times", n)
	}
	b := buildHandlers(services{Store: st, Metadata: trailerTMDB(t), Version: "test"}) // a restart
	now := time.Now()
	if why, bad := b.Trailers().Bad(ctx, "ageGated001", now); !bad || why != youtube.AgeRestricted {
		t.Fatalf("not kept across a restart: %q %v", why, bad)
	}
	lc := metadata.LookupCache{Store: st}
	if _, ok, _ := lc.Get(ctx, "trailerbad|test|ageGated001", now.Add(6*24*time.Hour)); !ok {
		t.Fatal("gone before 7 days")
	}
	if _, ok, _ := lc.Get(ctx, "trailerbad|test|ageGated001", now.Add(8*24*time.Hour)); ok {
		t.Fatal("kept past 7 days")
	}
	if _, bad := b.Trailers().Bad(ctx, "secondKey01", now); bad {
		t.Fatal("the key that played is marked bad")
	}
	// Past 7 days (the bad mark and the kept address over), and YouTube
	// plays it again: the picked trailer plays, not the next one.
	yt.set("ageGated001", "ok")
	_ = lc.Put(ctx, "trailerbad|test|ageGated001", []byte(youtube.AgeRestricted), now, now)
	_ = lc.Put(ctx, "trailer|movie|13|en", []byte("{}"), now, now)
	if s, err := b.Trailers().ResolveTitle(ctx, "movie", "13", "en"); err != nil || s.Key != "ageGated001" {
		t.Fatalf("after the bad mark %+v %v", s, err)
	}
}

// A premiere not on yet is not marked bad: only the normal miss, and it
// plays once it is on. YouTube's "try again later" is its door refusing
// this server, not the video: not marked bad, no next trailer asked, the
// health check counts it, and a music video is not dropped for it.
func TestTrailerShortRefusalsNotMarkedBad(t *testing.T) {
	yt := fakeReader(t, "premiere", "ageGated001", "secondKey01", "thirdKey001")
	_, st := trDiscoverRig(t, trailerTMDB(t))
	a := buildHandlers(services{Store: st, Metadata: trailerTMDB(t), Version: "test"})
	ctx := context.Background()
	now := time.Now()
	m := (*metadata.TrailerMissed)(nil)
	if _, err := a.Trailers().ResolveTitle(ctx, "movie", "13", "en"); !errors.As(err, &m) || m.Reason != youtube.Unplayable {
		t.Fatalf("premiere %v", err)
	}
	for _, k := range []string{"ageGated001", "secondKey01", "thirdKey001"} {
		if _, bad := a.Trailers().Bad(ctx, k, now); bad {
			t.Fatalf("%s marked bad", k)
		}
	}
	yt.set("ageGated001", "ok")
	a.Trailers().Forget() // the miss is over
	if s, err := a.Trailers().ResolveTitle(ctx, "movie", "13", "en"); err != nil || s.Key != "ageGated001" {
		t.Fatalf("premiere on %+v %v", s, err)
	}

	yt = fakeReader(t, "later", "ageGated001", "secondKey01", "thirdKey001")
	if _, err := a.Trailers().ResolveTitle(ctx, "movie", "14", "en"); !errors.As(err, &m) || m.Reason != youtube.Refused || m.Key != "ageGated001" {
		t.Fatalf("try again later %v %+v", err, m)
	}
	if n := yt.runs.Load(); n != 1 {
		t.Fatalf("lookups %d", n)
	}
	if _, bad := a.Trailers().Bad(ctx, "ageGated001", now); bad {
		t.Fatal("marked bad on try again later")
	}
	if s := a.Trailers().ReaderState(); s.Failed != 1 || s.LastReason != youtube.Refused {
		t.Fatalf("health %+v", s)
	}
	if _, err := a.TrailerTest().Music().TestHooks().ResolveMusicVideo(ctx, "secondKey01"); err == nil || a.TrailerTest().Music().TestHooks().VideoWontPlay(err) {
		t.Fatalf("music video dropped on try again later: %v", err)
	}
}

// When no trailer plays, the 404 says why. Every one tried unplayable for
// a video reason and none left: key "". At most two more are tried in one
// lookup; one left untried is the key for YouTube's player.
func TestTrailerAllUnplayableSaysWhy(t *testing.T) {
	yt := fakeReader(t, "age", "ageGated001", "thirdKey001", "fourthKey01")
	yt.set("secondKey01", "unavailable")
	srv, a := trailerRig(t)
	_ = lookUp(t, a, "movie", "14")
	var b struct{ Error, Reason, Key string }
	_ = json.Unmarshal(trDo(t, "GET", srv.URL+"/api/v1/trailer/movie/14", "", 404), &b)
	if b.Error != "no direct stream" || b.Reason != string(youtube.AgeRestricted) || b.Key != "" {
		t.Fatalf("all bad %+v", b)
	}
	if n := yt.runs.Load(); n != 3 {
		t.Fatalf("lookups %d", n)
	}
	// The answer is the same while the miss is kept (2 hours: the pass does
	// not ask again before).
	if e, ok := a.Trailers().Get("movie|14|en", time.Now()); !ok || time.Until(e.Miss) < 2*time.Hour-time.Minute {
		t.Fatalf("miss kept until %v", e.Miss)
	}
	if err := lookUp(t, a, "movie", "14"); err == nil {
		t.Fatal("no miss")
	}
	b.Reason, b.Key = "", ""
	_ = json.Unmarshal(trDo(t, "GET", srv.URL+"/api/v1/trailer/movie/14", "", 404), &b)
	if b.Reason != string(youtube.AgeRestricted) || b.Key != "" || yt.runs.Load() != 3 {
		t.Fatalf("kept miss %+v", b)
	}
	// A fresh install (no key marked bad yet): all four listed trailers are
	// tried (up to five are), so none is left for the YouTube player.
	srv, a = trailerRig(t)
	_ = lookUp(t, a, "movie", "15")
	b.Reason, b.Key = "", ""
	_ = json.Unmarshal(trDo(t, "GET", srv.URL+"/api/v1/trailer/movie/15", "", 404), &b)
	if b.Reason != string(youtube.AgeRestricted) || b.Key != "" || yt.count("fourthKey01") != 1 {
		t.Fatalf("all four tried %+v (asked %d)", b, yt.count("fourthKey01"))
	}
}

// Door failures count in a row for every lookup (the trailer pass stops on
// one); the health check counts different videos instead, so one video
// failing again and again stays one. A video that is plainly about itself
// (age) counts toward neither.
func TestTrailerFailuresCountDoorReasonsOnly(t *testing.T) {
	_, st := trDiscoverRig(t, trailerTMDB(t))
	a := buildHandlers(services{Store: st, Metadata: trailerTMDB(t), Version: "test"})
	ctx := context.Background()
	fakeReader(t, "bot")
	for i := 1; i <= 2; i++ {
		a.Trailers().Forget()
		_, _ = a.Trailers().ResolveTitle(ctx, "movie", "11", "en")
		if s := a.Trailers().ReaderState(); s.Failed != i || s.Videos != 1 || s.LastReason != youtube.BotCheck {
			t.Fatalf("bot check %d: %+v", i, s)
		}
	}
	fakeReader(t, "age", "ageVideo001", "ageVideo002")
	for _, k := range []string{"ageVideo001", "ageVideo002"} {
		_, _ = a.Trailers().Read(ctx, k, k, time.Now(), false)
	}
	if s := a.Trailers().ReaderState(); s.Failed != 2 || s.Videos != 1 || s.LastReason != youtube.BotCheck {
		t.Fatalf("an age-restricted video counted: %+v", s)
	}
}

// failVideos looks n different videos up again (deadVideo00,
// deadVideo01...), as renewals: they played here before.
func failVideos(a *handlers, from, n int) {
	for i := from; i < from+n; i++ {
		k := fmt.Sprintf("deadVideo%02d", i)
		_, _ = a.Trailers().Read(metadata.Renewing(context.Background()), k, k, time.Now(), false)
	}
}

// When YouTube changes, videos that played before fail one after another:
// five different ones failing with no good lookup in between reach the
// health check's limit, even when each says only "unavailable". The same
// video failing five times does not, and a good lookup starts again.
func TestTrailerUpkeepCountsDifferentVideos(t *testing.T) {
	fakeReader(t, "unavailable")
	_, st := trDiscoverRig(t, trailerTMDB(t))
	a := buildHandlers(services{Store: st, Metadata: trailerTMDB(t), Version: "test"})
	ctx := metadata.Renewing(context.Background())
	for range health.TrailerFailLimit {
		_, _ = a.Trailers().Read(ctx, "deadVideo00", "deadVideo00", time.Now(), false)
	}
	if s := a.Trailers().ReaderState(); s.Videos != 1 || s.Failed != 0 || s.LastReason != youtube.Unavailable {
		t.Fatalf("one video five times: %+v", s)
	}
	failVideos(a, 1, health.TrailerFailLimit-1)
	if s := a.Trailers().ReaderState(); s.Videos != health.TrailerFailLimit || s.LastReason != youtube.Unavailable || !s.LastGood.IsZero() {
		t.Fatalf("five videos: %+v", s)
	}
	yt := fakeReader(t, "ok", "goodVideo01")
	if _, err := a.Trailers().Read(ctx, "goodVideo01", "goodVideo01", time.Now(), false); err != nil {
		t.Fatal(err)
	}
	if s := a.Trailers().ReaderState(); s.Videos != 0 || s.Failed != 0 || time.Since(s.LastGood) > time.Minute {
		t.Fatalf("not reset: %+v", s)
	}
	// After the reset, four more different failures stay below the limit.
	yt.set("deadVideo00", "premiere") // unplayable
	failVideos(a, 0, health.TrailerFailLimit-1)
	if s := a.Trailers().ReaderState(); s.Videos != health.TrailerFailLimit-1 {
		t.Fatalf("after the reset: %+v", s)
	}
}

// The health state is kept in lookup_cache: a restart (a new API on the
// same store) carries on counting, and the same videos are not counted
// twice. It is written on a change only: a video failing again with the
// same reason writes nothing, and good lookups write at most once a minute.
func TestTrailerUpkeepSurvivesARestart(t *testing.T) {
	fakeReader(t, "unavailable", "goodVideo01")
	_, st := trDiscoverRig(t, trailerTMDB(t))
	a := buildHandlers(services{Store: st, Metadata: trailerTMDB(t), Version: "test"})
	ctx := context.Background()
	failVideos(a, 0, 3)
	b := buildHandlers(services{Store: st, Metadata: trailerTMDB(t), Version: "test"})
	if s := b.Trailers().ReaderState(); s.Videos != 3 || s.LastReason != youtube.Unavailable {
		t.Fatalf("after a restart: %+v", s)
	}
	failVideos(b, 1, 4) // deadVideo01..04: two of them new
	if s := b.Trailers().ReaderState(); s.Videos != health.TrailerFailLimit {
		t.Fatalf("carried on: %+v", s)
	}
	lc := metadata.LookupCache{Store: st}
	gone := func() bool {
		_, ok, _ := lc.Get(ctx, "trailerhealth|reader", time.Now())
		return !ok
	}
	wipe := func() { _ = lc.Put(ctx, "trailerhealth|reader", []byte("{}"), time.Now(), time.Now()) }
	wipe()
	failVideos(b, 2, 1)
	if !gone() {
		t.Fatal("written for a video that failed again")
	}
	fakeReader(t, "ok", "goodVideo01")
	_, _ = b.Trailers().Read(ctx, "goodVideo01", "goodVideo01", time.Now(), false)
	if gone() {
		t.Fatal("the reset is not written")
	}
	wipe()
	_, _ = b.Trailers().Read(ctx, "goodVideo01", "goodVideo01", time.Now(), false)
	if !gone() {
		t.Fatal("written on every good lookup")
	}
	b.Trailers().SetHealthTimes(time.Time{}, time.Now().Add(-2*time.Minute))
	_, _ = b.Trailers().Read(ctx, "goodVideo01", "goodVideo01", time.Now(), false)
	if gone() {
		t.Fatal("the last good lookup is not written after a minute")
	}
	c := buildHandlers(services{Store: st, Metadata: trailerTMDB(t), Version: "test"})
	if s := c.Trailers().ReaderState(); s.Videos != 0 || time.Since(s.LastGood) > time.Minute {
		t.Fatalf("after the reset and a restart: %+v", s)
	}
}

// The start point is the "most replayed" peak with the opening skipped
// (the opening is the highest part: every play starts there).
func TestTrailerPeakSkipsOpening(t *testing.T) {
	fakeReader(t, "heat")
	srv, a := trailerRig(t)
	if err := lookUp(t, a, "movie", "11"); err != nil {
		t.Fatal(err)
	}
	var s metadata.TrailerStream
	_ = json.Unmarshal(trDo(t, "GET", srv.URL+"/api/v1/trailer/movie/11", "", 200), &s)
	if s.Peak != 80 || s.Duration != 200 {
		t.Fatalf("peak %v duration %v", s.Peak, s.Duration)
	}
}

// A trailer needs only its addresses: once YouTube's visitor id is kept,
// the watch page (about a second) is not read for it. The music video's
// start point needs the page.
func TestTrailerSkipsTheWatchPage(t *testing.T) {
	yt := fakeReader(t, "ok")
	ctx := context.Background()
	if _, err := metadata.ResolveTrailer(ctx, "trailerKey1", time.Now(), false); err != nil || yt.pages.Load() != 1 {
		t.Fatalf("first lookup (no visitor id yet) %v pages %d", err, yt.pages.Load())
	}
	if _, err := metadata.ResolveTrailer(ctx, "trailerKey1", time.Now(), false); err != nil || yt.pages.Load() != 1 || yt.runs.Load() != 2 {
		t.Fatalf("trailer read the page %v pages %d", err, yt.pages.Load())
	}
	if _, err := metadata.ResolveTrailer(ctx, "trailerKey1", time.Now(), true); err != nil || yt.pages.Load() != 2 {
		t.Fatalf("music video skipped the page %v pages %d", err, yt.pages.Load())
	}
}

// A premiere among the trailers keeps a shorter miss, even when a later
// trailer is age-restricted: it is asked again within the hour (not every
// pass, since each ask tries all of the title's trailers, and not after the
// 2-hour miss of a lasting reason).
func TestTrailerPremiereKeepsTheShortMiss(t *testing.T) {
	yt := fakeReader(t, "premiere", "ageGated001")
	yt.set("secondKey01", "age")
	yt.set("thirdKey001", "age")
	_, st := trDiscoverRig(t, trailerTMDB(t))
	a := buildHandlers(services{Store: st, Metadata: trailerTMDB(t), Version: "test"})
	_, _ = a.Trailers().ResolveTitle(context.Background(), "movie", "13", "en")
	e, ok := a.Trailers().Get("movie|13|en", time.Now())
	if !ok || e.Miss.IsZero() || time.Until(e.Miss) > time.Hour+time.Minute || time.Until(e.Miss) < time.Hour-time.Minute {
		t.Fatalf("miss until %v (ok %v)", e.Miss, ok)
	}
	// YouTube's door failing is still asked again on the next pass.
	yt.set("ageGated001", "later")
	a.Trailers().Forget()
	_, _ = a.Trailers().ResolveTitle(context.Background(), "movie", "13", "en")
	if e, ok := a.Trailers().Get("movie|13|en", time.Now()); !ok || time.Until(e.Miss) > metadata.TrailerMiss+time.Minute {
		t.Fatalf("door miss until %v (ok %v)", e.Miss, ok)
	}
}

// A title TMDB does not know is kept as "no trailer", so the pass does not
// stop on it every time.
func TestTrailerUnknownTitleIsNoTrailer(t *testing.T) {
	fakeReader(t, "ok")
	_, st := trDiscoverRig(t, trailerTMDB(t))
	a := buildHandlers(services{Store: st, Metadata: trailerTMDB(t), Version: "test"})
	if _, err := a.Trailers().ResolveTitle(context.Background(), "movie", "987654", "en"); !errors.Is(err, metadata.ErrNoTrailer) {
		t.Fatalf("unknown title: %v", err)
	}
}

// After a restart, titles that never played here (known misses, looked up
// again) fail one after another: removed or blocked videos found on a
// first lookup say nothing about the reader, so no warning builds up. A
// door failure still counts on a first lookup.
func TestTrailerUpkeepIgnoresFirstLookupMisses(t *testing.T) {
	fakeReader(t, "unavailable")
	_, st := trDiscoverRig(t, trailerTMDB(t))
	a := buildHandlers(services{Store: st, Metadata: trailerTMDB(t), Version: "test"})
	recentGood(a, time.Hour)
	for i := range 2 * health.TrailerFailLimit {
		k := fmt.Sprintf("deadVideo%02d", i)
		_, _ = a.Trailers().Read(context.Background(), k, k, time.Now(), false)
	}
	if s := a.Trailers().ReaderState(); s.Videos != 0 {
		t.Fatalf("first lookups counted: %+v", s)
	}
	fakeReader(t, "bot", "doorVideo01")
	_, _ = a.Trailers().Read(context.Background(), "doorVideo01", "doorVideo01", time.Now(), false)
	if s := a.Trailers().ReaderState(); s.Videos != 1 || s.LastReason != youtube.BotCheck {
		t.Fatalf("door failure not counted: %+v", s)
	}
}

// recentGood sets the reader's last good lookup to ago before now (after
// the kept state was read).
func recentGood(a *handlers, ago time.Duration) {
	a.Trailers().ReaderState()
	a.Trailers().SetHealthTimes(time.Now().Add(-ago), time.Time{})
}

// Nothing played for longer than an address lives (the machine was off
// overnight, trailers were off): no title is a renewal any more, so first
// lookups that will not play count again, and a YouTube change that shows
// only as removed videos still warns. Never a good lookup counts the same.
func TestTrailerUpkeepCountsAfterALongGap(t *testing.T) {
	for _, ago := range []time.Duration{7*time.Hour + time.Minute, 0} {
		fakeReader(t, "unavailable")
		_, st := trDiscoverRig(t, trailerTMDB(t))
		a := buildHandlers(services{Store: st, Metadata: trailerTMDB(t), Version: "test"})
		if ago > 0 {
			recentGood(a, ago)
		}
		for i := range health.TrailerFailLimit {
			k := fmt.Sprintf("deadVideo%02d", i)
			_, _ = a.Trailers().Read(context.Background(), k, k, time.Now(), false)
		}
		if s := a.Trailers().ReaderState(); s.Videos != health.TrailerFailLimit {
			t.Fatalf("last good %v ago: %+v", ago, s)
		}
	}
}

// Bad marks are kept per HolyArrs version: after an update, trailers a
// broken reader marked are tried again at once.
func TestTrailerBadMarksPerVersion(t *testing.T) {
	_, st := trDiscoverRig(t, trailerTMDB(t))
	ctx := context.Background()
	now := time.Now()
	a := buildHandlers(services{Store: st, Metadata: trailerTMDB(t), Version: "1.0.0"})
	a.Trailers().MarkBad(ctx, "deadVideo00", youtube.Unavailable, now)
	if _, bad := buildHandlers(services{Store: st, Metadata: trailerTMDB(t), Version: "1.0.0"}).Trailers().Bad(ctx, "deadVideo00", now); !bad {
		t.Fatal("a restart of the same version forgot the mark")
	}
	if _, bad := buildHandlers(services{Store: st, Metadata: trailerTMDB(t), Version: "1.0.1"}).Trailers().Bad(ctx, "deadVideo00", now); bad {
		t.Fatal("an update kept the old mark")
	}
}

// A removed video found on a lookup Health does not count (a first lookup
// while trailers play fine) is not marked bad, so if the reader itself
// broke that way the next lookups reach it and, once counted, warn. On a
// renewal it is marked; an age-restricted video is marked either way.
func TestTrailerUncountedMissNotMarked(t *testing.T) {
	yt := fakeReader(t, "unavailable", "trailerKey1")
	_, st := trDiscoverRig(t, trailerTMDB(t))
	a := buildHandlers(services{Store: st, Metadata: trailerTMDB(t), Version: "test"})
	recentGood(a, time.Hour)
	ctx := context.Background()
	now := time.Now()
	_, _ = a.Trailers().ResolveTitle(ctx, "movie", "11", "en")
	if _, bad := a.Trailers().Bad(ctx, "trailerKey1", now); bad {
		t.Fatal("an uncounted first-lookup miss marked the key bad")
	}
	a.Trailers().Forget()
	_, _ = a.Trailers().ResolveTitle(metadata.Renewing(ctx), "movie", "11", "en")
	if _, bad := a.Trailers().Bad(ctx, "trailerKey1", now); !bad {
		t.Fatal("a renewal miss did not mark the key bad")
	}
	// Age-restricted is about the video, never counted, and marked either way.
	yt.set("ageGated001", "age")
	_, _ = a.Trailers().ResolveTitle(ctx, "movie", "13", "en")
	if r, bad := a.Trailers().Bad(ctx, "ageGated001", now); !bad || r != youtube.AgeRestricted {
		t.Fatalf("age-restricted not marked: %v %v", r, bad)
	}
}
