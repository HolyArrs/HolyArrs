package accounts

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/startup"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// The variables that set up the owner and an API key at start, for installs
// made by a tool (NixOS, Ansible, Kubernetes) with nobody to open the wizard.
const (
	EnvOwnerName     = "HOLYARRS_OWNER_NAME"
	EnvOwnerPassword = "HOLYARRS_OWNER_PASSWORD_FILE"
	EnvAPIKey        = "HOLYARRS_API_KEY_FILE"
	EnvAPIKeyScopes  = "HOLYARRS_API_KEY_SCOPES"
)

// envKeyLabel marks the key EnvAPIKey holds. A person never makes a key as
// the system, so the label and the system as maker find it again on the next
// start without a column of its own.
const envKeyLabel = "Set by " + EnvAPIKey

// keyRevokedSetting lists the hashes of keys from EnvAPIKey that a person
// deleted, so a start never brings a revoked key back.
const keyRevokedSetting = "revoked_env_api_keys"

// maxRevoked caps that list; older hashes drop off first. Each entry needs a
// person to remove the file's key and someone to change the file after, so
// a hundred is years of turnover; only a hash this old can come back.
const maxRevoked = 100

// maxValueFile caps a value file, as for the single sign-on client secret.
const maxValueFile = 4096

// keyRecipe makes a key EnvAPIKey accepts: 32 random bytes in URL-safe
// base64 with no padding, the shape of a key made in HolyArrs.
const keyRecipe = `echo "hak_$(openssl rand -base64 32 | tr '+/' '-_' | tr -d '=')"`

// keyBytes is how many random bytes a key made in HolyArrs holds (token).
const keyBytes = 32

// minKeyChars is the fewest different characters a key may use. Ten
// thousand made keys in the tests never come near it.
const minKeyChars = 16

// systemActor is the system as stored in created_by.
var systemActor = func() string { b, _ := json.Marshal(domain.System); return string(b) }()

// EnvSetup is the owner and the API key the environment asks for.
type EnvSetup struct {
	ownerName    string
	passwordPath string // read only when an owner has to be made
	key          string
	scopes       []string
}

// EnvSetupError is a start refused over the owner or API key variables, for
// a person to fix. Public is the reason for a page anyone can open: the error
// with file paths left out. No message here holds a password or a key.
type EnvSetupError struct {
	Public string
	err    error
}

func (e *EnvSetupError) Error() string { return e.err.Error() }

func (e *EnvSetupError) Unwrap() error { return e.err }

// refused marks err as one a person fixes, with paths left out of Public:
// quoted, as this file writes them, and bare, as an os error would.
func refused(err error, paths ...string) error {
	if err == nil {
		return nil
	}
	pub := err.Error()
	for _, p := range paths {
		if p != "" {
			pub = strings.ReplaceAll(pub, strconv.Quote(p), "named there")
			pub = strings.ReplaceAll(pub, p, "named there")
		}
	}
	return &EnvSetupError{Public: pub, err: err}
}

// ReadEnvSetup reads and checks the owner and API key variables. Errors name
// the variable and the file, never the value, and are an *EnvSetupError.
func ReadEnvSetup() (EnvSetup, error) {
	e, err := readEnvSetup()
	return e, refused(err, os.Getenv(EnvOwnerPassword), os.Getenv(EnvAPIKey))
}

func readEnvSetup() (EnvSetup, error) {
	var e EnvSetup
	e.ownerName = strings.TrimSpace(os.Getenv(EnvOwnerName))
	pwPath := os.Getenv(EnvOwnerPassword)
	if (e.ownerName == "") != (pwPath == "") {
		return EnvSetup{}, fmt.Errorf("set both %s and %s, or neither", EnvOwnerName, EnvOwnerPassword)
	}
	// The password file is read only when there is no owner yet, so it may
	// be removed once the first start made the owner.
	e.passwordPath = pwPath
	keyPath, scopes := os.Getenv(EnvAPIKey), os.Getenv(EnvAPIKeyScopes)
	if keyPath == "" {
		if strings.TrimSpace(scopes) != "" {
			return EnvSetup{}, fmt.Errorf("%s is set without %s", EnvAPIKeyScopes, EnvAPIKey)
		}
		return e, nil
	}
	k, err := readValueFile(EnvAPIKey, keyPath)
	if err != nil {
		return EnvSetup{}, err
	}
	if msg := envKeyProblem(k); msg != "" {
		return EnvSetup{}, fmt.Errorf("%s: %s", EnvAPIKey, msg)
	}
	e.key = k
	if e.scopes, err = parseEnvScopes(scopes); err != nil {
		return EnvSetup{}, err
	}
	return e, nil
}

