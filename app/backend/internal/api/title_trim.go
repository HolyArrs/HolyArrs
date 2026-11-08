package api

import (
	"context"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
)

// trimForViewer leaves out what only people who manage the Library need:
// the folder on the server, the Pick rules, why a Slot was turned down,
// and each file's path, release name and fingerprint. Quality, size and
// languages stay. With no person on the request (sign-in off, as in tests)
// nothing is left out.
func trimForViewer(ctx context.Context, v TitleView) TitleView {
	if seesLibraryDetail(ctx) {
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
		s.File = trimFile(s.File)
	}
	return v
}

// seesLibraryDetail reports whether the person may see the details
// trimForViewer leaves out; true with no person on the request.
func seesLibraryDetail(ctx context.Context) bool {
	who, ok := webserver.From(ctx)
	return !ok || who.Has(webserver.PermManageLibrary)
}

// trimFile copies f without its path, release name, group, fingerprint and
// who placed it.
func trimFile(f *domain.File) *domain.File {
	if f == nil {
		return nil
	}
	c := *f
	c.RelativePath, c.ReleaseName, c.Group, c.Fingerprint, c.PlacedBy = "", nil, nil, nil, ""
	return &c
}

// trimEpisodeSlots does the same for the Slots on episode and track lists.
func trimEpisodeSlots(ctx context.Context, slots map[int64][]EpisodeSlot) {
	if seesLibraryDetail(ctx) {
		return
	}
	for _, sl := range slots {
		for i := range sl {
			sl[i].Reason = ""
			sl[i].File = trimFile(sl[i].File)
		}
	}
}
