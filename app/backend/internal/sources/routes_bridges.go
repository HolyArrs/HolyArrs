package sources

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// Connect Jackett or Prowlarr: a guided setup in Settings > Sources
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

// bridgeBody is a Jackett or Prowlarr to test or connect.
type bridgeBody struct {
	Kind        string `json:"kind"`
	URL         string `json:"url"`
	Key         string `json:"key"`
	CallbackURL string `json:"callback_url"`
	// NewKeys lets the setup replace an arr key that exists already
	// (an app using the old one must get the new one).
	NewKeys bool `json:"new_keys"`
	// Keys are arr keys typed in, by side (sonarr, radarr), used instead of making new ones.
	Keys map[string]string `json:"keys"`
}

const bridgeHelp = `send {"kind": "jackett" or "prowlarr", "url": "http://jackett:9117", "key": "..."}`

func (h *Handlers) bridges() *BridgeService {
	h.bridgeOnce.Do(func() {
		if h.Bridges != nil {
			h.bridgeSvc = h.Bridges
			return
		}
		h.bridgeSvc = &BridgeService{Store: h.Store, Vault: h.Vault}
	})
	return h.bridgeSvc
}

func (h *Handlers) bridgeView(ctx context.Context, b Bridge) BridgeView {
	svc := h.bridges()
	return BridgeView{ID: b.ID, Kind: b.Kind, URL: b.URL, CallbackURL: b.CallbackURL, LastCheck: svc.LastChecked(b), LastError: b.LastError,
		Sites: b.SiteCount, Sources: svc.SourceCount(ctx, b), Local: b.Kind == BridgeKindJackett && b.Extra["local"] == "1"}
}

func bridgeFail(w http.ResponseWriter, status int, err error, field string) {
	var be *BridgeError
	if errors.As(err, &be) {
		if be.Detail != "" {
			log.Printf("sources: bridge: %s (%s)", be.Words, be.Detail)
		}
	} else if err != nil {
		log.Printf("sources: bridge: %v", err)
	}
	body := map[string]string{"error": BridgeWords(err)}
	if field != "" {
		body["field"] = field
	}
	webserver.WriteJSON(w, status, body)
}

// cleanBridge checks the kind and the addresses of an input.
func cleanBridge(w http.ResponseWriter, in *bridgeBody) bool {
	if in.Kind != BridgeKindJackett && in.Kind != BridgeKindProwlarr {
		webserver.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "Pick Jackett or Prowlarr.", "field": "kind"})
		return false
	}
	u, err := BridgeCleanURL(in.URL)
	if err != nil {
		bridgeFail(w, http.StatusBadRequest, err, "url")
		return false
	}
	in.URL = u
	if in.Key == "" {
		webserver.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "Paste the key.", "field": "key"})
		return false
	}
	if in.Kind == BridgeKindProwlarr {
		cb, err := BridgeCleanURL(in.CallbackURL)
		if err != nil {
			webserver.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "Give the address Prowlarr can reach HolyArrs at, such as http://192.168.1.10:6262.", "field": "callback_url"})
			return false
		}
		in.CallbackURL = cb
	}
	return true
}

func (h *Handlers) listBridges(w http.ResponseWriter, r *http.Request) {
	bs, err := h.bridges().List(r.Context())
	if err != nil {
		webserver.Internal(w, "sources", err)
		return
	}
	out := []BridgeView{}
	for _, b := range bs {
		out = append(out, h.bridgeView(r.Context(), b))
	}
	// home_ip: this machine's home-network address, for the page to offer as
	// HolyArrs's address for Prowlarr when the browser came in over Tailscale
	// or localhost (Prowlarr usually can't reach those).
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"bridges": out, "home_ip": localAddress()})
}

