package migrate

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestLidarrAPIAndBackup(t *testing.T) {
	key := "S3CR3T_LIDARR"
	srv := fakeAPI(t, "X-Api-Key", key, map[string]any{
		"/api/v1/system/status": map[string]any{"appName": "Lidarr", "version": "3.1.0.4875", "migrationVersion": 80},
		"/api/v1/artist": []any{map[string]any{"id": 3, "artistName": "Daft Punk", "foreignArtistId": "056e4f3e-d505-4dad-8ec1-d04f521cbb56",
			"path": "/music/Daft Punk", "rootFolderPath": "/music", "monitored": true, "monitorNewItems": "all", "qualityProfileId": 1,
			"metadataProfileId": 2, "added": "2024-01-01T00:00:00Z", "tags": []int{}}},
		"/api/v1/rootfolder":      []any{map[string]any{"id": 1, "name": "Music", "path": "/music", "defaultQualityProfileId": 1, "defaultMonitorOption": "all"}},
		"/api/v1/qualityprofile":  []any{map[string]any{"id": 1, "name": "Lossless", "cutoff": 6, "items": []any{map[string]any{"quality": map[string]any{"id": 6, "name": "FLAC"}, "allowed": true}}}},
		"/api/v1/metadataprofile": []any{map[string]any{"id": 2, "name": "Standard", "primaryAlbumTypes": []any{map[string]any{"name": "Album", "allowed": true, "apiKey": "S3CR3T_PROFILE"}}, "secondaryAlbumTypes": []any{map[string]any{"name": "Live", "allowed": true}}, "releaseStatuses": []any{map[string]any{"name": "Official", "allowed": true}}}},
		"/api/v1/album": []any{map[string]any{"id": 8, "artistId": 3, "foreignAlbumId": "11111111-1111-1111-1111-111111111111", "monitored": false,
			"releases": []any{map[string]any{"foreignReleaseId": "22222222-2222-2222-2222-222222222222", "monitored": true}}}},
		"/api/v1/indexer": []any{map[string]any{"id": 1, "name": "Headphones", "implementation": "Headphones", "enableRss": true, "fields": []any{map[string]any{"name": "username", "value": "me"}, map[string]any{"name": "password", "value": "********", "privacy": "password"}}}},
	})
	s := read(t, api(Lidarr, srv.URL, key))
	dp := findTitle(t, s, "Daft Punk")
	if dp.Kind != "artist" || dp.IDs["musicbrainz"] != "056e4f3e-d505-4dad-8ec1-d04f521cbb56" || dp.Profile != "Lossless" || dp.Extra["new_albums"] != "all" || dp.Extra["metadata_profile_id"] != int64(2) {
		t.Fatalf("artist %+v", dp)
	}
	if len(dp.MusicAlbums) != 1 || dp.MusicAlbums[0].Monitored || dp.MusicAlbums[0].WatchedEdition != "22222222-2222-2222-2222-222222222222" {
		t.Fatalf("artist albums %+v", dp.MusicAlbums)
	}
	if p, _ := musicPolicy(s, dp); p == nil || !slices.Equal(p.AlbumTypes, []string{"album"}) || !slices.Equal(p.SecondaryTypes, []string{"live"}) || !slices.Equal(p.ReleaseStatuses, []string{"official"}) {
		t.Fatalf("api metadata policy %+v", p)
	}
	if got := s.MetadataProfiles[0].Settings["primaryalbumtypes"]; strings.Contains(strings.ToLower(fmt.Sprint(got)), "secret") || strings.Contains(strings.ToLower(fmt.Sprint(got)), "apikey") {
		t.Fatalf("metadata profile retained arbitrary fields: %+v", got)
	}
	if len(s.MetadataProfiles) != 1 || s.Folders[0].Extra["name"] != "Music" || s.QualityProfiles[0].Cutoff != "FLAC" || !hasReentry(s, `indexer "Headphones": password`) {
		t.Fatalf("lidarr %+v %+v %+v", s.MetadataProfiles, s.Folders, s.NeedsReentry)
	}
	if !hasNote(s, "customformat") && len(s.CustomFormats) != 0 {
		t.Fatalf("lidarr has no custom formats: %+v", s.CustomFormats)
	}

	zp := filepath.Join(t.TempDir(), "lidarr_backup_v3.1.0.4875_2026.10.14_10.00.00.zip")
	makeZip(t, zp, map[string][]byte{"lidarr.db": dbBytes(t,
		`CREATE TABLE VersionInfo (Version INTEGER)`, `INSERT INTO VersionInfo VALUES (80)`,
		`CREATE TABLE ArtistMetadata (Id INTEGER PRIMARY KEY, ForeignArtistId TEXT, Name TEXT, Overview TEXT)`,
		`INSERT INTO ArtistMetadata VALUES (7, '056e4f3e-d505-4dad-8ec1-d04f521cbb56', 'Daft Punk', '')`,
		`CREATE TABLE Artists (Id INTEGER PRIMARY KEY, ArtistMetadataId INTEGER, CleanName TEXT, Path TEXT, Monitored INTEGER, MonitorNewItems INTEGER, QualityProfileId INTEGER, MetadataProfileId INTEGER, Tags TEXT, Added TEXT)`,
		`INSERT INTO Artists VALUES (3, 7, 'daftpunk', '/music/Daft Punk', 1, 0, 1, 2, '[]', '2024-01-01')`,
		`CREATE TABLE Albums (Id INTEGER PRIMARY KEY, ArtistMetadataId INTEGER, ForeignAlbumId TEXT, Monitored INTEGER)`,
		`INSERT INTO Albums VALUES (8, 7, '11111111-1111-1111-1111-111111111111', 0)`,
		`CREATE TABLE AlbumReleases (Id INTEGER PRIMARY KEY, AlbumId INTEGER, ForeignReleaseId TEXT, Monitored INTEGER)`,
		`INSERT INTO AlbumReleases VALUES (9, 8, '22222222-2222-2222-2222-222222222222', 1)`,
		`CREATE TABLE RootFolders (Id INTEGER PRIMARY KEY, Name TEXT, Path TEXT, DefaultMetadataProfileId INTEGER, DefaultQualityProfileId INTEGER)`,
		`INSERT INTO RootFolders VALUES (1, 'Music', '/music', 2, 1)`,
		`CREATE TABLE MetadataProfiles (Id INTEGER PRIMARY KEY, Name TEXT, PrimaryAlbumTypes TEXT, SecondaryAlbumTypes TEXT, ReleaseStatuses TEXT)`, `INSERT INTO MetadataProfiles VALUES (2, 'Standard', '[{"name":"Album","allowed":true}]', '[{"name":"Live","allowed":true}]', '[{"name":"Official","allowed":true}]')`,
		`CREATE TABLE DownloadClients (Id INTEGER PRIMARY KEY, Name TEXT, Implementation TEXT, Settings TEXT, Enable INTEGER, Priority INTEGER, Tags TEXT)`,
		`INSERT INTO DownloadClients VALUES (1, 'SAB', 'Sabnzbd', '{"host":"sab","port":8080,"apiKey":"S3CR3T_SABKEY","musicCategory":"music"}', 1, 1, '[]')`,
	)})
	b := read(t, file(Lidarr, zp))
	if d := findTitle(t, b, "Daft Punk"); d.IDs["musicbrainz"] == "" || d.Folder != "/music" || !d.Monitored || len(d.MusicAlbums) != 1 || d.MusicAlbums[0].WatchedEdition == "" {
		t.Fatalf("backup artist %+v", d)
	}
	if !hasReentry(b, `download client "SAB": apiKey`) || b.DownloadClients[0].Settings["musicCategory"] != "music" {
		t.Fatalf("backup client %+v %v", b.DownloadClients, b.NeedsReentry)
	}
	if d := findTitle(t, b, "Daft Punk"); func() bool {
		p, _ := musicPolicy(b, d)
		return p != nil && slices.Equal(p.AlbumTypes, []string{"album"}) && slices.Equal(p.SecondaryTypes, []string{"live"}) && slices.Equal(p.ReleaseStatuses, []string{"official"})
	}() == false {
		t.Fatal("backup metadata policy was not retained")
	}
}

