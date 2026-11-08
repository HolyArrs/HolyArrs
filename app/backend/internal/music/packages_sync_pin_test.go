package music

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// pinSource serves whole pages (the shared fake serves two, which costs a
// pageGap wait per page).
type pinSource struct {
	mu   sync.Mutex
	pkgs []json.RawMessage
}

func (f *pinSource) MusicPackages(_ context.Context, since int64, limit int) (metadata.PackagePage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []json.RawMessage
	next := since
	for v := since + 1; v <= int64(len(f.pkgs)) && len(out) < limit; v++ {
		out = append(out, f.pkgs[v-1])
		next = v
	}
	return metadata.PackagePage{SchemaVersion: 1, Since: since, Next: next, More: next < int64(len(f.pkgs)), Latest: int64(len(f.pkgs)), Packages: out}, nil
}

func (f *pinSource) WantMusicPackage(context.Context, string) error { return nil }

// Pins today's music_package* SQL (upsert, delete-then-insert of an
// artist's albums, cursor row, aggregates) before it moves to music/store.

func pinSyncer(t *testing.T) (*PackageSync, *pinSource) {
	t.Helper()
	picGap = 0
	src := &pinSource{}
	return &PackageSync{Store: openStore(t), Source: src}, src
}

func pinCount(t *testing.T, s *PackageSync, table string) int {
	t.Helper()
	var n int
	if err := s.Store.Read().QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestPinSyncLargeLibraryRows(t *testing.T) {
	s, src := pinSyncer(t)
	const n = 300
	for i := 1; i <= n; i++ {
		src.pkgs = append(src.pkgs, pkgJSON(i, int64(i), i))
	}
	changed, err := s.SyncOnce(context.Background())
	if err != nil || changed != n {
		t.Fatalf("changed %d err %v", changed, err)
	}
	if pinCount(t, s, "music_packages") != n || pinCount(t, s, "music_package_albums") != n || pinCount(t, s, "music_package_sync") != 1 {
		t.Fatal("row counts")
	}
	var cursor int64
	var rank int
	var complete bool
	var pics string
	if err := s.Store.Read().QueryRow("SELECT cursor FROM music_package_sync WHERE id = 1").Scan(&cursor); err != nil || cursor != n {
		t.Fatalf("cursor %d %v", cursor, err)
	}
	if err := s.Store.Read().QueryRow("SELECT rank, complete, pictures FROM music_packages WHERE mbid = ?", artistID(42)).Scan(&rank, &complete, &pics); err != nil {
		t.Fatal(err)
	}
	if rank != 42 || !complete || !strings.Contains(pics, `"photo"`) {
		t.Fatalf("row %d %v %s", rank, complete, pics)
	}
	st := s.Stats()
	if st.Packages != n || st.Albums != n || st.Cursor != n || st.Bytes <= 0 {
		t.Fatalf("stats %+v", st)
	}
	// A restart reads the same index from disk.
	s2 := &PackageSync{Store: s.Store, Source: src}
	if err := s2.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !s2.Covers(artistID(n)) || !s2.HasAlbum(albumID(n)) || s2.Covers(artistID(n+1)) {
		t.Fatal("index after load")
	}
}

// A new version of an artist's package deletes that artist's album rows and
// writes the new list: an album the artist no longer lists disappears, and
// an album that moved to this artist now points at it.
func TestPinSyncReplacesAnArtistsAlbumRows(t *testing.T) {
	s, src := pinSyncer(t)
	ctx := context.Background()
	for i := 1; i <= 3; i++ {
		src.pkgs = append(src.pkgs, pkgJSON(i, int64(i), i))
	}
	if _, err := s.SyncOnce(ctx); err != nil {
		t.Fatal(err)
	}
	// Artist 2 now lists artist 3's album instead of its own.
	moved := strings.ReplaceAll(string(pkgJSON(2, 4, 2)), albumID(2), albumID(3))
	src.pkgs = append(src.pkgs, json.RawMessage(moved))
	if n, err := s.SyncOnce(ctx); err != nil || n != 1 {
		t.Fatalf("changed %d %v", n, err)
	}
	if s.HasAlbum(albumID(2)) {
		t.Fatal("the dropped album is still listed")
	}
	if id, ok := s.LocalAlbumArtist(ctx, albumID(3)); !ok || id != artistID(2) {
		t.Fatalf("moved album artist %q %v", id, ok)
	}
	if pinCount(t, s, "music_package_albums") != 2 || pinCount(t, s, "music_packages") != 3 {
		t.Fatalf("rows: albums %d packages %d", pinCount(t, s, "music_package_albums"), pinCount(t, s, "music_packages"))
	}
	var version int64
	_ = s.Store.Read().QueryRow("SELECT version FROM music_packages WHERE mbid = ?", artistID(2)).Scan(&version)
	if version != 4 {
		t.Fatalf("version %d", version)
	}
}

// An album with no cover is stored with an empty cover and answers (nil, true);
// an invalid package is skipped but the cursor still moves past it.
func TestPinSyncNullCoverAndInvalidPackage(t *testing.T) {
	s, src := pinSyncer(t)
	ctx := context.Background()
	noCover := strings.Replace(string(pkgJSON(1, 1, 1)), `"cover":{"v":"c1","w":500,"h":500,"color":"#000000"},`, "", 1)
	if noCover == string(pkgJSON(1, 1, 1)) {
		t.Fatal("test fixture did not change")
	}
	src.pkgs = append(src.pkgs, json.RawMessage(noCover), json.RawMessage(`{"schema_version":1,"mbid":"","version":2}`))
	n, err := s.SyncOnce(ctx)
	if err != nil || n != 1 {
		t.Fatalf("changed %d %v", n, err)
	}
	var cover string
	if err := s.Store.Read().QueryRow("SELECT cover FROM music_package_albums WHERE rg = ?", albumID(1)).Scan(&cover); err != nil || cover != "" {
		t.Fatalf("cover %q %v", cover, err)
	}
	if c, ok := s.AlbumCover(albumID(1)); !ok || c != nil {
		t.Fatalf("cover answer %v %v", c, ok)
	}
	var cursor int64
	_ = s.Store.Read().QueryRow("SELECT cursor FROM music_package_sync WHERE id = 1").Scan(&cursor)
	if cursor != 2 || pinCount(t, s, "music_packages") != 1 {
		t.Fatalf("cursor %d packages %d", cursor, pinCount(t, s, "music_packages"))
	}
	if c, ok := s.AlbumCover(albumID(77)); ok || c != nil {
		t.Fatal("an unlisted album answered")
	}
}

func TestPinSyncStatsEmptyStore(t *testing.T) {
	s, _ := pinSyncer(t)
	if err := s.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	st := s.Stats()
	if st.Packages != 0 || st.Albums != 0 || st.Bytes != 0 || st.Cursor != 0 {
		t.Fatalf("stats %+v", st)
	}
	if s.Covers(artistID(1)) || s.HasAlbum(albumID(1)) {
		t.Fatal("empty store covers something")
	}
}

func TestPackageAlbumLookupCaseAndMissing(t *testing.T) {
	s, src := pinSyncer(t)
	ctx := context.Background()
	src.pkgs = append(src.pkgs, pkgJSON(1, 1, 1))
	if _, err := s.SyncOnce(ctx); err != nil {
		t.Fatal(err)
	}
	upper := strings.ToUpper(albumID(1))
	if !s.HasAlbum(upper) {
		t.Fatal("an upper-case album key is not found")
	}
	if s.HasAlbum(albumID(9)) {
		t.Fatal("an album no package lists reads as kept")
	}
	if c, ok := s.AlbumCover(upper); !ok || c == nil || c.V != "c1" {
		t.Fatalf("cover %+v %v", c, ok)
	}
	if c, ok := s.AlbumCover(albumID(9)); ok || c != nil {
		t.Fatalf("missing album cover %+v %v", c, ok)
	}
	if artist, _, ok := s.album(ctx, upper); !ok || artist != artistID(1) {
		t.Fatalf("album artist %q %v", artist, ok)
	}
	if n := s.albumCount(ctx); n != 1 {
		t.Fatalf("album count %d", n)
	}
	// The same artist again, now listing no albums.
	src.pkgs = append(src.pkgs, emptyPkgJSON(1, 2))
	if _, err := s.SyncOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if n := s.albumCount(ctx); n != 0 {
		t.Fatalf("album count after the artist lists none: %d", n)
	}
	if s.HasAlbum(albumID(1)) {
		t.Fatal("dropped album still kept")
	}
}
