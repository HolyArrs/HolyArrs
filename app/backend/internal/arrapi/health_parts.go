package arrapi

import (
	"context"
	"encoding/json"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/requests"
	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
)

// setupParts is what Home's setup gaps are read from: whether setup is over
// (something in the Library or asked for), whether Sources are turned off as
// a part, and how many Sources and Downloaders exist, are on, and already
// fail by their state. Removed ones are not counted.
type setupParts struct {
	Done, SourcesOff                   bool
	Sources, SourcesOn, SourcesFailing int
	Dls, DlsOn, DlsFailing             int
}

func readSetupParts(ctx context.Context, st *database.Store) (setupParts, error) {
	var p setupParts
	q := st.Read()
	kept, err := mediafiles.AnyKept(ctx, q)
	if err != nil {
		return p, err
	}
	asked, err := requests.AnyAsk(ctx, q)
	if err != nil {
		return p, err
	}
	p.Done = kept || asked
	if p.Sources, p.SourcesOn, p.SourcesFailing, err = sources.PartCounts(ctx, q); err != nil {
		return p, err
	}
	if p.Dls, p.DlsOn, p.DlsFailing, err = downloads.PartCounts(ctx, q); err != nil {
		return p, err
	}
	var parts []string
	_ = json.Unmarshal([]byte(database.SettingOr(ctx, q, "this_app", "parts_off", "[]")), &parts)
	for _, x := range parts {
		p.SourcesOff = p.SourcesOff || x == "sources"
	}
	return p, nil
}
