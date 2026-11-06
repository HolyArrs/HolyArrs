package migrate

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// radarrAPI is a Radarr 6 answering the API calls the manual lists.
func radarrAPI(t *testing.T) (string, string) { return radarrAPIWith(t, nil) }

// radarrAPIWith adds or replaces routes of radarrAPI.
func radarrAPIWith(t *testing.T, extra map[string]any) (string, string) {
	key := "S3CR3T_RADARR_KEY"
	routes := map[string]any{
		"/api/v3/system/status": map[string]any{"appName": "Radarr", "version": "6.4.4", "migrationVersion": 242},
		"/api/v3/movie": []any{
			map[string]any{"id": 1, "tmdbId": 603, "imdbId": "tt0133093", "title": "The Matrix", "year": 1999,
				"path": "/movies/The Matrix (1999)", "rootFolderPath": "/movies", "monitored": true, "qualityProfileId": 1,
				"minimumAvailability": "released", "added": "2024-01-02T03:04:05Z", "tags": []int{1}},
			map[string]any{"id": 2, "tmdbId": 604, "title": "Unwatched", "year": 2003, "path": "/movies/Unwatched (2003)",
				"monitored": false, "qualityProfileId": 9, "tags": []int{}},
		},
		"/api/v3/tag":        []any{map[string]any{"id": 1, "label": "kids"}},
		"/api/v3/rootfolder": []any{map[string]any{"id": 1, "path": "/movies", "accessible": true, "freeSpace": 100}},
		"/api/v3/qualityprofile": []any{map[string]any{"id": 1, "name": "HD", "upgradeAllowed": true, "cutoff": 7,
			"items": []any{
				map[string]any{"quality": map[string]any{"id": 7, "name": "Bluray-1080p"}, "items": []any{}, "allowed": true},
				map[string]any{"quality": map[string]any{"id": 4, "name": "HDTV-720p"}, "items": []any{}, "allowed": false},
			},
			"formatItems": []any{map[string]any{"format": 1, "name": "x265", "score": 50}}, "cutoffFormatScore": 100}},
		"/api/v3/customformat": []any{map[string]any{"id": 1, "name": "x265", "includeCustomFormatWhenRenaming": false,
			"specifications": []any{map[string]any{"name": "x265", "implementation": "ReleaseTitleSpecification", "negate": false, "required": true,
				"fields": []any{map[string]any{"name": "value", "value": "x265"}}}}}},
		"/api/v3/delayprofile": []any{map[string]any{"id": 1, "enableUsenet": true, "enableTorrent": true, "preferredProtocol": "usenet",
			"usenetDelay": 0, "torrentDelay": 60, "order": 2147483647, "tags": []int{}}},
		"/api/v3/config/naming": map[string]any{"id": 1, "renameMovies": true, "standardMovieFormat": "{Movie Title} ({Release Year})"},
		"/api/v3/indexer": []any{map[string]any{"id": 1, "name": "Nyaa", "implementation": "Torznab", "protocol": "torrent", "priority": 25,
			"enableRss": true, "enableAutomaticSearch": true, "tags": []int{1},
			"fields": []any{
				map[string]any{"name": "baseUrl", "value": "http://prowlarr:9696/1/"},
				map[string]any{"name": "apiKey", "value": "********", "privacy": "apiKey"},
				map[string]any{"name": "categories", "value": []int{2000, 2010}},
				// An older app may not mark the field private: the name decides.
				map[string]any{"name": "additionalApiKey", "value": "S3CR3T_UNMARKED"},
			}}},
		"/api/v3/downloadclient": []any{map[string]any{"id": 1, "name": "qBit", "implementation": "QBittorrent", "enable": true, "priority": 1,
			"protocol": "torrent", "fields": []any{
				map[string]any{"name": "host", "value": "qbit"}, map[string]any{"name": "port", "value": 8080},
				map[string]any{"name": "username", "value": "admin"},
				map[string]any{"name": "password", "value": "S3CR3T_QBIT", "privacy": "password"},
			}}},
		"/api/v3/importlist": []any{map[string]any{"id": 1, "name": "Trakt", "implementation": "TraktListImport", "enabled": true,
			"fields": []any{map[string]any{"name": "accessToken", "value": "S3CR3T_TRAKT"}, map[string]any{"name": "listName", "value": "watch"}}}},
		"/api/v3/notification": []any{map[string]any{"id": 1, "name": "Discord", "implementation": "Discord", "onGrab": true,
			"fields": []any{map[string]any{"name": "webHookUrl", "value": "https://discord.example/api/webhooks/1/S3CR3T_HOOK"}}}},
		"/api/v3/metadata": []any{map[string]any{"id": 1, "name": "Kodi", "implementation": "XbmcMetadata", "enable": false, "fields": []any{}}},
	}
	for k, v := range extra {
		routes[k] = v
	}
	srv := fakeAPI(t, "X-Api-Key", key, routes)
	return srv.URL, key
}

