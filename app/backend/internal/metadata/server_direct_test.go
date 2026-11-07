package metadata

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// TestServerDirectRoutes: Deezer marks, the artist bio, ids, genres,
// links, soundtracks and new releases come from the metadata server only.
func TestServerDirectRoutes(t *testing.T) {
	asked := map[string]int{}
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		asked[r.URL.Path]++
		switch r.URL.Path {
		case "/v1/music/deezer/artist/" + serviceArtistID:
			_ = json.NewEncoder(w).Encode(map[string]any{"artist": map[string]any{"id": 42, "fans": 9,
				"top":    []any{map[string]any{"title": "Lost", "length_ms": 1000, "explicit": true}},
				"albums": []any{map[string]any{"id": 7, "title": "Blonde", "explicit": true}}}})
		case "/v1/music/deezer/album/7":
			_ = json.NewEncoder(w).Encode(map[string]any{"album": map[string]any{"label": "Boys Don't Cry", "explicit": true,
				"tracks": []any{map[string]any{"title": "Nikes (Remix)", "explicit": true}}}})
		case "/v1/wikidata/artist/" + serviceArtistID:
			_ = json.NewEncoder(w).Encode(map[string]any{"summary": map[string]any{"title": "Band", "extract": "A band.", "url": "https://en.wikipedia.org/wiki/Band",
				"license_url": "https://creativecommons.org/licenses/by-sa/4.0/"}})
		case "/v1/music/musicbrainz/artist/" + serviceArtistID:
			serviceReply(w, "musicbrainz", serviceArtistID, musicItem(serviceArtistID, "artist", "Band", map[string]any{
				"id": serviceArtistID, "name": "Band",
				"genres": []any{map[string]any{"name": "Pop", "count": 2}, map[string]any{"name": "Rock", "count": 5}, map[string]any{"name": "zero", "count": 0}},
				"relations": []any{
					map[string]any{"type": "deezer", "url": map[string]any{"resource": "https://www.deezer.com/artist/42"}},
					map[string]any{"type": "wikidata", "url": map[string]any{"resource": "https://www.wikidata.org/wiki/Q5"}},
					map[string]any{"type": "member of band", "artist": map[string]any{"id": serviceAlbumID, "name": "Member"}},
					map[string]any{"type": "tribute", "artist": map[string]any{"id": "5b11f4ce-a62d-471e-81fc-a69a8278c7da", "name": "Other"}},
				}}))
		case "/v1/search":
			q := r.URL.Query()
			if q.Get("kind") != "release-group" || !strings.Contains(q.Get("q"), "secondarytype:soundtrack") {
				t.Errorf("soundtrack search %s", r.URL.RawQuery)
			}
			serviceReply(w, "musicbrainz", "", map[string]any{"pagination": map[string]any{"page": 1, "total_pages": 1, "total_results": 2, "offset": 0, "limit": 5}, "items": []any{
				musicItem(serviceAlbumID, "release-group", "Dune (Original Soundtrack)", map[string]any{"id": serviceAlbumID, "title": "Dune (Original Soundtrack)", "score": 100, "first-release-date": "2021-09-03"}),
				musicItem("5b11f4ce-a62d-471e-81fc-a69a8278c7da", "release-group", "Dune Low", map[string]any{"id": "5b11f4ce-a62d-471e-81fc-a69a8278c7da", "title": "Dune Low", "score": 40, "first-release-date": "2021-01-01"}),
			}})
		case "/v1/music/releases":
			q := r.URL.Query()
			if q.Get("types") != "album,ep" || q.Get("artists") != serviceArtistID || q.Get("from") != "2026-10-01" {
				t.Errorf("releases query %s", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{
				map[string]any{"mbid": serviceAlbumID, "artist_mbid": serviceArtistID, "title": "New", "artist_name": "Band", "release_date": "2026-10-20", "type": "album", "tags": []any{"pop"}},
				map[string]any{"mbid": "bad", "artist_mbid": serviceArtistID, "title": "Odd", "release_date": "2026-10-20"},
			}})
		default:
			w.WriteHeader(404)
		}
	})
	ctx := context.Background()
	p, err := s.MusicDeezerArtist(ctx, serviceArtistID)
	if err != nil || p.ID != 42 || len(p.Albums) != 1 || !p.Albums[0].Explicit || !p.Top[0].Explicit {
		t.Fatalf("deezer artist %+v %v", p, err)
	}
	if p, err := s.MusicDeezerArtist(ctx, "5b11f4ce-a62d-471e-81fc-a69a8278c7da"); err != nil || p.ID != 0 {
		t.Fatalf("no match %+v %v", p, err)
	}
	info, err := s.MusicDeezerAlbum(ctx, 7)
	if err != nil || info.Label != "Boys Don't Cry" || !info.Tracks[SongKey("Nikes")] {
		t.Fatalf("deezer album %+v %v", info, err)
	}
	bio, err := s.MusicArtistBio(ctx, serviceArtistID)
	if err != nil || bio == nil || bio.Extract != "A band." {
		t.Fatalf("bio %+v %v", bio, err)
	}
	m := s.Artists(nil).(*serviceMusic)
	ids, err := m.ArtistIDs(ctx, serviceArtistID)
	if err != nil || ids.Deezer != 42 || ids.Wikidata != "Q5" {
		t.Fatalf("ids %+v %v", ids, err)
	}
	g, links, err := m.ArtistGenresLinks(ctx, serviceArtistID)
	if err != nil || strings.Join(g, ",") != "rock,pop" || len(links) != 1 || links[0].Name != "Member" {
		t.Fatalf("genres %v links %+v %v", g, links, err)
	}
	st, err := m.SoundtrackFor(ctx, "Dune", 2021)
	if err != nil || len(st) != 1 || st[0].MBID != serviceAlbumID {
		t.Fatalf("soundtrack %+v %v", st, err)
	}
	rel, err := m.ReleasesBetween(ctx, "2026-10-01", "2026-10-31", []string{"album", "ep"}, []string{serviceArtistID, "nope"}, 100)
	if err != nil || len(rel) != 1 || rel[0].Name != "New" || rel[0].Tags[0] != "pop" {
		t.Fatalf("releases %+v %v", rel, err)
	}
}
