package migrate

import (
	"strconv"
	"strings"
)

func (r arrReader) build(in *arrRows) *Snapshot {
	s := &Snapshot{Version: in.version, Schema: in.schema, Notes: in.notes}
	tagName := map[int64]string{}
	for _, m := range in.sets["tags"] {
		t := Tag{ID: num(m["id"]), Label: str(m["label"])}
		tagName[t.ID] = t.Label
		s.Tags = append(s.Tags, t)
	}
	labels := func(v any) []string {
		var out []string
		for _, id := range ids(v) {
			if l, ok := tagName[id]; ok {
				out = append(out, l)
			} else {
				out = append(out, "tag "+strconv.FormatInt(id, 10))
			}
		}
		return out
	}
	for _, m := range in.sets["folders"] {
		f := Folder{Path: str(m["path"])}
		extra := map[string]any{}
		for k, v := range m {
			switch k {
			case "id", "path", "accessible", "freespace", "totalspace", "unmappedfolders":
				continue
			}
			extra[k] = v
		}
		f.Extra, _ = plainSettings(extra)
		s.Folders = append(s.Folders, f)
	}
	quality := map[int64]string{}
	for _, m := range in.sets["qualitydefs"] {
		quality[num(m["quality"])] = str(m["title"])
	}
	formatName := map[int64]string{}
	for _, m := range in.sets["formats"] {
		n := Named{ID: num(m["id"]), Name: str(m["name"])}
		formatName[n.ID] = n.Name
		n.Settings = map[string]any{"include_when_renaming": boolean(m["includecustomformatwhenrenaming"])}
		for _, sp := range list(m["specifications"]) {
			n.Items = append(n.Items, specItem(sp))
		}
		s.CustomFormats = append(s.CustomFormats, n)
	}
	profileName := map[int64]string{}
	for _, m := range in.sets["profiles"] {
		p := profile(m, quality, formatName)
		profileName[p.ID] = p.Name
		s.QualityProfiles = append(s.QualityProfiles, p)
	}
	for _, m := range in.sets["delays"] {
		s.DelayProfiles = append(s.DelayProfiles, DelayProfile{
			ID: num(m["id"]), EnableUsenet: boolean(m["enableusenet"]), EnableTorrent: boolean(m["enabletorrent"]),
			PreferredProtocol: protocolName(m["preferredprotocol"]), UsenetDelay: int(num(m["usenetdelay"])),
			TorrentDelay: int(num(m["torrentdelay"])), Order: int(num(m["order"])), Tags: labels(m["tags"]),
			BypassIfHighestQuality: boolean(m["bypassifhighestquality"]), BypassIfAboveScore: boolean(m["bypassifabovecustomformatscore"]),
		})
	}
	if rows := in.sets["naming"]; len(rows) > 0 {
		n := map[string]any{}
		for k, v := range rows[0] {
			if k != "id" {
				n[k] = v
			}
		}
		s.Naming, _ = plainSettings(n)
	}
	named := func(rows []map[string]any) []Named {
		var out []Named
		for _, m := range rows {
			rest := map[string]any{}
			for k, v := range m {
				if k != "id" && k != "name" {
					rest[k] = v
				}
			}
			set, _ := plainSettings(rest)
			out = append(out, Named{ID: num(m["id"]), Name: str(m["name"]), Settings: set})
		}
		return out
	}
	s.MetadataProfiles = named(in.sets["metaprofiles"])
	for i, m := range in.sets["metaprofiles"] {
		if i >= len(s.MetadataProfiles) {
			break
		}
		if s.MetadataProfiles[i].Settings == nil {
			s.MetadataProfiles[i].Settings = map[string]any{}
		}
		for _, key := range []string{"primaryalbumtypes", "secondaryalbumtypes", "releasestatuses"} {
			if v, ok := m[key]; ok {
				s.MetadataProfiles[i].Settings[key] = metadataSelections(v)
			}
		}
	}
	s.LanguageProfiles = named(in.sets["langprofiles"])
	prov := func(rows []map[string]any) []Provider {
		var out []Provider
		for _, m := range rows {
			out = append(out, provider(m, labels))
		}
		return out
	}
	s.Indexers = linkSafe(prov(in.sets["indexers"]), false)
	s.DownloadClients = prov(in.sets["clients"])
	s.ImportLists = prov(in.sets["lists"])
	for _, m := range in.sets["remotepaths"] {
		s.RemotePathMappings = append(s.RemotePathMappings, RemotePathMapping{Host: clip(str(m["host"]), 200),
			RemotePath: clip(str(m["remotepath"]), 1000), LocalPath: clip(str(m["localpath"]), 1000)})
	}
	s.Notifications = linkSafe(prov(in.sets["notifications"]), true)
	s.Metadata = prov(in.sets["metadata"])
	s.AppLinks = prov(in.sets["applinks"])
	s.IndexerProxies = prov(in.sets["proxies"])
	for _, m := range in.sets["releasewords"] {
		s.ReleaseProfiles = append(s.ReleaseProfiles, releaseWords(m, labels))
	}
	if rows := in.sets["indexerconfig"]; len(rows) > 0 && rows[0]["availabilitydelay"] != nil {
		s.Settings = map[string]any{"availability_delay": num(rows[0]["availabilitydelay"])}
	}
	for _, m := range in.sets["appprofiles"] {
		set, _ := plainSettings(m)
		s.AppProfiles = append(s.AppProfiles, set)
	}
	if r.kind == Prowlarr {
		return s
	}
	for _, m := range in.sets["titles"] {
		t := r.title(m, profileName, labels)
		if t.Folder == "" {
			t.Folder = folderOf(t.Path, s.Folders)
		}
		s.Titles = append(s.Titles, t)
	}
	if r.kind == Lidarr {
		byArtist := map[int64]int{}
		for i := range s.Titles {
			byArtist[s.Titles[i].SourceID] = i
			if id := num(s.Titles[i].Extra["artist_metadata_id"]); id != 0 {
				byArtist[id] = i
			}
		}
		releases := map[int64][]map[string]any{}
		for _, rel := range in.sets["albumreleases"] {
			releases[num(rel["albumid"])] = append(releases[num(rel["albumid"])], rel)
		}
		for _, m := range in.sets["albums"] {
			i, ok := byArtist[num(first(m, "artistid", "artistmetadataid"))]
			mbid := str(first(m, "foreignalbumid"))
			if !ok || !isMBID(mbid) {
				continue
			}
			a := MusicAlbum{MBID: strings.ToLower(mbid), Monitored: boolean(m["monitored"])}
			rawReleases := list(m["releases"])
			if len(rawReleases) == 0 {
				for _, rel := range releases[num(m["id"])] {
					rawReleases = append(rawReleases, rel)
				}
			}
			for _, raw := range rawReleases {
				rel := lower(obj(raw))
				if !boolean(rel["monitored"]) {
					continue
				}
				if id := str(first(rel, "foreignreleaseid")); isMBID(id) {
					a.WatchedEdition = strings.ToLower(id)
					break
				}
			}
			s.Titles[i].MusicAlbums = append(s.Titles[i].MusicAlbums, a)
		}
	}
	for _, m := range in.sets["exclusions"] {
		e := Exclusion{IDs: map[string]string{}, Name: str(first(m, "movietitle", "title", "name")), Year: int(num(first(m, "movieyear", "year")))}
		for k, id := range map[string]string{"tmdbid": "tmdb", "tvdbid": "tvdb", "foreignid": "musicbrainz"} {
			if v := str(m[k]); v != "" && v != "0" {
				e.IDs[id] = v
			}
		}
		s.Exclusions = append(s.Exclusions, e)
	}
	return s
}

