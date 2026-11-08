package library

import (
	"context"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

func addShowFolder(t *testing.T, s *database.Store) mediafiles.LibraryFolder {
	t.Helper()
	lf, err := AddFolder(context.Background(), s, FolderInput{Path: t.TempDir(), Kind: "show", Label: "Shows"}, nil, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	return lf
}
