package arrapi

import (
	"context"
	"encoding/json"
	"log"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// cutNote is the words an earlier version wrote for each format its Pick
// rules could not hold, when they held 64 traits.
const cutNote = "Pick rules hold at most 64 traits"

// RestoreCut gives the profiles an earlier version cut to 64 traits the
// formats they left out, up to the limit now. It reads each profile as it
// was sent, adds only the traits the Pick rules lack and leaves the points
// already there as they are. Profiles that were not cut are not touched,
// so a later start does nothing.
func (h *Handlers) RestoreCut(ctx context.Context) {
	rows, err := listProfileRows(ctx, h.Store.Read())
	if err != nil {
		log.Printf("old-app API: reading pushed profiles: %v", err)
		return
	}
	for _, row := range rows {
		if !strings.Contains(strings.Join(row.Notes, "\n"), cutNote) {
			continue
		}
		if err := h.restoreRow(ctx, row); err != nil {
			log.Printf("old-app API: restoring profile %q: %v", row.Name, err)
		}
	}
}

func (h *Handlers) restoreRow(ctx context.Context, row pushedProfile) error {
	var in wireProfile
	if err := json.Unmarshal([]byte(row.Body), &in); err != nil {
		return err
	}
	var rules mediafiles.PickRules
	if err := database.Get(ctx, h.Store.Read(), "pick_rules", row.PickRulesID, &rules); err != nil {
		return err
	}
	hd := &handler{Deps: h.deps(), side: row.Side}
	want, notes, err := hd.pickRulesOf(ctx, in)
	if err != nil {
		return err
	}
	have := map[string]bool{}
	for _, tp := range rules.Body.Traits {
		have[tp.Trait] = true
	}
	for _, tp := range want.Traits {
		if !have[tp.Trait] && len(rules.Body.Traits) < mediafiles.MaxTraits {
			rules.Body.Traits = append(rules.Body.Traits, tp)
		}
	}
	if _, err := library.UpdatePickRules(ctx, h.Store, row.PickRulesID, library.PickRulesInput{Name: rules.Name, Kinds: rules.Kinds, Body: rules.Body}, domain.System); err != nil {
		return err
	}
	row.Notes = notes
	return saveProfileRow(ctx, h.Store, row, domain.System)
}
