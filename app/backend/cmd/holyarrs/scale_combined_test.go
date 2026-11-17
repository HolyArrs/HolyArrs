package main

import (
	"bufio"
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

const combinedPinnedFeed = `<?xml version="1.0"?><rss version="2.0"><channel>
<item><title>Own.Unmatched.Movie.2026.1080p</title><link>http://fixture.invalid/movie.torrent</link><guid>own-unmatched-movie</guid><pubDate>Tue, 13 Oct 2026 12:00:00 +0000</pubDate></item>
<item><title>Own.Unmatched.Show.S01E01.1080p</title><link>http://fixture.invalid/show.torrent</link><guid>own-unmatched-show</guid><pubDate>Tue, 13 Oct 2026 12:00:00 +0000</pubDate></item>
</channel></rss>`

type combinedShape struct {
	Movies          int
	Shows           int
	EpisodesPerShow int
	Feeds           int
}

var combinedScaleShape = combinedShape{Movies: 5000, Shows: 5000, EpisodesPerShow: 10, Feeds: 20}

//go:embed testdata/lab-none.mkv
var combinedMediaFixture []byte

func combinedKnownFiles(shape combinedShape) int {
	return shape.Movies*4/5 + shape.Shows*shape.EpisodesPerShow/2
}

func seedCombinedScale(t *testing.T, s *database.Store, shape combinedShape, feedURL, mediaRoot string) error {
	t.Helper()
	if shape.Movies < 1 || shape.Shows < 1 || shape.EpisodesPerShow < 2 || shape.Feeds < 1 {
		return errors.New("combined scale shape needs movies, shows, at least two episodes, and feeds")
	}
	fixture := filepath.Join(mediaRoot, ".lab-none.mkv")
	if err := os.MkdirAll(mediaRoot, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(fixture, combinedMediaFixture, 0o644); err != nil {
		return err
	}
	return s.Write(context.Background(), func(tx *sql.Tx) error {
		now := database.FormatTime(time.Now().UTC())
		actor := `{"kind":"system"}`
		var rules int64
		if err := tx.QueryRow("SELECT id FROM pick_rules ORDER BY id LIMIT 1").Scan(&rules); err != nil {
			return err
		}
		folders := map[string]struct {
			id   int64
			path string
		}{}
		for _, kind := range []string{"movie", "show"} {
			folderPath := filepath.Join(mediaRoot, kind)
			if err := os.MkdirAll(folderPath, 0o755); err != nil {
				return err
			}
			res, err := tx.Exec(`INSERT INTO library_folders (created_at, created_by, updated_at, updated_by, path, kind, label, health, same_disk_as) VALUES (?, ?, ?, ?, ?, ?, ?, 'ok', '[]')`, now, actor, now, actor, folderPath, kind, strings.Title(kind))
			if err != nil {
				return err
			}
			id, err := res.LastInsertId()
			if err != nil {
				return err
			}
			folders[kind] = struct {
				id   int64
				path string
			}{id: id, path: folderPath}
		}

		titleStmt, err := tx.Prepare(`INSERT INTO titles (created_at, created_by, updated_at, updated_by, kind, name, sort_name, year, aliases, source_ids, metadata_source, keep_up_to_date, new_parts_policy, release_gate, music_types, why_added, artwork, status, state, refreshed_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, '[]', ?, 'tmdb', 1, 'all', 'released', '[]', 'by_hand', '[]', ?, 'active', ?)`)
		if err != nil {
			return err
		}
		defer titleStmt.Close()
		versionStmt, err := tx.Prepare(`INSERT INTO versions (created_at, created_by, updated_at, updated_by, title_id, label, pick_rules_id, library_folder_id, folder_name, keep_up_to_date, is_main) VALUES (?, ?, ?, ?, ?, 'HD', ?, ?, ?, 1, 1)`)
		if err != nil {
			return err
		}
		defer versionStmt.Close()
		partStmt, err := tx.Prepare(`INSERT INTO parts (created_at, created_by, updated_at, updated_by, title_id, kind, number, name, keep_up_to_date, source_ids, pressings) VALUES (?, ?, ?, ?, ?, 'season', 1, 'Season 1', 1, '{}', '[]')`)
		if err != nil {
			return err
		}
		defer partStmt.Close()
		itemStmt, err := tx.Prepare(`INSERT INTO items (created_at, created_by, updated_at, updated_by, title_id, part_id, kind, numbers, name, keep_up_to_date, source_ids) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?)`)
		if err != nil {
			return err
		}
		defer itemStmt.Close()
		slotStmt, err := tx.Prepare(`INSERT INTO slots (created_at, created_by, updated_at, updated_by, item_id, version_id, state) VALUES (?, ?, ?, ?, ?, ?, 'wanted')`)
		if err != nil {
			return err
		}
		defer slotStmt.Close()
		fileStmt, err := tx.Prepare(`INSERT INTO files (created_at, created_by, updated_at, updated_by, slot_ids, version_id, relative_path, size, added_at, quality, languages, media_info, release_name, origin, placed_by, role, state) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, '{}', '[]', '{}', ?, 'imported', 'hardlink', 'main', 'present')`)
		if err != nil {
			return err
		}
		defer fileStmt.Close()

		addTitle := func(kind, name, sourceIDs, status string, year int) (int64, int64, string, error) {
			folder := folders[kind]
			titleRes, err := titleStmt.Exec(now, actor, now, actor, kind, name, name, year, sourceIDs, status, now)
			if err != nil {
				return 0, 0, "", err
			}
			titleID, err := titleRes.LastInsertId()
			if err != nil {
				return 0, 0, "", err
			}
			versionRes, err := versionStmt.Exec(now, actor, now, actor, titleID, rules, folder.id, name)
			if err != nil {
				return 0, 0, "", err
			}
			versionID, err := versionRes.LastInsertId()
			if err != nil {
				return 0, 0, "", err
			}
			if err := os.MkdirAll(filepath.Join(folder.path, name), 0o755); err != nil {
				return 0, 0, "", err
			}
			return titleID, versionID, folder.path, nil
		}
		addItem := func(titleID, versionID int64, partID any, kind, numbers, name, sourceIDs string) (int64, error) {
			itemRes, err := itemStmt.Exec(now, actor, now, actor, titleID, partID, kind, numbers, name, sourceIDs)
			if err != nil {
				return 0, err
			}
			itemID, err := itemRes.LastInsertId()
			if err != nil {
				return 0, err
			}
			slotRes, err := slotStmt.Exec(now, actor, now, actor, itemID, versionID)
			if err != nil {
				return 0, err
			}
			return slotRes.LastInsertId()
		}
		addKnownFile := func(folderPath string, versionID, slotID int64, relative string) error {
			destination := filepath.Join(folderPath, filepath.FromSlash(relative))
			if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
				return err
			}
			if err := os.Link(fixture, destination); err != nil {
				return fmt.Errorf("hardlink %s: %w", relative, err)
			}
			release := strings.TrimSuffix(path.Base(relative), path.Ext(relative))
			fileRes, err := fileStmt.Exec(now, actor, now, actor, fmt.Sprintf("[%d]", slotID), versionID, relative, len(combinedMediaFixture), now, release)
			if err != nil {
				return err
			}
			fileID, err := fileRes.LastInsertId()
			if err != nil {
				return err
			}
			_, err = tx.Exec("UPDATE slots SET state = 'in_library', file_id = ?, updated_at = ? WHERE id = ?", fileID, now, slotID)
			return err
		}

		for i := range shape.Movies {
			name := fmt.Sprintf("Fixture Movie %04d", i)
			titleID, versionID, folderPath, err := addTitle("movie", name, fmt.Sprintf(`{"tmdb":"%d"}`, 100000+i), "released", 1960+i%60)
			if err != nil {
				return err
			}
			slotID, err := addItem(titleID, versionID, nil, "movie", "{}", name, "{}")
			if err != nil {
				return err
			}
			if i < shape.Movies*4/5 {
				relative := path.Join(name, fmt.Sprintf("Fixture.Movie.%04d.1080p.mkv", i))
				if err := addKnownFile(folderPath, versionID, slotID, relative); err != nil {
					return err
				}
			}
		}
		for i := range shape.Shows {
			name := fmt.Sprintf("Fixture Show %04d", i)
			titleID, versionID, folderPath, err := addTitle("show", name, fmt.Sprintf(`{"tmdb":"%d"}`, 200000+i), "running", 1990+i%30)
			if err != nil {
				return err
			}
			partRes, err := partStmt.Exec(now, actor, now, actor, titleID)
			if err != nil {
				return err
			}
			partID, err := partRes.LastInsertId()
			if err != nil {
				return err
			}
			for episode := 1; episode <= shape.EpisodesPerShow; episode++ {
				slotID, err := addItem(titleID, versionID, partID, "episode", fmt.Sprintf(`{"season":1,"episode":%d}`, episode), fmt.Sprintf("Episode %d", episode), fmt.Sprintf(`{"tmdb":"%d"}`, 300000+i*shape.EpisodesPerShow+episode))
				if err != nil {
					return err
				}
				if episode <= shape.EpisodesPerShow/2 {
					relative := path.Join(name, "Season 1", fmt.Sprintf("Fixture.Show.%04d.S01E%02d.1080p.mkv", i, episode))
					if err := addKnownFile(folderPath, versionID, slotID, relative); err != nil {
						return err
					}
				}
			}
		}
		for i := range shape.Feeds {
			if _, err := tx.Exec(`INSERT INTO sources (created_at, created_by, updated_at, updated_by, name, protocol, format, url, use_for, kinds, categories, limits, priority, state, extra, allow_local_network) VALUES (?, ?, ?, ?, ?, 'torrent', 'rss', ?, '{"feed":true}', '["movie","tv"]', '{"movie":[2000],"tv":[5000]}', '{}', 25, 'enabled', '{}', 1)`, now, actor, now, actor, fmt.Sprintf("Combined fixture feed %02d", i), fmt.Sprintf("%s/feed/%d", feedURL, i)); err != nil {
				return err
			}
		}
		return nil
	})
}

func addCombinedUnknownFiles(mediaRoot string) error {
	fixture := filepath.Join(mediaRoot, ".lab-none.mkv")
	unknown := []struct {
		kind     string
		relative string
	}{
		{"movie", path.Join("Fixture Movie 0000", "Own.Unmatched.Movie.Extra.mkv")},
		{"show", path.Join("Fixture Show 0000", "Season 1", "Fixture.Show.0000.S01E06.Own.Unknown.mkv")},
	}
	for _, file := range unknown {
		destination := filepath.Join(mediaRoot, file.kind, filepath.FromSlash(file.relative))
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return err
		}
		if err := os.Link(fixture, destination); err != nil {
			return fmt.Errorf("hardlink unknown %s: %w", file.relative, err)
		}
	}
	return nil
}

