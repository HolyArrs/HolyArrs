package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

const (
	sureHash   = "aaaa000000000000000000000000000000000001"
	unsureHash = "aaaa000000000000000000000000000000000002"
	isoHash    = "aaaa000000000000000000000000000000000003"
)

// mediaClient is a qBittorrent holding three torrents HolyArrs did not
// send; it fails the test on any write call.
func mediaClient(t *testing.T) *httptest.Server {
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/v2/")
		switch path {
		case "auth/login", "app/version", "app/webapiVersion", "app/preferences", "torrents/categories":
			_, _ = w.Write([]byte(`{}`))
		case "torrents/info":
			if r.URL.Query().Get("category") != "" || r.FormValue("category") != "" {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			_, _ = w.Write([]byte(`[
{"hash":"` + sureHash + `","name":"Paper.Harbour.2023.1080p.WEB-DL.H264-GRP","category":"movies","size":1000,"progress":1,"state":"uploading"},
{"hash":"` + unsureHash + `","name":"Quiet.Orchard.1080p.WEB-DL-GRP","category":"","size":1000,"progress":1,"state":"uploading"},
{"hash":"` + isoHash + `","name":"ubuntu-24.04-desktop-amd64.iso","category":"linux","size":5000,"progress":1,"state":"uploading"}]`))
		case "torrents/delete":
			w.WriteHeader(200)
		}
	}))
	t.Cleanup(qb.Close)
	return qb
}

