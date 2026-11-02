package main

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/api"
	"github.com/CoderLuii/holyarrs-metadata/internal/budget"
	"github.com/CoderLuii/holyarrs-metadata/internal/fetchq"
	"github.com/CoderLuii/holyarrs-metadata/internal/imgstore"
	"github.com/CoderLuii/holyarrs-metadata/internal/model"
	"github.com/CoderLuii/holyarrs-metadata/internal/musictags"
	"github.com/CoderLuii/holyarrs-metadata/internal/providers"
	"github.com/CoderLuii/holyarrs-metadata/internal/store"
)

// The picture resolver reads linked IDs from the stored artist answer,
// which keeps MusicBrainz's fields under source_data.
func TestSpeedLinksReadStoredRelations(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(dir, store.Options{MaxBytes: 1 << 30, MaxEntries: 1000})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	direct, err := providers.New(providers.Options{Providers: map[string]providers.Config{"musicbrainz": {}}, Timeout: time.Second, Attempts: 1, UserAgent: "test"})
	if err != nil {
		t.Fatal(err)
	}
	o := api.Options{}
	sp, err := openSpeed(config{DataDir: dir, Speed: &speedConfig{}}, &o, budget.New(nil), "test", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sp.Close()
	sp.handler = api.New(s, budget.Fetcher{Fetcher: direct, Budget: budget.New(nil)}, o)
	const id = "a74b1b7f-71a5-4011-9441-d0b5e4122711"
	data, _ := json.Marshal(map[string]any{"source": "musicbrainz", "source_data": map[string]any{"relations": []any{
		map[string]any{"url": map[string]string{"resource": "https://www.wikidata.org/wiki/Q44190"}},
		map[string]any{"type": "member of band"},
	}}})
	now := time.Now()
	if err := s.Put(context.Background(), model.Entry{Key: api.MBStoreKey("artist", id), Result: model.Result{Data: data, Complete: true}, FetchedAt: now, ExpiresAt: now.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if got := sp.links(context.Background(), id); len(got) != 1 || got[0] != "https://www.wikidata.org/wiki/Q44190" {
		t.Fatalf("links %v", got)
	}
	if o.Control == nil || o.Images == nil || o.Videos == nil || o.StatusExtra == nil || o.OnFetched == nil {
		t.Fatal("speed options not set")
	}
}

func TestImageKeysFromEnvironment(t *testing.T) {
	env := map[string]string{"YOUTUBE_API_KEY": " y ", "SPOTIFY_CLIENT_ID": "a", "SPOTIFY_CLIENT_SECRET": "b"}
	k := imageKeys(func(n string) string { return env[n] })
	if k.YouTube != "y" || k.SpotifyID != "a" || k.SpotifySecret != "b" {
		t.Fatalf("%+v", k)
	}
}

type fakeCatalog struct {
	albums  []musictags.AlbumRef
	artists []string
}

func (f fakeCatalog) Albums() []musictags.AlbumRef { return f.albums }
func (f fakeCatalog) Artists() []string            { return f.artists }
func (f fakeCatalog) Built() time.Time             { return time.Unix(1_800_000_000, 0) }

func openTestSpeed(t *testing.T, o *api.Options) *speed {
	t.Helper()
	sp, err := openSpeed(config{DataDir: t.TempDir(), Speed: &speedConfig{}}, o, budget.New(nil), "test", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sp.Close)
	return sp
}

// Every catalog album's cover is queued for the store: the leading ones at
// P2, the rest for idle time, a capped number per pass.
func TestCatalogCoversPassQueuesMissingAlbums(t *testing.T) {
	sp := openTestSpeed(t, &api.Options{})
	ids := []string{"11111111-1111-1111-1111-111111111111", "22222222-2222-2222-2222-222222222222", "33333333-3333-3333-3333-333333333333", "44444444-4444-4444-4444-444444444444"}
	var cat fakeCatalog
	for _, id := range ids {
		cat.albums = append(cat.albums, musictags.AlbumRef{MBID: id, Artist: "A", Title: "T"})
	}
	sp.catalog = cat
	top, pass := catalogTop, catalogPass
	catalogTop, catalogPass = 2, 3
	defer func() { catalogTop, catalogPass = top, pass }()
	ctx := context.Background()
	sp.catalogCoversPass(ctx)
	st := sp.imgq.Stats(ctx)
	if st.Queued[fetchq.P2Shared] != 2 || st.Queued[fetchq.P4Deep] != 1 {
		t.Fatalf("queued %v", st.Queued)
	}
	sp.catalogCoversPass(ctx)
	if st := sp.imgq.Stats(ctx); st.Queued[fetchq.P4Deep] != 2 {
		t.Fatalf("second pass queued %v", st.Queued)
	}
	if got := sp.catalogStatus(); got["albums"] != 4 || got["stored"] != 0 {
		t.Fatalf("status %v", got)
	}
}

// An album's chain reads its names from the catalog, else its stored
// answer, and its first artist's home country (Apple's second store).
func TestAlbumSubjectFromCatalogOrStoredAnswer(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(dir, store.Options{MaxBytes: 1 << 30, MaxEntries: 1000})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	direct, err := providers.New(providers.Options{Providers: map[string]providers.Config{"musicbrainz": {}}, Timeout: time.Second, Attempts: 1, UserAgent: "test"})
	if err != nil {
		t.Fatal(err)
	}
	o := api.Options{}
	sp, err := openSpeed(config{DataDir: dir, Speed: &speedConfig{}}, &o, budget.New(nil), "test", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sp.Close()
	sp.handler = api.New(s, budget.Fetcher{Fetcher: direct, Budget: budget.New(nil)}, o)
	const cat, rg, ar = "11111111-1111-1111-1111-111111111111", "22222222-2222-2222-2222-222222222222", "33333333-3333-3333-3333-333333333333"
	sp.catalog = fakeCatalog{albums: []musictags.AlbumRef{{MBID: cat, Artist: "Artist", Title: "Album", Year: 2016}}}
	ctx := context.Background()
	if got := sp.subject(ctx, imgstore.KindAlbum, cat); !got.Known || got.Name != "Artist" || got.Title != "Album" || got.Year != 2016 {
		t.Fatalf("catalog names %+v", got)
	}
	if got := sp.subject(ctx, imgstore.KindAlbum, rg); got.Known {
		t.Fatalf("nothing stored yet: %+v", got)
	}
	put := func(kind, id string, v any) {
		data, _ := json.Marshal(map[string]any{"source": "musicbrainz", "source_data": v})
		now := time.Now()
		if err := s.Put(ctx, model.Entry{Key: api.MBStoreKey(kind, id), Result: model.Result{Data: data, Complete: true}, FetchedAt: now, ExpiresAt: now.Add(time.Hour)}); err != nil {
			t.Fatal(err)
		}
	}
	put("release-group", rg, map[string]any{"title": "Stray Sheep", "first-release-date": "2020-08-05", "artist-credit": []any{map[string]any{"name": "Kenshi", "artist": map[string]any{"id": ar, "name": "Kenshi Yonezu"}}}})
	put("artist", ar, map[string]any{"name": "Kenshi Yonezu", "area": map[string]any{"iso-3166-1-codes": []string{"JP"}}})
	got := sp.subject(ctx, imgstore.KindAlbum, rg)
	if !got.Known || got.Name != "Kenshi Yonezu" || got.Title != "Stray Sheep" || got.Year != 2020 || got.Country != "JP" {
		t.Fatalf("stored answer %+v", got)
	}
}

// Every catalog artist's wide background and logo are queued for the store.
func TestCatalogPassQueuesArtistWideAndLogo(t *testing.T) {
	sp := openTestSpeed(t, &api.Options{})
	const ar = "55555555-5555-5555-5555-555555555555"
	sp.catalog = fakeCatalog{artists: []string{ar}}
	ctx := context.Background()
	sp.catalogCoversPass(ctx)
	for _, k := range []string{"img:wide:" + ar, "img:logo:" + ar} {
		if st, _ := sp.imgq.State(ctx, k); st != "queued" {
			t.Fatalf("%s not queued: %q", k, st)
		}
	}
	if got := sp.catalogStatus(); got["artists"] != 1 || got["artists_done"] != 0 {
		t.Fatalf("status %v", got)
	}
}

// A background picture whose stored answer is missing asks for it at its
// own priority and waits for it (asked again later, never a miss); an
// artist MusicBrainz has no answer for is not asked again, and its picture
// runs with the sources that go by ID.
func TestPictureAsksForItsAnswer(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(dir, store.Options{MaxBytes: 1 << 30, MaxEntries: 1000})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	direct, err := providers.New(providers.Options{Providers: map[string]providers.Config{"musicbrainz": {}}, Timeout: time.Second, Attempts: 1, UserAgent: "test"})
	if err != nil {
		t.Fatal(err)
	}
	o := api.Options{}
	sp, err := openSpeed(config{DataDir: dir, Speed: &speedConfig{}}, &o, budget.New(nil), "test", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sp.Close()
	sp.handler = api.New(s, budget.Fetcher{Fetcher: direct, Budget: budget.New(nil)}, o)
	mbq, err := fetchq.Open(filepath.Join(dir, "mb2.sqlite"), fetchq.Options{Name: "mb", Rate: 1, Do: func(context.Context, string, int) error { return fetchq.ErrNotFound }})
	if err != nil {
		t.Fatal(err)
	}
	sp.mbq.Close() // the test's queue stands in (sp.Close closes it)
	sp.mbq = mbq
	const ar = "66666666-6666-6666-6666-666666666666"
	ctx := context.Background()
	if err := sp.runImage(ctx, "img:wide:"+ar, fetchq.P2Shared); !errors.Is(err, fetchq.ErrLater) {
		t.Fatalf("a background picture waits for its answer, got %v", err)
	}
	if st := mbq.Stats(ctx); st.Queued[fetchq.P2Shared] != 1 {
		t.Fatalf("artist answer not asked for at P2: %v", st.Queued)
	}
	mbq.Step(ctx) // the artist job dies (no such artist)
	sp.runImage(ctx, "img:wide:"+ar, fetchq.P2Shared)
	if st, _ := mbq.State(ctx, api.QueueKey("artist", ar)); st != "dead" {
		t.Fatalf("a dead artist answer was asked again: %q", st)
	}
}

// Music-video jobs run on the picture queue: asked for by the API, and
// moved there when an older build left them on the MusicBrainz queue. A
// job whose artist answer is not stored asks for it and waits; a dead
// answer ends it.
func TestVideoJobsOnPictureQueue(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(dir, store.Options{MaxBytes: 1 << 30, MaxEntries: 1000})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	direct, err := providers.New(providers.Options{Providers: map[string]providers.Config{"musicbrainz": {}}, Timeout: time.Second, Attempts: 1, UserAgent: "test"})
	if err != nil {
		t.Fatal(err)
	}
	o := api.Options{}
	sp, err := openSpeed(config{DataDir: dir, Speed: &speedConfig{}}, &o, budget.New(nil), "test", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sp.Close()
	ctx := context.Background()
	const ar, old = "77777777-7777-7777-7777-777777777777", "88888888-8888-8888-8888-888888888888"
	o.WantVideos(ar)
	if st, _ := sp.imgq.State(ctx, "hv:"+ar); st != "queued" || sp.mbq.Has(ctx, "hv:"+ar) {
		t.Fatalf("video job state %q on the picture queue", st)
	}
	if err := sp.runMB(ctx, "hv:"+old, fetchq.P2Shared); err != nil {
		t.Fatal(err)
	}
	if st, _ := sp.imgq.State(ctx, "hv:"+old); st != "queued" {
		t.Fatalf("an older build's job not moved: %q", st)
	}
	sp.handler = api.New(s, budget.Fetcher{Fetcher: direct, Budget: budget.New(nil)}, o)
	mbq, err := fetchq.Open(filepath.Join(dir, "mb2.sqlite"), fetchq.Options{Name: "mb", Rate: 1, Do: func(context.Context, string, int) error { return fetchq.ErrNotFound }})
	if err != nil {
		t.Fatal(err)
	}
	sp.mbq.Close() // the test's queue stands in (sp.Close closes it)
	sp.mbq = mbq
	if err := sp.runImage(ctx, "hv:"+ar, fetchq.P2Shared); !errors.Is(err, fetchq.ErrLater) {
		t.Fatalf("a video job waits for its artist answer, got %v", err)
	}
	if st := mbq.Stats(ctx); st.Queued[fetchq.P2Shared] != 1 {
		t.Fatalf("artist answer not asked for at P2: %v", st.Queued)
	}
	mbq.Step(ctx) // the artist job dies (no such artist)
	if err := sp.runImage(ctx, "hv:"+ar, fetchq.P2Shared); !errors.Is(err, fetchq.ErrNotFound) {
		t.Fatalf("a dead artist answer: %v", err)
	}
}

// An album the loaded slim copy holds is not queued on the MusicBrainz
// queue (its job would end at once): only its picture is asked for. One the
// copy lacks is queued as before.
func TestQueueAlbumSlimHeld(t *testing.T) {
	dir := t.TempDir()
	o := api.Options{}
	sp, err := openSpeed(config{DataDir: dir, Speed: &speedConfig{}}, &o, budget.New(nil), "test", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sp.Close()
	const held, gap = "99999999-9999-9999-9999-999999999991", "99999999-9999-9999-9999-999999999992"
	sp.fresh = func(context.Context, string, string) bool { return false }
	sp.slimHas = func(_ context.Context, kind, id string) bool { return kind == "release-group" && id == held }
	ctx := context.Background()
	sp.queueAlbum(ctx, held, fetchq.P2Shared)
	sp.queueAlbum(ctx, gap, fetchq.P4Deep)
	if sp.mbq.Has(ctx, api.QueueKey("release-group", held)) {
		t.Fatal("a slim-held album was queued")
	}
	if !sp.mbq.Has(ctx, api.QueueKey("release-group", gap)) {
		t.Fatal("an album the copy lacks was not queued")
	}
	if st := sp.imgq.Stats(ctx); st.Queued[fetchq.P1Finish] != 1 {
		t.Fatalf("the held album's picture not asked for: %v", st.Queued)
	}
}