type scaleLiveStreams struct {
	cancel       context.CancelFunc
	responses    []*http.Response
	wg           sync.WaitGroup
	active       atomic.Int32
	rescanEvents atomic.Int32
}

func openScaleLiveStreams(t *testing.T, app *faultApp, count int) *scaleLiveStreams {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	streams := &scaleLiveStreams{cancel: cancel}
	for i := range count {
		client := newClient()
		status, body, _ := app.call(client, "", http.MethodPost, "/api/v1/auth/login", `{"name":"scale","password":"a long generated passphrase"}`)
		if status != http.StatusOK {
			streams.Close()
			t.Fatalf("live client %d sign-in: %d %s", i, status, body)
		}
		client.Timeout = 0
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, app.base+"/live", nil)
		if err != nil {
			streams.Close()
			t.Fatal(err)
		}
		resp, err := client.Do(req)
		if err != nil {
			streams.Close()
			t.Fatalf("live client %d: %v", i, err)
		}
		if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
			resp.Body.Close()
			streams.Close()
			t.Fatalf("live client %d: %d %s", i, resp.StatusCode, resp.Header.Get("Content-Type"))
		}
		reader := bufio.NewReader(resp.Body)
		first, err := reader.ReadString('\n')
		if err != nil || first != "retry: 3000\n" {
			resp.Body.Close()
			streams.Close()
			t.Fatalf("live client %d initial frame %q: %v", i, first, err)
		}
		blank, err := reader.ReadString('\n')
		if err != nil || blank != "\n" {
			resp.Body.Close()
			streams.Close()
			t.Fatalf("live client %d initial frame terminator %q: %v", i, blank, err)
		}
		streams.responses = append(streams.responses, resp)
		streams.active.Add(1)
		streams.wg.Add(1)
		go func() {
			defer streams.wg.Done()
			defer streams.active.Add(-1)
			scanner := bufio.NewScanner(reader)
			for scanner.Scan() {
				line := scanner.Text()
				if strings.HasPrefix(line, "data: ") && strings.Contains(line, domain.EventLibraryRescanned) {
					streams.rescanEvents.Add(1)
				}
			}
		}()
	}
	return streams
}

