package arrapi

import (
	"database/sql"
	"errors"
	"net/http"
	"os"
	"runtime"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/startup"
)

// status answers the server test. Seerr reads urlBase and uses it as the
// base path from then on. On the Sonarr side it also reads the version.
func (h *handler) status(w http.ResponseWriter, r *http.Request) {
	docker := startup.InContainer(func(p string) bool { _, err := os.Stat(p); return err == nil }, os.Getenv)
	writeJSON(w, http.StatusOK, map[string]any{
		"appName": AppName, "instanceName": AppName, "version": h.version, "urlBase": h.base(r.Context()),
		"isDebug": false, "isProduction": true, "isAdmin": false, "isUserInteractive": false,
		"osName": runtime.GOOS, "isLinux": runtime.GOOS == "linux", "isOsx": runtime.GOOS == "darwin", "isWindows": runtime.GOOS == "windows",
		"isDocker": docker, "mode": "console", "branch": "main", "databaseType": "sqLite", "runtimeName": "go",
	})
}

// qualityProfiles lists the Pick rules for movies and shows as quality
// profiles. Music rules are left out. Callers read only id and name.
func (h *handler) qualityProfiles(w http.ResponseWriter, r *http.Request) {
	ps, err := database.Select[mediafiles.PickRules](r.Context(), h.Store.Read(), "pick_rules", "1=1 ORDER BY id")
	if err != nil {
		internalError(w, err)
		return
	}
	out := []map[string]any{}
	for _, p := range ps {
		if mediafiles.IsMusicRules(p.Body) {
			continue
		}
		out = append(out, map[string]any{"id": p.ID, "name": p.Name})
	}
	writeJSON(w, http.StatusOK, out)
}

// folderKind returns the Library folder kind of this side.
func (h *handler) folderKind() string {
	if h.side == "sonarr" {
		return "show"
	}
	return "movie"
}

// rootFolders lists this side's Library folders.
func (h *handler) rootFolders(w http.ResponseWriter, r *http.Request) {
	fs, err := database.Select[mediafiles.LibraryFolder](r.Context(), h.Store.Read(), "library_folders", "kind = ? ORDER BY id", h.folderKind())
	if err != nil {
		internalError(w, err)
		return
	}
	out := []map[string]any{}
	for _, f := range fs {
		v := map[string]any{"id": f.ID, "path": f.Path, "accessible": f.Health == "ok", "unmappedFolders": []any{}}
		if f.FreeSpace >= 0 {
			v["freeSpace"] = f.FreeSpace
		}
		out = append(out, v)
	}
	writeJSON(w, http.StatusOK, out)
}

// languageProfiles answers the one fixed entry older clients need. Seerr
// skips the call for version 4 and up. Overseerr always makes it.
func (h *handler) languageProfiles(w http.ResponseWriter, _ *http.Request) {
	english := map[string]any{"id": 1, "name": "English"}
	writeJSON(w, http.StatusOK, []map[string]any{{"id": 1, "name": "English", "upgradeAllowed": false, "cutoff": english,
		"languages": []map[string]any{{"language": english, "allowed": true}}}})
}

type tag struct {
	ID    int64  `json:"id"`
	Label string `json:"label"`
}

func (h *handler) listTags(w http.ResponseWriter, r *http.Request) {
	out, err := listTags(r.Context(), h.Store.Read())
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// addTag makes a tag. A label already in use hands back that tag, so a
// repeated add is safe.
func (h *handler) addTag(w http.ResponseWriter, r *http.Request) {
	var in tag
	if !decode(w, r, &in) {
		return
	}
	in.Label = strings.TrimSpace(in.Label)
	if in.Label == "" || len(in.Label) > 200 {
		refuse(w, failure{PropertyName: "Label", ErrorMessage: "give the tag a label of 1 to 200 characters", AttemptedValue: in.Label})
		return
	}
	err := h.Store.Write(r.Context(), func(tx *sql.Tx) error {
		found, err := tagByLabel(tx, in.Label)
		if err == nil {
			in = found
			return nil
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		in.ID, err = insertTag(tx, in.Label, h.now())
		return err
	})
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, in)
}

// putTag renames a tag. The id comes from the path or the body, since
// clients send it either way.
func (h *handler) putTag(w http.ResponseWriter, r *http.Request) {
	var in tag
	if !decode(w, r, &in) {
		return
	}
	if v := r.PathValue("id"); v != "" {
		in.ID = atoi64(v)
	}
	in.Label = strings.TrimSpace(in.Label)
	if in.Label == "" || len(in.Label) > 200 {
		refuse(w, failure{PropertyName: "Label", ErrorMessage: "give the tag a label of 1 to 200 characters", AttemptedValue: in.Label})
		return
	}
	var n int64
	err := h.Store.Write(r.Context(), func(tx *sql.Tx) error {
		var err error
		n, err = renameTag(tx, in.ID, in.Label)
		return err
	})
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			refuse(w, failure{PropertyName: "Label", ErrorMessage: "another tag has that label", AttemptedValue: in.Label})
			return
		}
		internalError(w, err)
		return
	}
	if n == 0 {
		notFound(w)
		return
	}
	writeJSON(w, http.StatusAccepted, in)
}
