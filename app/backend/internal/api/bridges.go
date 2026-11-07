package api

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/auth"
	"github.com/HolyArrs/holyarrs/app/backend/internal/bridge"
	"github.com/HolyArrs/holyarrs/app/backend/internal/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/store"
)

// Connect Jackett or Prowlarr (#96): a guided setup in Settings > Sources
// that turns the sites set up there into Sources in one step.

// BridgeView is one connected Jackett or Prowlarr.
type BridgeView struct {
	ID          int64      `json:"id"`
	Kind        string     `json:"kind"`
	URL         string     `json:"url"`
	CallbackURL string     `json:"callback_url,omitempty"`
	LastCheck   *time.Time `json:"last_check,omitempty"`
	LastError   string     `json:"last_error,omitempty"`
	Sites       int        `json:"sites"`
	Sources     int        `json:"sources"`
	// Local: a Jackett on the local network, whose Sources may reach it there.
	Local bool `json:"local,omitempty"`
}

// BridgeInput is a Jackett or Prowlarr to test or connect.
type BridgeInput struct {
	Kind        string `json:"kind"`
	URL         string `json:"url"`
	Key         string `json:"key"`
	CallbackURL string `json:"callback_url"`
	// NewKeys lets the setup replace an old-app key that exists already
	// (an app using the old one must get the new one).
	NewKeys bool `json:"new_keys"`
	// Keys are old-app keys typed in, by side (sonarr, radarr), used instead of making new ones.
	Keys map[string]string `json:"keys"`
}

const bridgeHelp = `send {"kind": "jackett" or "prowlarr", "url": "http://jackett:9117", "key": "..."}`

func (a *API) bridges() *bridge.Service {
	a.bridgeOnce.Do(func() {
		if a.s.Bridges != nil {
			a.bridgeSvc = a.s.Bridges
			return
		}
		a.bridgeSvc = &bridge.Service{Store: a.s.Store, Vault: a.s.Secrets}
	})
	return a.bridgeSvc
}

func (a *API) bridgeView(ctx context.Context, b bridge.Bridge) BridgeView {
	svc := a.bridges()
	return BridgeView{ID: b.ID, Kind: b.Kind, URL: b.URL, CallbackURL: b.CallbackURL, LastCheck: svc.LastChecked(b), LastError: b.LastError,
		Sites: b.SiteCount, Sources: svc.SourceCount(ctx, b), Local: b.Kind == bridge.KindJackett && b.Extra["local"] == "1"}
}

func bridgeFail(w http.ResponseWriter, status int, err error, field string) {
	body := map[string]string{"error": bridge.Words(err)}
	if field != "" {
		body["field"] = field
	}
	writeJSON(w, status, body)
}

// cleanBridge checks the kind and the addresses of an input.
func cleanBridge(w http.ResponseWriter, in *BridgeInput) bool {
	if in.Kind != bridge.KindJackett && in.Kind != bridge.KindProwlarr {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Pick Jackett or Prowlarr.", "field": "kind"})
		return false
	}
	u, err := bridge.CleanURL(in.URL)
	if err != nil {
		bridgeFail(w, http.StatusBadRequest, err, "url")
		return false
	}
	in.URL = u
	if in.Key == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Paste the key.", "field": "key"})
		return false
	}
	if in.Kind == bridge.KindProwlarr {
		cb, err := bridge.CleanURL(in.CallbackURL)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Give the address Prowlarr can reach HolyArrs at, such as http://192.168.1.10:6262.", "field": "callback_url"})
			return false
		}
		in.CallbackURL = cb
	}
	return true
}

func (a *API) listBridges(w http.ResponseWriter, r *http.Request) {
	bs, err := a.bridges().List(r.Context())
	if err != nil {
		internal(w, err)
		return
	}
	out := []BridgeView{}
	for _, b := range bs {
		out = append(out, a.bridgeView(r.Context(), b))
	}
	// home_ip: this machine's home-network address, for the page to offer as
	// HolyArrs's address for Prowlarr when the browser came in over Tailscale
	// or localhost (Prowlarr usually can't reach those).
	writeJSON(w, http.StatusOK, map[string]any{"bridges": out, "home_ip": homeIP()})
}