func TestRadarrAPI(t *testing.T) {
	u, key := radarrAPI(t)
	info, err := Probe(context.Background(), api(Radarr, u, key))
	if err != nil || info.Version != "6.4.4" || info.Schema != 242 {
		t.Fatalf("probe %+v %v", info, err)
	}
	s := read(t, api(Radarr, u, key))
	if s.Kind != Radarr || s.Via != ViaAPI || s.Version != "6.4.4" || len(s.Titles) != 2 {
		t.Fatalf("snapshot %+v", s)
	}
	m := findTitle(t, s, "The Matrix")
	if m.Kind != "movie" || m.Year != 1999 || m.IDs["tmdb"] != "603" || m.IDs["imdb"] != "tt0133093" || !m.Monitored ||
		m.Profile != "HD" || m.Folder != "/movies" || m.Added == "" || !slices.Equal(m.Tags, []string{"kids"}) || m.Extra["minimum_availability"] != "released" {
		t.Fatalf("matrix %+v", m)
	}
	if u := findTitle(t, s, "Unwatched"); u.Monitored || u.Profile != "" || u.Folder != "/movies" {
		t.Fatalf("unwatched %+v", u)
	}
	p := s.QualityProfiles[0]
	if p.Name != "HD" || p.Cutoff != "Bluray-1080p" || !slices.Equal(p.Allowed, []string{"Bluray-1080p"}) || p.Scores["x265"] != 50 || p.CutoffScore != 100 {
		t.Fatalf("profile %+v", p)
	}
	if len(s.CustomFormats) != 1 || s.CustomFormats[0].Items[0]["value.value"] != "x265" {
		t.Fatalf("formats %+v", s.CustomFormats)
	}
	if len(s.DelayProfiles) != 1 || s.DelayProfiles[0].TorrentDelay != 60 || s.DelayProfiles[0].PreferredProtocol != "usenet" {
		t.Fatalf("delay %+v", s.DelayProfiles)
	}
	if s.Naming["standardmovieformat"] != "{Movie Title} ({Release Year})" {
		t.Fatalf("naming %+v", s.Naming)
	}
	ix := s.Indexers[0]
	if ix.Name != "Nyaa" || !ix.Enabled || ix.Protocol != "torrent" || ix.Priority != 25 || ix.Settings["baseUrl"] != "http://prowlarr:9696/1/" ||
		!slices.Equal(ix.Tags, []string{"kids"}) || !slices.Contains(ix.NeedsReentry, "apiKey") || !slices.Contains(ix.NeedsReentry, "additionalApiKey") {
		t.Fatalf("indexer %+v", ix)
	}
	dc := s.DownloadClients[0]
	if dc.Settings["host"] != "qbit" || dc.Settings["username"] != "admin" || dc.Settings["port"] != int64(8080) || !slices.Contains(dc.NeedsReentry, "password") {
		t.Fatalf("client %+v", dc)
	}
	if s.Metadata[0].Enabled || !s.Notifications[0].Enabled {
		t.Fatalf("enabled flags: %+v %+v", s.Metadata, s.Notifications)
	}
	for _, want := range []string{`indexer "Nyaa": apiKey`, `download client "qBit": password`, `import list "Trakt": accessToken`, `notification "Discord": webHookUrl`} {
		if !hasReentry(s, want) {
			t.Errorf("needs re-entry lacks %q: %v", want, s.NeedsReentry)
		}
	}
}

func TestArrWrongKeyAndWrongApp(t *testing.T) {
	u, _ := radarrAPI(t)
	if _, err := Probe(context.Background(), api(Radarr, u, "nope")); !errors.Is(err, ErrBadKey) {
		t.Fatalf("wrong key: %v", err)
	}
	u, key := radarrAPI(t)
	if _, err := Probe(context.Background(), api(Sonarr, u, key)); !errors.Is(err, ErrWrongApp) {
		t.Fatalf("radarr read as sonarr: %v", err)
	}
	if _, err := Read(context.Background(), api(Lidarr, u, key)); !errors.Is(err, ErrWrongApp) {
		t.Fatalf("radarr read as lidarr (no /api/v1): %v", err)
	}
}

// radarrDB is a Radarr 6 database (migration 242): movies joined to
// MovieMetadata, qualities named through QualityDefinitions.
func radarrDB() []string {
	return []string{
		`CREATE TABLE VersionInfo (Version INTEGER, AppliedOn TEXT, Description TEXT)`,
		`INSERT INTO VersionInfo VALUES (241, '', ''), (242, '', '')`,
		`CREATE TABLE Tags (Id INTEGER PRIMARY KEY, Label TEXT)`,
		`INSERT INTO Tags VALUES (1, 'kids')`,
		`CREATE TABLE RootFolders (Id INTEGER PRIMARY KEY, Path TEXT)`,
		`INSERT INTO RootFolders VALUES (1, '/movies'), (2, '/movies-4k')`,
		`CREATE TABLE QualityDefinitions (Id INTEGER PRIMARY KEY, Quality INTEGER, Title TEXT)`,
		`INSERT INTO QualityDefinitions VALUES (1, 7, 'Bluray-1080p'), (2, 3, 'WEBDL-1080p'), (3, 4, 'HDTV-720p')`,
		`CREATE TABLE QualityProfiles (Id INTEGER PRIMARY KEY, Name TEXT, UpgradeAllowed INTEGER, Cutoff INTEGER, Items TEXT, FormatItems TEXT, MinFormatScore INTEGER, CutoffFormatScore INTEGER, Language INTEGER)`,
		`INSERT INTO QualityProfiles VALUES (1, 'HD', 1, 1000, '[{"quality":4,"items":[],"allowed":false},{"id":1000,"name":"HD group","items":[{"quality":7,"items":[],"allowed":true},{"quality":3,"items":[],"allowed":true}],"allowed":true}]', '[{"format":1,"score":40}]', 0, 0, 1)`,
		`CREATE TABLE CustomFormats (Id INTEGER PRIMARY KEY, Name TEXT, Specifications TEXT, IncludeCustomFormatWhenRenaming INTEGER)`,
		`INSERT INTO CustomFormats VALUES (1, 'x265', '[{"name":"x265","implementation":"ReleaseTitleSpecification","negate":false,"required":true,"fields":{"value":"x265"}}]', 0)`,
		`CREATE TABLE DelayProfiles (Id INTEGER PRIMARY KEY, EnableUsenet INTEGER, EnableTorrent INTEGER, PreferredProtocol INTEGER, UsenetDelay INTEGER, TorrentDelay INTEGER, "Order" INTEGER, Tags TEXT)`,
		`INSERT INTO DelayProfiles VALUES (1, 1, 1, 2, 0, 30, 1, '[1]')`,
		`CREATE TABLE NamingConfig (Id INTEGER PRIMARY KEY, RenameMovies INTEGER, StandardMovieFormat TEXT, MovieFolderFormat TEXT)`,
		`INSERT INTO NamingConfig VALUES (1, 1, '{Movie Title}', '{Movie Title} ({Release Year})')`,
		`CREATE TABLE Indexers (Id INTEGER PRIMARY KEY, Name TEXT, Implementation TEXT, Settings TEXT, ConfigContract TEXT, EnableRss INTEGER, EnableAutomaticSearch INTEGER, EnableInteractiveSearch INTEGER, Priority INTEGER, Protocol INTEGER, Tags TEXT)`,
		`INSERT INTO Indexers VALUES (1, 'Nyaa', 'Torznab', '{"baseUrl":"http://prowlarr:9696/1/","apiKey":"S3CR3T_IDX","categories":[2000,2010],"minimumSeeders":1}', 'TorznabSettings', 1, 1, 1, 25, 2, '[]')`,
		`CREATE TABLE DownloadClients (Id INTEGER PRIMARY KEY, Name TEXT, Implementation TEXT, Settings TEXT, ConfigContract TEXT, Enable INTEGER, Priority INTEGER, Protocol INTEGER, Tags TEXT)`,
		`INSERT INTO DownloadClients VALUES (1, 'qBit', 'QBittorrent', '{"host":"qbit","port":8080,"username":"admin","password":"S3CR3T_QB","movieCategory":"radarr"}', 'QBittorrentSettings', 1, 1, 2, '[]')`,
		`CREATE TABLE ImportLists (Id INTEGER PRIMARY KEY, Name TEXT, Implementation TEXT, Settings TEXT, ConfigContract TEXT, Enabled INTEGER, EnableAuto INTEGER, Monitor INTEGER, SearchOnAdd INTEGER, QualityProfileId INTEGER, RootFolderPath TEXT, MinimumAvailability INTEGER, Tags TEXT)`,
		`INSERT INTO ImportLists VALUES (1, 'Trakt', 'TraktListImport', '{"accessToken":"S3CR3T_TRAKT","refreshToken":"S3CR3T_REFRESH","listName":"watch"}', 'TraktListSettings', 1, 1, 0, 1, 1, '/movies', 3, '[]')`,
		`CREATE TABLE Notifications (Id INTEGER PRIMARY KEY, Name TEXT, Implementation TEXT, Settings TEXT, ConfigContract TEXT, OnGrab INTEGER, Tags TEXT)`,
		`INSERT INTO Notifications VALUES (1, 'Apprise', 'Apprise', '{"notificationType":0,"statelessUrls":"discord://1/S3CR3T_TOK","serverUrl":"http://apprise:8000"}', 'AppriseSettings', 1, '[]')`,
		`CREATE TABLE Metadata (Id INTEGER PRIMARY KEY, Name TEXT, Implementation TEXT, Settings TEXT, ConfigContract TEXT, Enable INTEGER)`,
		`INSERT INTO Metadata VALUES (1, 'Kodi', 'XbmcMetadata', '{"movieMetadata":true}', 'XbmcMetadataSettings', 0)`,
		`CREATE TABLE MovieMetadata (Id INTEGER PRIMARY KEY, TmdbId INTEGER, ImdbId TEXT, Title TEXT, SortTitle TEXT, Year INTEGER)`,
		`INSERT INTO MovieMetadata VALUES (10, 603, 'tt0133093', 'The Matrix', 'matrix', 1999), (11, 999, '', 'Only In A Collection', '', 2020), (12, 605, '', 'Big', '', 2021)`,
		`CREATE TABLE Movies (Id INTEGER PRIMARY KEY, MovieMetadataId INTEGER, Path TEXT, Monitored INTEGER, MinimumAvailability INTEGER, QualityProfileId INTEGER, Tags TEXT, AddOptions TEXT, MovieFileId INTEGER, Added TEXT, LastSearchTime TEXT)`,
		`INSERT INTO Movies VALUES (1, 10, '/movies/The Matrix (1999)', 1, 3, 1, '[1]', '{}', 0, '2024-01-02 03:04:05Z', NULL), (2, 12, '/movies-4k/Big (2021)', 0, 3, 1, '[]', '{}', 0, '2024-02-02 03:04:05Z', NULL)`,
		`CREATE TABLE ImportExclusions (Id INTEGER PRIMARY KEY, TmdbId INTEGER, MovieTitle TEXT, MovieYear INTEGER)`,
		`INSERT INTO ImportExclusions VALUES (1, 680, 'Pulp Fiction', 1994)`,
	}
}

