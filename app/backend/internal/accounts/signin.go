package accounts

import (
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/startup"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

func (a *Auth) login(w http.ResponseWriter, r *http.Request) {
	c, ok := readCreds(w, r)
	if !ok {
		return
	}
	if len(c.Name) > 64 { // no account has a longer name; keeps the back-off map small
		fail(w, http.StatusUnauthorized, "wrong name or password")
		return
	}
	ipKey, altKey, sure := a.signInKeys(r)
	userKey := "name:" + strings.ToLower(c.Name)
	// Check and reserve in one locked step: the attempt counts as failed up
	// front and is cleared on success, so parallel attempts cannot slip past.
	var marks []failMark
	a.mu.Lock()
	wait := max(a.signInWait(ipKey, userKey, sure), a.altWait(altKey))
	if wait == 0 {
		marks = a.markFails(ipKey, altKey)
		a.noteFail(ipKey, altKey, userKey)
	}
	a.mu.Unlock()
	if wait > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		fail(w, http.StatusTooManyRequests, "too many failed sign-ins; wait a little and try again")
		return
	}
	select {
	case a.checks <- struct{}{}:
		defer func() { <-a.checks }()
	case <-r.Context().Done():
		return
	}
	// One lookup in every case (no extra query on a failure path), so the
	// time taken does not tell an unknown name from a turned-off member.
	id, name, hash, state, err := loginRow(r.Context(), a.s.Read(), c.Name)
	known := err == nil // a typed name that is no user may be a password typed in the wrong box
	turnedOff := known && state != "active"
	if turnedOff {
		err = errDisabled
	}
	if err != nil || hash == "" {
		// Same work whether the name is unknown or has no password (people
		// who sign in only through Plex, Jellyfin or Emby).
		hash = a.dummy
		err = errors.Join(err, errNoLocalSignIn)
	}
	good := CheckPassword(hash, c.Password) && err == nil
	if !good {
		log.Printf("auth: failed sign-in from %s", r.RemoteAddr)
		// A turned-off member keeps their name; a name that is no member stays
		// "unknown name" (it may be a password typed in the wrong box).
		who, how := unknownName, "password"
		if known {
			who = name
		}
		if turnedOff {
			how = "turned off"
		}
		a.failedEvent(r, who, how)
		fail(w, http.StatusUnauthorized, "wrong name or password")
		return
	}
	a.mu.Lock()
	a.undoFailLocked(marks) // only this attempt's address count; others' failures there stay
	delete(a.fails, userKey)
	a.mu.Unlock()
	a.passedPlace(w, r, id, name, AuthMethodLocalPassword, place{ipKey, userKey, sure})
}

func (a *Auth) setCookie(w http.ResponseWriter, r *http.Request, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{Name: CookieName, Value: value, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Secure: a.behindHTTPS(r), MaxAge: maxAge})
}

// startSession drops the session the request came with (so a planted id
// never survives a sign-in) and starts a fresh one.
func (a *Auth) startSession(w http.ResponseWriter, r *http.Request, userID int64, name string, code int) {
	a.startSessionAs(w, r, userID, name, code, AuthMethodLocalSetup)
}

// startSessionAs starts a fresh session with its authentication method. If
// the request presents an existing session, its complete rotation family is
// deleted before the replacement is made.
func (a *Auth) startSessionAs(w http.ResponseWriter, r *http.Request, userID int64, name string, code int, method string) {
	a.startSessionIn(w, r, userID, name, code, method, newFamily())
}

// startSetupSession starts the session first-run owner setup hands out; its
// family marks it for SetupRecent (see setupRecentFor).
func (a *Auth) startSetupSession(w http.ResponseWriter, r *http.Request, userID int64, name string, code int, method string) {
	a.startSessionIn(w, r, userID, name, code, method, setupFamily+newFamily())
}

func (a *Auth) startSessionIn(w http.ResponseWriter, r *http.Request, userID int64, name string, code int, method, family string) {
	now := a.now()
	tok, csrf := token(32), token(32)
	err := a.s.WriteCore(r.Context(), func(tx *sql.Tx) error {
		if ck, err := r.Cookie(CookieName); err == nil {
			if err := deletePresentedFamily(tx, ck.Value); err != nil {
				return err
			}
		}
		t := database.FormatTime(now)
		localAt := ""
		if method == AuthMethodLocalPassword || method == AuthMethodLocalSetup {
			localAt = t
		}
		return insertSession(r.Context(), tx, sum(tok), userID, csrf, t, database.FormatTime(now.Add(AbsoluteTimeout)),
			truncate(r.UserAgent(), 200), family, method, localAt)
	})
	if err != nil {
		log.Printf("auth: session: %v", err)
		fail(w, http.StatusInternalServerError, "something went wrong; see the log")
		return
	}
	a.setCookie(w, r, tok, int(AbsoluteTimeout.Seconds()))
	a.event(r, domain.Actor{Kind: "user", ID: userID}, "signed_in", name+" signed in", map[string]any{"name": name, "method": method})
	writeJSON(w, code, map[string]string{"name": name, "csrf": csrf})
}

