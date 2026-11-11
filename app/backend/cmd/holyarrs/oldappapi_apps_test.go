package main

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg" // the smaller poster copy is a JPEG
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

// The other apps that read the arr API: Bazarr
// copies the Library for subtitles, Unpackerr polls the download list,
// and Homepage's start-page widgets read counts and lists. Their calls
// and the fields they read come from the Sonarr, Radarr and Bazarr
// manuals.

func (f *oldAppFix) exec(q string, args ...any) {
	f.t.Helper()
	if err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(q, args...)
		return err
	}); err != nil {
		f.t.Fatal(err)
	}
}

func (f *oldAppFix) profile() int64 {
	f.t.Helper()
	return num(arrList(f.t, "profiles", f.arrOK(f.sonarrKey, "GET", "/sonarr/api/v3/qualityprofile", "", 200), -1)[0].(map[string]any)["id"])
}

// addLabShow adds the lab show as Seerr would, with only season 1 kept up
// to date: its two episodes aired in 2025, the special aired but is off,
// season 2 has not aired.
func (f *oldAppFix) addLabShow() int64 {
	f.t.Helper()
	body := fmt.Sprintf(`{"tvdbId":200,"title":"Lab Show","qualityProfileId":%d,"seasons":[{"seasonNumber":0,"monitored":false},
		{"seasonNumber":1,"monitored":true},{"seasonNumber":2,"monitored":false}],"monitored":true,"monitorNewItems":"all",
		"rootFolderPath":%q,"seriesType":"standard"}`, f.profile(), f.showDir)
	return num(f.arrOK(f.sonarrKey, "POST", "/sonarr/api/v3/series", body, 201).(map[string]any)["id"])
}

func (f *oldAppFix) addLabMovie() int64 {
	f.t.Helper()
	body := fmt.Sprintf(`{"tmdbId":693134,"qualityProfileId":%d,"rootFolderPath":%q,"monitored":true,"minimumAvailability":"released"}`, f.profile(), f.movieDir)
	return num(f.arrOK(f.radarrKey, "POST", "/radarr/api/v3/movie", body, 201).(map[string]any)["id"])
}

// putFileAs puts fl in the Library for a Slot, as an import would.
func (f *oldAppFix) putFileAs(sl mediafiles.Slot, fl mediafiles.File) int64 {
	f.t.Helper()
	fl.SlotIDs, fl.VersionID, fl.AddedAt = []int64{sl.ID}, sl.VersionID, time.Now().UTC()
	fl.Origin, fl.PlacedBy, fl.Role, fl.State = "imported", "hardlink", "main", "present"
	if fl.Languages == nil {
		fl.Languages = []string{}
	}
	if fl.MediaInfo == nil {
		fl.MediaInfo = map[string]any{}
	}
	var id int64
	if err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		var err error
		if id, err = database.Insert(tx, "files", &fl, domain.System); err != nil {
			return err
		}
		_, err = tx.Exec("UPDATE slots SET file_id = ?, state = 'in_library', active_download_id = NULL WHERE id = ?", id, sl.ID)
		return err
	}); err != nil {
		f.t.Fatal(err)
	}
	return id
}

// itemOf is the id of a show's episode.
func (f *oldAppFix) itemOf(showID int64, season, ep int) int64 {
	f.t.Helper()
	var id int64
	if err := f.s.Read().QueryRow(`SELECT id FROM items WHERE title_id = ? AND json_extract(numbers, '$.season') = ? AND json_extract(numbers, '$.episode') = ?`,
		showID, season, ep).Scan(&id); err != nil {
		f.t.Fatal(err)
	}
	return id
}

// storePoster puts a 400x600 poster in the artwork cache and on the title.
func (f *oldAppFix) storePoster(titleID int64, from string) []byte {
	f.t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 400, 600))
	for x := 0; x < 400; x++ {
		img.Set(x, x, color.RGBA{200, 20, 20, 255})
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		f.t.Fatal(err)
	}
	name := library.ArtworkName(from)
	if err := os.WriteFile(filepath.Join(f.art.Dir, name), b.Bytes(), 0o644); err != nil {
		f.t.Fatal(err)
	}
	f.exec("UPDATE titles SET artwork = ? WHERE id = ?", `["poster:`+name+`"]`, titleID)
	return b.Bytes()
}