func TestRadarrBackupZip(t *testing.T) {
	dir := t.TempDir()
	zp := filepath.Join(dir, "radarr_backup_v6.4.4_2026.10.14_10.00.00.zip")
	makeZip(t, zp, map[string][]byte{
		"radarr.db":  dbBytes(t, radarrDB()...),
		"config.xml": []byte("<Config><ApiKey>S3CR3T_CONFIG_KEY</ApiKey></Config>"),
		"INFO":       []byte("6.4.4"),
	})
	info, err := Probe(context.Background(), file(Radarr, zp))
	if err != nil || info.Schema != 242 || info.Via != ViaBackup {
		t.Fatalf("probe %+v %v", info, err)
	}
	s := read(t, file(Radarr, zp))
	if s.Schema != 242 || len(s.Titles) != 2 {
		t.Fatalf("snapshot %+v", s)
	}
	m := findTitle(t, s, "The Matrix")
	if m.Year != 1999 || m.IDs["tmdb"] != "603" || !m.Monitored || m.Profile != "HD" || m.Folder != "/movies" || !slices.Equal(m.Tags, []string{"kids"}) {
		t.Fatalf("matrix %+v", m)
	}
	if b := findTitle(t, s, "Big"); b.Folder != "/movies-4k" || b.Monitored {
		t.Fatalf("big %+v", b)
	}
	p := s.QualityProfiles[0]
	if p.Cutoff != "HD group" || !slices.Equal(p.Allowed, []string{"HD group"}) || p.Scores["x265"] != 40 {
		t.Fatalf("profile %+v", p)
	}
	if d := s.DelayProfiles[0]; d.PreferredProtocol != "torrent" || d.TorrentDelay != 30 || !slices.Equal(d.Tags, []string{"kids"}) {
		t.Fatalf("delay %+v", d)
	}
	if s.Indexers[0].Protocol != "torrent" || s.Indexers[0].Settings["minimumSeeders"] != int64(1) || !hasReentry(s, `indexer "Nyaa": apiKey`) {
		t.Fatalf("indexer %+v", s.Indexers[0])
	}
	for _, want := range []string{`import list "Trakt": accessToken`, `import list "Trakt": refreshToken`, `notification "Apprise": statelessUrls`} {
		if !hasReentry(s, want) {
			t.Errorf("needs re-entry lacks %q: %v", want, s.NeedsReentry)
		}
	}
	// Every link in a notification is a secret: the server address is
	// dropped and must be typed again.
	if s.Notifications[0].Settings["serverUrl"] != nil || !hasReentry(s, `notification "Apprise": serverUrl`) {
		t.Fatalf("notification %+v", s.Notifications[0])
	}
	if len(s.Exclusions) != 1 || s.Exclusions[0].IDs["tmdb"] != "680" || s.Exclusions[0].Name != "Pulp Fiction" {
		t.Fatalf("exclusions %+v", s.Exclusions)
	}
	// The database alone, and a folder holding the backup's files, read the same.
	plain := filepath.Join(t.TempDir(), "radarr.db")
	makeDB(t, plain, radarrDB()...)
	if s2 := read(t, file(Radarr, plain)); len(s2.Titles) != 2 {
		t.Fatalf("db alone: %+v", s2.Titles)
	}
	if s3 := read(t, file(Radarr, filepath.Dir(plain))); len(s3.Titles) != 2 {
		t.Fatalf("folder: %+v", s3.Titles)
	}
}

