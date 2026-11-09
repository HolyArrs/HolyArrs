package discover

import (
	"context"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// TitleDetails reads a title's details for the title pages through the
// Explore cache c, fetching them when missing or old.
func TitleDetails(ctx context.Context, c *Cache, p metadata.Provider, kind, id string) (metadata.Details, error) {
	d, ok := metadata.DiscovererFor(ctx, p)
	if !ok {
		return metadata.Details{}, metadata.ErrServiceUnavailable
	}
	key := "details|" + kind + "|" + id
	e, cached := c.Get(key)
	if !cached || time.Since(e.At) >= TTL["details"] {
		det, err := d.Details(ctx, kind, id)
		if err != nil {
			return metadata.Details{}, err
		}
		e = Entry{At: time.Now(), Details: &det}
		c.Put(key, e)
	}
	return *e.Details, nil
}
