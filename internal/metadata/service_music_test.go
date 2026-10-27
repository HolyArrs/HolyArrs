package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const (
	serviceArtistID    = "a0000000-0000-4000-8000-000000000001"
	serviceAlbumID     = "a0000000-0000-4000-8000-000000000002"
	servicePressingID  = "a0000000-0000-4000-8000-000000000003"
	serviceTrackID     = "a0000000-0000-4000-8000-000000000004"
	serviceRecordingID = "a0000000-0000-4000-8000-000000000005"
)

type serviceDirectMusic struct {
	search     []Artist
	artist     Artist
	pressings  []Pressing
	searchErr  error
	artistErr  error
	pressErr   error
	searchHits atomic.Int32
	artistHits atomic.Int32
	pressHits  atomic.Int32
}

func (d *serviceDirectMusic) SearchArtists(context.Context, string) ([]Artist, error) {
	d.searchHits.Add(1)
	return d.search, d.searchErr
}

func (d *serviceDirectMusic) Artist(context.Context, string) (Artist, error) {
	d.artistHits.Add(1)
	return d.artist, d.artistErr
}

func (d *serviceDirectMusic) Pressings(context.Context, string) ([]Pressing, error) {
	d.pressHits.Add(1)
	return d.pressings, d.pressErr
}

func musicItem(id, kind, title string, sourceData any) map[string]any {
	return map[string]any{"source": "musicbrainz", "source_id": id, "kind": kind, "title": title, "source_data": sourceData}
}

func musicSearchReply(w http.ResponseWriter, items []any) {
	serviceReply(w, "musicbrainz", "", map[string]any{"items": items,
		"pagination": map[string]any{"page": 1, "total_pages": 1, "total_results": len(items), "offset": 0, "limit": 25}})
}

func musicRequestID(r *http.Request) string {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	return parts[len(parts)-1]
}

func TestServiceArtistsSearchMapsMusicIdentity(t *testing.T) {
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/search" || r.URL.Query().Get("provider") != "musicbrainz" || r.URL.Query().Get("kind") != "artist" ||
			r.URL.Query().Get("q") != `Lab\+Band` || r.URL.Query().Get("page") != "1" || r.URL.Query().Get("limit") != "25" {
			t.Errorf("unexpected service search: %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		musicSearchReply(w, []any{musicItem(serviceArtistID, "artist", "Lab Band", map[string]any{
			"id": serviceArtistID, "name": "Lab Band", "sort-name": "Band, Lab", "type": "Group", "country": "GB", "score": 98,
			"disambiguation": "fixture", "life-span": map[string]any{"ended": true},
		})})
	})

	got, err := s.Artists(nil).SearchArtists(context.Background(), " Lab+Band ")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].MBID != serviceArtistID || got[0].Name != "Lab Band" || got[0].SortName != "Band, Lab" ||
		got[0].Type != "group" || got[0].Country != "GB" || got[0].Score != 98 || got[0].Status != "ended" || len(got[0].Albums) != 0 {
		t.Fatalf("mapped artist: %#v", got)
	}
}

func TestServiceArtistStaysPartialUnlessDirectIsComplete(t *testing.T) {
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		serviceReply(w, "musicbrainz", serviceArtistID, musicItem(serviceArtistID, "artist", "Service Artist", map[string]any{
			"id": serviceArtistID, "name": "Service Artist", "sort-name": "Artist, Service", "type": "Group",
			"release-groups": []any{map[string]any{"id": serviceAlbumID, "title": "Service Album", "primary-type": "Album", "first-release-date": "2020-01-02"}},
		}))
	})

	partial, err := s.Artists(nil).Artist(context.Background(), strings.ToUpper(serviceArtistID))
	if err != nil || !partial.Partial || partial.MBID != serviceArtistID || len(partial.Albums) != 1 || partial.Albums[0].MBID != serviceAlbumID {
		t.Fatalf("service artist: %#v, %v", partial, err)
	}
	direct := &serviceDirectMusic{artist: Artist{MBID: serviceArtistID, Name: "Direct Artist", Albums: []Album{{MBID: serviceAlbumID, Name: "Complete Album"}}}}
	complete, err := s.Artists(direct).Artist(context.Background(), serviceArtistID)
	if err != nil || complete.Partial || complete.Name != "Direct Artist" || direct.artistHits.Load() != 1 {
		t.Fatalf("complete direct artist: %#v, %v, hits %d", complete, err, direct.artistHits.Load())
	}
}

