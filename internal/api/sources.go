package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/CoderLuii/holyarrs/internal/auth"
	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/secrets"
	"github.com/CoderLuii/holyarrs/internal/source"
	"github.com/CoderLuii/holyarrs/internal/store"
)

func (a *API) sourceRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/sources", a.listSources)
	m.HandleFunc("POST /api/v1/sources", a.addSource)
	m.HandleFunc("PUT /api/v1/sources/{id}", a.editSource)
	m.HandleFunc("DELETE /api/v1/sources/{id}", a.deleteSource)
	m.HandleFunc("POST /api/v1/sources/{id}/test", a.testSource)
	m.HandleFunc("POST /api/v1/sources/{id}/pause", a.pauseSource)
	m.HandleFunc("POST /api/v1/sources/{id}/resume", a.resumeSource)
	m.HandleFunc("GET /api/v1/sources/{id}/stats", a.sourceStats)
	m.HandleFunc("POST /api/v1/sources/{id}/try", a.trySource)
	m.HandleFunc("POST /api/v1/sources/check", a.checkSources)
	m.HandleFunc("GET /api/v1/titles/{id}/search-outcome", a.searchOutcome)
}

// SourceView is a Source as the API shows it: the key, a passkey in the
// address path and the proxy password are masked, never sent.
type SourceView struct {
	ID         int64               `json:"id"`
	Name       string              `json:"name"`
	Protocol   string              `json:"protocol"`
	Format     string              `json:"format"`
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
	State  string        `json:"state"`
	Health source.Health `json:"health"`
	// AllowLocal says the Source may reach the local network.
	AllowLocal      bool           `json:"allow_local_network"`
	Proxy           string         `json:"proxy"`
	ProxyPassword   string         `json:"proxy_password"`
	ProxyAllowLocal bool           `json:"proxy_allow_local_network"`
	Summary         source.Summary `json:"summary"`
}

func sourceView(x domain.Source, sum source.Summary) SourceView {
	v := SourceView{ID: x.ID, Name: x.Name, Protocol: x.Protocol, Format: x.Format, URL: source.MaskURL(x.URL), UseFor: x.UseFor, Kinds: x.Kinds,
		Categories: x.Categories, MinSeeders: x.MinSeeders, Limits: x.Limits, Priority: x.Priority, State: x.State, Health: source.HealthOf(x),
		AllowLocal: x.AllowLocal, Proxy: x.ProxyURL, ProxyAllowLocal: x.ProxyAllowLocal, Summary: sum}
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

func (a *API) listSources(w http.ResponseWriter, r *http.Request) {
	srcs, err := store.Select[domain.Source](r.Context(), a.s.Store.Read(), "sources", "1=1 ORDER BY priority, name")
	if err != nil {
		internal(w, err)
		return
	}
	sums, err := a.s.Search.Summarise(r.Context(), time.Now())
	if err != nil {
		internal(w, err)
		return
	}
	out := []SourceView{}
	for _, x := range srcs {
		out = append(out, sourceView(x, sums[x.ID]))
	}
	writeJSON(w, http.StatusOK, map[string]any{"sources": out})
}

const sourceHelp = `send name, format (torznab, newznab or rss), url, and if needed api_key, protocol, use_for {feed, auto_search, hand_search}, kinds, categories, min_seeders, limits, priority (1 to 50), allow_local_network, proxy, proxy_password, proxy_allow_local_network`

// readSource reads a Source body; the masked placeholder keeps the stored secret.
func readSource(w http.ResponseWriter, r *http.Request) (source.Input, bool) {
	var in source.Input
	if !decode(w, r, &in, sourceHelp) {
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
	case errors.Is(err, source.ErrNameUsed), errors.Is(err, source.ErrKeyAgain), errors.Is(err, source.ErrKeyAgainProxy), errors.Is(err, source.ErrProxyAgain),
		errors.Is(err, source.ErrPathKey):
		fail(w, http.StatusConflict, err.Error())
	case errors.Is(err, source.ErrNoVault):
		fail(w, http.StatusServiceUnavailable, err.Error())
	case errors.Is(err, store.ErrNotFound):
		fail(w, http.StatusNotFound, "no such Source")
	case errors.Is(err, source.ErrName), errors.Is(err, source.ErrFormat), errors.Is(err, source.ErrProtocol), errors.Is(err, source.ErrURL),
		errors.Is(err, source.ErrPriority), errors.Is(err, source.ErrLimits), errors.Is(err, source.ErrKinds), errors.Is(err, source.ErrUse),
		errors.Is(err, source.ErrProxy), errors.Is(err, source.ErrSeeders):
		fail(w, http.StatusBadRequest, err.Error())
	default:
		internal(w, err)
	}
}

func sourceID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		fail(w, http.StatusNotFound, "no such Source")
		return 0, false
	}
	return id, true
}

func (a *API) publishSource(x domain.Source) {
	if a.s.Live != nil {
		a.s.Live.Publish("source", map[string]any{"id": x.ID, "health": source.HealthOf(x)})
	}
}

