package downloads

import "context"

// HasDownloader reports whether a Downloader that is not removed takes the
// protocol (torrent, usenet). A failed read says yes, so a passing database
// hiccup never stops a pick.
func (m *Manager) HasDownloader(ctx context.Context, protocol string) bool {
	var n int
	if err := m.Store.Read().QueryRowContext(ctx, "SELECT count(*) FROM downloaders WHERE protocol = ? AND removed = 0", protocol).Scan(&n); err != nil {
		return true
	}
	return n > 0
}
