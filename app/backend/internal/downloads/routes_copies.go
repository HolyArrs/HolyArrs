package downloads

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

func (h *Handlers) CopyRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/titles/{id}/copies", h.titleCopies)
	m.HandleFunc("POST /api/v1/titles/{id}/versions/{vid}/adopt", h.adoptMovieCopy)
	m.HandleFunc("PUT /api/v1/titles/{id}/files/{fid}/choices", h.setCopyChoice)
	m.HandleFunc("POST /api/v1/titles/{id}/files/{fid}/keep-and-add", h.keepAndAddCopy)
	m.HandleFunc("PUT /api/v1/downloads/{id}/edition", h.confirmDownloadEdition)
}

func fileChoiceID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("fid"), 10, 64)
	if err != nil || id <= 0 {
		webserver.Fail(w, http.StatusBadRequest, "choose a file")
		return 0, false
	}
	return id, true
}

func (h *Handlers) setCopyChoice(w http.ResponseWriter, r *http.Request) {
	t, ok := h.TitleID(w, r)
	if !ok {
		return
	}
	id, ok := fileChoiceID(w, r)
	if !ok {
		return
	}
	var in library.FileChoice
	if !webserver.Decode(w, r, &in, "send protected or edition, with acknowledge_protection to remove protection") {
		return
	}
	f, err := h.Library.SetFileChoice(r.Context(), t.ID, id, in, webserver.ActorOr(r, database.Actor{Kind: "user", ID: 1}))
	if err != nil {
		h.LibraryError(w, err)
		return
	}
	h.PublishTitle(r, t.ID)
	webserver.WriteJSON(w, http.StatusOK, f)
}

func (h *Handlers) keepAndAddCopy(w http.ResponseWriter, r *http.Request) {
	t, ok := h.TitleID(w, r)
	if !ok {
		return
	}
	id, ok := fileChoiceID(w, r)
	if !ok {
		return
	}
	var in library.VersionInput
	if !webserver.Decode(w, r, &in, "choose label, pick_rules_id and library_folder_id for a separate copy") {
		return
	}
	v, err := h.Library.KeepAndAddVersion(r.Context(), t.ID, id, in, webserver.ActorOr(r, database.Actor{Kind: "user", ID: 1}))
	if err != nil {
		h.LibraryError(w, err)
		return
	}
	h.PublishTitle(r, t.ID)
	webserver.WriteJSON(w, http.StatusCreated, h.VersionJSON(r.Context(), v))
}

func (h *Handlers) confirmDownloadEdition(w http.ResponseWriter, r *http.Request) {
	id, ok := copyPathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Edition string `json:"edition"`
		Confirm bool   `json:"confirm"`
		Retry   bool   `json:"retry"`
	}
	if !webserver.Decode(w, r, &in, "send edition and confirm:true after checking this movie download") {
		return
	}
	if !in.Confirm {
		webserver.Fail(w, http.StatusBadRequest, "confirm the edition after checking the download")
		return
	}
	if err := h.Library.ConfirmDownloadEdition(r.Context(), id, in.Edition, webserver.ActorOr(r, database.Actor{Kind: "user", ID: 1})); err != nil {
		h.LibraryError(w, err)
		return
	}
	var jobID int64
	if in.Retry {
		var err error
		jobID, err = h.retryMovieImport(r, id)
		if err != nil {
			h.LibraryError(w, err)
			return
		}
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"download_id": id, "edition": in.Edition, "confirmed": true, "job_id": jobID})
}

func (h *Handlers) titleCopies(w http.ResponseWriter, r *http.Request) {
	t, ok := h.TitleID(w, r)
	if !ok {
		return
	}
	files, err := database.Select[mediafiles.File](r.Context(), h.Store.Read(), "files", `version_id IN (SELECT id FROM versions WHERE title_id = ?) AND role = 'main' AND state IN ('present','missing') ORDER BY version_id,id`, t.ID)
	if err != nil {
		webserver.Internal(w, "downloads", err)
		return
	}
	if files == nil {
		files = []mediafiles.File{}
	}
	versions, err := h.titleVersions(r.Context(), t.ID)
	if err != nil {
		webserver.Internal(w, "downloads", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"files": files, "versions": versions})
}

