package migrate

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strconv"
)

// seerrReader reads Seerr and Overseerr: people (name, email, role,
// permissions and ask limits; no avatar, no sign-in hashes or tokens),
// the default limits, and the open and approved requests, for the move to
// turn into Requests.
type seerrReader struct{ kind Kind }

// Kind returns the app this reader handles.
func (r seerrReader) Kind() Kind { return r.kind }

// Permission bits, named as the manual quotes them.
var seerrPerms = []struct {
	bit  int64
	name string
}{
	{2, "ADMIN"}, {4, "MANAGE_SETTINGS"}, {8, "MANAGE_USERS"}, {16, "MANAGE_REQUESTS"},
	{32, "REQUEST"}, {128, "AUTO_APPROVE"}, {256, "AUTO_APPROVE_MOVIE"}, {512, "AUTO_APPROVE_TV"},
	{1024, "REQUEST_4K"}, {2048, "REQUEST_4K_MOVIE"}, {4096, "REQUEST_4K_TV"}, {8192, "REQUEST_ADVANCED"},
	{32768, "AUTO_APPROVE_4K"}, {65536, "AUTO_APPROVE_4K_MOVIE"}, {131072, "AUTO_APPROVE_4K_TV"},
	{262144, "REQUEST_MOVIE"}, {524288, "REQUEST_TV"}, {1048576, "MANAGE_ISSUES"}, {2097152, "VIEW_ISSUES"},
	{4194304, "CREATE_ISSUES"}, {8388608, "AUTO_REQUEST"}, {16777216, "AUTO_REQUEST_MOVIE"},
	{33554432, "AUTO_REQUEST_TV"}, {67108864, "RECENT_VIEW"}, {134217728, "WATCHLIST_VIEW"},
}

func permNames(p int64) []string {
	var out []string
	for _, x := range seerrPerms {
		if p&x.bit != 0 {
			out = append(out, x.name)
		}
	}
	return out
}

// requestStates are the request status numbers; only pending and approved
// requests are kept (the rest are done or turned down).
var requestStates = map[int64]string{1: "pending", 2: "approved", 3: "declined", 4: "failed", 5: "completed"}

func userKind(t int64) string {
	switch t {
	case 1:
		return "plex"
	case 2:
		return "local"
	case 3:
		return "jellyfin"
	case 4:
		return "emby"
	}
	return ""
}

func (r seerrReader) client(src Source) (*apiClient, error) {
	key := src.Key
	return newAPIClient(src, func(req *http.Request) { req.Header.Set("X-API-Key", key) })
}

var seerrFiles = []string{"db.sqlite3", "settings.json"}

// detect tells Seerr from Overseerr in a backup: only Seerr has the
// blocklist, watchlist and override_rule tables.
func detect(ctx context.Context, t table) (Kind, bool) {
	if !t.exists(ctx, "user") || !t.exists(ctx, "media_request") {
		return "", false
	}
	if t.exists(ctx, "blocklist") || t.exists(ctx, "watchlist") || t.exists(ctx, "override_rule") {
		return Seerr, true
	}
	return Overseerr, true
}

// Probe checks the request app answers and reports its version.
func (r seerrReader) Probe(ctx context.Context, src Source) (Info, error) {
	if src.Via() == ViaBackup {
		b, err := openBackup(src, seerrFiles)
		if err != nil {
			return Info{}, err
		}
		defer b.close()
		if !b.has("db.sqlite3") {
			return Info{}, ErrWrongApp
		}
		db, err := b.db("db.sqlite3")
		if err != nil {
			return Info{}, err
		}
		defer db.Close()
		var notes []string
		k, ok := detect(ctx, table{db: db, lim: b.lim, notes: &notes})
		if !ok || k != r.kind {
			return Info{}, ErrWrongApp
		}
		return Info{Kind: k, Via: ViaBackup}, nil
	}
	c, err := r.client(src)
	if err != nil {
		return Info{}, err
	}
	v, err := r.status(ctx, c)
	if err != nil {
		return Info{}, err
	}
	return Info{Kind: r.kind, Via: ViaAPI, Version: v,
		Notes: []string{"Seerr and Overseerr look alike through the API; the app you picked is kept"}}, nil
}

func (r seerrReader) status(ctx context.Context, c *apiClient) (string, error) {
	var st map[string]any
	if err := c.getJSON(ctx, "/api/v1/status", nil, &st); err != nil {
		if errors.Is(err, ErrBadAnswer) {
			return "", ErrWrongApp
		}
		return "", err
	}
	v := str(st["version"])
	if v == "" {
		return "", ErrWrongApp
	}
	return v, nil
}

