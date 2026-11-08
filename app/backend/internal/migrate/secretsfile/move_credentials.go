package secretsfile

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
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

// SaveMoveCredential stores (or replaces) the credential for an old app.
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
	b, _ := json.Marshal(c)
	return st.Write(ctx, func(tx *sql.Tx) error {
		if err := dropMove(ctx, tx, kind); err != nil {
			return err
		}
		id, err := v.PutTx(tx, "move_credential", string(b), by)
		if err != nil {
			return err
		}
		ab, _ := json.Marshal(by)
		_, err = tx.ExecContext(ctx, "INSERT INTO move_credentials (kind, secret_id, created_at, created_by) VALUES (?, ?, ?, ?)",
			kind, id, database.FormatTime(time.Now()), string(ab))
		return err
	})
}

// ReadMoveCredential reads the credential for an old app without using
// it up.
func ReadMoveCredential(ctx context.Context, st *database.Store, v *secrets.Vault, kind string) (MoveCredential, bool, error) {
	c, _, found, err := ReadMoveCredentialVersion(ctx, st, v, kind)
	return c, found, err
}

type MoveCredentialVersion struct {
	SecretID  int64
	SealedSHA [32]byte
}

// ReadMoveCredentialVersion also returns the immutable vault row identity
// that a successful read must still own when it consumes the credential.
func ReadMoveCredentialVersion(ctx context.Context, st *database.Store, v *secrets.Vault, kind string) (MoveCredential, MoveCredentialVersion, bool, error) {
	var c MoveCredential
	if v == nil || !validMoveKind(kind) {
		return c, MoveCredentialVersion{}, false, nil
	}
	var id int64
	err := st.Read().QueryRowContext(ctx, "SELECT secret_id FROM move_credentials WHERE kind = ?", kind).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return c, MoveCredentialVersion{}, false, nil
	}
	if err != nil {
		return c, MoveCredentialVersion{}, false, err
	}
	raw, revision, err := v.GetVersion(ctx, id)
	if err != nil {
		return c, MoveCredentialVersion{}, false, err
	}
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return c, MoveCredentialVersion{}, false, errors.New("the saved credential could not be read")
	}
	address, err := moveAddress(c.Address)
	if err != nil {
		return MoveCredential{}, MoveCredentialVersion{}, false, err
	}
	c.Address = address
	return c, MoveCredentialVersion{SecretID: id, SealedSHA: revision}, true, nil
}

// ListMoveCredentials says which old apps have a saved credential, with
// their addresses; never a key.
func ListMoveCredentials(ctx context.Context, st *database.Store, v *secrets.Vault) ([]SavedMove, error) {
	out := []SavedMove{}
	for _, k := range MoveKinds {
		c, ok, err := ReadMoveCredential(ctx, st, v, k)
		if err != nil {
			return out, err
		}
		if ok {
			out = append(out, SavedMove{Kind: k, Address: c.Address, AllowLocalNetwork: c.AllowLocalNetwork})
		}
	}
	return out, nil
}

func dropMove(ctx context.Context, tx *sql.Tx, kind string) error {
	var id int64
	err := tx.QueryRowContext(ctx, "SELECT secret_id FROM move_credentials WHERE kind = ?", kind).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM secrets WHERE id = ?", id); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "DELETE FROM move_credentials WHERE kind = ?", kind)
	return err
}

// ForgetMoveCredentialTx consumes one saved credential in the same
// transaction that keeps its successful read.
var ErrMoveCredentialChanged = errors.New("the saved credential changed or was already used; read again with the current key")

func ForgetMoveCredentialTx(ctx context.Context, tx *sql.Tx, kind string, version MoveCredentialVersion) error {
	if !validMoveKind(kind) {
		return errors.New("unknown old app")
	}
	var sealed string
	err := tx.QueryRowContext(ctx, "SELECT s.value FROM move_credentials m JOIN secrets s ON s.id = m.secret_id WHERE m.kind = ? AND m.secret_id = ?", kind, version.SecretID).Scan(&sealed)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrMoveCredentialChanged
	}
	if err != nil {
		return err
	}
	if sha256.Sum256([]byte(sealed)) != version.SealedSHA {
		return ErrMoveCredentialChanged
	}
	res, err := tx.ExecContext(ctx, "DELETE FROM move_credentials WHERE kind = ? AND secret_id = ?", kind, version.SecretID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrMoveCredentialChanged
	}
	_, err = tx.ExecContext(ctx, "DELETE FROM secrets WHERE id = ?", version.SecretID)
	return err
}

// ForgetMoveCredential deletes one old app's credential (kind) or, with
// kind empty, all of them. It says how many it deleted.
func ForgetMoveCredential(ctx context.Context, st *database.Store, kind string) (int, error) {
	n := 0
	err := st.Write(ctx, func(tx *sql.Tx) error {
		for _, k := range MoveKinds {
			if kind != "" && k != kind {
				continue
			}
			var has int
			if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM move_credentials WHERE kind = ?", k).Scan(&has); err != nil {
				return err
			}
			if has == 0 {
				continue
			}
			if err := dropMove(ctx, tx, k); err != nil {
				return err
			}
			n++
		}
		return nil
	})
	return n, err
}
