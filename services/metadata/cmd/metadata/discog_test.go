package main

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/CoderLuii/holyarrs-metadata/internal/discog"
	"github.com/CoderLuii/holyarrs-metadata/internal/fetchq"
	"github.com/CoderLuii/holyarrs-metadata/internal/model"
	"github.com/CoderLuii/holyarrs-metadata/internal/sharedfill"
)

// The prefetch queues the shared lists' artists and albums not stored
// yet (never past its cap, never twice), and none while MusicBrainz is
// paused.
func TestDiscogPrefetchPass(t *testing.T) {
	dir := t.TempDir()
	q, err := fetchq.Open(filepath.Join(dir, "q.sqlite"), fetchq.Options{Do: func(context.Context, string, int) error { return nil }, Logf: func(string, ...any) {}})
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	fail := error(nil)
	d, err := discog.Open(filepath.Join(dir, "d.sqlite"), discog.Options{Logf: func(string, ...any) {}, Get: func(context.Context, string, url.Values, any) error { return fail }})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	var ms []sharedfill.Member
	for i := range 5 {
		ms = append(ms, sharedfill.Member{Kind: "artist", MBID: fmt.Sprintf("00000000-0000-0000-0000-%012d", i)})
	}
	ms = append(ms, sharedfill.Member{Kind: "release-group", MBID: "22222222-2222-2222-2222-222222222222"})
	fill := &sharedfill.Filler{Lists: map[string]func(context.Context) []sharedfill.Member{"charts": func(context.Context) []sharedfill.Member { return ms }}, Enqueue: func(context.Context, sharedfill.Member) {}}
	fill.Once(context.Background())
	sp := &speed{mbq: q, disc: d, fill: fill}
	old := discogPass
	discogPass = 4
	defer func() { discogPass = old }()
	sp.discogPassOnce(context.Background(), discogSources{})
	st := sp.discogLast()
	if st.Queued != 4 || st.SkippedCap != 2 || st.Wanted["shared_artists"] != 5 || st.Wanted["shared_albums"] != 1 {
		t.Fatalf("pass %+v", st)
	}
	sp.discogPassOnce(context.Background(), discogSources{})
	if st := sp.discogLast(); st.Queued != 2 || st.AlreadyQueued != 4 {
		t.Fatalf("second pass %+v", st)
	}
	fail = &model.FetchError{Code: "upstream_unavailable", Status: 503}
	for i := range 3 {
		d.Run(context.Background(), discog.ArtistKey(fmt.Sprintf("00000000-0000-0000-0000-%012d", i)), fetchq.P0Waiting)
	}
	sp.discogPassOnce(context.Background(), discogSources{})
	if st := sp.discogLast(); !st.Paused || st.Queued != 0 {
		t.Fatalf("paused pass %+v", st)
	}
}
