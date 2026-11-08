package accounts

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// SessionView is one signed-in device as the screens show it.
type SessionView struct {
	ID       string    `json:"id"`
	Device   string    `json:"device"`
	Created  time.Time `json:"created_at"`
	LastUsed time.Time `json:"last_used_at"`
	Expires  time.Time `json:"expires_at"`
	// Current marks the session the request came with.
	Current bool `json:"current"`
}

// Device names a browser and system from a User-Agent in a few words,
// such as "Firefox on Windows"; it never repeats the raw text.
func Device(ua string) string {
	l := strings.ToLower(ua)
	if l == "" {
		return "Unknown device"
	}
	browser := ""
	switch {
	case strings.Contains(l, "edg/"):
		browser = "Edge"
	case strings.Contains(l, "opr/") || strings.Contains(l, "opera"):
		browser = "Opera"
	case strings.Contains(l, "firefox/") || strings.Contains(l, "fxios/"):
		browser = "Firefox"
	case strings.Contains(l, "chrome/") || strings.Contains(l, "crios/") || strings.Contains(l, "chromium/"):
		browser = "Chrome"
	case strings.Contains(l, "safari/"):
		browser = "Safari"
	case strings.HasPrefix(l, "curl/"):
		browser = "curl"
	case strings.Contains(l, "python"):
		browser = "Python script"
	case strings.HasPrefix(l, "go-http-client"):
		browser = "Go program"
	}
	system := ""
	switch {
	case strings.Contains(l, "iphone"):
		system = "iPhone"
	case strings.Contains(l, "ipad"):
		system = "iPad"
	case strings.Contains(l, "android"):
		system = "Android"
	case strings.Contains(l, "windows"):
		system = "Windows"
	case strings.Contains(l, "mac os x") || strings.Contains(l, "macintosh"):
		system = "Mac"
	case strings.Contains(l, "cros"):
		system = "ChromeOS"
	case strings.Contains(l, "linux"):
		system = "Linux"
	}
	switch {
	case browser != "" && system != "":
		return browser + " on " + system
	case browser != "":
		return browser
	case system != "":
		return "A browser on " + system
	}
	return "Unknown device"
}

// newFamily is a session's public id: random, never the cookie or its hash.
func newFamily() string { return token(16) }

func (a *Auth) sessionRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/auth/sessions", a.mySessions)
	m.HandleFunc("DELETE /api/v1/auth/sessions", a.endMyOtherSessions)
	m.HandleFunc("DELETE /api/v1/auth/sessions/{sid}", a.endMySession)
	m.HandleFunc("GET /api/v1/users/{id}/sessions", a.userSessions)
	m.HandleFunc("POST /api/v1/users/{id}/sessions/end-all", a.endUserSessions)
	m.HandleFunc("DELETE /api/v1/users/{id}/sessions/{sid}", a.endUserSession)
}

// currentFamily is the public id of the session the request came with
// ("" for an API key or the local network).
func (a *Auth) currentFamily(r *http.Request) string {
	ck, err := r.Cookie(CookieName)
	if err != nil || ck.Value == "" {
		return ""
	}
	f, _ := sessionFamily(r.Context(), a.s.Read(), sum(ck.Value))
	return f
}

// listSessions lists a person's live sessions, newest use first.
func (a *Auth) listSessions(ctx context.Context, userID int64, current string) ([]SessionView, error) {
	return liveSessions(ctx, a.s.Read(), userID, a.now(), IdleTimeout, current)
}

// endSessions ends a person's sessions: one (sid), or every one but keep.
func (a *Auth) endSessions(ctx context.Context, userID int64, sid, keep string) (int64, error) {
	var n int64
	err := a.s.WriteCore(ctx, func(tx *sql.Tx) error {
		var res sql.Result
		var err error
		if sid != "" {
			res, err = deleteUserSession(ctx, tx, userID, sid)
		} else {
			res, err = deleteUserSessionsExcept(ctx, tx, userID, keep)
		}
		if err != nil {
			return err
		}
		n, err = res.RowsAffected()
		return err
	})
	return n, err
}