// homeIP is the first 10.x or 192.168.x address of this machine, or "".
// 172.16/12 is skipped: inside Docker it is the container's own network.
func homeIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, a := range addrs {
		ipn, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		ip := ipn.IP.To4()
		if ip == nil {
			continue
		}
		if ip[0] == 10 || (ip[0] == 192 && ip[1] == 168) {
			return ip.String()
		}
	}
	return ""
}

func (a *API) findBridges(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()
	found := bridge.Find(ctx)
	if found == nil {
		found = []bridge.Found{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"found": found})
}

// prowlarrSides are the HolyArrs sides Prowlarr connects to: TV when Shows
// is on, movies when Movies is on.
func (a *API) prowlarrSides(callback string) []bridge.Side {
	var out []bridge.Side
	if a.partIsOn(parts.Shows) {
		out = append(out, bridge.Side{Kind: "Sonarr", Name: "HolyArrs TV shows", Base: callback + "/sonarr"})
	}
	if a.partIsOn(parts.Movies) {
		out = append(out, bridge.Side{Kind: "Radarr", Name: "HolyArrs Movies", Base: callback + "/radarr"})
	}
	return out
}

func sideOf(kind string) string {
	if kind == "Sonarr" {
		return "sonarr"
	}
	return "radarr"
}

// testBridge checks an address and key without saving anything.
func (a *API) testBridge(w http.ResponseWriter, r *http.Request) {
	var in BridgeInput
	if !decode(w, r, &in, bridgeHelp) || !cleanBridge(w, &in) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if in.Kind == bridge.KindJackett {
		sites, err := bridge.NewJackett(in.URL, in.Key).Sites(ctx)
		if err != nil {
			bridgeFail(w, http.StatusBadGateway, err, "")
			return
		}
		titles := []string{}
		for _, s := range sites {
			titles = append(titles, s.Title)
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "text": "Jackett answered with " + sitesWords(len(sites)) + ".", "sites": titles})
		return
	}
	version, err := bridge.NewProwlarr(in.URL, in.Key).Status(ctx)
	if err != nil {
		bridgeFail(w, http.StatusBadGateway, err, "")
		return
	}
	have, err := a.compatSides(ctx)
	if err != nil {
		internal(w, err)
		return
	}
	need := map[string]string{}
	for _, s := range a.prowlarrSides(in.CallbackURL) {
		need[sideOf(s.Kind)] = map[bool]string{true: "have", false: "new"}[have[sideOf(s.Kind)]]
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "text": "Prowlarr " + version + " answered.", "keys": need})
}

func sitesWords(n int) string {
	if n == 1 {
		return "1 site"
	}
	return strconv.Itoa(n) + " sites"
}

// compatSides says which old-app sides already have a key.
func (a *API) compatSides(ctx context.Context) (map[string]bool, error) {
	out := map[string]bool{}
	if a.s.Auth == nil {
		return out, nil
	}
	ks, err := a.s.Auth.CompatKeys(ctx)
	if err != nil {
		return nil, err
	}
	for _, k := range ks {
		out[k.Side] = true
	}
	return out, nil
}

