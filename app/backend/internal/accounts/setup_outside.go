package accounts

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// First-run setup through an outside account: the owner is made from a
// Plex, Jellyfin or Emby account instead of a name and password. As with
// setup(), whoever opens a fresh HolyArrs first makes the owner. No setup
// code. Every route here works only while there
// is no owner, and the owner row is made under the same in-transaction count
// and unique index as setup(), so two people cannot both become the owner.

// pinSetup marks a Plex PIN started to make the owner on first run.
const pinSetup = "setup"

// discoverPort is where Jellyfin and Emby answer the local network search.
const (
	discoverTo   = "255.255.255.255:7359"
	discoverWait = 1500 * time.Millisecond
	discoverMax  = 10
)

// setupWays are the ways the first-run screen offers to make the owner.
var setupWays = []string{"password", ProviderPlex, ProviderJellyfin, ProviderEmby}

var kindWord = map[string]string{ProviderJellyfin: "Jellyfin", ProviderEmby: "Emby"}

// foundServer is a Jellyfin or Emby server that answered the network search.
type foundServer struct {
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	Address string `json:"address"`
}

func (a *Auth) setupOutsideRoutes(m *http.ServeMux) {
	m.HandleFunc("POST /api/v1/auth/setup/plex/start", a.setupPlexStart)
	m.HandleFunc("POST /api/v1/auth/setup/plex/finish", a.setupPlexFinish)
	m.HandleFunc("POST /api/v1/auth/setup/media-server/{kind}", a.setupMediaServer)
	m.HandleFunc("GET /api/v1/auth/setup/discover", a.setupDiscover)
}

// setupOutsidePath says whether a request is one of the first-run calls
// that check the host name themselves (like POST /api/v1/auth/setup).
func setupOutsidePath(r *http.Request) bool {
	return r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/v1/auth/setup/")
}

// ownerTaken answers 409 when the owner exists already.
func (a *Auth) ownerTaken(w http.ResponseWriter, r *http.Request) bool {
	if _, _, exists := a.owner(r.Context()); exists {
		fail(w, http.StatusConflict, "the owner is already set up; sign in instead")
		return true
	}
	return false
}

// setupHost is setup()'s host rule: an allowed host, or a home name with no
// proxy in between, which is then saved with the owner. It answers 403
// itself when the name is refused.
func (a *Auth) setupHost(w http.ResponseWriter, r *http.Request) (string, bool) {
	if a.hostAllowed(r) {
		return "", true
	}
	h := normHost(r.Host)
	if !homeName(h) || a.anyProxy() || forwarded(r) {
		a.logRefusedHost(r.Host)
		fail(w, http.StatusForbidden, hostRefusedMsg(r.Host))
		return "", false
	}
	return h, true
}

// makeOwnerTx makes the owner with no local password, named after the
// outside account (made unique), and saves a home host name when one came.
func (a *Auth) makeOwnerTx(tx *sql.Tx, provider, extName, newHost string) (int64, string, []string, error) {
	n, err := countOwners(context.Background(), tx)
	if err != nil {
		return 0, "", nil, err
	}
	if n > 0 {
		return 0, "", nil, errOwnerExists
	}
	base := strings.TrimSpace(extName)
	if base == "" {
		base = provider + "-user"
	}
	base = truncate(base, 56)
	name := base
	for i := 2; ; i++ {
		c, err := countUsersNamed(context.Background(), tx, name)
		if err != nil {
			return 0, "", nil, err
		}
		if c == 0 {
			break
		}
		name = fmt.Sprintf("%s-%d", base, i)
	}
	id, err := insertUserTx(tx, &domain.User{Name: name, Role: webserver.RoleOwner, State: "active", Perms: "[]"}, domain.System)
	if err != nil || newHost == "" {
		return id, name, nil, err
	}
	saved := append(slices.Clone(a.savedHostList()), newHost)
	v, _ := json.Marshal(saved)
	return id, name, saved, database.PutSetting(tx, settingsGroup, signInPrefix+keyAllowedHosts, string(v), domain.System, a.now())
}

// ownerMade finishes an outside first-run setup: the saved host name, the
// log line and the session (no local password, so it is an outside session).
func (a *Auth) ownerMade(w http.ResponseWriter, r *http.Request, provider string, id int64, name string, saved []string) {
	if saved != nil {
		a.savedHosts.Store(&saved)
		log.Printf("auth: first-run setup on %s; the name is saved under Allowed host names", saved[len(saved)-1])
	}
	log.Printf("auth: first-run setup made the owner %q through %s from %s", name, provider, r.RemoteAddr)
	a.startSetupSession(w, r, id, name, http.StatusCreated, provider)
}