// localAddress is the first 10.x or 192.168.x address of this machine, or "".
// 172.16/12 is skipped: inside Docker it is the container's own network.
func localAddress() string {
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

// bridgeTries caps tests and searches per person: each one makes this
// server call addresses on the home network.
const bridgeTries = 10

// bridgeLimiter counts each person's bridge tests and searches in the last minute.
type bridgeLimiter struct {
	mu   sync.Mutex
	hits map[int64][]time.Time
}

// allow notes one try and says whether it is within the limit.
func (l *bridgeLimiter) allow(user int64, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.hits == nil {
		l.hits = map[int64][]time.Time{}
	}
	kept := l.hits[user][:0]
	for _, t := range l.hits[user] {
		if now.Sub(t) < time.Minute {
			kept = append(kept, t)
		}
	}
	if len(kept) >= bridgeTries {
		l.hits[user] = kept
		return false
	}
	l.hits[user] = append(kept, now)
	return true
}

// bridgeAllowed writes 429 when the person has tried too often.
func (h *Handlers) bridgeAllowed(w http.ResponseWriter, r *http.Request) bool {
	who, _ := webserver.From(r.Context())
	if h.bridgeLimit.allow(who.UserID, time.Now()) {
		return true
	}
	w.Header().Set("Retry-After", "60")
	webserver.WriteJSON(w, http.StatusTooManyRequests, map[string]string{"error": "Too many tries. Wait a minute and try again."})
	return false
}

func (h *Handlers) findBridges(w http.ResponseWriter, r *http.Request) {
	if !h.bridgeAllowed(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()
	found := FindBridges(ctx)
	if found == nil {
		found = []BridgeFound{}
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"found": found})
}

// prowlarrSides are the HolyArrs sides Prowlarr connects to: TV when Shows
// is on, movies when Movies is on.
func (h *Handlers) prowlarrSides(callback string) []BridgeSide {
	var out []BridgeSide
	if h.PartOn(parts.Shows) {
		out = append(out, BridgeSide{Kind: "Sonarr", Name: "HolyArrs TV shows", Base: callback + "/sonarr"})
	}
	if h.PartOn(parts.Movies) {
		out = append(out, BridgeSide{Kind: "Radarr", Name: "HolyArrs Movies", Base: callback + "/radarr"})
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
func (h *Handlers) testBridge(w http.ResponseWriter, r *http.Request) {
	if !h.bridgeAllowed(w, r) {
		return
	}
	var in bridgeBody
	if !webserver.Decode(w, r, &in, bridgeHelp) || !cleanBridge(w, &in) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if in.Kind == BridgeKindJackett {
		sites, err := NewBridgeJackett(in.URL, in.Key).Sites(ctx)
		if err != nil {
			bridgeFail(w, http.StatusBadGateway, err, "")
			return
		}
		titles := []string{}
		for _, s := range sites {
			titles = append(titles, s.Title)
		}
		webserver.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "text": "Jackett answered with " + siteCountText(len(sites)) + ".", "sites": titles})
		return
	}
	version, err := NewBridgeProwlarr(in.URL, in.Key).Status(ctx)
	if err != nil {
		bridgeFail(w, http.StatusBadGateway, err, "")
		return
	}
	have, err := h.compatSides(ctx)
	if err != nil {
		webserver.Internal(w, "sources", err)
		return
	}
	need := map[string]string{}
	for _, s := range h.prowlarrSides(in.CallbackURL) {
		need[sideOf(s.Kind)] = map[bool]string{true: "have", false: "new"}[have[sideOf(s.Kind)]]
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "text": "Prowlarr " + version + " answered.", "keys": need})
}

func siteCountText(n int) string {
	if n == 1 {
		return "1 site"
	}
	return strconv.Itoa(n) + " sites"
}

// compatSides says which arr sides already have a key.
func (h *Handlers) compatSides(ctx context.Context) (map[string]bool, error) {
	return h.OldApps.KeySides(ctx)
}

