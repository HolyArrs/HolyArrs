package accounts

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// Linking a person to their account on one of the owner's connected Jellyfin
// or Emby servers, so their watched marks show: the person signs in to that
// server's account themselves (the password goes to the server only, never
// kept), or the owner picks the account in Settings > Users. Only the
// account's id there is kept, on the person's media server link.

// LinkServer is a Jellyfin or Emby server the owner connected, as linking
// sees it. URL never leaves the server.
type LinkServer struct {
	ID   int64
	Name string
	Kind string
	URL  string
}

// ServerAccount is one account on a media server.
type ServerAccount struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// TakenBy and TakenName name the person the account is already linked to.
	TakenBy   int64  `json:"taken_by,omitempty"`
	TakenName string `json:"taken_name,omitempty"`
}

// MediaLinks reaches the owner's connected media servers. Servers lists the
// switched-on Jellyfin and Emby servers; Accounts reads one server's
// accounts with its kept key; Linked runs after a link changed, so the
// person's watched marks come without waiting for the next read.
type MediaLinks struct {
	Servers  func(ctx context.Context) ([]LinkServer, error)
	Accounts func(ctx context.Context, serverID int64) ([]ServerAccount, error)
	Linked   func()
	// Forget deletes the saved media servers of one kind (unlink during first-run setup).
	Forget func(ctx context.Context, kind string) error
}

// SetMediaLinks wires the media servers in; call it before serving.
func (a *Auth) SetMediaLinks(m MediaLinks) { a.media = m }

func (a *Auth) mediaLinkRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/auth/media-link", a.myMediaLinks)
	m.HandleFunc("POST /api/v1/auth/media-link/{server}", a.myMediaLink)
	m.HandleFunc("DELETE /api/v1/auth/media-link/{kind}", a.myMediaUnlink)
	m.HandleFunc("GET /api/v1/users/media-accounts", a.serverAccounts)
	m.HandleFunc("PUT /api/v1/users/{id}/media-link", a.userMediaLink)
	m.HandleFunc("DELETE /api/v1/users/{id}/media-link/{kind}", a.userMediaUnlink)
}

// linkServers lists the servers a person may link to.
func (a *Auth) linkServers(ctx context.Context) ([]LinkServer, error) {
	if a.media.Servers == nil {
		return nil, nil
	}
	ms, err := a.media.Servers(ctx)
	out := ms[:0:0]
	for _, m := range ms {
		if m.Kind == ProviderJellyfin || m.Kind == ProviderEmby {
			out = append(out, m)
		}
	}
	return out, err
}

func (a *Auth) linkServer(ctx context.Context, raw string) (LinkServer, bool, error) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return LinkServer{}, false, nil
	}
	ms, err := a.linkServers(ctx)
	for _, m := range ms {
		if m.ID == id {
			return m, true, err
		}
	}
	return LinkServer{}, false, err
}

// mediaServerView is a server as people see it: no address.
type mediaServerView struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// myMediaLinks lists the servers I may link to and my Jellyfin and Emby links.
func (a *Auth) myMediaLinks(w http.ResponseWriter, r *http.Request) {
	who, ok := webserver.From(r.Context())
	if !ok || who.UserID == 0 {
		fail(w, http.StatusForbidden, "sign in first")
		return
	}
	ms, err := a.linkServers(r.Context())
	if err != nil {
		serverError(w, "media link", err)
		return
	}
	servers := []mediaServerView{}
	for _, m := range ms {
		servers = append(servers, mediaServerView{m.ID, m.Name, m.Kind})
	}
	links, err := mediaLinksOf(r.Context(), a.s.Read(), who.UserID)
	if err != nil {
		serverError(w, "media link", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"servers": servers, "links": links})
}

