package migrate

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// The Downloaders Sonarr and Radarr use are added as soon as they are read:
// a client both share is added once with each app's category as it was, one
// that does not answer is still added, empty folder maps come from the old
// apps, and a full import later finds them here.
func TestBringDownloadersRightAfterRead(t *testing.T) {
	l := newMoveLabWith(t, func(radarr, _, _ *Snapshot) {
		radarr.DownloadClients[0].Settings["movieCategory"] = "radarr"
		radarr.RemotePathMappings = []RemotePathMapping{{Host: "qbit", RemotePath: "/downloads/", LocalPath: "/data/downloads/"}}
	})
	ctx := context.Background()
	sonarr := &Snapshot{Kind: Sonarr, Via: ViaAPI, DownloadClients: []Provider{
		{Name: "qBittorrent", Kind: "QBittorrent", Enabled: true, Priority: 1, Settings: map[string]any{"host": "qbit", "port": int64(8080), "username": "admin", "tvCategory": "tv-sonarr"}, NeedsReentry: []string{"password"}},
		{Name: "SABnzbd", Kind: "Sabnzbd", Enabled: true, Priority: 1, Settings: map[string]any{"host": "sab.invalid", "port": int64(8080)}, NeedsReentry: []string{"apiKey"}},
	}}
	sonarr.finish()
	saved, err := Save(ctx, l.st, 1, sonarr)
	if err != nil {
		t.Fatal(err)
	}
	snaps := append([]int64{saved.ID}, l.snaps[0])
	got, err := l.mv.BringDownloaders(ctx, snaps, domain.Actor{Kind: "user", ID: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("brought %+v, want qBittorrent and SABnzbd", got)
	}
	for _, b := range got {
		if b.ID == 0 || b.App != "sonarr" || b.Result != ResCreated {
			t.Fatalf("brought %+v: want a new row found in sonarr", b)
		}
	}
	var q domain.Downloader
	if err := database.Get(ctx, l.st.Read(), "downloaders", got[0].ID, &q); err != nil {
		t.Fatal(err)
	}
	if q.CategoryShows != "tv-sonarr" || q.CategoryMovies != "radarr" || q.Category != "tv-sonarr" {
		t.Fatalf("categories %q/%q/%q, want the old apps' own", q.Category, q.CategoryShows, q.CategoryMovies)
	}
	if len(q.PathMap) != 1 || q.PathMap[0].Ours != "/data/downloads" {
		t.Fatalf("folder maps %+v, want Radarr's", q.PathMap)
	}
	if n := l.count(t, "SELECT count(*) FROM downloaders WHERE removed = 0"); n != 2 {
		t.Fatalf("downloaders %d, want 2 (one qBittorrent for both apps)", n)
	}
	// Again: nothing twice.
	again, err := l.mv.BringDownloaders(ctx, snaps, domain.Actor{Kind: "user", ID: 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range again {
		if b.Result == ResCreated {
			t.Fatalf("second bring made %+v again", b)
		}
	}
	if n := l.count(t, "SELECT count(*) FROM downloaders WHERE removed = 0"); n != 2 {
		t.Fatalf("downloaders after a second bring %d, want 2", n)
	}
	// The repeat added nothing, so it leaves no empty import behind.
	if n := l.count(t, "SELECT count(*) FROM moves"); n != 1 {
		t.Fatalf("imports after a repeat bring %d, want 1", n)
	}
}

// A Downloader already here with no folder maps gets the old apps' ones;
// one with its own maps keeps them.
func TestImportFillsOnlyEmptyFolderMaps(t *testing.T) {
	l := newMoveLab(t)
	ctx := context.Background()
	by := domain.Actor{Kind: "user", ID: 1}
	add := func(url string, maps []domain.PathMap) int64 {
		var id int64
		err := l.st.Write(ctx, func(tx *sql.Tx) error {
			var err error
			id, err = database.Insert(tx, "downloaders", &domain.Downloader{Name: url, Adapter: "qbittorrent", Protocol: "torrent", URL: url, Category: "holyarrs", Priority: 1, PathMap: maps, AfterImport: "leave", State: "ok"}, by)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	empty := add("http://qbit:8080", nil)
	set := add("http://qbit2:8080", []domain.PathMap{{Theirs: "/x/", Ours: "/mine/"}})
	move, err := startQuickMove(ctx, l.st, 1, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	maps := []domain.PathMap{{Theirs: "/downloads/", Ours: "/data/downloads/"}}
	dry := &run{m: l.mv, ctx: ctx, by: by, dry: true, rep: &Report{Counts: map[string]int{}}}
	if got := dry.fillEmptyMaps("k", &DownloaderPlan{Adapter: "qbittorrent", URL: "http://qbit:8080"}, maps); !strings.HasPrefix(got, "it will get") {
		t.Fatalf("dry run says %q", got)
	}
	r := &run{m: l.mv, ctx: ctx, by: by, move: move, rep: &Report{Counts: map[string]int{}}}
	for _, u := range []string{"http://qbit:8080", "http://qbit2:8080"} {
		r.fillEmptyMaps("k", &DownloaderPlan{Adapter: "qbittorrent", URL: u}, maps)
	}
	var a, b domain.Downloader
	_ = database.Get(ctx, l.st.Read(), "downloaders", empty, &a)
	_ = database.Get(ctx, l.st.Read(), "downloaders", set, &b)
	if len(a.PathMap) != 1 || a.PathMap[0].Ours != "/data/downloads/" {
		t.Fatalf("empty maps became %+v", a.PathMap)
	}
	if len(b.PathMap) != 1 || b.PathMap[0].Ours != "/mine/" {
		t.Fatalf("set maps changed to %+v", b.PathMap)
	}
	// Undo takes the filled-in maps away again; the set ones stay.
	if _, err := l.mv.Undo(ctx, move, by); err != nil {
		t.Fatal(err)
	}
	_ = database.Get(ctx, l.st.Read(), "downloaders", empty, &a)
	_ = database.Get(ctx, l.st.Read(), "downloaders", set, &b)
	if len(a.PathMap) != 0 || len(b.PathMap) != 1 {
		t.Fatalf("after undo %+v / %+v", a.PathMap, b.PathMap)
	}
}

// Read one at a time (as the wizard does): the second app's folder maps
// still reach the qBittorrent the first app brought, when it has none.
func TestBringDownloadersOneAppAtATime(t *testing.T) {
	l := newMoveLabWith(t, func(radarr, _, _ *Snapshot) {
		radarr.RemotePathMappings = []RemotePathMapping{{Host: "qbit", RemotePath: "/downloads/", LocalPath: "/data/downloads/"}}
	})
	ctx := context.Background()
	by := domain.Actor{Kind: "user", ID: 1}
	sonarr := &Snapshot{Kind: Sonarr, Via: ViaAPI, DownloadClients: []Provider{
		{Name: "qBittorrent", Kind: "QBittorrent", Enabled: true, Priority: 1, Settings: map[string]any{"host": "qbit", "port": int64(8080), "tvCategory": "tv"}, NeedsReentry: []string{"password"}}}}
	sonarr.finish()
	saved, err := Save(ctx, l.st, 1, sonarr)
	if err != nil {
		t.Fatal(err)
	}
	first, err := l.mv.BringDownloaders(ctx, []int64{saved.ID}, by)
	if err != nil || len(first) != 1 || first[0].Result != ResCreated {
		t.Fatalf("sonarr bring %+v %v", first, err)
	}
	second, err := l.mv.BringDownloaders(ctx, []int64{l.snaps[0]}, by)
	if err != nil || len(second) != 1 || second[0].ID != first[0].ID {
		t.Fatalf("radarr bring %+v %v", second, err)
	}
	var q domain.Downloader
	if err := database.Get(ctx, l.st.Read(), "downloaders", first[0].ID, &q); err != nil {
		t.Fatal(err)
	}
	if len(q.PathMap) != 1 || q.PathMap[0].Ours != "/data/downloads" {
		t.Fatalf("folder maps %+v, want Radarr's", q.PathMap)
	}
}

// A client whose key or password the old app hides says so, with the client
// kind, so the row can ask for it once; one with nothing hidden asks nothing.
func TestBringSaysWhatTheOldAppHid(t *testing.T) {
	l := newMoveLab(t)
	ctx := context.Background()
	sonarr := &Snapshot{Kind: Sonarr, Via: ViaAPI, DownloadClients: []Provider{
		{Name: "SABnzbd", Kind: "Sabnzbd", Enabled: true, Priority: 1, Settings: map[string]any{"host": "sab.invalid", "port": int64(8080)}, NeedsReentry: []string{"apiKey"}},
		{Name: "Transmission", Kind: "Transmission", Enabled: true, Priority: 1, Settings: map[string]any{"host": "tr.invalid", "port": int64(9091)}},
	}}
	sonarr.finish()
	saved, err := Save(ctx, l.st, 1, sonarr)
	if err != nil {
		t.Fatal(err)
	}
	got, err := l.mv.BringDownloaders(ctx, []int64{saved.ID}, domain.Actor{Kind: "user", ID: 1})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][2]string{"SABnzbd": {"sabnzbd", "key"}, "Transmission": {"transmission", ""}}
	if len(got) != 2 {
		t.Fatalf("brought %+v", got)
	}
	for _, b := range got {
		if w := want[b.Name]; b.Kind != w[0] || b.Needs != w[1] {
			t.Fatalf("%s: kind %q needs %q, want %q %q", b.Name, b.Kind, b.Needs, w[0], w[1])
		}
	}
}
