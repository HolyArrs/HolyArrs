package arrapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
)

// The deeper calls phone apps such as Helmarr, LunaSea and nzb360 make,
// sent with the paths, queries and bodies those apps send.

// phoneRig is a pinRig whose two sides also search, send and remove.
type phoneRig struct {
	*pinRig
	q      *jobs.Queue
	search *sources.Searcher
	src    int64
}

func newPhoneRig(t *testing.T) *phoneRig {
	r := &phoneRig{pinRig: newPinRig(t)}
	r.q = jobs.New(r.st, jobs.Options{})
	r.search = &sources.Searcher{Store: r.st, Queue: r.q}
	r.src = r.exec(`INSERT INTO sources (created_at, created_by, updated_at, updated_by, name, protocol, format, url, state)
		VALUES (?, '{}', ?, '{}', 'LabTracker', 'torrent', 'torznab', 'http://x', 'enabled')`, pinStamp, pinStamp)
	return r
}

func (r *phoneRig) deps() Deps {
	return Deps{Store: r.st, Queue: r.q, Search: r.search, Now: func() time.Time { return pinClock }, SearchWait: 5 * time.Second,
		Downloads: func() *downloads.Manager { return &downloads.Manager{Store: r.st} }}
}

func (r *phoneRig) call(side, method, path, body string) (int, []byte) {
	r.t.Helper()
	h := Sonarr(r.deps())
	if side == "radarr" {
		h = Radarr(r.deps())
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, "/"+side+"/api/v3/"+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rec, req)
	b, _ := io.ReadAll(rec.Body)
	return rec.Code, b
}

func (r *phoneRig) slotOf(item int64) int64 {
	r.t.Helper()
	var id int64
	if err := r.st.Read().QueryRow("SELECT id FROM slots WHERE item_id = ? ORDER BY id LIMIT 1", item).Scan(&id); err != nil {
		r.t.Fatal(err)
	}
	return id
}

// release stores one judged Release found by job.
func (r *phoneRig) release(job any, name, state string, passed bool, slots []int64, reasons string, info string) int64 {
	js, _ := json.Marshal(slots)
	verdict := fmt.Sprintf(`{"passed":%t,"rank":1,"points":25,"reasons":[%s]}`, passed, reasons)
	parsed := "{}"
	if info != "" {
		parsed = fmt.Sprintf(`{"info_url":%q}`, info)
	}
	return r.exec(`INSERT INTO releases (created_at, created_by, updated_at, updated_by, source_id, name, link_ref, protocol, size, published_at, seeders, peers,
		parsed, matched_slot_ids, verdict, found_by, job_id, state) VALUES (?, '{}', ?, '{}', ?, ?, 'x', 'torrent', 1500000000, ?, 42, 3, ?, ?, ?, 'hand_search', ?, ?)`,
		pinStamp, pinStamp, r.src, name, pinDay(-2), parsed, string(js), verdict, job, state)
}

// worker plays the search worker: each hand search Job that only judges
// gets the Releases found(job) stores, then ends.
func (r *phoneRig) worker(ctx context.Context, found func(job int64, subject string)) {
	go func() {
		for ctx.Err() == nil {
			var id int64
			var subject string
			err := r.st.Read().QueryRow(`SELECT id, subject FROM jobs WHERE state = 'queued' AND kind IN (?, ?)
				AND json_extract(params, '$.choose') = 0 ORDER BY id LIMIT 1`, jobs.KindSearchSlot, jobs.KindSearchSeason).Scan(&id, &subject)
			if err == nil {
				found(id, subject)
				r.exec("UPDATE jobs SET state = 'done', ended_at = ? WHERE id = ?", pinStamp, id)
				continue
			}
			time.Sleep(20 * time.Millisecond)
		}
	}()
}

type phoneRelease struct {
	GUID                    string
	IndexerID               int64
	Indexer, Title, InfoURL string
	Protocol, ReleaseGroup  string
	Approved, Rejected      bool
	TemporarilyRejected     bool
	Rejections              []string
	SeriesID, MovieID       int64
	EpisodeIDs              []int64
	SeasonNumber            *int
	FullSeason              bool
	Seeders, Leechers, Age  int
	Size                    int64
	Quality                 struct{ Quality struct{ Name string } }
	CustomFormatScore       int
	DownloadAllowed         bool
	EpisodeRequested        *bool
}

