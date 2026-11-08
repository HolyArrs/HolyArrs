package api

import (
	"net/http"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"

	"github.com/HolyArrs/holyarrs/app/backend/internal/parts"
)

// titleBrief is one entry of GET /api/v1/titles?brief=1: only what Downloads,
// History and the Calendar read (posters, names, kinds, a movie's dates and
// its slot states), with the same part and Kid filters as the full list. The
// full list builds every title's seasons, artist facts, versions and asks;
// these pages need about a tenth of its bytes.
type titleBrief struct {
	ID           int64       `json:"id"`
	Kind         string      `json:"kind"`
	Name         string      `json:"name"`
	Artwork      []string    `json:"artwork"`
	CinemaDate   *time.Time  `json:"cinema_date,omitempty"`
	DigitalDate  *time.Time  `json:"digital_date,omitempty"`
	PhysicalDate *time.Time  `json:"physical_date,omitempty"`
	Slots        []briefSlot `json:"slots"`
}

type briefSlot struct {
	State string `json:"state"`
}

func (a *API) listTitlesBrief(w http.ResponseWriter, r *http.Request, ts []mediafiles.Title) {
	// Like the full list, only movies carry their slots (all of them, by id).
	slots := map[int64][]briefSlot{}
	rows, err := a.s.Store.Read().QueryContext(r.Context(), `SELECT i.title_id, s.state FROM slots s
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
		slots[id] = append(slots[id], briefSlot{State: st})
	}
	if err := rows.Err(); err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	out := []titleBrief{}
	for _, t := range ts {
		if p, ok := parts.ForTitleKind(t.Kind); ok && !a.partIsOn(p) && !a.s.Parts.SafeMode {
			continue // its part is off: hidden, as in the full list
		}
		if !a.titleAllowed(r, t) {
			continue // above a Kid's age-rating limit
		}
		b := titleBrief{ID: t.ID, Kind: t.Kind, Name: t.Name, Artwork: artworkLinks(t.Artwork),
			CinemaDate: t.CinemaDate, DigitalDate: t.DigitalDate, PhysicalDate: t.PhysicalDate, Slots: slots[t.ID]}
		if b.Slots == nil {
			b.Slots = []briefSlot{}
		}
		out = append(out, b)
	}
	webserver.WriteTaggedJSON(w, r, map[string]any{"titles": out})
}

// ownedHandlers serve the owned-title marks (mediafiles); the part switches,
// the Kid limit and the age rating check come from here.
func (a *API) ownedHandlers() *mediafiles.OwnedHandlers {
	return &mediafiles.OwnedHandlers{Store: a.s.Store, KidMax: a.kidMax, RatingAllows: metadata.RatingAllows,
		KindOn: func(kind string) bool {
			p, ok := parts.ForTitleKind(kind)
			return !ok || a.partIsOn(p) || a.s.Parts.SafeMode
		}}
}