// addBridge connects a Jackett or Prowlarr and brings its sites in. One
// already connected at that address takes the new key (and, for Prowlarr,
// the new address HolyArrs is reached at) instead.
func (h *Handlers) addBridge(w http.ResponseWriter, r *http.Request) {
	var in bridgeBody
	if !webserver.Decode(w, r, &in, bridgeHelp) || !cleanBridge(w, &in) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	svc := h.bridges()
	existing, found := svc.Find(ctx, in.Kind, in.URL)
	if in.Kind == BridgeKindJackett {
		if _, err := NewBridgeJackett(in.URL, in.Key).Sites(ctx); err != nil {
			bridgeFail(w, http.StatusBadGateway, err, "")
			return
		}
		var b Bridge
		var err error
		if found {
			b, err = svc.Update(ctx, existing, in.Key, "", actorOf(r))
		} else {
			b, err = svc.Add(ctx, in.Kind, in.URL, in.Key, "", actorOf(r))
		}
		if err != nil {
			h.bridgeAddError(w, err)
			return
		}
		// A failed first check stays on the connection as its last error.
		_, _ = svc.CheckJackett(ctx, b, actorOf(r))
		b, _ = svc.Get(ctx, b.ID)
		webserver.WriteJSON(w, http.StatusCreated, h.bridgeView(ctx, b))
		return
	}
	p := NewBridgeProwlarr(in.URL, in.Key)
	if _, err := p.Status(ctx); err != nil {
		bridgeFail(w, http.StatusBadGateway, err, "")
		return
	}
	sides := h.prowlarrSides(in.CallbackURL)
	if len(sides) == 0 {
		webserver.WriteJSON(w, http.StatusConflict, map[string]string{"error": "Movies and TV shows are both off, so Prowlarr has nothing to send. Turn one on first."})
		return
	}
	have, err := h.compatSides(ctx)
	if err != nil {
		webserver.Internal(w, "sources", err)
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
		webserver.WriteJSON(w, http.StatusConflict, map[string]any{"error": "HolyArrs already has an arr key for this. Make a new one (an app using the old one, such as Seerr, must get the new one too), or type the key you have.", "need": "keys", "sides": replace})
		return
	}
	// What this request made, so a failure takes it back: keys for sides
	// that had none, and the arr API switch.
	made := map[string]string{}
	var fresh []string
	turnedOn := false
	undo := func() {
		uctx, ucancel := context.WithTimeout(context.WithoutCancel(r.Context()), 30*time.Second)
		defer ucancel()
		for _, side := range fresh {
			_ = h.OldApps.DeleteKey(r.WithContext(uctx), side)
		}
		if turnedOn {
			_ = h.OldApps.SetOn(uctx, actorOf(r), false)
		}
	}
	for i, s := range sides {
		if s.Key != "" || !h.OldApps.CanMakeKeys() {
			continue
		}
		// The same gates as making an arr key by hand: a recent password
		// sign-in, an API key with the admin scope, and a person who may
		// manage the Library (the key acts as them).
		if !webserver.RequireStrong(w, r) {
			undo()
			return
		}
		// Making a key is a risky action: a browser session confirms its
		// password first, as on the arr key routes (only here, so adding
		// a bridge without a new key never asks).
		if who, ok := webserver.From(r.Context()); ok && who.Via == "session" && !accounts.RecentLocal(r) {
			undo()
			webserver.WriteJSON(w, http.StatusForbidden, map[string]string{"error": "confirm your local password again before making an arr key", "code": "reauth"})
			return
		}
		if who, ok := webserver.From(r.Context()); ok && !who.HasScope(webserver.ScopeAdmin) {
			undo()
			webserver.Fail(w, http.StatusForbidden, "this API key needs the admin scope to make an arr key")
			return
		}
		if who, ok := webserver.From(r.Context()); ok && !who.Has(webserver.PermManageLibrary) {
			undo()
			webserver.Fail(w, http.StatusForbidden, "the arr key acts as you, and your account may not manage the Library")
			return
		}
		side := sideOf(s.Kind)
		k, err := h.OldApps.MakeKey(r, side)
		if err != nil {
			undo()
			webserver.Internal(w, "sources", err)
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
	if !h.OldApps.On(ctx) {
		if err := h.OldApps.SetOn(ctx, actorOf(r), true); err != nil {
			undo()
			webserver.Internal(w, "sources", err)
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
	var b Bridge
	if found {
		if !bridgeSameURL(existing.CallbackURL, in.CallbackURL) {
			// HolyArrs is reached at another address now: its old entries go.
			_ = p.Disconnect(ctx, []string{existing.CallbackURL + "/sonarr", existing.CallbackURL + "/radarr"})
		}
		b, err = svc.Update(ctx, existing, in.Key, in.CallbackURL, actorOf(r))
	} else {
		b, err = svc.Add(ctx, in.Kind, in.URL, in.Key, in.CallbackURL, actorOf(r))
	}
	if err != nil {
		h.bridgeAddError(w, err)
		return
	}
	syncErr := p.Sync(ctx)
	svc.RecordProwlarr(ctx, b, syncErr)
	b, _ = svc.Get(ctx, b.ID)
	v := h.bridgeView(ctx, b)
	// A key made here is shown once: an app that used the one it replaced
	// (such as Seerr) needs it.
	webserver.WriteJSON(w, http.StatusCreated, struct {
		BridgeView
		Keys map[string]string `json:"keys,omitempty"`
	}{v, made})
}

func bridgeSameURL(a, b string) bool {
	x, _ := BridgeCleanURL(a)
	y, _ := BridgeCleanURL(b)
	return x != "" && x == y
}

func (h *Handlers) bridgeAddError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrBridgeTaken) {
		webserver.WriteJSON(w, http.StatusConflict, map[string]string{"error": "That one is connected already.", "field": "url"})
		return
	}
	webserver.Internal(w, "sources", err)
}

func (h *Handlers) bridgeOf(w http.ResponseWriter, r *http.Request) (Bridge, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		webserver.Fail(w, http.StatusBadRequest, "the id must be a number")
		return Bridge{}, false
	}
	b, err := h.bridges().Get(r.Context(), id)
	if errors.Is(err, database.ErrNotFound) {
		webserver.Fail(w, http.StatusNotFound, "no such connection")
		return b, false
	}
	if err != nil {
		webserver.Internal(w, "sources", err)
		return b, false
	}
	return b, true
}

// checkBridge brings a bridge's sites in step now.
func (h *Handlers) checkBridge(w http.ResponseWriter, r *http.Request) {
	b, ok := h.bridgeOf(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	svc := h.bridges()
	if b.Kind == BridgeKindJackett {
		res, err := svc.CheckJackett(ctx, b, actorOf(r))
		if err != nil {
			b, _ = svc.Get(ctx, b.ID)
			webserver.WriteJSON(w, http.StatusBadGateway, map[string]any{"error": BridgeWords(err), "bridge": h.bridgeView(ctx, b)})
			return
		}
		b, _ = svc.Get(ctx, b.ID)
		webserver.WriteJSON(w, http.StatusOK, map[string]any{"result": res, "bridge": h.bridgeView(ctx, b)})
		return
	}
	key, err := svc.Key(ctx, b)
	if err != nil {
		webserver.Internal(w, "sources", err)
		return
	}
	err = NewBridgeProwlarr(b.URL, key).Sync(ctx)
	svc.RecordProwlarr(ctx, b, err)
	b, _ = svc.Get(ctx, b.ID)
	if err != nil {
		webserver.WriteJSON(w, http.StatusBadGateway, map[string]any{"error": BridgeWords(err), "bridge": h.bridgeView(ctx, b)})
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"bridge": h.bridgeView(ctx, b)})
}

// deleteBridge disconnects; ?keep_sources=true keeps its Sources as ordinary ones.
func (h *Handlers) deleteBridge(w http.ResponseWriter, r *http.Request) {
	b, ok := h.bridgeOf(w, r)
	if !ok {
		return
	}
	keep := r.URL.Query().Get("keep_sources") == "true"
	if b.Kind == BridgeKindProwlarr {
		// Take HolyArrs out of Prowlarr's apps (Prowlarr does not remove
		// what it wrote when an app goes), then its Sources go here.
		if !keep {
			key, err := h.bridges().Key(r.Context(), b)
			if err == nil {
				ctx, cancel := context.WithTimeout(r.Context(), time.Minute)
				p := NewBridgeProwlarr(b.URL, key)
				err = p.Disconnect(ctx, []string{b.CallbackURL + "/sonarr", b.CallbackURL + "/radarr"})
				cancel()
			}
			if err != nil {
				bridgeFail(w, http.StatusBadGateway, err, "")
				return
			}
		}
	}
	n, err := h.bridges().Remove(r.Context(), b, keep)
	if err != nil {
		webserver.Internal(w, "sources", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"sources": n, "kept": keep})
}
