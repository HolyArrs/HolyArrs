package library

import (
	"context"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// An anime's AniList titles are kept as aliases, so Library search finds
// the title by its romaji or native name.
func TestAnimeAliasesKeptAndSearched(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	addFolder(t, s)
	var asked []string
	svc := &Service{Store: s, AnimeNames: func(_ context.Context, kind, tmdbID, name string) []string {
		asked = append(asked, kind+":"+tmdbID+":"+name)
		return []string{"Sen to Chihiro no Kamikakushi", "千と千尋の神隠し"}
	}}
	m := metadata.Movie{Name: "Spirited Away", Year: 2001, IDs: map[string]string{"tmdb": "129"}, Genres: []string{"Animation"}}
	opt := AddOptions{Aliases: svc.AnimeAliases(ctx, "movie", m.IDs["tmdb"], m.Name)}
	id, err := AddMovie(ctx, s, m, opt, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	var ti mediafiles.Title
	_ = database.Get(ctx, s.Read(), "titles", id, &ti)
	if len(ti.Aliases) != 2 || ti.Aliases[0] != "Sen to Chihiro no Kamikakushi" || len(asked) != 1 || asked[0] != "movie:129:Spirited Away" {
		t.Fatalf("aliases %v asked %v", ti.Aliases, asked)
	}
	for _, q := range []string{"chihiro", "Kamikakushi", "spirited"} {
		ts, err := mediafiles.SearchTitles(ctx, s, q)
		if err != nil || len(ts) != 1 || ts[0].ID != id {
			t.Fatalf("search %q: %v %v", q, ts, err)
		}
	}
	// No hook, no names: an empty list, never nil.
	if a := (&Service{}).AnimeAliases(ctx, "show", "1", "x"); a == nil || len(a) != 0 {
		t.Fatalf("no hook %v", a)
	}
	if a := (&Service{AnimeNames: func(context.Context, string, string, string) []string { return nil }}).AnimeAliases(ctx, "show", "1", "x"); a == nil {
		t.Fatal("nil aliases")
	}
}
