package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

func (a *API) askedByRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/asked-by", a.getAskedBy)
}

// viewer is the signed-in person's id (1, the owner, without sign-in).
func viewer(ctx context.Context) int64 {
	if who, ok := webserver.From(ctx); ok {
		return who.UserID
	}
	return 1
}

// askedByEntry is one title's askers.
type askedByEntry struct {
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
func (a *API) getAskedBy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var ids []int64
	raw := r.URL.Query()["title_id"]
	for _, s := range r.URL.Query()["title_ids"] {
		raw = append(raw, strings.Split(s, ",")...)
	}
	for _, s := range raw {
		n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
		if err != nil || n <= 0 {
			fail(w, http.StatusBadRequest, "title_id must be a positive number")
			return
		}
		ids = append(ids, n)
	}
	if len(ids) > 200 {
		fail(w, http.StatusBadRequest, "at most 200 title ids")
		return
	}
	me := viewer(ctx)
	out := map[string]askedByEntry{}
	if max := a.kidMax(r); max != "" && len(ids) > 0 {
		ok, err := a.allowedTitleIDs(ctx, max, ids)
		if err != nil {
			internal(w, err)
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
		args := make([]any, len(ids))
		for i, id := range ids {
			args[i] = id
		}
		rows, err := a.s.Store.Read().QueryContext(ctx, `SELECT a.title_id, p.user_id, u.name, min(p.asked_at) FROM asks a
			JOIN ask_people p ON p.ask_id = a.id JOIN users u ON u.id = p.user_id
			WHERE a.title_id IN (?`+strings.Repeat(", ?", len(ids)-1)+`) GROUP BY a.title_id, p.user_id ORDER BY min(p.asked_at), p.user_id`, args...)
		if err != nil {
			internal(w, err)
			return
		}
		defer rows.Close()
		for rows.Next() {
			var tid, uid int64
			var name, at string
			if err := rows.Scan(&tid, &uid, &name, &at); err != nil {
				internal(w, err)
				return
			}
			// Older rows were written in RFC 3339 rather than the store's layout.
			t, err := time.Parse(database.TimeLayout, at)
			if err != nil {
				t, _ = time.Parse(time.RFC3339Nano, at)
			}
			k := strconv.FormatInt(tid, 10)
			e := out[k]
			e.Askers = append(e.Askers, askedByPerson{ID: uid, Name: name, AskedAt: t, Me: uid == me})
			out[k] = e
		}
		if err := rows.Err(); err != nil {
			internal(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"titles": out})
}
