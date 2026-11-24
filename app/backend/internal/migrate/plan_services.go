package migrate

import (
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles"
)

// sourceKinds are the media kinds a Source from an app serves.
func sourceKinds(k Kind) []string {
	switch k {
	case Radarr:
		return []string{"movie"}
	case Sonarr:
		return []string{"tv"}
	case Lidarr:
		return []string{"music"}
	}
	return []string{"movie", "tv", "music"}
}

func setStr(m map[string]any, keys ...string) string {
	for _, k := range keys {
		for mk, v := range m {
			if strings.EqualFold(mk, k) {
				if s := str(v); s != "" {
					return s
				}
			}
		}
	}
	return ""
}

func setNum(m map[string]any, keys ...string) (int64, bool) {
	for _, k := range keys {
		for mk, v := range m {
			if strings.EqualFold(mk, k) && v != nil {
				return num(v), true
			}
		}
	}
	return 0, false
}

func hasField(fields []string, names ...string) bool {
	for _, f := range fields {
		for _, n := range names {
			if strings.EqualFold(f, n) {
				return true
			}
		}
	}
	return false
}

// webURL checks an address: http or https with a host, no user part.
func webURL(raw string) string {
	raw = clip(raw, 500)
	low := strings.ToLower(raw)
	if !strings.HasPrefix(low, "http://") && !strings.HasPrefix(low, "https://") {
		return ""
	}
	rest := raw[strings.Index(raw, "://")+3:]
	host := rest
	if i := strings.IndexAny(rest, "/?#"); i >= 0 {
		host = rest[:i]
	}
	if host == "" || strings.Contains(host, "@") || strings.ContainsAny(raw, " \t") {
		return ""
	}
	return raw
}

