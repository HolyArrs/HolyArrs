package api

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
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/store"
)

func TestGuessOf(t *testing.T) {
	cases := []struct {
		name, cat string
		kind      string
		title     string
		year      int
		covers    string
	}{
		{"Paper.Harbour.2023.1080p.WEB-DL.H264-GRP", "movies", "movie", "Paper Harbour", 2023, ""},
		{"Some.Show.S01E02.720p.HDTV.x264-GRP", "tv_shows", "show", "Some Show", 0, "S01E02"},
		{"Some.Show.S02.1080p.WEB-DL-GRP", "tv_shows", "show", "Some Show", 0, "Season 2"},
		{"Some.Show.S01E02E03.1080p.WEB-GRP", "", "show", "Some Show", 0, "S01E02-E03"},
		{"setup.exe", "", "", "", 0, ""},
	}
	for _, c := range cases {
		g := guessOf(c.name, c.cat)
		if g.kind != c.kind || (c.kind != "" && (!strings.EqualFold(g.title, c.title) || g.year != c.year)) {
			t.Errorf("%s: %+v", c.name, g)
		}
		if c.kind == "show" && coversLine(g) != c.covers {
			t.Errorf("%s covers %q", c.name, coversLine(g))
		}
	}
	g := guessOf("Some.Show.S02.1080p.WEB-DL-GRP", "tv")
	if !coversEpisode(g, 2, 5) || coversEpisode(g, 1, 5) {
		t.Fatalf("season pack covers %+v", g)
	}
}

func TestPickSure(t *testing.T) {
	one := []Candidate{{ID: "tmdb:1", Name: "Paper Harbour", Year: 2023, Score: likeness("Paper Harbour", "Paper Harbour")}, {ID: "tmdb:2", Name: "Paper Moon", Year: 1973, Score: likeness("Paper Harbour", "Paper Moon")}}
	if i := pickSure(one, 2023); i != 0 {
		t.Fatalf("one exact: %d", i)
	}
	if i := pickSure(one, 1990); i != -1 {
		t.Fatalf("wrong year must be unsure: %d", i)
	}
	twins := []Candidate{{ID: "tmdb:1", Name: "Quiet Orchard", Year: 2022, Score: 100}, {ID: "tmdb:9", Name: "Quiet Orchard", Year: 1999, Score: 100}}
	if i := pickSure(twins, 0); i != -1 {
		t.Fatalf("two alike without a year must be unsure: %d", i)
	}
	if i := pickSure(twins, 1999); i != 1 {
		t.Fatalf("the year decides: %d", i)
	}
	if likeness("The Office", "Office") != 100 || likeness("Paper Harbour", "Lighthouse") > 50 {
		t.Fatal("likeness")
	}
}

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

func addQB(t *testing.T, st *store.Store, url string) int64 {
	t.Helper()
	var id int64
	err := st.Write(context.Background(), func(tx *sql.Tx) error {
		dr := &domain.Downloader{Name: "Q", Adapter: "qbittorrent", Protocol: "torrent", URL: url, Category: "holyarrs", Priority: 1,
			PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok", AllowLocal: true}
		var err error
		id, err = store.Insert(tx, "downloaders", dr, domain.System)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func count(t *testing.T, st *store.Store, q string, args ...any) int {
	t.Helper()
	var n int
	if err := st.Read().QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// route: GET /api/v1/downloads/matches
// route: POST /api/v1/downloads/matches/run
// route: POST /api/v1/downloads/matches/{id}/pick
func TestClientMatchLinksAndConfirms(t *testing.T) {
	r := newAskRig(t)
	addQB(t, r.st, mediaClient(t).URL)
	md := r.api.s.Metadata
	var mu sync.Mutex
	lookups := 0
	r.api.match.searchMovies = func(ctx context.Context, name string, year int) ([]metadata.Movie, error) {
		mu.Lock()
		lookups++
		mu.Unlock()
		ms, err := md.Search(ctx, name, year)
		if strings.Contains(strings.ToLower(name), "orchard") {
			ms = append(ms, metadata.Movie{Name: "Quiet Orchard", Year: 1999, IDs: map[string]string{"tmdb": "900099"}})
		}
		return ms, err
	}
	done := make(chan struct{}, 10)
	r.api.match.done = func() { done <- struct{}{} }
	wait := func(n int) {
		for range n {
			select {
			case <-done:
			case <-time.After(10 * time.Second):
				t.Fatal("matcher did not finish")
			}
		}
	}
	downloadsBefore := count(t, r.st, "SELECT COUNT(*) FROM downloads")

	r.owner.call("GET", "/api/v1/downloads/view", "", 200)
	wait(3)
	// Reading again finds every torrent stored: no new lookups.
	r.owner.call("GET", "/api/v1/downloads/view", "", 200)
	r.owner.call("POST", "/api/v1/downloads/matches/run", "", 202)
	time.Sleep(100 * time.Millisecond)
	if lookups != 2 {
		t.Fatalf("lookups %d, want 2 (one per media torrent, never repeated)", lookups)
	}

	var view viewRows
	_ = json.Unmarshal(r.owner.call("GET", "/api/v1/downloads/view", "", 200), &view)
	by := map[string]DownloadRow{}
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
		Matches []MatchView    `json:"matches"`
		Counts  map[string]int `json:"counts"`
	}
	_ = json.Unmarshal(r.owner.call("GET", "/api/v1/downloads/matches", "", 200), &list)
	if list.Counts[linkLinked] != 1 || list.Counts[linkUnsure] != 1 || list.Counts[linkNotMedia] != 1 || len(list.Matches) != 1 || len(list.Matches[0].Candidates) != 2 {
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
	var picked MatchView
	_ = json.Unmarshal(r.owner.call("POST", fmt.Sprintf("/api/v1/downloads/matches/%d/pick", id), `{"candidate":"tmdb:900003"}`, 200), &picked)
	if picked.State != linkLinked || picked.TitleID == 0 {
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

// An indexer-manager category like "prowlarr" says nothing about the kind: a
// name with a year and a resolution is still a movie; other names stay out.
func TestGuessNeutralCategory(t *testing.T) {
	if g := guessOf("Root_Cause_2019_Bluray-1080p", "prowlarr"); g.kind != "movie" {
		t.Fatalf("prowlarr movie: %+v", g)
	}
	if g := guessOf("Some.Linux.Distro.x64", "linux"); g.kind != "" {
		t.Fatalf("linux: %+v", g)
	}
}
