package metadata

import (
	"context"
	"errors"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound/adaptertest"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMetadataAdapterConformance(t *testing.T) {
	fake := memoryMetadata{}
	adaptertest.Run(t, adaptertest.Contract{
		Name: "in-memory-fake", Success: func(ctx context.Context) error { _, err := fake.Search(ctx, "Synthetic", 2026); return err },
		Failure: func(ctx context.Context) error {
			_, err := (memoryMetadata{err: errors.New("synthetic metadata failure")}).Search(ctx, "Synthetic", 2026)
			return err
		},
		Cancellation: true,
	})

	movieServer := metadataFixture(t)
	newTMDB := func(base string) *TMDB {
		return &TMDB{BaseURL: base, Access: func(context.Context) (string, error) { return "x", nil }, Gap: time.Nanosecond}
	}
	static := &Static{Movies: []Movie{{Name: "Synthetic Movie", Year: 2026, IDs: map[string]string{"tmdb": "42"}}}}
	adaptertest.Run(t, adaptertest.Contract{
		Name:        "movie/static",
		Success:     func(ctx context.Context) error { _, err := static.Search(ctx, "Synthetic", 2026); return err },
		Failure:     func(ctx context.Context) error { _, err := static.Movie(ctx, "bad"); return err },
		Unsupported: []string{"caller cancellation (pure in-memory lookup completes immediately)", "changed-movie list"},
	})
	for _, tc := range []struct {
		name string
		new  func(string) Provider
	}{
		{"movie/tmdb", func(base string) Provider { return newTMDB(base) }},
		{"movie/switch", func(base string) Provider {
			return &Switch{Use: func(context.Context) bool { return true }, Then: newTMDB(base), Else: static}
		}},
	} {
		adaptertest.Run(t, adaptertest.Contract{Name: tc.name,
			Success: func(ctx context.Context) error {
				_, err := tc.new(movieServer.URL).Search(ctx, "Synthetic", 2026)
				return err
			},
			Failure: func(ctx context.Context) error {
				_, err := tc.new(movieServer.URL+"/fail").Search(ctx, "Synthetic", 2026)
				return err
			},
			Cancellation: true,
		})
	}

	showCases := []struct {
		name string
		new  func(string) ShowProvider
	}{
		{"show/tmdb", func(base string) ShowProvider { return newTMDB(base) }},
		{"show/tvmaze", func(base string) ShowProvider { return &TVMaze{BaseURL: base, Gap: time.Nanosecond} }},
		{"show/tvdb", func(base string) ShowProvider {
			return &TVDB{BaseURL: base, Key: func(context.Context) (string, error) { return "x", nil }, Gap: time.Nanosecond}
		}},
	}
	for _, tc := range showCases {
		adaptertest.Run(t, adaptertest.Contract{Name: tc.name,
			Success: func(ctx context.Context) error {
				_, err := tc.new(movieServer.URL).SearchShows(ctx, "Synthetic")
				return err
			},
			Failure: func(ctx context.Context) error {
				_, err := tc.new(movieServer.URL+"/fail").SearchShows(ctx, "Synthetic")
				return err
			},
			Cancellation: true,
		})
	}
	t.Run("show/source-selection", func(t *testing.T) {
		sources := ShowSources{"tmdb": newTMDB(movieServer.URL)}
		if _, err := sources.Get("tmdb"); err != nil {
			t.Fatal(err)
		}
		if _, err := sources.Get("unsupported"); !errors.Is(err, ErrUnknownSource) {
			t.Fatalf("unknown source: %v", err)
		}
	})

	newMB := func(base string) *MusicBrainz {
		return &MusicBrainz{BaseURL: func(context.Context) string { return base }, UserAgent: UserAgentFor("test"),
			Limiter: &Limiter{Every: time.Nanosecond}, Tries: 1}
	}
	adaptertest.Run(t, adaptertest.Contract{Name: "artist/musicbrainz",
		Success: func(ctx context.Context) error {
			_, err := newMB(movieServer.URL).SearchArtists(ctx, "Synthetic")
			return err
		},
		Failure: func(ctx context.Context) error {
			_, err := newMB(movieServer.URL+"/fail").SearchArtists(ctx, "Synthetic")
			return err
		},
		Cancellation: true,
	})
	adaptertest.Run(t, adaptertest.Contract{Name: "people/tmdb",
		Success: func(ctx context.Context) error {
			_, err := newTMDB(movieServer.URL).SearchPeople(ctx, "Synthetic", 1)
			return err
		},
		Failure: func(ctx context.Context) error {
			_, err := newTMDB(movieServer.URL+"/fail").SearchPeople(ctx, "Synthetic", 1)
			return err
		},
		Cancellation: true,
	})
	adaptertest.Run(t, adaptertest.Contract{Name: "changes/tmdb",
		Success: func(ctx context.Context) error {
			_, err := newTMDB(movieServer.URL).Changed(ctx, time.Now().Add(-time.Hour))
			return err
		},
		Failure: func(ctx context.Context) error {
			_, err := newTMDB(movieServer.URL+"/fail").Changed(ctx, time.Now().Add(-time.Hour))
			return err
		},
		Cancellation: true,
	})
	adaptertest.Run(t, adaptertest.Contract{Name: "changes/switch",
		Success: func(ctx context.Context) error {
			_, err := (&Switch{Use: func(context.Context) bool { return true }, Then: newTMDB(movieServer.URL), Else: static}).Changed(ctx, time.Now().Add(-time.Hour))
			return err
		},
		Failure: func(ctx context.Context) error {
			_, err := (&Switch{Use: func(context.Context) bool { return false }, Then: newTMDB(movieServer.URL), Else: static}).Changed(ctx, time.Now())
			return err
		},
		Cancellation: true,
		Unsupported:  []string{"changed-movie list when the selected provider does not implement ChangeLister"},
	})
}

