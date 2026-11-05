// Package secrets keeps passwords, API keys and tokens encrypted at rest with
// AES-256-GCM under a master key that lives outside the database.
package secrets

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// KeySize is the master key length in bytes.
const KeySize = 32

// KeyFile is the master key's name inside the config folder.
const KeyFile = "holyarrs.key"

// EnvKeyFile names a key file to use instead of the one in the config folder.
const EnvKeyFile = "HOLYARRS_MASTER_KEY_FILE"

var (
	// ErrNotFound means no secret has the id.
	ErrNotFound = errors.New("no such secret")
	// ErrWrongKey means the master key cannot open the stored secrets.
	ErrWrongKey = errors.New("the master key does not match this database; put back the key file that belongs to it")
	// ErrKeyMissing means the key file is gone but the database holds
	// secrets that only it can open, so no new key is made.
	ErrKeyMissing = errors.New("master key missing")
	// ErrKeyUnreadable means a key file is there (or named) but cannot be
	// used: no permission to read it, a named file that is missing, or a
	// file that holds no key.
	ErrKeyUnreadable = errors.New("master key file cannot be read")
	// ErrKeyIsFolder means the key path is a folder, often made by Docker
	// when a mounted host file did not exist.
	ErrKeyIsFolder = errors.New("the path is a folder, not a key file")
)

// LoadKey reads the master key. named, when set, is a key file the person
// supplied: it must exist. Otherwise the key file in dir is read, or made on
// first start with mode 0600.
func LoadKey(dir, named string) ([]byte, error) {
	if named != "" {
		b, err := os.ReadFile(named)
		if err != nil {
			return nil, fmt.Errorf("%w: %s (named in %s): %w", ErrKeyUnreadable, named, EnvKeyFile, folderErr(named, err))
		}
		return keyIn(named, b)
	}
	p := filepath.Join(dir, KeyFile)
	b, err := os.ReadFile(p)
	if err == nil {
		if st, err := os.Stat(p); err == nil && st.Mode().Perm()&0o077 != 0 {
			log.Printf("secrets: %s can be read by other users; on the host, run chown PUID:PGID and chmod 600 on it", p)
		}
		return keyIn(p, b)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s: %w", ErrKeyUnreadable, p, folderErr(p, err))
	}
	// A database that already holds secrets belongs to a key: a new one
	// could not open them, so the person must put the old key back.
	if saved, err := SavedIn(dir); err != nil {
		return nil, fmt.Errorf("master key: checking the database for saved secrets: %w", err)
	} else if saved {
		return nil, fmt.Errorf("%w: the master key file %s is missing, but the database already holds saved secrets that only that key can open; "+
			"put the key file (or your key kit renamed to %s) back in the config folder, or name it in %s", ErrKeyMissing, p, KeyFile, EnvKeyFile)
	}
	key := make([]byte, KeySize)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, err
	}
	_, werr := f.WriteString(base64.StdEncoding.EncodeToString(key) + "\n")
	if err := errors.Join(werr, f.Sync(), f.Close()); err != nil {
		return nil, err
	}
	log.Printf("secrets: made a new master key at %s; keep a copy, backups do not hold it", p)
	return key, nil
}

// Fingerprint is a short, safe label for a key: the first 8 hex digits of
// SHA-256 over its bytes. It cannot be turned back into the key.
func Fingerprint(key []byte) string {
	s := sha256.Sum256(key)
	return hex.EncodeToString(s[:])[:8]
}

// SavedIn reports whether the database in the config folder dir holds any
// secret, the check value included. A missing database holds none.
func SavedIn(dir string) (bool, error) {
	p := filepath.Join(dir, store.MainFile)
	if _, err := os.Stat(p); errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	db, err := store.OpenReadOnly(dir)
	if err != nil {
		return false, err
	}
	defer db.Close()
	var one int
	err = db.QueryRow("SELECT 1 FROM secrets LIMIT 1").Scan(&one)
	switch {
	case errors.Is(err, sql.ErrNoRows), err != nil && strings.Contains(err.Error(), "no such table"):
		return false, nil
	case err != nil:
		return false, err
	}
	return true, nil
}

// folderErr marks a read error on a path that is a folder.
func folderErr(p string, err error) error {
	if st, serr := os.Stat(p); serr == nil && st.IsDir() {
		return fmt.Errorf("%w: %w", ErrKeyIsFolder, err)
	}
	return err
}

