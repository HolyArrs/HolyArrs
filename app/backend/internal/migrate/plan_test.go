package migrate

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// fullSnapshots holds one Snapshot per app with every field filled, the
// shapes the readers write (see the readers' tests).
func fullSnapshots() []Input {
	prov := func(name, kind string, set map[string]any, re ...string) Provider {
		return Provider{ID: 1, Name: name, Kind: kind, Enabled: true, Priority: 10, Settings: set, NeedsReentry: re}
	}
	radarr := &Snapshot{Kind: Radarr, Via: ViaAPI, Version: "6.4.4", Schema: 242,
		Titles: []Title{
			{SourceID: 1, Kind: "movie", Name: "The Matrix", Year: 1999, IDs: map[string]string{"tmdb": "603", "imdb": "tt0133093"}, Path: "/movies/The Matrix (1999)",
				Folder: "/movies", Monitored: true, ProfileID: 1, Tags: []string{"kids"}, Extra: map[string]any{"minimum_availability": "released", "minimumAvailability": "released"}},
			{SourceID: 2, Kind: "movie", Name: "No id", Path: "/movies/No id"},
			{SourceID: 3, Kind: "movie", Name: "Elsewhere", IDs: map[string]string{"tmdb": "604"}, Path: "/other/Elsewhere"},
		},
		Tags:            []Tag{{ID: 1, Label: "kids"}},
		Folders:         []Folder{{Path: "/movies", Extra: map[string]any{"name": "Films"}}},
		QualityProfiles: []Profile{{ID: 1, Name: "HD-1080p", UpgradeAllowed: true, Cutoff: "Bluray-1080p", Allowed: []string{"WEBDL-1080p", "Bluray-1080p"}, Scores: map[string]int{"x265": 10}}, {ID: 2, Name: "Ultra-HD", Cutoff: "Remux-2160p"}},
		CustomFormats:   []Named{{ID: 1, Name: "x265"}},
		ReleaseProfiles: []Named{{ID: 1, Name: "No HC", Settings: map[string]any{"enabled": true, "ignored": []any{"HC"}}}},
		DelayProfiles:   []DelayProfile{{ID: 1, EnableTorrent: true, TorrentDelay: 60}},
		Naming:          map[string]any{"renameMovies": true, "colonReplacementFormat": "dash", "standardMovieFormat": "{Movie Title} ({Release Year}) {Quality Full}", "movieFolderFormat": "{Movie Title} ({Release Year}) {Movie Collection}", "includeQuality": true},
		Exclusions:      []Exclusion{{IDs: map[string]string{"tmdb": "999"}, Name: "Bad Movie", Year: 2001}},
		Indexers: []Provider{
			prov("Nyaa", "Torznab", map[string]any{"baseUrl": "http://prowlarr:9696/1/", "minimumSeeders": int64(2), "categories": []any{int64(2000)}, "link.enablerss": true}, "apiKey"),
			prov("Site", "Cardigann", map[string]any{"baseUrl": "https://x.example/"}),
		},
		DownloadClients: []Provider{prov("qBit", "QBittorrent", map[string]any{"host": "qbit", "port": int64(8080), "username": "admin", "movieCategory": "radarr"}, "password"),
			prov("Deluge", "Deluge", map[string]any{"host": "deluge"})},
		RemotePathMappings: []RemotePathMapping{{Host: "qbit", RemotePath: "/downloads/", LocalPath: "/data/downloads/"},
			{Host: "elsewhere", RemotePath: "/dl", LocalPath: "/data/dl"}, {Host: "qbit", RemotePath: "relative", LocalPath: "/data/x"}},
		ImportLists:   []Provider{prov("Trakt", "TraktListImport", nil, "accessToken")},
		Notifications: []Provider{prov("Discord", "Discord", nil, "webHookUrl"), prov("Mail", "Email", map[string]any{"server": "smtp.example", "port": int64(465), "useEncryption": "1", "from": "a@example.org", "to": []any{"b@example.org"}}, "password"), prov("Push", "Pushover", nil)},
		Metadata:      []Provider{prov("Kodi", "XbmcMetadata", nil)},
		Settings:      map[string]any{"availability_delay": int64(7)},
		NeedsReentry:  []string{`indexer "Nyaa": apiKey`, `import list "Trakt": accessToken`},
		Notes:         []string{"a note"},
	}
	sonarr := &Snapshot{Kind: Sonarr, Version: "4.0.0",
		Titles: []Title{{SourceID: 1, Kind: "show", Name: "Lost", IDs: map[string]string{"tvdb": "73739"}, Path: `D:\TV\Lost`, Monitored: true,
			Seasons: []Season{{Number: 0, Monitored: false}, {Number: 1, Monitored: true}}}},
		Folders:          []Folder{{Path: `D:\TV`}},
		LanguageProfiles: []Named{{ID: 1, Name: "English"}},
		Naming:           map[string]any{"standardEpisodeFormat": "{Series Title} - S{season:00}E{episode:00} - {Episode Title}", "seasonFolderFormat": "Season {season}"},
	}
	lidarr := &Snapshot{Kind: Lidarr,
		Titles:           []Title{{SourceID: 1, Kind: "artist", Name: "Björk", IDs: map[string]string{"musicbrainz": "87c5dedd-371d-4a53-9f7f-80522fb7f3cb"}, Path: "/music/Björk", Monitored: false}},
		Folders:          []Folder{{Path: "/music"}},
		QualityProfiles:  []Profile{{ID: 1, Name: "Lossless", Cutoff: "FLAC", Allowed: []string{"FLAC", "ALAC"}}},
		MetadataProfiles: []Named{{ID: 1, Name: "Standard"}},
	}
	prowlarr := &Snapshot{Kind: Prowlarr,
		Indexers:       []Provider{prov("NZBgeek", "Newznab", map[string]any{"baseUrl": "https://api.nzbgeek.example", "apiPath": "/api"}, "apiKey")},
		AppLinks:       []Provider{prov("Radarr", "Radarr", map[string]any{"baseUrl": "http://radarr:7878"}, "apiKey")},
		IndexerProxies: []Provider{prov("Flare", "FlareSolverr", map[string]any{"host": "http://flaresolverr:8191/"})},
		AppProfiles:    []map[string]any{{"name": "Standard"}},
	}
	bazarr := &Snapshot{Kind: Bazarr, Subtitles: &SubtitleSetup{Languages: []string{"en", "es", "fr"},
		Profiles: []SubtitleProfile{{ID: 1, Name: "Main", Languages: []SubtitleLanguage{{Language: "en"}, {Language: "es", HI: true}}}, {ID: 2, Name: "Anime"}},
		Providers: []Provider{{Name: "podnapisi", Kind: "podnapisi"}, {Name: "opensubtitlescom", Kind: "opensubtitlescom", NeedsReentry: []string{"password"}}, {Name: "addic7ed", Kind: "addic7ed"},
			{Name: "yifysubtitles", Kind: "yifysubtitles"}, {Name: "tvsubtitles", Kind: "tvsubtitles"}, {Name: "bsplayer", Kind: "bsplayer"}, {Name: "subf2m", Kind: "subf2m"}},
		Defaults: map[string]any{"movie_default_profile": int64(1), "use_embedded_subs": true}}}
	qbit := &Snapshot{Kind: QBittorrent, Downloader: &Downloader{Host: "10.0.0.5", Port: 8080, Username: "admin", NeedsPassword: true, SavePath: "/downloads",
		Categories: []Category{{Name: "radarr", Path: "/downloads/radarr"}}}}
	sab := &Snapshot{Kind: SABnzbd, Downloader: &Downloader{Host: "sab", Port: 8080, Servers: []Provider{{Name: "news.example"}}}}
	seerr := &Snapshot{Kind: Seerr,
		Users: []User{{SourceID: 1, Name: "luis", Admin: true, Owner: true, Permissions: []string{"ADMIN"}},
			{SourceID: 2, Name: "Ana", Email: "ana@example.org", Permissions: []string{"REQUEST", "AUTO_APPROVE_MOVIE", "REQUEST_4K", "CREATE_ISSUES"}, MovieLimit: 10, MovieDays: 14, ShowLimit: 2}},
		Requests: []Request{{SourceID: 7, Kind: "movie", IDs: map[string]string{"tmdb": "603"}, State: "pending", By: 2, ByName: "Ana"},
			{SourceID: 8, Kind: "show", IDs: map[string]string{"tvdb": "73739", "tmdb": "4607"}, State: "approved", Seasons: []int{1}, By: 2, DecidedBy: 1, FourK: true},
			{SourceID: 9, Kind: "movie", IDs: map[string]string{}, State: "pending", By: 9}},
		Settings: map[string]any{"default_movie_limit": int64(5)},
	}
	return []Input{{1, radarr}, {2, sonarr}, {3, lidarr}, {4, prowlarr}, {5, bazarr}, {6, qbit}, {7, sab}, {8, seerr}}
}