func (a *API) addSource(w http.ResponseWriter, r *http.Request) {
	if !auth.RequireStrong(w, r) {
		return
	}
	in, ok := readSource(w, r)
	if !ok {
		return
	}
	x, err := source.Add(r.Context(), a.s.Store, a.s.Secrets, in, actor(r))
	if err != nil {
		sourceError(w, err)
		return
	}
	a.publishSource(x)
	writeJSON(w, http.StatusCreated, sourceView(x, source.Summary{}))
}

func (a *API) editSource(w http.ResponseWriter, r *http.Request) {
	if !auth.RequireStrong(w, r) {
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
	x, err := source.Change(r.Context(), a.s.Store, a.s.Secrets, id, in, actor(r))
	if err != nil {
		sourceError(w, err)
		return
	}
	a.publishSource(x)
	writeJSON(w, http.StatusOK, sourceView(x, source.Summary{}))
}

func (a *API) deleteSource(w http.ResponseWriter, r *http.Request) {
	if !auth.RequireStrong(w, r) {
		return
	}
	id, ok := sourceID(w, r)
	if !ok {
		return
	}
	if err := source.Remove(r.Context(), a.s.Store, id); err != nil {
		sourceError(w, err)
		return
	}
	a.s.Search.ForgetSource(id)
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) setSourcePaused(w http.ResponseWriter, r *http.Request, paused bool) {
	id, ok := sourceID(w, r)
	if !ok {
		return
	}
	x, err := source.SetPaused(r.Context(), a.s.Store, id, paused, actor(r))
	if err != nil {
		sourceError(w, err)
		return
	}
	a.publishSource(x)
	writeJSON(w, http.StatusOK, sourceView(x, source.Summary{}))
}

func (a *API) pauseSource(w http.ResponseWriter, r *http.Request)  { a.setSourcePaused(w, r, true) }
func (a *API) resumeSource(w http.ResponseWriter, r *http.Request) { a.setSourcePaused(w, r, false) }

// testSource asks a Source for its caps (or reads its rss feed) and says
// the result in plain words. A working answer ends a back-off pause; a
// failed test leaves the Source as it was.
func (a *API) testSource(w http.ResponseWriter, r *http.Request) {
	id, ok := sourceID(w, r)
	if !ok {
		return
	}
	var x domain.Source
	if err := store.Get(r.Context(), a.s.Store.Read(), "sources", id, &x); err != nil {
		sourceError(w, err)
		return
	}
	a.runTest(w, r, x, false)
}

// trySource tries a paused Source now: it ends the wait, runs the same
// check as a test, and on a failure puts the back-off pause back.
func (a *API) trySource(w http.ResponseWriter, r *http.Request) {
	id, ok := sourceID(w, r)
	if !ok {
		return
	}
	var x domain.Source
	if err := store.Get(r.Context(), a.s.Store.Read(), "sources", id, &x); err != nil {
		sourceError(w, err)
		return
	}
	if x.State == "paused" {
		y, err := source.SetPaused(r.Context(), a.s.Store, id, false, actor(r))
		if err != nil {
			sourceError(w, err)
			return
		}
		x = y
	}
	a.runTest(w, r, x, true)
}

// runTest checks a Source and writes the answer. repause puts a failing
// Source back on its back-off pause.
func (a *API) runTest(w http.ResponseWriter, r *http.Request, x domain.Source, repause bool) {
	var sec source.SecretReader
	if a.s.Secrets != nil {
		sec = a.s.Secrets
	}
	cfg, err := source.ConfigFor(r.Context(), x, sec)
	if err != nil {
		internal(w, err)
		return
	}
	cfg.Timeout = 20 * time.Second
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	start := time.Now()
	items, err := source.New(cfg).Test(ctx)
	took := time.Since(start)
	source.Record(context.WithoutCancel(r.Context()), a.s.Store, x.ID, "test", took, err)
	out := map[string]any{"ok": err == nil, "ms": took.Milliseconds()}
	if err != nil {
		out["text"] = source.Explain(err)
		if repause && x.State == "enabled" && a.s.Search != nil {
			a.s.Search.TryFailed(context.WithoutCancel(r.Context()), x, err)
			if store.Get(r.Context(), a.s.Store.Read(), "sources", x.ID, &x) == nil {
				a.publishSource(x)
			}
		}
	} else {
		out["text"] = "The Source answered."
		if x.Format == "rss" {
			out["text"] = "The feed answered with " + strconv.Itoa(items) + " Releases."
		}
		out["items"] = items
		if x.State == "paused" {
			if y, err := source.SetPaused(r.Context(), a.s.Store, x.ID, false, actor(r)); err == nil {
				x = y
				a.publishSource(x)
			}
		}
	}
	out["health"] = source.HealthOf(x)
	writeJSON(w, http.StatusOK, out)
}

func (a *API) sourceStats(w http.ResponseWriter, r *http.Request) {
	id, ok := sourceID(w, r)
	if !ok {
		return
	}
	var x domain.Source
	if err := store.Get(r.Context(), a.s.Store.Read(), "sources", id, &x); err != nil {
		sourceError(w, err)
		return
	}
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days < 1 || days > 90 {
		days = 14
	}
	st, err := a.s.Search.StatsFor(r.Context(), id, days, time.Now())
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"source": sourceView(x, st.Summary), "days": st.Days, "recent": st.Recent})
}