func keyIn(p string, b []byte) ([]byte, error) {
	k, err := decodeKey(b)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrKeyUnreadable, p, err)
	}
	return k, nil
}

// decodeKey accepts 64 hex digits, base64 of 32 bytes or 32 raw bytes.
// Blank lines and lines starting with '#' are skipped, so a key kit with
// notes in it is a key file too.
func decodeKey(b []byte) ([]byte, error) {
	var keep []string
	for _, ln := range strings.Split(string(b), "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" || strings.HasPrefix(ln, "#") {
			continue
		}
		keep = append(keep, ln)
	}
	s := strings.Join(keep, "")
	if k, err := hex.DecodeString(s); err == nil && len(k) == KeySize {
		return k, nil
	}
	if k, err := base64.StdEncoding.DecodeString(s); err == nil && len(k) == KeySize {
		return k, nil
	}
	if len(b) == KeySize {
		return b, nil
	}
	return nil, errors.New("master key file must hold 32 bytes (64 hex digits or base64)")
}

// Vault reads and writes secrets.
type Vault struct {
	s    *store.Store
	aead cipher.AEAD
	// OnPut, when set, is told every value stored, before the store call
	// returns: the app log masks a new secret before anything can log it.
	OnPut func(plain string)
}

func (v *Vault) told(plain string) {
	if v.OnPut != nil && plain != "" {
		v.OnPut(plain)
	}
}

const (
	checkKind = "check"
	checkText = "holyarrs master key check"
)

// Open makes a Vault and checks the key against the database: the first
// open stores a check value, later opens must be able to read it.
func Open(ctx context.Context, s *store.Store, key []byte) (*Vault, error) {
	if len(key) != KeySize {
		return nil, errors.New("master key must be 32 bytes")
	}
	blk, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(blk)
	if err != nil {
		return nil, err
	}
	v := &Vault{s: s, aead: aead}
	var id int64
	err = s.Read().QueryRowContext(ctx, "SELECT id FROM secrets WHERE kind = ? ORDER BY id LIMIT 1", checkKind).Scan(&id)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if _, err := v.Put(ctx, checkKind, checkText); err != nil {
			return nil, err
		}
	case err != nil:
		return nil, err
	default:
		got, err := v.Get(ctx, id)
		if err != nil || got != checkText {
			return nil, ErrWrongKey
		}
	}
	return v, nil
}

func aad(id int64) []byte { return []byte("holyarrs-secret:" + strconv.FormatInt(id, 10)) }

