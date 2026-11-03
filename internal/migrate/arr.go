package migrate

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// arrReader reads Radarr, Sonarr and Lidarr. Both ways (API and backup)
// first gather rows with lower-case keys, then one builder turns them into
// a Snapshot, so the two ways cannot drift apart.
type arrReader struct{ kind Kind }

func (r arrReader) Kind() Kind { return r.kind }

// arrRows is what either way gathers: lower-case keys per row.
type arrRows struct {
	version string
	schema  int
	sets    map[string][]map[string]any
	notes   []string
}

// arrSets are the row sets: the API path, the backup table, and for the
// backup the columns read ("*" for all).
type arrSet struct {
	name   string
	api    []string // tried in order; a 404 tries the next
	table  string
	only   []Kind // nil: every arr app
	needed bool   // a failure fails the read (else a note)
}

func (r arrReader) apiBase() string {
	if r.kind == Lidarr || r.kind == Prowlarr {
		return "/api/v1"
	}
	return "/api/v3"
}

// dbNames are the database's names in a backup (an older Prowlarr
// backup may name it nzbdrone.db).
func (r arrReader) dbNames() []string {
	if r.kind == Prowlarr {
		return []string{"prowlarr.db", "nzbdrone.db"}
	}
	return []string{string(r.kind) + ".db"}
}

// backupFiles are the files copied out of a backup: the database only;
// config.xml holds the app's API key and is never copied.
func (r arrReader) backupFiles() []string { return r.dbNames() }

// openDB opens the backup's database, whichever name it has.
func (r arrReader) openDB(src Source) (*backup, *sql.DB, error) {
	b, err := openBackup(src, r.backupFiles())
	if err != nil {
		return nil, nil, err
	}
	for _, n := range r.dbNames() {
		if b.has(n) {
			db, err := b.db(n)
			if err != nil {
				b.close()
				return nil, nil, err
			}
			return b, db, nil
		}
	}
	b.close()
	return nil, nil, ErrNotBackup
}

func (r arrReader) titleSet() arrSet {
	switch r.kind {
	case Radarr:
		return arrSet{name: "titles", api: []string{"/movie"}, table: "Movies", needed: true}
	case Sonarr:
		return arrSet{name: "titles", api: []string{"/series"}, table: "Series", needed: true}
	case Prowlarr:
		return arrSet{name: "indexers", api: []string{"/indexer"}, table: "Indexers", needed: true}
	}
	return arrSet{name: "titles", api: []string{"/artist"}, table: "Artists", needed: true}
}

func (r arrReader) sets() []arrSet {
	if r.kind == Prowlarr {
		return []arrSet{
			r.titleSet(),
			{name: "tags", api: []string{"/tag"}, table: "Tags"},
			{name: "applinks", api: []string{"/applications"}, table: "Applications"},
			{name: "appprofiles", api: []string{"/appprofile"}, table: "AppSyncProfiles"},
			{name: "clients", api: []string{"/downloadclient"}, table: "DownloadClients"},
			{name: "notifications", api: []string{"/notification"}, table: "Notifications"},
			{name: "proxies", api: []string{"/indexerproxy"}, table: "IndexerProxies"},
		}
	}
	// Exclusions: Radarr names its route and table its own way.
	excl := arrSet{name: "exclusions", api: []string{"/importlistexclusion"}, table: "ImportListExclusions"}
	if r.kind == Radarr {
		excl = arrSet{name: "exclusions", api: []string{"/exclusions"}, table: "ImportExclusions"}
	}
	return []arrSet{
		r.titleSet(),
		{name: "tags", api: []string{"/tag"}, table: "Tags"},
		{name: "folders", api: []string{"/rootfolder"}, table: "RootFolders"},
		{name: "profiles", api: []string{"/qualityprofile"}, table: "QualityProfiles"},
		{name: "formats", api: []string{"/customformat"}, table: "CustomFormats"},
		{name: "releasewords", api: []string{"/releaseprofile"}, table: "ReleaseProfiles"},
		{name: "delays", api: []string{"/delayprofile"}, table: "DelayProfiles"},
		// The availability delay; a backup keeps it in the Config table (fromBackup).
		{name: "indexerconfig", api: []string{"/config/indexer"}, only: []Kind{Radarr}},
		excl,
		{name: "naming", api: []string{"/config/naming", "/settings/naming"}, table: "NamingConfig"},
		{name: "indexers", api: []string{"/indexer"}, table: "Indexers"},
		{name: "clients", api: []string{"/downloadclient"}, table: "DownloadClients"},
		{name: "lists", api: []string{"/importlist"}, table: "ImportLists"},
		{name: "notifications", api: []string{"/notification", "/connection"}, table: "Notifications"},
		{name: "metadata", api: []string{"/metadata"}, table: "Metadata"},
		{name: "metaprofiles", api: []string{"/metadataprofile"}, table: "MetadataProfiles", only: []Kind{Lidarr}},
		{name: "albums", api: []string{"/album"}, table: "Albums", only: []Kind{Lidarr}},
		{name: "albumreleases", table: "AlbumReleases", only: []Kind{Lidarr}},
		{name: "langprofiles", api: []string{"/languageprofile"}, table: "LanguageProfiles", only: []Kind{Sonarr}},
	}
}

