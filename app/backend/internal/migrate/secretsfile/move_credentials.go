package secretsfile

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
)

// Move credentials live in the move_credentials table: an old app's
// address and key (qBittorrent: or its sign-in), saved from the secrets
// file or by a successful read with a typed key, as one vault secret
// each. They are kept for "Read again" until a move finishes.

// MoveKinds are the old apps a credential may be saved for.
var MoveKinds = []string{"radarr", "sonarr", "lidarr", "prowlarr", "bazarr", "seerr", "overseerr", "qbittorrent", "sabnzbd"}

// MoveCredential is one old app's address and key (or sign-in).
type MoveCredential struct {
	Address           string `json:"address"`
	APIKey            string `json:"api_key"`
	Username          string `json:"username,omitempty"`
	Password          string `json:"password,omitempty"`
	AllowLocalNetwork bool   `json:"allow_local_network"`
}

// SavedMove is what the Move screen is told about a saved credential:
// never the key.
type SavedMove struct {
	Kind              string `json:"kind"`
	Address           string `json:"address"`
	AllowLocalNetwork bool   `json:"allow_local_network"`
}

func validMoveKind(k string) bool {
	for _, x := range MoveKinds {
		if x == k {
			return true
		}
	}
	return false
}

// moveAddress accepts only an old app's plain base address. Credentials
// belong in the vault fields, never in an address returned to the screen.
func moveAddress(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" ||
		u.User != nil || u.Opaque != "" || u.RawQuery != "" || u.ForceQuery || strings.Contains(raw, "#") {
		return "", errors.New("the old app address must be http or https, with no user, password, query or fragment")
	}
	u.Host = strings.ToLower(u.Host)
	return strings.TrimRight(u.String(), "/"), nil
}

// SaveMoveCredential stores (or replaces) the credential for an old app
// at its address; other addresses of the same app keep theirs.
func SaveMoveCredential(ctx context.Context, st *database.Store, v *secrets.Vault, kind string, c MoveCredential, by domain.Actor) error {
	if !validMoveKind(kind) {
		return errors.New("unknown old app")
	}
	if v == nil {
		return errors.New("the secrets vault is not open")
	}
	address, err := moveAddress(c.Address)
	if err != nil {
		return err
	}
	c.Address = address
	_, old, found, err := findMove(ctx, st.Read(), v, kind, address)
	if err != nil {
		return err
	}
	b, _ := json.Marshal(c)
	return st.Write(ctx, func(tx *sql.Tx) error {
		if found {
			if err := dropMove(ctx, tx, kind, old); err != nil {
				return err
			}
		}
		if err := dropMove(ctx, tx, kind, moveRow{address: address}); err != nil {
			return err
		}
		id, err := v.PutTx(tx, "move_credential", string(b), by)
		if err != nil {
			return err
		}
		ab, _ := json.Marshal(by)
		return insertMoveCredential(ctx, tx, kind, address, id, database.FormatTime(time.Now()), string(ab))
	})
}

// ReadMoveCredential reads the newest credential for an old app without
// using it up.
func ReadMoveCredential(ctx context.Context, st *database.Store, v *secrets.Vault, kind string) (MoveCredential, bool, error) {
	c, _, found, err := ReadMoveCredentialAt(ctx, st, v, kind, "")
	return c, found, err
}

// MoveCredentialVersion pins the stored secret a credential was read from, so a
// later write can tell whether it changed in between.
type MoveCredentialVersion struct {
	SecretID int64
}

// ReadMoveCredentialVersion reads the newest credential for an old app,
// with the immutable vault row identity it was read from.
func ReadMoveCredentialVersion(ctx context.Context, st *database.Store, v *secrets.Vault, kind string) (MoveCredential, MoveCredentialVersion, bool, error) {
	return ReadMoveCredentialAt(ctx, st, v, kind, "")
}

