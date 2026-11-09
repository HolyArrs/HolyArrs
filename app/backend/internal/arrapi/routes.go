package arrapi

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// The versions each side reports. Seerr and Bazarr refuse a major version
// below 4, and Seerr then skips the language profiles. Prowlarr wants at
// least 4.0.4 on the Radarr side.
const (
	SonarrVersion = "4.0.99.0"
	RadarrVersion = "5.0.99.0"
)

// AppName is the app name in the status answer. No client reads it.
const AppName = "HolyArrs"

// bodyMax caps a request body. A show sent back with every season and
// picture stays far below it.
const bodyMax = 1 << 20

// Deps holds what the adapter reads and calls.
type Deps struct {
	Store    *database.Store
	Library  *library.Service
	Metadata metadata.Provider
	Queue    *jobs.Queue
	// DeleteFiles reports whether a delete from an app may take the files
	// too. It is a setting and off by default. Nil never takes files.
	DeleteFiles func(context.Context) bool
	// Picture sends one picture for the covers route. src is a stored
	// poster ("poster:<cache name>") or a wide picture's address, which is
	// fetched once into the artwork cache. When px > 0 the poster is at
	// most px pixels a side. The adapter never fetches anything itself, so
	// nil answers 404.
	Picture func(w http.ResponseWriter, r *http.Request, src string, px int)
	// Secrets keeps the keys of the Sources Prowlarr writes. Nil refuses
	// any key.
	Secrets *secrets.Vault
	// SourceSaved and SourceRemoved tell the rest of the app when Prowlarr
	// writes or removes a Source. Nil does nothing.
	SourceSaved   func(domain.Source)
	SourceRemoved func(id int64)
	// Now is the clock. Nil means time.Now.
	Now func() time.Time
}

type handler struct {
	Deps
	side    string // sonarr or radarr
	version string
}

func (h *handler) now() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now()
}

// Sonarr serves the Sonarr side. The outer mux hands it every request
// under /sonarr/.
func Sonarr(d Deps) http.Handler {
	h := &handler{Deps: d, side: "sonarr", version: SonarrVersion}
	mux := http.NewServeMux()
	h.common(mux)
	mux.HandleFunc("GET /languageprofile", h.languageProfiles)
	mux.HandleFunc("GET /series", h.listSeries)
	mux.HandleFunc("GET /series/lookup", h.lookupSeries)
	mux.HandleFunc("GET /series/{id}", h.getSeries)
	mux.HandleFunc("POST /series", h.addSeries)
	mux.HandleFunc("PUT /series", h.putSeries)
	mux.HandleFunc("PUT /series/{id}", h.putSeries)
	mux.HandleFunc("DELETE /series/{id}", h.deleteTitle)
	mux.HandleFunc("GET /episode", h.listEpisodes)
	mux.HandleFunc("PUT /episode/monitor", h.monitorEpisodes)
	mux.HandleFunc("GET /wanted/missing", h.missingEpisodes)
	mux.HandleFunc("GET /calendar", h.calendarEpisodes)
	mux.HandleFunc("GET /queue/details", h.queueDetails)
	return h.serve(mux)
}

// Radarr serves the Radarr side, under /radarr/.
func Radarr(d Deps) http.Handler {
	h := &handler{Deps: d, side: "radarr", version: RadarrVersion}
	mux := http.NewServeMux()
	h.common(mux)
	mux.HandleFunc("GET /movie", h.listMovies)
	mux.HandleFunc("GET /movie/lookup", h.lookupMovies)
	mux.HandleFunc("GET /movie/{id}", h.getMovie)
	mux.HandleFunc("POST /movie", h.addMovie)
	mux.HandleFunc("PUT /movie", h.putMovie)
	mux.HandleFunc("PUT /movie/{id}", h.putMovie)
	mux.HandleFunc("DELETE /movie/{id}", h.deleteTitle)
	return h.serve(mux)
}

// common adds the routes both sides answer.
func (h *handler) common(mux *http.ServeMux) {
	mux.HandleFunc("GET /system/status", h.status)
	mux.HandleFunc("GET /qualityprofile", h.qualityProfiles)
	mux.HandleFunc("GET /rootfolder", h.rootFolders)
	mux.HandleFunc("GET /tag", h.listTags)
	mux.HandleFunc("POST /tag", h.addTag)
	mux.HandleFunc("PUT /tag", h.putTag)
	mux.HandleFunc("PUT /tag/{id}", h.putTag)
	mux.HandleFunc("GET /queue", h.queue)
	mux.HandleFunc("POST /command", h.command)
	mux.HandleFunc("GET /command/{id}", h.getCommand)
	mux.HandleFunc("GET /mediacover/{id}/{file}", h.cover)
	mux.HandleFunc("GET /indexer", h.listIndexers)
	mux.HandleFunc("GET /indexer/schema", h.indexerSchema)
	mux.HandleFunc("GET /indexer/{id}", h.getIndexer)
	mux.HandleFunc("POST /indexer", h.addIndexer)
	mux.HandleFunc("PUT /indexer/{id}", h.putIndexer)
	mux.HandleFunc("DELETE /indexer/{id}", h.deleteIndexer)
	mux.HandleFunc("POST /indexer/test", h.testIndexer)
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) { notFound(w) })
}

