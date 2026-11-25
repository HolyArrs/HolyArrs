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

// setupWaysNow is the ways the first-run screen offers now.
func setupWaysNow() []string { return setupWays }

var kindWord = map[string]string{ProviderJellyfin: "Jellyfin", ProviderEmby: "Emby"}

// mediaOffLine tells someone their account is turned off on the Jellyfin or
// Emby server, and what to do next.
func mediaOffLine(kind string) string {
	return "Your account is turned off on the " + kindWord[kind] + " server. Ask the person who runs it to turn it back on."
}

// mediaRefusedLine says the Jellyfin or Emby server refused the sign-in
// without saying why. Jellyfin answers the same way for a turned-off account,
// remote access that is off for this address, and a time it is not allowed in.
func mediaRefusedLine(kind string) string {
	return "The " + kindWord[kind] + " server refused the sign-in. Your account may be turned off there, or not allowed to connect from this address. Ask the person who runs it."
}

// mediaOff says whether a 403 sign-in answer says in words that the account
// is turned off. Only Emby does; Jellyfin sends the same plain error text for
// every refusal, so that text alone proves nothing.
func mediaOff(code int, body []byte) bool {
	return code == http.StatusForbidden && strings.Contains(strings.ToLower(string(body)), "account has been disabled")
}

