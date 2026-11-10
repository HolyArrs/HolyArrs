package sources

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// checkQuery is the title a test search asks for when none is given.
const checkQuery = "The Matrix"

// SourceCheck is one Source's answer to a test search.
type SourceCheck struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Answered bool   `json:"answered"`
	Paused   bool   `json:"paused"`
	Count    int    `json:"count"`
	MS       int64  `json:"ms"`
	Error    string `json:"error,omitempty"`
	Health   Health `json:"health"`
}

// checkSources runs one test search on every enabled Source at once.
// Paused Sources are listed but not asked. It never pauses a Source and
// never keeps Releases; each call is kept for the Source's stats.
func (h *Handlers) checkSources(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Query string `json:"query"`
		Kind  string `json:"kind"`
	}
	if r.ContentLength != 0 && !webserver.Decode(w, r, &in, "send {query, kind}") {
		return
	}
	q := strings.TrimSpace(in.Query)
	if q == "" {
		q = checkQuery
	}
	if len(q) > 200 {
		q = q[:200]
	}
	kind := in.Kind
	if kind != "show" && kind != "music" {
		kind = "movie"
	}
	srcs, err := database.Select[domain.Source](r.Context(), h.Store.Read(), "sources", "state != 'disabled' ORDER BY priority, name")
	if err != nil {
		webserver.Internal(w, "sources", err)
		return
	}
	var sec SecretReader
	if h.Vault != nil {
		sec = h.Vault
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	out := []SourceCheck{}
	var asked []domain.Source
	for _, src := range srcs {
		if src.Format == "rss" || !checkHandles(src, kind) {
			continue
		}
		out = append(out, SourceCheck{ID: src.ID, Name: src.Name, Health: HealthOf(src), Paused: src.State == "paused"})
		asked = append(asked, src)
	}
	var wg sync.WaitGroup
	for i, src := range asked {
		if src.State == "paused" {
			continue
		}
		wg.Add(1)
		go func(c *SourceCheck, src domain.Source) {
			defer wg.Done()
			checkOne(ctx, h, sec, src, q, kind, c)
		}(&out[i], src)
	}
	wg.Wait()
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"query": q, "kind": kind, "sources": out})
}

// checkHandles says whether a Source takes this kind of search.
func checkHandles(src domain.Source, kind string) bool {
	k := map[string]string{"movie": "movie", "show": "tv", "music": "music"}[kind]
	return len(src.Kinds) == 0 || slices.Contains(src.Kinds, k)
}

func checkOne(ctx context.Context, h *Handlers, sec SecretReader, src domain.Source, q, kind string, c *SourceCheck) {
	start := time.Now()
	cfg, err := ConfigFor(ctx, src, sec)
	var items []Item
	if err == nil {
		cfg.Timeout = 20 * time.Second
		key := map[string]string{"movie": "movie", "show": "tv", "music": "music"}[kind]
		cats := src.Categories[key]
		if len(cats) == 0 {
			cats = DefaultCategories[key]
		}
		qq := Query{Title: q, Categories: cats, TV: kind == "show"}
		if kind == "music" {
			qq = Query{Music: true, Artist: q, Categories: cats}
		}
		items, err = New(cfg).Search(ctx, qq)
	}
	took := time.Since(start)
	Record(context.WithoutCancel(ctx), h.Store, src.ID, "test", took, err)
	c.MS = took.Milliseconds()
	if err != nil {
		c.Error = ExplainVia(src.URL, err)
		return
	}
	c.Answered = true
	c.Count = len(items)
}