func TestServiceArtistKeepsPopularityWhenDirectAnswers(t *testing.T) {
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		serviceReply(w, "musicbrainz", serviceArtistID, musicItem(serviceArtistID, "artist", "Service Artist", map[string]any{
			"id": serviceArtistID, "name": "Service Artist", "popularity": map[string]any{"monthly_listeners": 94742435, "world_rank": 7, "listeners_change_today": 45004},
			"release-groups": []any{map[string]any{"id": serviceAlbumID, "title": "Service Album", "primary-type": "Album"}},
		}))
	})
	direct := &serviceDirectMusic{artist: Artist{MBID: serviceArtistID, Name: "Direct Artist", Albums: []Album{{MBID: serviceAlbumID, Name: "Complete Album"}}}}
	got, err := s.Artists(direct).Artist(context.Background(), serviceArtistID)
	if err != nil || got.Name != "Direct Artist" || got.Popularity == nil || got.Popularity.MonthlyListeners != 94742435 || got.Popularity.WorldRank != 7 {
		t.Fatalf("%#v %v", got, err)
	}
}

func TestServiceArtistTransientFallsBackButInvalidDoesNot(t *testing.T) {
	status := http.StatusServiceUnavailable
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(status)
	})
	direct := &serviceDirectMusic{artist: Artist{MBID: serviceArtistID, Name: "Direct Artist"}}
	got, err := s.Artists(direct).Artist(context.Background(), serviceArtistID)
	if err != nil || got.Name != "Direct Artist" || direct.artistHits.Load() != 1 {
		t.Fatalf("transient fallback: %#v, %v, hits %d", got, err, direct.artistHits.Load())
	}
	status = http.StatusBadRequest
	if _, err = s.Artists(direct).Artist(context.Background(), serviceArtistID); err == nil || direct.artistHits.Load() != 1 {
		t.Fatalf("terminal fallback: %v, hits %d", err, direct.artistHits.Load())
	}
}

func TestServiceMusicNotFoundMapsWithoutDirectFallback(t *testing.T) {
	t.Run("search", func(t *testing.T) {
		direct := &serviceDirectMusic{}
		s := testService(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) })
		if _, err := s.Artists(direct).SearchArtists(context.Background(), "missing"); !errors.Is(err, ErrArtistNotFound) || direct.searchHits.Load() != 0 {
			t.Fatalf("search 404: %v, direct hits %d", err, direct.searchHits.Load())
		}
	})
	t.Run("artist", func(t *testing.T) {
		direct := &serviceDirectMusic{}
		s := testService(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) })
		if _, err := s.Artists(direct).Artist(context.Background(), serviceArtistID); !errors.Is(err, ErrArtistNotFound) || direct.artistHits.Load() != 0 {
			t.Fatalf("artist 404: %v, direct hits %d", err, direct.artistHits.Load())
		}
	})
	for _, route := range []string{"release-group", "release", "recording"} {
		t.Run(route, func(t *testing.T) {
			direct := &serviceDirectMusic{}
			s := testService(t, func(w http.ResponseWriter, r *http.Request) {
				id := musicRequestID(r)
				switch {
				case route == "release-group":
					w.WriteHeader(http.StatusNotFound)
				case id == serviceAlbumID:
					serviceReply(w, "musicbrainz", id, musicItem(id, "release-group", "Album", map[string]any{
						"id": id, "releases": []any{map[string]any{"id": servicePressingID}},
					}))
				case route == "release":
					w.WriteHeader(http.StatusNotFound)
				case id == servicePressingID:
					serviceReply(w, "musicbrainz", id, musicItem(id, "release", "Pressing", map[string]any{
						"id": id, "title": "Pressing", "release-group": map[string]any{"id": serviceAlbumID},
						"media": []any{map[string]any{"position": 1, "track-count": 1, "tracks": []any{map[string]any{
							"id": serviceTrackID, "position": 1, "recording": map[string]any{"id": serviceRecordingID},
						}}}},
					}))
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			})
			if _, err := s.Artists(direct).Pressings(context.Background(), serviceAlbumID); !errors.Is(err, ErrArtistNotFound) || direct.pressHits.Load() != 0 {
				t.Fatalf("%s 404: %v, direct hits %d", route, err, direct.pressHits.Load())
			}
		})
	}
}

