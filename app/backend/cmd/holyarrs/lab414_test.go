//go:build lab

package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
)

// Stage 4.14 on the lab, from a throwaway container in the lab network with
// research/lab/downloads at /data/downloads and research/lab/fake-indexer
// (read-write) at HOLYARRS_LAB_INDEXER_DIR:
//
//	HOLYARRS_LAB_SELF=<container> HOLYARRS_LAB_INDEXER_DIR=/srv/fake-indexer-rw go test -tags lab -run 'TestLab(AlbumGrab|BrokenAlbum)' ./cmd/holyarrs/
//
// The lab fake indexer lists "Lab Lanterns - Small Hours (2025)" as FLAC and
// MP3 320 (research/lab/sample-library.json, albumListings). These tests
// make the torrents behind those two listings here (three tiny track files
// each), put them where the fake indexer serves them, and put the payload
// in the lab downloads folder first, so the lab qBittorrent finishes from
// what it already has. Everything runs as the app does: the album search,
// the grab, the Downloader sync, the import, and after a failure the album
// search again. The torrents, payloads and their copies are removed after.
const lab414Cat = "holyarrs-lab414"

const (
	smallFLAC = "Lab Lanterns - Small Hours (2025) [FLAC]"
	smallMP3  = "Lab Lanterns - Small Hours (2025) [MP3 320]"
	// The info hashes the fake indexer lists for them.
	smallFLACHash = "531de518e81c5ffbfb4e1e4f6c76acf4b52d4090"
	smallMP3Hash  = "8a6746d81ce16d22c27c746f32b9cc3edb059e0c"
)

var smallSongs = []string{"Low Lamp", "Tin Roof", "Last Tram"}

// albumFiles are an album's three tiny track files.
func albumFiles(seed byte, ext string) []labFile {
	var out []labFile
	for i, s := range smallSongs {
		out = append(out, labFile{name: fmt.Sprintf("%02d - %s.%s", i+1, s, ext), data: video(seed+byte(i), 200<<10)})
	}
	return out
}

type labAlbum struct {
	s      *database.Store
	srv    *httptest.Server
	m      *downloads.Manager
	music  string
	slots  []int64
	title  int64
	album  int64
	cancel context.CancelFunc
}

