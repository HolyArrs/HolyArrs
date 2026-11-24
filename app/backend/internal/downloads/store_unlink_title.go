package downloads

import "database/sql"

// A deleted title lets go of its Downloader links: each torrent or job
// matched to it goes back to the matcher, so it can match the title again
// once it is re-added, or show as not in the Library until then.
const (
	dropTitleLinksSQL  = `DELETE FROM client_links WHERE title_id = ?`
	rematchTitleRowSQL = `UPDATE dl_match_queue SET outcome = '', linked_title_id = NULL, attempts = 0, next_try_at = '', last_error = ''
		WHERE linked_title_id = ?`
)

// UnlinkTitleTx lets go of the Downloader links of a deleted title.
func UnlinkTitleTx(tx *sql.Tx, titleID int64) error {
	if _, err := tx.Exec(dropTitleLinksSQL, titleID); err != nil {
		return err
	}
	_, err := tx.Exec(rematchTitleRowSQL, titleID)
	return err
}
