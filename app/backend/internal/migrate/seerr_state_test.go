package migrate

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// Requests handled long ago in Seerr must not come over as waiting.
func TestSeerrRequestKeepsHandledState(t *testing.T) {
	for _, c := range []struct {
		status, media int64
		keep, done    bool
		state         string
	}{
		{1, 2, true, false, "pending"},
		{2, 3, true, false, "approved"},
		{2, 4, true, true, "approved"},
		{2, 5, true, true, "approved"},
		{5, 5, true, true, "approved"},
		{3, 0, false, false, ""},
	} {
		rq, keep := seerrRequest(1, c.status, "movie", map[string]any{"tmdbId": 5, "status": c.media}, 1, 1, false, "", "")
		if keep != c.keep || keep && (rq.Done != c.done || rq.State != c.state) {
			t.Errorf("status %d media %d: keep=%v done=%v state=%q", c.status, c.media, keep, rq.Done, rq.State)
		}
	}
}

func TestMoveApprovedRequestWithoutTitleIsNotWaiting(t *testing.T) {
	l := newMoveLab(t)
	ctx := context.Background()
	p := &Plan{Apps: []string{"seerr"}, Groups: []Group{{Name: GroupRequests, Label: GroupLabels[GroupRequests], Items: []Item{
		{Key: "ask:seerr:a", Name: "Processing one", Ask: &AskPlan{App: "seerr", Kind: "movie", Ref: "tmdb:910001", IDs: map[string]string{"tmdb": "910001"}, Approved: true}},
		{Key: "ask:seerr:b", Name: "Done one", Ask: &AskPlan{App: "seerr", Kind: "movie", Ref: "tmdb:910002", IDs: map[string]string{"tmdb": "910002"}, Approved: true, Done: true}},
		{Key: "ask:seerr:c", Name: "Waiting one", Ask: &AskPlan{App: "seerr", Kind: "movie", Ref: "tmdb:910003", IDs: map[string]string{"tmdb": "910003"}}},
	}}}}
	var moveID int64
	if err := l.st.Write(ctx, func(tx *sql.Tx) error {
		res, err := tx.Exec("INSERT INTO moves (created_at, owner_id, state) VALUES (?,1,'running')", database.FormatTime(time.Now().UTC()))
		if err == nil {
			moveID, err = res.LastInsertId()
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	rep := l.mv.Run(ctx, moveID, p, false, domain.Actor{Kind: "user", ID: 1}, nil)
	if rep.Counts[ResCreated] != 3 {
		t.Fatalf("results: %+v", rep.Counts)
	}
	for ref, want := range map[string]string{"tmdb:910001": "approved", "tmdb:910002": "available", "tmdb:910003": "asked"} {
		if n := l.count(t, "SELECT count(*) FROM asks WHERE ref = ? AND state = ?", ref, want); n != 1 {
			t.Errorf("%s: want state %s", ref, want)
		}
	}
}
