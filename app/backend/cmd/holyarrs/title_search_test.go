package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// route: GET /api/v1/titles
func TestLibraryTitleSearch(t *testing.T) {
	t.Parallel()
	srv, s, _ := setup(t)
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
		for _, title := range []mediafiles.Title{
			{Kind: "movie", Name: "Amélie", SortName: "amelie", Aliases: []string{"Le Fabuleux Destin"}, Year: intPtr(2001), MetadataSource: "tmdb", WhyAdded: "by_hand", State: "active"},
			{Kind: "movie", Name: "Other Film", SortName: "other film", MetadataSource: "tmdb", WhyAdded: "by_hand", State: "active"},
			{Kind: "movie", Name: "Removed Amélie", SortName: "removed amelie", MetadataSource: "tmdb", WhyAdded: "by_hand", State: "removed"},
		} {
			title.NewPartsPolicy, title.ReleaseGate, title.GateDriver, title.Status = "all", "released", "none", "released"
			if _, err := database.Insert(tx, "titles", &title, domain.System); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"amelie", "AMÉL", "Ame\u0301lie", "fabuleux", "2001", "destin fab"} {
		t.Run(q, func(t *testing.T) {
			var result struct {
				Titles []TitleView `json:"titles"`
			}
			body := do(t, "GET", srv.URL+"/api/v1/titles?q="+url.QueryEscape(q), "", 200)
			if err := json.Unmarshal(body, &result); err != nil {
				t.Fatal(err)
			}
			if len(result.Titles) != 1 || result.Titles[0].Name != "Amélie" {
				t.Fatalf("query %q returned %s", q, body)
			}
		})
	}
	for _, q := range []string{"\" OR *", "---", "\u0301", "not-a-film", `name:Other`} {
		var result struct {
			Titles []TitleView `json:"titles"`
		}
		body := do(t, "GET", srv.URL+"/api/v1/titles?q="+url.QueryEscape(q), "", 200)
		if err := json.Unmarshal(body, &result); err != nil || len(result.Titles) != 0 {
			t.Fatalf("literal query %q returned %s (%v)", q, body, err)
		}
	}
	for _, q := range []string{strings.Repeat("a", 257), strings.Repeat(" ", 257), strings.Repeat("word ", 17)} {
		do(t, "GET", srv.URL+"/api/v1/titles?q="+url.QueryEscape(q), "", 400)
	}
	for _, q := range []string{"", "  "} {
		var result struct {
			Titles []TitleView `json:"titles"`
		}
		body := do(t, "GET", srv.URL+"/api/v1/titles?q="+url.QueryEscape(q), "", 200)
		if err := json.Unmarshal(body, &result); err != nil || len(result.Titles) != 2 {
			t.Fatalf("empty query %q returned %s (%v)", q, body, err)
		}
	}
	before := s.WriteCount()
	do(t, "GET", srv.URL+"/api/v1/titles?q=amelie", "", 200)
	if after := s.WriteCount(); after != before {
		t.Fatalf("search wrote to the database: %d -> %d", before, after)
	}
}

func intPtr(n int) *int { return &n }
