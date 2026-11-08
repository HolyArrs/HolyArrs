package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata/youtube"
)

// manyListed is a TMDB whose movie 77 lists seven YouTube trailers
// (manyTrail01..07), and their keys.
func manyListed(t *testing.T) ([]string, *metadata.TMDB) {
	t.Helper()
	keys := make([]string, 7)
	var listed []string
	for i := range keys {
		keys[i] = fmt.Sprintf("manyTrail0%d", i+1)
		listed = append(listed, `{"key":"`+keys[i]+`","site":"YouTube","type":"Trailer","official":true,"iso_639_1":"en"}`)
	}
	tmdb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/3/genre/movie/list", "/3/genre/tv/list":
			_, _ = w.Write([]byte(`{"genres":[]}`))
		case "/3/movie/77":
			_, _ = w.Write([]byte(`{"id":77,"title":"M77","videos":{"results":[` + strings.Join(listed, ",") + `]}}`))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(tmdb.Close)
	return keys, &metadata.TMDB{BaseURL: tmdb.URL + "/3", Access: func(context.Context) (string, error) { return "fake-read-value", nil }, Gap: time.Millisecond}
}

// route: GET /api/v1/trailer/{kind}/{tmdb_id}
// A title with seven trailers whose first five ask for an age sign-in: one
// lookup tries the picked one and four more (five in all), never the rest,
// and the 404 says why and hands the sixth to YouTube's player.
func TestTrailerManyListedStopsAtFive(t *testing.T) {
	keys, p := manyListed(t)
	yt := fakeReader(t, "age", keys[:5]...)
	yt.set(keys[5], "ok")
	yt.set(keys[6], "ok")
	_, st := trDiscoverRig(t, p)
	a := buildHandlers(services{Store: st, Metadata: p, Version: "test"})
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)

	_ = lookUp(t, a, "movie", "77")
	if n := yt.runs.Load(); n != 5 {
		t.Fatalf("lookups %d, want 5", n)
	}
	for _, k := range keys[:5] {
		if yt.count(k) != 1 {
			t.Fatalf("%s asked %d times", k, yt.count(k))
		}
	}
	for _, k := range keys[5:] {
		if yt.count(k) != 0 {
			t.Fatalf("%s looked up past the limit", k)
		}
	}
	var b struct{ Error, Reason, Key string }
	_ = json.Unmarshal(trDo(t, "GET", srv.URL+"/api/v1/trailer/movie/77", "", 404), &b)
	if b.Error != "no direct stream" || b.Reason != string(youtube.AgeRestricted) || b.Key != keys[5] {
		t.Fatalf("answer %+v", b)
	}
}

// One title whose five tried trailers all fail (here: none will play
// here, which is not marked bad, so every lookup tries all five again)
// counts once toward the health check, however often it is looked up: a
// single title never adds up to the warning.
func TestTrailerUpkeepCountsATitleOnce(t *testing.T) {
	keys, p := manyListed(t)
	yt := fakeReader(t, "premiere", keys...)
	_, st := trDiscoverRig(t, p)
	a := buildHandlers(services{Store: st, Metadata: p, Version: "test"})
	// A first lookup of a title that will not play says nothing about the
	// reader (after a restart, known misses are looked up again).
	recentGood(a, time.Hour)
	_ = lookUp(t, a, "movie", "77")
	if s := a.Trailers().ReaderState(); s.Videos != 0 {
		t.Fatalf("first lookup counted: %+v", s)
	}
	yt.runs.Store(0)
	// The same title failing on its renewals (it played here before) counts,
	// once.
	for i := 1; i <= 3; i++ {
		a.Trailers().Forget()
		if _, err := a.Trailers().ResolveTitle(metadata.Renewing(context.Background()), "movie", "77", "en"); err == nil {
			t.Fatal("played")
		}
		if n := yt.runs.Load(); n != int32(5*i) {
			t.Fatalf("lookup %d: %d videos tried, want %d", i, n, 5*i)
		}
		if s := a.Trailers().ReaderState(); s.Videos != 1 || s.LastReason != youtube.Unplayable {
			t.Fatalf("lookup %d: %+v", i, s)
		}
	}
	// The same title in another language is still one title.
	a.Trailers().Forget()
	_, _ = a.Trailers().ResolveTitle(metadata.Renewing(context.Background()), "movie", "77", "de")
	if s := a.Trailers().ReaderState(); s.Videos != 1 {
		t.Fatalf("another language counted again: %+v", s)
	}
}