// TestRadarrOlderBackup: an older Radarr keeps title, year and ids on the
// Movies row (no MovieMetadata) and calls the profile column ProfileId;
// tables it lacks are skipped with a note.
func TestRadarrOlderBackup(t *testing.T) {
	p := filepath.Join(t.TempDir(), "radarr.db")
	makeDB(t, p,
		`CREATE TABLE VersionInfo (Version INTEGER)`, `INSERT INTO VersionInfo VALUES (180)`,
		`CREATE TABLE Profiles (Id INTEGER PRIMARY KEY, Name TEXT)`,
		`CREATE TABLE Movies (Id INTEGER PRIMARY KEY, TmdbId INTEGER, ImdbId TEXT, Title TEXT, Year INTEGER, Path TEXT, Monitored INTEGER, ProfileId INTEGER, Tags TEXT, Added TEXT)`,
		`INSERT INTO Movies VALUES (1, 603, 'tt0133093', 'The Matrix', 1999, '/movies/The Matrix (1999)', 1, 4, '[]', '2019-01-01')`,
	)
	s := read(t, file(Radarr, p))
	m := findTitle(t, s, "The Matrix")
	if m.IDs["tmdb"] != "603" || m.Year != 1999 || m.ProfileID != 4 {
		t.Fatalf("old movie %+v", m)
	}
	for _, missing := range []string{"QualityProfiles", "CustomFormats", "Indexers", "NamingConfig", "ImportExclusions"} {
		if !hasNote(s, missing) {
			t.Errorf("no note for missing %s: %v", missing, s.Notes)
		}
	}
	if s.Schema != 180 {
		t.Fatalf("schema %d", s.Schema)
	}
}

// sonarrAPI answers as Sonarr 4 (or, with v5, as the v5 build: naming
// under /settings/naming, notifications under /connection, no language
// profiles).
func sonarrAPI(t *testing.T, v5 bool) (string, string) { return sonarrAPIWith(t, v5, nil) }

// sonarrAPIWith adds or replaces routes of sonarrAPI.
func sonarrAPIWith(t *testing.T, v5 bool, extra map[string]any) (string, string) {
	key := "S3CR3T_SONARR_KEY"
	routes := map[string]any{
		"/api/v3/system/status": map[string]any{"appName": "Sonarr", "version": "4.0.20.0", "migrationVersion": 217},
		"/api/v3/series": []any{map[string]any{"id": 5, "tvdbId": 81189, "tmdbId": 1396, "imdbId": "tt0903747", "title": "Breaking Bad", "year": 2008,
			"path": "/tv/Breaking Bad", "rootFolderPath": "/tv", "monitored": true, "monitorNewItems": "all", "qualityProfileId": 1,
			"seasonFolder": true, "seriesType": "standard", "added": "2023-05-05T00:00:00Z", "tags": []int{},
			"seasons": []any{map[string]any{"seasonNumber": 0, "monitored": false}, map[string]any{"seasonNumber": 1, "monitored": true}}}},
		"/api/v3/tag":            []any{},
		"/api/v3/rootfolder":     []any{map[string]any{"id": 1, "path": "/tv"}},
		"/api/v3/qualityprofile": []any{map[string]any{"id": 1, "name": "Any", "cutoff": 1, "items": []any{}}},
		"/api/v3/customformat":   []any{},
		"/api/v3/delayprofile":   []any{},
		"/api/v3/indexer":        []any{},
		"/api/v3/downloadclient": []any{},
		"/api/v3/importlist":     []any{},
		"/api/v3/metadata":       []any{},
	}
	naming := map[string]any{"renameEpisodes": true, "standardEpisodeFormat": "{Series Title} - S{season:00}E{episode:00}"}
	if v5 {
		routes["/api/v3/system/status"] = map[string]any{"appName": "Sonarr", "version": "5.0.0.1", "migrationVersion": 236}
		routes["/api/v3/settings/naming"] = naming
		routes["/api/v3/connection"] = []any{map[string]any{"id": 1, "name": "Gotify", "implementation": "Gotify",
			"fields": []any{map[string]any{"name": "appToken", "value": "S3CR3T_GOTIFY"}, map[string]any{"name": "server", "value": "http://gotify"}}}}
	} else {
		routes["/api/v3/config/naming"] = naming
		routes["/api/v3/notification"] = []any{}
		routes["/api/v3/languageprofile"] = []any{map[string]any{"id": 1, "name": "English"}}
	}
	for k, v := range extra {
		routes[k] = v
	}
	srv := fakeAPI(t, "X-Api-Key", key, routes)
	return srv.URL, key
}

func TestSonarrAPI(t *testing.T) {
	for _, v5 := range []bool{false, true} {
		u, key := sonarrAPI(t, v5)
		s := read(t, api(Sonarr, u, key))
		bb := findTitle(t, s, "Breaking Bad")
		if bb.Kind != "show" || bb.IDs["tvdb"] != "81189" || bb.IDs["tmdb"] != "1396" || bb.Profile != "Any" || bb.Extra["show_type"] != "standard" ||
			bb.Extra["new_seasons"] != "all" || len(bb.Seasons) != 2 || bb.Seasons[0].Monitored || !bb.Seasons[1].Monitored {
			t.Fatalf("v5=%v show %+v", v5, bb)
		}
		if s.Naming["standardepisodeformat"] == nil {
			t.Fatalf("v5=%v naming %+v", v5, s.Naming)
		}
		if v5 {
			if len(s.Notifications) != 1 || !hasReentry(s, `notification "Gotify": appToken`) || len(s.LanguageProfiles) != 0 || !hasNote(s, "langprofiles") {
				t.Fatalf("v5 %+v / %v / %v", s.Notifications, s.NeedsReentry, s.Notes)
			}
		} else if len(s.LanguageProfiles) != 1 || s.LanguageProfiles[0].Name != "English" {
			t.Fatalf("v4 language profiles %+v", s.LanguageProfiles)
		}
	}
}

