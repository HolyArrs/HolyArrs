package importer

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// MaxImportFolders caps how many extra import folders can be saved.
const MaxImportFolders = 32

const importFoldersGroup, importFoldersKey = "where_files_go", "import_folders"

// ErrImportFolder is returned when an extra import folder is not a usable full path.
var ErrImportFolder = fmt.Errorf("each extra import folder is a full path, such as /data/imports or D:\\Imports (not / or a whole drive, and not inside or around the config folder), with no . or .. parts (%d at most)", MaxImportFolders)

// CleanImportFolders validates and normalizes a list of extra import folders.
func CleanImportFolders(in []string) ([]string, error) {
	out := []string{}
	seen := map[string]bool{}
	for _, p := range in {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if !absAnyOS(p) || strings.ContainsRune(p, 0) || strings.HasPrefix(p, `\\`) || strings.HasPrefix(p, "//") {
			return nil, ErrImportFolder
		}
		parts := strings.FieldsFunc(p, func(r rune) bool { return r == '/' || r == '\\' })
		for _, part := range parts {
			if part == ".." || part == "." {
				return nil, ErrImportFolder
			}
		}
		if len(parts) == 0 || (p[0] != '/' && len(parts) == 1) {
			return nil, ErrImportFolder
		}
		p = strings.TrimRight(p, `/\`)
		key := p
		if p[0] != '/' {
			key = strings.ToLower(strings.ReplaceAll(p, "/", `\`))
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, p)
	}
	if len(out) > MaxImportFolders {
		return nil, ErrImportFolder
	}
	return out, nil
}

func absAnyOS(p string) bool {
	if strings.HasPrefix(p, "/") {
		return true
	}
	if len(p) < 3 {
		return false
	}
	c := p[0] | 0x20
	return c >= 'a' && c <= 'z' && p[1] == ':' && (p[2] == '\\' || p[2] == '/')
}

// ReadImportFolders loads the saved extra import folders.
func ReadImportFolders(ctx context.Context, db database.Lister) ([]string, error) {
	rows, err := db.QueryContext(ctx, "SELECT value FROM settings WHERE grp = ? AND key = ?", importFoldersGroup, importFoldersKey)
	if err != nil {
		return []string{}, err
	}
	defer rows.Close()
	out := []string{}
	if rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return out, err
		}
		var list []string
		if json.Unmarshal([]byte(raw), &list) == nil {
			if c, err := CleanImportFolders(list); err == nil {
				out = c
			}
		}
	}
	return out, rows.Err()
}

// SaveImportFolders validates and stores the extra import folders.
func SaveImportFolders(ctx context.Context, s *database.Store, in []string, by domain.Actor) ([]string, error) {
	list, err := CleanImportFolders(in)
	if err != nil {
		return nil, err
	}
	for _, p := range list {
		if filepath.IsAbs(p) && UnsafeFolder(p, s.Dir()) {
			return nil, ErrImportFolder
		}
	}
	raw, _ := json.Marshal(list)
	err = s.Write(ctx, func(tx *sql.Tx) error {
		now := database.FormatTime(time.Now())
		_, err := tx.Exec(`INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (grp, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at, updated_by = excluded.updated_by`,
			importFoldersGroup, importFoldersKey, string(raw), now, actorText(by), now, actorText(by))
		return err
	})
	return list, err
}

// ByHandPlaces holds the folders a by-hand import may read from.
type ByHandPlaces struct {
	Library     []string `json:"library_folders"`
	Downloaders []string `json:"downloader_folders"`
	Extra       []string `json:"extra_folders"`
	// Downloads are the own folders (or one loose file) of finished downloads
	// HolyArrs sent: each may be read, never the save folder around it.
	Downloads []string `json:"-"`
}

func (ps ByHandPlaces) all() []string {
	var out []string
	seen := map[string]bool{}
	for _, l := range [][]string{ps.Library, ps.Downloaders, ps.Extra} {
		for _, p := range l {
			if p != "" && !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
		}
	}
	return out
}

func (ps ByHandPlaces) within(p string) string {
	for _, root := range append(ps.all(), ps.Downloads...) {
		if !filepath.IsAbs(root) || strings.HasPrefix(filepath.VolumeName(root), `\\`) {
			continue
		}
		roots := []string{filepath.Clean(root)}
		if real, err := filepath.EvalSymlinks(roots[0]); err == nil {
			roots = append(roots, real)
		}
		for _, r := range roots {
			if samePath(r, p) || under(r, p) {
				return r
			}
		}
	}
	return ""
}

// Holds reports whether p sits inside one of the allowed places.
func (ps ByHandPlaces) Holds(p string) bool {
	root := ps.within(p)
	if root == "" {
		return false
	}
	if _, err := os.Lstat(p); err != nil {
		return true
	}
	bad := fs.ModeIrregular
	if real, err := filepath.EvalSymlinks(p); err != nil {
		bad |= fs.ModeSymlink
	} else if ps.within(real) == "" {
		return false
	}
	for q := p; !samePath(q, root); q = filepath.Dir(q) {
		fi, err := os.Lstat(q)
		if err != nil || fi.Mode()&bad != 0 || filepath.Dir(q) == q {
			return false
		}
	}
	return true
}

// PlaceError is a refusal that carries a message meant for the user.
type PlaceError struct{ msg string }

// Error returns the message.
func (e *PlaceError) Error() string { return e.msg }

// Refusal returns an error that names the allowed places, or nil when none apply.
func (ps ByHandPlaces) Refusal() error {
	all := ps.all()
	if len(all) == 0 {
		return &PlaceError{"Import by hand reads only from the Library folders, the Downloaders' folders and the extra import folders, and none is set up yet. Add one in Settings > Library folders."}
	}
	return &PlaceError{"Import by hand reads only from the Library folders, the Downloaders' folders and the extra import folders in Settings > Library folders: " + strings.Join(all, ", ")}
}

func samePath(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func (im *Importer) byHandPlaces(ctx context.Context, db database.Lister, folders []mediafiles.LibraryFolder) (ByHandPlaces, error) {
	ps := ByHandPlaces{Library: []string{}, Downloaders: []string{}, Extra: []string{}}
	for _, lf := range folders {
		if !wholeDisk(lf.Path) {
			ps.Library = append(ps.Library, lf.Path)
		}
	}
	dls, err := database.Select[domain.Downloader](ctx, db, "downloaders", "removed = 0")
	if err != nil {
		return ps, err
	}
	seen := map[string]bool{}
	add := func(p string) {
		if p = strings.TrimSpace(p); p != "" && !seen[p] && !wholeDisk(p) {
			seen[p] = true
			ps.Downloaders = append(ps.Downloaders, p)
		}
	}
	add(im.SavePath)
	byID := map[int64]domain.Downloader{}
	for _, d := range dls {
		byID[d.ID] = d
		for _, m := range d.PathMap {
			add(m.Ours)
		}
	}
	// Import files by hand on a download HolyArrs sent opens its own folder,
	// which may sit in a save folder HolyArrs knows only from the Downloader.
	done, err := database.Select[domain.Download](ctx, db, "downloads", "state IN ('finished', 'seeding') AND release_id IS NOT NULL")
	if err != nil {
		return ps, err
	}
	for _, d := range done {
		if dr, ok := byID[d.DownloaderID]; ok {
			if p := im.downloadContent(d, dr); p != "" {
				ps.Downloads = append(ps.Downloads, p)
			}
		}
	}
	ps.Extra, err = ReadImportFolders(ctx, db)
	return ps, err
}

// ByHandPlaces returns the places a by-hand import may read from.
func (im *Importer) ByHandPlaces(ctx context.Context) (ByHandPlaces, error) {
	db := im.Store.Read()
	folders, err := database.Select[mediafiles.LibraryFolder](ctx, db, "library_folders", "1 = 1")
	if err != nil {
		return ByHandPlaces{}, err
	}
	return im.byHandPlaces(ctx, db, folders)
}

func wholeDisk(p string) bool {
	p = strings.TrimSpace(p)
	if p == "" {
		return true
	}
	c := path.Clean(strings.ReplaceAll(p, `\`, "/"))
	if c == "/" || c == "." || (len(c) == 2 && c[1] == ':') || (len(c) == 3 && c[1] == ':' && c[2] == '/') {
		return true
	}
	isRoot := func(r string) bool {
		r = filepath.Clean(r)
		return strings.Trim(r[len(filepath.VolumeName(r)):], `/\`) == ""
	}
	if r, err := filepath.EvalSymlinks(p); err == nil && isRoot(r) {
		return true
	}
	if r, err := os.Readlink(p); err == nil && isRoot(strings.TrimPrefix(r, `\??\`)) {
		return true
	}
	return false
}