func planOf(t *testing.T, ch Choices) *Plan {
	t.Helper()
	return BuildPlan(fullSnapshots(), ch)
}

// accounted says whether something from `from` is an item or a not-moved entry.
func accounted(p *Plan, from string) bool {
	match := func(f string) bool { return f == from || strings.HasPrefix(f, from+".") }
	for _, f := range p.Mapped {
		if match(f) {
			return true
		}
	}
	for _, g := range p.Groups {
		for _, it := range g.Items {
			if match(it.From) {
				return true
			}
		}
	}
	for _, n := range p.NotMoved {
		if match(n.From) {
			return true
		}
	}
	return false
}

// TestPlanAccountsForEverySnapshotField fails when a Snapshot field is
// neither mapped nor listed as not moved: every field is looked at, every
// list entry and map key of every filled Snapshot ends up somewhere.
func TestPlanAccountsForEverySnapshotField(t *testing.T) {
	in := fullSnapshots()
	p := BuildPlan(in, Choices{PathRules: []PathRule{{From: "D:/TV", To: "/tv"}}})
	st := reflect.TypeOf(Snapshot{})
	for i := 0; i < st.NumField(); i++ {
		name, _, _ := strings.Cut(st.Field(i).Tag.Get("json"), ",")
		if !p.covered[name] {
			t.Errorf("snapshot field %q is never looked at by the plan", name)
		}
	}
	filled := map[string]bool{}
	for _, x := range in {
		v := reflect.ValueOf(*x.Snap)
		for i := 0; i < st.NumField(); i++ {
			name, _, _ := strings.Cut(st.Field(i).Tag.Get("json"), ",")
			f := v.Field(i)
			switch name {
			case "needs_reentry", "notes":
				for _, s := range f.Interface().([]string) {
					// An import list's sign-in is told on that list's own Not moved line.
					if name == "needs_reentry" && importListLeft(p, x.Snap, s) {
						continue
					}
					if !slices.ContainsFunc(append(p.NeedsInput, p.Notes...), func(x string) bool { return strings.HasSuffix(x, s) }) {
						t.Errorf("%s %s %q lost", x.Snap.Kind, name, s)
					}
				}
				continue
			}
			switch f.Kind() {
			case reflect.Slice:
				for j := 0; j < f.Len(); j++ {
					filled[name] = true
					if from := fmt.Sprintf("%s %s[%d]", x.Snap.Kind, name, j); !accounted(p, from) {
						t.Errorf("%s is neither moved nor listed as not moved", from)
					}
				}
			case reflect.Map:
				for _, k := range f.MapKeys() {
					filled[name] = true
					from := fmt.Sprintf("%s %s.%s", x.Snap.Kind, name, k.String())
					if name == "naming" {
						if !accounted(p, from) && !namingMoved(p, x.Snap.Kind, k.String()) {
							t.Errorf("%s is neither moved nor listed", from)
						}
						continue
					}
					if !accounted(p, from) {
						t.Errorf("%s is neither moved nor listed as not moved", from)
					}
				}
			}
		}
		if s := x.Snap.Subtitles; s != nil {
			filled["subtitles"] = true
			for j := range s.Profiles {
				if from := fmt.Sprintf("bazarr subtitles.profiles[%d]", j); !accounted(p, from) {
					t.Errorf("%s lost", from)
				}
			}
			for j := range s.Providers {
				if from := fmt.Sprintf("bazarr subtitles.providers[%d]", j); !accounted(p, from) {
					t.Errorf("%s lost", from)
				}
			}
			for k := range s.Defaults {
				if !accounted(p, "bazarr subtitles.defaults."+k) && !(k == "movie_default_profile" && accounted(p, "bazarr subtitles.profiles[0]")) {
					t.Errorf("subtitle default %s lost", k)
				}
			}
			if !accounted(p, "bazarr subtitles.languages") {
				t.Error("subtitle languages lost")
			}
		}
		if d := x.Snap.Downloader; d != nil {
			filled["downloader"] = true
			if !accounted(p, string(x.Snap.Kind)+" downloader") {
				t.Errorf("%s downloader lost", x.Snap.Kind)
			}
			for j := range d.Categories {
				if !accounted(p, fmt.Sprintf("%s downloader.categories[%d]", x.Snap.Kind, j)) {
					t.Error("category lost")
				}
			}
			for j := range d.Servers {
				if !accounted(p, fmt.Sprintf("%s downloader.servers[%d]", x.Snap.Kind, j)) {
					t.Error("server lost")
				}
			}
		}
	}
	// The fixtures must fill every list and map field, or the test proves little.
	for i := 0; i < st.NumField(); i++ {
		f := st.Field(i)
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		k := f.Type.Kind()
		if (k == reflect.Slice || k == reflect.Map || k == reflect.Pointer) && name != "needs_reentry" && name != "notes" && !filled[name] {
			t.Errorf("no fixture fills %q", name)
		}
	}
}

// importListLeft says a secret named for an import list is accounted by
// that list's Not moved line.
func importListLeft(p *Plan, s *Snapshot, secret string) bool {
	for i, l := range s.ImportLists {
		if strings.HasPrefix(secret, `import list "`+l.Name+`": `) {
			_, ok := notMoved(p, fmt.Sprintf("%s import_lists[%d]", s.Kind, i))
			return ok
		}
	}
	return false
}

func namingMoved(p *Plan, app Kind, key string) bool {
	for _, it := range p.Group(GroupNaming).Items {
		if strings.HasPrefix(it.From, string(app)+" naming") && strings.Contains(it.Detail, key) {
			return true
		}
	}
	return false
}