// readValueFile reads a secret from the file a variable names, with the
// newline rule of FILE__ variables. Only a regular file is read: a pipe or a
// terminal would hold the start forever (openRegular).
func readValueFile(name, path string) (string, error) {
	cannot := func(err error) error {
		var pe *fs.PathError
		if errors.As(err, &pe) {
			err = pe.Err
		}
		return fmt.Errorf("%s: cannot read the file %q: %w", name, path, err)
	}
	f, err := openRegular(path)
	if errors.Is(err, errNotRegular) {
		return "", fmt.Errorf("%s: the file %q is not a regular file", name, path)
	}
	if err != nil {
		return "", cannot(err)
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxValueFile+1))
	if err != nil {
		return "", cannot(err)
	}
	if len(b) > maxValueFile {
		return "", fmt.Errorf("%s: the file %q is over %d bytes", name, path, maxValueFile)
	}
	v := startup.FileValue(b)
	if v == "" {
		return "", fmt.Errorf("%s: the file %q is empty", name, path)
	}
	return v, nil
}

// errNotRegular is a value file that is a pipe, a device or a folder.
var errNotRegular = errors.New("not a regular file")

// envKeyProblem says what is wrong with a key from a file, or "". The key
// must have the exact shape of a key HolyArrs makes: keyPrefix and 32 bytes
// in URL-safe base64 with no padding. The rest is a sanity check that refuses
// placeholders: a phrase in base64, a repeating pattern, few different
// characters, or no mix of
// capitals and small letters (a made key lacks one with odds of fewer than
// one in a billion). None of it tests randomness; the recipe gives that.
func envKeyProblem(k string) string {
	bad := "the key needs " + keyPrefix + " and then exactly 43 characters from A-Z, a-z, 0-9, - and _ (32 random bytes in URL-safe base64 with no padding), as HolyArrs makes them; make one with: " + keyRecipe
	body, ok := strings.CutPrefix(k, keyPrefix)
	if !ok || len(body) != base64.RawURLEncoding.EncodedLen(keyBytes) {
		return bad
	}
	b, err := base64.RawURLEncoding.Strict().DecodeString(body)
	if err != nil || len(b) != keyBytes || printable(b) {
		return bad
	}
	if repeats(body) || distinct(body) < minKeyChars ||
		!strings.ContainsFunc(body, unicode.IsUpper) || !strings.ContainsFunc(body, unicode.IsLower) {
		return bad
	}
	return ""
}

// printable reports whether every byte is printable ASCII: a phrase in
// base64, not random bytes (those are all printable with odds near 1e-14).
func printable(b []byte) bool {
	for _, c := range b {
		if c < 0x20 || c > 0x7e {
			return false
		}
	}
	return true
}

// repeats reports whether s is a pattern repeated: s[i] == s[i+p] for every i,
// for some period p up to half its length.
func repeats(s string) bool {
	for p := 1; p <= len(s)/2; p++ {
		if s[p:] == s[:len(s)-p] {
			return true
		}
	}
	return false
}

// distinct counts the different characters in s.
func distinct(s string) int {
	seen := map[rune]bool{}
	for _, c := range s {
		seen[c] = true
	}
	return len(seen)
}

// parseEnvScopes reads EnvAPIKeyScopes with the rules of a key made in
// HolyArrs; all when empty.
func parseEnvScopes(s string) ([]string, error) {
	var out []string
	for sc := range strings.SplitSeq(s, ",") {
		if sc = strings.TrimSpace(sc); sc != "" {
			out = append(out, sc)
		}
	}
	if len(out) == 0 {
		out = []string{webserver.ScopeAll}
	}
	for _, sc := range out {
		if msg := scopeProblem(sc); msg != "" {
			return nil, fmt.Errorf("%s: %s", EnvAPIKeyScopes, msg)
		}
	}
	if mixesMCP(out) {
		return nil, fmt.Errorf("%s: %s", EnvAPIKeyScopes, errMixedMCP)
	}
	return slices.Compact(slices.Sorted(slices.Values(out))), nil
}

// errNoOwnerForKey stops the start: going on would leave a fresh install for
// the first visitor to claim while its operator thinks it is set up.
var errNoOwnerForKey = errors.New(EnvAPIKey + " is set but there is no owner yet: set " + EnvOwnerName + " and " +
	EnvOwnerPassword + ", or finish setup first")

// errKeyTaken is a key file holding a key made in HolyArrs, which belongs to
// whoever made it.
var errKeyTaken = errors.New(EnvAPIKey + " holds a key that was made in HolyArrs; put a new key in the file. Make one with: " + keyRecipe)