func sonarrDB(v3 bool) []string {
	out := []string{
		`CREATE TABLE VersionInfo (Version INTEGER)`,
		`CREATE TABLE Tags (Id INTEGER PRIMARY KEY, Label TEXT)`, `INSERT INTO Tags VALUES (2, 'anime')`,
		`CREATE TABLE RootFolders (Id INTEGER PRIMARY KEY, Path TEXT)`, `INSERT INTO RootFolders VALUES (1, '/tv')`,
		`CREATE TABLE QualityProfiles (Id INTEGER PRIMARY KEY, Name TEXT, Cutoff INTEGER, Items TEXT, UpgradeAllowed INTEGER)`,
		`INSERT INTO QualityProfiles VALUES (1, 'HD-1080p', 3, '[{"quality":3,"items":[],"allowed":true}]', 1)`,
		`CREATE TABLE QualityDefinitions (Id INTEGER PRIMARY KEY, Quality INTEGER, Title TEXT)`, `INSERT INTO QualityDefinitions VALUES (1, 3, 'WEBDL-1080p')`,
		`CREATE TABLE NamingConfig (Id INTEGER PRIMARY KEY, RenameEpisodes INTEGER, StandardEpisodeFormat TEXT, SeriesFolderFormat TEXT)`,
		`INSERT INTO NamingConfig VALUES (1, 1, '{Series Title} - S{season:00}E{episode:00}', '{Series Title}')`,
		`CREATE TABLE ImportListExclusions (Id INTEGER PRIMARY KEY, TvdbId INTEGER, Title TEXT)`, `INSERT INTO ImportListExclusions VALUES (1, 121361, 'Game of Thrones')`,
	}
	if v3 {
		return append(out, `INSERT INTO VersionInfo VALUES (190)`,
			`CREATE TABLE LanguageProfiles (Id INTEGER PRIMARY KEY, Name TEXT, Languages TEXT, Cutoff INTEGER)`,
			`INSERT INTO LanguageProfiles VALUES (1, 'English', '[]', 1)`,
			`CREATE TABLE Series (Id INTEGER PRIMARY KEY, TvdbId INTEGER, ImdbId TEXT, Title TEXT, Year INTEGER, Path TEXT, Monitored INTEGER, ProfileId INTEGER, LanguageProfileId INTEGER, SeriesType INTEGER, SeasonFolder INTEGER, Tags TEXT, Added TEXT, Seasons TEXT)`,
			`INSERT INTO Series VALUES (5, 81189, 'tt0903747', 'Breaking Bad', 2008, '/tv/Breaking Bad', 1, 1, 1, 0, 1, '[2]', '2020-01-01', '[{"seasonNumber":1,"monitored":true},{"seasonNumber":2,"monitored":false}]')`)
	}
	return append(out, `INSERT INTO VersionInfo VALUES (217)`,
		`CREATE TABLE Series (Id INTEGER PRIMARY KEY, TvdbId INTEGER, TmdbId INTEGER, ImdbId TEXT, Title TEXT, Year INTEGER, Path TEXT, Monitored INTEGER, MonitorNewItems INTEGER, QualityProfileId INTEGER, SeriesType INTEGER, SeasonFolder INTEGER, Tags TEXT, Added TEXT, Seasons TEXT)`,
		`INSERT INTO Series VALUES (5, 81189, 1396, 'tt0903747', 'Breaking Bad', 2008, '/tv/Breaking Bad', 1, 0, 1, 0, 1, '[2]', '2023-01-01', '[{"seasonNumber":1,"monitored":true},{"seasonNumber":2,"monitored":false}]')`)
}

func TestSonarrBackupVersions(t *testing.T) {
	for _, v3 := range []bool{true, false} {
		dir := t.TempDir()
		zp := filepath.Join(dir, "sonarr_backup.zip")
		makeZip(t, zp, map[string][]byte{"sonarr.db": dbBytes(t, sonarrDB(v3)...), "config.xml": []byte("<Config><ApiKey>S3CR3T</ApiKey></Config>")})
		s := read(t, file(Sonarr, zp))
		bb := findTitle(t, s, "Breaking Bad")
		if bb.Profile != "HD-1080p" || bb.Folder != "/tv" || !slices.Equal(bb.Tags, []string{"anime"}) || len(bb.Seasons) != 2 || !bb.Seasons[0].Monitored || bb.Seasons[1].Monitored {
			t.Fatalf("v3=%v show %+v", v3, bb)
		}
		if s.QualityProfiles[0].Cutoff != "WEBDL-1080p" {
			t.Fatalf("v3=%v profile %+v", v3, s.QualityProfiles)
		}
		if len(s.Exclusions) != 1 || s.Exclusions[0].IDs["tvdb"] != "121361" {
			t.Fatalf("exclusions %+v", s.Exclusions)
		}
		if v3 && (len(s.LanguageProfiles) != 1 || bb.Extra["language_profile_id"] != int64(1) || s.Schema != 190) {
			t.Fatalf("v3 %+v %+v", s.LanguageProfiles, bb.Extra)
		}
		if !v3 && (len(s.LanguageProfiles) != 0 || !hasNote(s, "LanguageProfiles") || s.Schema != 217) {
			t.Fatalf("v4 %+v %v", s.LanguageProfiles, s.Notes)
		}
	}
}

// TestArrBackupWrongApp: a Radarr backup picked as Sonarr, and a database
// with no version table, are refused.
func TestArrBackupWrongApp(t *testing.T) {
	p := filepath.Join(t.TempDir(), "radarr.db")
	makeDB(t, p, radarrDB()...)
	if _, err := Read(context.Background(), file(Sonarr, p)); !errors.Is(err, ErrNotBackup) {
		t.Fatalf("radarr.db as sonarr: %v", err)
	}
	dir := t.TempDir()
	q := filepath.Join(dir, "sonarr.db")
	makeDB(t, q, `CREATE TABLE Series (Id INTEGER)`)
	if _, err := Read(context.Background(), file(Sonarr, q)); !errors.Is(err, ErrWrongApp) {
		t.Fatalf("no VersionInfo: %v", err)
	}
	// A renamed Radarr database has no Series table.
	b, _ := os.ReadFile(p)
	r := filepath.Join(t.TempDir(), "sonarr.db")
	_ = os.WriteFile(r, b, 0o600)
	if _, err := Probe(context.Background(), file(Sonarr, r)); !errors.Is(err, ErrWrongApp) {
		t.Fatalf("renamed radarr db: %v", err)
	}
}

// TestUnreachableErrorHoldsNoKey: errors never carry the address's query
// or the key.
func TestUnreachableErrorHoldsNoKey(t *testing.T) {
	srv := fakeAPI(t, "", "", map[string]any{})
	u := srv.URL
	srv.Close()
	_, err := Read(context.Background(), Source{Kind: Radarr, URL: u, Key: "S3CR3T_RADARR", AllowLocal: true})
	if !errors.Is(err, ErrUnreachable) || strings.Contains(err.Error(), secretMark) {
		t.Fatalf("error %v", err)
	}
}