func TestMetadataAdapterOperationConformance(t *testing.T) {
	srv := metadataFixture(t)
	newTMDB := func(base string) *TMDB {
		return &TMDB{BaseURL: base, Access: func(context.Context) (string, error) { return "x", nil }, Gap: time.Nanosecond}
	}
	newMB := func(base string) *MusicBrainz {
		return &MusicBrainz{BaseURL: func(context.Context) string { return base }, UserAgent: UserAgentFor("test"), Limiter: &Limiter{Every: time.Nanosecond}, Tries: 1}
	}
	const artistID = "11111111-1111-1111-1111-111111111111"
	const albumID = "22222222-2222-2222-2222-222222222222"

	runMetadataOperation(t, "movie/tmdb/Movie",
		func(ctx context.Context, m memoryMetadata) error { _, err := m.Movie(ctx, "fake:1"); return err },
		func(ctx context.Context) error { _, err := newTMDB(srv.URL).Movie(ctx, "tmdb:42"); return err },
		func(ctx context.Context) error { _, err := newTMDB(srv.URL+"/fail").Movie(ctx, "tmdb:42"); return err })
	runMetadataOperation(t, "movie/switch/Movie",
		func(ctx context.Context, m memoryMetadata) error { _, err := m.Movie(ctx, "fake:1"); return err },
		func(ctx context.Context) error {
			_, err := (&Switch{Use: func(context.Context) bool { return true }, Then: newTMDB(srv.URL), Else: Lab()}).Movie(ctx, "tmdb:42")
			return err
		},
		func(ctx context.Context) error {
			_, err := (&Switch{Use: func(context.Context) bool { return true }, Then: newTMDB(srv.URL + "/fail"), Else: Lab()}).Movie(ctx, "tmdb:42")
			return err
		})

	for _, tc := range []struct {
		name string
		new  func(string) ShowProvider
		id   string
	}{
		{"show/tmdb/Show", func(base string) ShowProvider { return newTMDB(base) }, "tmdb:7"},
		{"show/tvmaze/Show", func(base string) ShowProvider { return &TVMaze{BaseURL: base, Gap: time.Nanosecond} }, "tvmaze:8"},
		{"show/tvdb/Show", func(base string) ShowProvider {
			return &TVDB{BaseURL: base, Key: func(context.Context) (string, error) { return "x", nil }, Gap: time.Nanosecond}
		}, "tvdb:9"},
	} {
		tc := tc
		runMetadataOperation(t, tc.name,
			func(ctx context.Context, m memoryMetadata) error { _, err := m.Show(ctx, "fake:1"); return err },
			func(ctx context.Context) error { _, err := tc.new(srv.URL).Show(ctx, tc.id); return err },
			func(ctx context.Context) error { _, err := tc.new(srv.URL+"/fail").Show(ctx, tc.id); return err })
	}
	runMetadataOperation(t, "artist/musicbrainz/Artist",
		func(ctx context.Context, m memoryMetadata) error { _, err := m.Artist(ctx, artistID); return err },
		func(ctx context.Context) error { _, err := newMB(srv.URL).Artist(ctx, artistID); return err },
		func(ctx context.Context) error { _, err := newMB(srv.URL+"/fail").Artist(ctx, artistID); return err })
	runMetadataOperation(t, "artist/musicbrainz/Pressings",
		func(ctx context.Context, m memoryMetadata) error { _, err := m.Pressings(ctx, albumID); return err },
		func(ctx context.Context) error { _, err := newMB(srv.URL).Pressings(ctx, albumID); return err },
		func(ctx context.Context) error { _, err := newMB(srv.URL+"/fail").Pressings(ctx, albumID); return err })
	runMetadataOperation(t, "people/tmdb/Person",
		func(ctx context.Context, m memoryMetadata) error { _, err := m.Person(ctx, "tmdb:10"); return err },
		func(ctx context.Context) error { _, err := newTMDB(srv.URL).Person(ctx, "tmdb:10"); return err },
		func(ctx context.Context) error { _, err := newTMDB(srv.URL+"/fail").Person(ctx, "tmdb:10"); return err })
	runMetadataOperation(t, "people/tmdb/PersonMovies",
		func(ctx context.Context, m memoryMetadata) error {
			_, err := m.PersonMovies(ctx, "tmdb:10")
			return err
		},
		func(ctx context.Context) error { _, err := newTMDB(srv.URL).PersonMovies(ctx, "tmdb:10"); return err },
		func(ctx context.Context) error {
			_, err := newTMDB(srv.URL+"/fail").PersonMovies(ctx, "tmdb:10")
			return err
		})
	runMetadataOperation(t, "people/tmdb/MoviePeople",
		func(ctx context.Context, m memoryMetadata) error { _, err := m.MoviePeople(ctx, "tmdb:42"); return err },
		func(ctx context.Context) error { _, err := newTMDB(srv.URL).MoviePeople(ctx, "tmdb:42"); return err },
		func(ctx context.Context) error {
			_, err := newTMDB(srv.URL+"/fail").MoviePeople(ctx, "tmdb:42")
			return err
		})
}

