package main

import (
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// listTitlesBrief writes GET /api/v1/titles?brief=1 (mediafiles.ListTitlesBrief).
func (a *handlers) listTitlesBrief(w http.ResponseWriter, r *http.Request, ts []mediafiles.Title) {
	mediafiles.ListTitlesBrief(w, r, a.s.Store, ts, func(t mediafiles.Title) bool {
		if p, ok := parts.ForTitleKind(t.Kind); ok && !a.partIsOn(p) && !a.s.Parts.SafeMode {
			return false // its part is off: hidden, as in the full list
		}
		return a.titleAllowed(r, t) // above a Kid's age-rating limit is hidden
	})
}

// ownedHandlers serve the owned-title marks (mediafiles); the part switches,
// the Kid limit and the age rating check come from here.
func (a *handlers) ownedHandlers() *mediafiles.OwnedHandlers {
	return &mediafiles.OwnedHandlers{Store: a.s.Store, KidMax: a.kidMax, RatingAllows: metadata.RatingAllows,
		KindOn: func(kind string) bool {
			p, ok := parts.ForTitleKind(kind)
			return !ok || a.partIsOn(p) || a.s.Parts.SafeMode
		}}
}