func TestServicePressingsBuildsRelationshipsAndFillsRecording(t *testing.T) {
	var recordingHits atomic.Int32
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		id := musicRequestID(r)
		switch id {
		case serviceAlbumID:
			serviceReply(w, "musicbrainz", id, musicItem(id, "release-group", "Service Album", map[string]any{
				"id": id, "title": "Service Album", "releases": []any{map[string]any{"id": servicePressingID}},
			}))
		case servicePressingID:
			serviceReply(w, "musicbrainz", id, musicItem(id, "release", "Service Pressing", map[string]any{
				"id": id, "title": "Service Pressing", "status": "Official", "country": "US", "date": "2020-01-02",
				"release-group": map[string]any{"id": serviceAlbumID},
				"media": []any{map[string]any{"position": 1, "format": "CD", "track-count": 1, "tracks": []any{map[string]any{
					"id": serviceTrackID, "position": 1, "number": "A1", "title": "", "recording": map[string]any{"id": serviceRecordingID},
				}}}},
			}))
		case serviceRecordingID:
			recordingHits.Add(1)
			serviceReply(w, "musicbrainz", id, musicItem(id, "recording", "Filled Track", map[string]any{
				"id": id, "title": "Filled Track", "length": 123000,
			}))
		default:
			t.Errorf("unexpected detail id %q", id)
			w.WriteHeader(http.StatusNotFound)
		}
	})

	got, err := s.Artists(nil).Pressings(context.Background(), serviceAlbumID)
	if !errors.Is(err, ErrListCut) || len(got) != 1 || recordingHits.Load() != 1 {
		t.Fatalf("pressings: %#v, %v, recording hits %d", got, err, recordingHits.Load())
	}
	p := got[0]
	if p.MBID != servicePressingID || p.Name != "Service Pressing" || p.Status != "official" || len(p.Formats) != 1 || p.Formats[0] != "CD" || len(p.Tracks) != 1 {
		t.Fatalf("mapped pressing: %#v", p)
	}
	if tr := p.Tracks[0]; tr.ID != serviceTrackID || tr.RecordingID != serviceRecordingID || tr.Name != "Filled Track" || tr.LengthMS != 123000 || tr.Disc != 1 || tr.Number != 1 || tr.Position != 1 {
		t.Fatalf("mapped track: %#v", tr)
	}
}

func TestServicePressingsRejectsWrongGroupWithoutDirectFallback(t *testing.T) {
	wrongGroup := "b0000000-0000-4000-8000-000000000099"
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		id := musicRequestID(r)
		if id == serviceAlbumID {
			serviceReply(w, "musicbrainz", id, musicItem(id, "release-group", "Album", map[string]any{"id": id, "releases": []any{map[string]any{"id": servicePressingID}}}))
			return
		}
		serviceReply(w, "musicbrainz", id, musicItem(id, "release", "Pressing", map[string]any{
			"id": id, "title": "Pressing", "release-group": map[string]any{"id": wrongGroup}, "media": []any{},
		}))
	})
	direct := &serviceDirectMusic{pressings: []Pressing{{MBID: servicePressingID}}}
	_, err := s.Artists(direct).Pressings(context.Background(), serviceAlbumID)
	if !errors.Is(err, ErrServiceInvalid) || direct.pressHits.Load() != 0 {
		t.Fatalf("wrong-group result: %v, direct hits %d", err, direct.pressHits.Load())
	}
}

