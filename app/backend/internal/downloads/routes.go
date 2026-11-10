package downloads

import (
	"context"
	"database/sql"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// Deps is what the Downloads routes need: services and core types, and funcs
// for anything a higher module owns.
type Deps struct {
	Store *database.Store
	// KidMax is the signed-in person's age-rating limit ("" for none).
	KidMax  func(*http.Request) string
	Library *library.Service
	Queue   *jobs.Queue
	// TitleID reads the {id} title, answering itself when it fails.
	TitleID func(w http.ResponseWriter, r *http.Request) (mediafiles.Title, bool)
	// PublishTitle sends the changed title to live pages.
	PublishTitle func(r *http.Request, id int64)
	// VersionJSON is a Version as the API shows it.
	VersionJSON func(ctx context.Context, v mediafiles.Version) any
	// LibraryError answers a Library refusal the way the title routes do.
	LibraryError func(w http.ResponseWriter, err error)
	// ReadDB is the database reads use; nil uses Store.Read(). Tests set it.
	ReadDB *sql.DB
}

// Handlers serves the Downloads routes. Its state fields are private and
// zero-value ready, so &Handlers{Deps: d} works.
type Handlers struct {
	Deps
}

// Routes registers the Pick rules and own traits routes.
func (h *Handlers) Routes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/pick-rules", h.listPickRules)
	m.HandleFunc("POST /api/v1/pick-rules", h.addPickRules)
	m.HandleFunc("GET /api/v1/pick-rules/{id}", h.getPickRules)
	m.HandleFunc("PUT /api/v1/pick-rules/{id}", h.editPickRules)
	m.HandleFunc("DELETE /api/v1/pick-rules/{id}", h.deletePickRules)
	m.HandleFunc("GET /api/v1/own-traits", h.listOwnTraits)
	m.HandleFunc("POST /api/v1/own-traits", h.addOwnTrait)
	m.HandleFunc("PUT /api/v1/own-traits/{id}", h.editOwnTrait)
	m.HandleFunc("DELETE /api/v1/own-traits/{id}", h.deleteOwnTrait)
	m.HandleFunc("GET /api/v1/trash/profiles", h.listTrashProfiles)
	m.HandleFunc("POST /api/v1/pick-rules/{id}/trash", h.applyTrash)
}