func (b *builder) indexers(s *Snapshot) {
	b.cover("indexers", "indexer_proxies")
	for i, px := range s.IndexerProxies {
		b.not(GroupSources, b.from("indexer_proxies", i), px.Name, "HolyArrs sets one proxy address per Source; set it on the Sources that used it")
	}
	for i, ix := range s.Indexers {
		from := b.from("indexers", i)
		kind := strings.ToLower(ix.Kind)
		var format string
		switch kind {
		case "torznab":
			format = "torznab"
		case "newznab":
			format = "newznab"
		case "torrentrssindexer":
			format = "rss"
		}
		if format == "" {
			why := "HolyArrs reads Torznab, Newznab and RSS Sources; add this one through the Sources bridge"
			if s.Kind == Prowlarr {
				why = "a Prowlarr site definition; HolyArrs reaches such sites through the Sources bridge (Settings > Sources)"
			}
			b.not(GroupSources, from, ix.Name, why)
			continue
		}
		u := webURL(setStr(ix.Settings, "baseUrl", "feedUrl", "url"))
		if u == "" {
			b.not(GroupSources, from, ix.Name, "no usable address (http or https)")
			continue
		}
		if metadataHost(u) {
			b.not(GroupSources, from, ix.Name, "a link-local or cloud metadata address; HolyArrs never reaches those")
			continue
		}
		// The old apps call the address plus its feed path ("/api" unless
		// set), as Prowlarr's per-site addresses (http://prowlarr:9696/5/)
		// need. A path the address already ends in is not added twice.
		if format != "rss" {
			p := strings.Trim(clip(setStr(ix.Settings, "apiPath"), 100), "/")
			if p == "" {
				p = "api"
			}
			if base := strings.TrimSuffix(u, "/"); !strings.HasSuffix(base, "/"+p) {
				u = base + "/" + p
			}
		}
		protocol := "torrent"
		if format == "newznab" || ix.Protocol == "usenet" {
			protocol = "usenet"
		}
		sp := &SourcePlan{Format: format, Protocol: protocol, URL: u, Kinds: sourceKinds(s.Kind), On: ix.Enabled}
		if ix.Priority >= 1 && ix.Priority <= 50 {
			sp.Priority = ix.Priority
		} else {
			sp.Priority = 25
		}
		if n, ok := setNum(ix.Settings, "minimumSeeders"); ok && n >= 0 && n < 10000 && protocol == "torrent" {
			v := int(n)
			sp.MinSeeders = &v
		}
		for _, l := range list(anyKey(ix.Settings, "multiLanguages")) {
			if c := subtitles.OldAppLanguageCodeByID(string(s.Kind), int64(num(l))); c != "" && !slices.Contains(sp.MultiLanguages, c) && len(sp.MultiLanguages) < 60 {
				sp.MultiLanguages = append(sp.MultiLanguages, c)
			}
		}
		if cats := list(anyKey(ix.Settings, "categories")); len(cats) > 0 && len(sp.Kinds) == 1 {
			var cs []int
			for _, c := range cats {
				if n := num(c); n > 0 && n < 1000000 && len(cs) < 50 {
					cs = append(cs, int(n))
				}
			}
			if len(cs) > 0 {
				sp.Categories = map[string][]int{sp.Kinds[0]: cs}
			}
		}
		sp.NeedsKey = hasField(ix.NeedsReentry, "apiKey", "passkey", "cookie")
		it := Item{Key: "source:" + strings.ToLower(clip(ix.Name, 80)), Name: clip(ix.Name, 80), From: from, Source: sp,
			Detail: format + " at " + u}
		it.Detail += "; added paused: check it and resume it yourself"
		if sp.NeedsKey {
			it.Needs = "your key"
			it.Detail = format + " at " + u + "; added paused until you type its key and resume it"
		}
		// A backup holds no Prowlarr key: a Source through Prowlarr needs it typed.
		if s.Via == ViaBackup && throughProwlarr(u) {
			sp.NeedsKey = true
			it.Needs = "Prowlarr's API key"
			it.Detail = format + " at " + u + "; goes through Prowlarr, so it needs Prowlarr's API key: added paused until you type it and resume it"
		}
		// Prowlarr's indexer tags only pick apps and proxies: a proxy
		// sharing one of them was used for this indexer.
		for _, px := range s.IndexerProxies {
			if sharesTag(ix.Tags, px.Tags) {
				it.Detail += "; used proxy " + clip(px.Name, 80) + " there; set a proxy on this Source"
			}
		}
		if prev := b.sameSource(it.Key, sp); prev != nil {
			// The same indexer in another app: one Source serves both kinds.
			for _, k := range sp.Kinds {
				if !slices.Contains(prev.Kinds, k) {
					prev.Kinds = append(prev.Kinds, k)
				}
				if cs := sp.Categories[k]; len(cs) > 0 && len(prev.Categories[k]) == 0 {
					if prev.Categories == nil {
						prev.Categories = map[string][]int{}
					}
					prev.Categories[k] = cs
				}
			}
			for _, c := range sp.MultiLanguages {
				if !slices.Contains(prev.MultiLanguages, c) {
					prev.MultiLanguages = append(prev.MultiLanguages, c)
				}
			}
			b.p.Mapped = append(b.p.Mapped, from)
		} else {
			b.addTagged(s.Kind, GroupSources, it, ix.Tags)
		}
		keys := make([]string, 0, len(ix.Settings))
		for k := range ix.Settings {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			switch strings.ToLower(k) {
			case "baseurl", "feedurl", "url", "apipath", "minimumseeders", "categories":
				continue
			}
			if strings.HasPrefix(k, "link.") {
				continue
			}
			b.not(GroupSources, from+"."+k, ix.Name+": "+k, "no Source setting matches it")
		}
	}
}

// sameSource finds the Source already planned under this key when it is
// the same feed (format and address), or nil.
func (b *builder) sameSource(key string, sp *SourcePlan) *SourcePlan {
	if !b.keys[key] {
		return nil
	}
	for _, it := range b.items[GroupSources].Items {
		if it.Key == key && it.Source != nil && it.Source.Format == sp.Format && it.Source.URL == sp.URL && it.Source.Protocol == sp.Protocol {
			return it.Source
		}
	}
	return nil
}

