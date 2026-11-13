package importer

import "github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

// OtherNames are the names a Title's files may carry besides its own: its
// alternative titles and its original title.
func OtherNames(ti mediafiles.Title) []string {
	out := append([]string{}, ti.Aliases...)
	if ti.OriginalName != nil && *ti.OriginalName != "" && *ti.OriginalName != ti.Name {
		out = append(out, *ti.OriginalName)
	}
	return out
}
