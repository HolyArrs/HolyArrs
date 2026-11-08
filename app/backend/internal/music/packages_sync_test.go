package music

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/startup"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

func artistID(i int) string { return fmt.Sprintf("b8a7c51f-362c-4dcb-a259-%012d", i) }
func albumID(i int) string  { return fmt.Sprintf("3ff81a80-fe2f-4d9a-b542-%012d", i) }

func pkgJSON(i int, version int64, rank int) json.RawMessage {
	id, rg := artistID(i), albumID(i)
	return json.RawMessage(fmt.Sprintf(`{"schema_version":1,"mbid":"%s","version":%d,"hash":"h%d","rank":%d,"complete":true,"missing":[],
"artist":{"kind":"artist","source":"musicbrainz","source_id":"%s","title":"A%d","source_data":{"id":"%s","name":"Artist %d","type":"Group","life-span":{"ended":false}}},
"pictures":{"photo":{"v":"p%d","w":1200,"h":1200,"color":"#112233"}},"release_group_count":1,"release_groups_complete":true,"kinds":{"album":1},
"release_groups":[{"id":"%s","title":"Album %d","kind":"album","primary_type":"Album","secondary_types":[],"first_release_date":"2020","cover":{"v":"c%d","w":500,"h":500,"color":"#000000"},"tracklist":"stored"}],
"albums":{"%s":{"edition":{"id":"785f794d-2bd3-41af-bcd4-%012d","title":"Album %d","status":"Official","formats":["CD"],"track_count":1},
"tracks":[{"disc":1,"position":1,"number":"1","title":"Song %d","length_ms":1000,"recording":"6b4d4105-0ac8-41db-9c50-%012d"}],"track_count":1,"disc_count":1,"edition_count":1}}}`,
		id, version, version, rank, id, i, id, i, version, rg, i, version, rg, i, i, i, i))
}

type fakeSource struct {
	mu    sync.Mutex
	pkgs  []json.RawMessage
	wants []string
	asked []int64
}

func (f *fakeSource) MusicPackages(_ context.Context, since int64, limit int) (metadata.PackagePage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.asked = append(f.asked, since)
	limit = min(limit, 2)
	var out []json.RawMessage
	next := since
	for v := since + 1; v <= int64(len(f.pkgs)) && len(out) < limit; v++ {
		out = append(out, f.pkgs[v-1])
		next = v
	}
	return metadata.PackagePage{SchemaVersion: 1, Since: since, Next: next, More: next < int64(len(f.pkgs)), Latest: int64(len(f.pkgs)), Packages: out}, nil
}

func (f *fakeSource) WantMusicPackage(_ context.Context, mbid string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.wants = append(f.wants, mbid)
	return nil
}

