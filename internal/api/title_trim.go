package api

import (
	"context"

	"github.com/CoderLuii/holyarrs/internal/auth"
)

// trimForViewer leaves out what only people who manage the Library need:
// the folder on the server, the Pick rules, why a Slot was turned down,
// and each file's path, release name and fingerprint. Quality, size and
// languages stay. With no person on the request (sign-in off, as in tests)
// nothing is left out.
func trimForViewer(ctx context.Context, v TitleView) TitleView {
	if who, ok := auth.From(ctx); !ok || who.Has(auth.PermManageLibrary) {
		return v
	}
	v.LibraryFolderID, v.PickRulesID, v.FolderPath, v.PickRulesName = 0, 0, "", ""
	for i := range v.Versions {
		x := &v.Versions[i]
		x.LibraryFolderID, x.PickRulesID, x.FolderPath, x.PickRulesName = 0, 0, "", ""
	}
	for i := range v.Slots {
		s := &v.Slots[i]
		s.Reason = ""
		if s.File != nil {
			f := *s.File
			f.RelativePath, f.ReleaseName, f.Group, f.Fingerprint, f.PlacedBy = "", nil, nil, nil, ""
			s.File = &f
		}
	}
	return v
}
