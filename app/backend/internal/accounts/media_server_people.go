package accounts

import (
	"context"
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// Setting up outside sign-in from Settings (owner only): link the owner's
// Plex account and pick the server, test a Jellyfin or Emby address, and
// import the people of the linked server after a preview.

// keyPlexOwner names the vault setting that keeps the owner's Plex
// token, used to list the people the server is shared with.
const keyPlexOwner = "plex_owner"

const (
	importLife = 15 * time.Minute
	testWait   = 5 * time.Second
	testMax    = 64 << 10
)

var errTaken = errors.New("linked to someone else")

var errImportKidSkip = errors.New("kid import while skipping local sign-in")

// importPerson is one person an import preview offers.
type importPerson struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Email    string `json:"email,omitempty"`
	Kid      bool   `json:"kid"`                 // a hint: Plex managed user or restricted
	Admin    bool   `json:"server_admin"`        // a hint only; never grants HolyArrs admin
	LinkedTo string `json:"linked_to,omitempty"` // the HolyArrs user already linked
}

// importOffer is the last preview per provider; an import may only take
// people from it, so a request cannot invent links.
type importOffer struct {
	people  map[string]importPerson
	expires time.Time
}

func (a *Auth) mediaServerPeopleRoutes(m *http.ServeMux) {
	m.HandleFunc("POST /api/v1/auth/plex/link/start", a.plexLinkStart)
	m.HandleFunc("POST /api/v1/auth/plex/link/finish", a.plexLinkFinish)
	m.HandleFunc("DELETE /api/v1/auth/plex/link", a.plexUnlink)
	m.HandleFunc("POST /api/v1/auth/media-server/{kind}/test", a.mediaServerTest)
	m.HandleFunc("POST /api/v1/auth/import/{kind}/preview", a.importPreview)
	m.HandleFunc("POST /api/v1/auth/import/{kind}", a.importPeople)
}

// plexOwner says whether the owner's Plex account is linked and as whom.
func (a *Auth) plexOwner(ctx context.Context) map[string]any {
	name, err := plexOwnerName(ctx, a.s.Read())
	tok := false
	if a.vault != nil {
		v, _ := a.vault.Setting(ctx, settingsGroup, signInPrefix+keyPlexOwner)
		tok = v != ""
	}
	return map[string]any{"linked": err == nil, "name": name, "can_import": tok}
}

func (a *Auth) plexLinkStart(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	a.startPin(w, r, pinOwner)
}

func (a *Auth) plexLinkFinish(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	if a.vault == nil {
		fail(w, http.StatusServiceUnavailable, "the secret store is not open")
		return
	}
	ctx := r.Context()
	tok, ok := a.checkPin(w, r, pinOwner)
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
		log.Printf("auth: plex link user: status %d: %v", code, err)
		fail(w, http.StatusBadGateway, "Plex did not say who you are; try again")
		return
	}
	var res []struct {
		Name             string `json:"name"`
		ClientIdentifier string `json:"clientIdentifier"`
		Provides         string `json:"provides"`
		Owned            bool   `json:"owned"`
	}
	code, err = a.fetch(ctx, http.MethodGet, a.plexURL+"/api/v2/resources?includeHttps=1", hdr, nil, &res)
	if err != nil || code/100 != 2 {
		log.Printf("auth: plex link resources: status %d: %v", code, err)
		fail(w, http.StatusBadGateway, "Plex did not list your servers; try again")
		return
	}
	servers := []map[string]string{}
	for _, x := range res {
		if x.Owned && strings.Contains(x.Provides, "server") && x.ClientIdentifier != "" {
			servers = append(servers, map[string]string{"id": x.ClientIdentifier, "name": x.Name})
		}
	}
	me := webserver.Actor(ctx)
	extID := strconv.FormatInt(user.ID, 10)
	var moved bool
	err = a.s.WriteCore(ctx, func(tx *sql.Tx) error {
		var err error
		moved, err = a.linkPlexTx(tx, me.ID, extID, user.Username, tok, me)
		return err
	})
	if errors.Is(err, errTaken) {
		fail(w, http.StatusConflict, "this Plex account is already linked to another HolyArrs user")
		return
	}
	if err != nil {
		serverError(w, "plex link", err)
		return
	}
	if moved {
		a.dropTickets(func(t *ticket) bool { return t.userID == me.ID && t.method == ProviderPlex })
	}
	if err := a.vault.SetSetting(ctx, settingsGroup, signInPrefix+keyPlexOwner, tok, me); err != nil {
		serverError(w, "plex link", err)
		return
	}
	log.Printf("auth: owner %d linked a Plex account", me.ID)
	a.accountEvent(r, me.ID, "sign_in_linked", byName(r)+" linked the Plex account "+outsideName(user.Username), map[string]any{"provider": "plex"})
	writeJSON(w, http.StatusOK, map[string]any{"name": user.Username, "servers": servers})
}