func runMetadataOperation(t *testing.T, name string, fake func(context.Context, memoryMetadata) error, success, failure func(context.Context) error) {
	t.Helper()
	adaptertest.Run(t, adaptertest.Contract{Name: name + "/fake",
		Success: func(ctx context.Context) error { return fake(ctx, memoryMetadata{}) },
		Failure: func(ctx context.Context) error {
			return fake(ctx, memoryMetadata{err: errors.New("synthetic metadata failure")})
		}, Cancellation: true})
	adaptertest.Run(t, adaptertest.Contract{Name: name + "/actual", Success: success, Failure: failure, Cancellation: true})
}

func metadataFixture(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/fail/") {
			http.Error(w, "synthetic failure", http.StatusServiceUnavailable)
			return
		}
		switch r.URL.Path {
		case "/search/movie":
			ioJSON(w, `{"results":[{"id":42,"title":"Synthetic Movie","release_date":"2026-01-02"}]}`)
		case "/movie/42":
			ioJSON(w, `{"id":42,"title":"Synthetic Movie","release_date":"2026-01-02"}`)
		case "/search/tv":
			ioJSON(w, `{"results":[{"id":7,"name":"Synthetic Show","first_air_date":"2026-01-02","status":"Returning Series"}]}`)
		case "/tv/7":
			ioJSON(w, `{"id":7,"name":"Synthetic Show","first_air_date":"2026-01-02","status":"Returning Series","seasons":[]}`)
		case "/search/shows":
			ioJSON(w, `[{"show":{"id":8,"name":"Synthetic Maze","premiered":"2026-01-02","status":"Running"}}]`)
		case "/shows/8":
			ioJSON(w, `{"id":8,"name":"Synthetic Maze","premiered":"2026-01-02","status":"Running"}`)
		case "/shows/8/episodes":
			ioJSON(w, `[]`)
		case "/login":
			ioJSON(w, `{"data":{"token":"x"}}`)
		case "/search":
			ioJSON(w, `{"data":[{"tvdb_id":"9","name":"Synthetic TVDB","year":"2026"}]}`)
		case "/series/9/extended":
			ioJSON(w, `{"data":{"id":9,"name":"Synthetic TVDB","year":"2026"}}`)
		case "/series/9/episodes/default":
			ioJSON(w, `{"data":{"episodes":[]},"links":{"next":null}}`)
		case "/ws/2/artist":
			ioJSON(w, `{"artists":[{"id":"11111111-1111-1111-1111-111111111111","name":"Synthetic Artist"}]}`)
		case "/ws/2/artist/11111111-1111-1111-1111-111111111111":
			ioJSON(w, `{"id":"11111111-1111-1111-1111-111111111111","name":"Synthetic Artist"}`)
		case "/ws/2/release-group":
			ioJSON(w, `{"release-group-count":0,"release-groups":[]}`)
		case "/ws/2/release":
			ioJSON(w, `{"release-count":0,"releases":[]}`)
		case "/search/person":
			ioJSON(w, `{"page":1,"total_pages":1,"total_results":1,"results":[{"id":10,"name":"Synthetic Person"}]}`)
		case "/person/10":
			ioJSON(w, `{"id":10,"name":"Synthetic Person"}`)
		case "/person/10/movie_credits", "/movie/42/credits":
			ioJSON(w, `{"cast":[],"crew":[]}`)
		case "/movie/changes":
			ioJSON(w, `{"page":1,"total_pages":1,"results":[{"id":42}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func ioJSON(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(body))
}

type memoryMetadata struct{ err error }

func (m memoryMetadata) check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return m.err
}
func (m memoryMetadata) Search(ctx context.Context, _ string, _ int) ([]Movie, error) {
	return []Movie{{Name: "Synthetic"}}, m.check(ctx)
}
func (m memoryMetadata) Movie(ctx context.Context, _ string) (Movie, error) {
	return Movie{Name: "Synthetic"}, m.check(ctx)
}
func (m memoryMetadata) Changed(ctx context.Context, _ time.Time) ([]string, error) {
	return []string{"fake:1"}, m.check(ctx)
}
func (m memoryMetadata) SearchShows(ctx context.Context, _ string) ([]Show, error) {
	return []Show{{Name: "Synthetic"}}, m.check(ctx)
}
func (m memoryMetadata) Show(ctx context.Context, _ string) (Show, error) {
	return Show{Name: "Synthetic"}, m.check(ctx)
}
func (m memoryMetadata) SearchArtists(ctx context.Context, _ string) ([]Artist, error) {
	return []Artist{{Name: "Synthetic"}}, m.check(ctx)
}
func (m memoryMetadata) Artist(ctx context.Context, _ string) (Artist, error) {
	return Artist{Name: "Synthetic"}, m.check(ctx)
}
func (m memoryMetadata) Pressings(ctx context.Context, _ string) ([]Pressing, error) {
	return []Pressing{}, m.check(ctx)
}
func (m memoryMetadata) SearchPeople(ctx context.Context, _ string, _ int) (PersonPage, error) {
	return PersonPage{}, m.check(ctx)
}
func (m memoryMetadata) Person(ctx context.Context, _ string) (Person, error) {
	return Person{Name: "Synthetic"}, m.check(ctx)
}
func (m memoryMetadata) PersonMovies(ctx context.Context, _ string) (PersonMoviePage, error) {
	return PersonMoviePage{}, m.check(ctx)
}
func (m memoryMetadata) MoviePeople(ctx context.Context, _ string) ([]MoviePersonCredit, error) {
	return []MoviePersonCredit{}, m.check(ctx)
}

var _ Provider = memoryMetadata{}
var _ ChangeLister = memoryMetadata{}
var _ ShowProvider = memoryMetadata{}
var _ ArtistProvider = memoryMetadata{}
var _ PeopleProvider = memoryMetadata{}
