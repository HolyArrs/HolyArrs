package accounts

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/startup"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// WayView is one outside sign-in way as settings show it.
type WayView struct {
	On       bool   `json:"on"`
	URL      string `json:"url,omitempty"`       // Jellyfin and Emby
	ServerID string `json:"server_id,omitempty"` // Plex: the server people must share
	Join     string `json:"join"`
	Allow    string `json:"allow"` // comma-separated names or emails for JoinList
}

func (a *Auth) way(ctx context.Context, p string) WayView {
	v := WayView{
		On:       a.signInSetting(ctx, p+"_on") == "true" && (p != ProviderEmby || startup.FeatureOn(startup.EmbySignIn)),
		URL:      a.signInSetting(ctx, p+"_url"),
		ServerID: a.signInSetting(ctx, p+"_server_id"),
		Join:     a.signInSetting(ctx, p+"_join"),
		Allow:    a.signInSetting(ctx, p+"_allow"),
	}
	if v.Join == "" {
		v.Join = JoinNobody
	}
	return v
}

// clientID is this HolyArrs's id toward Plex and media servers, made once.
func (a *Auth) clientID(ctx context.Context) string {
	if id := a.signInSetting(ctx, keyClientID); id != "" {
		return id
	}
	id := "holyarrs-" + token(12)
	_ = a.s.WriteCore(ctx, func(tx *sql.Tx) error {
		return database.PutSettingIfMissing(tx, settingsGroup, signInPrefix+keyClientID, id, database.System, a.now())
	})
	return a.signInSetting(ctx, keyClientID)
}

// ways lists the sign-in ways the sign-in page offers.
func (a *Auth) ways(ctx context.Context) []string {
	out := []string{"password"}
	for _, p := range []string{ProviderPlex, ProviderJellyfin, ProviderEmby} {
		if a.way(ctx, p).On {
			out = append(out, p)
		}
	}
	sso := a.ssoNow()
	if sso.oidc != nil {
		out = append(out, "oidc")
	}
	if sso.forward != nil {
		out = append(out, "forward_auth")
	}
	return out
}

func (a *Auth) getSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	nets := []string{}
	if l := a.localNets.Load(); l != nil {
		nets = l.list
	}
	seen, untrusted := a.seenAddress(r)
	writeJSON(w, http.StatusOK, map[string]any{
		settingSkipLocal:   a.skipLocal.Load(),
		keyLocalNets:       nets,
		keyRequireAdmin2FA: a.requireAdmin2FA.Load(),
		keyMinLength:       MinPasswordLength(ctx, a.s.Read()),
		ProviderPlex:       a.way(ctx, ProviderPlex),
		ProviderJellyfin:   a.way(ctx, ProviderJellyfin),
		ProviderEmby:       a.way(ctx, ProviderEmby),
		"plex_owner":       a.plexOwner(ctx),
		keyAllowedHosts:    a.savedHostList(),
		// For the skip card: the address this browser shows up as, listed
		// networks that look like a container gateway, and whether a proxy
		// in front is not trusted (then the skip never applies).
		"seen_address":     seen,
		"gateway_networks": gatewayNets(nets, seen),
		"proxy_untrusted":  untrusted,
	})
}

