package migrate

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// b2-1 (decision 32, A-lite): the old apps' path rewrites for a download
// client (host, the path as the client reports it, the same folder as the
// app sees it) move onto the matching Downloader as its folder maps. They
// are keyed by host (any case), both sides must be absolute, a clash
// between two apps writes neither pair, and a folder inside HolyArrs's own
// config folder is refused.
//
// The Snapshot and the plan are reached through their JSON here
// ("remote_path_mappings" with host, remote_path and local_path; a
// Downloader's "path_map" of theirs and ours), so the tests build before
// the fix and fail on what is missing.

type rpm struct {
	Host   string `json:"host"`
	Remote string `json:"remote_path"`
	Local  string `json:"local_path"`
}

// withMappings puts path rewrites on s through its JSON.
func withMappings(t *testing.T, s *Snapshot, rows ...rpm) {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	m["remote_path_mappings"] = rows
	if b, err = json.Marshal(m); err != nil {
		t.Fatal(err)
	}
	*s = Snapshot{}
	if err := json.Unmarshal(b, s); err != nil {
		t.Fatal(err)
	}
}

// mappingsOf reads the path rewrites a Snapshot holds, through its JSON.
func mappingsOf(t *testing.T, s *Snapshot) []rpm {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Rows []rpm `json:"remote_path_mappings"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out.Rows
}

// mapsOf reads a planned Downloader's folder maps, through its JSON.
func mapsOf(t *testing.T, d *DownloaderPlan) []domain.PathMap {
	t.Helper()
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Maps []domain.PathMap `json:"path_map"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out.Maps
}