// TestArrExclusionsLive: exclusions come over on a live read as well: a
// plain list (Sonarr v3, Radarr) or the paging envelope (Sonarr v5), read
// page by page. When the route fails, the plan says to read a backup.
func TestArrExclusionsLive(t *testing.T) {
	u, key := sonarrAPIWith(t, false, map[string]any{
		"/api/v3/importlistexclusion": []any{map[string]any{"id": 1, "tvdbId": 121361, "title": "Game of Thrones"}}})
	s := read(t, api(Sonarr, u, key))
	if len(s.Exclusions) != 1 || s.Exclusions[0].IDs["tvdb"] != "121361" || s.Exclusions[0].Name != "Game of Thrones" {
		t.Fatalf("sonarr v3 %+v", s.Exclusions)
	}
	u, key = radarrAPIWith(t, map[string]any{
		"/api/v3/exclusions": []any{map[string]any{"id": 1, "tmdbId": 680, "movieTitle": "Pulp Fiction", "movieYear": 1994}}})
	s = read(t, api(Radarr, u, key))
	if len(s.Exclusions) != 1 || s.Exclusions[0].IDs["tmdb"] != "680" || s.Exclusions[0].Year != 1994 || hasNote(s, "exclusion") {
		t.Fatalf("radarr %+v %v", s.Exclusions, s.Notes)
	}
	if _, ok := notMoved(BuildPlan([]Input{{1, s}}, Choices{}), "radarr exclusions"); ok {
		t.Fatal("read exclusions must not be reported as missing")
	}
	// Sonarr v5 pages: three records, two to a page whatever size is asked.
	recs := []any{
		map[string]any{"id": 3, "tvdbId": 1, "title": "One"},
		map[string]any{"id": 2, "tvdbId": 2, "title": "Two"},
		map[string]any{"id": 1, "tvdbId": 3, "title": "Three"},
	}
	u, key = sonarrAPIWith(t, true, map[string]any{"/api/v3/importlistexclusion": func(r *http.Request) any {
		switch r.URL.Query().Get("page") {
		case "", "1":
			return map[string]any{"page": 1, "pageSize": 2, "totalRecords": 3, "records": recs[:2]}
		case "2":
			return map[string]any{"page": 2, "pageSize": 2, "totalRecords": 3, "records": recs[2:]}
		}
		return map[string]any{"totalRecords": 3, "records": []any{}}
	}})
	s = read(t, api(Sonarr, u, key))
	if len(s.Exclusions) != 3 || s.Exclusions[2].IDs["tvdb"] != "3" {
		t.Fatalf("sonarr v5 pages %+v", s.Exclusions)
	}
	// A later page that answers badly skips the list, not the whole read.
	u, key = sonarrAPIWith(t, true, map[string]any{"/api/v3/importlistexclusion": func(r *http.Request) any {
		if p := r.URL.Query().Get("page"); p == "" || p == "1" {
			return map[string]any{"page": 1, "pageSize": 2, "totalRecords": 3, "records": recs[:2]}
		}
		return "not json"
	}})
	s = read(t, api(Sonarr, u, key))
	if len(s.Exclusions) != 0 || len(s.Titles) == 0 {
		t.Fatalf("bad later page %+v %v", s.Exclusions, s.Notes)
	}
	if _, ok := notMoved(BuildPlan([]Input{{1, s}}, Choices{}), "sonarr exclusions"); !ok {
		t.Fatal("a skipped list must be reported")
	}
	// No route: a Not moved line under the Never-add list.
	u, key = radarrAPI(t)
	s = read(t, api(Radarr, u, key))
	n, ok := notMoved(BuildPlan([]Input{{1, s}}, Choices{}), "radarr exclusions")
	if !ok || n.Group != GroupNeverAdd || !strings.Contains(n.Why, "read a backup") {
		t.Fatalf("missing route %+v %v", n, s.Notes)
	}
	// A backup still reads them, with no such line.
	p := filepath.Join(t.TempDir(), "radarr.db")
	makeDB(t, p, radarrDB()...)
	s = read(t, file(Radarr, p))
	if len(s.Exclusions) != 1 || s.Exclusions[0].IDs["tmdb"] != "680" {
		t.Fatalf("backup %+v", s.Exclusions)
	}
	if _, ok := notMoved(BuildPlan([]Input{{1, s}}, Choices{}), "radarr exclusions"); ok {
		t.Fatal("a backup read must not report exclusions as missing")
	}
}