func (v *Vault) seal(id int64, plain string) (string, error) {
	nonce := make([]byte, v.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	out := v.aead.Seal(nonce, nonce, []byte(plain), aad(id))
	return "v1:" + base64.StdEncoding.EncodeToString(out), nil
}

func (v *Vault) open(id int64, stored string) (string, error) {
	b64, ok := strings.CutPrefix(stored, "v1:")
	if !ok {
		return "", errors.New("unknown secret format")
	}
	b, err := base64.StdEncoding.DecodeString(b64)
	n := v.aead.NonceSize()
	if err != nil || len(b) < n {
		return "", errors.New("damaged secret")
	}
	plain, err := v.aead.Open(nil, b[:n], b[n:], aad(id))
	if err != nil {
		return "", ErrWrongKey
	}
	return string(plain), nil
}

func now() string { return store.FormatTime(time.Now()) }

func actor(a domain.Actor) string {
	b, _ := json.Marshal(a)
	return string(b)
}

// PutTx stores a new secret inside an open write and returns its id.
func (v *Vault) PutTx(tx *sql.Tx, kind, plain string, by domain.Actor) (int64, error) {
	t, who := now(), actor(by)
	res, err := tx.Exec("INSERT INTO secrets(created_at, created_by, updated_at, updated_by, kind, value) VALUES (?, ?, ?, ?, ?, ?)",
		t, who, t, who, kind, "")
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	v.told(plain)
	sealed, err := v.seal(id, plain)
	if err != nil {
		return 0, err
	}
	_, err = tx.Exec("UPDATE secrets SET value = ? WHERE id = ?", sealed, id)
	return id, err
}

// Put stores a new secret and returns its id.
func (v *Vault) Put(ctx context.Context, kind, plain string) (int64, error) {
	var id int64
	err := v.s.Write(ctx, func(tx *sql.Tx) error {
		var err error
		id, err = v.PutTx(tx, kind, plain, domain.System)
		return err
	})
	return id, err
}

// Get returns a secret in the clear. Callers must never log it.
func (v *Vault) Get(ctx context.Context, id int64) (string, error) {
	plain, _, err := v.GetVersion(ctx, id)
	return plain, err
}

// GetVersion reads a secret and the digest of that exact sealed value in
// one query. The digest can detect replacement even if SQLite reuses an id.
// Neither value may be logged or answered.
func (v *Vault) GetVersion(ctx context.Context, id int64) (string, [32]byte, error) {
	var stored string
	err := v.s.Read().QueryRowContext(ctx, "SELECT value FROM secrets WHERE id = ?", id).Scan(&stored)
	if errors.Is(err, sql.ErrNoRows) {
		return "", [32]byte{}, ErrNotFound
	}
	if err != nil {
		return "", [32]byte{}, err
	}
	plain, err := v.open(id, stored)
	return plain, sha256.Sum256([]byte(stored)), err
}

// Values returns every stored secret in the clear, for masking them in the
// app log only (internal/applog). Callers must never log or answer them.
func (v *Vault) Values(ctx context.Context) ([]string, error) {
	rows, err := v.s.Read().QueryContext(ctx, "SELECT id, value FROM secrets WHERE kind != ?", checkKind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id int64
		var stored string
		if err := rows.Scan(&id, &stored); err != nil {
			return nil, err
		}
		if plain, err := v.open(id, stored); err == nil && plain != "" {
			out = append(out, plain)
		}
	}
	return out, rows.Err()
}

// RetiredValues returns the old values of changed or removed secrets
// (kept sealed in secrets_retired by a trigger) still waiting for the log
// masker, in the clear, and the last row id read. For masking only: callers
// must never log or answer them.
func (v *Vault) RetiredValues(ctx context.Context) ([]string, int64, error) {
	rows, err := v.s.Read().QueryContext(ctx, "SELECT id, secret_id, value FROM secrets_retired ORDER BY id")
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []string
	var last int64
	for rows.Next() {
		var sid int64
		var stored string
		if err := rows.Scan(&last, &sid, &stored); err != nil {
			return nil, 0, err
		}
		if plain, err := v.open(sid, stored); err == nil && plain != "" {
			out = append(out, plain)
		}
	}
	return out, last, rows.Err()
}

// ForgetRetired drops the old values up to and including id, once the log
// files hold them masked.
func (v *Vault) ForgetRetired(ctx context.Context, upTo int64) error {
	return v.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("DELETE FROM secrets_retired WHERE id <= ?", upTo)
		return err
	})
}