func trim(p string) string {
	if len(p) > 1 {
		return strings.TrimRight(p, `/\`)
	}
	return p
}

func hasMap(maps []domain.PathMap, theirs, ours string) bool {
	for _, m := range maps {
		if trim(m.Theirs) == trim(theirs) && trim(m.Ours) == trim(ours) {
			return true
		}
	}
	return false
}

func hasTheirs(maps []domain.PathMap, theirs string) bool {
	for _, m := range maps {
		if trim(m.Theirs) == trim(theirs) {
			return true
		}
	}
	return false
}

// mentions says whether the plan lists part anywhere a person reads it: a
// Not moved line or an item's detail.
func mentions(p *Plan, part string) bool {
	for _, n := range p.NotMoved {
		if strings.Contains(n.What+" "+n.Why+" "+n.From, part) {
			return true
		}
	}
	for _, g := range p.Groups {
		for _, it := range g.Items {
			if strings.Contains(it.Detail, part) || strings.Contains(it.Needs, part) {
				return true
			}
		}
	}
	return false
}

const qbitKey = "downloader:http://qbit:8080"

func qbitClient() Provider {
	return Provider{Name: "qBit", Kind: "QBittorrent", Enabled: true, Priority: 1,
		Settings: map[string]any{"host": "qbit", "port": int64(8080), "username": "admin"}, NeedsReentry: []string{"password"}}
}

func arrWithClient(t *testing.T, k Kind, rows ...rpm) *Snapshot {
	t.Helper()
	s := &Snapshot{Kind: k, DownloadClients: []Provider{qbitClient()}}
	withMappings(t, s, rows...)
	s.finish()
	return s
}

func qbitMaps(t *testing.T, p *Plan) []domain.PathMap {
	t.Helper()
	return mapsOf(t, item(t, p, GroupDownloaders, qbitKey).Downloader)
}

// The API and a backup both carry the rewrites into the Snapshot.
func TestRemotePathMappingsRead(t *testing.T) {
	u, key := radarrAPIWith(t, map[string]any{
		"/api/v3/remotepathmapping": []any{map[string]any{"id": 1, "host": "qbit", "remotePath": "/downloads/", "localPath": "/data/downloads/"}},
	})
	if rows := mappingsOf(t, read(t, api(Radarr, u, key))); len(rows) != 1 || rows[0].Host != "qbit" || trim(rows[0].Remote) != "/downloads" || trim(rows[0].Local) != "/data/downloads" {
		t.Fatalf("from the API: %+v", rows)
	}
	db := append(radarrDB(),
		`CREATE TABLE RemotePathMappings (Id INTEGER PRIMARY KEY, Host TEXT, RemotePath TEXT, LocalPath TEXT)`,
		`INSERT INTO RemotePathMappings VALUES (1, 'qbit', '/downloads/', '/data/downloads/')`)
	plain := filepath.Join(t.TempDir(), "radarr.db")
	makeDB(t, plain, db...)
	if rows := mappingsOf(t, read(t, file(Radarr, plain))); len(rows) != 1 || rows[0].Host != "qbit" || trim(rows[0].Remote) != "/downloads" || trim(rows[0].Local) != "/data/downloads" {
		t.Fatalf("from a backup: %+v", rows)
	}
}

// A rewrite for the client's host (any case) lands on that Downloader; one
// for another host does not.
func TestPlanMovesPathMappingsByHost(t *testing.T) {
	s := arrWithClient(t, Sonarr,
		rpm{Host: "QBit", Remote: "/downloads/", Local: "/data/downloads/"},
		rpm{Host: "sab", Remote: "/incomplete", Local: "/data/usenet"})
	p := BuildPlan([]Input{{1, s}}, Choices{})
	maps := qbitMaps(t, p)
	if !hasMap(maps, "/downloads", "/data/downloads") {
		t.Fatalf("maps %+v, want /downloads -> /data/downloads", maps)
	}
	if hasTheirs(maps, "/incomplete") {
		t.Fatalf("another host's rewrite landed here: %+v", maps)
	}
}

// Rows are untrusted: a relative side, a ".." part or an empty host is
// never written. A Windows client path is absolute too.
func TestPlanPathMappingsCheckedAsUntrusted(t *testing.T) {
	s := arrWithClient(t, Radarr,
		rpm{Host: "qbit", Remote: "downloads", Local: "/data/a"},
		rpm{Host: "qbit", Remote: "/b", Local: "data/b"},
		rpm{Host: "qbit", Remote: "/c/../etc", Local: "/data/c"},
		rpm{Host: "qbit", Remote: "/d", Local: "/data/../etc"},
		rpm{Host: "", Remote: "/e", Local: "/data/e"},
		rpm{Host: "qbit", Remote: `D:\Downloads\`, Local: "/data/windows"})
	p := BuildPlan([]Input{{1, s}}, Choices{})
	maps := qbitMaps(t, p)
	for _, bad := range []string{"downloads", "/b", "/c/../etc", "/d", "/e"} {
		if hasTheirs(maps, bad) {
			t.Errorf("unsafe rewrite %q written: %+v", bad, maps)
		}
	}
	if !hasMap(maps, `D:\Downloads`, "/data/windows") {
		t.Fatalf("Windows client path not moved: %+v", maps)
	}
}

// The same client in Sonarr and Radarr is one Downloader with the rewrites
// of both; a pair both hold is written once. Longest client path first.
func TestPlanMergesPathMappingsAcrossApps(t *testing.T) {
	sonarr := arrWithClient(t, Sonarr,
		rpm{Host: "qbit", Remote: "/downloads", Local: "/data/downloads"},
		rpm{Host: "qbit", Remote: "/downloads/tv", Local: "/mnt/tv"})
	radarr := arrWithClient(t, Radarr,
		rpm{Host: "qbit", Remote: "/downloads", Local: "/data/downloads"},
		rpm{Host: "qbit", Remote: "/movies-dl", Local: "/data/movies-dl"})
	p := BuildPlan([]Input{{1, sonarr}, {2, radarr}}, Choices{})
	maps := qbitMaps(t, p)
	if len(maps) != 3 || !hasMap(maps, "/downloads", "/data/downloads") || !hasMap(maps, "/downloads/tv", "/mnt/tv") || !hasMap(maps, "/movies-dl", "/data/movies-dl") {
		t.Fatalf("merged maps %+v", maps)
	}
	tv, dl := -1, -1
	for i, m := range maps {
		switch trim(m.Theirs) {
		case "/downloads/tv":
			tv = i
		case "/downloads":
			dl = i
		}
	}
	if tv > dl {
		t.Fatalf("the longer client path must come first: %+v", maps)
	}
}

// Two apps that send the same client path to different folders: neither
// pair is written, and the plan says so.
func TestPlanPathMappingClashWritesNeither(t *testing.T) {
	sonarr := arrWithClient(t, Sonarr,
		rpm{Host: "qbit", Remote: "/downloads", Local: "/data/tv-downloads"},
		rpm{Host: "qbit", Remote: "/seed", Local: "/data/seed"})
	radarr := arrWithClient(t, Radarr, rpm{Host: "qbit", Remote: "/downloads/", Local: "/data/movie-downloads"})
	p := BuildPlan([]Input{{1, sonarr}, {2, radarr}}, Choices{})
	maps := qbitMaps(t, p)
	if hasTheirs(maps, "/downloads") {
		t.Fatalf("a clashing pair was written: %+v", maps)
	}
	if !hasMap(maps, "/seed", "/data/seed") {
		t.Fatalf("the pair without a clash is lost: %+v", maps)
	}
	if !mentions(p, "/downloads") {
		t.Fatalf("the clash is not listed: %+v", p.NotMoved)
	}
}

// At most 100 folder maps per Downloader.
func TestPlanPathMappingsCapped(t *testing.T) {
	var rows []rpm
	for i := range 150 {
		rows = append(rows, rpm{Host: "qbit", Remote: fmt.Sprintf("/dl/%03d", i), Local: fmt.Sprintf("/data/dl/%03d", i)})
	}
	p := BuildPlan([]Input{{1, arrWithClient(t, Sonarr, rows...)}}, Choices{})
	if maps := qbitMaps(t, p); len(maps) == 0 || len(maps) > 100 {
		t.Fatalf("%d maps, want 1 to 100", len(maps))
	}
}

// addSonarr saves one more Snapshot (Sonarr, the same qBittorrent) with
// rewrites into the lab.
func (l *moveLab) addSonarr(t *testing.T, rows ...rpm) {
	t.Helper()
	s := &Snapshot{Kind: Sonarr, Version: "4.0.17", Via: ViaAPI, DownloadClients: []Provider{qbitClient()}}
	withMappings(t, s, rows...)
	s.finish()
	saved, err := Save(context.Background(), l.st, 1, s)
	if err != nil {
		t.Fatal(err)
	}
	l.snaps = append(l.snaps, saved.ID)
}

func (l *moveLab) downloaderMaps(t *testing.T) []domain.PathMap {
	t.Helper()
	var raw string
	if err := l.st.Read().QueryRow("SELECT path_map FROM downloaders WHERE url = 'http://qbit:8080' AND removed = 0").Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var maps []domain.PathMap
	if err := json.Unmarshal([]byte(raw), &maps); err != nil {
		t.Fatalf("path_map %q: %v", raw, err)
	}
	return maps
}

// The move writes the rewrites onto the new Downloader, but never one into
// HolyArrs's own config folder.
func TestMoveWritesPathMappingsButNotIntoConfig(t *testing.T) {
	l := newMoveLab(t)
	local := t.TempDir()
	inConfig := filepath.Join(l.st.Dir(), "downloads")
	if err := os.MkdirAll(inConfig, 0o755); err != nil {
		t.Fatal(err)
	}
	l.addSonarr(t,
		rpm{Host: "qbit", Remote: "/downloads", Local: filepath.ToSlash(local)},
		rpm{Host: "qbit", Remote: "/sneaky", Local: filepath.ToSlash(inConfig)},
		rpm{Host: "qbit", Remote: "/config-root", Local: filepath.ToSlash(l.st.Dir())})
	mv := l.move(t, Choices{})
	if mv.State != "done" || mv.Report == nil {
		t.Fatalf("move %+v", mv)
	}
	if got := lineOf(t, mv.Report, qbitKey); got.Result != ResCreated {
		t.Fatalf("downloader line %+v", got)
	}
	maps := l.downloaderMaps(t)
	if !hasMap(maps, "/downloads", filepath.ToSlash(local)) && !hasMap(maps, "/downloads", local) {
		t.Fatalf("maps %+v, want /downloads -> %s", maps, local)
	}
	if hasTheirs(maps, "/sneaky") || hasTheirs(maps, "/config-root") {
		t.Fatalf("a folder map into the config folder was written: %+v", maps)
	}
}

// When the Downloader is already here, nothing is changed on it, and the
// line says which folder maps to add by hand.
func TestMoveNotesPathMappingsWhenDownloaderAlreadyHere(t *testing.T) {
	l := newMoveLab(t)
	local := t.TempDir()
	// Set up by hand before the move.
	if err := l.st.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := database.Insert(tx, "downloaders", &domain.Downloader{Name: "My qBit", Adapter: "qbittorrent", Protocol: "torrent", URL: "http://qbit:8080",
			Category: "holyarrs", Priority: 1, PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok"}, domain.System)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	l.addSonarr(t, rpm{Host: "qbit", Remote: "/downloads", Local: filepath.ToSlash(local)})
	mv := l.move(t, Choices{})
	if mv.State != "done" || mv.Report == nil {
		t.Fatalf("move %+v", mv)
	}
	got := lineOf(t, mv.Report, qbitKey)
	if got.Result != ResAlready || !strings.Contains(got.Detail, "/downloads") {
		t.Fatalf("downloader line %+v, want already with a note naming /downloads", got)
	}
	if maps := l.downloaderMaps(t); len(maps) != 0 {
		t.Fatalf("an existing Downloader was changed: %+v", maps)
	}
}

// The app's side of a rewrite is the old app's own view of the folder. It
// goes through the owner's path rules like the library folders do, so the
// Downloader gets the folder as HolyArrs sees it.
func TestPlanPathMappingsFollowPathRules(t *testing.T) {
	s := arrWithClient(t, Sonarr, rpm{Host: "qbit", Remote: "/downloads", Local: "/downloads"})
	p := BuildPlan([]Input{{1, s}}, Choices{PathRules: []PathRule{{From: "/downloads", To: "/data/downloads"}}})
	if maps := qbitMaps(t, p); !hasMap(maps, "/downloads", "/data/downloads") {
		t.Fatalf("maps %+v, want /downloads -> /data/downloads through the path rule", maps)
	}
}

// A folder map whose folder HolyArrs can't see is still written, and the
// line says so, so the owner checks the mounts before imports fail. The
// same note goes on a Downloader that is already here.
func TestMoveNotesFolderMapsNotVisible(t *testing.T) {
	for _, already := range []bool{false, true} {
		l := newMoveLab(t)
		if already {
			if err := l.st.Write(context.Background(), func(tx *sql.Tx) error {
				_, err := database.Insert(tx, "downloaders", &domain.Downloader{Name: "My qBit", Adapter: "qbittorrent", Protocol: "torrent", URL: "http://qbit:8080",
					Category: "holyarrs", Priority: 1, PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok"}, domain.System)
				return err
			}); err != nil {
				t.Fatal(err)
			}
		}
		seen := filepath.ToSlash(t.TempDir())
		gone := filepath.ToSlash(filepath.Join(t.TempDir(), "not-mounted"))
		l.addSonarr(t, rpm{Host: "qbit", Remote: "/downloads", Local: seen}, rpm{Host: "qbit", Remote: "/seed", Local: gone})
		mv := l.move(t, Choices{})
		if mv.State != "done" || mv.Report == nil {
			t.Fatalf("move %+v", mv)
		}
		got := lineOf(t, mv.Report, qbitKey)
		at := strings.Index(got.Detail, "not visible here yet")
		if at < 0 || !strings.Contains(got.Detail[at:], gone) || strings.Contains(got.Detail[at:], seen) {
			t.Fatalf("already %v: line %+v, want a note naming only %s", already, got, gone)
		}
		if !already {
			if maps := l.downloaderMaps(t); !hasMap(maps, "/seed", gone) {
				t.Fatalf("the map to a folder not visible yet must still be written: %+v", maps)
			}
		}
	}
}
