// Package requests holds asks, approvals, follows, reports, outside lists
// and Plex watchlist sync: everything a member asks for and how it is
// decided. A second ask for the same thing joins the first; approving adds
// the title and starts its search, and the people who asked hear when it is
// approved, turned down and ready. Outside lists add the titles of a TMDB or
// Trakt list on a schedule and skip titles on the never-add list.
package requests