// newLabAlbum adds Lab Lanterns with Small Hours (three tracks, all
// wanted) on Lossless first, the lab fake indexer and the lab qBittorrent,
// and runs the job queue wired as the app wires it.
func newLabAlbum(t *testing.T) labAlbum {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	labNoSizeFloors(t, s)
	x := labAlbum{s: s, music: t.TempDir()}
	src := os.Getenv("HOLYARRS_LAB_SOURCE")
	if src == "" {
		src = "http://fake-indexer/api"
	}
	err = s.Write(context.Background(), func(tx *sql.Tx) error {
		var pr int64
		if err := tx.QueryRow("SELECT id FROM pick_rules WHERE name = ?", mediafiles.LosslessFirstName).Scan(&pr); err != nil {
			return err
		}
		lf := &mediafiles.LibraryFolder{Path: x.music, Kind: "music", Label: "Music", Health: "ok", SameDiskAs: []int64{}}
		if _, err := database.Insert(tx, "library_folders", lf, domain.System); err != nil {
			return err
		}
		ti := &mediafiles.Title{Kind: "artist", GateDriver: "none", Name: "Lab Lanterns", SortName: "lab lanterns", Aliases: []string{},
			SourceIDs: map[string]string{"musicbrainz": "lab"}, MetadataSource: "musicbrainz", KeepUpToDate: true, NewPartsPolicy: "all",
			ReleaseGate: "released", MusicTypes: []string{"album"}, WhyAdded: "by_hand", Artwork: []string{}, Status: "running", State: "active"}
		if _, err := database.Insert(tx, "titles", ti, domain.System); err != nil {
			return err
		}
		v := &mediafiles.Version{TitleID: ti.ID, Label: "Main", PickRulesID: pr, LibraryFolderID: lf.ID, FolderName: "Lab Lanterns", KeepUpToDate: true, IsMain: true}
		if _, err := database.Insert(tx, "versions", v, domain.System); err != nil {
			return err
		}
		date := time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)
		album := "album"
		p := &mediafiles.Part{TitleID: ti.ID, Kind: "album", Name: "Small Hours", ReleaseType: &album, ReleaseDate: &date, KeepUpToDate: true,
			Included: true, SourceIDs: map[string]string{}, Pressings: []mediafiles.Pressing{}, SecondaryTypes: []string{}}
		if _, err := database.Insert(tx, "parts", p, domain.System); err != nil {
			return err
		}
		x.title, x.album = ti.ID, p.ID
		for i, name := range smallSongs {
			disc, num, name, length := 1, i+1, name, 4*60_000
			it := &mediafiles.Item{TitleID: ti.ID, PartID: &p.ID, Kind: "track", Numbers: mediafiles.Numbers{Disc: &disc, Track: &num}, Name: &name,
				AirDate: &date, LengthMS: &length, KeepUpToDate: true, SourceIDs: map[string]string{}}
			if _, err := database.Insert(tx, "items", it, domain.System); err != nil {
				return err
			}
			sl := &mediafiles.Slot{ItemID: it.ID, VersionID: v.ID, State: mediafiles.SlotWanted}
			if _, err := database.Insert(tx, "slots", sl, domain.System); err != nil {
				return err
			}
			x.slots = append(x.slots, sl.ID)
		}
		so := &domain.Source{Name: "Lab", Protocol: "torrent", Format: "torznab", URL: src, AllowLocal: true,
			UseFor: domain.SourceUse{AutoSearch: true, HandSearch: true}, Kinds: []string{"music"}, Categories: map[string][]int{"music": {3000}},
			Priority: 25, State: "enabled", Extra: map[string]string{}}
		if _, err := database.Insert(tx, "sources", so, domain.System); err != nil {
			return err
		}
		d := &domain.Downloader{Name: "Lab qBittorrent", Adapter: "qbittorrent", Protocol: "torrent", URL: labQbit(), Category: lab414Cat, Priority: 1,
			PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok", AllowLocal: true}
		_, err := database.Insert(tx, "downloaders", d, domain.System)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	x.cancel = cancel
	q := jobs.New(s, jobs.Options{Workers: 2, BaseBackoff: time.Hour, PollInterval: 50 * time.Millisecond})
	x.m = &downloads.Manager{Store: s, SavePath: filepath.Join("/data/downloads/incomplete", lab414Cat)}
	im := &importer.Importer{Store: s}
	sr := &sources.Searcher{Store: s, Queue: q, OnChosen: func(ctx context.Context, slotID, releaseID int64) {
		_, _, _ = q.Enqueue(ctx, downloads.KindGrab, &domain.Subject{Type: "slot", ID: slotID}, map[string]any{"release_id": releaseID}, fmt.Sprintf("grab:%d", slotID), domain.System)
	}}
	x.m.OnFailed = func(ctx context.Context, slotID int64) {
		if err := sr.QueueAfterFailure(ctx, slotID, domain.System); err != nil {
			t.Errorf("search after a failed download: %v", err)
		}
	}
	x.m.OnFinished = func(ctx context.Context, id int64) {
		_, _, _ = q.Enqueue(ctx, importer.KindImport, &domain.Subject{Type: "download", ID: id}, nil, fmt.Sprintf("import:%d", id), domain.System)
	}
	im.OnRejected = func(ctx context.Context, slotID int64) {
		_, _, _ = q.Enqueue(ctx, jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: slotID}, nil, fmt.Sprintf("search_slot:%d", slotID), domain.System)
	}
	sr.Register(q)
	q.Handle(downloads.KindGrab, x.m.HandleGrab)
	q.Handle(importer.KindImport, im.HandleJob)
	if err := q.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); q.Stop() })
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Join("/data/downloads/incomplete", lab414Cat)) })
	x.srv = httptest.NewServer(routes(buildHandlers(services{Store: s, Metadata: metadata.Lab(), Version: "lab", Downloads: x.m, Queue: q}), config{}))
	t.Cleanup(x.srv.Close)
	return x
}

