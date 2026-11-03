package api

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs/internal/store"
	"github.com/CoderLuii/holyarrs/internal/youtube"
)

// A trailer YouTube will not play here (age) is marked bad for 7 days and
// the title's next trailer plays; the bad one is not asked for again, even
// for another language or after a restart. The picked one stays the
// title's key, so it plays again once its bad mark is over.
func TestTrailerNextOnVideoReason(t *testing.T) {
	yt := fakeReader(t, "age", "ageGated001")
	yt.set("secondKey01", "ok")
	_, st := discoverRig(t, trailerTMDB(t))
	a := New(Server{Store: st, Metadata: trailerTMDB(t), Version: "test"})
	ctx := context.Background()
	s, err := a.resolveTitle(ctx, "movie", "13", "en")
	if err != nil || s.Key != "secondKey01" || s.Type != "hls" {
		t.Fatalf("%+v %v", s, err)
	}
	if k := a.knownTrailerKey(ctx, "movie", "13", "en"); k != "ageGated001" {
		t.Fatalf("kept key %q", k)
	}
	if st := a.TrailerReader(); st.Failed != 0 || st.LastGood.IsZero() {
		t.Fatalf("a video reason is not a failure: %+v", st)
	}
	if s, err := a.resolveTitle(ctx, "movie", "13", "de"); err != nil || s.Key != "secondKey01" {
		t.Fatalf("de %+v %v", s, err)
	}
	if n := yt.count("ageGated001"); n != 1 {
		t.Fatalf("the bad key was asked %d times", n)
	}
	b := New(Server{Store: st, Metadata: trailerTMDB(t), Version: "test"}) // a restart
	now := time.Now()
	if why, bad := b.trailerBad(ctx, "ageGated001", now); !bad || why != youtube.AgeRestricted {
		t.Fatalf("not kept across a restart: %q %v", why, bad)
	}
	lc := store.LookupCache{Store: st}
	if _, ok, _ := lc.Get(ctx, "trailerbad|ageGated001", now.Add(6*24*time.Hour)); !ok {
		t.Fatal("gone before 7 days")
	}
	if _, ok, _ := lc.Get(ctx, "trailerbad|ageGated001", now.Add(8*24*time.Hour)); ok {
		t.Fatal("kept past 7 days")
	}
	if _, bad := b.trailerBad(ctx, "secondKey01", now); bad {
		t.Fatal("the key that played is marked bad")
	}
	// Past 7 days (the bad mark and the kept address over), and YouTube
	// plays it again: the picked trailer plays, not the next one.
	yt.set("ageGated001", "ok")
	_ = lc.Put(ctx, "trailerbad|ageGated001", []byte(youtube.AgeRestricted), now, now)
	_ = lc.Put(ctx, "trailer|movie|13|en", []byte("{}"), now, now)
	if s, err := b.resolveTitle(ctx, "movie", "13", "en"); err != nil || s.Key != "ageGated001" {
		t.Fatalf("after the bad mark %+v %v", s, err)
	}
}

// A premiere not on yet is not marked bad: only the normal miss, and it
// plays once it is on. YouTube's "try again later" is its door refusing
// this server, not the video: not marked bad, no next trailer asked, the
// health check counts it, and a music video is not dropped for it.
func TestTrailerShortRefusalsNotMarkedBad(t *testing.T) {
	yt := fakeReader(t, "premiere", "ageGated001", "secondKey01", "thirdKey001")
	_, st := discoverRig(t, trailerTMDB(t))
	a := New(Server{Store: st, Metadata: trailerTMDB(t), Version: "test"})
	ctx := context.Background()
	now := time.Now()
	m := (*trailerMissed)(nil)
	if _, err := a.resolveTitle(ctx, "movie", "13", "en"); !errors.As(err, &m) || m.reason != youtube.Unplayable {
		t.Fatalf("premiere %v", err)
	}
	for _, k := range []string{"ageGated001", "secondKey01", "thirdKey001"} {
		if _, bad := a.trailerBad(ctx, k, now); bad {
			t.Fatalf("%s marked bad", k)
		}
	}
	yt.set("ageGated001", "ok")
	a.trailers.cache = nil // the miss is over
	if s, err := a.resolveTitle(ctx, "movie", "13", "en"); err != nil || s.Key != "ageGated001" {
		t.Fatalf("premiere on %+v %v", s, err)
	}

	yt = fakeReader(t, "later", "ageGated001", "secondKey01", "thirdKey001")
	if _, err := a.resolveTitle(ctx, "movie", "14", "en"); !errors.As(err, &m) || m.reason != youtube.Refused || m.key != "ageGated001" {
		t.Fatalf("try again later %v %+v", err, m)
	}
	if n := yt.runs.Load(); n != 1 {
		t.Fatalf("lookups %d", n)
	}
	if _, bad := a.trailerBad(ctx, "ageGated001", now); bad {
		t.Fatal("marked bad on try again later")
	}
	if s := a.TrailerReader(); s.Failed != 1 || s.LastReason != youtube.Refused {
		t.Fatalf("health %+v", s)
	}
	if _, err := a.resolveMusicVideo(ctx, "secondKey01"); err == nil || videoWontPlay(err) {
		t.Fatalf("music video dropped on try again later: %v", err)
	}
}

