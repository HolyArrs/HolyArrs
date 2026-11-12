package arrapi

import (
	"context"
	"database/sql"
	"encoding/json"
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

// The tag tables: the tags those apps make and which titles carry them.

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

// deleteTag removes a tag and takes it off every title. It returns how
// many tags it removed; zero means the id is gone.
func deleteTag(tx *sql.Tx, id int64) (int64, error) {
	if _, err := tx.Exec("DELETE FROM compat_title_tags WHERE tag_id = ?", id); err != nil {
		return 0, err
	}
	res, err := tx.Exec("DELETE FROM compat_tags WHERE id = ?", id)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// tagLink is one title carrying one tag.
type tagLink struct{ tagID, titleID int64 }

func scanLinks(rows *sql.Rows, err error) ([]tagLink, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []tagLink
	for rows.Next() {
		var l tagLink
		if err := rows.Scan(&l.tagID, &l.titleID); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// allTagLinks lists every link, by tag and then title.
func allTagLinks(ctx context.Context, q database.Lister) ([]tagLink, error) {
	return scanLinks(q.QueryContext(ctx, "SELECT tag_id, title_id FROM compat_title_tags ORDER BY tag_id, title_id"))
}

// tagsOf maps each of the titles to the ids of its tags, lowest first.
func tagsOf(ctx context.Context, q database.Lister, titleIDs []int64) (map[int64][]int64, error) {
	out := map[int64][]int64{}
	if len(titleIDs) == 0 {
		return out, nil
	}
	ids, _ := json.Marshal(titleIDs)
	ls, err := scanLinks(q.QueryContext(ctx, "SELECT tag_id, title_id FROM compat_title_tags WHERE title_id IN (SELECT value FROM json_each(?)) ORDER BY title_id, tag_id", string(ids)))
	for _, l := range ls {
		out[l.titleID] = append(out[l.titleID], l.tagID)
	}
	return out, err
}

// tagLabelsOf lists the labels of a title's tags, lowest id first.
func tagLabelsOf(ctx context.Context, q database.Lister, titleID int64) ([]string, error) {
	rows, err := q.QueryContext(ctx, "SELECT t.label FROM compat_tags t JOIN compat_title_tags l ON l.tag_id = t.id WHERE l.title_id = ? ORDER BY t.id", titleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// linkTags puts the tags with these ids on a title. Ids no tag has are
// skipped, as are links already there.
func linkTags(ctx context.Context, st *database.Store, titleID int64, tagIDs []int64) error {
	if len(tagIDs) == 0 {
		return nil
	}
	ids, _ := json.Marshal(tagIDs)
	return st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("INSERT OR IGNORE INTO compat_title_tags (tag_id, title_id) SELECT id, ? FROM compat_tags WHERE id IN (SELECT value FROM json_each(?))", titleID, string(ids))
		return err
	})
}