// TestArrReleaseWords: release word rules are read in each shape (Sonarr 4
// and 5 through the API, an older Radarr backup with comma text), and
// Lidarr's format scoring rules are read too.
func TestArrReleaseWords(t *testing.T) {
	u, key := sonarrAPIWith(t, false, map[string]any{"/api/v3/releaseprofile": []any{
		map[string]any{"id": 1, "name": "No HC", "enabled": true, "required": []any{"x265", "hevc"}, "ignored": []any{"HC"}, "indexerId": 0, "tags": []int{}}}})
	s := read(t, api(Sonarr, u, key))
	if len(s.ReleaseProfiles) != 1 {
		t.Fatalf("sonarr 4 %+v", s.ReleaseProfiles)
	}
	rw := s.ReleaseProfiles[0].Settings
	if s.ReleaseProfiles[0].Name != "No HC" || fmt.Sprint(rw["required"]) != "[x265 hevc]" || fmt.Sprint(rw["ignored"]) != "[HC]" || rw["enabled"] != true || rw["indexer_ids"] != nil || rw["tags"] != nil {
		t.Fatalf("sonarr 4 %+v", rw)
	}
	u, key = sonarrAPIWith(t, true, map[string]any{
		"/api/v3/tag": []any{map[string]any{"id": 1, "label": "anime"}, map[string]any{"id": 2, "label": "kids"}},
		"/api/v3/releaseprofile": []any{map[string]any{"id": 2, "name": "Anime groups", "enabled": false, "required": []any{"SubsPlease"}, "ignored": []any{},
			"indexerIds": []int{3}, "tags": []int{1}, "excludedTags": []int{2}}}})
	s = read(t, api(Sonarr, u, key))
	rw = s.ReleaseProfiles[0].Settings
	if fmt.Sprint(rw["tags"]) != "[anime]" || fmt.Sprint(rw["excluded_tags"]) != "[kids]" || fmt.Sprint(rw["indexer_ids"]) != "[3]" || rw["enabled"] != false {
		t.Fatalf("sonarr 5 %+v", rw)
	}
	p := BuildPlan([]Input{{1, s}}, Choices{})
	if n, ok := notMoved(p, "sonarr release_profiles[0]"); !ok || n.Group != GroupRules || n.What != "Anime groups: must contain SubsPlease (tags: anime except tags: kids)" || n.Why != "it was turned off there" {
		t.Fatalf("turned-off rule line %+v", n)
	}

	db := filepath.Join(t.TempDir(), "radarr.db")
	makeDB(t, db, append(radarrDB(),
		`CREATE TABLE ReleaseProfiles (Id INTEGER PRIMARY KEY, Name TEXT, Enabled INTEGER, Required TEXT, Ignored TEXT, Tags TEXT, IndexerId INTEGER)`,
		`INSERT INTO ReleaseProfiles VALUES (1, NULL, 1, 'x265, hevc', 'cam,ts', '[1]', 2)`)...)
	s = read(t, file(Radarr, db))
	rw = s.ReleaseProfiles[0].Settings
	if fmt.Sprint(rw["required"]) != "[x265 hevc]" || fmt.Sprint(rw["ignored"]) != "[cam ts]" || fmt.Sprint(rw["tags"]) != "[kids]" || fmt.Sprint(rw["indexer_ids"]) != "[2]" {
		t.Fatalf("radarr backup %+v", rw)
	}
	p = BuildPlan([]Input{{1, s}}, Choices{})
	n, ok := notMoved(p, "radarr release_profiles[0]")
	if !ok || n.What != "release words: must contain x265, hevc / must not contain cam, ts (tags: kids)" || !strings.Contains(n.Why, "fixed list of traits, not free words") {
		t.Fatalf("radarr rule line %+v", n)
	}
	if n, ok := notMoved(p, "radarr custom_formats[0]"); !ok || n.Why != "format scoring rules do not move; Pick rules score a fixed list of traits" {
		t.Fatalf("format line %+v", n)
	}

	key = "S3CR3T_LIDARR"
	srv := fakeAPI(t, "X-Api-Key", key, map[string]any{
		"/api/v1/system/status": map[string]any{"appName": "Lidarr", "version": "3.1.0.4875"},
		"/api/v1/artist":        []any{},
		"/api/v1/customformat":  []any{map[string]any{"id": 1, "name": "Vinyl", "specifications": []any{}}},
	})
	s = read(t, api(Lidarr, srv.URL, key))
	if len(s.CustomFormats) != 1 || s.CustomFormats[0].Name != "Vinyl" {
		t.Fatalf("lidarr formats %+v", s.CustomFormats)
	}
	if _, ok := notMoved(BuildPlan([]Input{{1, s}}, Choices{}), "lidarr custom_formats[0]"); !ok {
		t.Fatal("a Lidarr format must be on the not-moved list")
	}
}

// TestRadarrAvailabilityDelay: the delay is read from /config/indexer or
// from a backup's Config table; a backup without it sets nothing.
func TestRadarrAvailabilityDelay(t *testing.T) {
	u, key := radarrAPIWith(t, map[string]any{"/api/v3/config/indexer": map[string]any{"id": 1, "availabilityDelay": 400, "maximumSize": 0}})
	s := read(t, api(Radarr, u, key))
	if s.Settings["availability_delay"] != int64(400) || len(s.Settings) != 1 {
		t.Fatalf("api %+v", s.Settings)
	}
	p := filepath.Join(t.TempDir(), "radarr.db")
	makeDB(t, p, append(radarrDB(), `CREATE TABLE Config (Id INTEGER PRIMARY KEY, Key TEXT, Value TEXT)`,
		`INSERT INTO Config VALUES (1, 'apikey', 'S3CR3T_IN_CONFIG'), (2, 'availabilitydelay', '7')`)...)
	if s := read(t, file(Radarr, p)); s.Settings["availability_delay"] != int64(7) || len(s.Settings) != 1 {
		t.Fatalf("backup %+v", s.Settings)
	}
	q := filepath.Join(t.TempDir(), "radarr.db")
	makeDB(t, q, append(radarrDB(), `CREATE TABLE Config (Id INTEGER PRIMARY KEY, Key TEXT, Value TEXT)`, `INSERT INTO Config VALUES (1, 'apikey', 'S3CR3T_IN_CONFIG')`)...)
	if s := read(t, file(Radarr, q)); s.Settings != nil {
		t.Fatalf("backup without the key %+v", s.Settings)
	}
	r := filepath.Join(t.TempDir(), "radarr.db")
	makeDB(t, r, radarrDB()...)
	if s := read(t, file(Radarr, r)); s.Settings != nil {
		t.Fatalf("backup without Config %+v", s.Settings)
	}
}