// ReadMoveCredentialAt reads the credential saved for one address of an
// old app; address "" means the newest one.
func ReadMoveCredentialAt(ctx context.Context, st *database.Store, v *secrets.Vault, kind, address string) (MoveCredential, MoveCredentialVersion, bool, error) {
	if v == nil || !validMoveKind(kind) {
		return MoveCredential{}, MoveCredentialVersion{}, false, nil
	}
	if address != "" {
		a, err := moveAddress(address)
		if err != nil {
			return MoveCredential{}, MoveCredentialVersion{}, false, nil
		}
		address = a
	}
	c, row, found, err := findMove(ctx, st.Read(), v, kind, address)
	if err != nil || !found {
		return MoveCredential{}, MoveCredentialVersion{}, false, err
	}
	return c, MoveCredentialVersion{SecretID: row.secretID}, true, nil
}

// findMove finds the credential for kind at address ("" for the newest).
// A row saved before keys were kept per address is matched by the address
// sealed inside it.
func findMove(ctx context.Context, q rowsQuerier, v *secrets.Vault, kind, address string) (MoveCredential, moveRow, bool, error) {
	rows, err := moveRows(ctx, q, kind)
	if err != nil {
		return MoveCredential{}, moveRow{}, false, err
	}
	for _, row := range rows {
		if address != "" && row.address != "" && row.address != address {
			continue
		}
		c, err := openMove(ctx, v, row)
		if err != nil {
			return MoveCredential{}, moveRow{}, false, err
		}
		if address == "" || c.Address == address {
			return c, row, true, nil
		}
	}
	return MoveCredential{}, moveRow{}, false, nil
}

func openMove(ctx context.Context, v *secrets.Vault, row moveRow) (MoveCredential, error) {
	var c MoveCredential
	raw, _, err := v.GetVersion(ctx, row.secretID)
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return c, errors.New("the saved credential could not be read")
	}
	address, err := moveAddress(c.Address)
	if err != nil {
		return MoveCredential{}, err
	}
	c.Address = address
	return c, nil
}

// ListMoveCredentials says which old apps have a saved credential, one
// line per address; never a key.
func ListMoveCredentials(ctx context.Context, st *database.Store, v *secrets.Vault) ([]SavedMove, error) {
	out := []SavedMove{}
	if v == nil {
		return out, nil
	}
	for _, k := range MoveKinds {
		rows, err := moveRows(ctx, st.Read(), k)
		if err != nil {
			return out, err
		}
		for _, row := range rows {
			c, err := openMove(ctx, v, row)
			if err != nil {
				return out, err
			}
			out = append(out, SavedMove{Kind: k, Address: c.Address, AllowLocalNetwork: c.AllowLocalNetwork})
		}
	}
	return out, nil
}

// dropMove deletes one saved row and its secret; a row not there is fine.
func dropMove(ctx context.Context, tx *sql.Tx, kind string, row moveRow) error {
	rows, err := moveRows(ctx, tx, kind)
	if err != nil {
		return err
	}
	for _, r := range rows {
		if r.address != row.address {
			continue
		}
		if err := secrets.DeleteTx(ctx, tx, r.secretID); err != nil {
			return err
		}
		return deleteMoveCredential(ctx, tx, kind, r.address)
	}
	return nil
}

// ForgetMoveCredential deletes one old app's credentials (kind) or, with
// kind empty, all of them. It says how many it deleted.
func ForgetMoveCredential(ctx context.Context, st *database.Store, kind string) (int, error) {
	n := 0
	err := st.Write(ctx, func(tx *sql.Tx) error {
		for _, k := range MoveKinds {
			if kind != "" && k != kind {
				continue
			}
			rows, err := moveRows(ctx, tx, k)
			if err != nil {
				return err
			}
			for _, row := range rows {
				if err := dropMove(ctx, tx, k, row); err != nil {
					return err
				}
			}
			if len(rows) > 0 {
				n++
			}
		}
		return nil
	})
	return n, err
}

// ForgetMoveCredentialAt deletes the credential kept for one address of
// an old app. It says whether one was deleted.
func ForgetMoveCredentialAt(ctx context.Context, st *database.Store, v *secrets.Vault, kind, address string) (bool, error) {
	if v == nil || !validMoveKind(kind) || address == "" {
		return false, nil
	}
	a, err := moveAddress(address)
	if err != nil {
		return false, nil
	}
	_, row, found, err := findMove(ctx, st.Read(), v, kind, a)
	if err != nil || !found {
		return false, err
	}
	return true, st.Write(ctx, func(tx *sql.Tx) error { return dropMove(ctx, tx, kind, row) })
}
