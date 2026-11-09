package jobs

// Job kinds for the search work. The values are stored in the jobs table,
// and the first three also name a schedules row, so they never change.
const (
	KindFeedCheck     = "feed_check"
	KindGrabHeld      = "grab_held"
	KindSearchMissing = "search_missing"
	KindSearchAlbum   = "search_album"
	KindSearchSlot    = "search_slot"
	KindSearchSeason  = "search_season"
)
