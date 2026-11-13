package migrate

import "testing"

// A qBittorrent brought over from Sonarr, Radarr or both is one Downloader
// with its own holyarrs category, left alone after import, paused until
// checked, and with no category move or own-only switch set.
func TestMoveSharedClientGetsOwnCategory(t *testing.T) {
	qbit := Provider{Name: "qBit", Kind: "QBittorrent", Enabled: true, Priority: 1, Settings: map[string]any{"host": "qbit", "port": int64(8080), "username": "admin"}, NeedsReentry: []string{"password"}}
	for name, change := range map[string]func(radarr, bazarr, seerr *Snapshot){
		"radarr only": nil,
		"sonarr only": func(radarr, bazarr, _ *Snapshot) {
			radarr.DownloadClients = nil
			*bazarr = Snapshot{Kind: Sonarr, Version: "4.0.16", DownloadClients: []Provider{qbit}}
		},
		"both on one client": func(_, bazarr, _ *Snapshot) {
			*bazarr = Snapshot{Kind: Sonarr, Version: "4.0.16", DownloadClients: []Provider{qbit}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			l := newMoveLabWith(t, change)
			if mv := l.move(t, Choices{}); mv.State != "done" {
				t.Fatalf("move %+v", mv)
			}
			if n := l.count(t, "SELECT count(*) FROM downloaders"); n != 1 {
				t.Fatalf("%d Downloaders", n)
			}
			if n := l.count(t, `SELECT count(*) FROM downloaders WHERE category = 'holyarrs' AND after_import = 'leave' AND state = 'disabled'
				AND category_after_import = '' AND own_only = 0`); n != 1 {
				t.Fatal("the Downloader is not on its own category, left alone after import")
			}
		})
	}
}
