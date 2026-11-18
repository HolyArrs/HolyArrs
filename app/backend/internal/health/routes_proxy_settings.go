package health

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// The app proxy: one setting every outside call goes through. The password
// lives in the vault; the rest is one JSON value in the this_app group.

const (
	proxyGroup       = "this_app"
	proxySettingKey  = "proxy"
	proxyPasswordKey = "proxy_password"
)

// ProxyHandlers serve GET and PUT /settings/proxy.
type ProxyHandlers struct {
	Store   *database.Store
	Secrets *secrets.Vault
}

// ProxyAnswer is the proxy setting as the API shows it. Password is the
// mask when one is set, never the value.
type ProxyAnswer struct {
	Type        string `json:"type"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	Bypass      string `json:"bypass"`
	BypassLocal bool   `json:"bypass_local"`
}

type storedProxy struct {
	Type        string `json:"type"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	Bypass      string `json:"bypass"`
	BypassLocal *bool  `json:"bypass_local"`
}

// Routes mounts the proxy setting routes.
func (h *ProxyHandlers) Routes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/settings/proxy", h.get)
	m.HandleFunc("PUT /api/v1/settings/proxy", h.put)
}

// LoadProxy reads the saved proxy and turns it on for every outside call.
// A saved value that no longer checks out leaves the proxy off.
func LoadProxy(ctx context.Context, st *database.Store, v *secrets.Vault) error {
	sp := readProxy(ctx, st)
	pass := ""
	if v != nil && sp.Type != "" {
		pass, _ = v.Setting(ctx, proxyGroup, proxyPasswordKey)
	}
	p, err := outbound.ParseProxy(sp.settings(pass))
	if err != nil {
		outbound.SetProxy(nil)
		return err
	}
	outbound.SetProxy(p)
	return nil
}

func (h *ProxyHandlers) get(w http.ResponseWriter, r *http.Request) {
	webserver.WriteJSON(w, http.StatusOK, h.answer(r.Context()))
}

func (h *ProxyHandlers) put(w http.ResponseWriter, r *http.Request) {
	// Every outside call goes through the proxy, so only the owner sets it.
	if who, signed := webserver.From(r.Context()); !signed || who.Role != webserver.RoleOwner {
		webserver.Fail(w, http.StatusForbidden, "only the owner may change the proxy")
		return
	}
	var in struct {
		Type        string  `json:"type"`
		Host        string  `json:"host"`
		Port        int     `json:"port"`
		Username    string  `json:"username"`
		Password    *string `json:"password"`
		Bypass      string  `json:"bypass"`
		BypassLocal *bool   `json:"bypass_local"`
	}
	if !webserver.Decode(w, r, &in, `send {"type": "http", "host": "proxy.lan", "port": 3128, "username": "", "password": "", "bypass": "", "bypass_local": true}, or {"type": ""} for none`) {
		return
	}
	ctx := r.Context()
	local := true
	if in.BypassLocal != nil {
		local = *in.BypassLocal
	}
	sp := storedProxy{Type: in.Type, Host: in.Host, Port: in.Port, Username: in.Username, Bypass: in.Bypass, BypassLocal: &local}
	pass, keep := "", in.Password == nil || *in.Password == secrets.Masked
	if keep {
		pass, _ = h.Secrets.Setting(ctx, proxyGroup, proxyPasswordKey)
	} else {
		pass = *in.Password
	}
	if sp.Type == "" {
		pass, keep = "", false
	}
	p, err := outbound.ParseProxy(sp.settings(pass))
	if err != nil {
		webserver.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if p != nil {
		sp.Type, sp.Host = p.Type, p.Host
	}
	by := webserver.Actor(ctx)
	if !keep {
		if err := h.Secrets.SetSetting(ctx, proxyGroup, proxyPasswordKey, pass, by); err != nil {
			webserver.Internal(w, "api", err)
			return
		}
	}
	raw, _ := json.Marshal(sp)
	err = h.Store.Write(ctx, func(tx *sql.Tx) error {
		return database.PutSetting(tx, proxyGroup, proxySettingKey, string(raw), by, time.Now())
	})
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	outbound.SetProxy(p)
	webserver.WriteJSON(w, http.StatusOK, h.answer(ctx))
}

func (h *ProxyHandlers) answer(ctx context.Context) ProxyAnswer {
	sp := readProxy(ctx, h.Store)
	a := ProxyAnswer{Type: sp.Type, Host: sp.Host, Port: sp.Port, Username: sp.Username, Bypass: sp.Bypass, BypassLocal: sp.BypassLocal == nil || *sp.BypassLocal}
	if pw, _ := h.Secrets.Setting(ctx, proxyGroup, proxyPasswordKey); pw != "" {
		a.Password = secrets.Masked
	}
	return a
}

func readProxy(ctx context.Context, st *database.Store) storedProxy {
	var sp storedProxy
	raw, found, err := database.SettingFound(ctx, st.Read(), proxyGroup, proxySettingKey)
	if err == nil && found {
		_ = json.Unmarshal([]byte(raw), &sp)
	}
	return sp
}

func (sp storedProxy) settings(pass string) outbound.ProxySettings {
	return outbound.ProxySettings{Type: sp.Type, Host: sp.Host, Port: strconv.Itoa(sp.Port), Username: sp.Username,
		Password: pass, Bypass: sp.Bypass, BypassLocal: sp.BypassLocal == nil || *sp.BypassLocal}
}
