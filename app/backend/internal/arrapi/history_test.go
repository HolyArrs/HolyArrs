package arrapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// The calls a phone app such as Helmarr makes for its History, disk and
// episode-file screens are answered.
func TestPhoneAppLists(t *testing.T) {
	r := newPinRig(t)
	title, items := r.show("Phone Show", "4242", true, 2, func(k int) string { return pinDay(-k) })
	var slot1 int64
	if err := r.st.Read().QueryRow("SELECT id FROM slots WHERE item_id = ?", items[0]).Scan(&slot1); err != nil {
		t.Fatal(err)
	}
	file := r.exec(`INSERT INTO files (created_at, created_by, updated_at, updated_by, version_id, relative_path, added_at, origin, placed_by)
		SELECT ?, '{}', ?, '{}', version_id, 'Phone Show/Season 01/e1.mkv', ?, 'imported', 'move' FROM slots WHERE item_id = ?`, pinStamp, pinStamp, pinStamp, items[0])
	r.exec("UPDATE slots SET state = 'in_library', file_id = ? WHERE item_id = ?", file, items[0])

	// A movie's Event and a routine Event never show on the Sonarr side.
	movie := r.exec(`INSERT INTO titles (created_at, created_by, updated_at, updated_by, kind, name, sort_name, year, source_ids, metadata_source, why_added, state, keep_up_to_date)
		VALUES (?, '{}', ?, '{}', 'movie', 'A Movie', 'a movie', 2020, '{}', 'tmdb', 'import', 'active', 1)`, pinStamp, pinStamp)
	ctx := context.Background()
	for _, e := range []domain.Event{
		{Kind: "download_sent", TitleID: &title, SlotID: &slot1, Summary: "Sent", Data: map[string]any{"release": "Phone.Show.S01E01.1080p.WEB-DL.x264-GRP", "downloader": "qBittorrent"}},
		{Kind: "import_done", TitleID: &title, SlotID: &slot1, Summary: "Imported", Data: map[string]any{"release": "Phone.Show.S01E01.1080p.WEB-DL.x264-GRP"}},
		{Kind: "search_done", TitleID: &title, Summary: "Searched"},
		{Kind: "download_sent", TitleID: &movie, Summary: "Sent", Data: map[string]any{"release": "A.Movie.2020.2160p.BluRay.REMUX-GRP"}},
	} {
		if err := r.st.AddEvent(ctx, &e); err != nil {
			t.Fatal(err)
		}
	}

	st, b := r.get("history?page=1&pageSize=50&sortKey=date&sortDirection=descending&includeSeries=true&includeEpisode=true")
	var page struct {
		Page, PageSize, TotalRecords int
		Records                      []struct {
			EpisodeID, SeriesID int64
			SourceTitle         string
			EventType           string
			Quality             struct{ Quality struct{ Name string } }
			Data                map[string]string
			Series              *struct{ Title string }
			Episode             *struct{ EpisodeNumber int }
		}
	}
	if st != 200 || json.Unmarshal(b, &page) != nil || page.TotalRecords != 2 || len(page.Records) != 2 {
		t.Fatalf("history: %d %s", st, b)
	}
	got := page.Records[1] // oldest last when descending; the grab came first
	if got.EventType != "grabbed" || got.SeriesID != title || got.EpisodeID != items[0] || got.Quality.Quality.Name != "WEBDL-1080p" ||
		got.Data["downloadClient"] != "qBittorrent" || got.Series == nil || got.Series.Title != "Phone Show" || got.Episode == nil || got.Episode.EpisodeNumber != 1 {
		t.Fatalf("grab row: %+v", got)
	}
	if page.Records[0].EventType != "downloadFolderImported" {
		t.Fatalf("import row: %+v", page.Records[0])
	}
	if st, b = r.get("history?eventType=3"); st != 200 || !strings.Contains(string(b), `"totalRecords":1`) {
		t.Fatalf("by event number: %d %s", st, b)
	}

	st, b = r.get("episodefile?seriesId=" + itoa(title))
	var files []struct {
		ID, SeriesID int64
		SeasonNumber int
		RelativePath string
	}
	if st != 200 || json.Unmarshal(b, &files) != nil || len(files) != 1 || files[0].ID != file || files[0].SeriesID != title || files[0].SeasonNumber != 1 {
		t.Fatalf("episodefile: %d %s", st, b)
	}
	if st, _ = r.get("episodefile"); st != 400 {
		t.Fatalf("episodefile without seriesId: %d", st)
	}
	if st, _ = r.get("episodefile?seriesId=999999"); st != 404 {
		t.Fatalf("episodefile of an unknown show: %d", st)
	}

	if st, b = r.get("customfilter"); st != 200 || strings.TrimSpace(string(b)) != "[]" {
		t.Fatalf("customfilter: %d %s", st, b)
	}
	st, b = r.get("diskspace")
	var disks []struct {
		Path                  string
		FreeSpace, TotalSpace int64
	}
	if st != 200 || json.Unmarshal(b, &disks) != nil {
		t.Fatalf("diskspace: %d %s", st, b)
	}
	for _, d := range disks {
		if d.TotalSpace <= 0 || d.FreeSpace < 0 || d.Path == "" {
			t.Fatalf("diskspace row: %+v", d)
		}
	}
	pinGoldenShape(t, "sonarr_history", func() []byte { _, b := r.get("history?pageSize=1"); return b }())
}

// The addresses outside the API a phone app checks first answer as in
// Sonarr, and Bazarr's probe still gets a bare 404.
func TestOutsideAPIAddresses(t *testing.T) {
	r := newPinRig(t)
	for path, want := range map[string]string{
		"/sonarr/ping": `{"status":"OK"}`, "/sonarr/api": `{"current":"v3","deprecated":[]}`,
		"/sonarr/system/status": "302 /login", "/sonarr/api/system/status": "404 ",
	} {
		rec := httptest.NewRecorder()
		r.son.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		got := strings.TrimSpace(rec.Body.String())
		if rec.Code != 200 {
			got = fmt.Sprint(rec.Code, " ", rec.Header().Get("Location"))
		}
		if got != want {
			t.Errorf("%s: got %q, want %q", path, got, want)
		}
	}
}
