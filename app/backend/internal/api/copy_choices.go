package api

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

func (a *API) copyRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/titles/{id}/copies", a.titleCopies)
	m.HandleFunc("POST /api/v1/titles/{id}/versions/{vid}/adopt", a.adoptMovieCopy)
	m.HandleFunc("PUT /api/v1/titles/{id}/files/{fid}/choices", a.setCopyChoice)
	m.HandleFunc("POST /api/v1/titles/{id}/files/{fid}/keep-and-add", a.keepAndAddCopy)
	m.HandleFunc("PUT /api/v1/downloads/{id}/edition", a.confirmDownloadEdition)
}

func fileChoiceID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("fid"), 10, 64)
	if err != nil || id <= 0 {
		webserver.Fail(w, http.StatusBadRequest, "choose a file")
		return 0, false
	}
	return id, true
}

func (a *API) setCopyChoice(w http.ResponseWriter, r *http.Request) {
	t, ok := a.titleID(w, r)
	if !ok {
		return
	}
	id, ok := fileChoiceID(w, r)
	if !ok {
		return
	}
	var in library.FileChoice
	if !decode(w, r, &in, "send protected or edition, with acknowledge_protection to remove protection") {
		return
	}
	f, err := a.s.Library.SetFileChoice(r.Context(), t.ID, id, in, actor(r))
	if err != nil {
		libraryError(w, err)
		return
	}
	a.publishTitle(r, t.ID)
	webserver.WriteJSON(w, http.StatusOK, f)
}

func (a *API) keepAndAddCopy(w http.ResponseWriter, r *http.Request) {
	t, ok := a.titleID(w, r)
	if !ok {
		return
	}
	id, ok := fileChoiceID(w, r)
	if !ok {
		return
	}
	var in library.VersionInput
	if !decode(w, r, &in, "choose label, pick_rules_id and library_folder_id for a separate copy") {
		return
	}
	v, err := a.s.Library.KeepAndAddVersion(r.Context(), t.ID, id, in, actor(r))
	if err != nil {
		libraryError(w, err)
		return
	}
	a.publishTitle(r, t.ID)
	webserver.WriteJSON(w, http.StatusCreated, versionView(r.Context(), a.s.Store.Read(), v))
}

func (a *API) confirmDownloadEdition(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		Edition string `json:"edition"`
		Confirm bool   `json:"confirm"`
		Retry   bool   `json:"retry"`
	}
	if !decode(w, r, &in, "send edition and confirm:true after checking this movie download") {
		return
	}
	if !in.Confirm {
		webserver.Fail(w, http.StatusBadRequest, "confirm the edition after checking the download")
		return
	}
	if err := a.s.Library.ConfirmDownloadEdition(r.Context(), id, in.Edition, actor(r)); err != nil {
		libraryError(w, err)
		return
	}
	var jobID int64
	if in.Retry {
		var err error
		jobID, err = a.retryMovieImport(r, id)
		if err != nil {
			libraryError(w, err)
			return
		}
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"download_id": id, "edition": in.Edition, "confirmed": true, "job_id": jobID})
}

func (a *API) titleCopies(w http.ResponseWriter, r *http.Request) {
	t, ok := a.titleID(w, r)
	if !ok {
		return
	}
	files, err := database.Select[mediafiles.File](r.Context(), a.s.Store.Read(), "files", `version_id IN (SELECT id FROM versions WHERE title_id = ?) AND role = 'main' AND state IN ('present','missing') ORDER BY version_id,id`, t.ID)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	if files == nil {
		files = []mediafiles.File{}
	}
	versions, err := a.titleVersions(r.Context(), t.ID)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"files": files, "versions": versions})
}

func (a *API) titleVersions(ctx context.Context, titleID int64) ([]VersionView, error) {
	vs, err := database.Select[mediafiles.Version](ctx, a.s.Store.Read(), "versions", "title_id = ? ORDER BY is_main DESC,id", titleID)
	out := []VersionView{}
	for _, v := range vs {
		out = append(out, versionView(ctx, a.s.Store.Read(), v))
	}
	return out, err
}

func (a *API) adoptMovieCopy(w http.ResponseWriter, r *http.Request) {
	t, ok := a.titleID(w, r)
	if !ok {
		return
	}
	vid, ok := versionID(w, r)
	if !ok {
		return
	}
	var in library.CopyAdoption
	if !decode(w, r, &in, "choose an existing relative_path in this Version's folder, and confirmed_edition only after checking it") {
		return
	}
	f, err := a.s.Library.AdoptMovieCopy(r.Context(), t.ID, vid, in, actor(r))
	if err != nil {
		libraryError(w, err)
		return
	}
	a.publishTitle(r, t.ID)
	webserver.WriteJSON(w, http.StatusCreated, f)
}

// retryMovieImport changes only a still-waiting Slot of this finished download.
// The importer rechecks every constraint and protection before disk work.
func (a *API) retryMovieImport(r *http.Request, downloadID int64) (int64, error) {
	if a.s.Queue == nil {
		return 0, fmt.Errorf("%w: imports are not running", library.ErrCopyChoice)
	}
	unlock := a.s.Store.LockFiles()
	defer unlock()
	var jobID int64
	err := a.s.Store.Write(r.Context(), func(tx *sql.Tx) error {
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
		if err := mediafiles.MoveSlotTx(tx, sl.ID, mediafiles.SlotImporting, "retrying after an explicit edition choice", actor(r)); err != nil {
			return err
		}
		key := "import:" + itoa(downloadID)
		err := tx.QueryRow("SELECT id FROM jobs WHERE dedupe_key = ? AND state IN ('queued','running')", key).Scan(&jobID)
		if err == nil {
			return nil
		}
		if err != sql.ErrNoRows {
			return err
		}
		j := domain.Job{Kind: importer.KindImport, Subject: &domain.Subject{Type: "download", ID: downloadID}, Params: map[string]any{}, Priority: "normal", State: "queued", MaxAttempts: 3, RunAfter: time.Now().UTC(), AskedBy: actor(r), DedupeKey: &key}
		jobID, err = database.Insert(tx, "jobs", &j, actor(r))
		return err
	})
	if err != nil {
		return 0, err
	}
	return jobID, nil
}
