package api

import (
	"encoding/json"
	"testing"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/downloader"
)

// HolyArrs' own row names the Source it came from and who added its
// Library title.
func TestDownloadRowSourcePostedRequested(t *testing.T) {
	x := seed41(t, domain.SlotDownloading, "downloading")
	exec41(t, x.s, "UPDATE releases SET published_at = '2026-01-02T03:04:05Z'")
	var out viewRows
	_ = json.Unmarshal(do(t, "GET", x.srv.URL+"/api/v1/downloads/view", "", 200), &out)
	r := out.Downloads[0]
	// A torrent has no Usenet post date, even when its release has one.
	if r.SourceName != "Lab" || r.SourceID == 0 || r.PostedAt != nil || r.RequestedBy != "" {
		t.Fatalf("own row %+v", r)
	}
	exec41(t, x.s, `INSERT INTO users(id, created_at, created_by, updated_at, updated_by, name, role, password_hash) VALUES (7, 't', '{}', 't', '{}', 'mara', 'member', 'x')`)
	exec41(t, x.s, `UPDATE titles SET created_by = '{"kind":"user","id":7}'`)
	out = viewRows{}
	_ = json.Unmarshal(do(t, "GET", x.srv.URL+"/api/v1/downloads/view", "", 200), &out)
	if r := out.Downloads[0]; r.RequestedBy != "mara" {
		t.Fatalf("requested by %q", r.RequestedBy)
	}
}

// Outside rows: no Source; a linked title nobody added reads "Added in
// qBittorrent", one a person added names them; no title, nothing.
func TestDownloadRowOutsideRequested(t *testing.T) {
	x := seed41(t, domain.SlotDownloading, "downloading")
	f := &allClient{}
	exec41(t, x.s, "UPDATE downloaders SET url = ?", f.server(t).URL)
	var titleID int64
	_ = x.s.Read().QueryRow("SELECT id FROM titles LIMIT 1").Scan(&titleID)
	exec41(t, x.s, `INSERT INTO client_links (downloader_id, hash, name, state, title_id, slot_ids, covers, candidates, how, matched_at) VALUES (1, ?, 'Linux.ISO', 'linked', ?, '[]', '', '[]', 'name', '')`, outHash, titleID)
	read := func() map[string]DownloadRow {
		feedStore(t, x.api)
		var out viewRows
		_ = json.Unmarshal(do(t, "GET", x.srv.URL+"/api/v1/downloads/view", "", 200), &out)
		by := map[string]DownloadRow{}
		for _, r := range out.Downloads {
			by[r.Hash] = r
		}
		return by
	}
	by := read()
	if o := by[outHash]; o.RequestedBy != "Added in qBittorrent" || o.SourceName != "" || o.PostedAt != nil {
		t.Fatalf("linked outside row %+v", o)
	}
	if s := by[seedHash]; s.RequestedBy != "" {
		t.Fatalf("untitled outside row %+v", s)
	}
	exec41(t, x.s, `INSERT INTO users(id, created_at, created_by, updated_at, updated_by, name, role, password_hash) VALUES (7, 't', '{}', 't', '{}', 'mara', 'member', 'x')`)
	exec41(t, x.s, `UPDATE titles SET created_by = '{"kind":"user","id":7}'`)
	if o := read()[outHash]; o.RequestedBy != "mara" {
		t.Fatalf("outside row by person %+v", o)
	}
}

// A Usenet client's post time reaches the row; torrents never carry one.
func TestClientRowPostedAt(t *testing.T) {
	u := domain.Downloader{ID: 2, Name: "S", Adapter: "sabnzbd", Protocol: "usenet"}
	if r := clientRow(u, downloader.Job{ID: "SABnzbd_nzo_1", PostedAt: 1700000000}); r.PostedAt == nil || r.PostedAt.Unix() != 1700000000 {
		t.Fatalf("usenet row %+v", r.PostedAt)
	}
	q := domain.Downloader{ID: 1, Name: "Q", Adapter: "qbittorrent", Protocol: "torrent"}
	if r := clientRow(q, downloader.Job{ID: outHash, PostedAt: 1700000000}); r.PostedAt != nil {
		t.Fatalf("torrent row %+v", r.PostedAt)
	}
}