// addBridge connects a Jackett or Prowlarr and brings its sites in. One
// already connected at that address takes the new key (and, for Prowlarr,
// the new address HolyArrs is reached at) instead.
func (a *API) addBridge(w http.ResponseWriter, r *http.Request) {
	var in BridgeInput
	if !decode(w, r, &in, bridgeHelp) || !cleanBridge(w, &in) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	svc := a.bridges()
	existing, found := svc.Find(ctx, in.Kind, in.URL)
	if in.Kind == bridge.KindJackett {
		if _, err := bridge.NewJackett(in.URL, in.Key).Sites(ctx); err != nil {
			bridgeFail(w, http.StatusBadGateway, err, "")
			return
		}
		var b bridge.Bridge
		var err error
		if found {
			b, err = svc.Update(ctx, existing, in.Key, "", actor(r))
		} else {
			b, err = svc.Add(ctx, in.Kind, in.URL, in.Key, "", actor(r))
		}
		if err != nil {
			a.bridgeAddError(w, err)
			return
		}
		// A failed first check stays on the connection as its last error.
		_, _ = svc.CheckJackett(ctx, b, actor(r))
		b, _ = svc.Get(ctx, b.ID)
		writeJSON(w, http.StatusCreated, a.bridgeView(ctx, b))
		return
	}
	p := bridge.NewProwlarr(in.URL, in.Key)
	if _, err := p.Status(ctx); err != nil {
		bridgeFail(w, http.StatusBadGateway, err, "")
		return
	}
	sides := a.prowlarrSides(in.CallbackURL)
	if len(sides) == 0 {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "Movies and TV shows are both off, so Prowlarr has nothing to send. Turn one on first."})
		return
	}
	have, err := a.compatSides(ctx)
	if err != nil {
		internal(w, err)
		return
	}
	// Keys are stored hashed and a new one replaces the old: make one only
	// for a side with none, unless the person said a new one is fine, or
	// they typed the one they have.
	var replace []string
	for i, s := range sides {
		side := sideOf(s.Kind)
		if k := in.Keys[side]; k != "" {
			sides[i].Key = k
			continue
		}
		if have[side] && !in.NewKeys {
			replace = append(replace, side)
		}
	}
	if len(replace) > 0 {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "HolyArrs already has an old-app key for this. Make a new one (an app using the old one, such as Seerr, must get the new one too), or type the key you have.", "need": "keys", "sides": replace})
		return
	}
	// What this request made, so a failure takes it back: keys for sides
	// that had none, and the old-app API switch.
	made := map[string]string{}
	var fresh []string
	turnedOn := false
	undo := func() {
		uctx, ucancel := context.WithTimeout(context.WithoutCancel(r.Context()), 30*time.Second)
		defer ucancel()
		for _, side := range fresh {
			_, _ = a.s.Auth.DeleteCompatKey(r.WithContext(uctx), side)
		}
		if turnedOn {
			off := false
			_ = a.saveOldAppSwitches(uctx, actor(r), &off, nil)
		}
	}
	for i, s := range sides {
		if s.Key != "" || a.s.Auth == nil {
			continue
		}
		// The same gates as making an old-app key by hand: a recent password
		// sign-in, an API key with the admin scope, and a person who may
		// manage the Library (the key acts as them).
		if !auth.RequireStrong(w, r) {
			undo()
			return
		}
		// Making a key is a risky action: a browser session confirms its
		// password first, as on the old-app key routes (only here, so adding
		// a bridge without a new key never asks).
		if who, ok := auth.From(r.Context()); ok && who.Via == "session" && !auth.RecentLocal(r) {
			undo()
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "confirm your local password again before making an old-app key", "code": "reauth"})
			return
		}
		if who, ok := auth.From(r.Context()); ok && !who.HasScope(auth.ScopeAdmin) {
			undo()
			fail(w, http.StatusForbidden, "this API key needs the admin scope to make an old-app key")
			return
		}
		if who, ok := auth.From(r.Context()); ok && !who.Has(auth.PermManageLibrary) {
			undo()
			fail(w, http.StatusForbidden, "the old-app key acts as you, and your account may not manage the Library")
			return
		}
		side := sideOf(s.Kind)
		k, err := a.s.Auth.MakeCompatKey(r, side)
		if err != nil {
			undo()
			internal(w, err)
			return
		}
		sides[i].Key = k
		// Shown once only when it replaced a key another app may hold
		// (such as Seerr); a first key goes straight into Prowlarr.
		if have[side] {
			made[side] = k
		} else {
			fresh = append(fresh, side)
		}
	}
	if !a.oldAppOn(ctx) {
		on := true
		if err := a.saveOldAppSwitches(ctx, actor(r), &on, nil); err != nil {
			undo()
			internal(w, err)
			return
		}
		turnedOn = true
	}
	for i, s := range sides {
		if err := p.Connect(ctx, s); err != nil {
			// Take back the entries this request wrote into Prowlarr too.
			var bases []string
			for _, d := range sides[:i] {
				bases = append(bases, d.Base)
			}
			if len(bases) > 0 {
				_ = p.Disconnect(context.WithoutCancel(ctx), bases)
			}
			undo()
			bridgeFail(w, http.StatusBadGateway, err, "")
			return
		}
	}
	var b bridge.Bridge
	if found {
		if !bridgeSameURL(existing.CallbackURL, in.CallbackURL) {
			// HolyArrs is reached at another address now: its old entries go.
			_ = p.Disconnect(ctx, []string{existing.CallbackURL + "/sonarr", existing.CallbackURL + "/radarr"})
		}
		b, err = svc.Update(ctx, existing, in.Key, in.CallbackURL, actor(r))
	} else {
		b, err = svc.Add(ctx, in.Kind, in.URL, in.Key, in.CallbackURL, actor(r))
	}
	if err != nil {
		a.bridgeAddError(w, err)
		return
	}
	syncErr := p.Sync(ctx)
	svc.RecordProwlarr(ctx, b, syncErr)
	b, _ = svc.Get(ctx, b.ID)
	v := a.bridgeView(ctx, b)
	// A key made here is shown once: an app that used the one it replaced
	// (such as Seerr) needs it.
	writeJSON(w, http.StatusCreated, struct {
		BridgeView
		Keys map[string]string `json:"keys,omitempty"`
	}{v, made})
}