// myMediaLink signs in to my account on one of the owner's servers and links
// it to me, replacing my earlier link of that kind.
func (a *Auth) myMediaLink(w http.ResponseWriter, r *http.Request) {
	who, ok := a.requireSSOLink(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	m, found, err := a.linkServer(ctx, r.PathValue("server"))
	if err != nil {
		serverError(w, "media link", err)
		return
	}
	if !found {
		fail(w, http.StatusNotFound, "no such media server")
		return
	}
	got, _, ok := a.checkMediaServer(w, r, m.Kind, WayView{URL: m.URL})
	if !ok {
		return
	}
	// The server took a while: the session must still be there and fresh.
	cookie, _ := sessionCookie(r)
	proof := ssoLinkProof{family: a.currentFamily(r), userID: who.UserID}
	if cookie != nil && cookie.Value != "" {
		proof.hash = sum(cookie.Value)
	}
	err = a.s.WriteCore(ctx, func(tx *sql.Tx) error {
		if proof.hash == "" || proof.family == "" {
			return errNotAllowed
		}
		if err := a.mediaLinkSessionFreshTx(tx, proof); err != nil {
			return err
		}
		return a.linkMediaTx(ctx, tx, who.UserID, m, got.ID, got.Name)
	})
	if !a.mediaLinkDone(w, r, who.UserID, m.Kind, got.Name, err) {
		return
	}
	log.Printf("auth: user %d linked a %s account from My account", who.UserID, m.Kind)
	writeJSON(w, http.StatusOK, LinkView{Provider: m.Kind, Name: got.Name})
}

// mediaLinkDone answers a failed link, or records a made one and says true.
func (a *Auth) mediaLinkDone(w http.ResponseWriter, r *http.Request, userID int64, kind, name string, err error) bool {
	switch {
	case errors.Is(err, errTaken):
		what := "this " + kind + " account"
		fail(w, http.StatusConflict, what+" is already linked to another HolyArrs user")
		return false
	case errors.Is(err, errNotAllowed):
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "confirm your local password again before linking", "code": "reauth"})
		return false
	case errors.Is(err, errNoWayIn):
		if who, _ := webserver.From(r.Context()); who.UserID == userID {
			fail(w, http.StatusConflict, "this is your only way to sign in; set a HolyArrs password first")
		} else {
			fail(w, http.StatusConflict, "this is their only way to sign in; set them a password first")
		}
		return false
	case err != nil:
		serverError(w, "media link", err)
		return false
	}
	a.accountEvent(r, userID, "sign_in_linked", byName(r)+" linked the "+kind+" account "+outsideName(name)+" to "+a.userName(r.Context(), userID), map[string]any{"provider": kind})
	if a.media.Linked != nil {
		a.media.Linked()
	}
	return true
}

// linkMediaTx links account extID of a Jellyfin or Emby server to userID,
// replacing the person's earlier link of that kind; the sessions that
// earlier account signed in end. errTaken: it belongs to someone else.
// errNoWayIn: the earlier link is the person's only way in and the new
// account is not on the sign-in server, so they could no longer sign in.
func (a *Auth) linkMediaTx(ctx context.Context, tx *sql.Tx, userID int64, m LinkServer, extID, name string) error {
	kind := m.Kind
	other, err := linkOwner(ctx, tx, kind, extID)
	switch {
	case err == nil && other != userID:
		return errTaken
	case err == nil:
		return nil // linked already
	case !errors.Is(err, sql.ErrNoRows):
		return err
	}
	if err := a.relinkKeepsWayIn(ctx, tx, userID, m); err != nil {
		return err
	}
	n, err := deleteMediaLink(ctx, tx, userID, kind)
	if err != nil {
		return err
	}
	if n > 0 {
		if _, err := deleteSessionsByMethod(ctx, tx, userID, kind); err != nil {
			return err
		}
	}
	return insertUserLink(ctx, tx, userID, kind, extID, truncate(name, 200), database.FormatTime(a.now()))
}

// relinkKeepsWayIn refuses replacing a link that is the person's only way
// in, unless the new server is the one people sign in with.
func (a *Auth) relinkKeepsWayIn(ctx context.Context, tx *sql.Tx, userID int64, m LinkServer) error {
	if sameServer(a.way(ctx, m.Kind).URL, m.URL) {
		return nil
	}
	links, err := mediaLinksOf(ctx, tx, userID)
	if err != nil {
		return err
	}
	had := false
	for _, l := range links {
		had = had || l.Provider == m.Kind
	}
	if !had {
		return nil
	}
	hash, err := passwordHashOf(ctx, tx, userID)
	if err != nil {
		return err
	}
	others, err := waysInBesidesLink(ctx, tx, userID, m.Kind)
	if err != nil {
		return err
	}
	if hash == "" && others == 0 {
		return errNoWayIn
	}
	return nil
}