// When no trailer plays, the 404 says why. Every one tried unplayable for
// a video reason and none left: key "". At most two more are tried in one
// lookup; one left untried is the key for YouTube's player.
func TestTrailerAllUnplayableSaysWhy(t *testing.T) {
	yt := fakeReader(t, "age", "ageGated001", "thirdKey001", "fourthKey01")
	yt.set("secondKey01", "unavailable")
	srv, _ := discoverRig(t, trailerTMDB(t))
	var b struct{ Error, Reason, Key string }
	_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/trailer/movie/14", "", 404), &b)
	if b.Error != "no direct stream" || b.Reason != string(youtube.AgeRestricted) || b.Key != "" {
		t.Fatalf("all bad %+v", b)
	}
	if n := yt.runs.Load(); n != 3 {
		t.Fatalf("lookups %d", n)
	}
	// The answer is the same while the miss is kept.
	b.Reason, b.Key = "", ""
	_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/trailer/movie/14", "", 404), &b)
	if b.Reason != string(youtube.AgeRestricted) || b.Key != "" || yt.runs.Load() != 3 {
		t.Fatalf("kept miss %+v", b)
	}
	// A fresh install (no key marked bad yet): all four listed trailers are
	// tried (up to five are), so none is left for the YouTube player.
	srv, _ = discoverRig(t, trailerTMDB(t))
	b.Reason, b.Key = "", ""
	_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/trailer/movie/15", "", 404), &b)
	if b.Reason != string(youtube.AgeRestricted) || b.Key != "" || yt.count("fourthKey01") != 1 {
		t.Fatalf("all four tried %+v (asked %d)", b, yt.count("fourthKey01"))
	}
}

// Only failures at YouTube's door count toward the health check's
// failures in a row; a video YouTube will not play here does not.
func TestTrailerFailuresCountDoorReasonsOnly(t *testing.T) {
	_, st := discoverRig(t, trailerTMDB(t))
	a := New(Server{Store: st, Metadata: trailerTMDB(t), Version: "test"})
	ctx := context.Background()
	fakeReader(t, "bot")
	for i := 1; i <= 2; i++ {
		a.trailers.cache = nil
		_, _ = a.resolveTitle(ctx, "movie", "11", "en")
		if s := a.TrailerReader(); s.Failed != i || s.LastReason != youtube.BotCheck {
			t.Fatalf("bot check %d: %+v", i, s)
		}
	}
	yt := fakeReader(t, "unavailable")
	a.trailers.cache = nil
	_, _ = a.resolveTitle(ctx, "movie", "11", "en")
	if s := a.TrailerReader(); yt.runs.Load() != 1 || s.Failed != 2 || s.LastReason != youtube.BotCheck {
		t.Fatalf("a removed video counted: %+v", s)
	}
}

// The start point is the "most replayed" peak with the opening skipped
// (the opening is the highest part: every play starts there).
func TestTrailerPeakSkipsOpening(t *testing.T) {
	fakeReader(t, "heat")
	srv, _ := discoverRig(t, trailerTMDB(t))
	var s TrailerStream
	_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/trailer/movie/11", "", 200), &s)
	if s.Peak != 80 || s.Duration != 200 {
		t.Fatalf("peak %v duration %v", s.Peak, s.Duration)
	}
}

// A music video is dropped from the artist's list only when YouTube will
// not play that video, not when its door fails.
func TestVideoWontPlay(t *testing.T) {
	for err, want := range map[error]bool{
		nil:                                      false,
		errNoStream:                              false,
		&trailerMissed{reason: youtube.BotCheck}: false,
		&trailerMissed{reason: youtube.Network}:  false,
		&trailerMissed{reason: youtube.Unavailable}:   true,
		&trailerMissed{reason: youtube.AgeRestricted}: true,
	} {
		if got := videoWontPlay(err); got != want {
			t.Fatalf("%v: %v", err, got)
		}
	}
}

// A trailer needs only its addresses: once YouTube's visitor id is kept,
// the watch page (about a second) is not read for it. The music video's
// start point needs the page.
func TestTrailerSkipsTheWatchPage(t *testing.T) {
	yt := fakeReader(t, "ok")
	ctx := context.Background()
	if _, err := resolveTrailer(ctx, "trailerKey1", time.Now(), false); err != nil || yt.pages.Load() != 1 {
		t.Fatalf("first lookup (no visitor id yet) %v pages %d", err, yt.pages.Load())
	}
	if _, err := resolveTrailer(ctx, "trailerKey1", time.Now(), false); err != nil || yt.pages.Load() != 1 || yt.runs.Load() != 2 {
		t.Fatalf("trailer read the page %v pages %d", err, yt.pages.Load())
	}
	if _, err := resolveTrailer(ctx, "trailerKey1", time.Now(), true); err != nil || yt.pages.Load() != 2 {
		t.Fatalf("music video skipped the page %v pages %d", err, yt.pages.Load())
	}
}