// errKeyRevoked is a key file holding a key a person removed, by deleting it
// or by changing the owner's password: they may know it leaked, so it never
// comes back on its own.
var errKeyRevoked = errors.New("the key in " + EnvAPIKey + " was removed (deleted, or the owner's password changed); put a new key in the file. Make one with: " + keyRecipe)

// envDone is what one start changed, for the log and the audit trail.
type envDone struct {
	ownerID, kept, made int64
	ownerExisted        bool
	rescoped            bool
	gone                []int64
}

// ApplyEnvSetup makes the owner and the API key ReadEnvSetup read, in one
// write, so a refused key leaves no owner half made. It runs before the
// routes are served, so nobody can make the owner first through the wizard.
// An owner who exists keeps their name and password. An error a person must
// fix is an *EnvSetupError; a database error is not, so a restart may cure it.
func (a *Auth) ApplyEnvSetup(ctx context.Context, e EnvSetup) error {
	if e.ownerName == "" && e.key == "" {
		return nil
	}
	// Any owner row counts, so an install that has one never pays for, or
	// fails over, a password it does not use.
	n, err := countOwners(ctx, a.s.Read())
	if err != nil {
		return err
	}
	makeOwner := e.ownerName != "" && n == 0
	var hash string
	if makeOwner {
		if msg := nameProblem(e.ownerName); msg != "" {
			return refused(fmt.Errorf("%s: %s", EnvOwnerName, msg))
		}
		pw, err := readValueFile(EnvOwnerPassword, e.passwordPath)
		if err != nil {
			return refused(err, e.passwordPath)
		}
		// A byte order mark some Windows editors add, as reset-owner drops it.
		pw = strings.TrimPrefix(pw, string(rune(0xfeff)))
		if msg := passwordProblem(pw, MinPasswordLength(ctx, a.s.Read())); msg != "" {
			return refused(fmt.Errorf("%s: %s", EnvOwnerPassword, msg))
		}
		if hash, err = hashPassword(pw, a.hash); err != nil {
			return err
		}
	}
	now := a.now()
	var d envDone
	err = a.s.WriteCore(ctx, func(tx *sql.Tx) error {
		d = envDone{ownerExisted: e.ownerName != "" && !makeOwner}
		if makeOwner {
			id, err := insertFirstOwner(ctx, tx, e.ownerName, hash)
			switch {
			case errors.Is(err, errOwnerExists):
				d.ownerExisted = true
			case err != nil:
				return err
			default:
				d.ownerID = id
			}
		}
		if e.key == "" {
			return nil
		}
		return envKeyTx(ctx, tx, e, database.FormatTime(now), &d)
	})
	if err != nil {
		if errors.Is(err, errNoOwnerForKey) || errors.Is(err, errKeyTaken) || errors.Is(err, errKeyRevoked) || errors.Is(err, errRevokedUnreadable) {
			return refused(err)
		}
		return fmt.Errorf("setting up from %s and %s: %w", EnvOwnerName, EnvAPIKey, err)
	}
	a.reportEnv(ctx, e, d)
	return nil
}

// envKeyTx makes the owner hold exactly one key from EnvAPIKey: kept when the
// file is the same, its scopes brought in line, and replaced when the file
// holds a new key.
func envKeyTx(ctx context.Context, tx *sql.Tx, e EnvSetup, now string, d *envDone) error {
	scopeJSON, _ := json.Marshal(e.scopes)
	want := sum(e.key)
	revoked, err := revokedKeys(ctx, tx)
	if err != nil {
		return err
	}
	if slices.Contains(revoked, want) {
		return errKeyRevoked
	}
	owner, err := firstOwnerID(ctx, tx)
	if errors.Is(err, sql.ErrNoRows) {
		return errNoOwnerForKey
	}
	if err != nil {
		return err
	}
	keys, err := madeKeys(ctx, tx, owner, envKeyLabel, systemActor)
	if err != nil {
		return err
	}
	for _, k := range keys {
		if k.hash == want && d.kept == 0 {
			d.kept = k.id
			if k.scope != string(scopeJSON) {
				d.rescoped = true
				if err := setKeyScope(ctx, tx, k.id, string(scopeJSON), now, systemActor); err != nil {
					return err
				}
			}
			continue
		}
		if _, err := deleteKey(ctx, tx, k.id, true, owner); err != nil {
			return err
		}
		d.gone = append(d.gone, k.id)
	}
	if d.kept != 0 {
		return nil
	}
	if used, err := keyHashUsed(ctx, tx, want); err != nil || used {
		if err == nil {
			err = errKeyTaken
		}
		return err
	}
	d.made, err = insertKey(ctx, tx, now, systemActor, owner, envKeyLabel, string(scopeJSON), "", want, e.key[len(e.key)-4:])
	return err
}

