package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
)

// route: GET /api/v1/titles/{id}/search-outcome
func TestSearchOutcomeOwnerSeesSourcesMemberDoesNot(t *testing.T) {
	t.Parallel()
	r := newAskRig(t)
	ctx := context.Background()
	var titleID int64
	err := r.st.Write(ctx, func(tx *sql.Tx) error {
		title := mediafiles.Title{Kind: "movie", Name: "Paper Harbour", SortName: "paper harbour", SourceIDs: map[string]string{"tmdb": "900002"}, MetadataSource: "tmdb",
			Rating: "PG", WhyAdded: "by_hand", State: "active", NewPartsPolicy: "all", ReleaseGate: "released", GateDriver: "none", Status: "released", KeepUpToDate: true}
		if _, err := database.Insert(tx, "titles", &title, domain.System); err != nil {
			return err
		}
		titleID = title.ID
		now := database.FormatTime(time.Now().UTC())
		_, err := tx.Exec(`INSERT INTO asks (created_at, created_by, updated_at, updated_by, kind, tmdb_id, name, state, title_id, asked_by, asked_at)
			VALUES (?, '{}', ?, '{}', 'movie', '900002', 'Paper Harbour', 'approved', ?, ?, ?)`, now, now, titleID, r.ana.id, now)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	type outcome struct {
		Empty   bool             `json:"empty"`
		Sources []map[string]any `json:"sources"`
	}
	read := func(s *signedIn) (outcome, map[string]any) {
		b := s.call("GET", "/api/v1/titles/"+itoa(titleID)+"/search-outcome", "", 200)
		var o outcome
		var raw map[string]any
		_ = json.Unmarshal(b, &o)
		_ = json.Unmarshal(b, &raw)
		return o, raw
	}
	if o, _ := read(r.owner); o.Empty {
		t.Fatal("no search yet must not read as empty")
	}
	tid := titleID
	if err := r.st.AddEvent(ctx, &domain.Event{Actor: domain.System, Kind: sources.EventSearchFailed, TitleID: &tid,
		Summary: "no Source answered: Alpha: paused (rate_limited)",
		Data:    map[string]any{"sources": []map[string]any{{"id": 7, "name": "Alpha", "why": "rate_limited"}}}}); err != nil {
		t.Fatal(err)
	}
	o, _ := read(r.owner)
	if !o.Empty || len(o.Sources) != 1 || o.Sources[0]["name"] != "Alpha" || o.Sources[0]["why"] != "rate_limited" {
		t.Fatalf("owner got %+v", o)
	}
	m, raw := read(r.ana)
	if !m.Empty {
		t.Fatalf("member got %+v", m)
	}
	if _, has := raw["sources"]; has {
		t.Fatalf("member must not see Sources: %v", raw)
	}
}
