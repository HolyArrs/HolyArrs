package accounts

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// "Forgot password?" on the sign-in page: the owner proves access to the
// host by reading a one-time code from the container log or from
// ResetCodeFile in the config folder. Only the code's hash is kept, in one
// settings row per slot: one for codes asked for on the sign-in page and one
// for codes made on the host (`reset-owner --code`, the start-up env var), so
// a web request can never replace or end a host code.

// ResetCodeFile is the file in the config folder that holds the live code.
const ResetCodeFile = "reset-owner-code.txt"

// EnvResetOwner set to 1 prints a fresh code at start-up.
const EnvResetOwner = "HOLYARRS_RESET_OWNER"

// Code rules.
const (
	ResetCodeLife    = 15 * time.Minute
	ResetCodeEnvLife = 30 * time.Minute
	resetCodeTries   = 5  // wrong tries end a sign-in page code
	resetHostTries   = 25 // a host code takes more, so strangers cannot easily end it
	resetPerIP       = time.Minute
	resetPerHour     = 5
	resetTryPerMin   = 10                      // check/finish requests per address a minute
	resetWrongPerIP  = 5                       // wrong codes per address per window
	resetWrongWindow = 15 * time.Minute        // window for wrong codes
	keyResetCode     = "owner_reset_code"      // {hash, expires, tries}, from the sign-in page
	keyResetHostCode = "owner_reset_code_host" // same, from reset-owner --code or the env var
	keyResetLink     = "owner_reset_link"      // "false" hides the link
)

// codeAlphabet has no look-alike characters (0/O, 1/I/L).
const codeAlphabet = "23456789ABCDEFGHJKMNPQRSTUVWXYZ"

var (
	errResetCode = errors.New("that code is wrong or has run out; make a new one")
	errNoOwner   = errors.New("there is no owner yet; open the setup page instead")
)

type resetCodeRow struct {
	Hash    string `json:"hash"`
	Expires string `json:"expires"`
	Tries   int    `json:"tries"`
}

// resetLimits counts code requests from the sign-in page, and check and
// finish tries per client address.
type resetLimits struct {
	mu     sync.Mutex
	perIP  map[string]time.Time
	recent []time.Time
	tries  map[string][]time.Time // check/finish requests in the last minute
	wrong  map[string][]time.Time // wrong codes in the last resetWrongWindow
}

func keepSince(ts []time.Time, now time.Time, d time.Duration) []time.Time {
	out := ts[:0]
	for _, t := range ts {
		if now.Sub(t) < d {
			out = append(out, t)
		}
	}
	return out
}

func pruneTimes(m map[string][]time.Time, now time.Time, d time.Duration) {
	for k, v := range m {
		if v = keepSince(v, now, d); len(v) == 0 {
			delete(m, k)
		} else {
			m[k] = v
		}
	}
}

// tryAllowed says whether each address may check a code now, and counts the
// request against each.
func (l *resetLimits) tryAllowed(now time.Time, ips ...string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.tries == nil {
		l.tries = map[string][]time.Time{}
	}
	if l.wrong == nil {
		l.wrong = map[string][]time.Time{}
	}
	pruneTimes(l.tries, now, time.Minute)
	pruneTimes(l.wrong, now, resetWrongWindow)
	ok := true
	for _, ip := range ips {
		if ip != "" && (len(l.tries[ip]) >= resetTryPerMin || len(l.wrong[ip]) >= resetWrongPerIP) {
			ok = false
		}
	}
	for _, ip := range ips {
		if ip != "" {
			l.tries[ip] = append(l.tries[ip], now)
		}
	}
	return ok
}

// countWrong counts a wrong code against each address.
func (l *resetLimits) countWrong(now time.Time, ips ...string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.wrong == nil {
		l.wrong = map[string][]time.Time{}
	}
	for _, ip := range ips {
		if ip != "" {
			l.wrong[ip] = append(l.wrong[ip], now)
		}
	}
}

