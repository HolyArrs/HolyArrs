package accounts

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// pinInvite starts the purpose of a Plex PIN started from an invite; the
// invite's hash follows, so the PIN finishes only that invite.
const pinInvite = "invite:"

// Joining through an invite with Plex, Jellyfin or Emby instead of a new
// password. The invite is the permission: the owner's join rule for that
// way does not apply, but the way must be on. The new user gets what a
// password join through the same invite would get, linked to the outside
// account. An outside account already linked to someone leaves the invite
// unused.

var errAlreadyLinked = errors.New("outside account already linked")

func (a *Auth) inviteOutsideRoutes(m *http.ServeMux) {
	m.HandleFunc("POST /api/v1/auth/invites/{token}/plex/start", a.invitePlexStart)
	m.HandleFunc("POST /api/v1/auth/invites/{token}/plex/finish", a.invitePlexFinish)
	m.HandleFunc("POST /api/v1/auth/invites/{token}/media-server/{kind}", a.inviteMediaServer)
}

// inviteOutsidePath says whether path is one of the calls above, for public.
func inviteOutsidePath(path string) bool {
	rest, ok := strings.CutPrefix(path, "/api/v1/auth/invites/")
	if !ok {
		return false
	}
	tok, way, ok := strings.Cut(rest, "/")
	if !ok || tok == "" {
		return false
	}
	switch way {
	case "plex/start", "plex/finish", "media-server/" + ProviderJellyfin, "media-server/" + ProviderEmby:
		return true
	}
	return false
}

// inviteOpen gates the invite token like the password join does and checks
// the invite is still open; otherwise it has answered.
func (a *Auth) inviteOpen(w http.ResponseWriter, r *http.Request) (string, bool) {
	key, ok := a.inviteGate(w, r)
	if !ok {
		return "", false
	}
	tok := r.PathValue("token")
	if _, _, _, _, _, ok := a.openInvite(r.Context(), a.s.Read(), tok); !ok {
		a.mu.Lock()
		a.noteFail(key)
		a.mu.Unlock()
		fail(w, http.StatusNotFound, "this invite link is used up, ended or wrong; ask for a new one")
		return "", false
	}
	return tok, true
}

func (a *Auth) invitePlexStart(w http.ResponseWriter, r *http.Request) {
	tok, ok := a.inviteOpen(w, r)
	if !ok {
		return
	}
	if !a.way(r.Context(), ProviderPlex).On {
		fail(w, http.StatusNotFound, "sign-in with Plex is off")
		return
	}
	a.startPin(w, r, pinInvite+sum(tok))
}

func (a *Auth) invitePlexFinish(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if _, ok := a.inviteGate(w, r); !ok {
		return
	}
	if !a.way(ctx, ProviderPlex).On {
		fail(w, http.StatusNotFound, "sign-in with Plex is off")
		return
	}
	tok := r.PathValue("token")
	pt, ok := a.checkPin(w, r, pinInvite+sum(tok))
	if !ok {
		return
	}
	user, _, ok := a.plexUser(ctx, w, pt)
	if !ok {
		return
	}
	extID := strconv.FormatInt(user.ID, 10)
	id, name, err := a.inviteJoin(ctx, tok, ProviderPlex, extID, user.Username)
	if err == nil {
		a.keepPlexToken(ctx, id, extID, pt) // for their Plex watchlist
	}
	a.inviteDone(w, r, ProviderPlex, id, name, err, place{})
}

func (a *Auth) inviteMediaServer(w http.ResponseWriter, r *http.Request) {
	tok, ok := a.inviteOpen(w, r)
	if !ok {
		return
	}
	kind := r.PathValue("kind")
	v, ok := a.mediaWay(w, r, kind)
	if !ok {
		return
	}
	got, pl, ok := a.checkMediaServer(w, r, kind, v)
	if !ok {
		return
	}
	id, name, err := a.inviteJoin(r.Context(), tok, kind, got.ID, got.Name)
	a.inviteDone(w, r, kind, id, name, err, pl)
}

// inviteJoin makes the invite's user from an outside account, links it and
// uses the invite up, all in one write: nothing is kept if a step fails.
func (a *Auth) inviteJoin(ctx context.Context, tok, provider, extID, extName string) (int64, string, error) {
	var id int64
	var name string
	err := a.s.WriteCore(ctx, func(tx *sql.Tx) error {
		n, err := countLinksOf(ctx, tx, provider, extID)
		if err != nil {
			return err
		}
		if n > 0 {
			return errAlreadyLinked
		}
		u, err := a.inviteUser(tx, tok)
		if err != nil {
			return err
		}
		if name, err = freeName(tx, outsideBase(provider, extName)); err != nil {
			return err
		}
		u.Name = name
		if id, err = insertUserTx(tx, &u, domain.System); err != nil {
			return err
		}
		if err := insertUserLink(ctx, tx, id, provider, extID, truncate(extName, 200), database.FormatTime(a.now())); err != nil {
			return err
		}
		return a.useInvite(tx, tok, id)
	})
	if err == nil {
		log.Printf("auth: invite used by new user %q (%d) through %s", name, id, provider)
	}
	return id, name, err
}

// inviteDone answers an invite join; a new user signs straight in.
func (a *Auth) inviteDone(w http.ResponseWriter, r *http.Request, provider string, id int64, name string, err error, pl place) {
	switch {
	case errors.Is(err, errInviteGone):
		fail(w, http.StatusNotFound, "this invite link is used up, ended or wrong; ask for a new one")
	case errors.Is(err, errAlreadyLinked):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "This account already has a HolyArrs account. Sign in instead.", "code": "linked"})
	default:
		a.outsideDone(w, r, provider, id, name, err, pl)
	}
}
