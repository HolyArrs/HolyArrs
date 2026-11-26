package sources

import (
	"context"
	"database/sql"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
)

// putSecret replaces the secret at ref with plain, and does nothing when plain is empty.
func putSecret(tx *sql.Tx, v *secrets.Vault, ref **int64, kind, plain string, by domain.Actor) error {
	if plain == "" {
		return nil
	}
	if v == nil {
		return ErrNoVault
	}
	if *ref != nil {
		if err := secrets.DeleteTx(context.Background(), tx, **ref); err != nil {
			return err
		}
	}
	id, err := v.PutTx(tx, kind, plain, by)
	*ref = &id
	return err
}

func dropSecret(tx *sql.Tx, ref **int64) error {
	if *ref == nil {
		return nil
	}
	err := secrets.DeleteTx(context.Background(), tx, **ref)
	*ref = nil
	return err
}