func (a *Auth) setupPlexStart(w http.ResponseWriter, r *http.Request) {
	if a.ownerTaken(w, r) {
		return
	}
	if _, ok := a.setupHost(w, r); !ok {
		return
	}
	a.startPin(w, r, pinSetup)
}

func (a *Auth) setupPlexFinish(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if a.ownerTaken(w, r) {
		return
	}
	newHost, ok := a.setupHost(w, r)
	if !ok {
		return
	}
	tok, ok := a.checkPin(w, r, pinSetup)
	if !ok {
		return
	}
	hdr := a.plexHeaders(ctx)
	hdr["X-Plex-Token"] = tok
	var user struct {
		ID       int64  `json:"id"`
		Username string `json:"username"`
	}
	code, err := a.fetch(ctx, http.MethodGet, a.plexURL+"/api/v2/user", hdr, nil, &user)
	if err != nil || code/100 != 2 || user.ID == 0 {
		log.Printf("auth: setup plex user: status %d: %v", code, err)
		fail(w, http.StatusBadGateway, "Plex did not say who you are; try again")
		return
	}
	// The server people must share, when the owner has exactly one (as
	// Settings picks it after linking). Best effort: none is fine.
	serverID := ""
	var res []struct {
		ClientIdentifier string `json:"clientIdentifier"`
		Provides         string `json:"provides"`
		Owned            bool   `json:"owned"`
	}
	if code, err := a.fetch(ctx, http.MethodGet, a.plexURL+"/api/v2/resources?includeHttps=1", hdr, nil, &res); err == nil && code/100 == 2 {
		var owned []string
		for _, x := range res {
			if x.Owned && strings.Contains(x.Provides, "server") && x.ClientIdentifier != "" {
				owned = append(owned, x.ClientIdentifier)
			}
		}
		if len(owned) == 1 {
			serverID = truncate(owned[0], 200)
		}
	}
	extID := strconv.FormatInt(user.ID, 10)
	var id int64
	var name string
	var saved []string
	err = a.s.WriteCore(ctx, func(tx *sql.Tx) error {
		var err error
		if id, name, saved, err = a.makeOwnerTx(tx, ProviderPlex, user.Username, newHost); err != nil {
			return err
		}
		// The link and its token, as linking Plex in Settings keeps them.
		if _, err := a.linkPlexTx(tx, id, extID, user.Username, tok, domain.System); err != nil {
			return err
		}
		g := signInPrefix + ProviderPlex
		if err := database.PutSetting(tx, settingsGroup, g+"_on", "true", domain.System, a.now()); err != nil {
			return err
		}
		if serverID != "" {
			return database.PutSetting(tx, settingsGroup, g+"_server_id", serverID, domain.System, a.now())
		}
		return nil
	})
	if a.setupFailed(w, err, "this Plex account") {
		return
	}
	if a.vault != nil {
		// Settings' "linked" token, used to list the people the server is shared with.
		if err := a.vault.SetSetting(ctx, settingsGroup, signInPrefix+keyPlexOwner, tok, domain.Actor{Kind: "user", ID: id}); err != nil {
			log.Printf("auth: setup: keeping the owner's Plex token: %v", err)
		}
	}
	a.ownerMade(w, r, ProviderPlex, id, name, saved)
}

// setupFailed answers a failed owner write; false when err is nil.
func (a *Auth) setupFailed(w http.ResponseWriter, err error, what string) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, errOwnerExists):
		fail(w, http.StatusConflict, "the owner is already set up; sign in instead")
	case errors.Is(err, errTaken):
		fail(w, http.StatusConflict, what+" is already linked to another HolyArrs user")
	default:
		log.Printf("auth: setup: %v", err)
		fail(w, http.StatusInternalServerError, "something went wrong; see the log")
	}
	return true
}

// serverAddress tidies a typed Jellyfin or Emby address: http:// when no
// scheme is given, no trailing slash. ok is false when it is not one.
func serverAddress(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	if !strings.Contains(s, "://") {
		s = "http://" + s
	}
	u, err := url.Parse(strings.TrimRight(s, "/"))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", false
	}
	out := strings.TrimRight(u.String(), "/")
	if len(out) > 500 {
		return "", false
	}
	return out, true
}