func TestProwlarrAPIAndBackup(t *testing.T) {
	key := "S3CR3T_PROWLARR"
	srv := fakeAPI(t, "X-Api-Key", key, map[string]any{
		"/api/v1/system/status": map[string]any{"appName": "Prowlarr", "version": "2.0.5"},
		"/api/v1/indexer": []any{map[string]any{"id": 1, "name": "1337x", "definitionName": "1337x", "implementation": "Cardigann", "protocol": "torrent",
			"enable": true, "priority": 25, "appProfileId": 1, "tags": []int{},
			"fields": []any{map[string]any{"name": "baseUrl", "value": "https://1337x.example/"}, map[string]any{"name": "torrentBaseSettings.seedRatio", "value": 1.5},
				map[string]any{"name": "cookie", "value": "S3CR3T_COOKIE"}}}},
		"/api/v1/applications": []any{map[string]any{"id": 1, "name": "Radarr", "implementation": "Radarr", "syncLevel": "fullSync",
			"fields": []any{map[string]any{"name": "prowlarrUrl", "value": "http://prowlarr:9696"}, map[string]any{"name": "baseUrl", "value": "http://radarr:7878"},
				map[string]any{"name": "apiKey", "value": "S3CR3T_APP"}, map[string]any{"name": "syncCategories", "value": []int{2000}}}}},
		"/api/v1/appprofile": []any{map[string]any{"id": 1, "name": "Standard", "enableRss": true, "minimumSeeders": 1}},
		"/api/v1/tag":        []any{},
	})
	s := read(t, api(Prowlarr, srv.URL, key))
	if len(s.Indexers) != 1 || s.Indexers[0].Settings["link.definitionname"] != "1337x" || s.Indexers[0].Settings["torrentBaseSettings.seedRatio"] != 1.5 || !hasReentry(s, `indexer "1337x": cookie`) {
		t.Fatalf("indexers %+v %v", s.Indexers, s.NeedsReentry)
	}
	if len(s.AppLinks) != 1 || s.AppLinks[0].Settings["baseUrl"] != "http://radarr:7878" || s.AppLinks[0].Settings["link.synclevel"] != "fullSync" || !hasReentry(s, `app link "Radarr": apiKey`) {
		t.Fatalf("app links %+v", s.AppLinks)
	}
	if len(s.AppProfiles) != 1 || len(s.Titles) != 0 || hasNote(s, "exclusion") {
		t.Fatalf("profiles %+v notes %v", s.AppProfiles, s.Notes)
	}
	// An older backup names its database nzbdrone.db.
	zp := filepath.Join(t.TempDir(), "prowlarr_backup.zip")
	makeZip(t, zp, map[string][]byte{"nzbdrone.db": dbBytes(t,
		`CREATE TABLE VersionInfo (Version INTEGER)`, `INSERT INTO VersionInfo VALUES (40)`,
		`CREATE TABLE Indexers (Id INTEGER PRIMARY KEY, Name TEXT, Implementation TEXT, ConfigContract TEXT, Settings TEXT, Enable INTEGER, Priority INTEGER, Redirect INTEGER, Added TEXT, AppProfileId INTEGER, Tags TEXT)`,
		`INSERT INTO Indexers VALUES (1, 'NZBgeek', 'Newznab', 'NewznabSettings', '{"baseUrl":"https://api.nzbgeek.example","apiKey":"S3CR3T_GEEK","baseSettings":{"queryLimit":100}}', 1, 25, 0, '2024', 1, '[]')`,
		`CREATE TABLE Applications (Id INTEGER PRIMARY KEY, Name TEXT, Implementation TEXT, ConfigContract TEXT, Settings TEXT, SyncLevel INTEGER, Tags TEXT)`,
		`INSERT INTO Applications VALUES (1, 'Sonarr', 'Sonarr', 'SonarrSettings', '{"baseUrl":"http://sonarr:8989","apiKey":"S3CR3T_SON"}', 2, '[]')`,
		`CREATE TABLE AppSyncProfiles (Id INTEGER PRIMARY KEY, Name TEXT, EnableRss INTEGER, MinimumSeeders INTEGER)`,
		`INSERT INTO AppSyncProfiles VALUES (1, 'Standard', 1, 1)`,
		`CREATE TABLE IndexerStatus (Id INTEGER PRIMARY KEY, Cookies TEXT)`, `INSERT INTO IndexerStatus VALUES (1, 'S3CR3T_STATUS_COOKIE')`,
		`CREATE TABLE Users (Id INTEGER PRIMARY KEY, Username TEXT, Password TEXT, Salt TEXT)`, `INSERT INTO Users VALUES (1, 'admin', 'S3CR3T_HASH', 'S3CR3T_SALT')`,
	)})
	b := read(t, file(Prowlarr, zp))
	if len(b.Indexers) != 1 || b.Indexers[0].Name != "NZBgeek" || !hasReentry(b, `indexer "NZBgeek": apiKey`) || len(b.AppLinks) != 1 || !hasReentry(b, `app link "Sonarr": apiKey`) {
		t.Fatalf("backup %+v %+v %v", b.Indexers, b.AppLinks, b.NeedsReentry)
	}
}