// allow says whether a request from ip may make a code now, and counts it.
func (l *resetLimits) allow(ip string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.perIP == nil {
		l.perIP = map[string]time.Time{}
	}
	for k, t := range l.perIP {
		if now.Sub(t) >= resetPerIP {
			delete(l.perIP, k)
		}
	}
	kept := l.recent[:0]
	for _, t := range l.recent {
		if now.Sub(t) < time.Hour {
			kept = append(kept, t)
		}
	}
	l.recent = kept
	if _, ok := l.perIP[ip]; ok || len(l.recent) >= resetPerHour {
		return false
	}
	l.perIP[ip] = now
	l.recent = append(l.recent, now)
	return true
}

func newResetCode() string {
	var b strings.Builder
	max := big.NewInt(int64(len(codeAlphabet)))
	for i := 0; i < 24; i++ {
		if i > 0 && i%4 == 0 {
			b.WriteByte('-')
		}
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			panic(err) // crypto/rand never fails on supported systems
		}
		b.WriteByte(codeAlphabet[n.Int64()])
	}
	return b.String()
}

// normCode drops spaces and dashes and upper-cases, so a pasted code with a
// stray space still matches.
func normCode(c string) string {
	c = strings.ToUpper(c)
	return strings.Map(func(r rune) rune {
		if r == ' ' || r == '-' || r == '\t' || r == '\r' || r == '\n' {
			return -1
		}
		return r
	}, c)
}

func hashCode(c string) string {
	h := sha256.Sum256([]byte("holyarrs-owner-reset:" + normCode(c)))
	return hex.EncodeToString(h[:])
}

// MakeOwnerResetCode makes a fresh one-time code on the host (reset-owner
// --code, the start-up env var), replacing any live host code. A code asked
// for on the sign-in page lives in its own slot and never replaces it.
func MakeOwnerResetCode(ctx context.Context, s *database.Store, life time.Duration, now time.Time) (string, error) {
	return makeResetCode(ctx, s, keyResetHostCode, life, now)
}

// readSlot reads the code row in slot; live says it has not run out.
func readSlot(ctx context.Context, q database.Querier, slot string, now time.Time) (row resetCodeRow, set, live bool, err error) {
	v, _, err := database.SettingFound(ctx, q, settingsGroup, signInPrefix+slot)
	if err != nil || v == "" {
		return row, false, false, err
	}
	if json.Unmarshal([]byte(v), &row) != nil {
		return row, true, false, nil
	}
	exp, perr := time.Parse(database.TimeLayout, row.Expires)
	return row, true, perr == nil && now.Before(exp), nil
}

func slotLive(ctx context.Context, s *database.Store, slot string, now time.Time) bool {
	_, _, live, _ := readSlot(ctx, s.Read(), slot, now)
	return live
}

// makeResetCode keeps only the code's hash in slot, writes the code to
// ResetCodeFile (owner-only; a sign-in page code leaves the file alone while
// a host code is live) and prints one line on the program's output. The
// stored log the app shows hides that code (logs.HideOwnerCode), so an admin
// reading the log viewer or download never sees it.
func makeResetCode(ctx context.Context, s *database.Store, slot string, life time.Duration, now time.Time) (string, error) {
	if _, _, err := ownerIdentity(ctx, s.Read()); err != nil {
		return "", errNoOwner
	}
	code := newResetCode()
	row, _ := json.Marshal(resetCodeRow{Hash: hashCode(code), Expires: database.FormatTime(now.Add(life))})
	if err := s.WriteCore(ctx, func(tx *sql.Tx) error {
		return database.PutSetting(tx, settingsGroup, signInPrefix+slot, string(row), domain.System, now)
	}); err != nil {
		return "", err
	}
	if slot == keyResetHostCode || !slotLive(ctx, s, keyResetHostCode, now) {
		file := filepath.Join(s.Dir(), ResetCodeFile)
		_ = os.Remove(file) // a fresh file gets the 0600 mode even if an old one had another
		body := fmt.Sprintf("HolyArrs owner reset code: %s\nIt works once, until %s.\n", code, now.Add(life).UTC().Format("2006-01-02 15:04 MST"))
		if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
			log.Printf("auth: could not write %s: %v", ResetCodeFile, err)
		}
	}
	log.Printf("OWNER RESET CODE: %s (works once, for %d minutes; paste it on the sign-in page)", code, int(life.Minutes()))
	return code, nil
}

