package requests

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// PersonQualities are the lines of what people can ask for that one person
// may pick, per kind, set in the person editor. Nil for a kind means every
// line that is shown (the default, and how everyone started).
type PersonQualities struct {
	Movie []string `json:"movie"`
	Show  []string `json:"show"`
}

// ErrQualityNoneForPerson: a kind with a choice needs at least one line.
var ErrQualityNoneForPerson = errors.New("leave at least one quality this person can pick for movies and one for shows")

// knownLine says id can be a line: a built-in one or one the owner added.
func knownLine(id string) bool {
	return id == QualityStandard || id == QualityFourK || ownLine(id)
}

// cleanLines keeps the known ids once each, in order; nil stays nil.
func cleanLines(ids []string) []string {
	if ids == nil {
		return nil
	}
	out := []string{}
	for _, id := range ids {
		if knownLine(id) && !slices.Contains(out, id) && len(out) < qualityLinesMax {
			out = append(out, id)
		}
	}
	return out
}

// ReadPersonQualities reads one person's choice; anything unreadable is the
// default.
func ReadPersonQualities(ctx context.Context, db database.Querier, userID int64) (PersonQualities, error) {
	var v sql.NullString
	err := db.QueryRowContext(ctx, personQualitiesSQL, userID).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !v.Valid) {
		return PersonQualities{}, nil
	}
	if err != nil {
		return PersonQualities{}, err
	}
	var q PersonQualities
	if json.Unmarshal([]byte(v.String), &q) != nil {
		return PersonQualities{}, nil
	}
	return PersonQualities{Movie: cleanLines(q.Movie), Show: cleanLines(q.Show)}, nil
}

// SetPersonQualities saves one person's choice; nil kinds go back to every
// line. Weekly limits and alert targets stay as they are.
func SetPersonQualities(ctx context.Context, st *database.Store, userID int64, q PersonQualities) error {
	q = PersonQualities{Movie: cleanLines(q.Movie), Show: cleanLines(q.Show)}
	if (q.Movie != nil && len(q.Movie) == 0) || (q.Show != nil && len(q.Show) == 0) {
		return ErrQualityNoneForPerson
	}
	var v any
	if q.Movie != nil || q.Show != nil {
		b, _ := json.Marshal(q)
		v = string(b)
	}
	return st.Write(ctx, func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRow(setPersonLimitSQL, userID).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			return database.ErrNotFound
		}
		_, err := tx.Exec(setPersonQualitiesSQL, userID, v)
		return err
	})
}

// loadPersonQualities puts p's stored choice on p.
func loadPersonQualities(ctx context.Context, db database.Querier, p *Person) error {
	if p.UserID == 0 {
		return nil
	}
	q, err := ReadPersonQualities(ctx, db, p.UserID)
	if err != nil {
		return err
	}
	p.Qualities = &q
	return nil
}

// mayPick says p's choice allows line id of kind (no choice: every line).
func (p Person) mayPick(kind, id string) bool {
	if p.Qualities == nil {
		return true
	}
	ids := p.Qualities.Movie
	if kind == "show" {
		ids = p.Qualities.Show
	}
	return ids == nil || slices.Contains(ids, id)
}