func item(t *testing.T, p *Plan, group, key string) Item {
	t.Helper()
	for _, it := range p.Group(group).Items {
		if it.Key == key {
			return it
		}
	}
	t.Fatalf("no %s item %q in %+v", group, key, p.Group(group).Items)
	return Item{}
}

func notMoved(p *Plan, from string) (NotMoved, bool) {
	for _, n := range p.NotMoved {
		if n.From == from {
			return n, true
		}
	}
	return NotMoved{}, false
}

func TestPlanMapsEachGroup(t *testing.T) {
	p := planOf(t, Choices{PathRules: []PathRule{{From: `D:\TV`, To: "/tv"}, {From: "/movies", To: "/data/movies"}}, People: map[string]string{"person:seerr:1": PersonOwner}})
	// Folders follow the path rules.
	if f := item(t, p, GroupFolders, "folder:/data/movies").Folder; f.Kind != "movie" || f.OldPath != "/movies" {
		t.Fatalf("movie folder %+v", f)
	}
	item(t, p, GroupFolders, "folder:/tv")
	item(t, p, GroupFolders, "folder:/music")
	// Pick rules: closest preset, with the reason.
	r := item(t, p, GroupRules, "pick_rules:1:radarr:1")
	if r.Rules.Preset != "Standard" || !strings.Contains(r.Why, "custom format scores") {
		t.Fatalf("rules %+v", r)
	}
	if item(t, p, GroupRules, "pick_rules:1:radarr:2").Rules.Preset != "4K" || item(t, p, GroupRules, "pick_rules:3:lidarr:1").Rules.Preset != "Lossless first" {
		t.Fatal("4K or lossless not matched")
	}
	// Titles: adopted in place, the profile's preset, Keep up to date.
	m := item(t, p, GroupTitles, "title:movie:tmdb:603").Title
	if m.Library != "/data/movies" || m.Folder != "The Matrix (1999)" || !m.KeepUpToDate || m.Preset != "Standard" {
		t.Fatalf("movie %+v", m)
	}
	s := item(t, p, GroupTitles, "title:show:tvdb:73739").Title
	if s.Library != "/tv" || s.Folder != "Lost" || !slices.Equal(s.SeasonsOff, []int{0}) {
		t.Fatalf("show %+v", s)
	}
	a := item(t, p, GroupTitles, "title:artist:mbid:87c5dedd-371d-4a53-9f7f-80522fb7f3cb").Title
	if a.KeepUpToDate || a.Folder != "Björk" {
		t.Fatalf("artist %+v", a)
	}
	if n, ok := notMoved(p, "radarr titles[1]"); !ok || !strings.Contains(n.Why, "TMDB") {
		t.Fatalf("no-id movie %+v", n)
	}
	if n, ok := notMoved(p, "radarr titles[2]"); !ok || !strings.Contains(n.Why, "none of the library folders") {
		t.Fatalf("unplaced movie %+v", n)
	}
	// Sources: Torznab with its seeders and categories, paused for its key.
	src := item(t, p, GroupSources, "source:nyaa")
	if src.Source.Format != "torznab" || !src.Source.NeedsKey || *src.Source.MinSeeders != 2 || src.Needs != "your key" || src.Source.Categories["movie"][0] != 2000 {
		t.Fatalf("source %+v", src.Source)
	}
	if g := item(t, p, GroupSources, "source:nzbgeek").Source; g.Format != "newznab" || g.Protocol != "usenet" || g.URL != "https://api.nzbgeek.example/api" {
		t.Fatalf("newznab %+v", g)
	}
	if _, ok := notMoved(p, "radarr indexers[1]"); !ok {
		t.Fatal("a site definition must be listed as not moved")
	}
	// Downloaders: from Radarr's client list and from qBittorrent itself.
	if d := item(t, p, GroupDownloaders, "downloader:http://qbit:8080").Downloader; d.Category != "radarr" || !d.NeedsInput {
		t.Fatalf("downloader %+v", d)
	}
	item(t, p, GroupDownloaders, "downloader:http://10.0.0.5:8080")
	if d := item(t, p, GroupDownloaders, "downloader:http://sab:8080"); d.Needs != "your key" {
		t.Fatalf("sab %+v", d)
	}
	// Naming: the words with a match; a pattern with an unknown word stays.
	nm := item(t, p, GroupNaming, "naming:radarr").Naming
	if nm["file_pattern"] != "{title} ({year}) {quality}" || nm["colon"] != "dash" || nm["rename_on_import"] != "true" {
		t.Fatalf("naming %+v", nm)
	}
	if n, ok := notMoved(p, "radarr naming.movieFolderFormat"); !ok || !strings.Contains(n.Why, "Movie Collection") {
		t.Fatalf("folder format %+v", n)
	}
	if sn := item(t, p, GroupNaming, "naming:sonarr").Naming; sn["episode_pattern"] != "{title} - {episode} - {episode_title}" || sn["season_folder_pattern"] != "Season {season}" {
		t.Fatalf("show naming %+v", sn)
	}
	// Subtitles: the default profile's languages and the site kinds we have.
	langs := item(t, p, GroupSubtitles, "subtitles:languages").Subtitles.Languages
	if len(langs) != 2 || langs[1].Language != "es" || langs[1].HearingImpaired != "hi" {
		t.Fatalf("languages %+v", langs)
	}
	item(t, p, GroupSubtitles, "subtitle_site:podnapisi")
	if n, ok := notMoved(p, "bazarr subtitles.providers[2]"); !ok || !strings.Contains(n.Why, "Gestdown") {
		t.Fatalf("addic7ed %+v", n)
	}
	for _, k := range []string{"yifysubtitles", "tvsubtitles", "bsplayer"} {
		if it := item(t, p, GroupSubtitles, "subtitle_site:"+k); it.Subtitles.SiteKind != k {
			t.Fatalf("%s %+v", k, it)
		}
	}
	if _, ok := notMoved(p, "bazarr subtitles.providers[6]"); !ok {
		t.Fatal("subf2m should stay behind")
	}
	// People: the owner maps to us; others are placeholders. Non-seven-day
	// source windows stay explicit in NotMoved rather than widening access.
	if o := item(t, p, GroupPeople, "person:seerr:1"); o.Person.Role != "owner" || o.Needs != "" {
		t.Fatalf("owner %+v", o)
	}
	ana := item(t, p, GroupPeople, "person:seerr:2").Person
	if ana.Role != "member" || !slices.Equal(ana.Perms, []string{"ask"}) || ana.MoviesPerWeek != nil || ana.ShowsPerWeek != nil {
		t.Fatalf("ana %+v", ana)
	}
	for _, from := range []string{"seerr users[1].movie_quota", "seerr users[1].show_quota"} {
		if _, ok := notMoved(p, from); !ok {
			t.Fatalf("quota window loss not reported: %s", from)
		}
	}
	if n, ok := notMoved(p, "seerr users[1].permissions"); !ok || !strings.Contains(n.What, "REQUEST_4K") {
		t.Fatalf("perms %+v", n)
	}
	// Requests: names from the read titles, attributed to the people.
	ask := item(t, p, GroupRequests, "ask:seerr:7")
	if ask.Name != "The Matrix" || ask.Ask.By != "person:seerr:2" || ask.Ask.Approved {
		t.Fatalf("ask %+v", ask)
	}
	if sh := item(t, p, GroupRequests, "ask:seerr:8").Ask; !sh.Approved || sh.Ref != "tvdb:73739" || sh.Decider != "person:seerr:1" || sh.Note == "" {
		t.Fatalf("show ask %+v", sh)
	}
	// Alerts: Discord and email (off until their secret), others listed.
	if al := item(t, p, GroupAlerts, "alert:mail").Alert; al.SMTPHost != "smtp.example" || al.SMTPPort != 465 || al.SMTPSecurity != "tls" || al.StaffTo != "b@example.org" {
		t.Fatalf("email %+v", al)
	}
	item(t, p, GroupAlerts, "alert:discord")
	if _, ok := notMoved(p, "radarr notifications[2]"); !ok {
		t.Fatal("pushover must be listed")
	}
	// Never-add list.
	if n := item(t, p, GroupNeverAdd, "never_add:movie:tmdb:999").NeverAdd; n.Kind != "movie" {
		t.Fatalf("never add %+v", n)
	}
	if len(p.NeedsInput) == 0 || len(p.Paths) != 3 {
		t.Fatalf("needs %v paths %v", p.NeedsInput, p.Paths)
	}
}