func addQB(t *testing.T, st *database.Store, url string) int64 {
	t.Helper()
	var id int64
	err := st.Write(context.Background(), func(tx *sql.Tx) error {
		dr := &domain.Downloader{Writable: true, Name: "Q", Adapter: "qbittorrent", Protocol: "torrent", URL: url, Category: "holyarrs", Priority: 1,
			PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok", AllowLocal: true}
		var err error
		id, err = database.Insert(tx, "downloaders", dr, domain.System)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// route: GET /api/v1/downloads/matches
// route: POST /api/v1/downloads/matches/run
// route: POST /api/v1/downloads/matches/{id}/pick
func TestClientMatchLinksAndConfirms(t *testing.T) {
	t.Parallel()
	r := newAskRig(t)
	addQB(t, r.st, mediaClient(t).URL)
	md := r.md
	var mu sync.Mutex
	lookups := 0
	r.api.DownloadsWeb().Match.SearchMovies = func(ctx context.Context, name string, year int) ([]metadata.Movie, error) {
		mu.Lock()
		lookups++
		mu.Unlock()
		ms, err := md.Search(ctx, name, year)
		if strings.Contains(strings.ToLower(name), "orchard") {
			ms = append(ms, metadata.Movie{Name: "Quiet Orchard", Year: 1999, IDs: map[string]string{"tmdb": "900099"}})
		}
		return ms, err
	}
	r.owner.call("PUT", "/api/v1/settings/downloaders", `{"stalled_after_hours":6,"stalled_act_hours":24,"grab_spacing_seconds":2,"downloads_auto_add":true}`, 200)
	downloadsBefore := count(t, r.st, "SELECT COUNT(*) FROM downloads")

	feedStore(t, r.api)
	matchStored(t, r.api)
	// Matching again finds every torrent stored: no new lookups.
	matchStored(t, r.api)
	r.owner.call("POST", "/api/v1/downloads/matches/run", "", 202)
	if lookups != 2 {
		t.Fatalf("lookups %d, want 2 (one per media torrent, never repeated)", lookups)
	}

	var view viewRows
	_ = json.Unmarshal(r.owner.call("GET", "/api/v1/downloads/view", "", 200), &view)
	by := map[string]downloads.DownloadRow{}
	for _, x := range view.Downloads {
		by[x.Hash] = x
	}
	if s := by[sureHash]; !s.Linked || !s.Outside || s.TitleID == 0 || s.Title != "Paper Harbour" || s.Kind != "movie" {
		t.Fatalf("sure row %+v", s)
	}
	if u := by[unsureHash]; u.Linked || u.TitleID != 0 {
		t.Fatalf("unsure row %+v", u)
	}

	var list struct {
		Matches []downloads.MatchView `json:"matches"`
		Counts  map[string]int        `json:"counts"`
	}
	_ = json.Unmarshal(r.owner.call("GET", "/api/v1/downloads/matches", "", 200), &list)
	if list.Counts[downloads.LinkLinked] != 1 || list.Counts[downloads.LinkUnsure] != 1 || list.Counts[downloads.LinkNotMedia] != 1 || len(list.Matches) != 1 || len(list.Matches[0].Candidates) != 2 {
		t.Fatalf("matches %+v", list)
	}

	// The added title neither searches nor keeps up to date, and nothing is
	// a download record or a file.
	if n := count(t, r.st, "SELECT COUNT(*) FROM downloads"); n != downloadsBefore {
		t.Fatalf("download records %d -> %d", downloadsBefore, n)
	}
	if n := count(t, r.st, "SELECT COUNT(*) FROM jobs WHERE kind LIKE '%search%'"); n != 0 {
		t.Fatalf("%d search jobs", n)
	}
	if n := count(t, r.st, "SELECT COUNT(*) FROM files"); n != 0 {
		t.Fatalf("%d files", n)
	}
	if n := count(t, r.st, "SELECT COUNT(*) FROM items WHERE title_id = ? AND keep_up_to_date = 1", by[sureHash].TitleID); n != 0 {
		t.Fatal("the added title keeps up to date")
	}

	// Confirm: a choice not in the list is refused; one in it adds and links.
	id := list.Matches[0].ID
	r.owner.call("POST", fmt.Sprintf("/api/v1/downloads/matches/%d/pick", id), `{"candidate":"tmdb:1"}`, 400)
	var picked downloads.MatchView
	_ = json.Unmarshal(r.owner.call("POST", fmt.Sprintf("/api/v1/downloads/matches/%d/pick", id), `{"candidate":"tmdb:900003"}`, 200), &picked)
	if picked.State != downloads.LinkLinked || picked.TitleID == 0 {
		t.Fatalf("picked %+v", picked)
	}
	// Picking again reuses the title: still one Quiet Orchard, one link row.
	r.owner.call("POST", fmt.Sprintf("/api/v1/downloads/matches/%d/pick", id), `{"candidate":"tmdb:900003"}`, 200)
	if n := count(t, r.st, "SELECT COUNT(*) FROM titles WHERE name = 'Quiet Orchard'"); n != 1 {
		t.Fatalf("%d Quiet Orchard titles", n)
	}
	if n := count(t, r.st, "SELECT COUNT(*) FROM client_links"); n != 3 {
		t.Fatalf("%d link rows", n)
	}
	r.owner.call("POST", fmt.Sprintf("/api/v1/downloads/matches/%d/pick", id), `{"not_media":true}`, 200)
}

// route: POST /api/v1/downloaders/{id}/items/{hash}/remove
// Removing a torrent HolyArrs did not send needs the settings permission.
func TestOutsideRemoveNeedsSettings(t *testing.T) {
	t.Parallel()
	r := newAskRig(t)
	dl := addQB(t, r.st, mediaClient(t).URL)
	r.owner.call("PUT", fmt.Sprintf("/api/v1/users/%d", r.ana.id), `{"perms":["ask","manage_library"]}`, 200)
	path := fmt.Sprintf("/api/v1/downloaders/%d/items/%s/remove", dl, isoHash)
	r.ana.call("POST", path, `{"delete_files":true}`, 403)
	r.ana.call("POST", path, "", 403)
	r.ana.call("POST", fmt.Sprintf("/api/v1/downloaders/%d/items/%s/pause", dl, isoHash), "", 200)
	r.owner.call("POST", path, `{"delete_files":false}`, 200)
}

// route: POST /api/v1/downloaders/{id}/items/{hash}/trackers
// route: POST /api/v1/downloaders/{id}/location
// route: POST /api/v1/downloaders/{id}/speed-limits
// route: PUT /api/v1/downloaders/{id}/items/{hash}/files
// Changing a torrent HolyArrs did not send, beyond pause and the like, needs
// the settings permission.
func TestOutsideChangesNeedSettings(t *testing.T) {
	t.Parallel()
	r := newAskRig(t)
	dl := addQB(t, r.st, mediaClient(t).URL)
	r.owner.call("PUT", fmt.Sprintf("/api/v1/users/%d", r.ana.id), `{"perms":["ask","manage_library"]}`, 200)
	item := fmt.Sprintf("/api/v1/downloaders/%d/items/%s", dl, isoHash)
	r.ana.call("POST", item+"/trackers", `{"urls":["https://t.example.org/announce"]}`, 403)
	r.ana.call("POST", item+"/trackers/remove", `{"refs":["x"]}`, 403)
	r.ana.call("PUT", item+"/files", `{"indexes":[0],"priority":0}`, 403)
	r.ana.call("POST", fmt.Sprintf("/api/v1/downloaders/%d/location", dl), `{"hashes":["`+isoHash+`"],"folder":"x"}`, 403)
	r.ana.call("POST", fmt.Sprintf("/api/v1/downloaders/%d/speed-limits", dl), `{"hashes":["`+isoHash+`"],"down":1,"up":1}`, 403)
	r.ana.call("POST", fmt.Sprintf("/api/v1/downloaders/%d/bulk", dl), `{"action":"pause","hashes":["`+isoHash+`"]}`, 200)
	r.owner.call("POST", item+"/trackers", `{"urls":["https://t.example.org/announce"]}`, 200)
	r.owner.call("POST", fmt.Sprintf("/api/v1/downloaders/%d/speed-limits", dl), `{"hashes":["`+isoHash+`"],"down":1,"up":1}`, 200)
}