func anyKey(m map[string]any, key string) any {
	if v, ok := m[key]; ok {
		return v
	}
	// Keys that differ only in case: the smallest wins, every time.
	best, found := "", false
	for k := range m {
		if strings.EqualFold(k, key) && (!found || k < best) {
			best, found = k, true
		}
	}
	if !found {
		return nil
	}
	return m[best]
}

func hostURL(host string, port int, tls bool, base string) string {
	host = clip(host, 200)
	if host == "" || strings.ContainsAny(host, "/@?# ") {
		return ""
	}
	scheme := "http"
	if tls {
		scheme = "https"
	}
	u := scheme + "://" + host
	if port > 0 && port < 65536 {
		u += ":" + strconv.Itoa(port)
	}
	if base = strings.Trim(clip(base, 100), "/"); base != "" && !strings.Contains(base, "..") {
		u += "/" + base
	}
	return webURL(u)
}

func (b *builder) downloaders(s *Snapshot) {
	b.cover("download_clients", "downloader")
	for i, d := range s.DownloadClients {
		from := b.from("download_clients", i)
		var adapter string
		switch k := strings.ToLower(d.Kind); k {
		case "qbittorrent", "sabnzbd", "transmission", "deluge", "rtorrent", "aria2", "nzbget":
			adapter = k
		}
		if adapter == "" {
			b.not(GroupDownloaders, from, d.Name, "HolyArrs talks to qBittorrent, Transmission, Deluge, rTorrent, aria2, SABnzbd and NZBGet only")
			continue
		}
		port, _ := setNum(d.Settings, "port")
		if port < 0 || port > 65535 {
			port = 0
		}
		tls := strings.EqualFold(setStr(d.Settings, "useSsl"), "true")
		path := setStr(d.Settings, "urlBase", "rpcPath")
		if adapter == "aria2" {
			path = downloads.Aria2Path(path)
		}
		u := hostURL(setStr(d.Settings, "host"), int(port), tls, path)
		if u == "" || metadataHost(u) {
			b.not(GroupDownloaders, from, d.Name, "no usable address")
			continue
		}
		locked := hasField(d.NeedsReentry, "password", "apiKey", "secretToken")
		dp := &DownloaderPlan{Adapter: adapter, URL: u, Username: clip(setStr(d.Settings, "username"), 100),
			Category: clean64(setStr(d.Settings, "category")), Priority: d.Priority, NeedsInput: locked}
		// Each app keeps its own kind's category on the client (tvCategory,
		// movieCategory, musicCategory); HolyArrs keeps one per kind too.
		switch s.Kind {
		case Sonarr:
			dp.CategoryShows = clean64(setStr(d.Settings, "tvCategory"))
		case Radarr:
			dp.CategoryMovies = clean64(setStr(d.Settings, "movieCategory"))
		case Lidarr:
			dp.CategoryMusic = clean64(setStr(d.Settings, "musicCategory"))
		}
		b.mergeDownloader(dp) // a client another app brought keeps this app's category too
		b.addDownloader(s.Kind, from, d.Name, dp, d.Tags)
		if adapter == "aria2" && setStr(d.Settings, "directory") != "" {
			b.not(GroupDownloaders, from+".directory", "download folders", "the Downloader keeps its own folders; HolyArrs reads where each download lands")
		}
	}
	if d := s.Downloader; d != nil {
		adapter := map[Kind]string{QBittorrent: "qbittorrent", SABnzbd: "sabnzbd"}[s.Kind]
		from := b.from("downloader", -1)
		u := hostURL(d.Host, d.Port, d.TLS, d.URLBase)
		if u == "" || adapter == "" || metadataHost(u) {
			b.not(GroupDownloaders, from, string(s.Kind), "no usable address")
		} else {
			b.addDownloader(s.Kind, from, map[string]string{"qbittorrent": "qBittorrent", "sabnzbd": "SABnzbd"}[adapter],
				&DownloaderPlan{Adapter: adapter, URL: u, Username: clip(d.Username, 100), NeedsInput: d.NeedsPassword || adapter == "sabnzbd"}, nil)
		}
		if d.SavePath != "" || d.CompletePath != "" {
			b.not(GroupDownloaders, from+".save_path", "download folders", "the Downloader keeps its own folders; HolyArrs reads where each download lands")
		}
		for i, c := range d.Categories {
			b.not(GroupDownloaders, b.from("downloader.categories", i), c.Name, "HolyArrs uses one category of its own per Downloader (holyarrs)")
		}
		for i, sv := range d.Servers {
			b.not(GroupDownloaders, b.from("downloader.servers", i), sv.Name, "usenet servers stay set up in SABnzbd")
		}
	}
}