func TestPlanSkipsGroupsAndDedupes(t *testing.T) {
	in := fullSnapshots()
	in = append(in, Input{ID: 9, Snap: in[0].Snap}) // the same Radarr twice
	p := BuildPlan(in, Choices{Skip: []string{GroupAlerts}})
	if !p.Group(GroupAlerts).Skipped || p.Group(GroupTitles).Skipped {
		t.Fatal("skip flags")
	}
	n := 0
	for _, it := range p.Group(GroupTitles).Items {
		if it.Key == "title:movie:tmdb:603" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("movie planned %d times", n)
	}
}

func TestMapPath(t *testing.T) {
	rules := []PathRule{{From: "/data", To: "/mnt/a"}, {From: "/data/movies", To: "/films"}, {From: `D:\Media`, To: "/media"}}
	for in, want := range map[string]string{
		"/data/movies/X (1999)": "/films/X (1999)",
		"/data/tv/Lost":         "/mnt/a/tv/Lost",
		"/database/x":           "/database/x",
		`D:\Media\Shows\Lost`:   "/media/Shows/Lost",
		"/films/":               "/films",
	} {
		if got, ok := MapPath(rules, in); !ok || got != want {
			t.Errorf("%s: %q, want %q", in, got, want)
		}
	}
	for _, bad := range []string{"relative/x", "/data/../etc", "", "C:relative"} {
		if _, ok := MapPath(rules, bad); ok {
			t.Errorf("%q must be refused", bad)
		}
	}
	if ValidRules([]PathRule{{From: "/a", To: "b"}}) == nil || ValidRules([]PathRule{{From: "/a/../b", To: "/b"}}) == nil || ValidRules(rules) != nil {
		t.Fatal("rule checks")
	}
}

// TestPlanHostileSnapshot: names and paths an old app (or a forged backup)
// could hold never reach the plan as they are.
func TestPlanHostileSnapshot(t *testing.T) {
	long := strings.Repeat("x", 5000)
	s := &Snapshot{Kind: Radarr,
		Folders: []Folder{{Path: "/movies"}, {Path: "../../etc"}, {Path: "/movies/../../etc"}},
		Titles: []Title{
			{Name: "Evil\x00\x1b[31m name" + long, IDs: map[string]string{"tmdb": "1"}, Path: "/movies/Evil"},
			{Name: "Climb", IDs: map[string]string{"tmdb": "2"}, Path: "/movies/../../etc/passwd"},
			{Name: "Deep", IDs: map[string]string{"tmdb": "3"}, Path: "/movies/a/b"},
			{Name: "Dot", IDs: map[string]string{"tmdb": "4"}, Path: "/movies/.hidden"},
			{Name: "Bad id", IDs: map[string]string{"tmdb": "1; DROP TABLE"}, Path: "/movies/Bad"},
		},
		Indexers: []Provider{{Name: "Creds", Kind: "Torznab", Settings: map[string]any{"baseUrl": "http://user:pw@host/"}},
			{Name: "Js", Kind: "Torznab", Settings: map[string]any{"baseUrl": "javascript:alert(1)"}}},
	}
	p := BuildPlan([]Input{{1, s}}, Choices{})
	if len(p.Group(GroupFolders).Items) != 1 {
		t.Fatalf("folders %+v", p.Group(GroupFolders).Items)
	}
	titles := p.Group(GroupTitles).Items
	if len(titles) != 1 || strings.ContainsAny(titles[0].Name, "\x00\x1b") || len([]rune(titles[0].Name)) > maxName {
		t.Fatalf("titles %+v", titles)
	}
	if len(p.Group(GroupSources).Items) != 0 {
		t.Fatalf("sources %+v", p.Group(GroupSources).Items)
	}
	// Caps: a flood of titles stops at the cap, the rest listed.
	many := &Snapshot{Kind: Sonarr, Folders: []Folder{{Path: "/tv"}}}
	for i := 1; i <= MaxPlanOthers+5; i++ {
		many.Exclusions = append(many.Exclusions, Exclusion{IDs: map[string]string{"tvdb": fmt.Sprint(i)}})
	}
	p = BuildPlan([]Input{{1, many}}, Choices{})
	if len(p.Group(GroupNeverAdd).Items) != MaxPlanOthers || len(p.NotMoved) != 5 {
		t.Fatalf("cap: %d items, %d not moved", len(p.Group(GroupNeverAdd).Items), len(p.NotMoved))
	}
}

// TestPlanImportListLines: each list gets the reason that fits its app and
// kind, and a list's sign-in is never asked for under Needs your input.
func TestPlanImportListLines(t *testing.T) {
	sonarr := &Snapshot{Kind: Sonarr, ImportLists: []Provider{{Name: "Trakt shows", Kind: "TraktListImport", NeedsReentry: []string{"accessToken"}}}}
	radarr := &Snapshot{Kind: Radarr, ImportLists: []Provider{{Name: "Top films", Kind: "TMDbListImport"}, {Name: "Watch", Kind: "TraktListImport", NeedsReentry: []string{"accessToken"}},
		{Name: "Other box", Kind: "RadarrImport", NeedsReentry: []string{"apiKey"}}, {Name: "Picks", Kind: "IMDbListImport"}}}
	lidarr := &Snapshot{Kind: Lidarr, ImportLists: []Provider{{Name: "Saved albums", Kind: "SpotifySavedAlbums", NeedsReentry: []string{"refreshToken"}}}}
	for _, s := range []*Snapshot{sonarr, radarr, lidarr} {
		s.finish()
	}
	p := BuildPlan([]Input{{1, sonarr}, {2, radarr}, {3, lidarr}}, Choices{})
	// A Radarr TMDB or Trakt list without its list number or names cannot
	// come over as an Outside list; it says what is missing.
	for from, want := range map[string]NotMoved{
		"sonarr import_lists[0]": {GroupOther, "", "", "HolyArrs Outside lists add movies only for now; this show list stays in Sonarr; its sign-in does not come over"},
		"radarr import_lists[0]": {GroupLists, "", "", "its TMDB list number did not come over; add it again under Settings > Outside lists"},
		"radarr import_lists[1]": {GroupLists, "", "", "its Trakt user or list name did not come over; add it again under Settings > Outside lists"},
		"radarr import_lists[2]": {GroupOther, "", "", "HolyArrs is one app; it does not copy from another instance; its sign-in does not come over"},
		"radarr import_lists[3]": {GroupOther, "", "", "HolyArrs reads TMDB lists and public Trakt lists only; this list stays in Radarr"},
		"lidarr import_lists[0]": {GroupOther, "", "", "HolyArrs Outside lists add movies only for now; this music list stays in Lidarr; its sign-in does not come over"},
	} {
		if n, ok := notMoved(p, from); !ok || n.Group != want.Group || n.Why != want.Why {
			t.Errorf("%s: %+v", from, n)
		}
	}
	for _, n := range p.NeedsInput {
		if strings.Contains(n, "import list") {
			t.Fatalf("an import list secret is asked for: %v", p.NeedsInput)
		}
	}
}

