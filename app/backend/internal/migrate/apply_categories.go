package migrate

import (
	"database/sql"
	"slices"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// categoryWords says which categories a new Downloader downloads into.
func categoryWords(d *DownloaderPlan, main string) string {
	var parts []string
	for _, k := range [][2]string{{"shows", d.CategoryShows}, {"movies", d.CategoryMovies}, {"music", d.CategoryMusic}} {
		if k[1] != "" {
			parts = append(parts, k[0]+" in "+k[1])
		}
	}
	if len(parts) == 0 {
		return "it downloads into the " + main + " category"
	}
	return "it downloads " + strings.Join(parts, ", ") + ", as the old apps did; HolyArrs makes a missing category on its first check"
}

// addKindCategories fills the kind categories this import brings on a
// Downloader an earlier import made, where that kind has none yet. One a
// person set up here is left as it is. It says what it set, or "".
func (r *run) addKindCategories(d *DownloaderPlan, name string) string {
	return r.addKindCategoriesTo(d, importedDownloader(r.ctx, r.m.Store.Read(), name))
}

// addKindCategoriesTo is addKindCategories for the imported Downloader id. A
// 1.2.0 import left the main category at "holyarrs", which the old apps never
// had; it moves to the old app's own one.
func (r *run) addKindCategoriesTo(d *DownloaderPlan, id int64) string {
	if d.CategoryShows == "" && d.CategoryMovies == "" && d.CategoryMusic == "" {
		return ""
	}
	var have domain.Downloader
	if id == 0 || database.Get(r.ctx, r.m.Store.Read(), "downloaders", id, &have) != nil || have.Removed {
		return ""
	}
	var set []string
	for _, f := range []struct {
		word      string
		cur, from *string
	}{{"shows", &have.CategoryShows, &d.CategoryShows}, {"movies", &have.CategoryMovies, &d.CategoryMovies}, {"music", &have.CategoryMusic, &d.CategoryMusic}} {
		if *f.cur == "" && *f.from != "" {
			*f.cur = *f.from
			set = append(set, f.word+" in "+*f.from)
			if !slices.ContainsFunc(have.CategoriesImported, func(x string) bool { return strings.EqualFold(x, *f.from) }) {
				have.CategoriesImported = append(have.CategoriesImported, *f.from)
			}
		}
	}
	if main, _ := importedCategories(d); strings.EqualFold(have.Category, "holyarrs") && main != "holyarrs" {
		have.Category = main
		set = append([]string{"everything else in " + main}, set...)
		if !slices.ContainsFunc(have.CategoriesImported, func(x string) bool { return strings.EqualFold(x, main) }) {
			have.CategoriesImported = append(have.CategoriesImported, main)
		}
	}
	if len(set) == 0 {
		return ""
	}
	if r.dry {
		return "it will download " + strings.Join(set, ", ")
	}
	err := r.m.Store.Write(r.ctx, func(tx *sql.Tx) error {
		return database.Update(tx, "downloaders", &have, r.by)
	})
	if err != nil {
		return ""
	}
	return "it now downloads " + strings.Join(set, ", ")
}