// serve strips the side and /api/v3 from the path and matches the rest in
// lower case, because the old apps match routes in any case and Seerr
// calls /qualityProfile. Covers are also answered right after the side
// (/sonarr/MediaCover/...), since Library answers point there and Bazarr
// fetches them. Every answer carries the version header.
func (h *handler) serve(mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Application-Version", h.version)
		rest := strings.ToLower(strings.TrimPrefix(r.URL.Path, "/"+h.side))
		inner, ok := strings.CutPrefix(rest, "/api/v3/")
		if !ok && strings.HasPrefix(rest, "/mediacover/") {
			inner, ok = rest[1:], true
		}
		if !ok {
			// The old apps answer anything outside their API with a bare
			// 404. Bazarr's first probe (/api/system/status) then falls
			// back to /api/v3. A JSON body would be read as a version.
			w.WriteHeader(http.StatusNotFound)
			return
		}
		// Bazarr lists with "series/", which means "series".
		if len(inner) > 1 {
			inner = strings.TrimSuffix(inner, "/")
		}
		r2 := new(http.Request)
		*r2 = *r
		u := *r.URL
		u.Path, u.RawPath = "/"+inner, ""
		r2.URL = &u
		mux.ServeHTTP(w, r2)
	})
}

// base is this side's address prefix: the URL base, then /sonarr or
// /radarr. Seerr keeps it as the base path from the status answer.
// Location headers leave the URL base out, because the URL base layer
// adds it to every Location on the way out.
func (h *handler) base(ctx context.Context) string { return webserver.URLBaseFrom(ctx) + "/" + h.side }

func actor(r *http.Request) domain.Actor { return webserver.Actor(r.Context()) }

// utc is a time written the old apps' way: UTC, whole seconds, a Z.
type utc time.Time

// MarshalJSON writes the time in UTC with whole seconds and a Z.
func (t utc) MarshalJSON() ([]byte, error) {
	return []byte(`"` + time.Time(t).UTC().Format("2006-01-02T15:04:05Z") + `"`), nil
}

func utcPtr(t *time.Time) *utc {
	if t == nil {
		return nil
	}
	u := utc(*t)
	return &u
}

// zeroDate is the "never" date the old apps send for a title not added.
var zeroDate = utc(time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC))

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// message answers with an error message object.
func message(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"message": msg})
}

func notFound(w http.ResponseWriter) { message(w, http.StatusNotFound, "not found") }

// failure is one failed check of a body. A 400 answer lists them all.
type failure struct {
	PropertyName   string `json:"propertyName"`
	ErrorMessage   string `json:"errorMessage"`
	AttemptedValue any    `json:"attemptedValue"`
	Severity       string `json:"severity"`
	ErrorCode      string `json:"errorCode,omitempty"`
}

func refuse(w http.ResponseWriter, fs ...failure) {
	for i := range fs {
		fs[i].Severity = "error"
	}
	writeJSON(w, http.StatusBadRequest, fs)
}

// decode reads a JSON body into v. Key case does not matter. It returns
// false after sending a 400.
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, bodyMax)).Decode(v); err != nil {
		message(w, http.StatusBadRequest, "the body is not valid JSON for this route")
		return false
	}
	return true
}

// query reads a query value in any case of its name, so seriesId and
// seriesid are the same.
func query(r *http.Request, name string) string {
	for k, vs := range r.URL.Query() {
		if strings.EqualFold(k, name) && len(vs) > 0 {
			return vs[0]
		}
	}
	return ""
}

func queryBool(r *http.Request, name string) bool {
	b, _ := strconv.ParseBool(query(r, name))
	return b
}

// pathID reads {id}. It returns false after sending a 404.
func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		notFound(w)
		return 0, false
	}
	return id, true
}

// title reads a Title of kind. A removed title, one of another kind or a
// missing id gives database.ErrNotFound, so every route answers 404.
// Titles are only marked removed and their ids are never reused, which is
// what lets an app keep the id it was given.
func (h *handler) title(ctx context.Context, id int64, kind string) (mediafiles.Title, error) {
	var t mediafiles.Title
	if err := database.Get(ctx, h.Store.Read(), "titles", id, &t); err != nil {
		return t, err
	}
	if t.Kind != kind || t.State == "removed" {
		return t, database.ErrNotFound
	}
	return t, nil
}

// internalError answers an unexpected failure. The detail goes only to
// the log, so file paths and database text never reach the caller.
func internalError(w http.ResponseWriter, err error) {
	if errors.Is(err, database.ErrNotFound) {
		notFound(w)
		return
	}
	log.Printf("arrapi: %v", err)
	message(w, http.StatusInternalServerError, "something went wrong")
}

func atoi64(s string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return n
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func ptr[T any](v T) *T { return &v }
