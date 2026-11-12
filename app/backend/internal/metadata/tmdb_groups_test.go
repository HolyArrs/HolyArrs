package metadata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
)

func fakeTMDBGroups(t *testing.T) *httptest.Server {
	t.Helper()
	list, err := os.ReadFile("testdata/tmdb_episode_groups_1433.json")
	if err != nil {
		t.Fatal(err)
	}
	one, err := os.ReadFile("testdata/tmdb_episode_group_trapped.json")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/3/tv/1433/episode_groups":
			_, _ = w.Write(list)
		case "/3/tv/64485/episode_groups":
			_, _ = w.Write([]byte(`{"id":64485,"results":[]}`))
		case "/3/tv/episode_group/64a0f1c2b7d8e9001c2a3b4c":
			_, _ = w.Write(one)
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestTMDBEpisodeGroups(t *testing.T) {
	p := newTMDB(fakeTMDBGroups(t), fakeCred)
	ctx := context.Background()
	gs, err := p.EpisodeGroups(ctx, "1433")
	if err != nil {
		t.Fatal(err)
	}
	if len(gs) != 2 || gs[0] != (EpisodeGroupInfo{ID: "5e9b7e4c1c6aa7001a3b4f21", Name: "Disney+ order", Type: 2, Seasons: 19, Episodes: 410}) || gs[1].Type != 3 {
		t.Fatalf("groups %+v", gs)
	}
	if gs, err := p.EpisodeGroups(ctx, "64485"); err != nil || gs == nil || len(gs) != 0 {
		t.Fatalf("no groups %+v %v", gs, err)
	}
	if _, err := p.EpisodeGroups(ctx, "x1"); err == nil {
		t.Fatal("a bad show id is refused")
	}
	g, err := p.EpisodeGroup(ctx, "64a0f1c2b7d8e9001c2a3b4c")
	if err != nil {
		t.Fatal(err)
	}
	if g.ID != "64a0f1c2b7d8e9001c2a3b4c" || g.Name != "International cut" || g.Type != 6 || len(g.Seasons) != 3 || len(g.Seasons[2].Episodes) != 6 {
		t.Fatalf("group %+v", g)
	}
	if e := g.Seasons[2].Episodes[2]; e.ID != "2000003" || e.Order != 2 || g.Seasons[2].Order != 2 {
		t.Fatalf("third of season 3 %+v", e)
	}
	if _, err := p.EpisodeGroup(ctx, "../tv/1"); err == nil {
		t.Fatal("a bad group id is refused")
	}
}

// trappedEpisodes are TMDB's eight episodes of season 3 plus a special.
func trappedEpisodes() []Episode {
	eps := []Episode{{Season: 0, Number: 1, ID: "999"}}
	for i := 0; i < 8; i++ {
		eps = append(eps, Episode{Season: 3, Number: i + 1, ID: strconv.Itoa(2000000 + i), AltSeason: 9, AltEpisode: 9})
	}
	return eps
}

func TestEpisodeGroupNumbers(t *testing.T) {
	p := newTMDB(fakeTMDBGroups(t), fakeCred)
	g, err := p.EpisodeGroup(context.Background(), "64a0f1c2b7d8e9001c2a3b4c")
	if err != nil {
		t.Fatal(err)
	}
	eps := trappedEpisodes()
	g.SetNumbers(eps)
	want := map[int][2]int{1: {3, 1}, 2: {3, 2}, 3: {3, 4}, 4: {3, 3}, 5: {3, 5}, 6: {3, 6}, 7: {0, 0}, 8: {0, 0}}
	for _, e := range eps[1:] {
		if got := [2]int{e.AltSeason, e.AltEpisode}; got != want[e.Number] {
			t.Fatalf("S3E%d: %v, want %v", e.Number, got, want[e.Number])
		}
	}
	if eps[0].AltSeason != 0 {
		t.Fatalf("special %+v", eps[0])
	}
}

func TestEpisodeGroupSpecialsFirst(t *testing.T) {
	g := EpisodeGroup{Seasons: []GroupSeason{{Name: "Specials", Order: 0, Episodes: []GroupEpisode{{ID: "1"}}}, {Name: "Part 1", Order: 1, Episodes: []GroupEpisode{{ID: "2", Order: 0}}}}}
	eps := []Episode{{Season: 1, Number: 1, ID: "1"}, {Season: 1, Number: 2, ID: "2"}}
	g.SetNumbers(eps)
	if eps[1].AltSeason != 1 || eps[1].AltEpisode != 1 || eps[0].AltSeason != 0 {
		t.Fatalf("specials group %+v", eps)
	}
}