func (s *scaleLiveStreams) Count() int { return int(s.active.Load()) }

func (s *scaleLiveStreams) RescanEvents() int { return int(s.rescanEvents.Load()) }

func (s *scaleLiveStreams) Close() {
	if s == nil || s.cancel == nil {
		return
	}
	s.cancel()
	for _, resp := range s.responses {
		_ = resp.Body.Close()
	}
	s.wg.Wait()
	s.cancel = nil
}

func runScaleRescanAPI(t *testing.T, app *faultApp, client *http.Client, csrf string, limit time.Duration) (library.RescanResult, error) {
	t.Helper()
	status, body, _ := app.call(client, csrf, http.MethodPost, "/api/v1/schedules/rescan_library/run", "")
	if status != http.StatusAccepted {
		return library.RescanResult{}, fmt.Errorf("queue rescan: %d %s", status, body)
	}
	var queued struct {
		JobID int64 `json:"job_id"`
	}
	if err := json.Unmarshal([]byte(body), &queued); err != nil || queued.JobID == 0 {
		return library.RescanResult{}, fmt.Errorf("queue rescan response: %w (%s)", err, body)
	}
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		status, body, _ = app.call(client, csrf, http.MethodGet, fmt.Sprintf("/api/v1/jobs/%d", queued.JobID), "")
		if status != http.StatusOK {
			return library.RescanResult{}, fmt.Errorf("poll rescan job: %d %s", status, body)
		}
		var job domain.Job
		if err := json.Unmarshal([]byte(body), &job); err != nil {
			return library.RescanResult{}, err
		}
		switch job.State {
		case "done":
			return library.RescanResult{
				Folders:     intValue(job.Result["folders"]),
				Unreachable: intValue(job.Result["unreachable"]),
				Missing:     intValue(job.Result["missing"]),
				Back:        intValue(job.Result["back"]),
				Found:       intValue(job.Result["found"]),
			}, nil
		case "failed", "cancelled":
			return library.RescanResult{}, fmt.Errorf("rescan job %s", job.State)
		}
		time.Sleep(10 * time.Millisecond)
	}
	return library.RescanResult{}, errors.New("rescan job timed out")
}

