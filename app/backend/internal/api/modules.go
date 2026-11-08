package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
)

// ModuleRoutes registers the routes of the modules that have left this
// package. They need services this package builds, so it hands them over.
func (a *API) ModuleRoutes(rt *webserver.Router) {
	a.sourceHandlers().Routes(rt)
}

// sourceHandlers is the Source, search and bridge routes; one value serves
// both the routes and the callers here that publish a changed Source.
func (a *API) sourceHandlers() *sources.Handlers {
	a.sourceOnce.Do(func() {
		h := &sources.Handlers{Store: a.s.Store, Vault: a.s.Secrets, Search: a.searcher, Bridges: a.s.Bridges, Queue: a.s.Queue,
			Catalog: libraryCatalog{a}, OldApps: oldApps{a}, PartOn: a.partIsOn}
		if a.s.Live != nil {
			h.Live = a.s.Live
		}
		a.sourceH = h
	})
	return a.sourceH
}

func (a *API) searcher() *sources.Searcher { return a.s.Search }

// publishSource tells open screens that a Source changed.
func (a *API) publishSource(x domain.Source) { a.sourceHandlers().PublishSource(x) }

// libraryCatalog answers the sources routes' questions about Titles, Slots
// and Parts.
type libraryCatalog struct{ a *API }

func (c libraryCatalog) TitleFromPath(w http.ResponseWriter, r *http.Request) (domain.Title, bool) {
	return c.a.titleID(w, r)
}

func (c libraryCatalog) EpisodeLabels(ctx context.Context, slotIDs []int64) string {
	return c.a.episodeLabels(ctx, slotIDs)
}

func (c libraryCatalog) AlbumLabels(ctx context.Context, slotIDs []int64) string {
	return c.a.albumLabels(ctx, slotIDs)
}

func (c libraryCatalog) PartLabel(ctx context.Context, partID int64) (int64, string, string, string) {
	return c.a.partLabel(ctx, partID)
}

// oldApps is the old-app key and switch side of connecting Prowlarr.
type oldApps struct{ a *API }

func (o oldApps) CanMakeKeys() bool { return o.a.s.Auth != nil }

func (o oldApps) KeySides(ctx context.Context) (map[string]bool, error) {
	out := map[string]bool{}
	if o.a.s.Auth == nil {
		return out, nil
	}
	ks, err := o.a.s.Auth.CompatKeys(ctx)
	if err != nil {
		return nil, err
	}
	for _, k := range ks {
		out[k.Side] = true
	}
	return out, nil
}

func (o oldApps) MakeKey(r *http.Request, side string) (string, error) {
	return o.a.s.Auth.MakeCompatKey(r, side)
}

func (o oldApps) DeleteKey(r *http.Request, side string) error {
	_, err := o.a.s.Auth.DeleteCompatKey(r, side)
	return err
}

func (o oldApps) On(ctx context.Context) bool { return o.a.oldAppOn(ctx) }

func (o oldApps) SetOn(ctx context.Context, by domain.Actor, on bool) error {
	return o.a.saveOldAppSwitches(ctx, by, &on, nil)
}

// actorJSONText is the actor as the JSON text stored in updated_by columns.
func actorJSONText(a domain.Actor) string {
	b, _ := json.Marshal(a)
	return string(b)
}
