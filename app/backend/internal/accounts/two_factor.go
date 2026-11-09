package accounts

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"database/sql"
	"encoding/base32"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts/qr"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// Two-factor sign-in: time-based six-digit codes (RFC 6238, SHA-1, 30 s)
// from an authenticator app, plus ten single-use backup codes.
const (
	totpStep        = 30
	totpDigits      = 6
	totpSkew        = 1 // steps either side accepted, for clock drift
	backupCodeCount = 10
	ticketLife      = 5 * time.Minute
	ticketTries     = 5
)

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// ticket is the half-way point of a sign-in: the password (or outside
// account) was right and a two-factor code is still needed.
type ticket struct {
	userKey string // the limit key of the first step, for known places
	userID  int64
	name    string
	method  string
	expires time.Time
	tries   int
}

type codeBody struct {
	Ticket string `json:"ticket,omitempty"`
	Code   string `json:"code"`
}

// totpCode is the code for one time step.
func totpCode(secret []byte, step int64) string {
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(step))
	m := hmac.New(sha1.New, secret)
	m.Write(msg[:])
	h := m.Sum(nil)
	off := h[len(h)-1] & 0x0f
	v := binary.BigEndian.Uint32(h[off:off+4]) & 0x7fffffff
	return fmt.Sprintf("%0*d", totpDigits, v%1_000_000)
}

// totpMatch returns the step a code matches near now, or 0. Steps at or
// before last are refused so a code works only once.
func totpMatch(secret []byte, code string, now time.Time, last int64) int64 {
	code = strings.ReplaceAll(strings.TrimSpace(code), " ", "")
	if len(code) != totpDigits {
		return 0
	}
	cur := now.Unix() / totpStep
	for d := int64(-totpSkew); d <= totpSkew; d++ {
		st := cur + d
		if st > last && subtle.ConstantTimeCompare([]byte(totpCode(secret, st)), []byte(code)) == 1 {
			return st
		}
	}
	return 0
}

func normBackup(c string) string {
	return strings.ToLower(strings.NewReplacer("-", "", " ", "").Replace(strings.TrimSpace(c)))
}

// passedAs finishes a sign-in while preserving how its first factor was
// checked across an optional two-factor ticket.
func (a *Auth) passedAs(w http.ResponseWriter, r *http.Request, userID int64, name, method string) {
	a.passedPlace(w, r, userID, name, method, place{})
}

// passedPlace is passedAs that also records the place as known once the
// sign-in is complete: now when there is no second step, else after it.
func (a *Auth) passedPlace(w http.ResponseWriter, r *http.Request, userID int64, name, method string, pl place) {
	on, err := twoFactorOn(r.Context(), a.s.Read(), userID)
	if err != nil {
		log.Printf("auth: sign-in: %v", err)
		fail(w, http.StatusInternalServerError, "something went wrong; see the log")
		return
	}
	if !on {
		a.mu.Lock()
		known := a.noteKnown(pl)
		a.mu.Unlock()
		a.saveKnown(known)
		a.startSessionAs(w, r, userID, name, http.StatusOK, method)
		return
	}
	t := token(32)
	now := a.now()
	a.mu.Lock()
	for k, v := range a.tickets {
		if now.After(v.expires) {
			delete(a.tickets, k)
		}
	}
	a.tickets[sum(t)] = &ticket{userKey: pl.userKey, userID: userID, name: name, method: method, expires: now.Add(ticketLife)}
	a.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"two_factor": true, "ticket": t})
}

func (a *Auth) twoFactorRoutes(m *http.ServeMux) {
	m.HandleFunc("POST /api/v1/auth/login/two-factor", a.loginTwoFactor)
	m.HandleFunc("POST /api/v1/auth/two-factor/setup", a.twoFactorSetup)
	m.HandleFunc("POST /api/v1/auth/two-factor/enable", a.twoFactorEnable)
	m.HandleFunc("POST /api/v1/auth/two-factor/disable", a.twoFactorDisable)
	m.HandleFunc("POST /api/v1/auth/two-factor/backup-codes", a.twoFactorNewCodes)
}