// dropResetCode ends the code in each slot given, and removes the file once
// no live code is left.
func dropResetCode(ctx context.Context, s *database.Store, now time.Time, slots ...string) {
	_ = s.WriteCore(ctx, func(tx *sql.Tx) error {
		for _, slot := range slots {
			if err := database.PutSetting(tx, settingsGroup, signInPrefix+slot, "", domain.System, now); err != nil {
				return err
			}
		}
		return nil
	})
	if slotLive(ctx, s, keyResetCode, now) || slotLive(ctx, s, keyResetHostCode, now) {
		return
	}
	if err := os.Remove(filepath.Join(s.Dir(), ResetCodeFile)); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("auth: could not remove %s: %v", ResetCodeFile, err)
	}
}

// checkResetCode says whether code is a live one, in either slot. A wrong
// code counts a try on each live code: resetCodeTries end a sign-in page
// code, resetHostTries a host code. consume ends every code on a match, as
// the owner is then reset.
func (a *Auth) checkResetCode(ctx context.Context, code string, consume bool) bool {
	now := a.now()
	var good bool
	var drop []string
	slots := []string{keyResetHostCode, keyResetCode}
	err := a.s.WriteCore(ctx, func(tx *sql.Tx) error {
		drop = nil
		type liveRow struct {
			slot string
			row  resetCodeRow
		}
		var live []liveRow
		for _, slot := range slots {
			row, set, ok, err := readSlot(ctx, tx, slot, now)
			if err != nil {
				return err
			}
			if set && !ok {
				drop = append(drop, slot)
			} else if ok {
				live = append(live, liveRow{slot, row})
			}
		}
		h := hashCode(code)
		for _, lr := range live {
			if normCode(code) != "" && subtle.ConstantTimeCompare([]byte(h), []byte(lr.row.Hash)) == 1 {
				good = true
			}
		}
		if good {
			if consume {
				drop = slots
			}
			return endSlots(tx, drop, now)
		}
		for _, lr := range live {
			lr.row.Tries++
			limit := resetCodeTries
			if lr.slot == keyResetHostCode {
				limit = resetHostTries
			}
			if lr.row.Tries >= limit {
				drop = append(drop, lr.slot)
				continue
			}
			b, _ := json.Marshal(lr.row)
			if err := database.PutSetting(tx, settingsGroup, signInPrefix+lr.slot, string(b), domain.System, now); err != nil {
				return err
			}
		}
		return endSlots(tx, drop, now)
	})
	if err != nil {
		log.Printf("auth: owner reset code: %v", err)
		return false
	}
	if len(drop) > 0 {
		dropResetCode(ctx, a.s, now, drop...)
	}
	return good
}

// endSlots clears the slots in the same write that checked them, so two
// requests at once can't both use one code.
func endSlots(tx *sql.Tx, slots []string, now time.Time) error {
	for _, slot := range slots {
		if err := database.PutSetting(tx, settingsGroup, signInPrefix+slot, "", domain.System, now); err != nil {
			return err
		}
	}
	return nil
}

// expireResetCode removes codes that ran out, and the file (from Cleanup).
func (a *Auth) expireResetCode(ctx context.Context) {
	var drop []string
	for _, slot := range []string{keyResetCode, keyResetHostCode} {
		if _, set, live, err := readSlot(ctx, a.s.Read(), slot, a.now()); err == nil && set && !live {
			drop = append(drop, slot)
		}
	}
	if len(drop) > 0 {
		dropResetCode(ctx, a.s, a.now(), drop...)
	}
}

// resetLinkOn says whether the sign-in page shows "Forgot password?".
func (a *Auth) resetLinkOn(ctx context.Context) bool {
	return a.signInSetting(ctx, keyResetLink) != "false"
}