func sameServer(a, b string) bool {
	a = strings.TrimRight(strings.TrimSpace(a), "/")
	b = strings.TrimRight(strings.TrimSpace(b), "/")
	return a != "" && strings.EqualFold(a, b)
}

// myMediaUnlink removes my link of one kind; refused when it is my only way in.
func (a *Auth) myMediaUnlink(w http.ResponseWriter, r *http.Request) {
	who, ok := webserver.From(r.Context())
	if !ok || who.UserID == 0 || !RecentLocal(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "confirm your local password again before using this route", "code": "reauth"})
		return
	}
	a.unlinkMedia(w, r, who.UserID, r.PathValue("kind"), true)
}

func (a *Auth) unlinkMedia(w http.ResponseWriter, r *http.Request, userID int64, kind string, self bool) {
	if kind != ProviderJellyfin && kind != ProviderEmby {
		fail(w, http.StatusNotFound, "no such linked sign-in")
		return
	}
	ctx := r.Context()
	inSetup := false
	err := a.s.WriteCore(ctx, func(tx *sql.Tx) error {
		hash, err := passwordHashOf(ctx, tx, userID)
		if err != nil {
			return err
		}
		others, err := waysInBesidesLink(ctx, tx, userID, kind)
		if err != nil {
			return err
		}
		if hash == "" && others == 0 {
			return errNoWayIn
		}
		n, err := deleteMediaLink(ctx, tx, userID, kind)
		if err != nil {
			return err
		}
		if n == 0 {
			return sql.ErrNoRows
		}
		// During first-run setup the link turned this sign-in way on (linkSetupServerTx); unlinking it there turns it off again.
		inSetup = database.SettingOr(ctx, tx, settingsGroup, settingFirstRunDone, "") != "1"
		if inSetup {
			g := signInPrefix + kind
			if err := database.PutSetting(tx, settingsGroup, g+"_on", "false", domain.System, a.now()); err != nil {
				return err
			}
			if err := database.PutSetting(tx, settingsGroup, g+"_url", "", domain.System, a.now()); err != nil {
				return err
			}
		}
		_, err = deleteSessionsByMethod(ctx, tx, userID, kind)
		return err
	})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		fail(w, http.StatusNotFound, "no such linked sign-in")
		return
	case errors.Is(err, errNoWayIn):
		if self {
			fail(w, http.StatusConflict, "this is your only way to sign in; set a HolyArrs password first")
		} else {
			fail(w, http.StatusConflict, "this is their only way to sign in; set them a password first")
		}
		return
	case err != nil:
		serverError(w, "media unlink", err)
		return
	}
	// During setup the server saved for this link goes too, so linking again starts clean.
	if inSetup {
		a.forgetSetupServer(ctx, kind)
	}
	a.dropTickets(func(t *ticket) bool { return t.userID == userID && t.method == kind })
	a.accountEvent(r, userID, "sign_in_unlinked", byName(r)+" unlinked the "+kind+" account of "+a.userName(ctx, userID), map[string]any{"provider": kind})
	if a.media.Linked != nil {
		a.media.Linked()
	}
	w.WriteHeader(http.StatusNoContent)
}

// forgetSetupServer drops what a first-run link of kind left behind: the
// media server saved for it and the API key kept for the Media server step.
func (a *Auth) forgetSetupServer(ctx context.Context, kind string) {
	a.dropSetupKey(kind)
	if a.media.Forget != nil {
		if err := a.media.Forget(ctx, kind); err != nil {
			log.Printf("accounts: setup unlink %s: forget media server: %v", kind, err)
		}
	}
}