func readCode(w http.ResponseWriter, r *http.Request) (codeBody, bool) {
	var b codeBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&b); err != nil {
		fail(w, http.StatusBadRequest, `send {"code": "..."}`)
		return b, false
	}
	return b, true
}

// checkCode checks a code from the app, or a backup code, for a user whose
// two-factor sign-in is on, and uses it up.
func (a *Auth) checkCode(ctx context.Context, userID int64, code string) (bool, error) {
	if a.vault == nil {
		return false, errors.New("no secrets vault")
	}
	sid, last, err := totpState(ctx, a.s.Read(), userID)
	if err != nil {
		return false, err
	}
	if !sid.Valid {
		return false, nil
	}
	sec, err := a.vault.Get(ctx, sid.Int64)
	if err != nil {
		return false, err
	}
	key, err := b32.DecodeString(sec)
	if err != nil {
		return false, err
	}
	if st := totpMatch(key, code, a.now(), last); st > 0 {
		var n int64
		err := a.s.WriteCore(ctx, func(tx *sql.Tx) error {
			var err error
			n, err = useTotpStep(ctx, tx, st, userID)
			return err
		})
		return n == 1, err
	}
	bc := normBackup(code)
	if len(bc) < 10 {
		return false, nil
	}
	var n int64
	err = a.s.WriteCore(ctx, func(tx *sql.Tx) error {
		var err error
		n, err = useBackupCode(ctx, tx, database.FormatTime(a.now()), userID, sum(bc))
		return err
	})
	return n == 1, err
}

func (a *Auth) loginTwoFactor(w http.ResponseWriter, r *http.Request) {
	b, ok := readCode(w, r)
	if !ok {
		return
	}
	h := sum(b.Ticket)
	ipKey, altKey, sure := a.signInKeys(r)
	// A ticket from single sign-on or the trusted proxy finishes like those
	// sign-ins: a settings change or an unlink waits for it, or ends it first.
	a.ssoMu.RLock()
	defer a.ssoMu.RUnlock()
	a.mu.Lock()
	t := a.tickets[h]
	if t != nil && (a.now().After(t.expires) || t.tries >= ticketTries) {
		delete(a.tickets, h)
		t = nil
	}
	var wait time.Duration
	var marks []failMark
	if t != nil {
		// The first step's name key: "name:<name>" after a password, the
		// media-server name after Jellyfin or Emby, none after Plex or single
		// sign-on, so strangers holding the local name's wait cannot stop them.
		userKey := t.userKey
		// codeKey survives a correct password, so a fresh ticket brings back
		// no free code guesses; only a right code clears it.
		codeKey := "2fa:" + itoa(t.userID)
		wait = max(a.signInWait(ipKey, userKey, sure), a.altWait(altKey), a.waitFor(codeKey, freeTries))
		if wait == 0 {
			t.tries++
			marks = a.markFails(ipKey, altKey)
			a.noteFail(ipKey, altKey, userKey, codeKey)
		}
	}
	a.mu.Unlock()
	if t == nil {
		fail(w, http.StatusUnauthorized, "this sign-in has ended; start again")
		return
	}
	if wait > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		fail(w, http.StatusTooManyRequests, "too many failed sign-ins; wait a little and try again")
		return
	}
	good, err := a.checkCode(r.Context(), t.userID, b.Code)
	if err != nil {
		log.Printf("auth: two-factor: %v", err)
	}
	if !good {
		log.Printf("auth: wrong two-factor code from %s", r.RemoteAddr)
		a.failedEvent(r, t.name, "two-factor")
		fail(w, http.StatusUnauthorized, "wrong code")
		return
	}
	a.mu.Lock()
	if a.tickets[h] != t {
		// Ended meanwhile, by a settings change or an unlink.
		a.mu.Unlock()
		fail(w, http.StatusUnauthorized, "this sign-in has ended; start again")
		return
	}
	delete(a.tickets, h)
	a.undoFailLocked(marks) // only this attempt's address count
	delete(a.fails, "name:"+strings.ToLower(t.name))
	delete(a.fails, "2fa:"+itoa(t.userID))
	known := a.noteKnown(place{ipKey, t.userKey, sure})
	a.mu.Unlock()
	a.saveKnown(known)
	a.startSessionAs(w, r, t.userID, t.name, http.StatusOK, t.method)
}