// startResetFromEnv prints a code at start-up when EnvResetOwner is 1.
func (a *Auth) startResetFromEnv() {
	if os.Getenv(EnvResetOwner) != "1" {
		return
	}
	if _, err := MakeOwnerResetCode(context.Background(), a.s, ResetCodeEnvLife, a.now()); err != nil {
		log.Printf("auth: %s=1 but no code was made: %v", EnvResetOwner, err)
		return
	}
	a.envReset = true
	log.Printf("auth: %s=1 is set. Remove it once you are back in; each start makes a new code.", EnvResetOwner)
}

func (a *Auth) ownerResetRoutes(m *http.ServeMux) {
	m.HandleFunc("POST /api/v1/auth/owner-reset/code", a.resetCodeRequest)
	m.HandleFunc("POST /api/v1/auth/owner-reset/check", a.resetCodeCheck)
	m.HandleFunc("POST /api/v1/auth/owner-reset/finish", a.resetCodeFinish)
}

// resetCodeRequest makes a code. It answers the same whether or not one was
// made, so probing it tells nothing.
func (a *Auth) resetCodeRequest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ip, _ := a.clientAddr(r)
	if a.resetLinkOn(ctx) && a.reset.allow(ip, a.now()) {
		if _, err := makeResetCode(ctx, a.s, keyResetCode, ResetCodeLife, a.now()); err != nil && !errors.Is(err, errNoOwner) {
			log.Printf("auth: owner reset code: %v", err)
		} else if err == nil {
			log.Printf("auth: owner reset code asked for from %s", r.RemoteAddr)
		}
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true})
}

type resetBody struct {
	Code     string `json:"code"`
	Name     string `json:"name"`
	Password string `json:"password"`
}

func readResetBody(w http.ResponseWriter, r *http.Request) (resetBody, bool) {
	var b resetBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&b); err != nil || len(b.Code) > 64 {
		fail(w, http.StatusBadRequest, `send {"code": "...", "name": "...", "password": "..."}`)
		return b, false
	}
	b.Name = strings.TrimSpace(b.Name)
	return b, true
}

// resetTry applies the per-address limit before a code is checked, and
// counts a wrong code against the address (both, when the proxy headers
// disagree). It answers false when it already failed the request.
func (a *Auth) resetTry(w http.ResponseWriter, r *http.Request, code string, consume bool) bool {
	addr, other, _, _ := a.clientAddrBoth(r)
	if !a.reset.tryAllowed(a.now(), addr, other) {
		fail(w, http.StatusTooManyRequests, "too many tries; wait a few minutes and try again")
		return false
	}
	if !a.checkResetCode(r.Context(), code, consume) {
		a.reset.countWrong(a.now(), addr, other)
		fail(w, http.StatusUnauthorized, errResetCode.Error())
		return false
	}
	return true
}

// resetCodeCheck checks a code without using it up, and gives the owner name
// to fill in. A wrong code counts a try.
func (a *Auth) resetCodeCheck(w http.ResponseWriter, r *http.Request) {
	b, ok := readResetBody(w, r)
	if !ok {
		return
	}
	if !a.resetTry(w, r, b.Code, false) {
		return
	}
	_, name, _ := a.owner(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"name": name})
}

// resetCodeFinish uses the code and does what reset-owner does.
func (a *Auth) resetCodeFinish(w http.ResponseWriter, r *http.Request) {
	b, ok := readResetBody(w, r)
	if !ok {
		return
	}
	// Check the new login first, so a typo does not use the code up.
	if err := CheckNewOwner(b.Name, b.Password, MinPasswordLength(r.Context(), a.s.Read())); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if !a.resetTry(w, r, b.Code, true) {
		return
	}
	if err := resetOwnerVia(r.Context(), a.s, b.Name, b.Password, resetByCode); err != nil {
		log.Printf("auth: owner reset with a code: %v", err)
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	log.Printf("auth: owner reset with a log code from %s", r.RemoteAddr)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "name": b.Name})
}