// storeWide puts a wide picture's address on the title and its copy in the
// artwork cache, as if fetched once before.
func (f *oldAppFix) storeWide(titleID int64, from string) []byte {
	f.t.Helper()
	b := []byte("wide picture " + from)
	if err := os.WriteFile(filepath.Join(f.art.Dir, library.ArtworkName(from)), b, 0o644); err != nil {
		f.t.Fatal(err)
	}
	f.exec("UPDATE titles SET backdrop = ? WHERE id = ?", from, titleID)
	return b
}

// TestOldAppBazarrSync makes Bazarr's full sync against both sides: the
// version probe, the folders, every show with its episodes and their
// files, every movie with its file, the tags and profiles, and the rescan
// it sends back after a subtitle change. Fields are the ones the Bazarr
// manual says it copies.
func TestOldAppBazarrSync(t *testing.T) {
	t.Parallel()
	f := newOldAppFix(t)
	f.turnOn()
	son, rad := f.sonarrKey, f.radarrKey
	s, r := "/sonarr/api/v3/", "/radarr/api/v3/"

	// The version probe without /api/v3 answers no version, so Bazarr asks
	// the v3 status, which reports 4 or more.
	// An empty body, as the old apps send: a JSON body reads as no version.
	st, _, b := f.arr(son, "GET", "/sonarr/api/system/status", "")
	if st != 404 || b != "" {
		t.Fatalf("short status: %d %q", st, b)
	}
	if v := f.arrOK(son, "GET", s+"system/status", "", 200).(map[string]any)["version"]; !strings.HasPrefix(v.(string), "4.") {
		t.Fatalf("version %v", v)
	}

	showID := f.addLabShow()
	f.exec(`UPDATE titles SET overview = 'A lab show.', aliases = '["Lab Show Alt"]', original_language = 'en' WHERE id = ?`, showID)
	f.storePoster(showID, "https://image.tmdb.org/t/p/w342/lab-show.png")
	f.storeWide(showID, "https://image.tmdb.org/t/p/w1280/lab-show-wide.jpg")
	slots := f.slotsOf(showID)
	scene := "Lab.Show.S01E01.1080p.WEB-DL.H264-LAB"
	f.putFileAs(slots[[2]int{1, 1}], mediafiles.File{RelativePath: "Lab Show (2025)/Season 01/Lab Show - S01E01.mkv", Size: 5 << 20, ReleaseName: &scene,
		Quality: mediafiles.Quality{Resolution: 1080, Source: "web_dl", Codec: "h264", Audio: "AAC"}, Languages: []string{"en", "ja"}})

	arrList(t, "rootfolder", f.arrOK(son, "GET", s+"rootfolder", "", 200), 1)
	all := arrList(t, "series", f.arrOK(son, "GET", s+"series", "", 200), 1)
	// Bazarr asks for the list as "series/".
	arrList(t, "series/", f.arrOK(son, "GET", s+"series/", "", 200), 1)
	sm := fields(t, "bazarr series", all[0], []string{"id", "title", "path", "tvdbId", "overview", "images", "sortTitle", "year", "alternateTitles",
		"tags", "seriesType", "imdbId", "monitored", "ended", "lastAired", "originalLanguage"})
	// lastAired is the newest air date gone by outside the specials (the
	// special aired later, on 1 March 2025).
	if sm["lastAired"] != "2025-01-13T02:00:00Z" || sm["originalLanguage"].(map[string]any)["name"] != "English" ||
		sm["alternateTitles"].([]any)[0].(map[string]any)["title"] != "Lab Show Alt" || sm["path"] != filepath.Join(f.showDir, "Lab Show (2025)") {
		t.Errorf("series: %v", sm)
	}
	ims := sm["images"].([]any)
	if len(ims) != 2 {
		t.Fatalf("images: %v", ims)
	}
	poster := fields(t, "poster", ims[0], []string{"coverType", "url", "remoteUrl"})
	wide := fields(t, "fanart", ims[1], []string{"coverType", "url", "remoteUrl"})
	at := "/sonarr/MediaCover/" + arrItoa(showID) + "/"
	if poster["coverType"] != "poster" || poster["url"] != at+"poster.jpg" || poster["remoteUrl"] != "" || wide["coverType"] != "fanart" || wide["url"] != at+"fanart.jpg" ||
		wide["remoteUrl"] != "https://image.tmdb.org/t/p/w1280/lab-show-wide.jpg" {
		t.Errorf("images: %v %v", poster, wide)
	}
	one := fields(t, "one series", f.arrOK(son, "GET", s+"series/"+arrItoa(showID), "", 200), []string{"lastAired", "alternateTitles", "originalLanguage", "images"})
	if one["lastAired"] != sm["lastAired"] {
		t.Errorf("one series: %v", one)
	}

	// Episodes with their files (the version is past the point where
	// Bazarr reads them embedded).
	eps := arrList(t, "episodes", f.arrOK(son, "GET", s+"episode?seriesId="+arrItoa(showID)+"&includeEpisodeFile=true", "", 200), 5)
	withFile := 0
	for _, x := range eps {
		e := fields(t, "bazarr episode", x, []string{"id", "seriesId", "title", "seasonNumber", "episodeNumber", "monitored", "hasFile", "episodeFileId", "tvdbId"})
		if e["hasFile"] != true {
			fields(t, "episode without file", e, nil, "episodeFile")
			continue
		}
		withFile++
		ef := fields(t, "episode file", e["episodeFile"], []string{"id", "path", "size", "sceneName", "languages", "quality", "mediaInfo"})
		q := ef["quality"].(map[string]any)["quality"].(map[string]any)
		mi := ef["mediaInfo"].(map[string]any)
		langs := ef["languages"].([]any)
		if num(ef["id"]) != num(e["episodeFileId"]) || ef["path"] != filepath.Join(f.showDir, "Lab Show (2025)", "Season 01", "Lab Show - S01E01.mkv") ||
			ef["relativePath"] != "Season 01/Lab Show - S01E01.mkv" ||
			num(ef["size"]) != 5<<20 || ef["sceneName"] != scene || q["name"] != "WEBDL-1080p" || num(q["resolution"]) != 1080 ||
			mi["videoCodec"] != "h264" || mi["audioCodec"] != "AAC" || len(langs) != 2 || langs[1].(map[string]any)["name"] != "Japanese" {
			t.Errorf("episode file: %v", ef)
		}
	}
	if withFile != 1 {
		t.Fatalf("episodes with a file: %d", withFile)
	}
	for _, x := range arrList(t, "episodes", f.arrOK(son, "GET", s+"episode?seriesId="+arrItoa(showID), "", 200), 5) {
		fields(t, "episode, no file asked", x, nil, "episodeFile", "series")
	}
	arrList(t, "tags", f.arrOK(son, "GET", s+"tag", "", 200), -1)

	// After a subtitle change Bazarr asks for a rescan of the show.
	cmd := fields(t, "rescan", f.arrOK(son, "POST", s+"command", fmt.Sprintf(`{"name":"RescanSeries","seriesId":%d}`, showID), 201), []string{"id", "name", "status"})
	if cmd["name"] != "RescanSeries" {
		t.Fatalf("rescan: %v", cmd)
	}
	scanned := false
	for _, j := range f.jobs(library.KindScanTitle) {
		scanned = scanned || (j.Subject != nil && j.Subject.ID == showID)
	}
	if !scanned {
		t.Fatal("no scan Job for the show")
	}

	// The Radarr side: every movie carries hasFile, and one with a file
	// its file; pictures are poster first, then the wide one.
	movieID := f.addLabMovie()
	f.exec(`UPDATE titles SET overview = 'A lab movie.', aliases = '["Dune 2"]', original_language = 'en',
		source_ids = json_set(source_ids, '$.imdb', 'tt0200') WHERE id = ?`, movieID)
	if m := arrList(t, "movies", f.arrOK(rad, "GET", r+"movie", "", 200), 1)[0].(map[string]any); m["hasFile"] != false {
		t.Fatalf("movie without file: %v", m)
	}
	f.storePoster(movieID, "https://image.tmdb.org/t/p/w342/dune.png")
	f.storeWide(movieID, "https://image.tmdb.org/t/p/w1280/dune-wide.jpg")
	f.putFileAs(f.slotsOf(movieID)[[2]int{0, 0}], mediafiles.File{RelativePath: "Dune Part Two (2024)/Dune Part Two (2024).mkv", Size: 9 << 20,
		ReleaseName: &scene, Quality: mediafiles.Quality{Resolution: 2160, Source: "bluray", Audio: "TrueHD"}, Languages: []string{"en"},
		MediaInfo: map[string]any{"video_codec": "h265"}})
	m := fields(t, "bazarr movie", arrList(t, "movies", f.arrOK(rad, "GET", r+"movie", "", 200), 1)[0], []string{"id", "title", "path", "tmdbId",
		"imdbId", "images", "monitored", "year", "sortTitle", "alternateTitles", "overview", "tags", "originalLanguage", "hasFile", "movieFile"})
	mf := fields(t, "movie file", m["movieFile"], []string{"id", "path", "size", "sceneName", "languages", "quality", "mediaInfo"})
	mi := mf["mediaInfo"].(map[string]any)
	if m["hasFile"] != true || mf["quality"].(map[string]any)["quality"].(map[string]any)["name"] != "Bluray-2160p" || mi["videoCodec"] != "h265" ||
		mi["audioCodec"] != "TrueHD" || mf["languages"].([]any)[0].(map[string]any)["name"] != "English" || m["imdbId"] != "tt0200" {
		t.Errorf("movie: %v", m)
	}
	mims := m["images"].([]any)
	if len(mims) != 2 || mims[0].(map[string]any)["coverType"] != "poster" || mims[1].(map[string]any)["coverType"] != "fanart" {
		t.Errorf("movie images: %v", mims)
	}
	arrList(t, "qualityprofile", f.arrOK(rad, "GET", r+"qualityprofile", "", 200), -1)
	arrList(t, "radarr tags", f.arrOK(rad, "GET", r+"tag", "", 200), -1)
	arrList(t, "radarr rootfolder", f.arrOK(rad, "GET", r+"rootfolder", "", 200), 1)
	f.arrOK(rad, "POST", r+"command", fmt.Sprintf(`{"name":"RescanMovie","movieId":%d}`, movieID), 201)

	// Calls marked out answer 404, never a pretend success.
	for _, c := range []struct{ key, path string }{
		{son, s + "filesystem?path=/&allowFoldersWithoutTrailingSlashes=true&includeFiles=false"},
		{son, s + "history?eventType=1&episodeId=1"},
		{rad, r + "history?eventType=1&movieIds=1"},
		{son, s + "episode/1"},
		{son, s + "episodefile?seriesId=" + arrItoa(showID)},
		{rad, r + "moviefile?movieId=" + arrItoa(movieID)},
	} {
		if st, _, b := f.arr(c.key, "GET", c.path, ""); st != 404 {
			t.Errorf("%s: %d %s, want 404", c.path, st, b)
		}
	}
	// The live feed hub takes no key in access_token: a bare 401.
	if st, _, b := f.call(http.DefaultClient, "", "GET", "/sonarr/signalr/messages?access_token="+son, ""); st != 401 || b != "" {
		t.Errorf("hub: %d %q", st, b)
	}
}