// serveAlbum makes the torrent behind a listing, puts it where the fake
// indexer serves it (and removes it after), and, when have, puts its
// payload in the lab downloads folder. Its info hash must be the one the
// listing gives (the files are the same bytes every time). It returns it.
func serveAlbum(t *testing.T, slug, name, listed string, files []labFile, have bool) string {
	t.Helper()
	dir := os.Getenv("HOLYARRS_LAB_INDEXER_DIR")
	if dir == "" {
		t.Skip("HOLYARRS_LAB_INDEXER_DIR names the fake indexer's folder (read-write)")
	}
	tor, hash := labTorrent(name, files)
	if hash != listed {
		t.Fatalf("the torrent made for %s has info hash %s; the fake indexer lists %s (research/lab/sample-library.json)", slug, hash, listed)
	}
	p := filepath.Join(dir, slug+".torrent")
	if err := os.WriteFile(p, tor, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(p) })
	if have {
		// Remove afterwards the topmost folder this made (the category
		// folder or the album folder), never one that was there before.
		cat := filepath.Join("/data/downloads/incomplete", lab414Cat)
		for _, d := range []string{cat, filepath.Join(cat, name)} {
			if _, err := os.Stat(d); os.IsNotExist(err) {
				t.Cleanup(func() { _ = os.RemoveAll(d) })
				break
			}
		}
		for _, f := range files {
			fp := filepath.Join("/data/downloads/incomplete", lab414Cat, name, f.name)
			if err := os.MkdirAll(filepath.Dir(fp), 0o777); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(fp, f.data, 0o666); err != nil {
				t.Fatal(err)
			}
		}
	}
	return hash
}

// albumSearch starts the album search as Search now on the album does.
func (x labAlbum) albumSearch(t *testing.T) {
	t.Helper()
	do(t, "POST", x.srv.URL+"/api/v1/titles/"+labItoa(x.title)+"/search", `{"album_id":`+labItoa(x.album)+`}`, 202)
}

// until syncs the Downloader every second until done says so.
func (x labAlbum) until(t *testing.T, what string, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Minute)
	for !done() {
		if time.Now().After(deadline) {
			var jobs, rels []string
			if rows, err := x.s.Read().Query("SELECT kind || ' ' || state || ' ' || COALESCE(result, '') || ' ' || COALESCE(error, '') FROM jobs ORDER BY id"); err == nil {
				for rows.Next() {
					var j string
					_ = rows.Scan(&j)
					jobs = append(jobs, j)
				}
				rows.Close()
			}
			if rows, err := x.s.Read().Query("SELECT name || ' ' || state || ' ' || COALESCE(verdict, '') FROM releases ORDER BY id"); err == nil {
				for rows.Next() {
					var r string
					_ = rows.Scan(&r)
					rels = append(rels, r)
				}
				rows.Close()
			}
			t.Fatalf("timed out waiting for %s; rows %+v; slots %v; jobs %v; releases %v", what, labRows(t, x.srv.URL), x.states(), jobs, rels)
		}
		_, _ = x.m.SyncOnce(context.Background())
		time.Sleep(time.Second)
	}
}

func (x labAlbum) states() []string {
	var out []string
	for _, id := range x.slots {
		var st string
		_ = x.s.Read().QueryRow("SELECT state FROM slots WHERE id = ?", id).Scan(&st)
		out = append(out, st)
	}
	return out
}

func (x labAlbum) placed(t *testing.T, ext string, files []labFile) bool {
	t.Helper()
	for i, s := range smallSongs {
		var st, rel string
		_ = x.s.Read().QueryRow("SELECT s.state, COALESCE(f.relative_path, '') FROM slots s LEFT JOIN files f ON f.id = s.file_id WHERE s.id = ?", x.slots[i]).Scan(&st, &rel)
		want := fmt.Sprintf("Lab Lanterns/Small Hours (2025)/%02d - %s.%s", i+1, s, ext)
		if (st != "in_library" && st != "better_wanted") || rel != want {
			return false
		}
		if b, err := os.ReadFile(filepath.Join(x.music, filepath.FromSlash(rel))); err != nil || string(b) != string(files[i].data) {
			return false
		}
	}
	return true
}

