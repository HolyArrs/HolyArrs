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

// Kind returns the app this reader handles.
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
		{name: "remotepaths", api: []string{"/remotepathmapping"}, table: "RemotePathMappings"},
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

// Probe checks the app answers and reports its version.
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

// Read fetches the app's setup into a Snapshot.
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
	start := c.got
	for page := 1; page <= c.lim.MaxRows; page++ {
		if c.got-start > pagedMax(c.lim) {
			return nil, ErrTooBig
		}
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
