package requests

import (
	"context"
	"database/sql"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// OwnerName is user 1's name (the stand-in person without sign-in).
func OwnerName(ctx context.Context, db *sql.DB) string {
	var name string
	_ = db.QueryRowContext(ctx, "SELECT name FROM users WHERE id = 1").Scan(&name)
	return name
}

// WhoName names the person for an Event summary.
func WhoName(r *http.Request) string {
	if who, ok := webserver.From(r.Context()); ok && who.Name != "" {
		return who.Name
	}
	return "The owner"
}