func (a *Auth) mySessions(w http.ResponseWriter, r *http.Request) {
	who, _ := webserver.From(r.Context())
	out, err := a.listSessions(r.Context(), who.UserID, a.currentFamily(r))
	if err != nil {
		serverError(w, "sessions", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": out})
}

// endMyOtherSessions signs the person out everywhere but here.
func (a *Auth) endMyOtherSessions(w http.ResponseWriter, r *http.Request) {
	// An API key or the local network has no current session: "other"
	// would be every session.
	if who, ok := webserver.From(r.Context()); ok && who.Via != "session" {
		fail(w, http.StatusBadRequest, "use a signed-in browser session; an API key has no session of its own to keep")
		return
	}
	if !webserver.RequireStrong(w, r) {
		return
	}
	who, _ := webserver.From(r.Context())
	n, err := a.endSessions(r.Context(), who.UserID, "", a.currentFamily(r))
	if err != nil {
		serverError(w, "sessions", err)
		return
	}
	log.Printf("auth: user %d signed out %d other sessions", who.UserID, n)
	a.accountEvent(r, who.UserID, "sessions_ended", who.Name+" signed out their other sessions", map[string]any{"ended": n})
	writeJSON(w, http.StatusOK, map[string]any{"ended": n})
}

func (a *Auth) endMySession(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	who, _ := webserver.From(r.Context())
	n, err := a.endSessions(r.Context(), who.UserID, r.PathValue("sid"), "")
	if err != nil {
		serverError(w, "sessions", err)
		return
	}
	if n == 0 {
		fail(w, http.StatusNotFound, "no such session")
		return
	}
	log.Printf("auth: user %d signed out one session", who.UserID)
	a.accountEvent(r, who.UserID, "session_ended", who.Name+" signed out one of their sessions", nil)
	w.WriteHeader(http.StatusNoContent)
}

// otherUser finds the person whose sessions an owner or admin looks at.
// The owner reaches everyone; an admin reaches members and themselves.
func (a *Auth) otherUser(w http.ResponseWriter, r *http.Request) (int64, bool) {
	by, _ := webserver.From(r.Context())
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		fail(w, http.StatusNotFound, "no such user")
		return 0, false
	}
	role, err := roleOfUser(r.Context(), a.s.Read(), id)
	if errors.Is(err, sql.ErrNoRows) {
		fail(w, http.StatusNotFound, "no such user")
		return 0, false
	}
	if err != nil {
		serverError(w, "sessions", err)
		return 0, false
	}
	if by.Role != webserver.RoleOwner && id != by.UserID && role != webserver.RoleMember {
		fail(w, http.StatusForbidden, "only the owner can see or end the owner's and admins' sessions")
		return 0, false
	}
	return id, true
}

func (a *Auth) userSessions(w http.ResponseWriter, r *http.Request) {
	id, ok := a.otherUser(w, r)
	if !ok {
		return
	}
	out, err := a.listSessions(r.Context(), id, a.currentFamily(r))
	if err != nil {
		serverError(w, "sessions", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": out})
}

// endUserSessions ends every session of a person (the caller's own
// session stays when it is theirs).
func (a *Auth) endUserSessions(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	id, ok := a.otherUser(w, r)
	if !ok {
		return
	}
	by, _ := webserver.From(r.Context())
	n, err := a.endSessions(r.Context(), id, "", a.currentFamily(r))
	if err != nil {
		serverError(w, "sessions", err)
		return
	}
	log.Printf("auth: user %d signed out %d sessions of user %d", by.UserID, n, id)
	a.accountEvent(r, id, "sessions_ended", byName(r)+" signed "+a.userName(r.Context(), id)+" out everywhere", map[string]any{"ended": n})
	writeJSON(w, http.StatusOK, map[string]any{"ended": n})
}

func (a *Auth) endUserSession(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	id, ok := a.otherUser(w, r)
	if !ok {
		return
	}
	by, _ := webserver.From(r.Context())
	n, err := a.endSessions(r.Context(), id, r.PathValue("sid"), "")
	if err != nil {
		serverError(w, "sessions", err)
		return
	}
	if n == 0 {
		fail(w, http.StatusNotFound, "no such session")
		return
	}
	log.Printf("auth: user %d signed out one session of user %d", by.UserID, id)
	a.accountEvent(r, id, "session_ended", byName(r)+" signed "+a.userName(r.Context(), id)+" out of one session", nil)
	w.WriteHeader(http.StatusNoContent)
}