func TestServicePressingsIncompleteFallsBackAndDirectCompleteWins(t *testing.T) {
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		id := musicRequestID(r)
		if id == serviceAlbumID {
			serviceReply(w, "musicbrainz", id, musicItem(id, "release-group", "Album", map[string]any{"id": id}))
			return
		}
		t.Fatalf("unexpected detail id %q", id)
	})
	direct := &serviceDirectMusic{pressings: []Pressing{{MBID: servicePressingID, Name: "Complete Direct"}}}
	got, err := s.Artists(direct).Pressings(context.Background(), serviceAlbumID)
	if err != nil || len(got) != 1 || got[0].Name != "Complete Direct" || direct.pressHits.Load() != 1 {
		t.Fatalf("incomplete fallback: %#v, %v, hits %d", got, err, direct.pressHits.Load())
	}

	direct.pressHits.Store(0)
	s = testService(t, func(w http.ResponseWriter, r *http.Request) {
		id := musicRequestID(r)
		switch id {
		case serviceAlbumID:
			serviceReply(w, "musicbrainz", id, musicItem(id, "release-group", "Album", map[string]any{"id": id, "releases": []any{map[string]any{"id": servicePressingID}}}))
		case servicePressingID:
			serviceReply(w, "musicbrainz", id, musicItem(id, "release", "Service", map[string]any{"id": id, "title": "Service", "release-group": map[string]any{"id": serviceAlbumID}, "media": []any{}}))
		}
	})
	got, err = s.Artists(direct).Pressings(context.Background(), serviceAlbumID)
	if err != nil || len(got) != 1 || got[0].Name != "Complete Direct" || direct.pressHits.Load() != 1 {
		t.Fatalf("complete direct: %#v, %v, hits %d", got, err, direct.pressHits.Load())
	}
}

func TestServiceMusicRejectsMalformedIdentifiersAndTrackCounts(t *testing.T) {
	tests := []struct {
		name string
		data map[string]any
	}{
		{"release id", map[string]any{"id": serviceAlbumID, "releases": []any{map[string]any{"id": "not-a-uuid"}}}},
		{"track count", map[string]any{
			"id": servicePressingID, "title": "Pressing", "release-group": map[string]any{"id": serviceAlbumID},
			"media": []any{map[string]any{"track-count": 2, "tracks": []any{
				map[string]any{"id": serviceTrackID, "recording": map[string]any{"id": serviceRecordingID}},
			}}},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := serviceItem{Source: "musicbrainz", SourceID: tt.data["id"].(string), Kind: "release", SourceData: mustJSON(t, tt.data)}
			if tt.name == "release id" {
				item.Kind = "release-group"
			}
			_, err := parseServicePressingItem(context.Background(), nil, serviceAlbumID, item, new(int))
			if tt.name == "release id" {
				_, err = parseServiceReleaseIDs(item)
			}
			if err == nil {
				t.Fatal("malformed relationship accepted")
			}
		})
	}
}

func TestServiceMusicRejectsSourceDataIdentityMismatches(t *testing.T) {
	otherID := "b0000000-0000-4000-8000-000000000099"
	t.Run("artist search", func(t *testing.T) {
		s := testService(t, func(w http.ResponseWriter, r *http.Request) {
			musicSearchReply(w, []any{musicItem(serviceArtistID, "artist", "Artist", map[string]any{"id": otherID, "name": "Artist"})})
		})
		if _, err := s.Artists(nil).SearchArtists(context.Background(), "Artist"); !errors.Is(err, ErrServiceInvalid) {
			t.Fatalf("artist identity mismatch: %v", err)
		}
	})
	t.Run("release group", func(t *testing.T) {
		s := testService(t, func(w http.ResponseWriter, r *http.Request) {
			serviceReply(w, "musicbrainz", serviceAlbumID, musicItem(serviceAlbumID, "release-group", "Album", map[string]any{"id": otherID, "releases": []any{}}))
		})
		if _, err := s.Artists(nil).Pressings(context.Background(), serviceAlbumID); !errors.Is(err, ErrServiceInvalid) {
			t.Fatalf("release-group identity mismatch: %v", err)
		}
	})
	t.Run("release", func(t *testing.T) {
		s := testService(t, func(w http.ResponseWriter, r *http.Request) {
			id := musicRequestID(r)
			if id == serviceAlbumID {
				serviceReply(w, "musicbrainz", id, musicItem(id, "release-group", "Album", map[string]any{"id": id, "releases": []any{map[string]any{"id": servicePressingID}}}))
				return
			}
			serviceReply(w, "musicbrainz", servicePressingID, musicItem(servicePressingID, "release", "Pressing", map[string]any{
				"id": otherID, "title": "Pressing", "release-group": map[string]any{"id": serviceAlbumID}, "media": []any{},
			}))
		})
		if _, err := s.Artists(nil).Pressings(context.Background(), serviceAlbumID); !errors.Is(err, ErrServiceInvalid) {
			t.Fatalf("release identity mismatch: %v", err)
		}
	})
}

