package arrapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// pushedFormat is a format an app sent to a side, kept as sent.
type pushedFormat struct {
	ID      int64
	Side    string
	Name    string
	Body    string
	TraitID int64 // 0 when it became no trait
	Skipped string
	Updated string
}

// pushedProfile is a profile an app sent to a side, and the Pick rules it became.
type pushedProfile struct {
	PickRulesID int64
	Side        string
	Name        string
	Body        string
	Notes       []string
	Updated     string
}

const formatColumns = "id, side, name, body, trait_id, skipped, updated_at"

func scanFormat(sc interface{ Scan(...any) error }) (pushedFormat, error) {
	var f pushedFormat
	var trait sql.NullInt64
	err := sc.Scan(&f.ID, &f.Side, &f.Name, &f.Body, &trait, &f.Skipped, &f.Updated)
	f.TraitID = trait.Int64
	return f, err
}

// listFormatRows lists a side's pushed formats, oldest first.
func listFormatRows(ctx context.Context, q database.Lister, side string) ([]pushedFormat, error) {
	rows, err := q.QueryContext(ctx, "SELECT "+formatColumns+" FROM compat_formats WHERE side = ? ORDER BY id", side)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []pushedFormat{}
	for rows.Next() {
		f, err := scanFormat(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// formatRow reads one of a side's pushed formats. A missing id, or one of
// the other side, gives database.ErrNotFound.
func formatRow(ctx context.Context, q database.Querier, side string, id int64) (pushedFormat, error) {
	f, err := scanFormat(q.QueryRowContext(ctx, "SELECT "+formatColumns+" FROM compat_formats WHERE side = ? AND id = ?", side, id))
	if errors.Is(err, sql.ErrNoRows) {
		return f, database.ErrNotFound
	}
	return f, err
}

// formatNameTaken reports whether another pushed format of the side (not
// id) has exactly this name.
func formatNameTaken(ctx context.Context, q database.Querier, side, name string, id int64) (bool, error) {
	var n int
	err := q.QueryRowContext(ctx, "SELECT count(*) FROM compat_formats WHERE side = ? AND name = ? AND id != ?", side, name, id).Scan(&n)
	return n > 0, err
}

func nullID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}

// saveFormatRow stores a new pushed format (f.ID 0) or replaces one in
// place, and returns its id.
func saveFormatRow(ctx context.Context, st *database.Store, f pushedFormat, by domain.Actor) (int64, error) {
	now, who := database.FormatTime(time.Now()), actorJSON(by)
	id := f.ID
	err := st.Write(ctx, func(tx *sql.Tx) error {
		if id == 0 {
			res, err := tx.Exec(`INSERT INTO compat_formats (side, name, body, trait_id, skipped, created_at, created_by, updated_at, updated_by)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, f.Side, f.Name, f.Body, nullID(f.TraitID), f.Skipped, now, who, now, who)
			if err != nil {
				return err
			}
			id, err = res.LastInsertId()
			return err
		}
		res, err := tx.Exec(`UPDATE compat_formats SET name = ?, body = ?, trait_id = ?, skipped = ?, updated_at = ?, updated_by = ? WHERE side = ? AND id = ?`,
			f.Name, f.Body, nullID(f.TraitID), f.Skipped, now, who, f.Side, id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return database.ErrNotFound
		}
		return nil
	})
	return id, err
}

// setFormatTrait records the trait a stored format became.
func setFormatTrait(ctx context.Context, st *database.Store, side string, id, trait int64) error {
	return st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE compat_formats SET trait_id = ? WHERE side = ? AND id = ?", nullID(trait), side, id)
		return err
	})
}

// deleteFormatRow removes one of a side's pushed formats.
func deleteFormatRow(ctx context.Context, st *database.Store, side string, id int64) error {
	return st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("DELETE FROM compat_formats WHERE side = ? AND id = ?", side, id)
		return err
	})
}

const profileColumns = "pick_rules_id, side, name, body, notes, updated_at"

func scanProfile(sc interface{ Scan(...any) error }) (pushedProfile, error) {
	var p pushedProfile
	var notes string
	err := sc.Scan(&p.PickRulesID, &p.Side, &p.Name, &p.Body, &notes, &p.Updated)
	_ = json.Unmarshal([]byte(notes), &p.Notes)
	return p, err
}

// listProfileRows lists the pushed profiles of both sides by Pick rules id.
func listProfileRows(ctx context.Context, q database.Lister) (map[int64]pushedProfile, error) {
	rows, err := q.QueryContext(ctx, "SELECT "+profileColumns+" FROM compat_profiles")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]pushedProfile{}
	for rows.Next() {
		p, err := scanProfile(rows)
		if err != nil {
			return nil, err
		}
		out[p.PickRulesID] = p
	}
	return out, rows.Err()
}

// profileRow reads the pushed profile of these Pick rules, or
// database.ErrNotFound when no app sent them.
func profileRow(ctx context.Context, q database.Querier, id int64) (pushedProfile, error) {
	p, err := scanProfile(q.QueryRowContext(ctx, "SELECT "+profileColumns+" FROM compat_profiles WHERE pick_rules_id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return p, database.ErrNotFound
	}
	return p, err
}

// saveProfileRow stores or replaces the pushed profile of p.PickRulesID.
func saveProfileRow(ctx context.Context, st *database.Store, p pushedProfile, by domain.Actor) error {
	now, who := database.FormatTime(time.Now()), actorJSON(by)
	notes, _ := json.Marshal(append([]string{}, p.Notes...))
	return st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO compat_profiles (pick_rules_id, side, name, body, notes, created_at, created_by, updated_at, updated_by)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (pick_rules_id) DO UPDATE SET name = excluded.name, body = excluded.body, notes = excluded.notes,
				updated_at = excluded.updated_at, updated_by = excluded.updated_by`,
			p.PickRulesID, p.Side, p.Name, p.Body, string(notes), now, who, now, who)
		return err
	})
}
