// Package dlsync is the background reader for download clients. It polls each
// Downloader on its own cadence, keeps one in-memory snapshot per client,
// works out what changed between reads, saves items and torrent log lines to
// the database, queues new items for title matching, and publishes patches
// to the live stream.
package downloads