func (a *Auth) twoFactorSetup(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	if a.vault == nil {
		fail(w, http.StatusServiceUnavailable, "two-factor sign-in needs the master key; see the log")
		return
	}
	who, _ := webserver.From(r.Context())
	if who.TwoFactor {
		fail(w, http.StatusConflict, "two-factor sign-in is already on; turn it off first to start over")
		return
	}
	raw := make([]byte, 20)
	_, _ = rand.Read(raw)
	secret := b32.EncodeToString(raw)
	err := a.s.WriteCore(r.Context(), func(tx *sql.Tx) error {
		old, err := pendingTotpID(r.Context(), tx, who.UserID)
		if err != nil {
			return err
		}
		if old.Valid {
			if err := a.vault.DeleteTx(r.Context(), tx, old.Int64); err != nil {
				return err
			}
		}
		id, err := a.vault.PutTx(tx, "two_factor", secret, webserver.Actor(r.Context()))
		if err != nil {
			return err
		}
		return setPendingTotp(r.Context(), tx, id, who.UserID)
	})
	if err != nil {
		log.Printf("auth: two-factor setup: %v", err)
		fail(w, http.StatusInternalServerError, "something went wrong; see the log")
		return
	}
	uri := "otpauth://totp/" + url.PathEscape("HolyArrs:"+who.Name) + "?" + url.Values{
		"secret": {secret}, "issuer": {"HolyArrs"}, "algorithm": {"SHA1"}, "digits": {"6"}, "period": {"30"}}.Encode()
	out := map[string]any{"secret": secret, "uri": uri}
	// A code to scan, as rows of "1" (dark) and "0"; left out if the link
	// is ever too long for one, and the key can still be typed.
	if m, err := qr.Encode(uri); err == nil {
		rows := make([]string, len(m))
		for y, row := range m {
			b := make([]byte, len(row))
			for x, dark := range row {
				b[x] = '0'
				if dark {
					b[x] = '1'
				}
			}
			rows[y] = string(b)
		}
		out["qr"] = rows
	}
	writeJSON(w, http.StatusOK, out)
}

// newBackupCodes replaces a user's backup codes inside tx and returns them.
func newBackupCodes(tx *sql.Tx, userID int64) ([]string, error) {
	if err := deleteBackupCodes(context.Background(), tx, userID); err != nil {
		return nil, err
	}
	const alpha = "abcdefghjkmnpqrstuvwxyz23456789"
	out := make([]string, 0, backupCodeCount)
	for range backupCodeCount {
		c := make([]byte, 0, 10)
		var one [1]byte
		for len(c) < 10 {
			_, _ = rand.Read(one[:])
			if int(one[0]) < 256/len(alpha)*len(alpha) { // no bias toward early letters
				c = append(c, alpha[int(one[0])%len(alpha)])
			}
		}
		code := string(c)
		if err := insertBackupCode(context.Background(), tx, userID, sum(code)); err != nil {
			return nil, err
		}
		out = append(out, code[:5]+"-"+code[5:])
	}
	return out, nil
}

