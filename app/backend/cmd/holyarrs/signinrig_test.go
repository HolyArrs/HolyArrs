package main

import (
	"context"
	"database/sql"
	"strconv"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// signinRig is an API with Library titles (kind, rating) and fake images:
// every title has a 3840-wide textless backdrop, a poster and a logo.
func signinRig(t *testing.T, titles [][2]string) (*handlers, *int) {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	err = s.Write(context.Background(), func(tx *sql.Tx) error {
		for i, k := range titles {
			id := strconv.Itoa(100 + i)
			ti := mediafiles.Title{Kind: k[0], Name: "Secret Name " + id, SortName: "secret " + id, SourceIDs: map[string]string{"tmdb": id}, Rating: k[1],
				MetadataSource: "tmdb", NewPartsPolicy: "all", WhyAdded: "by_hand", State: "active", ReleaseGate: "released", GateDriver: "none", Status: "released"}
			if _, err := database.Insert(tx, "titles", &ti, domain.System); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	a := buildHandlers(services{Store: s, Version: "test"})
	calls := 0
	a.signin.Images = func(_ context.Context, kind, id string) (metadata.CardImages, error) {
		calls++
		b := metadata.ImageBase + "/w780/"
		en := "en"
		return metadata.CardImages{
			Backdrops: []metadata.CardImage{{Path: b + "t" + id + ".jpg", Lang: &en, Width: 3840, Height: 2160}, {Path: b + "b" + id + ".jpg", Width: 3840, Height: 2160}},
			Logos:     []metadata.CardImage{{Path: metadata.ImageBase + "/w500/l" + id + ".png", Lang: &en, Width: 500}},
			Posters:   []metadata.CardImage{{Path: metadata.ImageBase + "/w500/p" + id + ".jpg", Width: 2000, Height: 3000}},
		}, nil
	}
	a.signin.Trending = func(_ context.Context, kind string) ([]metadata.DiscoverItem, error) {
		var out []metadata.DiscoverItem
		for i := 0; i < 8; i++ {
			id := strconv.Itoa(900 + i)
			if kind == "show" {
				id = strconv.Itoa(950 + i)
			}
			out = append(out, metadata.DiscoverItem{Kind: kind, TMDBID: id, Name: "Trend " + id, Popularity: float64(100 - i)})
		}
		return out, nil
	}
	return a, &calls
}