// mergeDownloader adds d's kind categories to the same client already in the
// plan from another app.
func (b *builder) mergeDownloader(d *DownloaderPlan) {
	g := b.items[GroupDownloaders]
	if g == nil {
		return
	}
	for i := range g.Items {
		have := g.Items[i].Downloader
		if have == nil || g.Items[i].Key != "downloader:"+d.URL {
			continue
		}
		for _, f := range [][2]*string{{&have.CategoryShows, &d.CategoryShows}, {&have.CategoryMovies, &d.CategoryMovies},
			{&have.CategoryMusic, &d.CategoryMusic}, {&have.Category, &d.Category}} {
			if *f[0] == "" {
				*f[0] = *f[1]
			}
		}
		return
	}
}

// importedCategories is the main category and the kind ones an import sets
// on a new Downloader: the main one follows shows, then movies, then music,
// and is holyarrs only when the old apps used none.
func importedCategories(d *DownloaderPlan) (main string, all []string) {
	main = "holyarrs"
	for _, c := range []string{d.CategoryShows, d.CategoryMovies, d.CategoryMusic, d.Category} {
		if c != "" {
			main = c
			break
		}
	}
	for _, c := range []string{main, d.CategoryShows, d.CategoryMovies, d.CategoryMusic} {
		if c != "" && !slices.ContainsFunc(all, func(x string) bool { return strings.EqualFold(x, c) }) {
			all = append(all, c)
		}
	}
	return main, all
}