// TestPlanTagScope: a moved Source, Downloader or Alert that had tags says
// it serves every title of its kind and lists the tags; an untagged
// one says nothing. Delay rules name their scope.
func TestPlanTagScope(t *testing.T) {
	s := &Snapshot{Kind: Radarr,
		Indexers: []Provider{{Name: "Nyaa", Kind: "Torznab", Tags: []string{"kids", "anime"}, Settings: map[string]any{"baseUrl": "http://prowlarr:9696/1/"}},
			{Name: "Plain", Kind: "Torznab", Settings: map[string]any{"baseUrl": "http://prowlarr:9696/2/"}}},
		DownloadClients: []Provider{{Name: "qBit", Kind: "QBittorrent", Tags: []string{"kids"}, Settings: map[string]any{"host": "qbit", "port": int64(8080)}},
			{Name: "qBit2", Kind: "QBittorrent", Settings: map[string]any{"host": "qbit2", "port": int64(8080)}}},
		Notifications: []Provider{{Name: "Discord", Kind: "Discord", Tags: []string{"kids"}}, {Name: "Hook", Kind: "Webhook"}},
		DelayProfiles: []DelayProfile{{ID: 2, EnableUsenet: true, EnableTorrent: true, PreferredProtocol: "torrent", UsenetDelay: 30, TorrentDelay: 0, Order: 1, Tags: []string{"kids"}},
			{ID: 1, EnableUsenet: true, PreferredProtocol: "usenet", TorrentDelay: 60, Order: 2147483647}},
	}
	p := BuildPlan([]Input{{1, s}}, Choices{})
	for _, c := range []struct{ group, key, from string }{
		{GroupSources, "source:nyaa", "radarr indexers[0]"},
		{GroupDownloaders, "downloader:http://qbit:8080", "radarr download_clients[0]"},
		{GroupAlerts, "alert:discord", "radarr notifications[0]"},
	} {
		it := item(t, p, c.group, c.key)
		if !strings.Contains(it.Detail, "only for titles tagged ") || !strings.Contains(it.Detail, " there; here it serves every movie title") {
			t.Errorf("%s detail %q", c.key, it.Detail)
		}
		want := "HolyArrs has no tags; check it before you resume it"
		if c.group == GroupAlerts {
			want = "HolyArrs has no tags; check it before you turn it on"
		}
		if n, ok := notMoved(p, c.from+".tags"); !ok || n.Group != c.group || n.Why != want {
			t.Errorf("%s tag line %+v", c.key, n)
		}
	}
	if d := item(t, p, GroupSources, "source:nyaa").Detail; !strings.Contains(d, "tagged kids, anime there") {
		t.Errorf("both tags named: %q", d)
	}
	for _, c := range []struct{ group, key, from string }{
		{GroupSources, "source:plain", "radarr indexers[1]"},
		{GroupDownloaders, "downloader:http://qbit2:8080", "radarr download_clients[1]"},
		{GroupAlerts, "alert:hook", "radarr notifications[1]"},
	} {
		if it := item(t, p, c.group, c.key); strings.Contains(it.Detail, "tagged") {
			t.Errorf("%s untagged detail %q", c.key, it.Detail)
		}
		if _, ok := notMoved(p, c.from+".tags"); ok {
			t.Errorf("%s untagged got a tag line", c.key)
		}
	}
	if n, _ := notMoved(p, "radarr delay_profiles[0]"); n.What != "wait before grabbing (tags: kids)" ||
		n.Why != "applied only to titles tagged kids; HolyArrs has no tags, so its waits (prefer torrent, Usenet 30 min, torrent 0 min) were not imported; set them on a copy of the Pick rules if those titles need them" {
		t.Errorf("tagged delay %+v", n)
	}
	// With no quality profile read, the default rule has no Pick rules to go to.
	if n, _ := notMoved(p, "radarr delay_profiles[1]"); n.What != "wait before grabbing (every title)" || n.Why != "no quality profile came over to carry it: prefer usenet, Usenet 0 min, torrent 60 min, torrent off; set them under Pick rules" {
		t.Errorf("torrent-off delay %+v", n)
	}
	if n, _ := notMoved(p, "radarr delay_profiles[1].enable_torrent"); n.Why != "Pick rules cannot turn a protocol off, so pause the torrent Sources instead; Usenet is preferred" {
		t.Errorf("torrent-off delay %+v", n)
	}
	// The old app's untouched default asks nothing; a long name keeps its tags.
	s.DelayProfiles = []DelayProfile{{ID: 1, EnableUsenet: true, EnableTorrent: true, PreferredProtocol: "usenet", Order: 2147483647}}
	s.Indexers[0].Name = strings.Repeat("N", 300)
	p = BuildPlan([]Input{{1, s}}, Choices{})
	if n, ok := notMoved(p, "radarr delay_profiles[0]"); ok || !slices.Contains(p.Mapped, "radarr delay_profiles[0]") {
		t.Errorf("default delay %+v %v", n, p.Mapped)
	}
	if n, _ := notMoved(p, "radarr indexers[0].tags"); !strings.HasSuffix(n.What, " (tags: kids, anime)") {
		t.Errorf("long name tag line %q", n.What)
	}
}

// TestPlanReleaseWordsLine: a rule with no words has no empty part, and a
// long rule keeps its scope.
func TestPlanReleaseWordsLine(t *testing.T) {
	s := &Snapshot{Kind: Sonarr, ReleaseProfiles: []Named{{Name: "Empty", Settings: map[string]any{"enabled": true}},
		{Name: "Long", Settings: map[string]any{"enabled": true, "required": []any{strings.Repeat("w", 300)}, "tags": []any{"anime"}}}}}
	p := BuildPlan([]Input{{1, s}}, Choices{})
	if n, _ := notMoved(p, "sonarr release_profiles[0]"); n.What != "Empty (every title)" {
		t.Errorf("empty rule %q", n.What)
	}
	if n, _ := notMoved(p, "sonarr release_profiles[1]"); !strings.HasSuffix(n.What, " (tags: anime)") {
		t.Errorf("long rule %q", n.What)
	}
}

