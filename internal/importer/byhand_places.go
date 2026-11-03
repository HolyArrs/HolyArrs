package importer

// The places a path given by hand may be in: the Library folders, the
// folders the Downloaders save into (as HolyArrs sees them) and the extra
// import folders the owner adds in Settings. Anything else is refused.

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

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// MaxImportFolders is the most extra import folders one may add.
const MaxImportFolders = 32

const importFoldersGroup, importFoldersKey = "where_files_go", "import_folders"

// ErrImportFolder says an extra import folder is not a usable path.
var ErrImportFolder = fmt.Errorf("each extra import folder is a full path, such as /data/imports or D:\\Imports (not / or a whole drive), with no . or .. parts (%d at most)", MaxImportFolders)

// CleanImportFolders checks a list of extra import folders: trimmed, full
// paths in Linux or Windows form, no . or .. parts, not / or a bare drive,
// no network share paths, at most MaxImportFolders. Blank lines and
// repeats are dropped.
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
		// The whole disk would undo the limit: "/" and "C:\" are refused.
		if len(parts) == 0 || (p[0] != '/' && len(parts) == 1) {
			return nil, ErrImportFolder
		}
		p = strings.TrimRight(p, `/\`)
		key := p
		if p[0] != '/' {
			key = strings.ToLower(strings.ReplaceAll(p, "/", `\`)) // drive paths ignore case
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

// absAnyOS says p is a full path on Linux (/x) or Windows (C:\x, C:/x).
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

// ReadImportFolders reads the extra import folders; a broken value is none.
func ReadImportFolders(ctx context.Context, db store.Lister) ([]string, error) {
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

// SaveImportFolders stores a checked list of extra import folders and
// returns it as stored.
func SaveImportFolders(ctx context.Context, s *store.Store, in []string, by domain.Actor) ([]string, error) {
	list, err := CleanImportFolders(in)
	if err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(list)
	err = s.Write(ctx, func(tx *sql.Tx) error {
		now := store.FormatTime(time.Now())
		_, err := tx.Exec(`INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (grp, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at, updated_by = excluded.updated_by`,
			importFoldersGroup, importFoldersKey, string(raw), now, actorText(by), now, actorText(by))
		return err
	})
	return list, err
}

// ByHandPlaces are the folders a path given by hand may be inside.
type ByHandPlaces struct {
	Library     []string `json:"library_folders"`
	Downloaders []string `json:"downloader_folders"`
	Extra       []string `json:"extra_folders"`
}

// all lists every place once.
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

// within returns the place (as written or with its links followed) that p
// is or is inside, comparing p as written; "" when there is none.
func (ps ByHandPlaces) within(p string) string {
	for _, root := range ps.all() {
		if !filepath.IsAbs(root) || strings.HasPrefix(filepath.VolumeName(root), `\\`) {
			continue // a path for another OS, or a share: never stat it
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

// Holds says p (a cleaned absolute path) is one of the places or inside
// one, as written and, when it is there, once its links are followed, so a
// link inside a place cannot lead out of it.
func (ps ByHandPlaces) Holds(p string) bool {
	root := ps.within(p)
	if root == "" {
		return false
	}
	if _, err := os.Lstat(p); err != nil {
		return true // not there: nothing to follow, and nothing to read
	}
	// A Windows junction is not followed (EvalSymlinks keeps it or fails
	// below it), so a step below the place that is one is refused; when the
	// links cannot be followed at all, any link step is refused too.
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

// PlaceError is the plain answer for a path outside every place.
type PlaceError struct{ msg string }

func (e *PlaceError) Error() string { return e.msg }

// Refusal names the places a path given by hand may be in.
func (ps ByHandPlaces) Refusal() error {
	all := ps.all()
	if len(all) == 0 {
		return &PlaceError{"import by hand reads only from the Library folders, the Downloaders' folders and the extra import folders, and none is set up yet; add one in Settings > Library folders"}
	}
	return &PlaceError{"import by hand reads only from the Library folders, the Downloaders' folders and the extra import folders in Settings > Library folders: " + strings.Join(all, ", ")}
}

// samePath compares two cleaned paths, ignoring case on Windows.
func samePath(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// byHandPlaces reads the places: Library folders, the save path and the
// HolyArrs side of every path map of each Downloader still set up, and
// the extras.
func (im *Importer) byHandPlaces(ctx context.Context, db store.Lister, folders []domain.LibraryFolder) (ByHandPlaces, error) {
	ps := ByHandPlaces{Library: []string{}, Downloaders: []string{}, Extra: []string{}}
	for _, lf := range folders {
		if !wholeDisk(lf.Path) {
			ps.Library = append(ps.Library, lf.Path)
		}
	}
	dls, err := store.Select[domain.Downloader](ctx, db, "downloaders", "removed = 0")
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
	for _, d := range dls {
		for _, m := range d.PathMap {
			add(m.Ours)
		}
	}
	ps.Extra, err = ReadImportFolders(ctx, db)
	return ps, err
}

// ByHandPlaces reads the places a path given by hand may be in.
func (im *Importer) ByHandPlaces(ctx context.Context) (ByHandPlaces, error) {
	db := im.Store.Read()
	folders, err := store.Select[domain.LibraryFolder](ctx, db, "library_folders", "1 = 1")
	if err != nil {
		return ByHandPlaces{}, err
	}
	return im.byHandPlaces(ctx, db, folders)
}

// wholeDisk says p is a disk's root ("/", "C:\"). A Library folder or a
// Downloader's folder set to one never counts as a place to import from by
// hand, or it would open the whole disk again.
func wholeDisk(p string) bool {
	p = strings.TrimSpace(p)
	if p == "" {
		return true
	}
	// "/data/.." and "C:\x\.." are a root too, on any system.
	c := path.Clean(strings.ReplaceAll(p, `\`, "/"))
	if c == "/" || c == "." || (len(c) == 2 && c[1] == ':') || (len(c) == 3 && c[1] == ':' && c[2] == '/') {
		return true
	}
	// So is a link that leads to one (Go cannot always follow a Windows
	// junction, so its target is read as well).
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