// metadataSelections keeps only the bounded policy fields used by the move.
// It deliberately does not retain arbitrary fields from upstream objects.
func metadataSelections(v any) []any {
	rows := list(v)
	if len(rows) > 100 {
		rows = rows[:100]
	}
	out := make([]any, 0, len(rows))
	for _, row := range rows {
		m := obj(row)
		if m == nil {
			continue
		}
		name := clip(str(m["name"]), 100)
		if name == "" {
			continue
		}
		out = append(out, map[string]any{"name": name, "allowed": boolean(m["allowed"])})
	}
	return out
}

// title builds one Title from a movie, series or artist row.
func (r arrReader) title(m map[string]any, profileName map[int64]string, labels func(any) []string) Title {
	t := Title{SourceID: num(m["id"]), Path: str(m["path"]), Folder: str(m["rootfolderpath"]),
		Monitored: boolean(m["monitored"]), Added: str(m["added"]), Tags: labels(m["tags"]),
		IDs: map[string]string{}, Extra: map[string]any{}}
	t.ProfileID = num(first(m, "qualityprofileid", "profileid"))
	t.Profile = profileName[t.ProfileID]
	idCols := map[string]string{"tmdbid": "tmdb", "imdbid": "imdb", "tvdbid": "tvdb", "tvmazeid": "tvmaze", "foreignartistid": "musicbrainz"}
	for col, key := range idCols {
		if v := str(m[col]); v != "" && v != "0" {
			t.IDs[key] = v
		}
	}
	switch r.kind {
	case Radarr:
		t.Kind, t.Name, t.Year = "movie", str(m["title"]), int(num(m["year"]))
		t.Extra["minimum_availability"] = str(m["minimumavailability"])
	case Sonarr:
		t.Kind, t.Name, t.Year = "show", str(m["title"]), int(num(m["year"]))
		t.Extra["show_type"] = str(m["seriestype"])
		t.Extra["season_folder"] = boolean(m["seasonfolder"])
		if v := str(m["monitornewitems"]); v != "" {
			t.Extra["new_seasons"] = v
		}
		if v := num(m["languageprofileid"]); v != 0 {
			t.Extra["language_profile_id"] = v
		}
		for _, se := range list(m["seasons"]) {
			sm := lower(obj(se))
			t.Seasons = append(t.Seasons, Season{Number: int(num(sm["seasonnumber"])), Monitored: boolean(sm["monitored"])})
		}
	case Lidarr:
		t.Kind, t.Name = "artist", str(first(m, "artistname", "name"))
		if v := str(m["monitornewitems"]); v != "" {
			t.Extra["new_albums"] = v
		}
		t.Extra["metadata_profile_id"] = num(m["metadataprofileid"])
		if v := num(m["artistmetadataid"]); v != 0 {
			t.Extra["artist_metadata_id"] = v
		}
	}
	t.Extra, _ = plainSettings(t.Extra)
	if len(t.IDs) == 0 {
		t.IDs = nil
	}
	return t
}

