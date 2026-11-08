package downloads

import (
	"database/sql"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// EnsureDownloader adds the Downloader once, matched by its address.
func EnsureDownloader(tx *sql.Tx, d domain.Downloader) error {
	var n int
	if err := tx.QueryRow("SELECT count(*) FROM downloaders WHERE url = ?", d.URL).Scan(&n); err != nil || n > 0 {
		return err
	}
	_, err := database.Insert(tx, "downloaders", &d, domain.System)
	return err
}
