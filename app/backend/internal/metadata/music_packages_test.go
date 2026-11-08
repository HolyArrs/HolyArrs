package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
)

const pkgArtist = "b8a7c51f-362c-4dcb-a259-bc6e0095f0a6"
const pkgAlbum = "3ff81a80-fe2f-4d9a-b542-4cb02fd08431"

// testPackage is a small package as the server sends it.
func testPackage(version int64) string {
	return `{"schema_version":1,"mbid":"` + pkgArtist + `","version":` + itoa(version) + `,"hash":"h","rank":3,"complete":true,"missing":[],
"artist":{"kind":"artist","source":"musicbrainz","source_id":"` + pkgArtist + `","title":"Ed","source_data":{"id":"` + pkgArtist + `","name":"Ed Sheeran","type":"Person","country":"GB","life-span":{"ended":false}}},
"pictures":{"photo":{"v":"abc123","w":1200,"h":1200,"color":"#3a66a9"}},"release_group_count":1,"release_groups_complete":true,"kinds":{"album":1},
"release_groups":[{"id":"` + pkgAlbum + `","title":"Spinning Man","kind":"album","primary_type":"Album","secondary_types":[],"first_release_date":"2005","cover":{"v":"c0ver","w":500,"h":500,"color":"#000000"},"tracklist":"stored"}],
"albums":{"` + pkgAlbum + `":{"edition":{"id":"785f794d-2bd3-41af-bcd4-d65e297fe3e6","title":"Spinning Man","status":"Official","date":"2005","country":"XW","formats":["CD"],"track_count":2},
"tracks":[{"disc":1,"position":1,"number":"1","title":"One","length_ms":1000,"recording":"6b4d4105-0ac8-41db-9c50-af651c3d5836"},{"disc":1,"position":2,"number":"2","title":"Two","length_ms":2000,"recording":"6b4d4105-0ac8-41db-9c50-af651c3d5837"}],
"track_count":2,"disc_count":1,"length_ms":3000,"edition_count":1,"fetched_at":"2026-10-28T14:17:47Z"}}}`
}

func itoa(n int64) string { b, _ := json.Marshal(n); return string(b) }

func TestMusicPackagesSendsNoCredential(t *testing.T) {
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("the package list must not carry the install's credential")
		}
		if r.URL.Path != "/v1/music/packages" || r.URL.Query().Get("since") != "5" || r.URL.Query().Get("limit") != "200" {
			t.Errorf("asked %s", r.URL)
		}
		_, _ = w.Write([]byte(`{"schema_version":1,"since":5,"next":6,"more":false,"latest":6,"packages":[` + testPackage(6) + `]}`))
	})
	page, err := s.MusicPackages(context.Background(), 5, 500)
	if err != nil || page.Next != 6 || page.More || len(page.Packages) != 1 {
		t.Fatalf("page %+v err %v", page, err)
	}
	var p MusicPackage
	if json.Unmarshal(page.Packages[0], &p) != nil || !p.Valid() {
		t.Fatal("package not valid")
	}
	a, err := p.PackageArtist()
	if err != nil || a.Name != "Ed Sheeran" || a.Country != "GB" || len(a.Albums) != 1 || a.Albums[0].PrimaryType != "album" || a.Partial {
		t.Fatalf("artist %+v err %v", a, err)
	}
	ps, ok, err := p.Pressings(pkgAlbum)
	if !ok || err != nil || len(ps) != 1 || len(ps[0].Tracks) != 2 || ps[0].Tracks[1].Name != "Two" {
		t.Fatalf("pressings %+v ok %v err %v", ps, ok, err)
	}
}