// TestPlanReleaseGate: Radarr's release stage becomes each movie's Release
// gate and its availability delay the gate delay (cut to 365); an unknown
// stage keeps this server's default and is listed. The delay is never
// shown as a Request setting.
func TestPlanReleaseGate(t *testing.T) {
	movie := func(id int, stage string) Title {
		n := fmt.Sprint("M", id)
		return Title{SourceID: int64(id), Kind: "movie", Name: n, IDs: map[string]string{"tmdb": fmt.Sprint(id)}, Path: "/movies/" + n, Monitored: true,
			Extra: map[string]any{"minimum_availability": stage}}
	}
	s := &Snapshot{Kind: Radarr, Folders: []Folder{{Path: "/movies"}},
		Titles:   []Title{movie(1, "released"), movie(2, "inCinemas"), movie(3, "tba"), movie(4, "someday"), movie(5, "2")},
		Settings: map[string]any{"availability_delay": int64(400)}}
	p := BuildPlan([]Input{{1, s}}, Choices{})
	for id, want := range map[int]string{1: "released", 2: "in_cinemas", 3: "announced", 4: "", 5: "in_cinemas"} {
		it := item(t, p, GroupTitles, fmt.Sprint("title:movie:tmdb:", id))
		if it.Title.ReleaseGate != want || it.Title.GateDelayDays == nil || *it.Title.GateDelayDays != 365 {
			t.Errorf("movie %d: %+v", id, it.Title)
		}
		if !strings.Contains(it.Detail, "(+365 days, cut to 365)") {
			t.Errorf("movie %d detail %q", id, it.Detail)
		}
	}
	if d := item(t, p, GroupTitles, "title:movie:tmdb:2").Detail; !strings.Contains(d, "release gate: in cinemas (+365 days, cut to 365)") {
		t.Errorf("detail %q", d)
	}
	if n, ok := notMoved(p, "radarr titles[3].minimum_availability"); !ok || n.What != "M4: release stage someday" || n.Why != "unknown release stage someday; the title takes this server's default" {
		t.Errorf("unknown stage %+v", n)
	}
	if _, ok := notMoved(p, "radarr titles.minimum_availability"); ok {
		t.Error("the release stage is moved, not listed as an extra")
	}
	for _, n := range p.NotMoved {
		if strings.Contains(n.From, "availability_delay") {
			t.Errorf("the delay is listed as not moved: %+v", n)
		}
	}
	if !slices.Contains(p.Mapped, "radarr settings.availability_delay") || !slices.ContainsFunc(p.Notes, func(n string) bool {
		return strings.Contains(n, "this server's default for new movies stays as it is")
	}) {
		t.Errorf("mapped %v notes %v", p.Mapped, p.Notes)
	}
	// Without a delay the gate alone is set, and the server's delay applies.
	s.Settings = nil
	p = BuildPlan([]Input{{1, s}}, Choices{})
	if it := item(t, p, GroupTitles, "title:movie:tmdb:1"); it.Title.GateDelayDays != nil || !strings.HasSuffix(it.Detail, "; release gate: released") {
		t.Errorf("no delay %+v %q", it.Title, it.Detail)
	}
}

// waitSnap is a read whose one quality profile matches Standard, with
// the given wait rules.
func waitSnap(kind Kind, ds ...DelayProfile) *Snapshot {
	return &Snapshot{Kind: kind, QualityProfiles: []Profile{{ID: 1, Name: "HD", UpgradeAllowed: true, Cutoff: "Bluray-1080p"}}, DelayProfiles: ds}
}

// waitItem returns the planned wait change for a ready-made Pick rules (nil
// when there is none).
func waitItem(p *Plan, preset string) *Item {
	for _, it := range p.Group(GroupRules).Items {
		if it.Key == "pick_rules_protocol:"+preset {
			return &it
		}
	}
	return nil
}

// TestPlanDefaultWait: the untagged default wait rule becomes the wait of
// the ready-made Pick rules its app's profiles matched, unless it is the old
// app's untouched default; waits are cut to 7 days, a protocol switched off
// or a score skip is listed, tagged rules are listed, and apps that disagree
// change nothing.
func TestPlanDefaultWait(t *testing.T) {
	def := DelayProfile{ID: 1, EnableUsenet: true, EnableTorrent: true, PreferredProtocol: "usenet", Order: 2147483647}
	// The factory default plans nothing and lists nothing.
	p := BuildPlan([]Input{{1, waitSnap(Radarr, def)}}, Choices{})
	if it := waitItem(p, "Standard"); it != nil {
		t.Fatalf("factory default planned %+v", it)
	}
	if _, ok := notMoved(p, "radarr delay_profiles[0]"); ok || !slices.Contains(p.Mapped, "radarr delay_profiles[0]") {
		t.Fatalf("factory default not mapped: %v", p.Mapped)
	}
	// Its skip switches do nothing without a wait, so it stays the default.
	f := def
	f.BypassIfHighestQuality, f.BypassIfAboveScore = true, true
	p = BuildPlan([]Input{{1, waitSnap(Radarr, f)}}, Choices{})
	if it := waitItem(p, "Standard"); it != nil || len(p.NotMoved) != 0 {
		t.Fatalf("factory default with skips planned %+v %+v", it, p.NotMoved)
	}

	// A torrent wait of 120 minutes, Usenet preferred.
	d := def
	d.TorrentDelay = 120
	tagged := DelayProfile{ID: 2, EnableUsenet: true, EnableTorrent: true, PreferredProtocol: "torrent", UsenetDelay: 30, Order: 1, Tags: []string{"anime"}}
	p = BuildPlan([]Input{{1, waitSnap(Radarr, tagged, d)}}, Choices{})
	it := waitItem(p, "Standard")
	if it == nil || *it.Protocol != (ProtocolPlan{Preset: "Standard", Prefer: "usenet", WaitTorrent: 120, Part: "movies"}) || it.From != "radarr delay_profiles[1]" {
		t.Fatalf("wait item %+v", it)
	}
	if it.Detail != "from the default wait rule of radarr: prefer usenet, Usenet 0 min, torrent 120 min" {
		t.Errorf("detail %q", it.Detail)
	}
	if n, ok := notMoved(p, "radarr delay_profiles[0]"); !ok || !strings.HasPrefix(n.Why, "applied only to titles tagged anime; HolyArrs has no tags, so its waits (prefer torrent, Usenet 30 min, torrent 0 min) were not imported") {
		t.Errorf("tagged rule %+v", n)
	}

	// Waits over 7 days are cut; the top-quality skip becomes the Good
	// enough skip with a note; a score skip is listed.
	d = def
	d.UsenetDelay, d.TorrentDelay, d.BypassIfHighestQuality, d.BypassIfAboveScore = 20000, 60, true, true
	p = BuildPlan([]Input{{1, waitSnap(Radarr, d)}}, Choices{})
	it = waitItem(p, "Standard")
	if it == nil || *it.Protocol != (ProtocolPlan{Preset: "Standard", Prefer: "usenet", WaitUsenet: 10080, WaitTorrent: 60, SkipAtGoodEnough: true, Part: "movies"}) {
		t.Fatalf("cut wait %+v", it)
	}
	for _, want := range []string{"Usenet 20000 min cut to 10080 (HolyArrs allows up to 7 days)", "no wait once a release is Good enough", "which can be below the top allowed quality"} {
		if !strings.Contains(it.Detail, want) {
			t.Errorf("detail %q lacks %q", it.Detail, want)
		}
	}
	if n, ok := notMoved(p, "radarr delay_profiles[0].bypass_score"); !ok || !strings.HasPrefix(n.Why, "format scores are not imported") {
		t.Errorf("score skip %+v", n)
	}

	// Only torrent on: torrent is preferred and the switch is listed.
	d = def
	d.EnableUsenet, d.TorrentDelay = false, 10
	p = BuildPlan([]Input{{1, waitSnap(Radarr, d)}}, Choices{})
	if it = waitItem(p, "Standard"); it == nil || it.Protocol.Prefer != "torrent" || it.Protocol.WaitTorrent != 10 {
		t.Fatalf("torrent only %+v", it)
	}
	if n, ok := notMoved(p, "radarr delay_profiles[0].enable_usenet"); !ok || n.Why != "Pick rules cannot turn a protocol off, so pause the Usenet Sources instead; torrent is preferred" {
		t.Errorf("usenet off %+v", n)
	}

	// Radarr and Sonarr disagree on Standard: nothing changes there and
	// both are listed with their values.
	r := def
	r.TorrentDelay = 120
	s := def
	s.UsenetDelay = 15
	p = BuildPlan([]Input{{1, waitSnap(Radarr, r)}, {2, waitSnap(Sonarr, s)}}, Choices{})
	if it := waitItem(p, "Standard"); it != nil {
		t.Fatalf("disagreeing apps planned %+v", it)
	}
	for from, values := range map[string]string{"radarr delay_profiles[0]": "radarr had prefer usenet, Usenet 0 min, torrent 120 min", "sonarr delay_profiles[0]": "sonarr had prefer usenet, Usenet 15 min, torrent 0 min"} {
		if n, ok := notMoved(p, from); !ok || n.Why != "the old apps set different waits for Standard, so this server's wait stays; "+values {
			t.Errorf("%s: %+v", from, n)
		}
	}
	// An app left at its default counts as no wait, so it disagrees too.
	p = BuildPlan([]Input{{1, waitSnap(Radarr, def)}, {2, waitSnap(Sonarr, r)}}, Choices{})
	if it := waitItem(p, "Standard"); it != nil {
		t.Fatalf("default and a wait planned %+v", it)
	}
	for _, from := range []string{"radarr delay_profiles[0]", "sonarr delay_profiles[0]"} {
		if n, ok := notMoved(p, from); !ok || !strings.HasPrefix(n.Why, "the old apps set different waits for Standard") {
			t.Errorf("%s: %+v", from, n)
		}
	}
	// When they agree, one change names both; a second profile on 4K gets its own.
	ra := waitSnap(Radarr, r)
	ra.QualityProfiles = append(ra.QualityProfiles, Profile{ID: 2, Name: "Ultra-HD", Cutoff: "Remux-2160p"})
	p = BuildPlan([]Input{{1, ra}, {2, waitSnap(Sonarr, r)}}, Choices{})
	if it := waitItem(p, "Standard"); it == nil || !strings.HasPrefix(it.Detail, "from the default wait rule of radarr and sonarr: ") || !slices.Contains(p.Mapped, "sonarr delay_profiles[0]") {
		t.Fatalf("agreeing apps %+v %v", it, p.Mapped)
	}
	if it := waitItem(p, "Standard"); it.Protocol.Part != "" {
		t.Errorf("a change from two parts has part %q", it.Protocol.Part)
	}
	if it := waitItem(p, "4K"); it == nil || it.Protocol.WaitTorrent != 120 || it.Protocol.Part != "movies" {
		t.Fatalf("4K wait %+v", it)
	}
}