// Hand search of one episode: a phone app asks GET /release?episodeId=,
// HolyArrs searches, and the answer lists what the search found with its
// verdict and page link. The grab sends a passed Release the app's way and
// refuses a failed one, as the app's Choose button does without force.
func TestPhoneReleaseSearchAndGrab(t *testing.T) {
	r := newPhoneRig(t)
	show, items := r.show("Lab Show", "4242", true, 2, func(k int) string { return pinDay(-k) })
	s1, s2 := r.slotOf(items[0]), r.slotOf(items[1])
	var good, bad, other int64
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	r.worker(ctx, func(job int64, _ string) {
		good = r.release(job, "Lab.Show.S01E01.1080p.WEB-DL.H264-LAB", "judged", true, []int64{s1}, `{"step":"points","rule":"total","effect":"pass","detail":"25 points"}`,
			"https://tracker.example/details/101")
		bad = r.release(job, "Lab.Show.S01E01.720p.HDTV.x264-OLD", "refused", false, []int64{s1}, `{"step":"hard","rule":"min_step","effect":"fail","detail":"720p HDTV is below the lowest step"}`, "")
		other = r.release(job, "Lab.Show.S01E02.1080p.WEB-DL.H264-LAB", "judged", true, []int64{s2}, "", "")
	})

	st, b := r.call("sonarr", "GET", "release?episodeId="+itoa(items[0]), "")
	var rows []phoneRelease
	if st != 200 || json.Unmarshal(b, &rows) != nil || len(rows) != 2 {
		t.Fatalf("release: %d %s", st, b)
	}
	g, f := rows[0], rows[1]
	if g.GUID != guidPrefix+itoa(good) || g.IndexerID != r.src || g.Indexer != "LabTracker" || g.InfoURL != "https://tracker.example/details/101" ||
		!g.Approved || g.Rejected || g.SeriesID != show || len(g.EpisodeIDs) != 1 || g.EpisodeIDs[0] != items[0] || g.Protocol != "torrent" ||
		g.ReleaseGroup != "LAB" || g.Seeders != 42 || g.Leechers != 3 || g.Age != 2 || g.Size != 1500000000 || g.Quality.Quality.Name != "WEBDL-1080p" ||
		g.SeasonNumber == nil || *g.SeasonNumber != 1 || g.FullSeason || g.CustomFormatScore != 25 || !g.DownloadAllowed || g.EpisodeRequested == nil {
		t.Fatalf("passed row: %+v", g)
	}
	if f.GUID != guidPrefix+itoa(bad) || f.Approved || !f.Rejected || len(f.Rejections) != 1 || f.Rejections[0] != "720p HDTV is below the lowest step" || f.InfoURL != "" {
		t.Fatalf("refused row: %+v", f)
	}
	_ = other

	for _, c := range []struct {
		side, body string
		want       int
	}{
		{"sonarr", fmt.Sprintf(`{"guid":%q,"indexerId":%d}`, f.GUID, r.src), 409},             // failed the Pick rules: not sent without force
		{"sonarr", fmt.Sprintf(`{"guid":%q,"indexerId":%d}`, g.GUID, r.src+1), 404},           // another Source's guid
		{"sonarr", fmt.Sprintf(`{"guid":"%s999999","indexerId":%d}`, guidPrefix, r.src), 404}, // gone
		{"sonarr", `{"guid":"https://tracker.example/details/101","indexerId":1}`, 404},       // not one HolyArrs answered
		{"radarr", fmt.Sprintf(`{"guid":%q,"indexerId":%d}`, g.GUID, r.src), 404},             // the other side
		{"sonarr", `not json`, 400},
	} {
		if st, b := r.call(c.side, "POST", "release", c.body); st != c.want {
			t.Fatalf("grab %s %s: %d, want %d: %s", c.side, c.body, st, c.want, b)
		}
	}
	var state string
	_ = r.st.Read().QueryRow("SELECT state FROM slots WHERE id = ?", s1).Scan(&state)
	if state != "wanted" {
		t.Fatalf("a refused grab moved the slot to %s", state)
	}
	st, b = r.call("sonarr", "POST", "release", string(mustJSON(t, g)))
	if st != 200 {
		t.Fatalf("grab: %d %s", st, b)
	}
	_ = r.st.Read().QueryRow("SELECT state FROM slots WHERE id = ?", s1).Scan(&state)
	var rel string
	_ = r.st.Read().QueryRow("SELECT state FROM releases WHERE id = ?", good).Scan(&rel)
	if state != "chosen" || rel != "chosen" {
		t.Fatalf("after grab: slot %s, release %s", state, rel)
	}
	// The slot is taken now: a second pick is refused as the app refuses it.
	if st, b := r.call("sonarr", "POST", "release", fmt.Sprintf(`{"guid":"%s%d","indexerId":%d}`, guidPrefix, other, r.src)); st != 200 {
		t.Fatalf("grab of the other episode: %d %s", st, b)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// The asks that start no search or name nothing known.
func TestPhoneReleaseAsks(t *testing.T) {
	r := newPhoneRig(t)
	show, _ := r.show("Lab Show", "4242", true, 1, func(k int) string { return pinDay(-k) })
	movie := r.movie("Lab Movie", "693134", 1)
	for _, c := range []struct {
		side, path string
		want       int
		body       string
	}{
		{"sonarr", "release", 200, "[]"},
		{"radarr", "release", 200, "[]"},
		{"sonarr", "release?episodeId=999999", 404, ""},
		{"sonarr", "release?seriesId=" + itoa(show), 400, ""},
		{"sonarr", "release?seasonNumber=1", 400, ""},
		{"sonarr", "release?seriesId=999999&seasonNumber=1", 404, ""},
		{"sonarr", "release?seriesId=" + itoa(show) + "&seasonNumber=7", 404, ""},
		{"radarr", "release?movieId=999999", 404, ""},
		{"radarr", "release?movieId=" + itoa(show), 404, ""}, // a show is not a movie
		{"sonarr", "release?seriesId=" + itoa(movie) + "&seasonNumber=1", 404, ""},
	} {
		st, b := r.call(c.side, "GET", c.path, "")
		if st != c.want || c.body != "" && strings.TrimSpace(string(b)) != c.body {
			t.Fatalf("%s %s: %d %s, want %d", c.side, c.path, st, b, c.want)
		}
	}
}

// A season search and a movie search answer what their searches found.
func TestPhoneReleaseSeasonAndMovie(t *testing.T) {
	r := newPhoneRig(t)
	show, items := r.show("Lab Show", "4242", true, 2, func(k int) string { return pinDay(-k) })
	movie := r.movie("Lab Movie", "693134", 1)
	var movieSlot int64
	_ = r.st.Read().QueryRow("SELECT s.id FROM slots s JOIN items i ON i.id = s.item_id WHERE i.title_id = ?", movie).Scan(&movieSlot)
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	r.worker(ctx, func(job int64, subject string) {
		if strings.Contains(subject, `"part"`) {
			r.release(job, "Lab.Show.S01.1080p.WEB-DL.H264-LAB", "judged", true, []int64{r.slotOf(items[0]), r.slotOf(items[1])}, "", "")
			return
		}
		r.release(job, "Lab.Movie.2001.2160p.UHD.BluRay.REMUX-LAB", "held", true, []int64{movieSlot}, "", "https://tracker.example/details/7")
	})
	st, b := r.call("sonarr", "GET", fmt.Sprintf("release?seriesId=%d&seasonNumber=1", show), "")
	var rows []phoneRelease
	if st != 200 || json.Unmarshal(b, &rows) != nil || len(rows) != 1 || !rows[0].FullSeason || len(rows[0].EpisodeIDs) != 2 || rows[0].SeriesID != show {
		t.Fatalf("season: %d %s", st, b)
	}
	st, b = r.call("radarr", "GET", "release?movieId="+itoa(movie), "")
	rows = nil
	if st != 200 || json.Unmarshal(b, &rows) != nil || len(rows) != 1 || rows[0].MovieID != movie || !rows[0].TemporarilyRejected || rows[0].Approved ||
		rows[0].InfoURL != "https://tracker.example/details/7" || rows[0].Quality.Quality.Name != "Remux-2160p" {
		t.Fatalf("movie: %d %s", st, b)
	}
	if strings.Contains(string(b), "episodeIds") || strings.Contains(string(b), "seriesId") {
		t.Fatalf("movie row carries show fields: %s", b)
	}
}

// One episode, its monitor switch through the editor route's path, the
// history of one title, the upgrade list and movie files.
func TestPhoneTitleCalls(t *testing.T) {
	r := newPhoneRig(t)
	show, items := r.show("Lab Show", "4242", true, 2, func(k int) string { return pinDay(-k) })
	s1 := r.slotOf(items[0])
	if st, b := r.call("sonarr", "GET", "episode/"+itoa(items[0]), ""); st != 200 || !strings.Contains(string(b), `"episodeNumber":1`) || !strings.Contains(string(b), `"series":{`) {
		t.Fatalf("episode: %d %s", st, b)
	}
	for _, p := range []string{"episode/999999", "episode/x"} {
		if st, _ := r.call("sonarr", "GET", p, ""); st != 404 {
			t.Fatalf("%s: %d", p, st)
		}
	}
	if st, _ := r.call("sonarr", "PUT", "episode/999999", `{"monitored":false}`); st != 404 {
		t.Fatalf("PUT unknown episode: %d", st)
	}
	if st, _ := r.call("radarr", "GET", "episode/"+itoa(items[0]), ""); st != 404 {
		t.Fatalf("episode on the Radarr side: %d", st)
	}

	// History of one show, one season, one episode, by event type.
	movie := r.movie("Lab Movie", "693134", 1)
	ctx := context.Background()
	r.recordEvents(ctx, show, s1, movie)
	for _, c := range []struct {
		side, path string
		want       int
	}{
		{"sonarr", "history/series?seriesId=" + itoa(show), 2},
		{"sonarr", "history/series?seriesId=" + itoa(show) + "&seasonNumber=1", 2},
		{"sonarr", "history/series?seriesId=" + itoa(show) + "&seasonNumber=2", 0},
		{"sonarr", "history/series?seriesId=" + itoa(show) + "&eventType=1", 1},
		{"sonarr", "history/series?seriesId=" + itoa(show) + "&eventType=grabbed&includeSeries=true&includeEpisode=true", 1},
		{"sonarr", "history/series?seriesId=" + itoa(show) + "&episodeId=" + itoa(items[1]), 0},
		{"radarr", "history/movie?movieId=" + itoa(movie), 1},
	} {
		st, b := r.call(c.side, "GET", c.path, "")
		var rows []map[string]any
		if st != 200 || json.Unmarshal(b, &rows) != nil || len(rows) != c.want {
			t.Fatalf("%s: %d %s, want %d rows", c.path, st, b, c.want)
		}
	}
	for _, c := range []struct{ side, path string }{{"sonarr", "history/series"}, {"radarr", "history/movie"}} {
		if st, _ := r.call(c.side, "GET", c.path, ""); st != 400 {
			t.Fatalf("%s without an id: %d", c.path, st)
		}
	}
	if st, _ := r.call("radarr", "GET", "history/movie?movieId="+itoa(show), ""); st != 404 {
		t.Fatalf("history/movie of a show: %d", st)
	}

	// The upgrade list holds what HolyArrs wants a better copy of.
	r.exec("UPDATE slots SET state = 'better_wanted' WHERE id = ?", s1)
	r.exec("UPDATE slots SET state = 'better_wanted' WHERE item_id IN (SELECT id FROM items WHERE title_id = ?)", movie)
	st, b := r.call("sonarr", "GET", "wanted/cutoff?page=1&pageSize=20&includeSeries=true", "")
	p := decodePage(t, b)
	if st != 200 || p.TotalRecords != 1 || len(p.Records) != 1 || p.Records[0].ID != items[0] || p.SortKey != "episodes.airDateUtc" {
		t.Fatalf("sonarr cutoff: %d %s", st, b)
	}
	st, b = r.call("radarr", "GET", "wanted/cutoff", "")
	p = decodePage(t, b)
	if st != 200 || p.TotalRecords != 1 || p.Records[0].ID != movie {
		t.Fatalf("radarr cutoff: %d %s", st, b)
	}
	if st, b = r.call("sonarr", "GET", "wanted/cutoff?monitored=false", ""); st != 200 || decodePage(t, b).TotalRecords != 0 {
		t.Fatalf("cutoff of the not-kept: %d %s", st, b)
	}

	// Movie files: by movie, by file, one file; neither asks 400.
	if st, b := r.call("radarr", "GET", "moviefile?movieId="+itoa(movie), ""); st != 200 || strings.TrimSpace(string(b)) != "[]" {
		t.Fatalf("moviefile of a movie with no file: %d %s", st, b)
	}
	if st, _ := r.call("radarr", "GET", "moviefile", ""); st != 400 {
		t.Fatalf("moviefile with nothing: %d", st)
	}
	if st, _ := r.call("radarr", "GET", "moviefile/999999", ""); st != 404 {
		t.Fatalf("unknown moviefile: %d", st)
	}
	file := r.exec(`INSERT INTO files (created_at, created_by, updated_at, updated_by, version_id, relative_path, added_at, origin, placed_by, size)
		SELECT ?, '{}', ?, '{}', s.version_id, 'Lab Movie (2001)/Lab.Movie.2001.1080p.BluRay.x264-LAB.mkv', ?, 'imported', 'move', 8000000000
		FROM slots s JOIN items i ON i.id = s.item_id WHERE i.title_id = ?`, pinStamp, pinStamp, pinStamp, movie)
	r.exec("UPDATE slots SET state = 'in_library', file_id = ? WHERE item_id IN (SELECT id FROM items WHERE title_id = ?)", file, movie)
	for _, p := range []string{"moviefile?movieId=" + itoa(movie), "moviefile?movieFileIds=" + itoa(file), "moviefile?movieId=" + itoa(movie) + "&movieId=999999"} {
		st, b := r.call("radarr", "GET", p, "")
		var files []struct {
			ID, MovieID, Size int64
			RelativePath      string
		}
		if st != 200 || json.Unmarshal(b, &files) != nil || len(files) != 1 || files[0].ID != file || files[0].MovieID != movie || files[0].Size != 8000000000 {
			t.Fatalf("%s: %d %s", p, st, b)
		}
	}
	if st, b := r.call("radarr", "GET", "moviefile/"+itoa(file), ""); st != 200 || !strings.Contains(string(b), `"movieId":`+itoa(movie)) {
		t.Fatalf("one moviefile: %d %s", st, b)
	}
	if st, b := r.call("radarr", "GET", "moviefile?movieFileIds=999999", ""); st != 200 || strings.TrimSpace(string(b)) != "[]" {
		t.Fatalf("unknown file ids: %d %s", st, b)
	}
}

// recordEvents stores a grab and an import of the show's first episode
// and a grab of the movie.
func (r *phoneRig) recordEvents(ctx context.Context, show, slot, movie int64) {
	r.t.Helper()
	add := func(kind string, title int64, slot *int64, rel string) {
		e := domain.Event{Kind: kind, TitleID: &title, SlotID: slot, Summary: "x", Data: map[string]any{"release": rel}}
		if err := r.st.AddEvent(ctx, &e); err != nil {
			r.t.Fatal(err)
		}
	}
	add("download_sent", show, &slot, "Lab.Show.S01E01.1080p.WEB-DL.H264-LAB")
	add("import_done", show, &slot, "Lab.Show.S01E01.1080p.WEB-DL.H264-LAB")
	add("download_sent", movie, nil, "Lab.Movie.2001.1080p.BluRay.x264-LAB")
}

// Queue removal takes the download out the way the Downloads page's
// Remove does; the options read like the old apps'. The other side's key
// and unknown rows remove nothing.
func TestPhoneQueueRemove(t *testing.T) {
	for _, c := range []struct {
		query                         string
		deleteFiles, turn, searchMore bool
	}{
		{"", true, false, false},
		{"removeFromClient=false", false, false, false},
		{"removeFromClient=true&blocklist=true", true, true, true},
		{"blocklist=true&skipRedownload=true", true, true, false},
		{"blocklist=TRUE&skipRedownload=false&changeCategory=true", true, true, true},
	} {
		got := removeChoice(httptest.NewRequest("DELETE", "/queue/1?"+c.query, nil))
		if got.DeleteFiles != c.deleteFiles || got.TurnDown != c.turn || got.SearchAgain != c.searchMore {
			t.Errorf("%q: %+v", c.query, got)
		}
	}
	r := newPhoneRig(t)
	_, items := r.show("Lab Show", "4242", true, 1, func(k int) string { return pinDay(-k) })
	client := r.exec(`INSERT INTO downloaders (created_at, created_by, updated_at, updated_by, name, adapter, protocol, url) VALUES (?, '{}', ?, '{}', 'qb', 'qbittorrent', 'torrent', 'http://x')`, pinStamp, pinStamp)
	dl := r.exec(`INSERT INTO downloads (created_at, created_by, updated_at, updated_by, downloader_id, slot_ids, external_id, progress, state, started_by)
		VALUES (?, '{}', ?, '{}', ?, ?, 'abc123', 0.5, 'downloading', '{}')`, pinStamp, pinStamp, client, fmt.Sprintf("[%d]", r.slotOf(items[0])))
	st, b := r.call("sonarr", "GET", "queue", "")
	var page struct{ Records []struct{ ID int64 } }
	if st != 200 || json.Unmarshal(b, &page) != nil || len(page.Records) != 1 {
		t.Fatalf("queue: %d %s", st, b)
	}
	row := page.Records[0].ID
	for _, c := range []struct {
		side, method, path, body string
		want                     int
	}{
		{"radarr", "DELETE", "queue/" + itoa(row), "", 404},
		{"sonarr", "DELETE", "queue/999999000", "", 404},
		{"sonarr", "DELETE", "queue/x", "", 404},
		{"sonarr", "DELETE", "queue/bulk", `{"ids":[]}`, 400},
		{"sonarr", "DELETE", "queue/bulk", fmt.Sprintf(`{"ids":[%d,999999000]}`, row), 404},
	} {
		if st, b := r.call(c.side, c.method, c.path, c.body); st != c.want {
			t.Fatalf("%s %s %s: %d, want %d: %s", c.side, c.method, c.path, st, c.want, b)
		}
	}
	var state string
	_ = r.st.Read().QueryRow("SELECT state FROM downloads WHERE id = ?", dl).Scan(&state)
	if state != "downloading" {
		t.Fatalf("a refused remove changed the download to %s", state)
	}
	if st, b := r.call("sonarr", "DELETE", "queue/"+itoa(row)+"?removeFromClient=true&blocklist=false", ""); st != 200 {
		t.Fatalf("remove: %d %s", st, b)
	}
	_ = r.st.Read().QueryRow("SELECT state FROM downloads WHERE id = ?", dl).Scan(&state)
	if state != "cleaned" {
		t.Fatalf("after remove: %s", state)
	}
	if st, b := r.call("sonarr", "GET", "queue", ""); st != 200 || !strings.Contains(string(b), `"totalRecords":0`) {
		t.Fatalf("queue after remove: %d %s", st, b)
	}
	if st, _ := r.call("sonarr", "DELETE", "queue/"+itoa(row), ""); st != 409 {
		t.Fatalf("second remove: %d", st)
	}
}

// The blocklist is the Turned down list of this side; removing a row
// takes it off that list.
func TestPhoneBlocklist(t *testing.T) {
	r := newPhoneRig(t)
	_, items := r.show("Lab Show", "4242", true, 1, func(k int) string { return pinDay(-k) })
	movie := r.movie("Lab Movie", "693134", 1)
	var movieSlot int64
	_ = r.st.Read().QueryRow("SELECT s.id FROM slots s JOIN items i ON i.id = s.item_id WHERE i.title_id = ?", movie).Scan(&movieSlot)
	ep := r.release(nil, "Lab.Show.S01E01.720p.HDTV.x264-OLD", "turned_down", false, []int64{r.slotOf(items[0])}, "", "")
	r.exec(`UPDATE releases SET verdict = json_set(verdict, '$.turned_down_reason', 'bad audio') WHERE id = ?`, ep)
	mv := r.release(nil, "Lab.Movie.2001.CAM.x264-BAD", "turned_down", false, []int64{movieSlot}, "", "")

	st, b := r.call("sonarr", "GET", "blocklist?page=1&pageSize=20", "")
	var page struct {
		TotalRecords int
		Records      []struct {
			ID, SeriesID         int64
			EpisodeIDs           []int64
			SourceTitle, Message string
			Indexer, Protocol    string
		}
	}
	if st != 200 || json.Unmarshal(b, &page) != nil || page.TotalRecords != 1 || page.Records[0].ID != ep || page.Records[0].Message != "bad audio" ||
		page.Records[0].Indexer != "LabTracker" || len(page.Records[0].EpisodeIDs) != 1 || page.Records[0].EpisodeIDs[0] != items[0] {
		t.Fatalf("sonarr blocklist: %d %s", st, b)
	}
	if st, b := r.call("radarr", "GET", "blocklist", ""); st != 200 || !strings.Contains(string(b), `"totalRecords":1`) || !strings.Contains(string(b), `"movieId":`+itoa(movie)) {
		t.Fatalf("radarr blocklist: %d %s", st, b)
	}
	for _, c := range []struct {
		side, path, body string
		want             int
	}{
		{"radarr", "blocklist/" + itoa(ep), "", 404},
		{"sonarr", "blocklist/999999", "", 404},
		{"sonarr", "blocklist/bulk", fmt.Sprintf(`{"ids":[%d,%d]}`, ep, mv), 404},
		{"sonarr", "blocklist/bulk", `{"ids":[]}`, 400},
		{"sonarr", "blocklist/" + itoa(ep), "", 200},
		{"radarr", "blocklist/bulk", fmt.Sprintf(`{"ids":[%d]}`, mv), 200},
	} {
		if st, b := r.call(c.side, "DELETE", c.path, c.body); st != c.want {
			t.Fatalf("DELETE %s %s: %d, want %d: %s", c.side, c.path, st, c.want, b)
		}
	}
	for _, side := range []string{"sonarr", "radarr"} {
		if st, b := r.call(side, "GET", "blocklist", ""); st != 200 || !strings.Contains(string(b), `"totalRecords":0`) {
			t.Fatalf("%s blocklist after removal: %d %s", side, st, b)
		}
	}
}

// Tasks, the log and the update list answer in the old apps' shapes.
// The log stays empty: its lines are for admins only.
func TestPhoneSystemLists(t *testing.T) {
	r := newPhoneRig(t)
	var kind string
	var id int64
	if err := r.st.Read().QueryRow("SELECT id, kind FROM schedules WHERE enabled = 1 ORDER BY kind LIMIT 1").Scan(&id, &kind); err != nil {
		t.Skip("no schedule rows in a fresh database")
	}
	st, b := r.call("sonarr", "GET", "system/task", "")
	var tasks []struct {
		ID             int64
		Name, TaskName string
		Interval       int64
		LastDuration   string
		NextExecution  *string
	}
	if st != 200 || json.Unmarshal(b, &tasks) != nil || len(tasks) == 0 {
		t.Fatalf("tasks: %d %s", st, b)
	}
	if tasks[0].ID != id || tasks[0].TaskName != taskName(kind) || tasks[0].Interval <= 0 || tasks[0].LastDuration == "" {
		t.Fatalf("task: %+v (kind %s)", tasks[0], kind)
	}
	if st, b := r.call("radarr", "GET", "system/task/"+itoa(id), ""); st != 200 || !strings.Contains(string(b), `"taskName":"`+taskName(kind)+`"`) {
		t.Fatalf("one task: %d %s", st, b)
	}
	if st, _ := r.call("radarr", "GET", "system/task/999999", ""); st != 404 {
		t.Fatalf("unknown task: %d", st)
	}
	if taskName("search_missing") != "SearchMissing" || taskName("feed_check") != "FeedCheck" {
		t.Fatal("task names")
	}
	st, b = r.call("sonarr", "GET", "log?page=1&pageSize=50&sortKey=time&sortDirection=descending", "")
	if st != 200 || !strings.Contains(string(b), `"totalRecords":0`) || !strings.Contains(string(b), `"records":[]`) {
		t.Fatalf("log: %d %s", st, b)
	}
	st, b = r.call("radarr", "GET", "update", "")
	var ups []struct {
		Version           string
		Installed, Latest bool
		Installable       bool
	}
	if st != 200 || json.Unmarshal(b, &ups) != nil || len(ups) != 1 || ups[0].Version != RadarrVersion || !ups[0].Installed || !ups[0].Latest || ups[0].Installable {
		t.Fatalf("update: %d %s", st, b)
	}
}

// Waiting hand searches are capped per caller and for the whole server;
// a caller past its cap gets 429, and a freed slot can be taken again.
func TestPhoneSearchLimit(t *testing.T) {
	l := &searchLimit{perKey: map[domain.Actor]chan struct{}{}, all: make(chan struct{}, serverSearches)}
	key := func(n int64) domain.Actor { return domain.Actor{Kind: "link", ID: n} }
	var held []func()
	for _, c := range []struct {
		who  int64
		want bool
	}{
		{1, true}, {1, true}, {1, false}, // two per key
		{2, true}, {2, true}, {3, true}, {3, true}, {4, true}, {4, true}, // eight in all
		{5, false}, // the server is full
	} {
		done, ok := l.take(key(c.who))
		if ok != c.want {
			t.Fatalf("take for %d: %v, want %v", c.who, ok, c.want)
		}
		if ok {
			held = append(held, done)
		}
	}
	held[0]()
	if _, ok := l.take(key(5)); !ok {
		t.Fatal("a freed slot was not taken again")
	}
	if _, ok := l.take(key(1)); ok {
		t.Fatal("the server cap let a ninth search wait")
	}

	// Through the route: a full key answers 429 and starts no search.
	r := newPhoneRig(t)
	show, _ := r.show("Lab Show", "4242", true, 1, func(k int) string { return pinDay(-k) })
	var fill []func()
	for range perKeySearches {
		done, ok := searchSlots.take(webserver.Actor(context.Background()))
		if !ok {
			t.Fatal("could not fill the slots")
		}
		fill = append(fill, done)
	}
	st, b := r.call("sonarr", "GET", fmt.Sprintf("release?seriesId=%d&seasonNumber=1", show), "")
	for _, f := range fill {
		f()
	}
	var n int
	_ = r.st.Read().QueryRow("SELECT count(*) FROM jobs").Scan(&n)
	if st != 429 || n != 0 {
		t.Fatalf("full: %d %s, %d jobs", st, b, n)
	}
}

// removeFromClient=false keeps the files: the download is not marked for
// file removal. The default removes them.
func TestPhoneQueueRemoveKeepsFiles(t *testing.T) {
	for _, c := range []struct {
		query   string
		pending int
	}{{"removeFromClient=false", 0}, {"", 1}, {"removeFromClient=true", 1}} {
		r := newPhoneRig(t)
		_, items := r.show("Lab Show", "4242", true, 1, func(k int) string { return pinDay(-k) })
		client := r.exec(`INSERT INTO downloaders (created_at, created_by, updated_at, updated_by, name, adapter, protocol, url) VALUES (?, '{}', ?, '{}', 'qb', 'qbittorrent', 'torrent', 'http://x')`, pinStamp, pinStamp)
		dl := r.exec(`INSERT INTO downloads (created_at, created_by, updated_at, updated_by, downloader_id, slot_ids, external_id, progress, state, started_by)
			VALUES (?, '{}', ?, '{}', ?, ?, 'abc123', 0.5, 'downloading', '{}')`, pinStamp, pinStamp, client, fmt.Sprintf("[%d]", r.slotOf(items[0])))
		if st, b := r.call("sonarr", "DELETE", fmt.Sprintf("queue/%d?%s", dl*1000, c.query), ""); st != 200 {
			t.Fatalf("%q: %d %s", c.query, st, b)
		}
		var pending int
		_ = r.st.Read().QueryRow("SELECT removal_pending FROM downloads WHERE id = ?", dl).Scan(&pending)
		if pending != c.pending {
			t.Fatalf("%q: removal_pending %d, want %d", c.query, pending, c.pending)
		}
	}
}

// A download is this side's only when every Slot it holds is: one that
// mixes an episode and a movie is refused on both sides.
func TestPhoneQueueSideAllSlots(t *testing.T) {
	r := newPhoneRig(t)
	_, items := r.show("Lab Show", "4242", true, 1, func(k int) string { return pinDay(-k) })
	movie := r.movie("Lab Movie", "693134", 1)
	var movieSlot int64
	_ = r.st.Read().QueryRow("SELECT s.id FROM slots s JOIN items i ON i.id = s.item_id WHERE i.title_id = ?", movie).Scan(&movieSlot)
	client := r.exec(`INSERT INTO downloaders (created_at, created_by, updated_at, updated_by, name, adapter, protocol, url) VALUES (?, '{}', ?, '{}', 'qb', 'qbittorrent', 'torrent', 'http://x')`, pinStamp, pinStamp)
	n := 0
	add := func(slots string) int64 {
		n++
		return r.exec(`INSERT INTO downloads (created_at, created_by, updated_at, updated_by, downloader_id, slot_ids, external_id, progress, state, started_by)
			VALUES (?, '{}', ?, '{}', ?, ?, ?, 0.5, 'downloading', '{}')`, pinStamp, pinStamp, client, slots, fmt.Sprint("h", n))
	}
	ep, mv := r.slotOf(items[0]), movieSlot
	h := &handler{Deps: r.deps()}
	for _, c := range []struct {
		side, slots string
		want        bool
	}{
		{"sonarr", fmt.Sprintf("[%d]", ep), true},
		{"radarr", fmt.Sprintf("[%d]", mv), true},
		{"sonarr", fmt.Sprintf("[%d,%d]", ep, mv), false},
		{"radarr", fmt.Sprintf("[%d,%d]", ep, mv), false},
		{"radarr", fmt.Sprintf("[%d]", ep), false},
		{"sonarr", "[]", false},
	} {
		h.side = c.side
		got, err := h.downloadOnSide(context.Background(), add(c.slots))
		if err != nil || got != c.want {
			t.Fatalf("%s %s: %v %v, want %v", c.side, c.slots, got, err, c.want)
		}
	}
}
