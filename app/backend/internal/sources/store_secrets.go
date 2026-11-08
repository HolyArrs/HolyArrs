package sources

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
)

// extraSecretKeys are the extra fields that hold a secret wherever an older
// setup may have kept it in the clear.
var extraSecretKeys = []string{"apikey", "api_key", "password", "token"}

type plainSource struct {
	id    int64
	extra map[string]string
	ref   sql.NullInt64
}

// EncryptPlainExtras moves the keys an older setup left in the clear in
// Source extras into the vault. It is registered with secrets.RegisterRewriter
// from the program wiring and says how many Sources it changed.
func EncryptPlainExtras(tx *sql.Tx, v *secrets.Vault) (int, error) {
	srcs, err := plainSources(tx)
	if err != nil {
		return 0, err
	}
	moved := 0
	for _, x := range srcs {
		var plain string
		for _, k := range extraSecretKeys {
			if val, ok := x.extra[k]; ok {
				if plain == "" {
					plain = val
				}
				delete(x.extra, k)
			}
		}
		if plain == "" {
			continue
		}
		ref := x.ref.Int64
		if !x.ref.Valid {
			if ref, err = v.PutTx(tx, "source_api_key", plain, database.System); err != nil {
				return moved, err
			}
		}
		b, _ := json.Marshal(x.extra)
		if _, err := tx.Exec("UPDATE sources SET extra = ?, secret_ref = ?, updated_at = ? WHERE id = ?", string(b), ref, database.FormatTime(time.Now()), x.id); err != nil {
			return moved, err
		}
		moved++
	}
	return moved, nil
}

func plainSources(tx *sql.Tx) ([]plainSource, error) {
	rows, err := tx.Query("SELECT id, extra, secret_ref FROM sources")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []plainSource
	for rows.Next() {
		var x plainSource
		var raw string
		if err := rows.Scan(&x.id, &raw, &x.ref); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(raw), &x.extra)
		out = append(out, x)
	}
	return out, rows.Err()
}

// ReleaseIDs lists the ids of the Releases one Source found.
func ReleaseIDs(ctx context.Context, q database.Querier, sourceID int64) ([]int64, error) {
	var raw string
	if err := q.QueryRowContext(ctx, "SELECT coalesce(json_group_array(id), '[]') FROM releases WHERE source_id = ?", sourceID).Scan(&raw); err != nil {
		return nil, err
	}
	var ids []int64
	err := json.Unmarshal([]byte(raw), &ids)
	return ids, err
}
