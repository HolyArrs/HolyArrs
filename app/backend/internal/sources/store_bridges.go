package sources

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// listBridges returns every bridge, oldest first.
func listBridges(ctx context.Context, q database.Lister) ([]Bridge, error) {
	return database.Select[Bridge](ctx, q, "bridges", "1=1 ORDER BY id")
}

// getBridge returns one bridge.
func getBridge(ctx context.Context, q database.Querier, id int64) (Bridge, error) {
	var b Bridge
	err := database.Get(ctx, q, "bridges", id, &b)
	return b, err
}

// bridgesAt counts the bridges of a kind at an address, ignoring case.
func bridgesAt(ctx context.Context, q database.Querier, kind, url string) (int, error) {
	var n int
	err := q.QueryRowContext(ctx, "SELECT count(*) FROM bridges WHERE kind = ? AND lower(url) = lower(?)", kind, url).Scan(&n)
	return n, err
}

// countBridges counts the bridges of a kind.
func countBridges(ctx context.Context, q database.Querier, kind string) (int, error) {
	var n int
	err := q.QueryRowContext(ctx, "SELECT count(*) FROM bridges WHERE kind = ?", kind).Scan(&n)
	return n, err
}

func insertBridgeTx(tx *sql.Tx, b *Bridge, by domain.Actor) error {
	_, err := database.Insert(tx, "bridges", b, by)
	return err
}

func updateBridgeTx(tx *sql.Tx, b *Bridge, by domain.Actor) error {
	return database.Update(tx, "bridges", b, by)
}

func deleteBridgeTx(ctx context.Context, tx *sql.Tx, id int64) error {
	_, err := tx.ExecContext(ctx, "DELETE FROM bridges WHERE id = ?", id)
	return err
}

// rememberBridgeTx keeps a short hint of the key, the sites brought in and
// whether the bridge is local.
func rememberBridgeTx(ctx context.Context, tx *sql.Tx, id int64, keyHint, knownSites, local string) error {
	_, err := tx.ExecContext(ctx, `UPDATE bridges SET extra = json_set(extra, '$.key_hint', ?, '$.known_sites', ?, '$.local', ?) WHERE id = ?`,
		keyHint, knownSites, local, id)
	return err
}

// recordBridgeCheckTx keeps the outcome of the last check of a bridge.
func recordBridgeCheckTx(ctx context.Context, tx *sql.Tx, id int64, at time.Time, lastError string, sites int) error {
	_, err := tx.ExecContext(ctx, `UPDATE bridges SET last_check = ?, last_error = ?, site_count = ?, updated_at = ? WHERE id = ?`,
		database.FormatTime(at), lastError, sites, database.FormatTime(at), id)
	return err
}

// releaseSourcesTx lets the Sources a bridge brought in stay as ordinary Sources.
func releaseSourcesTx(ctx context.Context, tx *sql.Tx, bridgeID int64) error {
	_, err := tx.ExecContext(ctx, `UPDATE sources SET extra = json_remove(extra, '$.bridge', '$.bridge_site') WHERE json_extract(extra, '$.bridge') = ?`, strconv.FormatInt(bridgeID, 10))
	return err
}

// joinBridgeTx marks a Source as brought in by a bridge for one of its sites.
func joinBridgeTx(ctx context.Context, tx *sql.Tx, sourceID, bridgeID int64, site string) error {
	_, err := tx.ExecContext(ctx, `UPDATE sources SET extra = json_set(extra, '$.bridge', ?, '$.bridge_site', ?) WHERE id = ?`,
		strconv.FormatInt(bridgeID, 10), site, sourceID)
	return err
}

// handMadeSourceAt finds the Source a person added by hand for a feed address.
func handMadeSourceAt(ctx context.Context, q database.Lister, feed string) ([]domain.Source, error) {
	return database.Select[domain.Source](ctx, q, "sources",
		"lower(rtrim(url, '/')) = lower(?) AND json_extract(extra, '$.bridge') IS NULL AND json_extract(extra, '$.old_app') IS NULL ORDER BY id LIMIT 1",
		trimSlash(feed))
}

// bridgeSources lists the Sources a Jackett bridge made.
func bridgeSources(ctx context.Context, q database.Lister, bridgeID int64) ([]domain.Source, error) {
	return database.Select[domain.Source](ctx, q, "sources", "json_extract(extra, '$.bridge') = ? ORDER BY id", strconv.FormatInt(bridgeID, 10))
}

// prowlarrSources lists the arr Sources whose address starts with a pattern from likePrefix.
func prowlarrSources(ctx context.Context, q database.Lister, pattern string) ([]domain.Source, error) {
	return database.Select[domain.Source](ctx, q, "sources",
		"json_extract(extra, '$.old_app') IS NOT NULL AND lower(url) LIKE lower(?) ESCAPE '\\' ORDER BY id", pattern)
}

// sourcesCalled counts the Sources with a name, ignoring case.
func sourcesCalled(ctx context.Context, q database.Querier, name string) int {
	var n int
	_ = q.QueryRowContext(ctx, "SELECT count(*) FROM sources WHERE lower(name) = lower(?)", name).Scan(&n)
	return n
}

func trimSlash(s string) string { return strings.TrimRight(s, "/") }
