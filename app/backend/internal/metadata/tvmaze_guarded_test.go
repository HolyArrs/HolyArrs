package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

// TestGuardedTVMazeMatchesTheServer is the parity check: the same TVmaze
// answers read directly (Your own keys) and through the metadata server give
// the same show.
func TestGuardedTVMazeMatchesTheServer(t *testing.T) {
	var hits atomic.Int32
	maze := fakeTVMaze(t, &hits)
	read := func(path string) json.RawMessage {
		resp, err := http.Get(maze.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return b
	}
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		var show map[string]any
		_ = json.Unmarshal(read("/shows/300"), &show)
		var eps []any
		_ = json.Unmarshal(read("/shows/300/episodes?specials=1"), &eps)
		switch r.URL.Path {
		case "/v1/shows/tvmaze/300":
			show["_embedded"] = map[string]any{"episodes": eps}
			serviceReply(w, "tvmaze", "300", normalizedItem("tvmaze", "300", "show", show))
		case "/v1/search":
			serviceReply(w, "tvmaze", "", map[string]any{"items": []any{normalizedItem("tvmaze", "300", "show", show)},
				"pagination": map[string]any{"page": 1, "total_pages": 1, "total_results": 1}})
		default:
			w.WriteHeader(404)
		}
	})
	ctx := context.Background()
	direct := GuardedTVMaze{&TVMaze{BaseURL: maze.URL, Gap: time.Millisecond}}
	viaServer := s.Shows("tvmaze", nil)
	a, err := direct.Show(ctx, "tvmaze:300")
	if err != nil {
		t.Fatal(err)
	}
	b, err := viaServer.Show(ctx, "tvmaze:300")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("direct and server differ:\n%+v\n%+v", a, b)
	}
	if len(a.Episodes) == 0 {
		t.Fatal("no episodes")
	}
	if byTVDB, err := direct.Show(ctx, "tvdb:200"); err != nil || !reflect.DeepEqual(byTVDB, a) {
		t.Fatalf("tvdb lookup: %v", err)
	}
	sa, err := direct.SearchShows(ctx, "lab")
	if err != nil {
		t.Fatal(err)
	}
	sb, err := viaServer.SearchShows(ctx, "lab")
	if err != nil || !reflect.DeepEqual(sa, sb) {
		t.Fatalf("search differs: %+v %+v %v", sa, sb, err)
	}
}

func TestGuardedTVMazeRefusesWhatTheServerRefuses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/shows/7":
			_, _ = w.Write([]byte(`{"id":7,"name":"Maze","image":{"original":"javascript:alert(1)"}}`))
		case "/shows/7/episodes":
			_, _ = w.Write([]byte(`[]`))
		case "/search/shows":
			_, _ = w.Write([]byte(`[{"show":{"id":7,"name":"Maze","image":{"original":"javascript:alert(1)"}}},{"show":{"id":8,"name":"Fine"}}]`))
		}
	}))
	defer srv.Close()
	g := GuardedTVMaze{&TVMaze{BaseURL: srv.URL, Gap: time.Millisecond}}
	if _, err := g.Show(context.Background(), "tvmaze:7"); !errors.Is(err, ErrServiceInvalid) {
		t.Fatalf("unsafe poster: %v", err)
	}
	got, err := g.SearchShows(context.Background(), "maze")
	if err != nil || len(got) != 1 || got[0].Name != "Fine" {
		t.Fatalf("search keeps only safe results: %+v %v", got, err)
	}
}

func TestModeShowsPicksBySetting(t *testing.T) {
	m := NewModes(envOf(nil))
	p := ModeShows{Modes: m, Server: staticShows{"server"}, Direct: staticShows{"direct"}}
	ctx := context.Background()
	if s, _ := p.Show(ctx, "tvmaze:1"); s.Name != "server" {
		t.Fatalf("server mode: %q", s.Name)
	}
	_ = m.Set(ModeOwn)
	if s, _ := p.Show(ctx, "tvmaze:1"); s.Name != "direct" {
		t.Fatalf("own keys: %q", s.Name)
	}
	_ = m.Set(ModeOff)
	if _, err := p.SearchShows(ctx, "x"); !SourceOff(err) {
		t.Fatalf("off: %v", err)
	}
}

type staticShows struct{ name string }

func (s staticShows) SearchShows(context.Context, string) ([]Show, error) {
	return []Show{{Name: s.name}}, nil
}
func (s staticShows) Show(context.Context, string) (Show, error) { return Show{Name: s.name}, nil }
