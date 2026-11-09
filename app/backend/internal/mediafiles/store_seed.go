package mediafiles

import (
	"database/sql"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// EnsureFolder adds a Library folder at path once; an existing one is left alone.
func EnsureFolder(tx *sql.Tx, path, kind, label string) error {
	var n int
	if err := tx.QueryRow("SELECT count(*) FROM library_folders WHERE path = ?", path).Scan(&n); err != nil || n > 0 {
		return err
	}
	_, err := database.Insert(tx, "library_folders", &LibraryFolder{Path: path, Kind: kind, Label: label,
		Health: "unknown", SameDiskAs: []int64{}}, database.System)
	return err
}
