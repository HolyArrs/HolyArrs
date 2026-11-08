package main

import (
	"encoding/json"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
)

func TestDownloadRowSourcePostedRequested(t *testing.T) {
	t.Parallel()
	x := seed41(t, mediafiles.SlotDownloading, "downloading")
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
	t.Parallel()
	x := seed41(t, mediafiles.SlotDownloading, "downloading")
	f := &allClient{}
	exec41(t, x.s, "UPDATE downloaders SET url = ?", f.server(t).URL)
	var titleID int64
	_ = x.s.Read().QueryRow("SELECT id FROM titles LIMIT 1").Scan(&titleID)
	exec41(t, x.s, `INSERT INTO client_links (downloader_id, hash, name, state, title_id, slot_ids, covers, candidates, how, matched_at) VALUES (1, ?, 'Linux.ISO', 'linked', ?, '[]', '', '[]', 'name', '')`, outHash, titleID)
	read := func() map[string]downloads.DownloadRow {
		feedStore(t, x.api)
		var out viewRows
		_ = json.Unmarshal(do(t, "GET", x.srv.URL+"/api/v1/downloads/view", "", 200), &out)
		by := map[string]downloads.DownloadRow{}
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

// Which Source and who asked stay with people who manage the Library: a
// member watching their own ask sees neither; the owner sees both.
func TestDownloadRowFieldsHiddenFromMember(t *testing.T) {
	t.Parallel()
	x := seed41(t, mediafiles.SlotDownloading, "downloading")
	exec41(t, x.s, `INSERT INTO users(id, created_at, created_by, updated_at, updated_by, name, role, password_hash) VALUES
		(1, 't', '{}', 't', '{}', 'owner', 'owner', 'x'), (2, 't', '{}', 't', '{}', 'ana', 'member', 'x')`)
	exec41(t, x.s, `UPDATE titles SET created_by = '{"kind":"user","id":2}'`)
	exec41(t, x.s, `INSERT INTO asks (id, created_at, created_by, updated_at, updated_by, kind, tmdb_id, name, state, title_id, asked_by, asked_at)
		SELECT 1, 't', '{}', 't', '{}', 'movie', '1', 'Salt Choir', 'approved', t.id, 2, 't' FROM titles t LIMIT 1`)
	exec41(t, x.s, `INSERT INTO ask_people (ask_id, user_id, asked_at) VALUES (1, 2, 't')`)
	mine := viewAs(t, x, webserver.Caller{UserID: 2, Role: webserver.RoleMember, Perms: []webserver.Perm{webserver.PermAsk}, Via: "session"})
	if len(mine) != 1 || mine[0].SourceName != "" || mine[0].SourceID != 0 || mine[0].RequestedBy != "" {
		t.Fatalf("member row %+v", mine)
	}
	own := viewAs(t, x, webserver.Caller{UserID: 1, Role: webserver.RoleOwner, Via: "session"})
	if len(own) != 1 || own[0].SourceName != "Lab" || own[0].SourceID == 0 || own[0].RequestedBy != "ana" {
		t.Fatalf("owner row %+v", own)
	}
}