func (a *Auth) plexUnlink(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	ctx := r.Context()
	me := webserver.Actor(ctx)
	if err := a.s.WriteCore(ctx, func(tx *sql.Tx) error {
		ids, err := plexTokenIDsOfUser(ctx, tx, me.ID)
		if err != nil {
			return err
		}
		if err := a.vault.DeleteTx(ctx, tx, ids...); err != nil {
			return err
		}
		_, err = deletePlexLink(ctx, tx, me.ID)
		return err
	}); err != nil {
		serverError(w, "plex unlink", err)
		return
	}
	if a.vault != nil {
		if err := a.vault.SetSetting(ctx, settingsGroup, signInPrefix+keyPlexOwner, "", me); err != nil {
			serverError(w, "plex unlink", err)
			return
		}
	}
	a.accountEvent(r, me.ID, "sign_in_unlinked", byName(r)+" unlinked their Plex account", map[string]any{"provider": "plex"})
	writeJSON(w, http.StatusOK, a.plexOwner(ctx))
}

// testClient calls an address the owner typed. It refuses link-local,
// multicast and unspecified addresses at dial time (so a name that later
// points elsewhere is checked too), follows no redirects and gives up
// after testWait. Home addresses stay allowed: the server is usually there.
func testClient() *http.Client {
	d := &net.Dialer{Timeout: testWait, Control: func(_, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return err
		}
		if ip := net.ParseIP(host); ip == nil || blockedTestIP(ip) {
			return fmt.Errorf("address %s is not allowed", host)
		}
		return nil
	}}
	return &http.Client{
		Timeout:       testWait,
		Transport:     &http.Transport{DialContext: d.DialContext, Proxy: nil, ResponseHeaderTimeout: testWait},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

func blockedTestIP(ip net.IP) bool {
	return ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified()
}

// checkServerURL is the shape a Jellyfin or Emby address must have.
func checkServerURL(raw string) (string, error) {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("the address must look like http://192.168.1.10:8096")
	}
	return raw, nil
}

func (a *Auth) mediaServerTest(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	kind := r.PathValue("kind")
	if kind != ProviderJellyfin && kind != ProviderEmby {
		fail(w, http.StatusNotFound, "no such server kind")
		return
	}
	var body struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<10)).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, `send {"url": "http://..."}`)
		return
	}
	base, err := checkServerURL(body.URL)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), testWait)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/System/Info/Public", nil)
	if err != nil {
		fail(w, http.StatusBadRequest, "the address must look like http://192.168.1.10:8096")
		return
	}
	req.Header.Set("Accept", "application/json")
	resp, err := a.tester.Do(req)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "message": "The server did not answer at this address."})
		return
	}
	defer resp.Body.Close()
	var info struct {
		ServerName  string `json:"ServerName"`
		Version     string `json:"Version"`
		ProductName string `json:"ProductName"`
		ID          string `json:"Id"`
	}
	data, _ := io.ReadAll(io.LimitReader(resp.Body, testMax))
	if resp.StatusCode/100 != 2 || json.Unmarshal(data, &info) != nil || info.ID == "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "message": "Something answered, but it does not look like a " + kind + " server."})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "name": truncate(info.ServerName, 120), "version": truncate(info.Version, 40), "product": truncate(info.ProductName, 60)})
}

func (a *Auth) importPreview(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	ctx := r.Context()
	kind := r.PathValue("kind")
	var people []importPerson
	switch kind {
	case ProviderPlex:
		var ok bool
		if people, ok = a.plexPeople(w, r); !ok {
			return
		}
	case ProviderJellyfin, ProviderEmby:
		var ok bool
		if people, ok = a.serverPeople(w, r, kind); !ok {
			return
		}
	default:
		fail(w, http.StatusNotFound, "no such sign-in way")
		return
	}
	offer := &importOffer{people: map[string]importPerson{}, expires: a.now().Add(importLife)}
	for i, p := range people {
		if name, err := linkedUserName(ctx, a.s.Read(), kind, p.ID); err == nil {
			people[i].LinkedTo = name
		}
		offer.people[p.ID] = people[i]
	}
	a.mu.Lock()
	if a.offers == nil {
		a.offers = map[string]*importOffer{}
	}
	a.offers[kind] = offer
	a.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"people": people})
}