// TestOldAppMediaCover: the covers the Library answers point at, with the
// key, in any case of the path, straight after the side or under /api/v3.
func TestOldAppMediaCover(t *testing.T) {
	t.Parallel()
	f := newOldAppFix(t)
	f.turnOn()
	son, rad := f.sonarrKey, f.radarrKey
	showID := f.addLabShow()
	movieID := f.addLabMovie()
	stored := f.storePoster(showID, "https://image.tmdb.org/t/p/w342/lab-show.png")
	wide := f.storeWide(showID, "https://image.tmdb.org/t/p/w1280/lab-show-wide.jpg")
	f.storePoster(movieID, "https://image.tmdb.org/t/p/w342/dune.png")
	get := func(key, path string) (int, http.Header, string) {
		t.Helper()
		return f.arr(key, "GET", path, "")
	}
	at := "/sonarr/MediaCover/" + arrItoa(showID) + "/"
	for _, p := range []string{at + "poster.jpg", at + "poster.jpg?lastWrite=638000000", "/sonarr/mediacover/" + arrItoa(showID) + "/POSTER.jpg",
		"/sonarr/api/v3/mediacover/" + arrItoa(showID) + "/poster.jpg"} {
		st, hd, b := get(son, p)
		// The address stays the same when the poster changes: kept a day, not a year.
		if st != 200 || b != string(stored) || hd.Get("Content-Type") != "image/png" || hd.Get("Cache-Control") != "private, max-age=86400" {
			t.Errorf("%s: %d %q %q %d bytes", p, st, hd.Get("Content-Type"), hd.Get("Cache-Control"), len(b))
		}
	}
	// Bazarr shows the 250 copy of a show's poster.
	st, _, b := get(son, at+"poster-250.jpg")
	if st != 200 {
		t.Fatalf("poster-250: %d", st)
	}
	cfg, _, err := image.DecodeConfig(strings.NewReader(b))
	if err != nil || cfg.Width > 250 || cfg.Height != 250 {
		t.Fatalf("poster-250: %v %+v", err, cfg)
	}
	if st, hd, b := get(son, at+"fanart.jpg"); st != 200 || b != string(wide) || strings.Contains(hd.Get("Cache-Control"), "immutable") {
		t.Errorf("fanart: %d %q %q", st, hd.Get("Cache-Control"), b)
	}
	if st, _, _ := get(rad, "/radarr/MediaCover/"+arrItoa(movieID)+"/poster.jpg"); st != 200 {
		t.Errorf("movie poster: %d", st)
	}
	for _, c := range []struct{ key, path string }{
		{son, at + "banner.jpg"},
		{son, "/radarr/MediaCover/" + arrItoa(movieID) + "/poster.jpg"}, // the other side's key
		{rad, "/radarr/MediaCover/" + arrItoa(movieID) + "/fanart.jpg"}, // no wide picture
		{son, "/sonarr/MediaCover/" + arrItoa(movieID) + "/poster.jpg"}, // a movie on the Sonarr side
		{rad, "/radarr/MediaCover/" + arrItoa(showID) + "/poster.jpg"},  // a show on the Radarr side
		{son, "/sonarr/MediaCover/999999/poster.jpg"},
		{son, "/sonarr/MediaCover/x/poster.jpg"},
		{son, "/sonarr/MediaCoverProxy/x/poster.jpg"},
	} {
		if st, _, _ := get(c.key, c.path); st != 404 && !(c.key == son && strings.HasPrefix(c.path, "/radarr/") && st == 401) {
			t.Errorf("%s: %d, want 404", c.path, st)
		}
	}
	if st, _, b := f.call(http.DefaultClient, "", "GET", at+"poster.jpg", ""); st != 401 || b != "" {
		t.Errorf("no key: %d %q", st, b)
	}
	f.arrOK(son, "DELETE", "/sonarr/api/v3/series/"+arrItoa(showID), "", 200)
	if st, _, _ := get(son, at+"poster.jpg"); st != 404 {
		t.Errorf("removed show: %d", st)
	}
}