// reportEnv logs and records what a start changed.
func (a *Auth) reportEnv(ctx context.Context, e EnvSetup, d envDone) {
	if d.ownerExisted {
		log.Printf("auth: the owner exists; %s and %s change nothing", EnvOwnerName, EnvOwnerPassword)
	}
	if d.ownerID != 0 {
		a.systemEvent(ctx, EventAccountChanged, "Owner account made from "+EnvOwnerName+" at start",
			map[string]any{"user_id": d.ownerID, "action": "owner_created_from_env", "name": e.ownerName})
		log.Printf("auth: first-run setup made the owner %q from %s", e.ownerName, EnvOwnerName)
	}
	if e.key == "" {
		return
	}
	scopes := strings.Join(e.scopes, ", ")
	for _, id := range d.gone {
		a.systemEvent(ctx, "api_key_deleted", "API key "+envKeyLabel+" replaced from the file", map[string]any{"key_id": id, "label": envKeyLabel})
	}
	switch {
	case d.made != 0:
		a.systemEvent(ctx, "api_key_created", "API key "+envKeyLabel+" created", map[string]any{"key_id": d.made, "label": envKeyLabel, "scopes": e.scopes})
		log.Printf("auth: made the owner's API key from %s (scopes %s)", EnvAPIKey, scopes)
	case d.rescoped:
		a.systemEvent(ctx, "api_key_changed", "API key "+envKeyLabel+" changed", map[string]any{"key_id": d.kept, "label": envKeyLabel, "scopes": e.scopes})
		log.Printf("auth: the API key from %s now has the scopes %s", EnvAPIKey, scopes)
	default:
		log.Printf("auth: the API key from %s is in place (scopes %s)", EnvAPIKey, scopes)
	}
}

// errRevokedUnreadable stops the start rather than risk letting a removed
// key back in.
var errRevokedUnreadable = errors.New("the list of removed " + EnvAPIKey + " keys (setting " + settingsGroup + "/" + keyRevokedSetting + ") cannot be read")

// revokedKeys reads keyRevokedSetting. An unreadable list is an error, never
// an empty one.
func revokedKeys(ctx context.Context, q database.Querier) ([]string, error) {
	v, found, err := database.SettingFound(ctx, q, settingsGroup, keyRevokedSetting)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errRevokedUnreadable, err)
	}
	if !found {
		return nil, nil
	}
	var out []string
	if err := json.Unmarshal([]byte(v), &out); err != nil {
		return nil, fmt.Errorf("%w: %w", errRevokedUnreadable, err)
	}
	return out, nil
}

// envKeyHash is the hash of key id when the key came from EnvAPIKey, else "".
func envKeyHash(ctx context.Context, q conn, id int64) (string, error) {
	hash, err := madeKeyHash(ctx, q, id, envKeyLabel, systemActor)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return hash, err
}

// noteRevokedOf lists the file's key a person holds, if any, as removed.
// Call it in the write that deletes their keys for a password change.
func noteRevokedOf(ctx context.Context, tx *sql.Tx, userID int64, by domain.Actor, now time.Time) error {
	keys, err := madeKeys(ctx, tx, userID, envKeyLabel, systemActor)
	if err != nil {
		return err
	}
	for _, k := range keys {
		if err := noteRevoked(ctx, tx, k.hash, by, now); err != nil {
			return err
		}
	}
	return nil
}

// noteRevoked adds a deleted key's hash to keyRevokedSetting. Call it in the
// write that deleted the key.
func noteRevoked(ctx context.Context, tx *sql.Tx, hash string, by domain.Actor, now time.Time) error {
	list, err := revokedKeys(ctx, tx)
	if err != nil {
		return err
	}
	if slices.Contains(list, hash) {
		return nil
	}
	list = append(list, hash)
	list = list[max(0, len(list)-maxRevoked):]
	v, _ := json.Marshal(list)
	return database.PutSetting(tx, settingsGroup, keyRevokedSetting, string(v), by, now)
}

// systemEvent records a change HolyArrs made on its own at start.
func (a *Auth) systemEvent(ctx context.Context, kind, summary string, data map[string]any) {
	_ = a.s.AddEvent(ctx, &domain.Event{Actor: domain.System, Kind: kind, Summary: summary, Data: data})
}
