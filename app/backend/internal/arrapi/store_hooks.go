package arrapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
)

// hook is one webhook record a side keeps for a request app.
type hook struct {
	ID        int64
	Side      string
	Name      string
	Method    string // POST or PUT
	OnImport  bool
	OnUpgrade bool
	Tags      []int64
	Body      string // the record as sent, without its secrets
	SecretID  *int64
}

// hookSecretKind is the vault kind of a hook's sealed part.
const hookSecretKind = "old_app_hook"

const hookColumns = "id, side, name, method, on_import, on_upgrade, tags, body, secret_id"

func scanHook(sc interface{ Scan(...any) error }) (hook, error) {
	var hk hook
	var tags string
	var secretID sql.NullInt64
	if err := sc.Scan(&hk.ID, &hk.Side, &hk.Name, &hk.Method, &hk.OnImport, &hk.OnUpgrade, &tags, &hk.Body, &secretID); err != nil {
		return hk, err
	}
	_ = json.Unmarshal([]byte(tags), &hk.Tags)
	if secretID.Valid {
		hk.SecretID = &secretID.Int64
	}
	return hk, nil
}

// listHookRows lists a side's hooks, oldest first.
func listHookRows(ctx context.Context, q database.Lister, side string) ([]hook, error) {
	rows, err := q.QueryContext(ctx, "SELECT "+hookColumns+" FROM compat_hooks WHERE side = ? ORDER BY id", side)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []hook{}
	for rows.Next() {
		hk, err := scanHook(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, hk)
	}
	return out, rows.Err()
}

// hookRow reads one of a side's hooks. A missing id, or one of the other
// side, gives database.ErrNotFound.
func hookRow(ctx context.Context, q database.Querier, side string, id int64) (hook, error) {
	hk, err := scanHook(q.QueryRowContext(ctx, "SELECT "+hookColumns+" FROM compat_hooks WHERE side = ? AND id = ?", side, id))
	if errors.Is(err, sql.ErrNoRows) {
		return hk, database.ErrNotFound
	}
	return hk, err
}

// hookNameTaken reports whether another hook of the side (not id) has the
// name, in any case.
func hookNameTaken(ctx context.Context, q database.Querier, side, name string, id int64) (bool, error) {
	var n int
	err := q.QueryRowContext(ctx, "SELECT count(*) FROM compat_hooks WHERE side = ? AND name = ? AND id != ?", side, name, id).Scan(&n)
	return n > 0, err
}

// countHookRows counts a side's hooks.
func countHookRows(ctx context.Context, q database.Querier, side string) (int, error) {
	var n int
	err := q.QueryRowContext(ctx, "SELECT count(*) FROM compat_hooks WHERE side = ?", side).Scan(&n)
	return n, err
}

func tagsJSON(ids []int64) string {
	if ids == nil {
		ids = []int64{}
	}
	b, _ := json.Marshal(ids)
	return string(b)
}

// saveHook stores a new hook (hk.ID 0) or replaces one in place, with its
// sealed part, in one write. A replaced hook's old sealed part is removed.
// It returns the hook's id.
func saveHook(ctx context.Context, st *database.Store, v *secrets.Vault, hk hook, sealed string, by domain.Actor) (int64, error) {
	now, who := database.FormatTime(time.Now()), actorJSON(by)
	id := hk.ID
	err := st.Write(ctx, func(tx *sql.Tx) error {
		secretID, err := v.PutTx(tx, hookSecretKind, sealed, by)
		if err != nil {
			return err
		}
		if id == 0 {
			res, err := tx.Exec(`INSERT INTO compat_hooks (side, name, method, on_import, on_upgrade, tags, body, secret_id, created_at, created_by, updated_at, updated_by)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, hk.Side, hk.Name, hk.Method, hk.OnImport, hk.OnUpgrade, tagsJSON(hk.Tags), hk.Body, secretID, now, who, now, who)
			if err != nil {
				return err
			}
			id, err = res.LastInsertId()
			return err
		}
		var old sql.NullInt64
		if err := tx.QueryRow("SELECT secret_id FROM compat_hooks WHERE side = ? AND id = ?", hk.Side, id).Scan(&old); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return database.ErrNotFound
			}
			return err
		}
		if _, err := tx.Exec(`UPDATE compat_hooks SET name = ?, method = ?, on_import = ?, on_upgrade = ?, tags = ?, body = ?, secret_id = ?, updated_at = ?, updated_by = ?
			WHERE side = ? AND id = ?`, hk.Name, hk.Method, hk.OnImport, hk.OnUpgrade, tagsJSON(hk.Tags), hk.Body, secretID, now, who, hk.Side, id); err != nil {
			return err
		}
		if old.Valid {
			return secrets.DeleteTx(ctx, tx, old.Int64)
		}
		return nil
	})
	return id, err
}

// deleteHookRow removes one of a side's hooks and its sealed part. It
// reports whether there was one.
func deleteHookRow(ctx context.Context, st *database.Store, side string, id int64) (bool, error) {
	found := false
	err := st.Write(ctx, func(tx *sql.Tx) error {
		var old sql.NullInt64
		err := tx.QueryRow("SELECT secret_id FROM compat_hooks WHERE side = ? AND id = ?", side, id).Scan(&old)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		found = true
		if _, err := tx.Exec("DELETE FROM compat_hooks WHERE side = ? AND id = ?", side, id); err != nil {
			return err
		}
		if old.Valid {
			return secrets.DeleteTx(ctx, tx, old.Int64)
		}
		return nil
	})
	return found, err
}

func actorJSON(a domain.Actor) string {
	b, _ := json.Marshal(a)
	return string(b)
}