// TestOldAppUnpackerrQueue polls the download list as Unpackerr does (2000
// rows a page): a finished torrent waiting for its import says completed,
// with its own folder as outputPath; once imported its rows are gone.
func TestOldAppUnpackerrQueue(t *testing.T) {
	t.Parallel()
	f := newOldAppFix(t)
	f.turnOn()
	showID := f.addLabShow()
	movieID := f.addLabMovie()
	slots := f.slotsOf(showID)
	movieSlot := f.slotsOf(movieID)[[2]int{0, 0}]
	var pack, single int64
	if err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		dr := domain.Downloader{Adapter: "qbittorrent", Name: "Lab qBit", Protocol: "torrent", URL: "http://lab.invalid",
			PathMap: []domain.PathMap{{Theirs: "/downloads", Ours: "/data/downloads"}}, AfterImport: "leave", State: "ok"}
		if _, err := database.Insert(tx, "downloaders", &dr, domain.System); err != nil {
			return err
		}
		for _, d := range []struct {
			id    *int64
			hash  string
			save  string
			slots []int64
			seen  []domain.SeenFile
		}{
			{&pack, "PACK01", "/downloads/tv", []int64{slots[[2]int{1, 1}].ID, slots[[2]int{1, 2}].ID},
				[]domain.SeenFile{{Name: "Lab.Show.S01/Lab.Show.S01E01.mkv", Size: 5 << 20}, {Name: "Lab.Show.S01/Lab.Show.S01E02.rar", Size: 5 << 20}}},
			{&single, "MOVIE01", "/downloads/movies", []int64{movieSlot.ID}, []domain.SeenFile{{Name: "Dune.Part.Two.2024.2160p.mkv", Size: 9 << 20}}},
		} {
			dl := domain.Download{DownloaderID: dr.ID, ExternalID: d.hash, SlotIDs: d.slots, State: "finished", Progress: 1, SavePath: d.save,
				FilesSeen: d.seen, StartedBy: domain.System}
			id, err := database.Insert(tx, "downloads", &dl, domain.System)
			if err != nil {
				return err
			}
			*d.id = id
			if _, err := tx.Exec("UPDATE slots SET active_download_id = ?, state = 'importing' WHERE id IN "+idSQL(d.slots), id); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	poll := func(key, side string) map[string]any {
		t.Helper()
		return fields(t, side+" queue", f.arrOK(key, "GET", "/"+side+"/api/v3/queue?page=1&pageSize=2000&includeUnknownSeriesItems=false", "", 200),
			[]string{"page", "pageSize", "totalRecords", "records"})
	}
	q := poll(f.sonarrKey, "sonarr")
	if num(q["page"]) != 1 || num(q["pageSize"]) != 2000 || num(q["totalRecords"]) != 2 || len(q["records"].([]any)) != 2 {
		t.Fatalf("sonarr queue: %v", q)
	}
	for _, x := range q["records"].([]any) {
		row := fields(t, "unpackerr row", x, []string{"title", "status", "protocol", "outputPath", "downloadId", "seriesId", "episodeId", "size", "sizeleft", "statusMessages"})
		if row["status"] != "completed" || row["protocol"] != "torrent" || row["outputPath"] != "/data/downloads/tv/Lab.Show.S01" ||
			row["downloadId"] != "PACK01" || num(row["seriesId"]) != showID || num(row["sizeleft"]) != 0 {
			t.Errorf("sonarr row: %v", row)
		}
	}
	// A page past every page shows the last one, on a 32-bit build too.
	last := fields(t, "sonarr queue", f.arrOK(f.sonarrKey, "GET", "/sonarr/api/v3/queue?page=9223372036854775807&pageSize=1", "", 200), []string{"page", "records"})
	if num(last["page"]) != 2 || len(last["records"].([]any)) != 1 {
		t.Fatalf("queue past every page: %v", last)
	}
	q = poll(f.radarrKey, "radarr")
	rows := arrList(t, "radarr rows", q["records"], 1)
	if row := fields(t, "unpackerr movie row", rows[0], []string{"title", "status", "protocol", "outputPath", "downloadId", "movieId", "size", "sizeleft", "statusMessages"}); row["status"] != "completed" ||
		row["outputPath"] != "/data/downloads/movies/Dune.Part.Two.2024.2160p.mkv" || num(row["movieId"]) != movieID || num(q["totalRecords"]) != 1 {
		t.Errorf("radarr row: %v", row)
	}
	// Imported: the rows leave the list, which Unpackerr reads as done.
	f.putFile(slots[[2]int{1, 1}], "Lab Show/S01E01.mkv", 1080)
	f.putFile(slots[[2]int{1, 2}], "Lab Show/S01E02.mkv", 1080)
	f.exec("UPDATE slots SET active_download_id = NULL WHERE active_download_id = ?", pack)
	if q := poll(f.sonarrKey, "sonarr"); num(q["totalRecords"]) != 0 || len(q["records"].([]any)) != 0 {
		t.Fatalf("after import: %v", q)
	}
}

// idSQL writes ids as an SQL list (test helper).
func idSQL(ids []int64) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = arrItoa(id)
	}
	return "(" + strings.Join(parts, ",") + ")"
}

