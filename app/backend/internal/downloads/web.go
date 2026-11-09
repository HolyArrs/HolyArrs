package downloads

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
)

// Web serves the Downloads routes: the Downloads screen, Needs a decision,
// Downloaders, the per-item controls and the outside-torrent matches. api
// builds one and injects what lives in later modules.
type Web struct {
	Store *database.Store
	// DLSync is the background download reader; nil keeps the view's own
	// client reads.
	DLSync *Sync
	// Downloads follows Downloaders; made on first use when nil.
	Downloads *Manager
	Live      *webserver.LiveHub
	Queue     *jobs.Queue
	Secrets   *secrets.Vault
	// Auth nil serves without sign-in (tests only).
	Auth     *accounts.Auth
	Library  *library.Service
	Metadata metadata.Provider
	Search   *sources.Searcher

	// Given by the program.
	// ShowSource is the show metadata source by name.
	ShowSource func(name string) (metadata.ShowProvider, error)
	// AddMovieTitle and AddShowTitle add a matched title the normal way,
	// without searching and without keeping it up to date.
	AddMovieTitle func(ctx context.Context, id string) (int64, error)
	AddShowTitle  func(ctx context.Context, id string) (int64, error)
	// LibraryError writes a Library add error the way the title routes do.
	LibraryError func(w http.ResponseWriter, err error)

	// Match links outside torrents to Library titles (tests swap its hooks).
	Match ClientMatch
	// ViewDB, when set, takes the Downloads view's reads (tests count them).
	ViewDB *sql.DB

	tlog     torrentLog
	dlHealth dlHealthSent
}

// read is the read pool the Downloads view uses.
func (a *Web) read() *sql.DB {
	if a.ViewDB != nil {
		return a.ViewDB
	}
	return a.Store.Read()
}

func (a *Web) searcher() *sources.Searcher { return a.Search }

// actor is the signed-in person; without sign-in (tests) it is the owner.
func actor(r *http.Request) domain.Actor {
	if _, ok := webserver.From(r.Context()); ok {
		return webserver.Actor(r.Context())
	}
	return domain.Actor{Kind: "user", ID: 1}
}

// whoName names the person for an Event summary.
func whoName(r *http.Request) string {
	if who, ok := webserver.From(r.Context()); ok && who.Name != "" {
		return who.Name
	}
	return "The owner"
}

// actorJSONText is the actor as the JSON text stored in updated_by columns.
func actorJSONText(a domain.Actor) string {
	b, _ := json.Marshal(a)
	return string(b)
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		webserver.Fail(w, http.StatusNotFound, "not found")
		return 0, false
	}
	return id, true
}

func limit(r *http.Request, def int) int {
	n, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || n <= 0 || n > 500 {
		return def
	}
	return n
}
