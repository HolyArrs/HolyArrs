package sources

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
)

// SourceView is a Source as the API shows it: the key, a passkey in the
// address path and the proxy password are masked, never sent.
type SourceView struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Protocol string `json:"protocol"`
	Format   string `json:"format"`
	// Site is the built-in site a site-format Source reads.
	Site       string              `json:"site,omitempty"`
	URL        string              `json:"url"`
	APIKey     string              `json:"api_key"`
	UseFor     domain.SourceUse    `json:"use_for"`
	Kinds      []string            `json:"kinds"`
	Categories map[string][]int    `json:"categories"`
	MinSeeders *int                `json:"min_seeders"`
	Limits     domain.SourceLimits `json:"limits"`
	Priority   int                 `json:"priority"`
	// State is the stored state (enabled, paused, disabled); Health says
	// it in plain words (working, unreachable, auth_failed, disabled).
	State  string `json:"state"`
	Health Health `json:"health"`
	// AllowLocal says the Source may reach the local network.
	AllowLocal      bool     `json:"allow_local_network"`
	Proxy           string   `json:"proxy"`
	ProxyPassword   string   `json:"proxy_password"`
	ProxyAllowLocal bool     `json:"proxy_allow_local_network"`
	Summary         Summary  `json:"summary"`
	Tags            []string `json:"tags"`
}

// SourceViewOf is x as the API shows it, with its summary.
func SourceViewOf(x domain.Source, sum Summary) SourceView {
	v := SourceView{ID: x.ID, Name: x.Name, Protocol: x.Protocol, Format: x.Format, Site: x.Extra[extraSiteKind], URL: MaskURL(x.URL), UseFor: x.UseFor, Kinds: x.Kinds,
		Categories: x.Categories, MinSeeders: x.MinSeeders, Limits: x.Limits, Priority: x.Priority, State: x.State, Health: HealthOf(x),
		AllowLocal: x.AllowLocal, Proxy: x.ProxyURL, ProxyAllowLocal: x.ProxyAllowLocal, Summary: sum, Tags: x.Tags}
	if v.Tags == nil {
		v.Tags = []string{}
	}
	if v.Kinds == nil {
		v.Kinds = []string{}
	}
	if x.SecretRef != nil {
		v.APIKey = secrets.Masked
	}
	if x.ProxySecretRef != nil {
		v.ProxyPassword = secrets.Masked
	}
	return v
}

func (h *Handlers) listSources(w http.ResponseWriter, r *http.Request) {
	srcs, err := database.Select[domain.Source](r.Context(), h.Store.Read(), "sources", "1=1 ORDER BY priority, name")
	if err != nil {
		webserver.Internal(w, "sources", err)
		return
	}
	sums, err := h.search().Summarise(r.Context(), time.Now())
	if err != nil {
		webserver.Internal(w, "sources", err)
		return
	}
	out := []SourceView{}
	for _, x := range srcs {
		out = append(out, SourceViewOf(x, sums[x.ID]))
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"sources": out})
}

// listSites lists the built-in sites a Source can read directly.
func (h *Handlers) listSites(w http.ResponseWriter, r *http.Request) {
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"sites": Sites})
}

const sourceHelp = `send name, format (torznab, newznab, rss or site; a site Source also sends site: knaben, torrentscsv, subsplease or iptorrents), url, and if needed api_key, protocol, use_for {feed, auto_search, hand_search}, kinds, categories, min_seeders, limits, priority (1 to 50), allow_local_network, proxy, proxy_password, proxy_allow_local_network`

// readSource reads a Source body; the masked placeholder keeps the stored secret.
func readSource(w http.ResponseWriter, r *http.Request) (Input, bool) {
	var in Input
	if !webserver.Decode(w, r, &in, sourceHelp) {
		return in, false
	}
	if in.APIKey == secrets.Masked {
		in.APIKey = ""
	}
	if in.ProxyPassword == secrets.Masked {
		in.ProxyPassword = ""
	}
	return in, true
}

func sourceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNameUsed), errors.Is(err, ErrKeyAgain), errors.Is(err, ErrKeyAgainProxy), errors.Is(err, ErrProxyAgain),
		errors.Is(err, ErrPathKey), errors.Is(err, ErrPathKeyNone), errors.Is(err, ErrPathKeyRSS):
		webserver.Fail(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrNoVault):
		webserver.Fail(w, http.StatusServiceUnavailable, err.Error())
	case errors.Is(err, database.ErrNotFound):
		webserver.Fail(w, http.StatusNotFound, "no such Source")
	case errors.Is(err, ErrName), errors.Is(err, ErrFormat), errors.Is(err, ErrSite), errors.Is(err, ErrProtocol), errors.Is(err, ErrURL),
		errors.Is(err, ErrPriority), errors.Is(err, ErrLimits), errors.Is(err, ErrKinds), errors.Is(err, ErrUse),
		errors.Is(err, ErrProxy), errors.Is(err, ErrSeeders):
		webserver.Fail(w, http.StatusBadRequest, err.Error())
	default:
		webserver.Internal(w, "sources", err)
	}
}

func sourceID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		webserver.Fail(w, http.StatusNotFound, "no such Source")
		return 0, false
	}
	return id, true
}

// PublishSource tells open screens that a Source changed.
func (h *Handlers) PublishSource(x domain.Source) {
	if h.Live != nil {
		h.Live.Publish("source", map[string]any{"id": x.ID, "health": HealthOf(x)})
	}
}

func (h *Handlers) addSource(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	in, ok := readSource(w, r)
	if !ok {
		return
	}
	x, err := Add(r.Context(), h.Store, h.Vault, in, actorOf(r))
	if err != nil {
		sourceError(w, err)
		return
	}
	h.PublishSource(x)
	webserver.WriteJSON(w, http.StatusCreated, SourceViewOf(x, Summary{}))
}

func (h *Handlers) editSource(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	id, ok := sourceID(w, r)
	if !ok {
		return
	}
	in, ok := readSource(w, r)
	if !ok {
		return
	}
	x, err := Change(r.Context(), h.Store, h.Vault, id, in, actorOf(r))
	if err != nil {
		sourceError(w, err)
		return
	}
	h.PublishSource(x)
	webserver.WriteJSON(w, http.StatusOK, SourceViewOf(x, Summary{}))
}

func (h *Handlers) deleteSource(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	id, ok := sourceID(w, r)
	if !ok {
		return
	}
	if err := Remove(r.Context(), h.Store, id); err != nil {
		sourceError(w, err)
		return
	}
	h.search().ForgetSource(id)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) setSourcePaused(w http.ResponseWriter, r *http.Request, paused bool) {
	id, ok := sourceID(w, r)
	if !ok {
		return
	}
	x, err := SetPaused(r.Context(), h.Store, id, paused, actorOf(r))
	if err != nil {
		sourceError(w, err)
		return
	}
	h.PublishSource(x)
	webserver.WriteJSON(w, http.StatusOK, SourceViewOf(x, Summary{}))
}

func (h *Handlers) pauseSource(w http.ResponseWriter, r *http.Request) { h.setSourcePaused(w, r, true) }
func (h *Handlers) resumeSource(w http.ResponseWriter, r *http.Request) {
	h.setSourcePaused(w, r, false)
}

// TestSource asks a Source for its caps (or reads its rss feed) and says
// the result in plain words. A working answer ends a back-off pause; a
// failed test leaves the Source as it was.
func (h *Handlers) TestSource(w http.ResponseWriter, r *http.Request) {
	id, ok := sourceID(w, r)
	if !ok {
		return
	}
	var x domain.Source
	if err := database.Get(r.Context(), h.Store.Read(), "sources", id, &x); err != nil {
		sourceError(w, err)
		return
	}
	h.runTest(w, r, x, false)
}