func clean64(s string) string {
	s = clip(s, 64)
	if strings.ContainsAny(s, `/\`) {
		return ""
	}
	return s
}

func (b *builder) addDownloader(app Kind, from, name string, d *DownloaderPlan, tags []string) {
	if d.Priority < 1 || d.Priority > 50 {
		d.Priority = 1
	}
	it := Item{Key: "downloader:" + d.URL, Name: clip(name, 80), From: from, Downloader: d, Detail: d.Adapter + " at " + d.URL}
	it.Detail += "; added paused: check it and resume it yourself"
	if d.NeedsInput {
		it.Needs = "your password"
		switch d.Adapter {
		case "sabnzbd":
			it.Needs = "your key"
		case "aria2":
			it.Needs = "your secret token"
		}
		it.Detail = d.Adapter + " at " + d.URL + "; added paused until you type " + it.Needs + " and resume it"
	}
	b.addTagged(app, GroupDownloaders, it, tags)
}

// namingKeys maps their naming settings (lower case) to ours. The rename,
// colon and character rules go to the app's own part (see namingPart).
var namingKeys = map[string]string{
	"renamemovies": "rename_on_import", "renameepisodes": "rename_on_import", "renametracks": "rename_on_import",
	"colonreplacementformat": "colon", "replaceillegalcharacters": "illegal",
	"standardmovieformat": "file_pattern", "moviefolderformat": "folder_pattern",
	"seriesfolderformat": "show_folder_pattern", "seasonfolderformat": "season_folder_pattern", "specialsfolderformat": "specials_folder",
	"standardepisodeformat": "episode_pattern", "dailyepisodeformat": "daily_pattern", "animeepisodeformat": "anime_pattern",
	"multiepisodestyle":  "multi_episode_style",
	"artistfolderformat": "artist_folder_pattern", "standardtrackformat": "track_pattern", "multidisctrackformat": "multi_disc_track_pattern",
}

// namingPart is the key prefix of each app's rename, colon and character
// rules: each app keeps its own, as HolyArrs does per part.
var namingPart = map[Kind]string{Sonarr: "show_", Lidarr: "music_"}

// patternWords maps their pattern words to ours, longest first.
var patternWords = [][2]string{
	{"S{season:00}E{episode:00}", "{episode}"}, {"s{season:00}e{episode:00}", "{episode}"},
	{"{Movie TitleThe}", "{sort_title}"}, {"{Movie Title}", "{title}"}, {"{Movie CleanTitle}", "{title}"},
	{"{Series TitleThe}", "{sort_title}"}, {"{Series TitleYear}", "{title} ({year})"}, {"{Series Title}", "{title}"}, {"{Series CleanTitle}", "{title}"},
	{"{Release Year}", "{year}"}, {"{Series Year}", "{year}"}, {"{Quality Full}", "{quality}"}, {"{Quality Title}", "{quality}"},
	{"{Edition Tags}", "{edition}"}, {"{Release Group}", "{group}"}, {"{TmdbId}", "{tmdb}"}, {"{TvdbId}", "{tvdb}"}, {"{ImdbId}", "{imdb}"},
	{"{Original Title}", "{release}"}, {"{Episode Title}", "{episode_title}"}, {"{Episode CleanTitle}", "{episode_title}"},
	{"{Air-Date}", "{air_date}"}, {"{Air Date}", "{air_date}"}, {"{absolute:000}", "{absolute}"}, {"{absolute:00}", "{absolute}"},
	{"{season:00}", "{season_padded}"}, {"{season:0}", "{season}"},
	{"{Artist NameThe}", "{artist_sort}"}, {"{Artist Name}", "{artist}"}, {"{Artist CleanName}", "{artist}"},
	{"{Album Title}", "{album}"}, {"{Album CleanTitle}", "{album}"}, {"{Album Type}", "{album_type}"}, {"{Album Disambiguation}", "{edition}"},
	{"{Track Title}", "{track_title}"}, {"{Track CleanTitle}", "{track_title}"}, {"{Track ArtistName}", "{track_artist}"},
	{"{track:00}", "{track}"}, {"{track:0}", "{track}"}, {"{medium:00}", "{disc}"}, {"{medium:0}", "{disc}"},
}

var (
	wrappedWord = regexp.MustCompile(`\{([\[(\-._ ]*)([A-Za-z][^{}\[\]()]*?)([\]) \-._]*)\}`)
	idWord      = regexp.MustCompile(`(?i)\{(tmdb|tvdb|imdb)id\}`)
)

// translatePattern rewrites one of their patterns in our words; ok is
// false when a word has no match in the pattern's part (words, as ourWords
// or musicOurWords).
func translatePattern(p string, words map[string]bool) (string, string, bool) {
	out := clip(p, 200)
	// Their "{(Release Year)}" and "{-Release Group}" carry the text around a
	// word inside its braces (TRaSH's names use them); ours put it outside.
	out = wrappedWord.ReplaceAllString(out, "$1{$2}$3") // a plain {word} stays as it is
	out = idWord.ReplaceAllStringFunc(out, strings.ToLower)
	for _, w := range []string{"tmdb", "tvdb", "imdb"} {
		out = strings.ReplaceAll(out, "{"+w+"id}", "{"+w+"}")
	}
	for _, w := range patternWords {
		out = strings.ReplaceAll(out, w[0], w[1])
	}
	// Every word left must be one of ours.
	for rest := out; ; {
		i := strings.IndexByte(rest, '{')
		if i < 0 {
			break
		}
		j := strings.IndexByte(rest[i:], '}')
		if j < 0 {
			return "", "an unclosed {", false
		}
		w := rest[i+1 : i+j]
		if strings.Contains(w, "{") {
			// A literal brace around a word, as in {tmdb-{tmdb}}: check the inner word.
			rest = rest[i+1:]
			continue
		}
		if !words[w] {
			return "", "uses {" + clip(w, 40) + "}, which has no HolyArrs word", false
		}
		rest = rest[i+j+1:]
	}
	return out, "", true
}

var ourWords = map[string]bool{"title": true, "sort_title": true, "year": true, "quality": true, "resolution": true, "source": true,
	"edition": true, "group": true, "tmdb": true, "tvdb": true, "imdb": true, "release": true, "season": true, "episode": true,
	"episode_title": true, "air_date": true, "absolute": true, "season_padded": true}

// musicOurWords are the words music patterns take: a movie or show word
// there (such as {quality}) is listed as no match, not moved to fail the
// whole Lidarr naming line.
var musicOurWords = map[string]bool{"artist": true, "artist_sort": true, "album": true, "album_type": true, "disc": true, "track": true,
	"track_title": true, "track_artist": true, "year": true, "edition": true}

// splitMusicPattern splits a Lidarr track pattern, which may hold the album
// folder (and a disc folder) before its file name, into those parts.
func splitMusicPattern(p string) (album, disc, file string, ok bool) {
	parts := strings.Split(strings.ReplaceAll(p, `\`, "/"), "/")
	for _, x := range parts {
		if strings.TrimSpace(x) == "" {
			return "", "", "", false
		}
	}
	switch len(parts) {
	case 1:
		return "", "", parts[0], true
	case 2:
		return parts[0], "", parts[1], true
	case 3:
		return parts[0], parts[1], parts[2], true
	}
	return "", "", "", false
}

func (b *builder) naming(s *Snapshot) {
	b.cover("naming")
	b.names(s.Kind)
	if len(s.Naming) == 0 {
		return
	}
	keys := make([]string, 0, len(s.Naming))
	for k := range s.Naming {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := map[string]string{}
	var froms []string
	for _, k := range keys {
		v := s.Naming[k]
		from := b.from("naming", -1) + "." + k
		ours, ok := namingKeys[strings.ToLower(k)]
		if !ok {
			b.not(GroupNaming, from, k, "no HolyArrs naming setting matches it")
			continue
		}
		part := namingPart[s.Kind]
		switch ours {
		case "rename_on_import":
			b.p.Mapped = append(b.p.Mapped, from) // folded into the names item: the owner's choice decides it
			continue
		case "illegal":
			out[part+ours] = map[bool]string{true: "replace", false: "remove"}[boolean(v)]
		case "track_pattern", "multi_disc_track_pattern":
			pats, why := musicPatterns(ours, str(v))
			if why != "" {
				b.not(GroupNaming, from, k, why)
				continue
			}
			for pk, pv := range pats {
				if _, set := out[pk]; !set || pk != "album_folder_pattern" || ours == "track_pattern" {
					out[pk] = pv
				}
			}
		case "colon":
			c := map[string]string{"0": "remove", "delete": "remove", "1": "dash", "dash": "dash", "2": "space_dash", "spacedash": "space_dash",
				"3": "space_dash_space", "spacedashspace": "space_dash_space", "4": "smart", "smart": "smart"}[strings.ToLower(str(v))]
			if c == "" {
				b.not(GroupNaming, from, k, "this colon choice has no HolyArrs match")
				continue
			}
			out[part+ours] = c
		case "multi_episode_style":
			m := map[string]string{"0": "each", "extend": "each", "1": "each_dash", "duplicate": "each_dash", "2": "each", "repeat": "each",
				"3": "each_dash", "scene": "each_dash", "4": "short_range", "range": "short_range", "5": "range", "prefixedrange": "range"}[strings.ToLower(str(v))]
			if m == "" {
				b.not(GroupNaming, from, k, "this style has no HolyArrs match")
				continue
			}
			out[ours] = m
		default:
			words := ourWords
			if ours == "artist_folder_pattern" {
				words = musicOurWords
			}
			pat, why, ok := translatePattern(str(v), words)
			if !ok || pat == "" || strings.ContainsAny(pat, `/\`) {
				if why == "" {
					why = "it names more than one folder"
				}
				b.not(GroupNaming, from, k, why)
				continue
			}
			out[ours] = pat
		}
		froms = append(froms, k)
	}
	if len(out) == 0 {
		return
	}
	detail := fmt.Sprintf("%d settings: %s", len(out), strings.Join(froms, ", "))
	b.add(GroupNaming, Item{Key: fmt.Sprintf("naming:%s", s.Kind), Name: GroupLabels[GroupNaming] + " from " + string(s.Kind), From: b.from("naming", -1),
		Detail: detail, Why: "only settings with a HolyArrs match; nothing on disk is renamed", Naming: out})
}

// names adds the owner's names choice for an app with files as its own
// item, so it applies even when Naming is left out: keep turns rename on
// import off for that part, whatever the old app had.
func (b *builder) names(k Kind) {
	if k != Radarr && k != Sonarr && k != Lidarr {
		return
	}
	detail := "off: files keep the names they have, now and on later imports"
	if b.rename {
		detail = "on: later imports get HolyArrs names; files already there change only through Rename"
	}
	b.add(GroupNaming, Item{Key: "names:" + string(k), Name: "Rename on import for " + string(k), From: b.from("naming", -1),
		Detail: detail, Why: "your names choice; nothing on disk is renamed by the import", Naming: map[string]string{namingPart[k] + "rename_on_import": strconv.FormatBool(b.rename)}})
}

// musicPatterns turns one Lidarr track pattern into ours: the file name,
// plus the album folder (and, for the multi-disc pattern, the disc folder,
// which turns disc folders on) when it names folders. why says what has no
// match.
func musicPatterns(key, p string) (map[string]string, string) {
	album, disc, file, ok := splitMusicPattern(p)
	if !ok || (disc != "" && key != "multi_disc_track_pattern") {
		return nil, "it names more folders than HolyArrs keeps"
	}
	out := map[string]string{}
	for _, x := range [][2]string{{"album_folder_pattern", album}, {"disc_folder_pattern", disc}, {key, file}} {
		if x[1] == "" {
			continue
		}
		pat, why, ok := translatePattern(x[1], musicOurWords)
		if !ok || pat == "" {
			if why == "" {
				why = "it is empty"
			}
			return nil, why
		}
		out[x[0]] = pat
	}
	if disc != "" {
		out["disc_folders"] = "true"
	}
	return out, ""
}

// subtitleSiteKinds maps their subtitle site names to ours.
var subtitleSiteKinds = map[string]string{"podnapisi": "podnapisi", "gestdown": "gestdown", "yifysubtitles": "yifysubtitles",
	"tvsubtitles": "tvsubtitles", "bsplayer": "bsplayer", "napiprojekt": "napiprojekt", "titlovi": "titlovi"}

func (b *builder) subtitles(s *Snapshot) {
	b.cover("subtitles")
	sub := s.Subtitles
	if sub == nil {
		return
	}
	// The default profile's languages become the default wishes.
	def := num(sub.Defaults["movie_default_profile"])
	if def == 0 {
		def = num(sub.Defaults["serie_default_profile"])
	}
	var wants []SubtitleWant
	used := -1
	for i, p := range sub.Profiles {
		if p.ID == def && def != 0 {
			used = i
			for _, l := range p.Languages {
				if code := clip(l.Language, 8); code != "" && len(wants) < 20 {
					hi := "normal"
					if l.HI {
						hi = "hi"
					}
					wants = append(wants, SubtitleWant{Language: code, HearingImpaired: hi, Forced: l.Forced})
				}
			}
		}
	}
	langFrom := b.from("subtitles.languages", -1)
	if used < 0 {
		for _, l := range sub.Languages {
			if code := clip(l, 8); code != "" && len(wants) < 20 {
				wants = append(wants, SubtitleWant{Language: code, HearingImpaired: "normal"})
			}
		}
	} else {
		langFrom = b.from("subtitles.profiles", used)
		if len(sub.Languages) > 0 {
			b.not(GroupSubtitles, b.from("subtitles.languages", -1), strings.Join(sub.Languages, ", "), "the default profile's languages were imported; the other enabled languages are only choices in Bazarr")
		}
	}
	if len(wants) > 0 {
		var names []string
		for _, w := range wants {
			names = append(names, w.Language)
		}
		b.add(GroupSubtitles, Item{Key: "subtitles:languages", Name: "Default subtitle languages", From: langFrom,
			Detail: strings.Join(names, ", "), Why: "new movies and shows wish for these languages", Subtitles: &SubtitlePlan{Languages: wants}})
	}
	for i, p := range sub.Profiles {
		if i == used {
			continue
		}
		b.not(GroupSubtitles, b.from("subtitles.profiles", i), p.Name, "HolyArrs has one default list of languages and a list per title; set this one on the titles that used it")
	}
	for i, pr := range sub.Providers {
		from := b.from("subtitles.providers", i)
		kind := subtitleSiteKinds[strings.ToLower(pr.Kind)]
		if kind == "" {
			why := "HolyArrs has no subtitle site of this kind yet"
			if strings.EqualFold(pr.Kind, "addic7ed") {
				why = "HolyArrs reaches Addic7ed subtitles through Gestdown, with no account; add Gestdown under Settings > Subtitles"
			}
			b.not(GroupSubtitles, from, pr.Name, why)
			continue
		}
		b.add(GroupSubtitles, Item{Key: "subtitle_site:" + kind, Name: clip(pr.Name, 100), From: from, Detail: "subtitle site " + kind,
			Subtitles: &SubtitlePlan{SiteKind: kind}})
	}
	keys := make([]string, 0, len(sub.Defaults))
	for k := range sub.Defaults {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		from := b.from("subtitles.defaults", -1) + "." + k
		switch k {
		case "movie_default_profile", "serie_default_profile":
			if used >= 0 {
				continue
			}
		}
		b.not(GroupSubtitles, from, k, "no HolyArrs subtitle setting matches it")
	}
}

// personKey names a person from an app.
func personKey(app Kind, id int64) string { return fmt.Sprintf("person:%s:%d", app, id) }

func stablePersonKey(app Kind, u User) string {
	kind := strings.ToLower(clip(u.Kind, 20))
	external := clip(u.ExternalID, 100)
	if external == "" || kind == "" || strings.ContainsAny(kind+external, ":/\\") {
		return personKey(app, u.SourceID)
	}
	return fmt.Sprintf("person:%s:%s:%s", app, kind, external)
}

func weekly(limit, days int) *int {
	if limit <= 0 || limit > 1000 {
		return nil
	}
	if days != 7 {
		return nil
	}
	w := limit
	return &w
}

// defaultWeekly differs from a person's quota: an explicit zero in the old
// app means unlimited, while a missing value means there is no setting to move.
func defaultWeekly(settings map[string]any, kind string) *int {
	limit, ok := settings["default_"+kind+"_limit"]
	if !ok {
		return nil
	}
	n := int(num(limit))
	if n == 0 {
		return &n
	}
	return weekly(n, int(num(settings["default_"+kind+"_days"])))
}

func sourceTime(v string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999Z07:00", "2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, v); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

// throughProwlarr says whether addr is a Prowlarr per-site feed
// (http://prowlarr:9696/5/api).
func throughProwlarr(addr string) bool {
	u, err := url.Parse(addr)
	if err != nil {
		return false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 2 || parts[1] != "api" {
		return false
	}
	_, err = strconv.Atoi(parts[0])
	return err == nil
}