func TestBazarr(t *testing.T) {
	key := "S3CR3T_BAZARR"
	settings := map[string]any{
		"general":          map[string]any{"enabled_providers": []any{"opensubtitlescom", "podnapisi"}, "serie_default_profile": 1, "flask_secret_key": "S3CR3T_FLASK"},
		"auth":             map[string]any{"apikey": "S3CR3T_AUTH", "type": "form", "username": "admin", "password": "S3CR3T_MD5"},
		"opensubtitlescom": map[string]any{"username": "bob", "password": "S3CR3T_OS", "use_hash": true},
		"podnapisi":        map[string]any{"verify_ssl": true},
		"addic7ed":         map[string]any{"cookies": "S3CR3T_A7"},
		"plex":             map[string]any{"token": "S3CR3T_PLEX", "encryption_key": "S3CR3T_ENC"},
	}
	srv := fakeAPI(t, "X-API-KEY", key, map[string]any{
		"/api/system/status": map[string]any{"data": map[string]any{"bazarr_version": "1.5.3", "python_version": "3.12"}},
		"/api/system/languages": []any{map[string]any{"name": "English", "code2": "en", "code3": "eng", "enabled": true},
			map[string]any{"name": "French", "code2": "fr", "code3": "fra", "enabled": false}, map[string]any{"name": "Dutch", "code2": "nl", "code3": "nld", "enabled": true}},
		"/api/system/languages/profiles": []any{map[string]any{"profileId": 1, "name": "English", "cutoff": 1, "tag": "en",
			"items": []any{map[string]any{"id": 1, "language": "en", "forced": "False", "hi": "True", "audio_exclude": "False"}}}},
		"/api/system/settings": settings,
	})
	s := read(t, api(Bazarr, srv.URL, key))
	sub := s.Subtitles
	if s.Version != "1.5.3" || !slices.Equal(sub.Languages, []string{"en", "nl"}) || len(sub.Profiles) != 1 || sub.Profiles[0].Cutoff != "en" ||
		!sub.Profiles[0].Languages[0].HI || sub.Profiles[0].Languages[0].Forced || sub.Defaults["serie_default_profile"] != int64(1) {
		t.Fatalf("bazarr %+v", sub)
	}
	if len(sub.Providers) != 2 || sub.Providers[0].Name != "opensubtitlescom" || sub.Providers[0].Settings["username"] != "bob" ||
		!hasReentry(s, `subtitle site "opensubtitlescom": password`) || sub.Providers[1].Settings["verify_ssl"] != true {
		t.Fatalf("sites %+v %v", sub.Providers, s.NeedsReentry)
	}
	if _, err := Probe(context.Background(), api(Bazarr, srv.URL, "wrong")); !errors.Is(err, ErrBadKey) {
		t.Fatalf("wrong key: %v", err)
	}

	db := dbBytes(t,
		`CREATE TABLE alembic_version (version_num TEXT)`, `INSERT INTO alembic_version VALUES ('cee6a710cb71')`,
		`CREATE TABLE table_settings_languages (code3 TEXT, code2 TEXT, name TEXT, enabled INTEGER)`,
		`INSERT INTO table_settings_languages VALUES ('eng', 'en', 'English', 1), ('fra', 'fr', 'French', 0)`,
		`CREATE TABLE table_languages_profiles (profileId INTEGER, name TEXT, cutoff INTEGER, items TEXT, mustContain TEXT, mustNotContain TEXT, originalFormat INTEGER, tag TEXT)`,
		`INSERT INTO table_languages_profiles VALUES (1, 'English', 65535, '[{"id":1,"language":"en","forced":"False","hi":"False","audio_exclude":"False"}]', '[]', '[]', 0, NULL)`,
		`CREATE TABLE table_settings_notifier (name TEXT, enabled INTEGER, url TEXT)`, `INSERT INTO table_settings_notifier VALUES ('Discord', 1, 'discord://S3CR3T/x')`,
	)
	yaml := []byte(`auth:
  apikey: S3CR3T_Y
  type: null
general:
  enabled_providers:
  - opensubtitlescom
  movie_default_profile: 1
opensubtitlescom:
  username: bob
  password: S3CR3T_Y2
`)
	zp := filepath.Join(t.TempDir(), "bazarr_backup_v1.5.3.zip")
	makeZip(t, zp, map[string][]byte{"bazarr.db": db, "config.yaml": yaml})
	b := read(t, file(Bazarr, zp))
	if b.Version != "database cee6a710cb71" || !slices.Equal(b.Subtitles.Languages, []string{"en"}) || b.Subtitles.Profiles[0].Cutoff != "any" ||
		len(b.Subtitles.Providers) != 1 || !hasReentry(b, `subtitle site "opensubtitlescom": password`) || b.Subtitles.Defaults["movie_default_profile"] != int64(1) {
		t.Fatalf("yaml backup %+v %v", b.Subtitles, b.NeedsReentry)
	}
	// An older install wrote config.ini (lists as Python text).
	ini := []byte(`[general]
enabled_providers = ['podnapisi', 'opensubtitlescom']
[opensubtitlescom]
username = bob
password = S3CR3T_INI
`)
	zp2 := filepath.Join(t.TempDir(), "bazarr_backup_old.zip")
	makeZip(t, zp2, map[string][]byte{"bazarr.db": db, "config.ini": ini})
	if o := read(t, file(Bazarr, zp2)); len(o.Subtitles.Providers) != 2 || !hasReentry(o, `"opensubtitlescom": password`) {
		t.Fatalf("ini backup %+v", o.Subtitles.Providers)
	}
	// A database alone reads without sites, with a note.
	zp3 := filepath.Join(t.TempDir(), "bazarr_db_only.zip")
	makeZip(t, zp3, map[string][]byte{"bazarr.db": db})
	if o := read(t, file(Bazarr, zp3)); len(o.Subtitles.Providers) != 0 || !hasNote(o, "no settings file") {
		t.Fatalf("db only %+v %v", o.Subtitles, o.Notes)
	}
}