// listSnap is a Radarr read with one movie folder, an HD and an Ultra-HD
// profile, and the given lists.
func listSnap(ls ...Provider) *Snapshot {
	return &Snapshot{Kind: Radarr, Folders: []Folder{{Path: "/movies"}},
		QualityProfiles: []Profile{{ID: 1, Name: "HD", Cutoff: "Bluray-1080p"}, {ID: 2, Name: "Ultra-HD", Cutoff: "Remux-2160p"}}, ImportLists: ls}
}

// TestPlanOutsideLists: Radarr TMDB lists, Trakt lists and Trakt watchlists
// become paused Outside lists with their folder, Pick rules, keep up to
// date, search on add, Release gate and refresh gap; a folder that did not
// come over, a watched or collection list and the other kinds are listed.
func TestPlanOutsideLists(t *testing.T) {
	s := listSnap(
		Provider{Name: "Top", Kind: "TMDbListImport", Enabled: true, Settings: map[string]any{"listId": "8136", "link.rootfolderpath": "/movies",
			"link.qualityprofileid": int64(2), "link.monitor": "movieOnly", "link.searchonadd": true, "link.minimumavailability": "inCinemas", "link.minrefreshinterval": "1.00:00:00"}},
		Provider{Name: "Favs", Kind: "TraktListImport", Settings: map[string]any{"username": "Sean", "listname": "My Favs!", "link.rootfolderpath": "/movies",
			"link.monitor": int64(2), "link.minimumavailability": int64(3)}, NeedsReentry: []string{"accessToken"}},
		Provider{Name: "Watch", Kind: "TraktUserImport", Enabled: true, Settings: map[string]any{"traktListType": int64(0), "username": "", "authUser": "luis", "link.rootfolderpath": "/movies/",
			"link.monitor": "movieAndCollection", "link.enabled": true, "link.enableauto": false}},
		Provider{Name: "Shelf", Kind: "TraktUserImport", Settings: map[string]any{"traktListType": int64(2), "username": "luis", "link.rootfolderpath": "/movies"}},
		Provider{Name: "Away", Kind: "TMDbListImport", Settings: map[string]any{"listId": "77", "link.rootfolderpath": "/elsewhere"}},
		Provider{Name: "top", Kind: "TMDbListImport", Enabled: true, Settings: map[string]any{"listId": "99", "link.rootfolderpath": "/movies", "link.enabled": false, "link.enableauto": true}},
		Provider{Name: "Picks", Kind: "IMDbListImport"},
	)
	s.finish()
	p := BuildPlan([]Input{{1, s}}, Choices{PathRules: []PathRule{{From: "/movies", To: "/data/movies"}}})
	top := item(t, p, GroupLists, "outside_list:tmdb_list:8136")
	if *top.OutsideList != (OutsideListPlan{Kind: "tmdb_list", Ref: "8136", Library: "/data/movies", Preset: "4K", KeepUpToDate: true, SearchOnAdd: true, ReleaseGate: "in_cinemas", SyncEveryHours: 24}) || top.Name != "Top" {
		t.Fatalf("tmdb list %+v %+v", top, top.OutsideList)
	}
	if top.Detail != "TMDB list 8136 into /data/movies; added paused (was on there), every 24 hours; release gate: in cinemas" {
		t.Errorf("detail %q", top.Detail)
	}
	favs := item(t, p, GroupLists, "outside_list:trakt_list:sean/my-favs")
	if *favs.OutsideList != (OutsideListPlan{Kind: "trakt_list", Ref: "Sean/my-favs", Library: "/data/movies", ReleaseGate: "released", SyncEveryHours: 12}) {
		t.Fatalf("trakt list %+v", favs.OutsideList)
	}
	if !strings.Contains(favs.Detail, "(was off there)") || !strings.Contains(favs.Detail, "Keep up to date off") || !strings.HasSuffix(favs.Detail, "needs your Trakt key, and the list must be public") {
		t.Errorf("trakt detail %q", favs.Detail)
	}
	watch := item(t, p, GroupLists, "outside_list:trakt_list:luis/watchlist")
	if w := watch.OutsideList; w.Ref != "luis/watchlist" || !w.KeepUpToDate || w.ReleaseGate != "" {
		t.Fatalf("watchlist %+v", w)
	}
	// On but not adding by itself, and off but adding (Radarr never read it).
	if !strings.Contains(watch.Detail, "(was on there but added no movies by itself)") {
		t.Errorf("watchlist detail %q", watch.Detail)
	}
	if n, ok := notMoved(p, "radarr import_lists[2].monitor"); !ok || n.Why != "lists add only the movies on the list, not the rest of their movie series; add those by hand" {
		t.Errorf("collection part %+v", n)
	}
	if clash := item(t, p, GroupLists, "outside_list:tmdb_list:99"); clash.Name != "top (radarr)" || !strings.Contains(clash.Detail, "(was off there)") {
		t.Errorf("name clash %q %q", clash.Name, clash.Detail)
	}
	for from, want := range map[string]NotMoved{
		"radarr import_lists[3]": {GroupLists, "", "", "HolyArrs reads Trakt lists and watchlists; a watched or collection list stays in Radarr"},
		"radarr import_lists[4]": {GroupLists, "", "", "its library folder /elsewhere did not come over"},
		"radarr import_lists[6]": {GroupOther, "", "", "HolyArrs reads TMDB lists and public Trakt lists only; this list stays in Radarr"},
	} {
		if n, ok := notMoved(p, from); !ok || n.Group != want.Group || n.Why != want.Why {
			t.Errorf("%s: %+v", from, n)
		}
	}
	for _, n := range p.NeedsInput {
		if strings.Contains(n, "import list") {
			t.Fatalf("an import list secret is asked for: %v", p.NeedsInput)
		}
	}
	// A Sonarr Trakt list stays report-only.
	so := &Snapshot{Kind: Sonarr, ImportLists: []Provider{{Name: "Shows", Kind: "TraktListImport", Settings: map[string]any{"username": "sean", "listname": "tv"}}}}
	if p := BuildPlan([]Input{{1, so}}, Choices{}); len(p.Group(GroupLists).Items) != 0 {
		t.Fatalf("a show list was planned: %+v", p.Group(GroupLists).Items)
	}
}