// 4.14.3 done when: a lab album goes from the album search to grabbed,
// finished in the lab qBittorrent and imported track by track.
func TestLabAlbumGrabImports(t *testing.T) {
	x := newLabAlbum(t)
	files := albumFiles(11, "flac")
	hash := serveAlbum(t, "lab-small-hours-flac", smallFLAC, smallFLACHash, files, true)
	labRemove(t, x.m, labQbit(), hash)
	x.albumSearch(t)
	seen := []string{}
	x.until(t, "the album to be in the library", func() bool {
		rows := labRows(t, x.srv.URL)
		if len(rows) > 0 && (len(seen) == 0 || seen[len(seen)-1] != rows[0].Phase) {
			seen = append(seen, rows[0].Phase)
		}
		return x.placed(t, "flac", files)
	})
	rows := labRows(t, x.srv.URL)
	if len(rows) != 1 {
		t.Fatalf("rows %+v", rows)
	}
	x.until(t, "the row to say done", func() bool { return labRows(t, x.srv.URL)[0].Phase == "done" })
	r := labRows(t, x.srv.URL)[0]
	if r.Name != smallFLAC || r.Kind != "artist" || r.Title != "Lab Lanterns" || r.Album != "Small Hours" || r.Covers != "3 tracks" || r.Text != "3 of 3 tracks in the library." {
		t.Fatalf("row %+v", r)
	}
	t.Logf("album through the lab: phases %v, %s, slots %v", seen, r.Text, x.states())
}

// 4.14.1 done when: a broken lab album download ends with the next Release
// grabbed. The FLAC copy has no payload and no peer; Mark failed turns it
// down for every track, the album is searched again and the MP3 320 copy is
// grabbed, finishes and is imported.
func TestLabBrokenAlbumNextGrabbed(t *testing.T) {
	x := newLabAlbum(t)
	broken := albumFiles(11, "flac") // never on disk this time
	next := albumFiles(71, "mp3")
	bh := serveAlbum(t, "lab-small-hours-flac", smallFLAC, smallFLACHash, broken, false)
	nh := serveAlbum(t, "lab-small-hours-mp3", smallMP3, smallMP3Hash, next, true)
	labRemove(t, x.m, labQbit(), bh, nh)
	x.albumSearch(t)
	var first downloads.DownloadRow
	x.until(t, "the FLAC copy to be sent", func() bool {
		rows := labRows(t, x.srv.URL)
		if len(rows) == 1 && rows[0].Name == smallFLAC && rows[0].Active {
			first = rows[0]
			return true
		}
		return false
	})
	for i := 0; i < 5; i++ {
		_, _ = x.m.SyncOnce(context.Background())
		time.Sleep(time.Second)
	}
	if r := labRows(t, x.srv.URL)[0]; r.Progress >= 1 {
		t.Fatalf("the broken album moved: %+v", r)
	}
	do(t, "POST", x.srv.URL+"/api/v1/downloads/"+labItoa(first.ID)+"/mark-failed", "", 200)
	var rs string
	_ = x.s.Read().QueryRow("SELECT state FROM releases WHERE name = ?", smallFLAC).Scan(&rs)
	if rs != "turned_down" {
		t.Fatalf("broken release %s", rs)
	}
	x.until(t, "the MP3 copy to be placed", func() bool { return x.placed(t, "mp3", next) })
	var albums int
	_ = x.s.Read().QueryRow("SELECT count(*) FROM jobs WHERE kind = ?", jobs.KindSearchAlbum).Scan(&albums)
	if albums != 2 {
		t.Fatalf("%d album searches, want the first and one after the failure", albums)
	}
	rows := labRows(t, x.srv.URL)
	t.Logf("broken album replaced: %d rows (%s %s; %s %s), slots %v", len(rows), rows[0].Name, rows[0].Label, rows[1].Name, rows[1].Label, x.states())
}
