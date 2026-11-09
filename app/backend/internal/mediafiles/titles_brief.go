package mediafiles

import (
	"net/http"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// TitleBrief is one entry of GET /api/v1/titles?brief=1: only what Downloads,
// History and the Calendar read (posters, names, kinds, a movie's dates and
// its slot states), with the same part and Kid filters as the full list. The
// full list builds every title's seasons, artist facts, versions and asks;
// these pages need about a tenth of its bytes.
type TitleBrief struct {
	ID           int64       `json:"id"`
	Kind         string      `json:"kind"`
	Name         string      `json:"name"`
	Artwork      []string    `json:"artwork"`
	CinemaDate   *time.Time  `json:"cinema_date,omitempty"`
	DigitalDate  *time.Time  `json:"digital_date,omitempty"`
	PhysicalDate *time.Time  `json:"physical_date,omitempty"`
	Slots        []BriefSlot `json:"slots"`
}

// BriefSlot is one movie slot's state in a TitleBrief.
type BriefSlot struct {
	State string `json:"state"`
}

// ListTitlesBrief writes the brief titles list of ts; keep says whether a
// title is shown (its part is on and it fits a Kid's limit).
func ListTitlesBrief(w http.ResponseWriter, r *http.Request, store *database.Store, ts []Title, keep func(Title) bool) {
	// Like the full list, only movies carry their slots (all of them, by id).
	slots := map[int64][]BriefSlot{}
	rows, err := store.Read().QueryContext(r.Context(), `SELECT i.title_id, s.state FROM slots s
		JOIN items i ON i.id = s.item_id JOIN titles t ON t.id = i.title_id WHERE t.kind = 'movie' ORDER BY s.id`)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var st string
		if err := rows.Scan(&id, &st); err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		slots[id] = append(slots[id], BriefSlot{State: st})
	}
	if err := rows.Err(); err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	out := []TitleBrief{}
	for _, t := range ts {
		if !keep(t) {
			continue
		}
		b := TitleBrief{ID: t.ID, Kind: t.Kind, Name: t.Name, Artwork: artworkLinks(t.Artwork),
			CinemaDate: t.CinemaDate, DigitalDate: t.DigitalDate, PhysicalDate: t.PhysicalDate, Slots: slots[t.ID]}
		if b.Slots == nil {
			b.Slots = []BriefSlot{}
		}
		out = append(out, b)
	}
	webserver.WriteTaggedJSON(w, r, map[string]any{"titles": out})
}