// folderOf picks the library folder holding path (longest match).
func folderOf(path string, folders []Folder) string {
	best := ""
	p := strings.ToLower(strings.ReplaceAll(path, `\`, "/"))
	for _, f := range folders {
		fp := strings.TrimRight(strings.ToLower(strings.ReplaceAll(f.Path, `\`, "/")), "/") + "/"
		if strings.HasPrefix(p, fp) && len(f.Path) > len(best) {
			best = f.Path
		}
	}
	return best
}

// profile builds a quality profile from API or backup rows. The API gives
// each quality as an object with its name; a backup gives only its id,
// named through QualityDefinitions.
func profile(m map[string]any, quality, formatName map[int64]string) Profile {
	p := Profile{ID: num(m["id"]), Name: str(m["name"]), UpgradeAllowed: boolean(m["upgradeallowed"]),
		MinScore: int(num(m["minformatscore"])), CutoffScore: int(num(m["cutoffformatscore"]))}
	cutoff := num(m["cutoff"])
	var walk func(items []any)
	walk = func(items []any) {
		for _, it := range items {
			im := lower(obj(it))
			name, id := "", int64(0)
			if q := im["quality"]; q != nil {
				if qm := obj(q); qm != nil {
					qm = lower(qm)
					name, id = str(qm["name"]), num(qm["id"])
				} else {
					id = num(q)
					name = quality[id]
					if name == "" {
						name = "quality " + strconv.FormatInt(id, 10)
					}
				}
			} else {
				name, id = str(im["name"]), num(im["id"])
			}
			if id == cutoff && name != "" && p.Cutoff == "" {
				p.Cutoff = name
			}
			if boolean(im["allowed"]) && name != "" {
				p.Allowed = append(p.Allowed, name)
			}
			if sub := list(im["items"]); len(sub) > 0 && !boolean(im["allowed"]) {
				walk(sub)
			}
		}
	}
	walk(list(m["items"]))
	for _, fi := range list(m["formatitems"]) {
		fm := lower(obj(fi))
		name := str(fm["name"])
		if name == "" {
			name = formatName[num(fm["format"])]
		}
		if name != "" && num(fm["score"]) != 0 {
			if p.Scores == nil {
				p.Scores = map[string]int{}
			}
			p.Scores[name] = int(num(fm["score"]))
		}
	}
	return p
}

// specItem keeps a custom format condition's plain parts.
func specItem(sp any) map[string]any {
	m := lower(obj(sp))
	out := map[string]any{"name": str(m["name"]), "kind": str(m["implementation"]),
		"negate": boolean(m["negate"]), "required": boolean(m["required"])}
	var set map[string]any
	if f := list(m["fields"]); f != nil {
		set = fieldsToMap(f)
	} else {
		set = obj(m["fields"])
	}
	if set != nil {
		plain, _ := providerSettings(set)
		for k, v := range plain {
			out["value."+k] = v
		}
	}
	return out
}

// releaseWords keeps one release word rule: its must-contain and
// must-not-contain words (a JSON list, or an older comma text), its tag
// scope and the indexers it was tied to (none or 0: every indexer).
func releaseWords(m map[string]any, labels func(any) []string) Named {
	set := map[string]any{"enabled": m["enabled"] == nil || boolean(m["enabled"]),
		"required": words(m["required"]), "ignored": words(m["ignored"])}
	if t := labels(m["tags"]); len(t) > 0 {
		set["tags"] = strs(t)
	}
	if t := labels(m["excludedtags"]); len(t) > 0 {
		set["excluded_tags"] = strs(t)
	}
	var ix []any
	for _, id := range ids(m["indexerids"]) {
		if id != 0 {
			ix = append(ix, id)
		}
	}
	if id := num(m["indexerid"]); id != 0 && len(ix) == 0 {
		ix = append(ix, id)
	}
	if len(ix) > 0 {
		set["indexer_ids"] = ix
	}
	set, _ = plainSettings(set)
	return Named{ID: num(m["id"]), Name: str(m["name"]), Settings: set}
}

// words reads a word list: a JSON list, or comma-separated text.
func words(v any) []any {
	var raw []string
	if l := list(v); l != nil {
		for _, e := range l {
			raw = append(raw, str(e))
		}
	} else {
		raw = strings.Split(str(v), ",")
	}
	out := []any{}
	for _, w := range raw {
		if w = clip(w, 100); w != "" && len(out) < 100 {
			out = append(out, w)
		}
	}
	return out
}

func strs(in []string) []any {
	out := make([]any, 0, len(in))
	for _, s := range in {
		out = append(out, s)
	}
	return out
}

// protocolName reads a protocol word or the backup's number.
func protocolName(v any) string {
	switch s := strings.ToLower(str(v)); s {
	case "1":
		return "usenet"
	case "2":
		return "torrent"
	case "0":
		return ""
	default:
		return s
	}
}

// provider builds one link from an API object (fields list) or a backup
// row (Settings JSON column). Plain settings are kept; secrets are named.
func provider(m map[string]any, labels func(any) []string) Provider {
	p := Provider{ID: num(m["id"]), Name: str(m["name"]), Kind: str(m["implementation"]),
		Priority: int(num(m["priority"])), Protocol: protocolName(m["protocol"]), Tags: labels(m["tags"])}
	set := map[string]any{}
	if f := list(m["fields"]); f != nil {
		set = fieldsToMap(f)
	} else if o := obj(m["settings"]); o != nil {
		set = o
	}
	enabled, seen := false, false
	for k, v := range m {
		switch k {
		case "id", "name", "implementation", "implementationname", "configcontract", "fields", "settings",
			"tags", "priority", "protocol", "infolink", "message", "presets", "capabilities", "indexerurls", "legacyurls", "description", "language", "sortname":
			continue
		case "enable", "enabled", "enablerss", "enableautomaticsearch", "enableinteractivesearch", "enableauto", "enableautomaticadd":
			seen = true
			enabled = enabled || boolean(v)
		}
		if strings.HasPrefix(k, "supports") {
			continue
		}
		set["link."+k] = v
	}
	p.Enabled = enabled || !seen
	p.Settings, p.NeedsReentry = providerSettings(set)
	return p
}