func intValue(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	default:
		return 0
	}
}

func TestCombinedScaleFixtureAndAuthenticatedRescan(t *testing.T) {
	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(combinedPinnedFeed)) }))
	defer feed.Close()
	dir := t.TempDir()
	mediaRoot := filepath.Join(dir, "combined-media")
	s, err := database.Open(dir, database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	shape := combinedShape{Movies: 5, Shows: 5, EpisodesPerShow: 10, Feeds: 2}
	if err := seedCombinedScale(t, s, shape, feed.URL, mediaRoot); err != nil {
		_ = s.Close()
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	var control *parts.Control
	reg, addr := startApp(t, config{ConfigDir: dir}, runOpts{parts: func(c *parts.Control) { control = c }})
	app := &faultApp{t: t, base: "http://" + addr, dir: dir, pc: control}
	client := newClient()
	status, body, _ := app.call(client, "", http.MethodPost, "/api/v1/auth/setup", `{"name":"scale","password":"a long generated passphrase"}`)
	if status != http.StatusCreated {
		t.Fatalf("setup: %d %s", status, body)
	}
	var owner struct {
		CSRF string `json:"csrf"`
	}
	if err := json.Unmarshal([]byte(body), &owner); err != nil || owner.CSRF == "" {
		t.Fatalf("setup response: %v %s", err, body)
	}
	startupDeadline := time.Now().Add(30 * time.Second)
	for {
		live, err := scaleLiveJobs(control.Store)
		if err != nil {
			t.Fatal(err)
		}
		if scaleSchedulerPassed(reg) && len(live) == 0 {
			break
		}
		if time.Now().After(startupDeadline) {
			t.Fatalf("combined fixture startup did not settle; schedules passed=%t live jobs=%+v", scaleSchedulerPassed(reg), live)
		}
		time.Sleep(10 * time.Millisecond)
	}
	streams := openScaleLiveStreams(t, app, 2)
	defer streams.Close()
	if err := addCombinedUnknownFiles(mediaRoot); err != nil {
		t.Fatal(err)
	}
	result, err := runScaleRescanAPI(t, app, client, owner.CSRF, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for streams.RescanEvents() != 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	var titles, items, files, folders, sources int
	for query, destination := range map[string]*int{
		"SELECT count(*) FROM titles WHERE kind IN ('movie','show')":                              &titles,
		"SELECT count(*) FROM items WHERE kind IN ('movie','episode')":                            &items,
		"SELECT count(*) FROM files WHERE state IN ('present','missing')":                         &files,
		"SELECT count(*) FROM library_folders WHERE kind IN ('movie','show')":                     &folders,
		`SELECT count(*) FROM sources WHERE kinds = '["movie","tv"]' AND allow_local_network = 1`: &sources,
	} {
		if err := control.Store.Read().QueryRow(query).Scan(destination); err != nil {
			t.Fatal(err)
		}
	}
	if titles != 10 || items != 55 || files != combinedKnownFiles(shape)+2 || folders != 2 || sources != 2 {
		t.Fatalf("combined fixture titles=%d items=%d files=%d folders=%d sources=%d", titles, items, files, folders, sources)
	}
	if result.Folders != 2 || result.Unreachable != 0 || result.Found != 2 {
		t.Fatalf("rescan %+v", result)
	}
	rows, err := control.Store.Read().Query("SELECT relative_path FROM files WHERE origin = 'found' ORDER BY relative_path")
	if err != nil {
		t.Fatal(err)
	}
	var foundPaths []string
	for rows.Next() {
		var relative string
		if err := rows.Scan(&relative); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		foundPaths = append(foundPaths, relative)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	// The show file names an episode that is still empty, so the rescan links it.
	if len(foundPaths) != 1 || foundPaths[0] != "Fixture Movie 0000/Own.Unmatched.Movie.Extra.mkv" {
		t.Fatalf("found paths %q", foundPaths)
	}
	var linked int
	if err := control.Store.Read().QueryRow(`SELECT count(*) FROM files WHERE origin = 'adopted' AND relative_path = 'Fixture Show 0000/Season 1/Fixture.Show.0000.S01E06.Own.Unknown.mkv'`).Scan(&linked); err != nil || linked != 1 {
		t.Fatalf("rescan did not link the S01E06 file: %d %v", linked, err)
	}
	if streams.Count() != 2 || streams.RescanEvents() != 2 {
		t.Fatalf("live clients=%d rescan events=%d", streams.Count(), streams.RescanEvents())
	}
}