func TestQBittorrent(t *testing.T) {
	srv := fakeAPI(t, "", "", map[string]any{
		"/api/v2/auth/login": func(r *http.Request) any {
			_ = r.ParseForm()
			if r.Form.Get("username") != "admin" || r.Form.Get("password") != "S3CR3T_PASS" {
				return "Fails."
			}
			return "Ok."
		},
		"/api/v2/app/version":         "v5.1.2",
		"/api/v2/app/preferences":     map[string]any{"save_path": "/downloads", "web_ui_port": 8080, "web_ui_username": "admin", "api_key": "S3CR3T_QBKEY", "mail_notification_password": "S3CR3T_SMTP"},
		"/api/v2/torrents/categories": map[string]any{"radarr": map[string]any{"name": "radarr", "savePath": "/downloads/movies"}, "tv": map[string]any{"name": "tv", "savePath": ""}},
	})
	// The fake checks the login itself: a wrong password gets no cookie, and
	// the real app then answers 403; here a login with no password is refused first.
	if _, err := Read(context.Background(), Source{Kind: QBittorrent, URL: srv.URL, Username: "admin", AllowLocal: true}); !errors.Is(err, ErrBadKey) {
		t.Fatalf("no password: %v", err)
	}
	s := read(t, Source{Kind: QBittorrent, URL: srv.URL, Username: "admin", Password: "S3CR3T_PASS", AllowLocal: true})
	d := s.Downloader
	if s.Version != "v5.1.2" || d.SavePath != "/downloads" || d.Username != "admin" || !d.NeedsPassword || len(d.Categories) != 2 || d.Categories[0].Path != "/downloads/movies" {
		t.Fatalf("qbit %+v %+v", s, d)
	}
	if !hasReentry(s, "downloader: needs your password") {
		t.Fatalf("needs %v", s.NeedsReentry)
	}

	conf := []byte("[BitTorrent]\nSession\\DefaultSavePath=/data/torrents\n\n[Preferences]\nWebUI\\Port=8090\nWebUI\\Username=luis\nWebUI\\Password_PBKDF2=\"@ByteArray(S3CR3T_SALT:S3CR3T_HASH)\"\n")
	cats := []byte(`{"radarr":{"save_path":"/data/torrents/movies"},"sonarr":{"save_path":""}}`)
	zp := filepath.Join(t.TempDir(), "qbit.zip")
	makeZip(t, zp, map[string][]byte{"config/qBittorrent.conf": conf, "config/categories.json": cats})
	b := read(t, file(QBittorrent, zp))
	if b.Downloader.Port != 8090 || b.Downloader.Username != "luis" || b.Downloader.SavePath != "/data/torrents" || len(b.Downloader.Categories) != 2 {
		t.Fatalf("qbit backup %+v", b.Downloader)
	}
	// The Windows name of the settings file reads the same.
	zp2 := filepath.Join(t.TempDir(), "qbit-win.zip")
	makeZip(t, zp2, map[string][]byte{"qBittorrent.ini": conf})
	if w := read(t, file(QBittorrent, zp2)); w.Downloader.Port != 8090 {
		t.Fatalf("ini name %+v", w.Downloader)
	}
	if _, err := Probe(context.Background(), file(QBittorrent, zp)); err != nil {
		t.Fatalf("probe backup: %v", err)
	}
}

