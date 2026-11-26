// Package downloads sends Releases to download clients and keeps track of
// what those clients do with them. Each torrent or Usenet client sits behind
// the Engine interface, so the rest of HolyArrs never needs to know which one
// runs a Download. A background reader polls every Downloader on its own
// cadence and keeps one snapshot per client in memory, so pages read fresh
// state without asking the clients each time. It saves only what changed
// between reads, queues new items for title matching, and sends small
// patches to the live stream instead of whole lists.
//
// A Download is never deleted when it ends. A removed one is marked cleaned
// and kept: its History Events, Imports and Slot links point at it, its
// removal and clear retries live on it, and the sync uses it to recognise a
// job HolyArrs already sent. The Downloads screen leaves cleaned rows out and
// they are not purged automatically; cleaned_at says when, and only briefly
// hides the Downloader's copy of the job from the screen's client rows.
package downloads
