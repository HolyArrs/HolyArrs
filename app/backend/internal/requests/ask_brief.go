package requests

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// AskBrief is an open ask shown next to a looked-up movie.
type AskBrief struct {
	ID         int64  `json:"id"`
	State      string `json:"state"`
	StateLabel string `json:"state_label"`
	Mine       bool   `json:"mine"`
	People     int    `json:"people"`
	Seasons    []int  `json:"seasons,omitempty"` // a show ask's seasons; empty: the whole show
	// Albums are an artist ask's albums; empty: the whole artist.
	Albums []domain.AskAlbum `json:"albums,omitempty"`
}

// OpenMovieAsk finds the open plain or 4K ask for a movie, if any.
func OpenMovieAsk(ctx context.Context, db *sql.DB, tmdbID string, me int64, fourK bool) (*AskBrief, error) {
	var b AskBrief
	err := db.QueryRowContext(ctx, openMovieAskSQL,
		me, tmdbID, fourK).Scan(&b.ID, &b.State, &b.Mine, &b.People)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	b.StateLabel = StateLabels[b.State]
	return &b, nil
}

// OpenShowAsk finds the oldest open ask for a show with any of ids, if any.
func OpenShowAsk(ctx context.Context, db *sql.DB, ids map[string]string, me int64) (*AskBrief, error) {
	var or []string
	var args []any
	args = append(args, me)
	for _, k := range []string{"tmdb", "tvdb", "tvmaze", "imdb"} {
		if ids[k] != "" {
			or = append(or, "json_extract(a.source_ids, '$."+k+"') = ?")
			args = append(args, ids[k])
		}
	}
	if len(or) == 0 {
		return nil, nil
	}
	var b AskBrief
	var seasons string
	err := db.QueryRowContext(ctx, openShowAskSQL+strings.Join(or, " OR ")+`)
		ORDER BY a.id LIMIT 1`, args...).Scan(&b.ID, &b.State, &seasons, &b.Mine, &b.People)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	b.StateLabel = StateLabels[b.State]
	b.Seasons = []int{}
	_ = json.Unmarshal([]byte(seasons), &b.Seasons)
	return &b, nil
}

// HideDeciderNames drops the decider's sign-in name from asks shown to
// people who do not approve; the display name stays.
func HideDeciderNames(vs []View) {
	for i := range vs {
		if vs[i].DecidedBy != nil {
			vs[i].DecidedBy.Name = ""
		}
	}
}

// TitleAsks are the asks that led to a Title: who asked and who decided.
// People who do not approve see only the asks they are part of.
func (s *Service) TitleAsks(ctx context.Context, titleID int64) ([]View, error) {
	who, ok := webserver.From(ctx)
	if !ok || who.Has(webserver.PermApprove) {
		return s.Views(ctx, "a.title_id = ?", titleID)
	}
	vs, err := s.Views(ctx, titleAsksMineSQL, titleID, who.UserID)
	for i := range vs {
		vs[i].Mine = true
	}
	HideDeciderNames(vs)
	return vs, err
}

// OpenArtistAsks lists every open ask for an artist, oldest first (an
// artist can have several, for different albums).
func OpenArtistAsks(ctx context.Context, db *sql.DB, mbid string, me int64) ([]AskBrief, error) {
	rows, err := db.QueryContext(ctx, openArtistAsksSQL, me, mbid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AskBrief
	for rows.Next() {
		var b AskBrief
		var albums string
		if err := rows.Scan(&b.ID, &b.State, &albums, &b.Mine, &b.People); err != nil {
			return nil, err
		}
		b.StateLabel = StateLabels[b.State]
		b.Albums = []domain.AskAlbum{}
		_ = json.Unmarshal([]byte(albums), &b.Albums)
		out = append(out, b)
	}
	return out, rows.Err()
}