func TestServiceMusicBoundsRelationshipFanout(t *testing.T) {
	releases := make([]serviceReleaseLink, serviceReleaseLimit+1)
	for i := range releases {
		releases[i].ID = fmt.Sprintf("a1000000-0000-4000-8000-%012d", i)
	}
	group := serviceItem{Source: "musicbrainz", SourceID: serviceAlbumID, Kind: "release-group",
		SourceData: mustJSON(t, serviceReleaseGroupData{ID: serviceAlbumID, Releases: &releases})}
	if _, err := parseServiceReleaseIDs(group); !errors.Is(err, ErrServiceIncomplete) {
		t.Fatalf("release bound: %v", err)
	}

	tracks := []serviceTrackData{{ID: serviceTrackID, Position: 1, Title: "Track"}}
	tracks[0].Length = new(int)
	tracks[0].Recording.ID = serviceRecordingID
	media := []serviceMediumData{{Position: 1, TrackCount: intPointer(1), Tracks: &tracks}}
	release := serviceItem{Source: "musicbrainz", SourceID: servicePressingID, Kind: "release",
		SourceData: mustJSON(t, serviceReleaseDataWith(servicePressingID, serviceAlbumID, media))}
	total := serviceTrackLimit
	if _, err := parseServicePressingItem(context.Background(), nil, serviceAlbumID, release, &total); !errors.Is(err, ErrServiceIncomplete) {
		t.Fatalf("track bound: %v", err)
	}
}

func serviceReleaseDataWith(id, groupID string, media []serviceMediumData) serviceReleaseData {
	data := serviceReleaseData{ID: id, Title: "Pressing", Media: &media}
	data.ReleaseGroup.ID = groupID
	return data
}

func intPointer(value int) *int { return &value }

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestServicePressingsOversizedReleaseListFetchesNoRelease(t *testing.T) {
	var releaseHits atomic.Int32
	releases := []any{}
	for i := 0; i <= serviceReleaseLimit; i++ {
		releases = append(releases, map[string]any{"id": fmt.Sprintf("b0000000-0000-4000-8000-%012d", i)})
	}
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		id := musicRequestID(r)
		if id == serviceAlbumID {
			serviceReply(w, "musicbrainz", id, musicItem(id, "release-group", "Album", map[string]any{"id": id, "releases": releases}))
			return
		}
		releaseHits.Add(1)
		w.WriteHeader(http.StatusNotFound)
	})
	direct := &serviceDirectMusic{pressings: []Pressing{{MBID: servicePressingID, Name: "Direct"}}}
	got, err := s.Artists(direct).Pressings(context.Background(), serviceAlbumID)
	if err != nil || len(got) != 1 || got[0].Name != "Direct" || releaseHits.Load() != 0 {
		t.Fatalf("oversized list: %#v, %v, release hits %d", got, err, releaseHits.Load())
	}
}