// TestProwlarrIndexerProxies: proxies are read from the API and from a
// backup, their secrets named; a moved indexer sharing a proxy's tag says
// which proxy it used.
func TestProwlarrIndexerProxies(t *testing.T) {
	key := "S3CR3T_PROWLARR"
	srv := fakeAPI(t, "X-Api-Key", key, map[string]any{
		"/api/v1/system/status": map[string]any{"appName": "Prowlarr", "version": "2.0.5"},
		"/api/v1/tag":           []any{map[string]any{"id": 1, "label": "flare"}, map[string]any{"id": 2, "label": "socks"}},
		"/api/v1/indexer": []any{map[string]any{"id": 1, "name": "Feed", "implementation": "Torznab", "protocol": "torrent", "enable": true, "priority": 25, "tags": []int{1},
			"fields": []any{map[string]any{"name": "baseUrl", "value": "https://feed.example/api"}}}},
		"/api/v1/indexerproxy": []any{
			map[string]any{"id": 1, "name": "Flare", "implementation": "FlareSolverr", "tags": []int{1},
				"fields": []any{map[string]any{"name": "host", "value": "http://flaresolverr:8191/"}, map[string]any{"name": "requestTimeout", "value": 60}}},
			map[string]any{"id": 2, "name": "Socks", "implementation": "Socks5", "tags": []int{2},
				"fields": []any{map[string]any{"name": "host", "value": "proxy"}, map[string]any{"name": "port", "value": 1080},
					map[string]any{"name": "password", "value": "S3CR3T_PROXY", "privacy": "password"}}},
		},
	})
	s := read(t, api(Prowlarr, srv.URL, key))
	if len(s.IndexerProxies) != 2 || s.IndexerProxies[0].Name != "Flare" || !slices.Equal(s.IndexerProxies[0].Tags, []string{"flare"}) || !hasReentry(s, `indexer proxy "Socks": password`) {
		t.Fatalf("proxies %+v %v", s.IndexerProxies, s.NeedsReentry)
	}
	p := BuildPlan([]Input{{1, s}}, Choices{})
	for i := range s.IndexerProxies {
		if n, ok := notMoved(p, fmt.Sprintf("prowlarr indexer_proxies[%d]", i)); !ok || n.Group != GroupSources || !strings.Contains(n.Why, "one proxy address per Source") {
			t.Fatalf("proxy %d line %+v", i, n)
		}
	}
	src := item(t, p, GroupSources, "source:feed")
	if !strings.Contains(src.Detail, "used proxy Flare there; set a proxy on this Source") || strings.Contains(src.Detail, "Socks") || strings.Contains(src.Detail, "only for titles tagged") {
		t.Fatalf("source detail %q", src.Detail)
	}
	if _, ok := notMoved(p, "prowlarr indexers[0].tags"); ok {
		t.Fatal("Prowlarr indexer tags only pick apps and proxies; no tag line")
	}
	if !slices.Contains(p.NeedsInput, `prowlarr: indexer proxy "Socks": password`) {
		t.Fatalf("proxy secret not asked for: %v", p.NeedsInput)
	}

	zp := filepath.Join(t.TempDir(), "prowlarr_backup.zip")
	makeZip(t, zp, map[string][]byte{"prowlarr.db": dbBytes(t,
		`CREATE TABLE VersionInfo (Version INTEGER)`, `INSERT INTO VersionInfo VALUES (40)`,
		`CREATE TABLE Tags (Id INTEGER PRIMARY KEY, Label TEXT)`, `INSERT INTO Tags VALUES (1, 'flare')`,
		`CREATE TABLE Indexers (Id INTEGER PRIMARY KEY, Name TEXT, Implementation TEXT, ConfigContract TEXT, Settings TEXT, Enable INTEGER, Priority INTEGER, Tags TEXT)`,
		`INSERT INTO Indexers VALUES (1, 'Feed', 'Torznab', 'TorznabSettings', '{"baseUrl":"https://feed.example/api"}', 1, 25, '[1]')`,
		`CREATE TABLE IndexerProxies (Id INTEGER PRIMARY KEY, Name TEXT, Implementation TEXT, ConfigContract TEXT, Settings TEXT, Tags TEXT)`,
		`INSERT INTO IndexerProxies VALUES (1, 'Flare', 'FlareSolverr', 'FlareSolverrSettings', '{"host":"http://flaresolverr:8191/","requestTimeout":60}', '[1]')`,
	)})
	b := read(t, file(Prowlarr, zp))
	if len(b.IndexerProxies) != 1 || b.IndexerProxies[0].Name != "Flare" || !slices.Equal(b.IndexerProxies[0].Tags, []string{"flare"}) {
		t.Fatalf("backup proxies %+v", b.IndexerProxies)
	}
	if d := item(t, BuildPlan([]Input{{1, b}}, Choices{}), GroupSources, "source:feed").Detail; !strings.Contains(d, "used proxy Flare there") {
		t.Fatalf("backup source detail %q", d)
	}
}

// TestRadarrWaitSkipsAndListsRead: the wait rule's two skip switches are
// read (a backup without them reads both off), and a Radarr TMDB list and
// Trakt watchlist come out of the API and a backup with the settings the
// move needs (the Trakt account name included) and no token.
func TestRadarrWaitSkipsAndListsRead(t *testing.T) {
	u, key := radarrAPIWith(t, map[string]any{
		"/api/v3/delayprofile": []any{map[string]any{"id": 1, "enableUsenet": true, "enableTorrent": true, "preferredProtocol": "torrent",
			"usenetDelay": 0, "torrentDelay": 120, "bypassIfHighestQuality": true, "bypassIfAboveCustomFormatScore": true, "minimumCustomFormatScore": 50,
			"order": 2147483647, "tags": []int{}}},
		"/api/v3/importlist": []any{map[string]any{"id": 1, "name": "Top", "implementation": "TMDbListImport", "configContract": "TMDbListSettings",
			"enabled": true, "enableAuto": true, "monitor": "movieOnly", "searchOnAdd": true, "qualityProfileId": 1, "rootFolderPath": "/movies",
			"minimumAvailability": "inCinemas", "listType": "tmdb", "tags": []int{},
			"fields": []any{map[string]any{"name": "listId", "value": "8136"}}}},
	})
	s := read(t, api(Radarr, u, key))
	if d := s.DelayProfiles[0]; !d.BypassIfHighestQuality || !d.BypassIfAboveScore || d.TorrentDelay != 120 {
		t.Fatalf("api wait %+v", d)
	}
	p := BuildPlan([]Input{{1, s}}, Choices{})
	if w := item(t, p, GroupRules, "pick_rules_protocol:Standard").Protocol; *w != (ProtocolPlan{Preset: "Standard", Prefer: "torrent", WaitTorrent: 120, SkipAtGoodEnough: true, Part: "movies"}) {
		t.Fatalf("api wait plan %+v", w)
	}
	top := item(t, p, GroupLists, "outside_list:tmdb_list:8136")
	if l := top.OutsideList; l.Library != "/movies" || l.Preset != "Standard" || !l.SearchOnAdd || l.ReleaseGate != "in_cinemas" {
		t.Fatalf("api list %+v", l)
	}
	if !strings.Contains(top.Detail, "(was on there)") {
		t.Errorf("api list detail %q", top.Detail)
	}

	db := filepath.Join(t.TempDir(), "radarr.db")
	makeDB(t, db, append(radarrDB(), `INSERT INTO ImportLists VALUES (2, 'Watch', 'TraktUserImport', '{"traktListType":0,"username":"","authUser":"luis","accessToken":"S3CR3T_TRAKT_USER","refreshToken":"S3CR3T_TRAKT_REFRESH","limit":100}', 'TraktUserSettings', 0, 0, 2, 0, 1, '/movies', 3, '[]')`)...)
	b := read(t, file(Radarr, db))
	if d := b.DelayProfiles[0]; d.BypassIfHighestQuality || d.BypassIfAboveScore {
		t.Fatalf("backup without the skips %+v", d)
	}
	w := b.ImportLists[1]
	if w.Settings["authUser"] != "luis" || w.Settings["link.rootfolderpath"] != "/movies" || !hasReentry(b, `import list "Watch": accessToken`) {
		t.Fatalf("backup list %+v %v", w, b.NeedsReentry)
	}
	bw := item(t, BuildPlan([]Input{{1, b}}, Choices{}), GroupLists, "outside_list:trakt_list:luis/watchlist")
	if !strings.Contains(bw.Detail, "(was off there)") {
		t.Errorf("backup watchlist detail %q", bw.Detail)
	}
	if l := bw.OutsideList; l.KeepUpToDate || l.ReleaseGate != "released" || l.Library != "/movies" {
		t.Fatalf("backup watchlist %+v", l)
	}
}