func (a *Auth) setupMediaServer(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	kind := r.PathValue("kind")
	word := kindWord[kind]
	if word == "" {
		fail(w, http.StatusNotFound, "no such sign-in way")
		return
	}
	if a.ownerTaken(w, r) {
		return
	}
	newHost, ok := a.setupHost(w, r)
	if !ok {
		return
	}
	var body struct {
		URL      string `json:"url"`
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, `send {"url": "...", "name": "...", "password": "..."}`)
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	addr, ok := serverAddress(body.URL)
	if !ok {
		fail(w, http.StatusBadRequest, "give the "+word+" address, like http://192.168.1.10:8096")
		return
	}
	if body.Name == "" || len(body.Name) > 128 || len(body.Password) > MaxPassword {
		fail(w, http.StatusBadRequest, "give your "+word+" name and password")
		return
	}
	key := "media:" + a.clientKey(r)
	if a.ownerSetupLimited(key) {
		w.Header().Set("Retry-After", strconv.Itoa(int(setupWindow.Seconds())))
		fail(w, http.StatusTooManyRequests, "too many tries; wait a minute and try again")
		return
	}
	// Count the try before the call, so many tries at once cannot all pass
	// the limit while the first calls are still out.
	a.noteOwnerSetupFail(key)
	authz := fmt.Sprintf(`MediaBrowser Client="%s", Device="%s", DeviceId="%s", Version="1"`, product, product, a.clientID(ctx))
	hdr := map[string]string{"Authorization": authz, "X-Emby-Authorization": authz}
	var got struct {
		User struct {
			ID     string `json:"Id"`
			Name   string `json:"Name"`
			Policy struct {
				IsAdministrator bool `json:"IsAdministrator"`
				IsDisabled      bool `json:"IsDisabled"`
			} `json:"Policy"`
		} `json:"User"`
		AccessToken string `json:"AccessToken"`
	}
	// The address is typed before any owner exists, so the call goes through the
	// address-test client, which refuses link-local and metadata addresses.
	code, err := a.call(ctx, a.tester, http.MethodPost, addr+"/Users/AuthenticateByName", hdr, map[string]string{"Username": body.Name, "Pw": body.Password}, &got)
	notServer := "that address did not answer like a " + word + " server; check it and try again"
	switch {
	case err != nil && code == 0:
		log.Printf("auth: setup %s at %s: %v", kind, addr, err)
		fail(w, http.StatusBadGateway, "could not reach "+addr+"; check the address and that the server is on")
		return
	case code == http.StatusNotFound || code == http.StatusMethodNotAllowed || code/100 == 3:
		fail(w, http.StatusBadGateway, notServer)
		return
	case code/100 == 4:
		log.Printf("auth: failed %s first-run sign-in from %s", kind, r.RemoteAddr)
		fail(w, http.StatusUnauthorized, "wrong name or password")
		return
	case err != nil || code/100 != 2 || got.User.ID == "":
		log.Printf("auth: setup %s at %s: status %d: %v", kind, addr, code, err)
		fail(w, http.StatusBadGateway, notServer)
		return
	}
	if got.AccessToken != "" {
		// HolyArrs does not keep the server's token: end that session now.
		lh := map[string]string{"Authorization": fmt.Sprintf("%s, Token=%q", authz, got.AccessToken), "X-Emby-Token": got.AccessToken}
		_, _ = a.call(context.WithoutCancel(ctx), a.tester, http.MethodPost, addr+"/Sessions/Logout", lh, nil, nil)
	}
	if got.User.Policy.IsDisabled {
		fail(w, http.StatusForbidden, "this account is turned off on the "+word+" server")
		return
	}
	if !got.User.Policy.IsAdministrator {
		fail(w, http.StatusForbidden, "Use an admin account on that server.")
		return
	}
	var id int64
	var name string
	var saved []string
	err = a.s.WriteCore(ctx, func(tx *sql.Tx) error {
		var err error
		if id, name, saved, err = a.makeOwnerTx(tx, kind, got.User.Name, newHost); err != nil {
			return err
		}
		switch _, err := linkOwner(ctx, tx, kind, got.User.ID); {
		case err == nil:
			return errTaken
		case !errors.Is(err, sql.ErrNoRows):
			return err
		}
		if err := insertUserLink(ctx, tx, id, kind, got.User.ID, truncate(got.User.Name, 200), database.FormatTime(a.now())); err != nil {
			return err
		}
		// The same keys Settings > Sign-in saves, so it shows this way on.
		g := signInPrefix + kind
		if err := database.PutSetting(tx, settingsGroup, g+"_url", addr, domain.System, a.now()); err != nil {
			return err
		}
		return database.PutSetting(tx, settingsGroup, g+"_on", "true", domain.System, a.now())
	})
	if a.setupFailed(w, err, "this "+word+" account") {
		return
	}
	a.ownerMade(w, r, kind, id, name, saved)
}