func bridgeSameURL(a, b string) bool {
	x, _ := bridge.CleanURL(a)
	y, _ := bridge.CleanURL(b)
	return x != "" && x == y
}

func (a *API) bridgeAddError(w http.ResponseWriter, err error) {
	if errors.Is(err, bridge.ErrTaken) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "That one is connected already.", "field": "url"})
		return
	}
	internal(w, err)
}

func (a *API) bridgeOf(w http.ResponseWriter, r *http.Request) (bridge.Bridge, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		fail(w, http.StatusBadRequest, "the id must be a number")
		return bridge.Bridge{}, false
	}
	b, err := a.bridges().Get(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		fail(w, http.StatusNotFound, "no such connection")
		return b, false
	}
	if err != nil {
		internal(w, err)
		return b, false
	}
	return b, true
}

// checkBridge brings a bridge's sites in step now.
func (a *API) checkBridge(w http.ResponseWriter, r *http.Request) {
	b, ok := a.bridgeOf(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	svc := a.bridges()
	if b.Kind == bridge.KindJackett {
		res, err := svc.CheckJackett(ctx, b, actor(r))
		if err != nil {
			b, _ = svc.Get(ctx, b.ID)
			writeJSON(w, http.StatusBadGateway, map[string]any{"error": bridge.Words(err), "bridge": a.bridgeView(ctx, b)})
			return
		}
		b, _ = svc.Get(ctx, b.ID)
		writeJSON(w, http.StatusOK, map[string]any{"result": res, "bridge": a.bridgeView(ctx, b)})
		return
	}
	key, err := svc.Key(ctx, b)
	if err != nil {
		internal(w, err)
		return
	}
	err = bridge.NewProwlarr(b.URL, key).Sync(ctx)
	svc.RecordProwlarr(ctx, b, err)
	b, _ = svc.Get(ctx, b.ID)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": bridge.Words(err), "bridge": a.bridgeView(ctx, b)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"bridge": a.bridgeView(ctx, b)})
}

// deleteBridge disconnects; ?keep_sources=true keeps its Sources as ordinary ones.
func (a *API) deleteBridge(w http.ResponseWriter, r *http.Request) {
	b, ok := a.bridgeOf(w, r)
	if !ok {
		return
	}
	keep := r.URL.Query().Get("keep_sources") == "true"
	if b.Kind == bridge.KindProwlarr {
		// Take HolyArrs out of Prowlarr's apps (Prowlarr does not remove
		// what it wrote when an app goes), then its Sources go here.
		if !keep {
			key, err := a.bridges().Key(r.Context(), b)
			if err == nil {
				ctx, cancel := context.WithTimeout(r.Context(), time.Minute)
				p := bridge.NewProwlarr(b.URL, key)
				err = p.Disconnect(ctx, []string{b.CallbackURL + "/sonarr", b.CallbackURL + "/radarr"})
				cancel()
			}
			if err != nil {
				bridgeFail(w, http.StatusBadGateway, err, "")
				return
			}
		}
	}
	n, err := a.bridges().Remove(r.Context(), b, keep)
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sources": n, "kept": keep})
}
