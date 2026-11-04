package api

import (
	"context"
	"net/http"
	"path/filepath"
	"strconv"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/library"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// VersionView is one wanted copy of a title as the API shows it.
type VersionView struct {
	ID              int64    `json:"id"`
	Label           string   `json:"label"`
	Main            bool     `json:"main"`
	PickRulesID     int64    `json:"pick_rules_id"`
	PickRulesName   string   `json:"pick_rules_name"`
	LibraryFolderID int64    `json:"library_folder_id"`
	FolderPath      string   `json:"folder_path"`
	KeepUpToDate    bool     `json:"keep_up_to_date"`
	Edition         string   `json:"edition"`
	AudioLanguages  []string `json:"audio_languages"`
}

func versionView(ctx context.Context, db store.Querier, v domain.Version) VersionView {
	out := VersionView{ID: v.ID, Label: v.Label, Main: v.IsMain, PickRulesID: v.PickRulesID, LibraryFolderID: v.LibraryFolderID,
		KeepUpToDate: v.KeepUpToDate, Edition: v.Edition, AudioLanguages: v.AudioLanguages}
	if out.AudioLanguages == nil {
		out.AudioLanguages = []string{}
	}
	m, _ := ctx.Value(lookupMemoKey{}).(*lookupMemo)
	if path, ok := m.folder(ctx, db, v.LibraryFolderID); ok {
		out.FolderPath = filepath.Join(path, v.FolderName)
	}
	if name, ok := m.rules(ctx, db, v.PickRulesID); ok {
		out.PickRulesName = name
	}
	return out
}

// lookupMemo keeps the Library folder paths and Pick rules names read during
// one request, so listing every Title reads each row once, not per Version.
type lookupMemo struct{ folders, picks map[int64]*string }

type lookupMemoKey struct{}

func withLookupMemo(ctx context.Context) context.Context {
	return context.WithValue(ctx, lookupMemoKey{}, &lookupMemo{folders: map[int64]*string{}, picks: map[int64]*string{}})
}

// memoRead answers id from seen, or reads it once with get and keeps it
// (a nil memo just reads).
func memoRead(seen map[int64]*string, id int64, get func() (string, bool)) (string, bool) {
	if seen != nil {
		if v, ok := seen[id]; ok {
			return deref(v), v != nil
		}
	}
	v, ok := get()
	if seen != nil {
		if ok {
			seen[id] = &v
		} else {
			seen[id] = nil
		}
	}
	return v, ok
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func (m *lookupMemo) folder(ctx context.Context, db store.Querier, id int64) (string, bool) {
	var seen map[int64]*string
	if m != nil {
		seen = m.folders
	}
	return memoRead(seen, id, func() (string, bool) {
		var lf domain.LibraryFolder
		err := store.Get(ctx, db, "library_folders", id, &lf)
		return lf.Path, err == nil
	})
}

func (m *lookupMemo) rules(ctx context.Context, db store.Querier, id int64) (string, bool) {
	var seen map[int64]*string
	if m != nil {
		seen = m.picks
	}
	return memoRead(seen, id, func() (string, bool) {
		var pr domain.PickRules
		err := store.Get(ctx, db, "pick_rules", id, &pr)
		return pr.Name, err == nil
	})
}

func versionID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("vid"), 10, 64)
	if err != nil || id <= 0 {
		fail(w, http.StatusNotFound, "not found")
		return 0, false
	}
	return id, true
}

// publishTitle sends the changed title to live pages.
func (a *API) publishTitle(r *http.Request, id int64) {
	if a.s.Live == nil {
		return
	}
	var t domain.Title
	if store.Get(r.Context(), a.s.Store.Read(), "titles", id, &t) != nil {
		return
	}
	if v, err := a.titleView(r.Context(), t); err == nil {
		a.publishTitleView(v)
	}
}

func (a *API) addVersion(w http.ResponseWriter, r *http.Request) {
	t, ok := a.titleID(w, r)
	if !ok {
		return
	}
	var in library.VersionInput
	if !decode(w, r, &in, `send {"label": "4K"} with optional pick_rules_id, library_folder_id, keep_up_to_date`) {
		return
	}
	v, err := library.AddVersion(r.Context(), a.s.Store, t.ID, in, a.downloadPaths(r), a.s.Library.Clock(), actor(r))
	if err != nil {
		libraryError(w, err)
		return
	}
	a.s.Library.VersionEvent(r.Context(), t, "Added the "+v.Label+" Version", v, actor(r))
	a.publishTitle(r, t.ID)
	writeJSON(w, http.StatusCreated, versionView(r.Context(), a.s.Store.Read(), v))
}

func (a *API) editVersion(w http.ResponseWriter, r *http.Request) {
	t, ok := a.titleID(w, r)
	if !ok {
		return
	}
	vid, ok := versionID(w, r)
	if !ok {
		return
	}
	var c library.VersionChange
	if !decode(w, r, &c, `send any of label, pick_rules_id, keep_up_to_date`) {
		return
	}
	v, err := library.EditVersion(r.Context(), a.s.Store, t.ID, vid, c, actor(r))
	if err != nil {
		libraryError(w, err)
		return
	}
	a.s.Library.VersionEvent(r.Context(), t, "Changed the "+v.Label+" Version", v, actor(r))
	a.publishTitle(r, t.ID)
	writeJSON(w, http.StatusOK, versionView(r.Context(), a.s.Store.Read(), v))
}

func (a *API) deleteVersion(w http.ResponseWriter, r *http.Request) {
	t, ok := a.titleID(w, r)
	if !ok {
		return
	}
	vid, ok := versionID(w, r)
	if !ok {
		return
	}
	var v domain.Version
	_ = store.Get(r.Context(), a.s.Store.Read(), "versions", vid, &v)
	if err := library.RemoveVersion(r.Context(), a.s.Store, t.ID, vid, actor(r)); err != nil {
		libraryError(w, err)
		return
	}
	a.s.Library.VersionEvent(r.Context(), t, "Removed the "+v.Label+" Version", v, actor(r))
	a.publishTitle(r, t.ID)
	w.WriteHeader(http.StatusNoContent)
}
