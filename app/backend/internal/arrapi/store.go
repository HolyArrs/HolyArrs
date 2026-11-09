package arrapi

import (
	"context"
	"database/sql"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// switchValue reads one of the two switches. It is "" when never saved.
func switchValue(ctx context.Context, st *database.Store, key string) string {
	v, _ := database.Setting(ctx, st.Read(), settingsGroup, key)
	return v
}

// saveSwitches writes every switch that is not nil in one transaction.
func saveSwitches(ctx context.Context, st *database.Store, who domain.Actor, values map[string]*bool) error {
	now := time.Now()
	return st.Write(ctx, func(tx *sql.Tx) error {
		for key, v := range values {
			if v == nil {
				continue
			}
			if err := database.PutSetting(tx, settingsGroup, key, map[bool]string{true: "on", false: "off"}[*v], who, now); err != nil {
				return err
			}
		}
		return nil
	})
}

// The tag table is the only table this package owns.

func listTags(ctx context.Context, q database.Lister) ([]tag, error) {
	rows, err := q.QueryContext(ctx, "SELECT id, label FROM compat_tags ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []tag{}
	for rows.Next() {
		var t tag
		if err := rows.Scan(&t.ID, &t.Label); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

// tagByLabel returns sql.ErrNoRows when no tag has the label, so a caller
// can tell a new label from a failed read.
func tagByLabel(tx *sql.Tx, label string) (tag, error) {
	var t tag
	err := tx.QueryRow("SELECT id, label FROM compat_tags WHERE label = ?", label).Scan(&t.ID, &t.Label)
	return t, err
}

func insertTag(tx *sql.Tx, label string, now time.Time) (int64, error) {
	res, err := tx.Exec("INSERT INTO compat_tags(label, created_at) VALUES (?, ?)", label, database.FormatTime(now))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// renameTag returns how many tags it renamed. Zero means the id is gone.
func renameTag(tx *sql.Tx, id int64, label string) (int64, error) {
	res, err := tx.Exec("UPDATE compat_tags SET label = ? WHERE id = ?", label, id)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