func (h *Handlers) titleVersions(ctx context.Context, titleID int64) ([]any, error) {
	vs, err := database.Select[mediafiles.Version](ctx, h.Store.Read(), "versions", "title_id = ? ORDER BY is_main DESC,id", titleID)
	out := []any{}
	for _, v := range vs {
		out = append(out, h.VersionJSON(ctx, v))
	}
	return out, err
}

func (h *Handlers) adoptMovieCopy(w http.ResponseWriter, r *http.Request) {
	t, ok := h.TitleID(w, r)
	if !ok {
		return
	}
	vid, ok := copyPathID(w, r, "vid")
	if !ok {
		return
	}
	var in library.CopyAdoption
	if !webserver.Decode(w, r, &in, "choose an existing relative_path in this Version's folder, and confirmed_edition only after checking it") {
		return
	}
	f, err := h.Library.AdoptMovieCopy(r.Context(), t.ID, vid, in, webserver.ActorOr(r, database.Actor{Kind: "user", ID: 1}))
	if err != nil {
		h.LibraryError(w, err)
		return
	}
	h.PublishTitle(r, t.ID)
	webserver.WriteJSON(w, http.StatusCreated, f)
}

// retryMovieImport changes only a still-waiting Slot of this finished download.
// The importer rechecks every constraint and protection before disk work.
func (h *Handlers) retryMovieImport(r *http.Request, downloadID int64) (int64, error) {
	if h.Queue == nil {
		return 0, fmt.Errorf("%w: imports are not running", library.ErrCopyChoice)
	}
	unlock := h.Store.LockFiles()
	defer unlock()
	var jobID int64
	err := h.Store.Write(r.Context(), func(tx *sql.Tx) error {
		var d domain.Download
		if err := database.Get(r.Context(), tx, "downloads", downloadID, &d); err != nil {
			return err
		}
		if len(d.SlotIDs) != 1 || d.State != "finished" && d.State != "seeding" {
			return library.ErrCopyChoice
		}
		var sl mediafiles.Slot
		if err := database.Get(r.Context(), tx, "slots", d.SlotIDs[0], &sl); err != nil {
			return err
		}
		var kind, state string
		if err := tx.QueryRow(`SELECT t.kind,t.state FROM titles t JOIN items i ON i.title_id=t.id WHERE i.id=?`, sl.ItemID).Scan(&kind, &state); err != nil {
			return err
		}
		if kind != "movie" || state == "removed" || sl.State != mediafiles.SlotNeedsADecision || sl.ActiveDownloadID == nil || *sl.ActiveDownloadID != d.ID {
			return fmt.Errorf("%w: this download no longer waits for a decision", library.ErrCopyChoice)
		}
		if err := mediafiles.MoveSlotTx(tx, sl.ID, mediafiles.SlotImporting, "retrying after an explicit edition choice", webserver.ActorOr(r, database.Actor{Kind: "user", ID: 1})); err != nil {
			return err
		}
		key := "import:" + strconv.FormatInt(downloadID, 10)
		err := tx.QueryRow("SELECT id FROM jobs WHERE dedupe_key = ? AND state IN ('queued','running')", key).Scan(&jobID)
		if err == nil {
			return nil
		}
		if err != sql.ErrNoRows {
			return err
		}
		j := domain.Job{Kind: importer.KindImport, Subject: &domain.Subject{Type: "download", ID: downloadID}, Params: map[string]any{}, Priority: "normal", State: "queued", MaxAttempts: 3, RunAfter: time.Now().UTC(), AskedBy: webserver.ActorOr(r, database.Actor{Kind: "user", ID: 1}), DedupeKey: &key}
		jobID, err = database.Insert(tx, "jobs", &j, webserver.ActorOr(r, database.Actor{Kind: "user", ID: 1}))
		return err
	})
	if err != nil {
		return 0, err
	}
	return jobID, nil
}

// copyPathID reads a numeric path value, answering 404 itself.
func copyPathID(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil || id <= 0 {
		webserver.Fail(w, http.StatusNotFound, "not found")
		return 0, false
	}
	return id, true
}
