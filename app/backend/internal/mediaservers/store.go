package mediaservers

import (
	"context"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
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
