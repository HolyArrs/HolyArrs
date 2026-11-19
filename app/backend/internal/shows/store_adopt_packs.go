package shows

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"strings"
)

func tvFolderAvailable(ctx context.Context, tx *sql.Tx, libraryID int64, folder string, fold bool) error {
	names, err := mediafiles.VersionFolderNames(ctx, tx, libraryID)
	if err != nil {
		return err
	}
	key := library.FolderKey(folder, fold)
	for _, existing := range names {
		other := library.FolderKey(existing, fold)
		if key == other || strings.HasPrefix(key, other+"/") || strings.HasPrefix(other, key+"/") {
			return fmt.Errorf("this folder overlaps a show already in the Library")
		}
	}
	return nil
}
