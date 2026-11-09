package main

import (
	"context"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/discover"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/music"
	"github.com/HolyArrs/holyarrs/app/backend/internal/requests"
	engine "github.com/HolyArrs/holyarrs/app/backend/internal/suggest"
)

// TrailerHooks reaches the API parts the trailer tests in cmd/holyarrs
// check (Explore pools, card languages, music videos).
type TrailerHooks struct{ a *handlers }

// TrailerTest returns the trailer test hooks.
func (a *handlers) TrailerTest() TrailerHooks { return TrailerHooks{a} }

func (h TrailerHooks) Explore() *discover.Explore { return &h.a.explore }
func (h TrailerHooks) PutServiceTop(region string, v metadata.ServiceTop) {
	h.a.serviceTop.Put(region, v)
}
func (h TrailerHooks) PoolPage(p engine.Pool, member int64, sw, region string, m engine.Moment) engine.Page {
	return h.a.disc.PoolPage(p, member, sw, region, m)
}
func (h TrailerHooks) PassLanguages(ctx context.Context) []string {
	return h.a.trailerPassLanguages(ctx)
}
func (h TrailerHooks) NoteCardLang(lang string)               { h.a.explore.Cards.NoteLang(lang) }
func (h TrailerHooks) Person(r *http.Request) requests.Person { return h.a.person(r) }
func (h TrailerHooks) FillTrailerKeys(r *http.Request, items []discover.ItemView) {
	h.a.fillTrailerKeys(r, items)
}
func (h TrailerHooks) Music() *music.Handlers { return h.a.mh() }