// Read fetches the request app's setup into a Snapshot.
func (r seerrReader) Read(ctx context.Context, src Source) (*Snapshot, error) {
	if src.Via() == ViaBackup {
		return r.fromBackup(ctx, src)
	}
	c, err := r.client(src)
	if err != nil {
		return nil, err
	}
	v, err := r.status(ctx, c)
	if err != nil {
		return nil, err
	}
	s := &Snapshot{Version: v}
	users, err := r.pages(ctx, c, "/api/v1/user", nil)
	if err != nil {
		return nil, err
	}
	names := map[int64]string{}
	limits := false
	for _, m := range users {
		u := seerrUser(m)
		limits = limits || m["movieQuotaLimit"] != nil || m["tvQuotaLimit"] != nil
		names[u.SourceID] = u.Name
		s.Users = append(s.Users, u)
	}
	if !limits {
		s.Notes = append(s.Notes, "per-person ask limits are read from a backup only; through the API each person keeps the default")
	}
	reqs, err := r.pages(ctx, c, "/api/v1/request", url.Values{"filter": {"all"}, "sort": {"added"}})
	if err != nil {
		return nil, err
	}
	var main map[string]any
	if err := c.getJSON(ctx, "/api/v1/settings/main", nil, &main); err == nil {
		s.Settings = seerrDefaults(main)
	} else if !errors.Is(err, ErrBadAnswer) {
		return nil, err
	}
	left := 0
	for _, m := range reqs {
		media := obj(m["media"])
		rq, keep := seerrRequest(num(m["id"]), num(m["status"]), str(first(media, "mediaType")), media,
			num(obj(m["requestedBy"])["id"]), num(obj(m["modifiedBy"])["id"]), boolean(m["is4k"]),
			str(m["createdAt"]), str(m["updatedAt"]))
		if !keep {
			left++
			continue
		}
		for _, se := range list(m["seasons"]) {
			rq.Seasons = append(rq.Seasons, int(num(obj(se)["seasonNumber"])))
		}
		rq.ByName = names[rq.By]
		s.Requests = append(s.Requests, rq)
	}
	if left > 0 {
		s.Notes = append(s.Notes, strconv.Itoa(left)+" finished, failed or turned-down requests were left out")
	}
	return s, nil
}

// pages reads a paged list (take and skip) up to MaxRows rows.
func (r seerrReader) pages(ctx context.Context, c *apiClient, path string, q url.Values) ([]map[string]any, error) {
	const take = 100
	var out []map[string]any
	start := c.got
	for skip := 0; ; skip += take {
		if c.got-start > pagedMax(c.lim) {
			return nil, ErrTooBig
		}
		qq := url.Values{"take": {strconv.Itoa(take)}, "skip": {strconv.Itoa(skip)}}
		for k, v := range q {
			qq[k] = v
		}
		var page struct {
			Results  []map[string]any `json:"results"`
			PageInfo map[string]any   `json:"pageInfo"`
		}
		if err := c.getJSON(ctx, path, qq, &page); err != nil {
			return nil, err
		}
		out = append(out, page.Results...)
		if len(out) > c.lim.MaxRows {
			return nil, ErrTooBig
		}
		if len(page.Results) < take {
			return out, nil
		}
		if pages := num(page.PageInfo["pages"]); pages > 0 && int64(skip/take+1) >= pages {
			return out, nil
		}
	}
}

// seerrUser keeps a person's name, email, role, permissions and limits.
func seerrUser(m map[string]any) User {
	u := User{SourceID: num(m["id"]), Email: str(m["email"]), Kind: userKind(num(m["userType"])),
		MovieLimit: int(num(m["movieQuotaLimit"])), MovieDays: int(num(m["movieQuotaDays"])),
		ShowLimit: int(num(m["tvQuotaLimit"])), ShowDays: int(num(m["tvQuotaDays"])), Created: str(m["createdAt"])}
	_, u.MovieLimitSet = m["movieQuotaLimit"]
	u.MovieLimitSet = u.MovieLimitSet && m["movieQuotaLimit"] != nil
	_, u.ShowLimitSet = m["tvQuotaLimit"]
	u.ShowLimitSet = u.ShowLimitSet && m["tvQuotaLimit"] != nil
	switch u.Kind {
	case "plex":
		u.ExternalID = str(first(m, "plexId", "plexUserId"))
	case "jellyfin", "emby":
		u.ExternalID = str(first(m, "jellyfinUserId", "embyUserId"))
	}
	for _, k := range []string{"displayName", "username", "plexUsername", "jellyfinUsername", "email"} {
		if u.Name = str(m[k]); u.Name != "" {
			break
		}
	}
	p := num(m["permissions"])
	u.Admin = p&2 != 0
	u.Owner = u.SourceID == 1 // Seerr and Overseerr make their first person the owner
	u.Permissions = permNames(p)
	return u
}

