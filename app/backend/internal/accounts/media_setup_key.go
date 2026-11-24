package accounts

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// setupKeyHold keeps the API key that a Jellyfin or Emby first-run sign-in
// made on the server, so the Connect step uses it instead of asking the owner
// to paste one. It lives in memory for a short while only; after a restart
// the Connect step asks for a key as before.
type setupKeyHold struct {
	mu     sync.Mutex
	kind   string
	origin string
	key    string
	until  time.Time
}

var setupKeys sync.Map // *Auth -> *setupKeyHold

const setupKeyLife = 6 * time.Hour

func (a *Auth) setupKeyHold() *setupKeyHold {
	h, _ := setupKeys.LoadOrStore(a, &setupKeyHold{})
	return h.(*setupKeyHold)
}

// makeSetupKey asks the server for an API key named after HolyArrs with the
// owner's fresh sign-in, and keeps it. Any failure just leaves no key.
func (a *Auth) makeSetupKey(ctx context.Context, kind, addr, authz, session string) {
	hdr := map[string]string{"Authorization": fmt.Sprintf("%s, Token=%q", authz, session), "X-Emby-Token": session}
	if code, err := a.call(ctx, a.tester, http.MethodPost, addr+"/Auth/Keys?App="+product, hdr, nil, nil); err != nil || code/100 != 2 {
		return
	}
	var list struct {
		Items []struct {
			AccessToken string `json:"AccessToken"`
			AppName     string `json:"AppName"`
		} `json:"Items"`
	}
	if code, err := a.call(ctx, a.tester, http.MethodGet, addr+"/Auth/Keys", hdr, nil, &list); err != nil || code/100 != 2 {
		return
	}
	key := ""
	for _, it := range list.Items {
		if it.AppName == product && it.AccessToken != "" {
			key = it.AccessToken // the newest is last
		}
	}
	if key == "" {
		return
	}
	h := a.setupKeyHold()
	h.mu.Lock()
	h.kind, h.origin, h.key, h.until = kind, keyOrigin(addr), key, a.now().Add(setupKeyLife)
	h.mu.Unlock()
}

// MediaSignInKey is the API key the owner's first-run Jellyfin or Emby
// sign-in made, for that same server only.
func (a *Auth) MediaSignInKey(_ context.Context, kind, rawURL string) (string, error) {
	h := a.setupKeyHold()
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.key == "" || h.kind != kind || h.origin != keyOrigin(rawURL) || a.now().After(h.until) {
		return "", errors.New("no key from sign-in for that server")
	}
	return h.key, nil
}

func keyOrigin(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Scheme + "://" + u.Host)
}

// dropSetupKey forgets the kept key of one kind (the owner unlinked it).
func (a *Auth) dropSetupKey(kind string) {
	h := a.setupKeyHold()
	h.mu.Lock()
	if h.kind == kind {
		h.kind, h.origin, h.key = "", "", ""
	}
	h.mu.Unlock()
}

// FirstRunDone says the first-run wizard finished.
func FirstRunDone(ctx context.Context, q database.Querier) bool {
	return database.SettingOr(ctx, q, settingsGroup, settingFirstRunDone, "") == "1"
}