// setupDiscover looks for Jellyfin and Emby servers on the local network,
// only while there is no owner. It never fails: no answer is an empty list.
func (a *Auth) setupDiscover(w http.ResponseWriter, r *http.Request) {
	if _, _, exists := a.owner(r.Context()); exists {
		fail(w, http.StatusNotFound, "not found")
		return
	}
	if _, ok := a.setupHost(w, r); !ok {
		return
	}
	out := []foundServer{}
	// Each search counts towards the first-run limit; one runs at a time and
	// a second caller meanwhile gets an empty list.
	key := "find:" + a.clientKey(r)
	if a.ownerSetupLimited(key) {
		writeJSON(w, http.StatusOK, map[string]any{"servers": out})
		return
	}
	a.noteOwnerSetupFail(key)
	if a.discovering.CompareAndSwap(false, true) {
		out = a.discover()
		a.discovering.Store(false)
	}
	writeJSON(w, http.StatusOK, map[string]any{"servers": out})
}

// discover sends each server kind's search message from its own socket, so
// an answer's kind is the message it answered, and collects answers for a
// moment.
func (a *Auth) discover() []foundServer {
	to, wait := a.discoverTo, a.discoverWait
	if to == "" {
		to = discoverTo
	}
	if wait <= 0 {
		wait = discoverWait
	}
	out := []foundServer{}
	dst, err := net.ResolveUDPAddr("udp4", to)
	if err != nil {
		return out
	}
	type answer struct {
		kind string
		from net.IP
		b    []byte
	}
	ch := make(chan answer, 64)
	deadline := time.Now().Add(wait)
	var wg sync.WaitGroup
	for _, k := range []struct{ kind, msg string }{
		{ProviderJellyfin, "Who is JellyfinServer?"},
		{ProviderEmby, "who is EmbyServer?"},
	} {
		c, err := net.ListenUDP("udp4", nil)
		if err != nil {
			continue
		}
		_ = c.SetDeadline(deadline)
		if _, err := c.WriteToUDP([]byte(k.msg), dst); err != nil {
			c.Close()
			continue
		}
		// A broadcast packet can be lost; ask once more a moment later.
		go func(msg string) {
			time.Sleep(wait / 6)
			_, _ = c.WriteToUDP([]byte(msg), dst)
		}(k.msg)
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer c.Close()
			buf := make([]byte, 8<<10)
			for range 32 {
				n, from, err := c.ReadFromUDP(buf)
				if err != nil {
					return
				}
				select {
				case ch <- answer{k.kind, from.IP, append([]byte(nil), buf[:n]...)}:
				default:
				}
			}
		}()
	}
	go func() { wg.Wait(); close(ch) }()
	seen := map[string]bool{}
	for ans := range ch {
		var v struct {
			Address string `json:"Address"`
			Name    string `json:"Name"`
		}
		if json.Unmarshal(ans.b, &v) != nil {
			continue
		}
		// The address is built from the IP the answer came from, with the port
		// it gave, so an answer can only point at the machine that sent it.
		addr, ok := fromAnswer(v.Address, ans.from)
		if !ok || seen[strings.ToLower(addr)] || len(out) >= discoverMax {
			continue
		}
		seen[strings.ToLower(addr)] = true
		name := strings.TrimSpace(v.Name)
		if name == "" {
			name = kindWord[ans.kind]
		}
		out = append(out, foundServer{Kind: ans.kind, Name: truncate(name, 100), Address: addr})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

// fromAnswer keeps the scheme and port of a discovery answer's address and
// puts the sender's IP in place of its host.
func fromAnswer(advertised string, from net.IP) (string, bool) {
	addr, ok := serverAddress(advertised)
	if !ok || from == nil {
		return "", false
	}
	u, err := url.Parse(addr)
	if err != nil {
		return "", false
	}
	port := u.Port()
	if port == "" {
		port = "8096"
		if u.Scheme == "https" {
			port = "8920"
		}
	}
	u.Host = net.JoinHostPort(from.String(), port)
	return strings.TrimRight(u.String(), "/"), true
}
