package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/discover"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	engine "github.com/HolyArrs/holyarrs/app/backend/internal/suggest"
)

// The trailer tests run on the whole API; these are their rigs.

func trDo(t *testing.T, method, url, body string, want int) []byte {
	t.Helper()
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != want {
		t.Fatalf("%s %s: %d, want %d: %s", method, url, resp.StatusCode, want, b)
	}
	return b
}

func trGetJSON(t *testing.T, url string, want int) map[string]any {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != want {
		t.Fatalf("%s: %d, want %d", url, resp.StatusCode, want)
	}
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return out
}

// trDiscoverRig is a store with an owner, a movie (11), a show (22), a
// Never-add movie (13) and an asked movie (12), behind an API server.
func trDiscoverRig(t *testing.T, p metadata.Provider, off ...parts.Part) (*httptest.Server, *database.Store) {
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
	srv := httptest.NewServer(buildHandlers(services{Store: s, Metadata: p, Parts: pc, Version: "test"}).Handler())
	t.Cleanup(srv.Close)
	return srv, s
}

// trListTMDB is a fake TMDB that answers every list with 30 items.
func trListTMDB(t *testing.T) *metadata.TMDB {
	t.Helper()
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case strings.HasSuffix(r.URL.Path, "/genre/movie/list"), strings.HasSuffix(r.URL.Path, "/genre/tv/list"):
			_, _ = w.Write([]byte(`{"genres":[{"id":18,"name":"Drama"},{"id":35,"name":"Comedy"}]}`))
		case strings.HasSuffix(r.URL.Path, "/search/person"):
			_, _ = w.Write([]byte(`{"results":[{"id":5}]}`))
		case strings.HasSuffix(r.URL.Path, "/combined_credits"):
			_, _ = w.Write([]byte(`{"cast":[]}`))
		case strings.HasSuffix(r.URL.Path, "/collection/77"):
			var ps []string
			for i, d := range []string{"2001-05-01", "2004-05-01", "2008-05-01", "2030-05-01"} {
				ps = append(ps, fmt.Sprintf(`{"id":%d,"title":"Saga %d","release_date":%q,"genre_ids":[18],"popularity":50,"vote_average":7.5,"vote_count":400}`, 501+i, i+1, d))
			}
			_, _ = w.Write([]byte(`{"name":"Saga","parts":[` + strings.Join(ps, ",") + `]}`))
		default:
			var ps []string
			base := len(r.URL.String()) * 7
			for i := 0; i < 30; i++ {
				id := 1000 + (base+i*13)%900
				ps = append(ps, fmt.Sprintf(`{"id":%d,"title":"M%d","name":"S%d","release_date":"2026-09-01","first_air_date":"2026-09-01","genre_ids":[18],"popularity":%d,"vote_average":7.5,"vote_count":400}`, id, id, id, 100-i))
			}
			_, _ = w.Write([]byte(`{"results":[` + strings.Join(ps, ",") + `]}`))
		}
	}))
	t.Cleanup(srv.Close)
	return &metadata.TMDB{BaseURL: srv.URL + "/3", Access: func(context.Context) (string, error) { return "fake-read-value", nil }, Gap: time.Millisecond}
}

// trExploreRig is an API whose Explore lists come from a fake TMDB, with a
// member (2) and a Kid member (3) beside the owner.
func trExploreRig(t *testing.T, pick func(ctx context.Context, kind, id, lang string) (string, error)) (*handlers, *database.Store, *metadata.TMDB) {
	t.Helper()
	p := trListTMDB(t)
	_, s := trDiscoverRig(t, p)
	a := buildHandlers(services{Store: s, Metadata: p, Version: "test", TrailerPick: pick})
	a.TrailerTest().Explore().Paced = &engine.Paced{Source: discover.ListerSource{L: p}, Gap: time.Millisecond}
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO users(id, created_at, created_by, updated_at, updated_by, name, role, password_hash, max_rating) VALUES
			(2, 't', '{}', 't', '{}', 'zelda-member', 'member', 'x', ''), (3, 't', '{}', 't', '{}', 'kiddo', 'member', 'x', 'PG')`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return a, s, p
}