// serverAccounts lists every connected Jellyfin and Emby server with its
// accounts, for the owner picking one in Settings > Users. A server that
// does not answer comes with an empty list and its error.
func (a *Auth) serverAccounts(w http.ResponseWriter, r *http.Request) {
	if !ownerOnly(w, r) {
		return
	}
	ms, err := a.linkServers(r.Context())
	if err != nil {
		serverError(w, "media accounts", err)
		return
	}
	type serverView struct {
		mediaServerView
		Accounts []ServerAccount `json:"accounts"`
		Error    string          `json:"error,omitempty"`
	}
	out := []serverView{}
	for _, m := range ms {
		v := serverView{mediaServerView: mediaServerView{m.ID, m.Name, m.Kind}, Accounts: []ServerAccount{}}
		if got, err := a.media.Accounts(r.Context(), m.ID); err != nil {
			log.Printf("auth: accounts of %s: %v", m.Name, err)
			kind := m.Kind
			v.Error = "the " + kind + " server did not answer"
		} else if got != nil {
			v.Accounts = got
			taken, err := linkedAccounts(r.Context(), a.s.Read(), m.Kind)
			if err != nil {
				serverError(w, "media accounts", err)
				return
			}
			for i := range v.Accounts {
				if p, ok := taken[v.Accounts[i].ID]; ok {
					v.Accounts[i].TakenBy, v.Accounts[i].TakenName = p.ID, p.Name
				}
			}
		}
		out = append(out, v)
	}
	writeJSON(w, http.StatusOK, map[string]any{"servers": out})
}

// userMediaLink links a person to an account the owner picked on a server.
func (a *Auth) userMediaLink(w http.ResponseWriter, r *http.Request) {
	if !ownerOnly(w, r) {
		return
	}
	ctx := r.Context()
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		fail(w, http.StatusNotFound, "no such user")
		return
	}
	if _, err := roleOfUser(ctx, a.s.Read(), id); err != nil {
		fail(w, http.StatusNotFound, "no such user")
		return
	}
	var in struct {
		Server  int64  `json:"server"`
		Account string `json:"account"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&in); err != nil || in.Account == "" {
		fail(w, http.StatusBadRequest, `send {"server": 1, "account": "..."}`)
		return
	}
	m, found, err := a.linkServer(ctx, strconv.FormatInt(in.Server, 10))
	if err != nil {
		serverError(w, "media link", err)
		return
	}
	if !found {
		fail(w, http.StatusNotFound, "no such media server")
		return
	}
	// Only an account the server lists: a request cannot invent one.
	accts, err := a.media.Accounts(ctx, m.ID)
	if err != nil {
		log.Printf("auth: accounts of %s: %v", m.Name, err)
		kind := m.Kind
		fail(w, http.StatusBadGateway, "the "+kind+" server did not answer")
		return
	}
	var pick *ServerAccount
	for i := range accts {
		if accts[i].ID == in.Account {
			pick = &accts[i]
		}
	}
	if pick == nil {
		fail(w, http.StatusNotFound, "no such account on that server")
		return
	}
	err = a.s.WriteCore(ctx, func(tx *sql.Tx) error { return a.linkMediaTx(ctx, tx, id, m, pick.ID, pick.Name) })
	if !a.mediaLinkDone(w, r, id, m.Kind, pick.Name, err) {
		return
	}
	writeJSON(w, http.StatusOK, LinkView{Provider: m.Kind, Name: pick.Name})
}

func (a *Auth) userMediaUnlink(w http.ResponseWriter, r *http.Request) {
	if !ownerOnly(w, r) {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		fail(w, http.StatusNotFound, "no such user")
		return
	}
	a.unlinkMedia(w, r, id, r.PathValue("kind"), false)
}

// ownerOnly lets only the owner, on a strong sign-in, see or change other
// people's media server accounts, besides the route gate; it answers 403
// and says false for anyone else.
func ownerOnly(w http.ResponseWriter, r *http.Request) bool {
	if !webserver.RequireStrong(w, r) {
		return false
	}
	if who, ok := webserver.From(r.Context()); !ok || who.Role != webserver.RoleOwner {
		fail(w, http.StatusForbidden, "only the owner links people to server accounts")
		return false
	}
	return true
}