// plexPeople lists the friends and home users of the owner's Plex account,
// keeping only those who share the chosen server when one is set.
func (a *Auth) plexPeople(w http.ResponseWriter, r *http.Request) ([]importPerson, bool) {
	ctx := r.Context()
	tok := ""
	if a.vault != nil {
		tok, _ = a.vault.Setting(ctx, settingsGroup, signInPrefix+keyPlexOwner)
	}
	if tok == "" {
		fail(w, http.StatusConflict, "link your Plex account first")
		return nil, false
	}
	server := a.way(ctx, ProviderPlex).ServerID
	hdr := a.plexHeaders(ctx)
	hdr["X-Plex-Token"] = tok
	// Plex answers 410 at /api/v2/friends now; /api/users lists the same
	// people (friends and Plex Home) in XML, with the servers shared with each.
	hdr["Accept"] = "application/xml"
	var raw []byte
	code, err := a.fetch(ctx, http.MethodGet, a.plexURL+"/api/users", hdr, nil, &raw)
	var list struct {
		Users []struct {
			ID         int64  `xml:"id,attr"`
			Username   string `xml:"username,attr"`
			Title      string `xml:"title,attr"`
			Email      string `xml:"email,attr"`
			Restricted string `xml:"restricted,attr"`
			Servers    []struct {
				MachineIdentifier string `xml:"machineIdentifier,attr"`
			} `xml:"Server"`
		} `xml:"User"`
	}
	if err == nil && code/100 == 2 {
		err = xml.Unmarshal(raw, &list)
	}
	log.Printf("auth: plex people: /api/users answered %d (%d people): %v", code, len(list.Users), err)
	switch {
	case err == nil && code/100 == 2:
	case code == http.StatusUnauthorized || code == http.StatusForbidden:
		fail(w, http.StatusBadGateway, "Plex refused your linked Plex account; link it again")
		return nil, false
	case code == http.StatusNotFound || code == http.StatusGone:
		fail(w, http.StatusBadGateway, "Plex no longer offers the list of people your server is shared with")
		return nil, false
	default:
		fail(w, http.StatusBadGateway, "Plex did not list the people your server is shared with; try again")
		return nil, false
	}
	out := []importPerson{}
	for _, f := range list.Users {
		if f.ID <= 0 {
			continue
		}
		if server != "" && len(f.Servers) > 0 && !slices.ContainsFunc(f.Servers, func(s struct {
			MachineIdentifier string `xml:"machineIdentifier,attr"`
		}) bool {
			return s.MachineIdentifier == server
		}) {
			continue
		}
		name := f.Username
		if name == "" {
			name = f.Title
		}
		kid := f.Restricted == "1" || strings.EqualFold(f.Restricted, "true")
		out = append(out, importPerson{ID: strconv.FormatInt(f.ID, 10), Name: truncate(name, 56), Email: truncate(f.Email, 200), Kid: kid})
	}
	return out, true
}

// serverPeople signs in to Jellyfin or Emby once as an admin, lists the
// users and ends that session. The password is neither kept nor logged.
func (a *Auth) serverPeople(w http.ResponseWriter, r *http.Request, kind string) ([]importPerson, bool) {
	ctx := r.Context()
	v := a.way(ctx, kind)
	if _, err := checkServerURL(v.URL); err != nil {
		fail(w, http.StatusConflict, "give the "+kind+" server address first")
		return nil, false
	}
	c, ok := readCreds(w, r)
	if !ok {
		return nil, false
	}
	if c.Name == "" || len(c.Name) > 128 || len(c.Password) > MaxPassword {
		fail(w, http.StatusBadRequest, "give the name and password of an admin on the "+kind+" server")
		return nil, false
	}
	authz := fmt.Sprintf(`MediaBrowser Client="%s", Device="%s", DeviceId="%s", Version="1"`, product, product, a.clientID(ctx))
	hdr := map[string]string{"Authorization": authz, "X-Emby-Authorization": authz}
	var got struct {
		User struct {
			Policy struct {
				IsAdministrator bool `json:"IsAdministrator"`
			} `json:"Policy"`
		} `json:"User"`
		AccessToken string `json:"AccessToken"`
	}
	code, err := a.serverFetch(ctx, http.MethodPost, v.URL+"/Users/AuthenticateByName", hdr, map[string]string{"Username": c.Name, "Pw": c.Password}, &got)
	if err != nil {
		fail(w, http.StatusBadGateway, "the "+kind+" server did not answer")
		return nil, false
	}
	if code/100 == 4 {
		fail(w, http.StatusUnauthorized, "wrong name or password")
		return nil, false
	}
	if code/100 != 2 || got.AccessToken == "" {
		fail(w, http.StatusBadGateway, "the "+kind+" server gave an odd answer")
		return nil, false
	}
	th := map[string]string{"Authorization": fmt.Sprintf("%s, Token=%q", authz, got.AccessToken), "X-Emby-Token": got.AccessToken}
	defer func() {
		_, _ = a.serverFetch(context.WithoutCancel(ctx), http.MethodPost, v.URL+"/Sessions/Logout", th, nil, nil)
	}()
	if !got.User.Policy.IsAdministrator {
		fail(w, http.StatusForbidden, "this account is not an admin on the "+kind+" server")
		return nil, false
	}
	var users []struct {
		ID     string `json:"Id"`
		Name   string `json:"Name"`
		Policy struct {
			IsAdministrator bool `json:"IsAdministrator"`
			IsDisabled      bool `json:"IsDisabled"`
			// Set when the server limits what this person may watch.
			MaxParentalRating *int `json:"MaxParentalRating"`
		} `json:"Policy"`
	}
	code, err = a.serverFetch(ctx, http.MethodGet, v.URL+"/Users", th, nil, &users)
	if err != nil || code/100 != 2 {
		fail(w, http.StatusBadGateway, "the "+kind+" server did not list its users")
		return nil, false
	}
	out := []importPerson{}
	for _, u := range users {
		if u.ID == "" || u.Policy.IsDisabled {
			continue
		}
		out = append(out, importPerson{ID: truncate(u.ID, 100), Name: truncate(u.Name, 56), Admin: u.Policy.IsAdministrator, Kid: u.Policy.MaxParentalRating != nil})
	}
	return out, true
}