// Set replaces a secret's value.
func (v *Vault) Set(ctx context.Context, id int64, plain string) error {
	v.told(plain)
	sealed, err := v.seal(id, plain)
	if err != nil {
		return err
	}
	return v.s.Write(ctx, func(tx *sql.Tx) error {
		res, err := tx.Exec("UPDATE secrets SET value = ?, updated_at = ? WHERE id = ?", sealed, now(), id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// Delete removes a secret.
func (v *Vault) Delete(ctx context.Context, id int64) error {
	return v.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("DELETE FROM secrets WHERE id = ?", id)
		return err
	})
}

// Masked is what an API response shows in place of a secret that is set.
const Masked = "••••••••"

// Mask is what an API response shows for a secret: never the value, only
// whether one is set.
func Mask(plain string) string {
	if plain == "" {
		return ""
	}
	return Masked
}

const settingRef = "secret:"

// SetSetting stores a secret setting, such as the TMDB token, encrypted. The
// settings row holds only a reference.
func (v *Vault) SetSetting(ctx context.Context, grp, key, plain string, by domain.Actor) error {
	return v.s.Write(ctx, func(tx *sql.Tx) error {
		var cur string
		err := tx.QueryRow("SELECT value FROM settings WHERE grp = ? AND key = ?", grp, key).Scan(&cur)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if ref, ok := strings.CutPrefix(cur, settingRef); ok {
			if id, err := strconv.ParseInt(ref, 10, 64); err == nil {
				v.told(plain)
				sealed, err := v.seal(id, plain)
				if err != nil {
					return err
				}
				_, err = tx.Exec("UPDATE secrets SET value = ?, updated_at = ?, updated_by = ? WHERE id = ?", sealed, now(), actor(by), id)
				return err
			}
		}
		id, err := v.PutTx(tx, "setting:"+grp+"/"+key, plain, by)
		if err != nil {
			return err
		}
		t, who := now(), actor(by)
		_, err = tx.Exec(`INSERT INTO settings(grp, key, value, created_at, created_by, updated_at, updated_by) VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(grp, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at, updated_by = excluded.updated_by`,
			grp, key, settingRef+strconv.FormatInt(id, 10), t, who, t, who)
		return err
	})
}

// Setting reads a secret setting; "" when it is not set.
func (v *Vault) Setting(ctx context.Context, grp, key string) (string, error) {
	var cur string
	err := v.s.Read().QueryRowContext(ctx, "SELECT value FROM settings WHERE grp = ? AND key = ?", grp, key).Scan(&cur)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	ref, ok := strings.CutPrefix(cur, settingRef)
	if !ok {
		return "", fmt.Errorf("setting %s/%s is not stored as a secret", grp, key)
	}
	id, err := strconv.ParseInt(ref, 10, 64)
	if err != nil {
		return "", err
	}
	return v.Get(ctx, id)
}

// Keys that hold secrets wherever an older setup may have kept them in the clear.
var (
	extraSecretKeys   = []string{"apikey", "api_key", "password", "token"}
	settingSecretKeys = map[string]bool{"tmdb_token": true}
)

type plainSource struct {
	id    int64
	extra map[string]string
	ref   sql.NullInt64
}

type plainSetting struct{ grp, key, val string }

// MigratePlaintext moves secrets an older setup stored in the clear (Source
// extras and secret settings) into the vault, and says how many it moved.
func (v *Vault) MigratePlaintext(ctx context.Context) (int, error) {
	moved := 0
	err := v.s.Write(ctx, func(tx *sql.Tx) error {
		moved = 0
		srcs, err := plainSources(tx)
		if err != nil {
			return err
		}
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
				if ref, err = v.PutTx(tx, "source_api_key", plain, domain.System); err != nil {
					return err
				}
			}
			b, _ := json.Marshal(x.extra)
			if _, err := tx.Exec("UPDATE sources SET extra = ?, secret_ref = ?, updated_at = ? WHERE id = ?", string(b), ref, now(), x.id); err != nil {
				return err
			}
			moved++
		}
		sets, err := plainSettings(tx)
		if err != nil {
			return err
		}
		for _, x := range sets {
			id, err := v.PutTx(tx, "setting:"+x.grp+"/"+x.key, x.val, domain.System)
			if err != nil {
				return err
			}
			if _, err := tx.Exec("UPDATE settings SET value = ?, updated_at = ? WHERE grp = ? AND key = ?",
				settingRef+strconv.FormatInt(id, 10), now(), x.grp, x.key); err != nil {
				return err
			}
			moved++
		}
		return nil
	})
	return moved, err
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

func plainSettings(tx *sql.Tx) ([]plainSetting, error) {
	rows, err := tx.Query("SELECT grp, key, value FROM settings")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []plainSetting
	for rows.Next() {
		var x plainSetting
		if err := rows.Scan(&x.grp, &x.key, &x.val); err != nil {
			return nil, err
		}
		if settingSecretKeys[x.key] && x.val != "" && !strings.HasPrefix(x.val, settingRef) {
			out = append(out, x)
		}
	}
	return out, rows.Err()
}

// CheckFile reports whether key opens the secrets in a database file, such
// as one inside a backup. A database without secrets passes.
func CheckFile(path string, key []byte) error {
	db, err := sql.Open("sqlite", store.FileURI(path)+"?mode=ro")
	if err != nil {
		return err
	}
	defer db.Close()
	var id int64
	var stored string
	err = db.QueryRow("SELECT id, value FROM secrets WHERE kind = ? ORDER BY id LIMIT 1", checkKind).Scan(&id, &stored)
	if errors.Is(err, sql.ErrNoRows) || (err != nil && strings.Contains(err.Error(), "no such table")) {
		return nil
	}
	if err != nil {
		return err
	}
	blk, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	aead, err := cipher.NewGCM(blk)
	if err != nil {
		return err
	}
	if got, err := (&Vault{aead: aead}).open(id, stored); err != nil || got != checkText {
		return ErrWrongKey
	}
	return nil
}
