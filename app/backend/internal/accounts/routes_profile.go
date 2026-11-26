package accounts

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

const displayNameMax = 64

type myProfile struct {
	DisplayName string `json:"display_name"`
	Username    string `json:"username"`
	Language    string `json:"language"`
	OpensOn     string `json:"opens_on"`
}

// opensOnPages are the pages a person may have the app open on.
var opensOnPages = map[string]bool{"home": true, "explore": true}

// languages the app has words for; "" follows the browser.
var languages = map[string]bool{"": true, "en": true, "es": true, "pt-br": true, "fr": true, "de": true, "ru": true, "uk": true, "zh-cn": true, "ja": true, "ko": true, "hi": true, "ar": true, "it": true, "nl": true, "pl": true, "tr": true,
	"pt-pt": true, "sv": true, "nb": true, "da": true, "fi": true, "cs": true, "hu": true, "el": true, "he": true, "ro": true, "id": true, "vi": true, "th": true, "zh-tw": true,
	"fa": true, "bg": true, "sk": true, "ca": true, "ms": true, "sr": true, "hr": true, "lt": true, "fil": true, "bn": true, "et": true, "lv": true, "is": true, "ta": true}

// My profile: the display name each person sets for themselves. The sign-in
// name (username) and the role are not editable here; an owner changes a
// role on Users and roles, and the username stays what people sign in with.
func (a *Auth) profileRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/my/profile", a.getProfile)
	m.HandleFunc("PUT /api/v1/my/profile", a.putProfile)
	m.HandleFunc("PUT /api/v1/my/language", a.putLanguage)
	m.HandleFunc("PUT /api/v1/my/opens-on", a.putOpensOn)
}

// cleanDisplayName trims the name and refuses control and hidden format
// characters (joiners inside a word stay) and names over 64 characters. An empty name means "show my username".
func cleanDisplayName(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > displayNameMax {
		return "", false
	}
	for _, r := range s {
		if unicode.IsControl(r) || (unicode.Is(unicode.Cf, r) && r != 0x200c && r != 0x200d) {
			return "", false
		}
	}
	return s, true
}

func (a *Auth) getProfile(w http.ResponseWriter, r *http.Request) {
	who, ok := webserver.From(r.Context())
	if !ok {
		fail(w, http.StatusUnauthorized, "sign in first")
		return
	}
	p := myProfile{Username: who.Name}
	var err error
	p.DisplayName, err = displayName(r.Context(), a.s.Read(), who.UserID)
	if err == nil {
		p.Language, err = language(r.Context(), a.s.Read(), who.UserID)
	}
	if err == nil {
		p.OpensOn, err = opensOn(r.Context(), a.s.Read(), who.UserID)
	}
	if p.OpensOn == "" {
		p.OpensOn = "home"
	}
	if err != nil && err != sql.ErrNoRows {
		webserver.Internal(w, "accounts", err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (a *Auth) putProfile(w http.ResponseWriter, r *http.Request) {
	var in struct {
		DisplayName string `json:"display_name"`
	}
	if !webserver.Decode(w, r, &in, `send {"display_name": "..."}`) {
		return
	}
	name, ok := cleanDisplayName(in.DisplayName)
	if !ok {
		fail(w, http.StatusBadRequest, "pick a display name of up to 64 characters")
		return
	}
	me, _ := webserver.From(r.Context())
	err := a.s.WriteCore(r.Context(), func(tx *sql.Tx) error {
		return putDisplayName(r.Context(), tx, me.UserID, name)
	})
	if err != nil {
		webserver.Internal(w, "accounts", err)
		return
	}
	a.getProfile(w, r)
}

func (a *Auth) putLanguage(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Language string `json:"language"`
	}
	if !webserver.Decode(w, r, &in, `send {"language": "de"}`) {
		return
	}
	l := strings.ToLower(strings.TrimSpace(in.Language))
	if !languages[l] {
		fail(w, http.StatusBadRequest, "pick one of the app's languages")
		return
	}
	me, _ := webserver.From(r.Context())
	err := a.s.WriteCore(r.Context(), func(tx *sql.Tx) error {
		return putLanguage(r.Context(), tx, me.UserID, l)
	})
	if err != nil {
		webserver.Internal(w, "accounts", err)
		return
	}
	a.getProfile(w, r)
}

// opensOnOf is the page a person's app opens on, for the session answer:
// "home" when it cannot be read, so the session never fails over it.
func (a *Auth) opensOnOf(ctx context.Context, userID int64) string {
	p, _ := opensOn(ctx, a.s.Read(), userID)
	return p
}

// putOpensOn saves the page the app opens on for the person signed in.
func (a *Auth) putOpensOn(w http.ResponseWriter, r *http.Request) {
	var in struct {
		OpensOn string `json:"opens_on"`
	}
	if !webserver.Decode(w, r, &in, `send {"opens_on": "explore"}`) {
		return
	}
	p := strings.ToLower(strings.TrimSpace(in.OpensOn))
	if !opensOnPages[p] {
		fail(w, http.StatusBadRequest, "pick home or explore")
		return
	}
	me, _ := webserver.From(r.Context())
	err := a.s.WriteCore(r.Context(), func(tx *sql.Tx) error {
		return putOpensOn(r.Context(), tx, me.UserID, p)
	})
	if err != nil {
		webserver.Internal(w, "accounts", err)
		return
	}
	a.getProfile(w, r)
}
