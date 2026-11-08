package sources

import (
	"database/sql"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// EnsureOwn adds the Source named on the command line once and lets it reach
// the local network: it is the person's own (the lab's fake indexer on a
// private network).
func EnsureOwn(tx *sql.Tx, src domain.Source) error {
	var n int
	if err := tx.QueryRow("SELECT count(*) FROM sources WHERE url = ?", src.URL).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		if _, err := database.Insert(tx, "sources", &src, domain.System); err != nil {
			return err
		}
	}
	_, err := tx.Exec("UPDATE sources SET allow_local_network = 1 WHERE url = ?", src.URL)
	return err
}
