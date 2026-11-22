package arrapi

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// keptFolderShape reads the path an app sent with an add: one folder
// directly in the root folder it names, which Radarr and Sonarr then use
// as it is. Kometa sends it for titles already on disk. "" means no path
// was sent. The root is matched as folderByPath matches it.
func keptFolderShape(root, p string) (string, *failure) {
	p = strings.TrimRight(strings.TrimSpace(p), `/\`)
	if p == "" {
		return "", nil
	}
	bad := &failure{PropertyName: "Path", ErrorMessage: "the path must be one folder directly in the root folder", AttemptedValue: p}
	root = strings.TrimRight(strings.TrimSpace(root), `/\`)
	if root == "" || len(p) <= len(root)+1 || !strings.EqualFold(p[:len(root)], root) || (p[len(root)] != '/' && p[len(root)] != '\\') {
		return "", bad
	}
	rel := p[len(root)+1:]
	// Hidden folders (HolyArrs' own holding folder among them) and the
	// folders a NAS keeps for itself are never a title's folder.
	if !filepath.IsLocal(rel) || strings.ContainsAny(rel, `/\`) || strings.HasPrefix(rel, ".") || library.NasSystemDir(rel) ||
		strings.IndexFunc(rel, unicode.IsControl) >= 0 {
		return "", bad
	}
	return rel, nil
}

// keptMu lets one add that keeps its own folder check and claim it at a
// time, so two adds sent at once can never both take one folder.
var keptMu sync.Mutex

// lockKept holds keptMu for an add that sends its own folder; it returns
// the unlock (a no-op for an add without one).
func lockKept(p string) func() {
	if strings.TrimSpace(p) == "" {
		return func() {}
	}
	keptMu.Lock()
	return keptMu.Unlock
}

// keptFolder is keptFolderShape for an add into the Library folder with
// this id: the folder must not belong to another title, as adopting a
// folder by hand requires.
func (h *handler) keptFolder(ctx context.Context, folderID int64, root, p string) (string, mediafiles.LibraryFolder, *failure, error) {
	var lf mediafiles.LibraryFolder
	rel, f := keptFolderShape(root, p)
	if f != nil || rel == "" {
		return rel, lf, f, nil
	}
	if err := database.Get(ctx, h.Store.Read(), "library_folders", folderID, &lf); err != nil {
		return "", lf, nil, err
	}
	// A folder already there must be a real folder inside the Library
	// folder: a link to somewhere else would let an add adopt files there.
	dir := filepath.Join(lf.Path, rel)
	if fi, err := os.Lstat(dir); err == nil && (!fi.IsDir() || !importer.InsideRealFile(lf.Path, dir)) {
		return "", lf, &failure{PropertyName: "Path", ErrorMessage: "the path must be a folder inside the root folder, not a link or a file", AttemptedValue: p}, nil
	}
	owner, err := library.FolderTaken(ctx, h.Store.Read(), lf, rel)
	if err != nil {
		return "", lf, nil, err
	}
	if owner != 0 {
		return "", lf, &failure{PropertyName: "Path", ErrorMessage: "another title in the Library already uses this folder", AttemptedValue: p}, nil
	}
	return rel, lf, nil, nil
}

// keepFlag is Radarr's monitored: true or false, or from version 4 on the
// words movieOnly and movieAndCollection (kept up to date) and none, which
// Kometa sends for its monitor choice. Known is false for any other word,
// so the add or edit is refused with a plain reason, not read as a guess.
type keepFlag struct {
	On    bool
	Known bool
	Text  string
}

func (k *keepFlag) UnmarshalJSON(b []byte) error {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	switch x := v.(type) {
	case bool:
		k.On, k.Known = x, true
	case string:
		k.Text = x
		switch strings.ToLower(strings.TrimSpace(x)) {
		case "true", "movieonly", "movieandcollection":
			k.On, k.Known = true, true
		case "false", "none":
			k.On, k.Known = false, true
		}
	}
	return nil
}

// badKeep is the refusal for a monitored value keepFlag does not know.
func badKeep(k *keepFlag) *failure {
	if k == nil || k.Known {
		return nil
	}
	return &failure{PropertyName: "Monitored", ErrorMessage: "monitored must be true, false, movieOnly, movieAndCollection or none", AttemptedValue: k.Text}
}