func (a *Auth) twoFactorEnable(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	b, ok := readCode(w, r)
	if !ok {
		return
	}
	if a.vault == nil {
		fail(w, http.StatusServiceUnavailable, "two-factor sign-in needs the master key; see the log")
		return
	}
	who, _ := webserver.From(r.Context())
	pid, err := pendingTotpID(r.Context(), a.s.Read(), who.UserID)
	if err != nil || !pid.Valid {
		fail(w, http.StatusConflict, "start two-factor setup first")
		return
	}
	sec, err := a.vault.Get(r.Context(), pid.Int64)
	key, err2 := b32.DecodeString(sec)
	if err != nil || err2 != nil {
		fail(w, http.StatusInternalServerError, "something went wrong; see the log")
		return
	}
	st := totpMatch(key, b.Code, a.now(), 0)
	if st == 0 {
		fail(w, http.StatusBadRequest, "that code does not match; check the clock on your phone and try again")
		return
	}
	var codes []string
	err = a.s.WriteCore(r.Context(), func(tx *sql.Tx) error {
		if err := confirmTotp(r.Context(), tx, st, who.UserID); err != nil {
			return err
		}
		var err error
		codes, err = newBackupCodes(tx, who.UserID)
		return err
	})
	if err != nil {
		log.Printf("auth: two-factor enable: %v", err)
		fail(w, http.StatusInternalServerError, "something went wrong; see the log")
		return
	}
	log.Printf("auth: two-factor sign-in turned on for user %d", who.UserID)
	a.accountEvent(r, who.UserID, "two_factor_on", who.Name+" turned on two-factor sign-in", nil)
	writeJSON(w, http.StatusOK, map[string]any{"backup_codes": codes})
}

// clearTwoFactor turns two-factor sign-in off for a user inside tx.
func clearTwoFactor(ctx context.Context, tx *sql.Tx, vault *secrets.Vault, userID int64) error {
	ids, err := totpSecretIDs(ctx, tx, userID)
	if err != nil {
		return err
	}
	if err := vault.DeleteTx(ctx, tx, ids...); err != nil {
		return err
	}
	if err := resetTotpColumns(ctx, tx, userID); err != nil {
		return err
	}
	return deleteBackupCodes(ctx, tx, userID)
}

func (a *Auth) twoFactorDisable(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	b, ok := readCode(w, r)
	if !ok {
		return
	}
	who, _ := webserver.From(r.Context())
	if !a.codeOrFail(w, r, who, b.Code) {
		return
	}
	err := a.s.WriteCore(r.Context(), func(tx *sql.Tx) error { return clearTwoFactor(r.Context(), tx, a.vault, who.UserID) })
	if err != nil {
		fail(w, http.StatusInternalServerError, "something went wrong; see the log")
		return
	}
	log.Printf("auth: two-factor sign-in turned off for user %d", who.UserID)
	a.accountEvent(r, who.UserID, "two_factor_off", who.Name+" turned off two-factor sign-in", nil)
	w.WriteHeader(http.StatusNoContent)
}

func (a *Auth) twoFactorNewCodes(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	b, ok := readCode(w, r)
	if !ok {
		return
	}
	who, _ := webserver.From(r.Context())
	if !a.codeOrFail(w, r, who, b.Code) {
		return
	}
	var codes []string
	err := a.s.WriteCore(r.Context(), func(tx *sql.Tx) error {
		var err error
		codes, err = newBackupCodes(tx, who.UserID)
		return err
	})
	if err != nil {
		fail(w, http.StatusInternalServerError, "something went wrong; see the log")
		return
	}
	a.accountEvent(r, who.UserID, "backup_codes_new", who.Name+" made new two-factor backup codes; the old ones stopped working", nil)
	writeJSON(w, http.StatusOK, map[string]any{"backup_codes": codes})
}

// codeOrFail checks a code for a signed-in person, with the same back-off as
// sign-in so a stolen session cannot guess codes quickly.
func (a *Auth) codeOrFail(w http.ResponseWriter, r *http.Request, who webserver.Caller, code string) bool {
	if !who.TwoFactor {
		fail(w, http.StatusConflict, "two-factor sign-in is off")
		return false
	}
	key := "code:" + itoa(who.UserID)
	a.mu.Lock()
	wait := a.waitFor(key, freeTries)
	if wait == 0 {
		a.noteFail(key)
	}
	a.mu.Unlock()
	if wait > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		fail(w, http.StatusTooManyRequests, "too many wrong codes; wait a little and try again")
		return false
	}
	good, err := a.checkCode(r.Context(), who.UserID, code)
	if err != nil {
		log.Printf("auth: two-factor: %v", err)
	}
	if !good {
		a.failedEvent(r, who.Name, "two-factor check")
		fail(w, http.StatusUnauthorized, "wrong code")
		return false
	}
	a.mu.Lock()
	delete(a.fails, key)
	a.mu.Unlock()
	return true
}