// trySource tries a paused Source now: it ends the wait, runs the same
// check as a test, and on a failure puts the back-off pause back.
func (h *Handlers) trySource(w http.ResponseWriter, r *http.Request) {
	id, ok := sourceID(w, r)
	if !ok {
		return
	}
	var x domain.Source
	if err := database.Get(r.Context(), h.Store.Read(), "sources", id, &x); err != nil {
		sourceError(w, err)
		return
	}
	if x.State == "paused" {
		y, err := SetPaused(r.Context(), h.Store, id, false, actorOf(r))
		if err != nil {
			sourceError(w, err)
			return
		}
		x = y
	}
	h.runTest(w, r, x, true)
}

// runTest checks a Source and writes the answer. repause puts a failing
// Source back on its back-off pause.
func (h *Handlers) runTest(w http.ResponseWriter, r *http.Request, x domain.Source, repause bool) {
	var sec SecretReader
	if h.Vault != nil {
		sec = h.Vault
	}
	cfg, err := ConfigFor(r.Context(), x, sec)
	if err != nil {
		webserver.Internal(w, "sources", err)
		return
	}
	cfg.Timeout = 20 * time.Second
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	start := time.Now()
	items, err := New(cfg).Test(ctx)
	took := time.Since(start)
	Record(context.WithoutCancel(r.Context()), h.Store, x.ID, "test", took, err)
	out := map[string]any{"ok": err == nil, "ms": took.Milliseconds()}
	if err != nil {
		out["text"] = ExplainVia(x.URL, err)
		if repause && x.State == "enabled" && h.search() != nil {
			h.search().TryFailed(context.WithoutCancel(r.Context()), x, err)
			if database.Get(r.Context(), h.Store.Read(), "sources", x.ID, &x) == nil {
				h.PublishSource(x)
			}
		} else if x.State == "enabled" {
			// Keep why it failed, so the Source does not read as working
			// once the page is opened again.
			kind, detail := KindDown, ""
			var se *Error
			if errors.As(err, &se) {
				kind, detail = se.Kind, se.Detail
			}
			if h.Store.Write(r.Context(), func(tx *sql.Tx) error { return markTestFailedTx(tx, r.Context(), x.ID, kind, detail) }) == nil &&
				database.Get(r.Context(), h.Store.Read(), "sources", x.ID, &x) == nil {
				h.PublishSource(x)
			}
		}
	} else {
		if x.Extra[extraTestFailed] != "" {
			if h.Store.Write(r.Context(), func(tx *sql.Tx) error { return recoverSourceTx(tx, r.Context(), x.ID) }) == nil &&
				database.Get(r.Context(), h.Store.Read(), "sources", x.ID, &x) == nil {
				h.PublishSource(x)
			}
		}
		out["text"] = "The Source answered."
		if x.Format == "rss" {
			out["text"] = "The feed answered with " + strconv.Itoa(items) + " Releases."
		}
		out["items"] = items
		if x.State == "paused" {
			if y, err := SetPaused(r.Context(), h.Store, x.ID, false, actorOf(r)); err == nil {
				x = y
				h.PublishSource(x)
			}
		}
	}
	out["health"] = HealthOf(x)
	webserver.WriteJSON(w, http.StatusOK, out)
}

func (h *Handlers) sourceStats(w http.ResponseWriter, r *http.Request) {
	id, ok := sourceID(w, r)
	if !ok {
		return
	}
	var x domain.Source
	if err := database.Get(r.Context(), h.Store.Read(), "sources", id, &x); err != nil {
		sourceError(w, err)
		return
	}
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days < 1 || days > 90 {
		days = 14
	}
	st, err := h.search().StatsFor(r.Context(), id, days, time.Now())
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		webserver.Internal(w, "sources", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"source": SourceViewOf(x, st.Summary), "days": st.Days, "recent": st.Recent})
}
