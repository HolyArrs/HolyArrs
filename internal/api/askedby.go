package api

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs/internal/asks"
	"github.com/CoderLuii/holyarrs/internal/auth"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// Values of the household "Show who asked" setting.
const (
	ShowWhoAskedApprovers = "approvers" // owner, admins and managers only (default)
	ShowWhoAskedEveryone  = "everyone"  // everyone in the household, for askers who opt in
)

func (a *API) askedByRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/asked-by", a.getAskedBy)
	m.HandleFunc("GET /api/v1/my/ask-privacy", a.getAskPrivacy)
	m.HandleFunc("PUT /api/v1/my/ask-privacy", a.putAskPrivacy)
}

// askerSeer decides, on the server, which askers a viewer may see.
type askerSeer struct {
	me       int64
	all      bool           // owner, admins and managers (approve right), or no sign-in
	kid      bool           // a Kid sees only themself
	everyone bool           // household setting is "everyone"
	shown    map[int64]bool // askers who turned on "Show my name on my asks"
}

func (a *API) seer(ctx context.Context) (askerSeer, error) {
	who, ok := auth.From(ctx)
	if !ok {
		return askerSeer{me: 1, all: true}, nil
	}
	s := askerSeer{me: who.UserID, all: who.Has(auth.PermApprove)}
	if s.all {
		return s, nil
	}
	s.kid = who.Role == auth.RoleMember && who.MaxRating != ""
	if s.kid {
		return s, nil
	}
	set, err := asks.ReadSettings(ctx, a.s.Store.Read())
	if err != nil {
		return s, err
	}
	s.everyone = set.ShowWhoAsked == ShowWhoAskedEveryone
	if !s.everyone {
		return s, nil
	}
	s.shown = map[int64]bool{}
	rows, err := a.s.Store.Read().QueryContext(ctx, "SELECT user_id FROM user_ask_settings WHERE show_my_name = 1")
	if err != nil {
		return s, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return s, err
		}
		s.shown[id] = true
	}
	return s, rows.Err()
}

func (s askerSeer) sees(id int64) bool {
	return s.all || id == s.me || (s.everyone && !s.kid && s.shown[id])
}

// trim drops the askers the viewer may not see from an ask view.
func (s askerSeer) trim(v *asks.View) {
	if s.all {
		return
	}
	kept := []asks.Asker{}
	for _, p := range v.People {
		if s.sees(p.ID) {
			kept = append(kept, p)
		}
	}
	v.People = kept
	if !s.sees(v.AskedBy.ID) {
		if len(kept) > 0 {
			v.AskedBy = kept[0].Who
		} else {
			v.AskedBy = asks.Who{}
		}
	}
}

// askedByEntry is one title's askers as the viewer may see them.
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
// title's askers, earliest first, trimmed by the privacy rules.
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
	s, err := a.seer(ctx)
	if err != nil {
		internal(w, err)
		return
	}
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
			if !s.sees(uid) {
				continue
			}
			// Older rows were written in RFC 3339 rather than the store's layout.
			t, err := time.Parse(store.TimeLayout, at)
			if err != nil {
				t, _ = time.Parse(time.RFC3339Nano, at)
			}
			k := strconv.FormatInt(tid, 10)
			e := out[k]
			e.Askers = append(e.Askers, askedByPerson{ID: uid, Name: name, AskedAt: t, Me: uid == s.me})
			out[k] = e
		}
		if err := rows.Err(); err != nil {
			internal(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"titles": out, "sees_all": s.all})
}

type askPrivacy struct {
	ShowMyName bool `json:"show_my_name"`
}

func (a *API) getAskPrivacy(w http.ResponseWriter, r *http.Request) {
	var p askPrivacy
	err := a.s.Store.Read().QueryRowContext(r.Context(), "SELECT show_my_name = 1 FROM user_ask_settings WHERE user_id = ?", a.person(r).UserID).Scan(&p.ShowMyName)
	if err != nil && err != sql.ErrNoRows {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (a *API) putAskPrivacy(w http.ResponseWriter, r *http.Request) {
	var p askPrivacy
	if !decode(w, r, &p, `send {"show_my_name": true|false}`) {
		return
	}
	me := a.person(r).UserID
	err := a.s.Store.Write(r.Context(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO user_ask_settings (user_id, show_my_name) VALUES (?, ?)
			ON CONFLICT (user_id) DO UPDATE SET show_my_name = excluded.show_my_name`, me, p.ShowMyName)
		return err
	})
	if err != nil {
		internal(w, err)
		return
	}
	a.getAskPrivacy(w, r)
}