func TestQBittorrentBearerKey(t *testing.T) {
	srv := fakeAPI(t, "Authorization", "Bearer S3CR3T_QBKEY", map[string]any{
		"/api/v2/app/version":         "v5.2.0",
		"/api/v2/app/preferences":     map[string]any{"save_path": "/dl"},
		"/api/v2/torrents/categories": map[string]any{},
	})
	if info, err := Probe(context.Background(), Source{Kind: QBittorrent, URL: srv.URL, Key: "S3CR3T_QBKEY", AllowLocal: true}); err != nil || info.Version != "v5.2.0" {
		t.Fatalf("bearer %+v %v", info, err)
	}
}

func TestSABnzbd(t *testing.T) {
	key := "S3CR3T_SABAPI"
	srv := fakeAPI(t, "", "", map[string]any{
		"/api": func(r *http.Request) any {
			q := r.URL.Query()
			switch {
			case q.Get("mode") == "version":
				return map[string]any{"version": "4.5.1"}
			case q.Get("apikey") != key:
				return map[string]any{"status": false, "error": "API Key Incorrect"}
			case q.Get("mode") == "get_config":
				return map[string]any{"config": map[string]any{
					"misc": map[string]any{"host": "0.0.0.0", "port": "8080", "url_base": "/sabnzbd", "api_key": "S3CR3T_A", "nzb_key": "S3CR3T_N",
						"username": "luis", "password": "S3CR3T_P", "download_dir": "/incomplete", "complete_dir": "/complete"},
					"categories": []any{map[string]any{"name": "*", "dir": ""}, map[string]any{"name": "tv", "dir": "tv", "priority": -100}},
					"servers": []any{map[string]any{"name": "news.example.com", "displayname": "Example", "host": "news.example.com", "port": 563,
						"username": "u", "password": "S3CR3T_SRV", "ssl": 1, "connections": 20, "enable": 1, "priority": 0}},
				}}
			}
			return map[string]any{}
		},
	})
	gone := fakeAPI(t, "", "", map[string]any{})
	gone.Close()
	// SABnzbd's key rides in the query: an error must not carry it.
	if _, err := Read(context.Background(), Source{Kind: SABnzbd, URL: gone.URL, Key: key, AllowLocal: true}); !errors.Is(err, ErrUnreachable) || strings.Contains(err.Error(), key) {
		t.Fatalf("unreachable: %v", err)
	}
	if _, err := Read(context.Background(), Source{Kind: SABnzbd, URL: srv.URL, Key: "wrong", AllowLocal: true}); !errors.Is(err, ErrBadKey) {
		t.Fatalf("wrong key: %v", err)
	}
	s := read(t, Source{Kind: SABnzbd, URL: srv.URL, Key: key, AllowLocal: true})
	d := s.Downloader
	u, _ := url.Parse(srv.URL)
	if s.Version != "4.5.1" || d.Host != u.Hostname() || d.Port != 8080 || d.URLBase != "/sabnzbd" || d.SavePath != "/incomplete" || d.CompletePath != "/complete" ||
		len(d.Categories) != 2 || d.Categories[1].Path != "tv" || len(d.Servers) != 1 || d.Servers[0].Name != "Example" || d.Servers[0].Settings["connections"] != int64(20) {
		t.Fatalf("sab %+v", d)
	}
	if !hasReentry(s, `usenet server "Example": password`) || !hasReentry(s, "downloader: needs your password or key") {
		t.Fatalf("needs %v", s.NeedsReentry)
	}

	ini := []byte(`__version__ = 19
__encoding__ = utf-8
[misc]
host = ::
port = 8080
https_port = 9090
enable_https = 1
api_key = S3CR3T_K
download_dir = /config/Downloads/incomplete
complete_dir = /config/Downloads/complete
[servers]
[[news.example.com]]
name = news.example.com
host = news.example.com
username = u
password = S3CR3T_SP
[categories]
[[*]]
name = *
[[movies]]
name = movies
dir = movies
[pushover]
pushover_token = S3CR3T_PUSH
`)
	zp := filepath.Join(t.TempDir(), "sab.zip")
	makeZip(t, zp, map[string][]byte{"sabnzbd.ini": ini})
	b := read(t, file(SABnzbd, zp))
	if b.Version != "settings 19" || !b.Downloader.TLS || b.Downloader.Port != 9090 || len(b.Downloader.Categories) != 2 || len(b.Downloader.Servers) != 1 ||
		!hasReentry(b, `usenet server "news.example.com": password`) {
		t.Fatalf("sab backup %+v %v", b.Downloader, b.NeedsReentry)
	}
	if _, err := Probe(context.Background(), file(SABnzbd, zp)); err != nil {
		t.Fatalf("probe ini: %v", err)
	}
	// A qBittorrent settings file is not SABnzbd's.
	zq := filepath.Join(t.TempDir(), "other.zip")
	makeZip(t, zq, map[string][]byte{"sabnzbd.ini": []byte("[Preferences]\nx=1\n")})
	if _, err := Read(context.Background(), file(SABnzbd, zq)); !errors.Is(err, ErrWrongApp) {
		t.Fatalf("wrong ini: %v", err)
	}
}