func TestSyncHours(t *testing.T) {
	for in, want := range map[any]int{"12:00:00": 12, "06:30:00": 7, "00:30:00": 6, "1.00:00:00": 24, "200.00:00:00": 168, "soon": 12, nil: 12, int64(5): 12} {
		if got := syncHours(in); got != want {
			t.Errorf("%v: %d, want %d", in, got, want)
		}
	}
	if got := listSlug("  My Favs! (2024) "); got != "my-favs-2024" {
		t.Errorf("slug %q", got)
	}
	taken := map[string]bool{"top": true, "top (radarr)": true}
	if got := freeListName("top", func(n string) bool { return taken[n] }); got != "top (radarr 2)" {
		t.Errorf("second clash %q", got)
	}
	if got := freeListName("new", func(n string) bool { return taken[n] }); got != "new" {
		t.Errorf("free name %q", got)
	}
}

// Transmission, Deluge, rTorrent, aria2 and NZBGet set up in the old apps come
// over as Downloaders, like qBittorrent and SABnzbd; other clients stay behind.
func TestPlanEveryDownloaderKind(t *testing.T) {
	s := &Snapshot{Kind: Sonarr, DownloadClients: []Provider{
		{Name: "Trans", Kind: "Transmission", Settings: map[string]any{"host": "transmission", "port": int64(9091), "urlBase": "/transmission/", "username": "me"}, NeedsReentry: []string{"password"}},
		{Name: "Del", Kind: "Deluge", Settings: map[string]any{"host": "deluge", "port": int64(8112)}, NeedsReentry: []string{"password"}},
		{Name: "RT", Kind: "RTorrent", Settings: map[string]any{"host": "rtorrent", "port": int64(8080), "urlBase": "RPC2", "username": "me", "tvCategory": "tv-sonarr"}, NeedsReentry: []string{"password"}},
		{Name: "A2", Kind: "Aria2", Settings: map[string]any{"host": "aria2", "port": int64(6800), "rpcPath": "/rpc", "useSsl": false, "directory": "/data/dl"}, NeedsReentry: []string{"secretToken"}},
		{Name: "NZB", Kind: "Nzbget", Settings: map[string]any{"host": "nzbget", "port": int64(6789), "username": "nzb"}},
		{Name: "Old", Kind: "Vuze", Settings: map[string]any{"host": "vuze", "port": int64(9091)}},
		{Name: "A2 open", Kind: "Aria2", Settings: map[string]any{"host": "aria2-open", "port": int64(6800), "directory": ""}},
	}}
	s.finish()
	p := BuildPlan([]Input{{1, s}}, Choices{})
	got := map[string]*DownloaderPlan{}
	needs := map[string]string{}
	for _, it := range p.Group(GroupDownloaders).Items {
		got[it.Name] = it.Downloader
		needs[it.Name] = it.Needs
	}
	for name, want := range map[string]string{"Trans": "transmission", "Del": "deluge", "RT": "rtorrent", "A2": "aria2", "A2 open": "aria2", "NZB": "nzbget"} {
		if d := got[name]; d == nil || d.Adapter != want {
			t.Fatalf("%s: %+v", name, d)
		}
	}
	if got["Trans"].URL != "http://transmission:9091/transmission" || got["Trans"].Username != "me" || !got["Trans"].NeedsInput || got["NZB"].NeedsInput {
		t.Fatalf("details: %+v %+v", got["Trans"], got["NZB"])
	}
	if rt := got["RT"]; rt.URL != "http://rtorrent:8080/RPC2" || rt.Username != "me" || !rt.NeedsInput {
		t.Fatalf("rtorrent: %+v", rt)
	}
	if a := got["A2"]; a.URL != "http://aria2:6800/jsonrpc" || a.Username != "" || !a.NeedsInput || needs["A2"] != "your secret token" {
		t.Fatalf("aria2: %+v needs %q", a, needs["A2"])
	}
	if n, ok := notMoved(p, "sonarr download_clients[3].directory"); !ok || n.Why != "the Downloader keeps its own folders; HolyArrs reads where each download lands" {
		t.Fatalf("aria2 folder: %+v", n)
	}
	if a := got["A2 open"]; a.URL != "http://aria2-open:6800/jsonrpc" || a.NeedsInput || needs["A2 open"] != "" {
		t.Fatalf("aria2 without a secret: %+v needs %q", a, needs["A2 open"])
	}
	if n, ok := notMoved(p, "sonarr download_clients[6].directory"); ok {
		t.Fatalf("empty aria2 folder noted: %+v", n)
	}
	if n, ok := notMoved(p, "sonarr download_clients[5]"); !ok || n.Why != "HolyArrs talks to qBittorrent, Transmission, Deluge, rTorrent, aria2, SABnzbd and NZBGet only" {
		t.Fatalf("vuze: %+v", n)
	}
}