// mediaRefused says whether a 403 sign-in answer is the server's own refusal,
// by the plain texts Jellyfin and Emby send for it. Only those undo the try
// against the name: any other 403 (empty, a proxy or firewall page, anything
// a caller can make up) still counts, so it cannot get round the name limit.
func mediaRefused(code int, body []byte) bool {
	if code != http.StatusForbidden {
		return false
	}
	b := strings.ToLower(strings.TrimSpace(string(body)))
	switch b {
	case "error processing request.", "forbidden.", "forbidden", "user is not allowed access at this time.":
		return true
	}
	return strings.Contains(b, "account has been disabled")
}

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
	base := NewPersonName(extName, provider+"-user", 56)
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
	typedScheme := strings.Contains(s, "://")
	if !typedScheme {
		s = "http://" + s
	}
	u, err := url.Parse(strings.TrimRight(s, "/"))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", false
	}
	// Just an IP or name typed: a home address is plain http on Jellyfin's and
	// Emby's usual 8096; an outside one is https (a password goes there).
	if !typedScheme {
		if homeAddress(u.String()) {
			if u.Port() == "" {
				u.Host = net.JoinHostPort(u.Hostname(), "8096")
			}
		} else {
			u.Scheme = "https"
		}
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
	if word == "" || !slices.Contains(setupWaysNow(), kind) {
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
	addr, user, ok := a.setupServerSignIn(w, r, kind)
	if !ok {
		return
	}
	var id int64
	var name string
	var saved []string
	err := a.s.WriteCore(ctx, func(tx *sql.Tx) error {
		var err error
		if id, name, saved, err = a.makeOwnerTx(tx, kind, user.Name, newHost); err != nil {
			return err
		}
		return a.linkSetupServerTx(ctx, tx, id, kind, addr, user)
	})
	if a.setupFailed(w, err, "this "+word+" account") {
		return
	}
	a.ownerMade(w, r, kind, id, name, saved)
}

// serverUser is the Jellyfin or Emby account a first-run sign-in found.
type serverUser struct{ ID, Name string }

// linkSetupServerTx links the owner to the Jellyfin or Emby account and turns
// that sign-in way on with the address, the keys Settings > Sign-in saves.
// The owner's earlier link of that kind is replaced. errTaken: someone else
// has that account.
func (a *Auth) linkSetupServerTx(ctx context.Context, tx *sql.Tx, ownerID int64, kind, addr string, user serverUser) error {
	switch other, err := linkOwner(ctx, tx, kind, user.ID); {
	case err == nil && other != ownerID:
		return errTaken
	case err != nil && !errors.Is(err, sql.ErrNoRows):
		return err
	}
	if _, err := deleteMediaLink(ctx, tx, ownerID, kind); err != nil {
		return err
	}
	if err := insertUserLink(ctx, tx, ownerID, kind, user.ID, truncate(user.Name, 200), database.FormatTime(a.now())); err != nil {
		return err
	}
	g := signInPrefix + kind
	if err := database.PutSetting(tx, settingsGroup, g+"_url", addr, domain.System, a.now()); err != nil {
		return err
	}
	return database.PutSetting(tx, settingsGroup, g+"_on", "true", domain.System, a.now())
}

// setupServerSignIn checks an admin's name and password on a Jellyfin or
// Emby server typed during first-run setup, with the first-run try limit,
// and keeps the API key that sign-in made for the Media server step. It
// answers the request itself when it says false.
func (a *Auth) setupServerSignIn(w http.ResponseWriter, r *http.Request, kind string) (string, serverUser, bool) {
	ctx := r.Context()
	word := kindWord[kind]
	var body struct {
		URL      string `json:"url"`
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, `send {"url": "...", "name": "...", "password": "..."}`)
		return "", serverUser{}, false
	}
	body.Name = strings.TrimSpace(body.Name)
	addr, ok := serverAddress(body.URL)
	if !ok {
		fail(w, http.StatusBadRequest, "give the "+word+" address, like http://192.168.1.10:8096")
		return "", serverUser{}, false
	}
	// The password goes to this address: plain http only on the home network,
	// like Plex; an outside address needs https.
	if strings.HasPrefix(addr, "http://") && !homeAddress(addr) {
		fail(w, http.StatusBadRequest, "an outside "+word+" address needs https, like https://"+word+".example.com")
		return "", serverUser{}, false
	}
	if body.Name == "" || len(body.Name) > 128 || len(body.Password) > MaxPassword {
		fail(w, http.StatusBadRequest, "give your "+word+" name and password")
		return "", serverUser{}, false
	}
	key := "media:" + a.clientKey(r)
	if a.ownerSetupLimited(key) {
		w.Header().Set("Retry-After", strconv.Itoa(int(setupWindow.Seconds())))
		fail(w, http.StatusTooManyRequests, "too many tries; wait a minute and try again")
		return "", serverUser{}, false
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
	var refusal []byte
	code, err := a.call(ctx, a.tester, http.MethodPost, addr+"/Users/AuthenticateByName", hdr, map[string]string{"Username": body.Name, "Pw": body.Password}, keepRefusal{&got, &refusal})
	notServer := "that address did not answer like " + word + "; check it and try again"
	switch {
	case err != nil && code == 0:
		log.Printf("auth: setup %s at %s: %v", kind, addr, err)
		fail(w, http.StatusBadGateway, "could not reach "+addr+"; check the address and that the server is on")
		return "", serverUser{}, false
	case mediaOff(code, refusal):
		fail(w, http.StatusForbidden, mediaOffLine(kind))
		return "", serverUser{}, false
	case mediaRefused(code, refusal):
		fail(w, http.StatusForbidden, mediaRefusedLine(kind))
		return "", serverUser{}, false
	case code == http.StatusNotFound || code == http.StatusMethodNotAllowed || code == http.StatusForbidden || code/100 == 3:
		fail(w, http.StatusBadGateway, notServer)
		return "", serverUser{}, false
	case code/100 == 4:
		log.Printf("auth: failed %s first-run sign-in from %s", kind, r.RemoteAddr)
		fail(w, http.StatusUnauthorized, "wrong name or password")
		return "", serverUser{}, false
	case err != nil || code/100 != 2 || got.User.ID == "":
		log.Printf("auth: setup %s at %s: status %d: %v", kind, addr, code, err)
		fail(w, http.StatusBadGateway, notServer)
		return "", serverUser{}, false
	}
	if got.AccessToken != "" {
		// An admin's sign-in makes HolyArrs its own API key there, for the Connect step.
		if got.User.Policy.IsAdministrator && !got.User.Policy.IsDisabled {
			a.makeSetupKey(context.WithoutCancel(ctx), kind, addr, authz, got.AccessToken)
		}
		// HolyArrs does not keep the sign-in session itself: end it now.
		lh := map[string]string{"Authorization": fmt.Sprintf("%s, Token=%q", authz, got.AccessToken), "X-Emby-Token": got.AccessToken}
		_, _ = a.call(context.WithoutCancel(ctx), a.tester, http.MethodPost, addr+"/Sessions/Logout", lh, nil, nil)
	}
	if got.User.Policy.IsDisabled {
		fail(w, http.StatusForbidden, mediaOffLine(kind))
		return "", serverUser{}, false
	}
	if !got.User.Policy.IsAdministrator {
		fail(w, http.StatusForbidden, "Use an admin account on that server.")
		return "", serverUser{}, false
	}
	return addr, serverUser{got.User.ID, got.User.Name}, true
}

// SetupOpen reports whether the caller may use a first-run-only route, and
// answers the request itself when not. Before an owner exists anyone on the
// first-run screen may (from a home address); after, only the owner while
// setup is still open.
func (a *Auth) SetupOpen(w http.ResponseWriter, r *http.Request) bool {
	if _, _, exists := a.owner(r.Context()); exists {
		// These routes are public, so the caller is read from the session here.
		who, ok := a.session(w, r)
		if !ok || who.Role != webserver.RoleOwner ||
			database.SettingOr(r.Context(), a.s.Read(), settingsGroup, settingFirstRunDone, "") == "1" {
			fail(w, http.StatusNotFound, "not found")
			return false
		}
		return true
	}
	_, ok := a.setupHost(w, r)
	return ok
}

// setupDiscover looks for Jellyfin and Emby servers on the local network,
// only while there is no owner. It never fails: no answer is an empty list.
func (a *Auth) setupDiscover(w http.ResponseWriter, r *http.Request) {
	if !a.SetupOpen(w, r) {
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
		// The broadcast and a direct look at the addresses next to the browser's
		// own one run together (a container on a bridge network never hears
		// the broadcast answers); the same address found twice counts once.
		ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
		near := make(chan []foundServer, 1)
		// Only the network HolyArrs itself was reached on is searched: the
		// server works it out from the address in the request, never from
		// anything the page sends.
		reached := r.Host
		if h, _, err := net.SplitHostPort(r.Host); err == nil {
			reached = h
		}
		go func() { near <- scanNear(ctx, reached) }()
		out = a.discover()
		seen := map[string]bool{}
		for _, f := range out {
			seen[strings.ToLower(f.Address)] = true
		}
		for _, f := range <-near {
			if !seen[strings.ToLower(f.Address)] && len(out) < discoverMax {
				seen[strings.ToLower(f.Address)] = true
				out = append(out, f)
			}
		}
		for i := range out {
			out[i] = realCase(r.Context(), out[i])
		}
		cancel()
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

// homeAddress is true for an address on the home network: a private,
// loopback or link-local IP, or a plain or home-style name (nas, tower.local).
func homeAddress(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	h := strings.ToLower(u.Hostname())
	if ip := net.ParseIP(h); ip != nil {
		return ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast()
	}
	if !strings.Contains(h, ".") {
		return true
	}
	for _, end := range []string{".local", ".lan", ".home", ".internal", ".home.arpa"} {
		if strings.HasSuffix(h, end) {
			return true
		}
	}
	// A real name that points inside the home (jellyfin.mydomain.com ->
	// 192.168.1.20) counts as home too; every address it gives must be home.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, h)
	if err != nil || len(ips) == 0 {
		return false
	}
	for _, ip := range ips {
		if !(ip.IP.IsPrivate() || ip.IP.IsLoopback() || ip.IP.IsLinkLocalUnicast()) {
			return false
		}
	}
	return true
}