func (s arrSet) for_(k Kind) bool {
	if s.only == nil {
		return true
	}
	for _, x := range s.only {
		if x == k {
			return true
		}
	}
	return false
}

func (r arrReader) client(src Source) (*apiClient, error) {
	key := src.Key
	return newAPIClient(src, func(req *http.Request) { req.Header.Set("X-Api-Key", key) })
}

func (r arrReader) Probe(ctx context.Context, src Source) (Info, error) {
	if src.Via() == ViaBackup {
		b, db, err := r.openDB(src)
		if err != nil {
			return Info{}, err
		}
		defer b.close()
		defer db.Close()
		var notes []string
		t := table{db: db, lim: b.lim, notes: &notes}
		schema := t.maxVersion(ctx, "VersionInfo", "Version")
		if schema == 0 || !t.exists(ctx, r.titleSet().table) {
			return Info{}, ErrWrongApp
		}
		return Info{Kind: r.kind, Via: ViaBackup, Schema: schema, Notes: notes}, nil
	}
	c, err := r.client(src)
	if err != nil {
		return Info{}, err
	}
	st, err := r.status(ctx, c)
	if err != nil {
		return Info{}, err
	}
	return Info{Kind: r.kind, Via: ViaAPI, Version: str(st["version"]), Schema: int(num(st["migrationversion"]))}, nil
}

// status reads /system/status and checks the app's name.
func (r arrReader) status(ctx context.Context, c *apiClient) (map[string]any, error) {
	var st map[string]any
	if err := c.getJSON(ctx, r.apiBase()+"/system/status", nil, &st); err != nil {
		if errors.Is(err, ErrBadAnswer) {
			return nil, ErrWrongApp
		}
		return nil, err
	}
	st = lower(st)
	if !strings.EqualFold(str(st["appname"]), string(r.kind)) {
		return nil, ErrWrongApp
	}
	return st, nil
}

func (r arrReader) Read(ctx context.Context, src Source) (*Snapshot, error) {
	var rows *arrRows
	var err error
	if src.Via() == ViaBackup {
		rows, err = r.fromBackup(ctx, src)
	} else {
		rows, err = r.fromAPI(ctx, src)
	}
	if err != nil {
		return nil, err
	}
	return r.build(rows), nil
}

func (r arrReader) fromAPI(ctx context.Context, src Source) (*arrRows, error) {
	c, err := r.client(src)
	if err != nil {
		return nil, err
	}
	st, err := r.status(ctx, c)
	if err != nil {
		return nil, err
	}
	out := &arrRows{version: str(st["version"]), schema: int(num(st["migrationversion"])), sets: map[string][]map[string]any{}}
	for _, s := range r.sets() {
		if !s.for_(r.kind) {
			continue
		}
		var got any
		var lastErr error
		var used string
		for _, p := range s.api {
			used = p
			lastErr = c.getJSON(ctx, r.apiBase()+p, nil, &got)
			if lastErr == nil || !errors.Is(lastErr, ErrBadAnswer) {
				break
			}
		}
		if lastErr != nil {
			if s.needed || !errors.Is(lastErr, ErrBadAnswer) {
				return nil, lastErr
			}
			out.notes = append(out.notes, "the app has no "+s.name+" list; skipped")
			continue
		}
		var rows []map[string]any
		switch x := got.(type) {
		case []any:
			if len(x) > c.lim.MaxRows {
				return nil, ErrTooBig
			}
			for _, e := range x {
				if m, ok := e.(map[string]any); ok {
					rows = append(rows, lower(m))
				}
			}
		case map[string]any:
			if _, paged := lower(x)["records"]; paged {
				if rows, err = r.pages(ctx, c, used); err != nil {
					// A later page that fails is skipped like a missing route.
					if s.needed || !errors.Is(err, ErrBadAnswer) {
						return nil, err
					}
					out.notes = append(out.notes, "the app has no "+s.name+" list; skipped")
					continue
				}
				break
			}
			rows = append(rows, lower(x))
		}
		out.sets[s.name] = rows
	}
	return out, nil
}

