package requests

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// AskedByHandlers answer who asked for which titles.
type AskedByHandlers struct {
	Store *database.Store
	// KidMax is the age limit of the signed-in Kid, or "" for everyone else.
	KidMax func(r *http.Request) string
	// AllowedTitleIDs keeps the ids within a Kid's age limit.
	AllowedTitleIDs func(ctx context.Context, max string, ids []int64) (map[int64]bool, error)
}

// Routes registers the asked-by route.
func (h *AskedByHandlers) Routes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/asked-by", h.getAskedBy)
}

// viewerID is the signed-in person's id (1, the owner, without sign-in).
func viewerID(ctx context.Context) int64 {
	if who, ok := webserver.From(ctx); ok {
		return who.UserID
	}
	return 1
}

// AskedByEntry is one title's askers.
type AskedByEntry struct {
	Askers []askedByPerson `json:"askers"`
}

type askedByPerson struct {
	ID      int64     `json:"id"`
	Name    string    `json:"name"`
	AskedAt time.Time `json:"asked_at"`
	Me      bool      `json:"me"`
}

// getAskedBy answers ?title_id=1&title_id=2 (or title_ids=1,2) with each
// title's askers, earliest first. Everyone in the household sees every
// asker; a Kid only gets titles within their age limit.
func (h *AskedByHandlers) getAskedBy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var ids []int64
	raw := r.URL.Query()["title_id"]
	for _, s := range r.URL.Query()["title_ids"] {
		raw = append(raw, strings.Split(s, ",")...)
	}
	for _, s := range raw {
		n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
		if err != nil || n <= 0 {
			webserver.Fail(w, http.StatusBadRequest, "title_id must be a positive number")
			return
		}
		ids = append(ids, n)
	}
	if len(ids) > 200 {
		webserver.Fail(w, http.StatusBadRequest, "at most 200 title ids")
		return
	}
	me := viewerID(ctx)
	out := map[string]AskedByEntry{}
	if max := h.KidMax(r); max != "" && len(ids) > 0 {
		ok, err := h.AllowedTitleIDs(ctx, max, ids)
		if err != nil {
			webserver.Internal(w, "requests", err)
			return
		}
		kept := ids[:0]
		for _, id := range ids {
			if ok[id] {
				kept = append(kept, id)
			}
		}
		ids = kept
	}
	if len(ids) > 0 {
		rows, err := askedByRows(ctx, h.Store, ids)
		if err != nil {
			webserver.Internal(w, "requests", err)
			return
		}
		for _, x := range rows {
			k := strconv.FormatInt(x.titleID, 10)
			e := out[k]
			e.Askers = append(e.Askers, askedByPerson{ID: x.userID, Name: x.name, AskedAt: x.at, Me: x.userID == me})
			out[k] = e
		}
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"titles": out})
}
