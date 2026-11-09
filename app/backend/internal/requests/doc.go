// Package requests decides what members ask for. It holds asks and their
// approvals, follows, problem reports, outside lists and Plex watchlist
// sync. A second ask for the same title joins the first, so one decision
// answers everyone who asked. Approving adds the title and starts a search,
// and the askers hear when it is approved, turned down and ready. Outside
// lists add the titles of a TMDB or Trakt list on a schedule and skip any
// title on the never-add list, so a removed title does not come back.
package requests