func TestServicePressingsTrackCapStopsFetchingRest(t *testing.T) {
	const n = 60
	perRelease := serviceTrackLimit/10 + 1 // the cap passes inside the tenth release
	var releaseHits atomic.Int32
	releases := []any{}
	for i := 0; i < n; i++ {
		releases = append(releases, map[string]any{"id": fmt.Sprintf("b0000000-0000-4000-8000-%012d", i)})
	}
	tracks := []any{}
	for j := 0; j < perRelease; j++ {
		tracks = append(tracks, map[string]any{"id": fmt.Sprintf("c0000000-0000-4000-8000-%012d", j), "position": j + 1, "title": "T", "length": 1000,
			"recording": map[string]any{"id": fmt.Sprintf("d0000000-0000-4000-8000-%012d", j), "title": "T", "length": 1000}})
	}
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		id := musicRequestID(r)
		if id == serviceAlbumID {
			serviceReply(w, "musicbrainz", id, musicItem(id, "release-group", "Album", map[string]any{"id": id, "releases": releases}))
			return
		}
		releaseHits.Add(1)
		serviceReply(w, "musicbrainz", id, musicItem(id, "release", "Pressing", map[string]any{
			"id": id, "title": "Pressing", "release-group": map[string]any{"id": serviceAlbumID},
			"media": []any{map[string]any{"position": 1, "track-count": perRelease, "tracks": tracks}},
		}))
	})
	if _, err := s.Artists(nil).Pressings(context.Background(), serviceAlbumID); err == nil {
		t.Fatal("expected the track cap to stop the list")
	}
	if hits := releaseHits.Load(); hits > 10+servicePressingFetches {
		t.Fatalf("asked for %d releases after the cap; want at most %d", hits, 10+servicePressingFetches)
	}
}

func TestServiceArtistCompleteListSkipsMusicBrainz(t *testing.T) {
	groups := []any{}
	for i := 0; i < 150; i++ {
		groups = append(groups, map[string]any{"id": fmt.Sprintf("%08x-0000-4000-8000-000000000000", i+1), "title": fmt.Sprint("Album ", i), "primary-type": "Album"})
	}
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		item := musicItem(serviceArtistID, "artist", "Service Artist", map[string]any{"id": serviceArtistID, "name": "Service Artist", "release-groups": groups})
		item["release_groups_complete"] = true
		serviceReply(w, "musicbrainz", serviceArtistID, item)
	})
	direct := &serviceDirectMusic{artist: Artist{MBID: serviceArtistID, Name: "Direct Artist"}}
	got, err := s.Artists(direct).Artist(QuickArtist(context.Background(), time.Second), serviceArtistID)
	if err != nil || got.Partial || len(got.Albums) != 150 || direct.artistHits.Load() != 0 {
		t.Fatalf("%v partial=%v albums=%d hits=%d", err, got.Partial, len(got.Albums), direct.artistHits.Load())
	}
}

func TestServiceAlbumOneCallForPageViews(t *testing.T) {
	const ed = "aaaaaaaa-0000-4000-8000-000000000001"
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/music/album/"+serviceAlbumID {
			t.Errorf("asked %s", r.URL.Path)
			w.WriteHeader(404)
			return
		}
		_, _ = w.Write([]byte(`{"schema_version":1,"mbid":"` + serviceAlbumID + `","title":"True","edition":{"id":"` + ed + `","title":"True","status":"Official","formats":["CD"]},"tracks":[{"disc":1,"position":1,"number":"1","title":"Wake Me Up","length_ms":247000,"recording":"BBBBBBBB-0000-4000-8000-000000000001"},{"disc":1,"position":2,"number":"2","title":"You Make Me","length_ms":233000}],"edition_count":12,"editions_complete":true}`))
	})
	direct := &serviceDirectMusic{}
	ps, err := s.Artists(direct).Pressings(MemberLookup(context.Background()), serviceAlbumID)
	if !errors.Is(err, ErrListCut) || len(ps) != 1 || ps[0].MBID != ed || len(ps[0].Tracks) != 2 || ps[0].Tracks[0].Name != "Wake Me Up" || ps[0].Tracks[0].RecordingID != "bbbbbbbb-0000-4000-8000-000000000001" || ps[0].Tracks[1].Position != 2 {
		t.Fatalf("%+v %v", ps, err)
	}
}