func (a *Auth) importPeople(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	ctx := r.Context()
	kind := r.PathValue("kind")
	var body struct {
		People []struct {
			ID  string `json:"id"`
			Kid *bool  `json:"kid"` // left out: follow the server's restricted flag
			// ClearKid confirms importing a restricted server account as an adult.
			ClearKid bool `json:"confirm_not_kid"`
		} `json:"people"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&body); err != nil || len(body.People) == 0 || len(body.People) > 500 {
		fail(w, http.StatusBadRequest, `send {"people": [{"id": "...", "kid": false}]}`)
		return
	}
	a.mu.Lock()
	offer := a.offers[kind]
	if offer != nil && a.now().After(offer.expires) {
		delete(a.offers, kind)
		offer = nil
	}
	a.mu.Unlock()
	if offer == nil {
		fail(w, http.StatusConflict, "the list has run out; show it again")
		return
	}
	for _, pick := range body.People {
		if p, ok := offer.people[pick.ID]; ok && p.Kid && pick.Kid != nil && !*pick.Kid && !pick.ClearKid {
			fail(w, http.StatusConflict, p.Name+" is restricted on the server; confirm before importing them as an adult")
			return
		}
	}
	kidPerms := permsJSON([]webserver.Perm{webserver.PermAsk})
	made, skipped := []string{}, 0
	blockKids := a.skipBlocksKids(ctx)
	err := a.s.WriteCore(ctx, func(tx *sql.Tx) error {
		for _, pick := range body.People {
			p, ok := offer.people[pick.ID]
			if !ok {
				skipped++
				continue
			}
			n, err := countLinksOf(ctx, tx, kind, p.ID)
			if err != nil {
				return err
			}
			if n > 0 {
				skipped++
				continue
			}
			base := strings.TrimSpace(p.Name)
			if base == "" {
				base = kind + "-user"
			}
			name := base
			for i := 2; ; i++ {
				var err error
				if n, err = countUsersNamed(ctx, tx, name); err != nil {
					return err
				}
				if n == 0 {
					break
				}
				name = fmt.Sprintf("%s-%d", base, i)
			}
			u := &domain.User{Name: name, Role: webserver.RoleMember, State: "active", Perms: permsJSON(webserver.DefaultPerms(webserver.RoleMember))}
			kid := p.Kid
			if pick.Kid != nil {
				kid = *pick.Kid // clearing a restricted flag was confirmed above
			}
			if kid {
				if blockKids {
					return errImportKidSkip
				}
				u.Perms, u.MaxRating = kidPerms, "PG"
			}
			id, err := insertUserTx(tx, u, webserver.Actor(ctx))
			if err != nil {
				return err
			}
			if err := insertUserLink(ctx, tx, id, kind, p.ID, truncate(p.Name, 200), database.FormatTime(a.now())); err != nil {
				return err
			}
			made = append(made, name)
		}
		return nil
	})
	if errors.Is(err, errImportKidSkip) {
		fail(w, http.StatusConflict, errSkipLocalKid)
		return
	}
	if err != nil {
		serverError(w, "import", err)
		return
	}
	log.Printf("auth: imported %d %s people (skipped %d)", len(made), kind, skipped)
	if len(made) > 0 {
		a.accountEvent(r, webserver.Actor(r.Context()).ID, "people_imported", byName(r)+" imported "+itoa(int64(len(made)))+" "+kind+" accounts as members",
			map[string]any{"count": len(made), "provider": kind})
	}
	writeJSON(w, http.StatusOK, map[string]any{"made": made, "skipped": skipped})
}