// pages reads a list the app answers with its paging envelope (records
// and totalRecords; Sonarr v5's exclusions): from the first page, 1000 at
// a time, until every record is read or a page comes back empty.
func (r arrReader) pages(ctx context.Context, c *apiClient, path string) ([]map[string]any, error) {
	var rows []map[string]any
	for page := 1; page <= c.lim.MaxRows; page++ {
		var env map[string]any
		q := url.Values{"page": {strconv.Itoa(page)}, "pageSize": {"1000"}}
		if err := c.getJSON(ctx, r.apiBase()+path, q, &env); err != nil {
			return nil, err
		}
		env = lower(env)
		recs := list(env["records"])
		for _, e := range recs {
			if m, ok := e.(map[string]any); ok {
				rows = append(rows, lower(m))
			}
		}
		if len(rows) > c.lim.MaxRows {
			return nil, ErrTooBig
		}
		if len(recs) == 0 || int64(len(rows)) >= num(env["totalrecords"]) {
			break
		}
	}
	return rows, nil
}

func (r arrReader) fromBackup(ctx context.Context, src Source) (*arrRows, error) {
	b, db, err := r.openDB(src)
	if err != nil {
		return nil, err
	}
	defer b.close()
	defer db.Close()
	out := &arrRows{sets: map[string][]map[string]any{}}
	t := table{db: db, lim: b.lim, notes: &out.notes}
	out.schema = t.maxVersion(ctx, "VersionInfo", "Version")
	if out.schema == 0 || !t.exists(ctx, r.titleSet().table) {
		return nil, ErrWrongApp
	}
	for _, s := range r.sets() {
		if !s.for_(r.kind) || s.table == "" {
			continue
		}
		rows, err := t.all(ctx, s.table)
		if err != nil {
			return nil, err
		}
		out.sets[s.name] = rows
	}
	// Quality names live in QualityDefinitions; profiles hold ids.
	defs, err := t.all(ctx, "QualityDefinitions")
	if err != nil {
		return nil, err
	}
	out.sets["qualitydefs"] = defs
	if err := r.joinTitles(ctx, t, out); err != nil {
		return nil, err
	}
	// Radarr's availability delay is one key of the Config table (keys
	// lower-cased); a backup without it leaves this server's default.
	if r.kind == Radarr && t.exists(ctx, "Config") {
		rows, err := t.all(ctx, "Config")
		if err != nil {
			return nil, err
		}
		for _, m := range rows {
			if strings.EqualFold(str(m["key"]), "availabilitydelay") {
				out.sets["indexerconfig"] = []map[string]any{{"availabilitydelay": m["value"]}}
			}
		}
	}
	return out, nil
}

// joinTitles fills title, year and ids from the metadata tables (Radarr's
// MovieMetadata, Lidarr's ArtistMetadata). An older Radarr keeps them on
// the Movies row itself, which is used as it is.
func (r arrReader) joinTitles(ctx context.Context, t table, out *arrRows) error {
	meta := map[Kind]string{Radarr: "MovieMetadata", Lidarr: "ArtistMetadata"}[r.kind]
	link := map[Kind]string{Radarr: "moviemetadataid", Lidarr: "artistmetadataid"}[r.kind]
	if meta == "" || !t.exists(ctx, meta) {
		return nil
	}
	rows, err := t.all(ctx, meta)
	if err != nil {
		return err
	}
	byID := map[int64]map[string]any{}
	for _, m := range rows {
		byID[num(m["id"])] = m
	}
	for _, tr := range out.sets["titles"] {
		md := byID[num(tr[link])]
		if md == nil {
			continue
		}
		for k, v := range md {
			if k == "id" {
				continue
			}
			if _, ok := tr[k]; !ok || tr[k] == nil {
				tr[k] = v
			}
		}
		if r.kind == Lidarr {
			tr["artistname"] = md["name"]
		}
	}
	return nil
}

// all reads every column of a table (lower-case keys).
func (t table) all(ctx context.Context, name string) ([]map[string]any, error) {
	cols, err := t.columns(ctx, name)
	if err != nil {
		return nil, err
	}
	if len(cols) == 0 {
		*t.notes = append(*t.notes, "the backup has no "+name+" table; skipped")
		return nil, nil
	}
	var names []string
	for _, real := range cols {
		names = append(names, real)
	}
	sort.Strings(names)
	rows, err := t.rows(ctx, name, names...)
	if err != nil {
		return nil, err
	}
	for i, m := range rows {
		rows[i] = lower(m)
	}
	return rows, nil
}

// lower copies a map with lower-case keys (top level only).
func lower(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[strings.ToLower(k)] = v
	}
	return out
}

// first returns the first key present.
func first(m map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			return v
		}
	}
	return nil
}

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