func openStore(t *testing.T) *database.Store {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestSyncOnce_CopiesPackagesAndPictures(t *testing.T) {
	picGap = 0
	st := openStore(t)
	src := &fakeSource{}
	for i := 1; i <= 5; i++ {
		src.pkgs = append(src.pkgs, pkgJSON(i, int64(i), i*100))
	}
	var mu sync.Mutex
	pics := map[string]bool{}
	s := &PackageSync{Store: st, Source: src, Pictures: func(_ context.Context, kind, mbid, size, v string) bool {
		mu.Lock()
		pics[kind+"/"+mbid+"/"+size+"?"+v] = true
		mu.Unlock()
		return true
	}}
	n, err := s.SyncOnce(context.Background())
	if err != nil || n != 5 {
		t.Fatalf("changed %d err %v", n, err)
	}
	if !s.Covers(artistID(3)) || !s.HasAlbum(albumID(4)) {
		t.Fatal("index misses copied packages")
	}
	if len(pics) != 5+2+5 || !pics["artist/"+artistID(1)+"/1200?p1"] || pics["artist/"+artistID(3)+"/1200?p3"] || !pics["album/"+albumID(5)+"/500?c5"] {
		t.Fatalf("pictures %v", pics)
	}
	a, ok := s.LocalArtist(context.Background(), artistID(2))
	if !ok || a.Name != "Artist 2" || len(a.Albums) != 1 || a.Partial {
		t.Fatalf("artist %+v", a)
	}
	ps, ok, err := s.LocalPressings(context.Background(), albumID(2))
	if !ok || err != nil || len(ps) != 1 || ps[0].Tracks[0].Name != "Song 2" {
		t.Fatalf("pressings %+v", ps)
	}
	if id, ok := s.LocalAlbumArtist(context.Background(), albumID(2)); !ok || id != artistID(2) {
		t.Fatal("album artist")
	}
	if c, ok := s.AlbumCover(albumID(1)); !ok || c == nil || c.V != "c1" {
		t.Fatalf("cover %+v", c)
	}
	st1 := s.Stats()
	if st1.Packages != 5 || st1.Albums != 5 || st1.Cursor != 5 || st1.Bytes <= 0 {
		t.Fatalf("stats %+v", st1)
	}
	src.asked = nil
	if n, err := s.SyncOnce(context.Background()); err != nil || n != 0 || src.asked[0] != 5 {
		t.Fatalf("second copy changed %d asked %v err %v", n, src.asked, err)
	}
	src.pkgs = append(src.pkgs, json.RawMessage(string(pkgJSON(2, 6, 2))))
	if n, _ := s.SyncOnce(context.Background()); n != 1 {
		t.Fatalf("changed %d", n)
	}
	if c, _ := s.AlbumCover(albumID(2)); c == nil || c.V != "c6" {
		t.Fatalf("cover after change %+v", c)
	}
	s2 := &PackageSync{Store: st, Source: src}
	if err := s2.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !s2.Covers(artistID(5)) || !s2.HasAlbum(albumID(1)) {
		t.Fatal("index not read back")
	}
	if p, ok := s2.ArtistPictures(artistID(1)); !ok || p.Photo == nil || p.Photo.V != "p1" {
		t.Fatalf("pictures read back %+v", p)
	}
	if c, ok := s2.AlbumCover(albumID(2)); !ok || c == nil || c.V != "c6" {
		t.Fatalf("cover read back %+v", c)
	}
	if s2.Stats().Albums != 5 || s2.HasAlbum(albumID(99)) {
		t.Fatalf("albums %d, unlisted album found %v", s2.Stats().Albums, s2.HasAlbum(albumID(99)))
	}
}

// Low-memory mode copies every 3 hours, and a set schedule always wins.
func TestSchedule_LowMemory(t *testing.T) {
	t.Cleanup(func() { startup.SetLowMemory(false) })
	if got := (&PackageSync{}).schedule(); got != DefaultEvery {
		t.Fatalf("off: %v", got)
	}
	startup.SetLowMemory(true)
	if got := (&PackageSync{}).schedule(); got != LowMemoryEvery {
		t.Fatalf("low: %v", got)
	}
	if got := (&PackageSync{Every: time.Minute}).schedule(); got != time.Minute {
		t.Fatalf("set schedule: %v", got)
	}
}

func TestUncovered_WantsOnlyAfterWholeCopy(t *testing.T) {
	st := openStore(t)
	src := &fakeSource{pkgs: []json.RawMessage{pkgJSON(1, 1, 1)}}
	s := &PackageSync{Store: st, Source: src}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.startWants(ctx)
	s.Uncovered(artistID(9))
	if _, err := s.SyncOnce(ctx); err != nil {
		t.Fatal(err)
	}
	s.Uncovered(artistID(1))
	s.Uncovered(artistID(9))
	s.Uncovered(artistID(9))
	s.Uncovered("not-an-id")
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		src.mu.Lock()
		n := len(src.wants)
		src.mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(300 * time.Millisecond)
	src.mu.Lock()
	defer src.mu.Unlock()
	if len(src.wants) != 1 || src.wants[0] != artistID(9) {
		t.Fatalf("wants %v", src.wants)
	}
}

func TestSyncOnce_SkipsInvalidPackages(t *testing.T) {
	st := openStore(t)
	src := &fakeSource{pkgs: []json.RawMessage{json.RawMessage(`{"schema_version":2,"mbid":"x"}`), pkgJSON(2, 2, 1)}}
	s := &PackageSync{Store: st, Source: src}
	if n, err := s.SyncOnce(context.Background()); err != nil || n != 1 || s.Covers("x") || !s.Covers(artistID(2)) {
		t.Fatalf("changed %d err %v", n, err)
	}
}

// emptyPkgJSON builds a package whose release groups are not built yet.
func emptyPkgJSON(i int, version int64) json.RawMessage {
	id := artistID(i)
	return json.RawMessage(fmt.Sprintf(`{"schema_version":1,"mbid":"%s","version":%d,"hash":"e%d","rank":1,"complete":false,"missing":["release_groups"],
"artist":{"kind":"artist","source":"musicbrainz","source_id":"%s","title":"A%d","source_data":{"id":"%s","name":"Artist %d","type":"Group","life-span":{"ended":false}}},
"pictures":{},"release_group_count":0,"release_groups_complete":false,"kinds":{},"release_groups":[],"albums":{}}`, id, version, version, id, i, id, i))
}

func TestLocalArtist_EmptyPackageIsNoAnswerAndWanted(t *testing.T) {
	st := openStore(t)
	src := &fakeSource{pkgs: []json.RawMessage{pkgJSON(1, 1, 1), emptyPkgJSON(2, 2)}}
	s := &PackageSync{Store: st, Source: src}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.startWants(ctx)
	if _, err := s.SyncOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if !s.Covers(artistID(2)) {
		t.Fatal("the empty package was not kept")
	}
	if _, ok := s.LocalArtist(ctx, artistID(2)); ok {
		t.Fatal("a package with no release groups answered as a full artist")
	}
	if a, ok := s.LocalArtist(ctx, artistID(1)); !ok || len(a.Albums) != 1 {
		t.Fatalf("full package: %+v %v", a, ok)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		src.mu.Lock()
		n := len(src.wants)
		src.mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	src.mu.Lock()
	defer src.mu.Unlock()
	if len(src.wants) != 1 || src.wants[0] != artistID(2) {
		t.Fatalf("wants %v", src.wants)
	}
}

// cursorSource serves two packages a page and fails every request after
// failAfter of them, so a copy stops half way.
type cursorSource struct {
	pkgs      []json.RawMessage
	failAfter int
	asked     []int64
}

func (f *cursorSource) MusicPackages(_ context.Context, since int64, limit int) (metadata.PackagePage, error) {
	f.asked = append(f.asked, since)
	if len(f.asked) > f.failAfter {
		return metadata.PackagePage{}, fmt.Errorf("server went away")
	}
	var out []json.RawMessage
	next := since
	for v := since + 1; v <= int64(len(f.pkgs)) && len(out) < 2; v++ {
		out = append(out, f.pkgs[v-1])
		next = v
	}
	return metadata.PackagePage{SchemaVersion: 1, Since: since, Next: next, More: next < int64(len(f.pkgs)), Latest: int64(len(f.pkgs)), Packages: out}, nil
}

func (f *cursorSource) WantMusicPackage(context.Context, string) error { return nil }

func savedCursor(t *testing.T, st *database.Store) int64 {
	t.Helper()
	var c int64
	if err := st.Read().QueryRow("SELECT cursor FROM music_package_sync WHERE id = 1").Scan(&c); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestSyncOnce_ResumesFromSavedCursor(t *testing.T) {
	picGap = 0
	st := openStore(t)
	src := &cursorSource{failAfter: 1}
	for i := 1; i <= 5; i++ {
		src.pkgs = append(src.pkgs, pkgJSON(i, int64(i), i))
	}
	ctx := context.Background()
	first := &PackageSync{Store: st, Source: src}
	if n, err := first.SyncOnce(ctx); err == nil || n != 2 {
		t.Fatalf("first run changed %d err %v", n, err)
	}
	if c := savedCursor(t, st); c != 2 {
		t.Fatalf("saved cursor %d, want 2", c)
	}
	// A new copier on the same store carries on from the saved cursor.
	src.asked, src.failAfter = nil, 99
	second := &PackageSync{Store: st, Source: src}
	if n, err := second.SyncOnce(ctx); err != nil || n != 3 {
		t.Fatalf("second run changed %d err %v", n, err)
	}
	if len(src.asked) == 0 || src.asked[0] != 2 {
		t.Fatalf("second run asked %v, want to start at 2", src.asked)
	}
	if c := savedCursor(t, st); c != 5 {
		t.Fatalf("saved cursor %d, want 5", c)
	}
}
