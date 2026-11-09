// Package downloads sends Releases to download clients and keeps track of
// what those clients do with them. Each torrent or Usenet client sits behind
// the Engine interface, so the rest of HolyArrs never needs to know which one
// runs a Download. A background reader polls every Downloader on its own
// cadence and keeps one snapshot per client in memory, so pages read fresh
// state without asking the clients each time. It saves only what changed
// between reads, queues new items for title matching, and sends small
// patches to the live stream instead of whole lists.
package downloads
