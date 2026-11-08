package accounts

import "github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"

// SessionNeedsRecentLocal says whether a browser session must show a recent
// local password proof before using the route pattern. The route-table
// snapshot test reads it so a move cannot change which routes ask.
func SessionNeedsRecentLocal(pattern string) bool {
	return needsRecentLocal(pattern, webserver.Caller{Via: "session"})
}
