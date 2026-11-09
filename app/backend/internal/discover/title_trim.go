package discover

import (
	"context"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// TrimForViewer leaves out what only people who manage the Library need:
// the folder on the server, the Pick rules, why a Slot was turned down,
// and each file's path, release name and fingerprint. Quality, size and
// languages stay. With no person on the request (sign-in off, as in tests)
// nothing is left out.
func TrimForViewer(ctx context.Context, v TitleView) TitleView {
	if SeesLibraryDetail(ctx) {
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
		s.File = TrimFile(s.File)
	}
	return v
}

// SeesLibraryDetail reports whether the person may see the details
// TrimForViewer leaves out; true with no person on the request.
func SeesLibraryDetail(ctx context.Context) bool {
	who, ok := webserver.From(ctx)
	return !ok || who.Has(webserver.PermManageLibrary)
}

// TrimFile copies f without its path, release name, group, fingerprint and
// who placed it.
func TrimFile(f *mediafiles.File) *mediafiles.File {
	if f == nil {
		return nil
	}
	c := *f
	c.RelativePath, c.ReleaseName, c.Group, c.Fingerprint, c.PlacedBy = "", nil, nil, nil, ""
	return &c
}
