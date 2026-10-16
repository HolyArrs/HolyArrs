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
				Folder: "/movies", Monitored: true, ProfileID: 1, Tags: []string{"kids"}, Extra: map[string]any{"minimumAvailability": "released"}},
			{SourceID: 2, Kind: "movie", Name: "No id", Path: "/movies/No id"},
			{SourceID: 3, Kind: "movie", Name: "Elsewhere", IDs: map[string]string{"tmdb": "604"}, Path: "/other/Elsewhere"},
		},
		Tags:            []Tag{{ID: 1, Label: "kids"}},
		Folders:         []Folder{{Path: "/movies", Extra: map[string]any{"name": "Films"}}},
		QualityProfiles: []Profile{{ID: 1, Name: "HD-1080p", UpgradeAllowed: true, Cutoff: "Bluray-1080p", Allowed: []string{"WEBDL-1080p", "Bluray-1080p"}, Scores: map[string]int{"x265": 10}}, {ID: 2, Name: "Ultra-HD", Cutoff: "Remux-2160p"}},
		CustomFormats:   []Named{{ID: 1, Name: "x265"}},
		DelayProfiles:   []DelayProfile{{ID: 1, EnableTorrent: true, TorrentDelay: 60}},
		Naming:          map[string]any{"renameMovies": true, "colonReplacementFormat": "dash", "standardMovieFormat": "{Movie Title} ({Release Year}) {Quality Full}", "movieFolderFormat": "{Movie Title} ({Release Year}) {Movie Collection}", "includeQuality": true},
		Exclusions:      []Exclusion{{IDs: map[string]string{"tmdb": "999"}, Name: "Bad Movie", Year: 2001}},
		Indexers: []Provider{
			prov("Nyaa", "Torznab", map[string]any{"baseUrl": "http://prowlarr:9696/1/", "minimumSeeders": int64(2), "categories": []any{int64(2000)}, "link.enablerss": true}, "apiKey"),
			prov("Site", "Cardigann", map[string]any{"baseUrl": "https://x.example/"}),
		},
		DownloadClients: []Provider{prov("qBit", "QBittorrent", map[string]any{"host": "qbit", "port": int64(8080), "username": "admin", "movieCategory": "radarr"}, "password"),
			prov("Deluge", "Deluge", map[string]any{"host": "deluge"})},
		ImportLists:   []Provider{prov("Trakt", "TraktListImport", nil, "accessToken")},
		Notifications: []Provider{prov("Discord", "Discord", nil, "webHookUrl"), prov("Mail", "Email", map[string]any{"server": "smtp.example", "port": int64(465), "useEncryption": "1", "from": "a@example.org", "to": []any{"b@example.org"}}, "password"), prov("Push", "Pushover", nil)},
		Metadata:      []Provider{prov("Kodi", "XbmcMetadata", nil)},
		NeedsReentry:  []string{`indexer "Nyaa": apiKey`},
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
		Indexers:    []Provider{prov("NZBgeek", "Newznab", map[string]any{"baseUrl": "https://api.nzbgeek.example", "apiPath": "/api"}, "apiKey")},
		AppLinks:    []Provider{prov("Radarr", "Radarr", map[string]any{"baseUrl": "http://radarr:7878"}, "apiKey")},
		AppProfiles: []map[string]any{{"name": "Standard"}},
	}
	bazarr := &Snapshot{Kind: Bazarr, Subtitles: &SubtitleSetup{Languages: []string{"en", "es", "fr"},
		Profiles:  []SubtitleProfile{{ID: 1, Name: "Main", Languages: []SubtitleLanguage{{Language: "en"}, {Language: "es", HI: true}}}, {ID: 2, Name: "Anime"}},
		Providers: []Provider{{Name: "podnapisi", Kind: "podnapisi"}, {Name: "opensubtitlescom", Kind: "opensubtitlescom", NeedsReentry: []string{"password"}}, {Name: "addic7ed", Kind: "addic7ed"}},
		Defaults:  map[string]any{"movie_default_profile": int64(1), "use_embedded_subs": true}}}
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
	if r.Rules.Preset != "Good everyday" || !strings.Contains(r.Why, "custom format scores") {
		t.Fatalf("rules %+v", r)
	}
	if item(t, p, GroupRules, "pick_rules:1:radarr:2").Rules.Preset != "4K" || item(t, p, GroupRules, "pick_rules:3:lidarr:1").Rules.Preset != "Lossless first" {
		t.Fatal("4K or lossless not matched")
	}
	// Titles: adopted in place, the profile's preset, Keep up to date.
	m := item(t, p, GroupTitles, "title:movie:tmdb:603").Title
	if m.Library != "/data/movies" || m.Folder != "The Matrix (1999)" || !m.KeepUpToDate || m.Preset != "Good everyday" {
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
	if g := item(t, p, GroupSources, "source:nzbgeek").Source; g.Format != "newznab" || g.Protocol != "usenet" || g.URL != "https://api.nzbgeek.example" {
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
	// Wishlist asks: names from the read titles, attributed to the people.
	ask := item(t, p, GroupWishlist, "ask:seerr:7")
	if ask.Name != "The Matrix" || ask.Ask.By != "person:seerr:2" || ask.Ask.Approved {
		t.Fatalf("ask %+v", ask)
	}
	if sh := item(t, p, GroupWishlist, "ask:seerr:8").Ask; !sh.Approved || sh.Ref != "tvdb:73739" || sh.Decider != "person:seerr:1" || sh.Note == "" {
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