func (a *Auth) putSettings(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	var body struct {
		SkipLocal    *bool    `json:"skip_login_local_network"`
		LocalNets    []string `json:"local_networks"`
		RequireAdmin *bool    `json:"require_two_factor_admins"`
		MinPassword  *int     `json:"min_password_length"`
		Plex         *WayView `json:"plex"`
		Jellyfin     *WayView `json:"jellyfin"`
		Emby         *WayView `json:"emby"`
		// AllowedHosts are the host names saved in Settings; names from
		// HOLYARRS_ALLOWED_HOSTS are not listed and stay.
		AllowedHosts *[]string `json:"allowed_hosts"`
		// ConfirmEveryone must be true to open a way to everyone the
		// server lets in, when it was not open to everyone already.
		ConfirmEveryone bool `json:"confirm_everyone"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&body); err != nil ||
		(body.SkipLocal == nil && body.LocalNets == nil && body.RequireAdmin == nil && body.MinPassword == nil && body.Plex == nil && body.Jellyfin == nil && body.Emby == nil && body.AllowedHosts == nil) {
		fail(w, http.StatusBadRequest, `send {"skip_login_local_network": true|false, "local_networks": ["192.168.1.0/24"], "require_two_factor_admins": true|false, "min_password_length": 4..64, "plex": {...}, "jellyfin": {...}, "emby": {...}}`)
		return
	}
	if body.SkipLocal != nil && *body.SkipLocal {
		kids, err := countKids(r.Context(), a.s.Read())
		if err != nil {
			serverError(w, "sign-in settings", err)
			return
		}
		if kids > 0 {
			fail(w, http.StatusBadRequest, "Skipping sign-in on the local network cannot be on while a Kid account has an age limit: anyone at home would sign in as the owner. Remove the limits first.")
			return
		}
		// An open invite with an age limit makes a Kid once it is used.
		expiries, err := kidInviteExpiries(r.Context(), a.s.Read())
		if err != nil {
			serverError(w, "sign-in settings", err)
			return
		}
		open := false
		for _, expires := range expiries {
			if a.now().Before(parseTime(expires)) {
				open = true
			}
		}
		if open {
			fail(w, http.StatusBadRequest, "Skipping sign-in on the local network cannot be on while an open invite has an age limit: the Kid it makes would sign in as the owner at home. Withdraw the invite or let it run out first.")
			return
		}
	}
	// Skipping needs at least one listed network; an empty list skips nowhere.
	nets := a.localNets.Load()
	if body.LocalNets != nil {
		var err error
		if nets, err = parseLocalNets(body.LocalNets); err != nil {
			fail(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	skip := a.skipLocal.Load()
	if body.SkipLocal != nil {
		skip = *body.SkipLocal
	}
	if (body.SkipLocal != nil || body.LocalNets != nil) && skip && (nets == nil || len(nets.list) == 0) {
		fail(w, http.StatusBadRequest, "List at least one local network first, like 192.168.1.0/24: sign-in is skipped only there.")
		return
	}
	set := map[string]string{}
	if body.SkipLocal != nil {
		set["this_app/"+settingSkipLocal] = strconv.FormatBool(*body.SkipLocal)
	}
	if body.LocalNets != nil {
		v, _ := json.Marshal(nets.list)
		set[settingsGroup+"/"+signInPrefix+keyLocalNets] = string(v)
	}
	if body.RequireAdmin != nil {
		set[settingsGroup+"/"+signInPrefix+keyRequireAdmin2FA] = strconv.FormatBool(*body.RequireAdmin)
	}
	if body.MinPassword != nil {
		if *body.MinPassword < MinPassword || *body.MinPassword > MaxMinPassword {
			fail(w, http.StatusBadRequest, fmt.Sprintf("the shortest password allowed must be %d to %d characters", MinPassword, MaxMinPassword))
			return
		}
		set[settingsGroup+"/"+signInPrefix+keyMinLength] = strconv.Itoa(*body.MinPassword)
	}
	for p, v := range map[string]*WayView{ProviderPlex: body.Plex, ProviderJellyfin: body.Jellyfin, ProviderEmby: body.Emby} {
		if v == nil {
			continue
		}
		if v.Join == "" {
			v.Join = JoinNobody
		}
		if v.Join != JoinNobody && v.Join != JoinList && v.Join != JoinEveryone {
			fail(w, http.StatusBadRequest, "join must be nobody, list or everyone")
			return
		}
		v.URL = strings.TrimRight(strings.TrimSpace(v.URL), "/")
		if p != ProviderPlex && v.URL != "" {
			u, err := url.Parse(v.URL)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
				fail(w, http.StatusBadRequest, p+" address must look like http://192.168.1.10:8096")
				return
			}
		}
		if v.Join == JoinEveryone && !body.ConfirmEveryone && a.way(r.Context(), p).Join != JoinEveryone {
			fail(w, http.StatusConflict, "letting everyone from "+p+" join needs confirming: send confirm_everyone")
			return
		}
		if p == ProviderPlex && v.Join == JoinEveryone && strings.TrimSpace(v.ServerID) == "" {
			// Without a server to share, everyone would mean every Plex account in the world.
			fail(w, http.StatusBadRequest, "to let everyone from Plex join, give the server id people must share")
			return
		}
		if p != ProviderPlex && v.On && v.URL == "" {
			fail(w, http.StatusBadRequest, "give the "+p+" server address first")
			return
		}
		g := settingsGroup + "/" + signInPrefix + p
		set[g+"_on"] = strconv.FormatBool(v.On)
		set[g+"_join"] = v.Join
		set[g+"_allow"] = truncate(strings.TrimSpace(v.Allow), 4000)
		if p == ProviderPlex {
			set[g+"_server_id"] = truncate(strings.TrimSpace(v.ServerID), 200)
		} else {
			set[g+"_url"] = truncate(v.URL, 500)
		}
	}
	var hosts []string
	if body.AllowedHosts != nil {
		// A browser session with a recent password only (Gate asks for the
		// password again); never an API key.
		if who, ok := webserver.From(r.Context()); ok && who.Via != "session" {
			fail(w, http.StatusForbidden, "change Allowed host names in the browser, signed in with your password")
			return
		}
		var err error
		if hosts, err = cleanHostList(*body.AllowedHosts); err != nil {
			fail(w, http.StatusBadRequest, err.Error())
			return
		}
		// Removing the name this browser uses would lock it out.
		if h := normHost(r.Host); h != "" && net.ParseIP(h) == nil && !a.hosts[h] && !slices.Contains(hosts, h) {
			fail(w, http.StatusConflict, "you opened HolyArrs as "+echoHost(r.Host)+"; keep that name in the list, or change the list from the IP address")
			return
		}
		v, _ := json.Marshal(hosts)
		set[settingsGroup+"/"+signInPrefix+keyAllowedHosts] = string(v)
	}
	by := webserver.Actor(r.Context())
	err := a.s.WriteCore(r.Context(), func(tx *sql.Tx) error {
		for k, v := range set {
			grp, key, _ := strings.Cut(k, "/")
			if err := database.PutSetting(tx, grp, key, v, by, a.now()); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		serverError(w, "sign-in settings", err)
		return
	}
	if body.LocalNets != nil {
		a.localNets.Store(nets)
		log.Printf("auth: local networks set to %s", strings.Join(nets.list, ", "))
	}
	if body.SkipLocal != nil {
		a.skipLocal.Store(*body.SkipLocal)
		log.Printf("auth: skip sign-in on the local network set to %t", *body.SkipLocal)
	}
	if body.RequireAdmin != nil {
		a.requireAdmin2FA.Store(*body.RequireAdmin)
	}
	if body.AllowedHosts != nil {
		a.savedHosts.Store(&hosts)
		log.Printf("auth: allowed host names set to %s", strings.Join(hosts, ", "))
	}
	log.Printf("auth: sign-in settings changed by user %d", by.ID)
	a.getSettings(w, r)
}

// allowed says whether a not-yet-linked outside account may join.
func allowed(v WayView, names ...string) bool {
	switch v.Join {
	case JoinEveryone:
		return true
	case JoinList:
		for _, e := range strings.FieldsFunc(v.Allow, func(r rune) bool { return r == ',' || r == '\n' || r == ' ' }) {
			for _, n := range names {
				if n != "" && strings.EqualFold(e, n) {
					return true
				}
			}
		}
	}
	return false
}