// session checks the cookie, ends idle or old sessions, and rotates the id
// once an hour while the session is used.
func (a *Auth) session(w http.ResponseWriter, r *http.Request) (webserver.Caller, bool) {
	ck, err := r.Cookie(CookieName)
	if err != nil || ck.Value == "" {
		return webserver.Caller{}, false
	}
	h := sum(ck.Value)
	var who webserver.Caller
	row, err := sessionBy(r.Context(), a.s.Read(), h, &who)
	if err != nil {
		return webserver.Caller{}, false
	}
	created, rotated, seen, expires, localAuthAt, family := row.created, row.rotated, row.seen, row.expires, row.localAuthAt, row.family
	a.fill(&who, row.perms)
	now := a.now()
	localAt := parseTime(localAuthAt)
	localMethod := who.AuthMethod == AuthMethodLocalPassword || who.AuthMethod == AuthMethodLocalSetup
	who.LocalRecent = localMethod && !localAt.IsZero() && !localAt.After(now) && now.Sub(localAt) < localRecentFor
	switch who.AuthMethod {
	case ProviderPlex, ProviderJellyfin, ProviderEmby:
		at := parseTime(created)
		who.OutsideRecent = !at.IsZero() && !at.After(now) && now.Sub(at) < localRecentFor
	}
	if strings.HasPrefix(family, setupFamily) {
		at := parseTime(created)
		who.SetupRecent = !at.IsZero() && !at.After(now) && now.Sub(at) < setupRecentFor
	}
	if !now.Before(parseTime(expires)) || now.Sub(parseTime(seen)) > IdleTimeout {
		_ = a.s.WriteCore(r.Context(), func(tx *sql.Tx) error {
			return deleteSession(r.Context(), tx, h)
		})
		a.setCookie(w, r, "", -1)
		return webserver.Caller{}, false
	}
	who.Via = "session"
	t := database.FormatTime(now)
	switch {
	case now.Sub(parseTime(rotated)) > RotateEvery:
		tok := token(32)
		abs := parseTime(created).Add(AbsoluteTimeout)
		err := a.s.WriteCore(r.Context(), func(tx *sql.Tx) error {
			if err := copySessionRotated(r.Context(), tx, sum(tok), t, h); err != nil {
				return err
			}
			// The old id lives on briefly for requests already in flight.
			grace := now.Add(rotateGrace)
			if grace.After(abs) {
				grace = abs
			}
			return retireSession(r.Context(), tx, database.FormatTime(grace), t, h)
		})
		if err == nil {
			a.setCookie(w, r, tok, int(abs.Sub(now).Seconds()))
		}
	case now.Sub(parseTime(seen)) > touchEvery:
		_ = a.s.WriteCore(r.Context(), func(tx *sql.Tx) error {
			return touchSession(r.Context(), tx, t, h)
		})
	}
	return who, true
}

func (a *Auth) logout(w http.ResponseWriter, r *http.Request) {
	if who, ok := webserver.From(r.Context()); ok && who.UserID != 0 {
		a.event(r, webserver.Actor(r.Context()), "signed_out", who.Name+" signed out", map[string]any{"name": who.Name})
	}
	if ck, err := r.Cookie(CookieName); err == nil {
		_ = a.s.WriteCore(r.Context(), func(tx *sql.Tx) error {
			return deletePresentedFamily(tx, ck.Value)
		})
	}
	a.setCookie(w, r, "", -1)
	// The browser drops what it kept for this person, such as Explore rows.
	w.Header().Set("Clear-Site-Data", `"cache"`)
	w.WriteHeader(http.StatusNoContent)
}

func (a *Auth) me(w http.ResponseWriter, r *http.Request) {
	who, _ := webserver.From(r.Context())
	// People who joined through a media server have no password until they set one.
	hasPw := true
	if has, err := hasPassword(r.Context(), a.s.Read(), who.UserID); err == nil {
		hasPw = has
	}
	// The owner stays in the first-run wizard until it is finished.
	setupOpen := false
	if who.Role == webserver.RoleOwner {
		setupOpen = database.SettingOr(r.Context(), a.s.Read(), settingsGroup, settingFirstRunDone, "") != "1"
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": who.UserID, "name": who.Name, "via": who.Via, "csrf": who.CSRF, "role": who.Role,
		"perms": who.Effective(), "two_factor": who.TwoFactor, "must_two_factor": who.MustTwoFactor, "auth_method": who.AuthMethod,
		"local_recent": who.LocalRecent, "outside_recent": who.OutsideRecent, "setup_recent": who.SetupRecent, "recent_proof": RecentLocal(r), "setup_open": setupOpen, "has_password": hasPw,
		"avatar": a.avatarOf(r.Context(), who.UserID), "safe_mode": a.safeMode, "held": startup.FeaturesOff(), "max_rating": who.MaxRating, "rating_country": countryOr(who.RatingCountry),
		"views": a.viewsOf(r.Context(), who.UserID)})
}