func TestMusicPackagesRefusesBadPage(t *testing.T) {
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"schema_version":2,"next":1}`))
	})
	if _, err := s.MusicPackages(context.Background(), 0, 10); !errors.Is(err, ErrServiceInvalid) {
		t.Fatalf("err %v", err)
	}
}

func TestWantMusicPackage(t *testing.T) {
	code := http.StatusAccepted
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fixture-installation" || !strings.HasSuffix(r.URL.Path, "/v1/music/package-want/"+pkgArtist) {
			t.Errorf("asked %s", r.URL)
		}
		w.WriteHeader(code)
	})
	if err := s.WantMusicPackage(context.Background(), strings.ToUpper(pkgArtist)); err != nil {
		t.Fatal(err)
	}
	code = http.StatusOK
	if err := s.WantMusicPackage(context.Background(), pkgArtist); !errors.Is(err, ErrPackageCovered) {
		t.Fatal(err)
	}
	code = http.StatusTooManyRequests
	if err := s.WantMusicPackage(context.Background(), pkgArtist); !errors.Is(err, ErrServiceBusy) {
		t.Fatal(err)
	}
}

type fakeLocal struct{ uncovered []string }

func (f *fakeLocal) LocalArtist(_ context.Context, mbid string) (Artist, bool) {
	if mbid != pkgArtist {
		return Artist{}, false
	}
	return Artist{MBID: mbid, Name: "Kept", Albums: []Album{{MBID: pkgAlbum, Name: "A"}}}, true
}
func (f *fakeLocal) LocalPressings(_ context.Context, rg string) ([]Pressing, bool, error) {
	if rg != pkgAlbum {
		return nil, false, nil
	}
	return []Pressing{{MBID: "785f794d-2bd3-41af-bcd4-d65e297fe3e6", Tracks: []Track{{Name: "One"}}}}, true, nil
}
func (f *fakeLocal) LocalAlbumArtist(_ context.Context, rg string) (string, bool) {
	return pkgArtist, rg == pkgAlbum
}
func (f *fakeLocal) Uncovered(mbid string) { f.uncovered = append(f.uncovered, mbid) }

func TestHostedMusicAnswersFromLocalFirst(t *testing.T) {
	asked := 0
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		asked++
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	l := &fakeLocal{}
	s.SetLocalMusic(l)
	m := s.Artists(nil)
	a, err := m.Artist(context.Background(), pkgArtist)
	if err != nil || a.Name != "Kept" {
		t.Fatalf("artist %+v err %v", a, err)
	}
	ps, err := m.Pressings(MemberLookup(context.Background()), pkgAlbum)
	if err != nil || len(ps) != 1 {
		t.Fatalf("pressings %v err %v", ps, err)
	}
	if id, err := m.(AlbumArtistFinder).AlbumArtist(context.Background(), pkgAlbum); err != nil || id != pkgArtist {
		t.Fatalf("album artist %q err %v", id, err)
	}
	if asked != 0 {
		t.Fatalf("the server was asked %d times for kept music", asked)
	}
	other := "35723b60-732e-4bd8-957f-320b416e7b7f"
	_, _ = m.Artist(context.Background(), other)
	if len(l.uncovered) != 1 || l.uncovered[0] != other {
		t.Fatalf("uncovered %v", l.uncovered)
	}
}

type partialLocal struct{ fakeLocal }

func (f *partialLocal) LocalArtist(ctx context.Context, mbid string) (Artist, bool) {
	a, ok := f.fakeLocal.LocalArtist(ctx, mbid)
	a.Partial = true
	return a, ok
}

func TestHostedMusicWholeArtistSkipsPartialPackage(t *testing.T) {
	asked := 0
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		asked++
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	s.SetLocalMusic(&partialLocal{})
	m := s.Artists(nil)
	if a, err := m.Artist(MemberLookup(context.Background()), pkgArtist); err != nil || a.Name != "Kept" || asked != 0 {
		t.Fatalf("page view: %+v %v asked %d", a, err, asked)
	}
	_, _ = m.Artist(WholeArtist(MemberLookup(context.Background())), pkgArtist)
	if asked == 0 {
		t.Fatal("a whole-list ask answered from the partial package")
	}
}

func TestTracklistOnlyNeverFansOut(t *testing.T) {
	var paths []string
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	})
	m := s.Artists(nil)
	_, _ = m.Pressings(TracklistOnly(MemberLookup(context.Background())), pkgAlbum)
	for _, p := range paths {
		if !strings.HasPrefix(p, "/v1/music/album/") && !strings.Contains(p, "token") && !strings.Contains(p, "access") {
			t.Fatalf("a warm-up ask went past the album answer: %v", paths)
		}
	}
	paths = nil
	_, _ = m.Pressings(MemberLookup(context.Background()), pkgAlbum)
	fanned := false
	for _, p := range paths {
		if !strings.HasPrefix(p, "/v1/music/album/") {
			fanned = true
		}
	}
	if !fanned {
		t.Fatalf("a page ask should fall back past the album answer: %v", paths)
	}
}
