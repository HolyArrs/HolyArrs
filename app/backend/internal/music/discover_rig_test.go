package music

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

type discoverTMDB struct {
	srv     *httptest.Server
	hits    atomic.Int32
	limited atomic.Bool
}

func newDiscoverTMDB(t *testing.T) *discoverTMDB {
	t.Helper()
	f := &discoverTMDB{}
	list := func(ids ...int) string {
		var out []string
		for _, id := range ids {
			out = append(out, fmt.Sprintf(`{"id":%d,"title":"M%d","name":"S%d","release_date":"2024-05-01","first_air_date":"2022-01-01","poster_path":"/p.jpg","genre_ids":[18]}`, id, id, id))
		}
		return `{"results":[` + strings.Join(out, ",") + `]}`
	}
	many := make([]int, 30)
	for i := range many {
		many[i] = 100 + i
	}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.hits.Add(1)
		if f.limited.Load() {
			w.WriteHeader(429)
			return
		}
		switch r.URL.Path {
		case "/3/genre/movie/list", "/3/genre/tv/list":
			_, _ = w.Write([]byte(`{"genres":[{"id":18,"name":"Drama"}]}`))
		case "/3/trending/movie/week":
			_, _ = w.Write([]byte(list(append([]int{11, 12, 13}, many...)...)))
		case "/3/trending/tv/week":
			_, _ = w.Write([]byte(list(21, 22)))
		case "/3/movie/popular", "/3/movie/upcoming", "/3/tv/popular", "/3/tv/on_the_air", "/3/discover/movie", "/3/discover/tv":
			_, _ = w.Write([]byte(list(31)))
		case "/3/watch/providers/movie":
			_, _ = w.Write([]byte(`{"results":[{"provider_id":8,"provider_name":"Netflix","logo_path":"/n.jpg","display_priority":5,"display_priorities":{"US":1}},{"provider_id":337,"provider_name":"Disney Plus","logo_path":"/d.jpg","display_priorities":{"US":2}}]}`))
		case "/3/watch/providers/tv":
			_, _ = w.Write([]byte(`{"results":[{"provider_id":15,"provider_name":"Hulu","display_priorities":{"US":0}},{"provider_id":8,"provider_name":"Netflix","display_priorities":{"US":3}}]}`))
		case "/3/movie/11/recommendations":
			_, _ = w.Write([]byte(list(41, 12)))
		case "/3/movie/11":
			_, _ = w.Write([]byte(`{"id":11,"title":"M11","release_date":"2024-05-01","runtime":100,"genres":[{"name":"Drama"}],
				"credits":{"cast":[{"name":"Ann","character":"Hero"}],"crew":[{"id":5,"name":"Dee","job":"Director"}]},"keywords":{"keywords":[{"name":"heist"}]},"belongs_to_collection":{"id":90,"name":"Set"},"recommendations":` + list(12, 13, 41) + `}`))
		case "/3/tv/21/watch/providers":
			_, _ = w.Write([]byte(`{"results":{"DE":{"flatrate":[{"provider_id":8,"provider_name":"Netflix"}]}}}`))
		case "/3/collection/90":
			_, _ = w.Write([]byte(`{"name":"The Set","parts":[{"id":11,"title":"M11","release_date":"2024-05-01","poster_path":"/p.jpg"},{"id":12,"title":"M12","release_date":"2025-05-01","poster_path":"/p.jpg"}]}`))
		case "/3/tv/21/season/1":
			_, _ = w.Write([]byte(`{"episodes":[{"episode_number":1,"name":"Pilot","runtime":42}]}`))
		case "/3/tv/21":
			_, _ = w.Write([]byte(`{"id":21,"name":"S21","seasons":[{"season_number":1,"episode_count":8,"air_date":"2022-01-01"}],"created_by":[{"id":6,"name":"Cee"}]}`))
		case "/3/person/5/movie_credits":
			_, _ = w.Write([]byte(`{"crew":[{"id":11,"title":"M11","job":"Director","popularity":50},{"id":12,"title":"M12","job":"Director","popularity":9},
				{"id":14,"title":"M14","job":"Director","popularity":7},{"id":15,"title":"M15","job":"Director","popularity":8},{"id":16,"title":"M16","job":"Director","popularity":1}]}`))
		case "/3/person/6/tv_credits":
			_, _ = w.Write([]byte(`{"crew":[{"id":21,"name":"S21","job":"Creator"},{"id":22,"name":"S22","job":"Creator"}]}`))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *discoverTMDB) provider() *metadata.TMDB {
	return &metadata.TMDB{BaseURL: f.srv.URL + "/3", Access: func(context.Context) (string, error) { return "fake-read-value", nil }, Gap: time.Millisecond}
}

func discoverRig(t *testing.T, p metadata.Provider, off ...parts.Part) (*httptest.Server, *database.Store) {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	err = s.Write(context.Background(), func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT INTO users(id, created_at, created_by, updated_at, updated_by, name, role, password_hash) VALUES (1, 't', '{}', 't', '{}', 'owner', 'owner', 'x')`); err != nil {
			return err
		}
		title := mediafiles.Title{Kind: "movie", Name: "M11", SortName: "m11", SourceIDs: map[string]string{"tmdb": "11"}, MetadataSource: "tmdb", NewPartsPolicy: "all", WhyAdded: "by_hand", State: "active", ReleaseGate: "released", GateDriver: "none", Status: "released"}
		if _, err := database.Insert(tx, "titles", &title, domain.System); err != nil {
			return err
		}
		show := mediafiles.Title{Kind: "show", Name: "S22", SortName: "s22", SourceIDs: map[string]string{"tmdb": "22", "tvdb": "9"}, MetadataSource: "tmdb", NewPartsPolicy: "all", WhyAdded: "by_hand", State: "active", ReleaseGate: "released", GateDriver: "none", Status: "running"}
		if _, err := database.Insert(tx, "titles", &show, domain.System); err != nil {
			return err
		}
		never := mediafiles.NeverAdd{Kind: "movie", SourceIDs: map[string]string{"tmdb": "13"}, Name: "M13", Reason: "test", CreatedAt: time.Now()}
		if _, err := database.Insert(tx, "never_add", &never, domain.System); err != nil {
			return err
		}
		ask := domain.Ask{Kind: "movie", TMDBID: "12", Name: "M12", State: domain.AskAsked, AskedBy: 1, AskedAt: time.Now(), SourceIDs: map[string]string{"tmdb": "12"}}
		if _, err := database.Insert(tx, "asks", &ask, domain.System); err != nil {
			return err
		}
		_, err := tx.Exec("INSERT INTO ask_people (ask_id, user_id, asked_at, counted) VALUES (?, 1, ?, 1)", ask.ID, database.FormatTime(time.Now()))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	var pc *parts.Control
	if len(off) > 0 {
		pc = &parts.Control{Store: s, Reg: jobs.NewRegistry(), Drain: time.Second, Logf: func(string, ...any) {}}
		if err := pc.Load(context.Background()); err != nil {
			t.Fatal(err)
		}
		for _, part := range off {
			if err := pc.Set(context.Background(), part, false, domain.System); err != nil {
				t.Fatal(err)
			}
		}
	}
	srv := httptest.NewServer(New(Server{Store: s, Metadata: p, Parts: pc, Version: "test"}).Handler())
	t.Cleanup(srv.Close)
	return srv, s
}
