package mediaservers

import (
	"context"
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// fakePlexAll plays a Plex server with a movie, a TV and a music library
// under /data, as the server sees them, and records every refresh.
func fakePlexAll(t *testing.T) (*httptest.Server, func() []call) {
	var mu sync.Mutex
	var calls []call
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Plex-Token") != tok {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/library/sections" {
			_, _ = io.WriteString(w, `<MediaContainer size="3">`+
				`<Directory key="1" type="movie" title="Movies"><Location id="1" path="/data/movies"/></Directory>`+
				`<Directory key="2" type="show" title="TV"><Location id="2" path="/data/tv"/></Directory>`+
				`<Directory key="3" type="artist" title="Music"><Location id="3" path="/data/music"/></Directory>`+
				`<Directory key="4" type="photo" title="Photos"><Location id="4" path="/data/photos"/></Directory></MediaContainer>`)
			return
		}
		mu.Lock()
		calls = append(calls, call{method: r.Method, path: r.URL.Path, query: r.URL.RawQuery})
		mu.Unlock()
	}))
	t.Cleanup(srv.Close)
	return srv, func() []call { mu.Lock(); defer mu.Unlock(); return append([]call{}, calls...) }
}

// addTitle stores a show or artist in its own library folder under base,
// with one file at rel (relative to the library folder).
func addTitle(t *testing.T, w *world, base, libName, kind, name, folder, rel string) (titleID, slotID int64) {
	t.Helper()
	err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		var pr int64
		_ = tx.QueryRow("SELECT id FROM pick_rules LIMIT 1").Scan(&pr)
		lfKind := "show"
		if kind == "artist" {
			lfKind = "music"
		}
		lf := &mediafiles.LibraryFolder{Path: filepath.Join(base, libName), Kind: lfKind, Label: libName, Health: "ok", SameDiskAs: []int64{}}
		if _, err := database.Insert(tx, "library_folders", lf, domain.System); err != nil {
			return err
		}
		ti := &mediafiles.Title{Kind: kind, GateDriver: "none", Name: name, SortName: strings.ToLower(name), MetadataSource: "local", NewPartsPolicy: "all",
			ReleaseGate: "released", WhyAdded: "by_hand", Status: "running", State: "active", SourceIDs: map[string]string{}, Aliases: []string{}, MusicTypes: []string{}, Artwork: []string{}}
		if _, err := database.Insert(tx, "titles", ti, domain.System); err != nil {
			return err
		}
		titleID = ti.ID
		v := &mediafiles.Version{TitleID: ti.ID, Label: "Main", PickRulesID: pr, LibraryFolderID: lf.ID, FolderName: folder, IsMain: true}
		if _, err := database.Insert(tx, "versions", v, domain.System); err != nil {
			return err
		}
		one := 1
		itemKind := "episode"
		if kind == "artist" {
			itemKind = "track"
		}
		it := &mediafiles.Item{TitleID: ti.ID, Kind: itemKind, Numbers: mediafiles.Numbers{Track: &one}, SourceIDs: map[string]string{}}
		if _, err := database.Insert(tx, "items", it, domain.System); err != nil {
			return err
		}
		sl := &mediafiles.Slot{ItemID: it.ID, VersionID: v.ID, State: mediafiles.SlotInLibrary}
		if _, err := database.Insert(tx, "slots", sl, domain.System); err != nil {
			return err
		}
		slotID = sl.ID
		f := &mediafiles.File{SlotIDs: []int64{sl.ID}, VersionID: v.ID, RelativePath: rel, Languages: []string{}, MediaInfo: map[string]any{}, Origin: "imported",
			PlacedBy: "hardlink", Role: "main", State: "present"}
		if _, err := database.Insert(tx, "files", f, domain.System); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE slots SET file_id = ? WHERE id = ?", f.ID, sl.ID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return titleID, slotID
}

// A show episode import, a music album import, a show rename and an
// episode delete each send Plex one scan of the right library, only the
// changed folder, under the server's own path.
func TestPlexShowsAndMusicPartialScan(t *testing.T) {
	w := newWorld(t)
	base := filepath.Dir(w.lib) // movies, tv and music sit side by side
	show, epSlot := addTitle(t, w, base, "tv", "show", "Copper Valley", "Copper Valley (2025)", "Copper Valley (2025)/Season 01/Copper Valley - S01E01.mkv")
	artist, trackSlot := addTitle(t, w, base, "music", "artist", "Lab Lanterns", "Lab Lanterns", "Lab Lanterns/Small Hours (2025)/01 - A.flac")
	srv, calls := fakePlexAll(t)
	ctx := context.Background()
	if _, err := Add(ctx, w.s, w.v, Input{Name: "Plex", Kind: "plex", URL: srv.URL, Token: tok, PathFrom: base, PathTo: "/data"}, domain.System); err != nil {
		t.Fatal(err)
	}
	l := &Linker{Store: w.s, Vault: w.v, Delay: time.Hour}
	l.Watch(ctx)
	showDir := filepath.Join(base, "tv", "Copper Valley (2025)")
	albumDir := filepath.Join(base, "music", "Lab Lanterns", "Small Hours (2025)")
	for _, c := range []struct {
		name, kind string
		title      int64
		data       map[string]any
		folder     string
		change     string
		section    string
		path       string
	}{
		{"show episode import", "import_done", show, map[string]any{"placed_slot_ids": []any{float64(epSlot)}}, showDir, Created, "2", "/data/tv/Copper Valley (2025)"},
		{"music album import", "import_done", artist, map[string]any{"placed_slot_ids": []any{float64(trackSlot)}}, albumDir, Created, "3", "/data/music/Lab Lanterns/Small Hours (2025)"},
		{"show rename", domain.EventRenamed, show, nil, showDir, Modified, "2", "/data/tv/Copper Valley (2025)"},
		{"episode delete", domain.EventEpisodesRemoved, show, map[string]any{"delete_files": true}, showDir, Deleted, "2", "/data/tv/Copper Valley (2025)"},
	} {
		t.Run(c.name, func(t *testing.T) {
			before := len(calls())
			title := c.title
			if err := w.s.AddEvent(ctx, &domain.Event{Kind: c.kind, TitleID: &title, Summary: c.kind, Data: c.data}); err != nil {
				t.Fatal(err)
			}
			waitPending(t, l, c.folder, c.change)
			l.Flush(ctx)
			cs := calls()[before:]
			if len(cs) != 1 || cs[0].path != "/library/sections/"+c.section+"/refresh" {
				t.Fatalf("calls %+v", cs)
			}
			q, _ := url.ParseQuery(cs[0].query)
			if q.Get("path") != c.path {
				t.Fatalf("path %q, want %q", q.Get("path"), c.path)
			}
		})
	}
	// A show folder no library holds scans the TV library whole, never the
	// movie or music ones.
	l.queue(ctx, "/elsewhere/Other Show", Created, "show")
	before := len(calls())
	l.Flush(ctx)
	if cs := calls()[before:]; len(cs) != 1 || cs[0].path != "/library/sections/2/refresh" || cs[0].query != "" {
		t.Fatalf("fallback %+v", cs)
	}
	// The test button names every library HolyArrs can tell about.
	if res, err := l.Test(ctx, mustGet(t, w, 1)); err != nil || res != "Libraries: Movies, TV, Music" {
		t.Fatalf("test %q %v", res, err)
	}
}