// TestOldAppHomepageWidgets makes the Sonarr widget's calls: the show
// list, the queue and its unpaged details, the missing list and the
// calendar, with the paging rules of the Sonarr manual.
func TestOldAppHomepageWidgets(t *testing.T) {
	t.Parallel()
	f := newOldAppFix(t)
	f.turnOn()
	son := f.sonarrKey
	s := "/sonarr/api/v3/"
	showID := f.addLabShow()
	arrList(t, "series", f.arrOK(son, "GET", s+"series", "", 200), 1)
	fields(t, "queue", f.arrOK(son, "GET", s+"queue", "", 200), []string{"totalRecords", "records"})

	// The missing list: aired, no file, kept up to date with its season and
	// show (the special aired but is off; season 2 has not aired).
	e101, e102, special := f.itemOf(showID, 1, 1), f.itemOf(showID, 1, 2), f.itemOf(showID, 0, 1)
	missing := func(q string) (map[string]any, []int64) {
		t.Helper()
		m := fields(t, "missing", f.arrOK(son, "GET", s+"wanted/missing"+q, "", 200), []string{"page", "pageSize", "sortKey", "sortDirection", "totalRecords", "records"})
		var ids []int64
		for _, x := range m["records"].([]any) {
			ids = append(ids, num(fields(t, "missing episode", x, []string{"id", "seriesId", "seasonNumber", "episodeNumber", "title", "airDateUtc", "hasFile", "monitored"})["id"]))
		}
		return m, ids
	}
	m, ids := missing("")
	if num(m["totalRecords"]) != 2 || num(m["pageSize"]) != 10 || num(m["page"]) != 1 || m["sortKey"] != "episodes.airDateUtc" ||
		m["sortDirection"] != "descending" || fmt.Sprint(ids) != fmt.Sprint([]int64{e102, e101}) {
		t.Fatalf("missing: %v %v", m, ids)
	}
	if m, ids := missing("?sortDirection=default&sortKey=nope"); m["sortDirection"] != "ascending" || m["sortKey"] != "episodes.airDateUtc" ||
		fmt.Sprint(ids) != fmt.Sprint([]int64{e101, e102}) {
		t.Errorf("default direction: %v %v", m, ids)
	}
	if m, ids := missing("?page=9"); num(m["page"]) != 9 || len(ids) != 0 || num(m["totalRecords"]) != 2 {
		t.Errorf("past the end: %v", m)
	}
	if m, ids := missing("?page=9223372036854775807&pageSize=10000"); len(ids) != 0 || num(m["totalRecords"]) != 2 {
		t.Errorf("huge page: %v", m)
	}
	if m, ids := missing("?pageSize=1&page=2"); fmt.Sprint(ids) != fmt.Sprint([]int64{e101}) || num(m["pageSize"]) != 1 {
		t.Errorf("second page: %v %v", m, ids)
	}
	if m, ids := missing("?monitored=false"); num(m["totalRecords"]) != 1 || fmt.Sprint(ids) != fmt.Sprint([]int64{special}) {
		t.Errorf("not kept: %v %v", m, ids)
	}
	if m, _ := missing("?includeSeries=true&pageSize=1"); fields(t, "missing series", m["records"].([]any)[0].(map[string]any)["series"], []string{"id", "title"})["title"] != "Lab Show" {
		t.Errorf("includeSeries: %v", m)
	}
	f.putFile(f.slotsOf(showID)[[2]int{1, 1}], "Lab Show/S01E01.mkv", 1080)
	if m, ids := missing(""); num(m["totalRecords"]) != 1 || fmt.Sprint(ids) != fmt.Sprint([]int64{e102}) {
		t.Errorf("after a file: %v %v", m, ids)
	}

	// The calendar: from today to two days on by default; episodes not
	// kept up to date only with unmonitored=true.
	now := time.Now().UTC()
	soon := now.Add(24 * time.Hour).Truncate(time.Second)
	f.exec("UPDATE items SET air_date = ?, air_day = ?, air_timed = 1 WHERE id IN (?, ?)", database.FormatTime(soon), soon.Format("2006-01-02"), e102, f.itemOf(showID, 2, 1))
	calendar := func(q string) []any {
		t.Helper()
		l := arrList(t, "calendar", f.arrOK(son, "GET", s+"calendar"+q, "", 200), -1)
		for _, x := range l {
			fields(t, "calendar episode", x, []string{"id", "seriesId", "seasonNumber", "episodeNumber", "airDateUtc", "monitored", "hasFile"})
		}
		return l
	}
	if l := calendar(""); len(l) != 1 || num(l[0].(map[string]any)["id"]) != e102 {
		t.Fatalf("calendar: %v", l)
	}
	if l := calendar("?unmonitored=true&includeSeries=true&includeEpisodeFile=true"); len(l) != 2 ||
		fields(t, "calendar series", l[0].(map[string]any)["series"], []string{"title"})["title"] != "Lab Show" {
		t.Errorf("unmonitored: %v", l)
	}
	day := func(d int) string { return now.AddDate(0, 0, d).Format("2006-01-02") }
	if l := calendar("?start=" + day(3) + "&end=" + day(5)); len(l) != 0 {
		t.Errorf("later range: %v", l)
	}
	if l := calendar("?start=" + day(-1) + "T00:00:00Z&end=" + day(3)); len(l) != 1 {
		t.Errorf("range with times: %v", l)
	}
	if l := calendar("?start=" + day(-5)); len(l) != 1 { // end still defaults to today plus two days
		t.Errorf("start only: %v", l)
	}

	// The queue details: every row, unpaged; filtered by show or episode.
	if err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		dr := domain.Downloader{Adapter: "sabnzbd", Name: "Lab SAB", Protocol: "usenet", URL: "http://lab.invalid", PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok"}
		if _, err := database.Insert(tx, "downloaders", &dr, domain.System); err != nil {
			return err
		}
		eta := int64(60)
		d := domain.Download{DownloaderID: dr.ID, ExternalID: "SAB01", SlotIDs: []int64{f.slotsOf(showID)[[2]int{1, 2}].ID}, State: "downloading",
			Progress: 0.25, ETA: &eta, SavePath: "/downloads/tv", FilesSeen: []domain.SeenFile{}, StartedBy: domain.System}
		_, err := database.Insert(tx, "downloads", &d, domain.System)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	details := func(q string, n int) []any {
		t.Helper()
		return arrList(t, "queue details"+q, f.arrOK(son, "GET", s+"queue/details"+q, "", 200), n)
	}
	row := fields(t, "details row", details("", 1)[0], []string{"id", "seriesId", "episodeId", "title", "status", "size", "sizeleft", "timeleft", "protocol"}, "series", "episode")
	if row["status"] != "downloading" || row["protocol"] != "usenet" || num(row["episodeId"]) != e102 {
		t.Errorf("details row: %v", row)
	}
	details("?seriesId=999", 0)
	details("?episodeIds="+arrItoa(e101), 0)
	row = fields(t, "details with show", details("?seriesId="+arrItoa(showID)+"&episodeIds="+arrItoa(e102)+"&includeSeries=true&includeEpisode=true", 1)[0], []string{"series", "episode"})
	if row["series"].(map[string]any)["title"] != "Lab Show" {
		t.Errorf("details with show: %v", row)
	}

	// The Radarr widget's lists wait for the research answer on what it
	// calls; until then they answer 404, never a pretend success.
	for _, p := range []string{"wanted/missing", "calendar", "queue/details"} {
		if st, _, _ := f.arr(f.radarrKey, "GET", "/radarr/api/v3/"+p, ""); st != 404 {
			t.Errorf("radarr %s: %d", p, st)
		}
	}
}
