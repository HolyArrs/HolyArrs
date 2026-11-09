package mediaservers

import (
	"context"
	"database/sql"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// listServers returns every media server by name.
func listServers(ctx context.Context, q database.Lister) ([]domain.MediaServer, error) {
	return database.Select[domain.MediaServer](ctx, q, "media_servers", "1=1 ORDER BY name")
}

// getServer returns one media server, or database.ErrNotFound.
func getServer(ctx context.Context, q database.Querier, id int64) (domain.MediaServer, error) {
	var m domain.MediaServer
	err := database.Get(ctx, q, "media_servers", id, &m)
	return m, err
}

// enabledServers returns the media servers that are switched on, oldest first.
func enabledServers(ctx context.Context, q database.Lister) ([]domain.MediaServer, error) {
	return database.Select[domain.MediaServer](ctx, q, "media_servers", "enabled = 1 ORDER BY id")
}

func insertServer(tx *sql.Tx, m *domain.MediaServer, by domain.Actor) error {
	_, err := database.Insert(tx, "media_servers", m, by)
	return err
}

func updateServer(tx *sql.Tx, m *domain.MediaServer, by domain.Actor) error {
	return database.Update(tx, "media_servers", m, by)
}

func deleteServer(tx *sql.Tx, id int64) error {
	_, err := tx.Exec("DELETE FROM media_servers WHERE id = ?", id)
	return err
}

// nameUsed says another media server already has name, ignoring case.
func nameUsed(tx *sql.Tx, name string, id int64) error {
	var n int
	if err := tx.QueryRow("SELECT count(*) FROM media_servers WHERE name = ? COLLATE NOCASE AND id != ?", name, id).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return ErrNameUsed
	}
	return nil
}

// markError keeps why the last call to a media server failed.
func markError(ctx context.Context, s *database.Store, id int64, msg string) {
	_ = s.Write(context.WithoutCancel(ctx), func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE media_servers SET last_error = ? WHERE id = ?", msg, id)
		return err
	})
}

// markOK keeps when the last call to a media server worked and clears the error.
func markOK(ctx context.Context, s *database.Store, id int64, at string) {
	_ = s.Write(context.WithoutCancel(ctx), func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE media_servers SET last_ok_at = ?, last_error = '' WHERE id = ?", at, id)
		return err
	})
}