// seerrRequest builds one request; keep is false for done or turned-down ones.
func seerrRequest(id, status int64, mediaType string, media map[string]any, by, decided int64, fourK bool, created, updated string) (Request, bool) {
	state := requestStates[status]
	if state != "pending" && state != "approved" {
		return Request{}, false
	}
	rq := Request{SourceID: id, State: state, By: by, FourK: fourK, Created: created, Updated: updated, IDs: map[string]string{}}
	if decided != 0 {
		rq.DecidedBy = decided
	}
	switch mediaType {
	case "tv":
		rq.Kind = "show"
	case "movie":
		rq.Kind = "movie"
	default:
		return Request{}, false // a request of no known kind is not brought over
	}
	for k, id := range map[string]string{"tmdbId": "tmdb", "tvdbId": "tvdb", "imdbId": "imdb"} {
		if v := str(first(media, k)); v != "" && v != "0" {
			rq.IDs[id] = v
		}
	}
	return rq, true
}

// seerrDefaults keeps the default limits and permissions of the main settings.
func seerrDefaults(main map[string]any) map[string]any {
	out := map[string]any{}
	if p, ok := main["defaultPermissions"]; ok {
		var names []any
		for _, n := range permNames(num(p)) {
			names = append(names, n)
		}
		out["default_permissions"] = names
	}
	q := obj(main["defaultQuotas"])
	for kind, key := range map[string]string{"movie": "movie", "tv": "show"} {
		k := obj(q[kind])
		if k == nil {
			continue
		}
		if v, ok := k["quotaLimit"]; ok && v != nil {
			out["default_"+key+"_limit"] = num(v)
		}
		if v, ok := k["quotaDays"]; ok && v != nil {
			out["default_"+key+"_days"] = num(v)
		}
	}
	for _, k := range []string{"applicationTitle", "localLogin", "newPlexLogin"} {
		if v, ok := main[k]; ok {
			out[k] = v
		}
	}
	set, _ := plainSettings(out)
	return set
}

func (r seerrReader) fromBackup(ctx context.Context, src Source) (*Snapshot, error) {
	b, err := openBackup(src, seerrFiles)
	if err != nil {
		return nil, err
	}
	defer b.close()
	if !b.has("db.sqlite3") {
		return nil, ErrNotBackup
	}
	db, err := b.db("db.sqlite3")
	if err != nil {
		return nil, err
	}
	defer db.Close()
	s := &Snapshot{}
	t := table{db: db, lim: b.lim, notes: &s.Notes}
	if k, ok := detect(ctx, t); !ok || k != r.kind {
		return nil, ErrWrongApp
	}
	users, err := t.rows(ctx, "user", "id", "email", "username", "plexUsername", "jellyfinUsername", "displayName", "plexId", "jellyfinUserId",
		"userType", "permissions", "movieQuotaLimit", "movieQuotaDays", "tvQuotaLimit", "tvQuotaDays", "createdAt")
	if err != nil {
		return nil, err
	}
	names := map[int64]string{}
	for _, m := range users {
		u := seerrUser(m)
		names[u.SourceID] = u.Name
		s.Users = append(s.Users, u)
	}
	media, err := t.rows(ctx, "media", "id", "mediaType", "tmdbId", "tvdbId", "imdbId")
	if err != nil {
		return nil, err
	}
	mediaByID := map[int64]map[string]any{}
	for _, m := range media {
		mediaByID[num(m["id"])] = m
	}
	seasons, err := t.rows(ctx, "season_request", "seasonNumber", "requestId")
	if err != nil {
		return nil, err
	}
	byReq := map[int64][]int{}
	for _, m := range seasons {
		byReq[num(m["requestId"])] = append(byReq[num(m["requestId"])], int(num(m["seasonNumber"])))
	}
	reqs, err := t.rows(ctx, "media_request", "id", "status", "type", "is4k", "mediaId", "requestedById", "modifiedById", "createdAt", "updatedAt")
	if err != nil {
		return nil, err
	}
	left := 0
	for _, m := range reqs {
		md := mediaByID[num(m["mediaId"])]
		mt := str(m["type"])
		if mt == "" {
			mt = str(md["mediaType"])
		}
		rq, keep := seerrRequest(num(m["id"]), num(m["status"]), mt, md, num(m["requestedById"]), num(m["modifiedById"]),
			boolean(m["is4k"]), str(m["createdAt"]), str(m["updatedAt"]))
		if !keep {
			left++
			continue
		}
		rq.Seasons = byReq[rq.SourceID]
		sort.Ints(rq.Seasons)
		rq.ByName = names[rq.By]
		s.Requests = append(s.Requests, rq)
	}
	if left > 0 {
		s.Notes = append(s.Notes, strconv.Itoa(left)+" finished, failed or turned-down requests were left out")
	}
	if js, ok, err := b.read("settings.json"); err == nil && ok {
		var set map[string]any
		if decodeJSON(js, b.lim.MaxDepth, &set) == nil {
			s.Settings = seerrDefaults(obj(set["main"]))
		} else {
			s.Notes = append(s.Notes, "settings.json could not be read; default limits were not read")
		}
	}
	return s, nil
}
