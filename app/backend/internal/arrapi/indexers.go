package arrapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
)

// Prowlarr's app sync. Prowlarr writes one Newznab or Torznab indexer into
// each app for every indexer it has, all pointing at itself. It reads the
// templates, tests, adds, reads back, rewrites when a value it manages
// differs, and deletes. Each one it writes becomes a Source of this side's
// kind.
//
// Such a Source may reach the local network, since Prowlarr usually lives
// there. It arrives paused, as any Source does after its address moves,
// so a person turns it on. Only Sources made here are listed, changed or
// deleted here.

// The extras a Source made here keeps: the side that made it, what
// Prowlarr last wrote without its key, and the name HolyArrs gave it. The
// kept copy makes the next read compare equal, so Prowlarr does not write
// it again and again.
const (
	extraSide = "old_app"
	extraBody = "old_app_indexer"
	extraName = "old_app_name"
)

// maskedKey is how those apps show a key that is set. Sent back, it keeps
// the stored key.
const maskedKey = "********"

// bodyKeep caps what is kept of one indexer, so extras stay small.
const bodyKeep = 64 << 10

// sourceKind returns this side's Source kind.
func (h *handler) sourceKind() string {
	if h.side == "sonarr" {
		return "tv"
	}
	return "movie"
}

type schemaField struct {
	Order    int    `json:"order"`
	Name     string `json:"name"`
	Label    string `json:"label"`
	Value    any    `json:"value"`
	Type     string `json:"type"`
	Advanced bool   `json:"advanced"`
}

// schemaFields are the settings of one template: the ones Prowlarr fills
// and the ones it copies from the template when it makes an entry.
func (h *handler) schemaFields(torrent bool) []schemaField {
	sonarr := h.side == "sonarr"
	fs := []schemaField{
		{Name: "baseUrl", Label: "Address", Value: "", Type: "text"},
		{Name: "apiPath", Label: "Feed path", Value: "/api", Type: "text", Advanced: true},
		{Name: "apiKey", Label: "Key", Value: "", Type: "text"},
		{Name: "categories", Label: "Categories", Value: sources.DefaultCategories[h.sourceKind()], Type: "list"},
	}
	if sonarr {
		fs = append(fs, schemaField{Name: "animeCategories", Label: "Anime categories", Value: []int{}, Type: "list"},
			schemaField{Name: "animeStandardFormatSearch", Label: "Search anime by season and episode too", Value: false, Type: "switch", Advanced: true})
	}
	fs = append(fs, schemaField{Name: "additionalParameters", Label: "Extra address values", Value: "", Type: "text", Advanced: true})
	if !sonarr {
		fs = append(fs, schemaField{Name: "multiLanguages", Label: "Languages of multi-language releases", Value: []int{}, Type: "list", Advanced: true},
			schemaField{Name: "removeYear", Label: "Search without the year", Value: false, Type: "switch", Advanced: true},
			schemaField{Name: "requiredFlags", Label: "Required release flags", Value: []int{}, Type: "list", Advanced: true})
	}
	if torrent {
		fs = append(fs, schemaField{Name: "minimumSeeders", Label: "Least seeders", Value: 1, Type: "number", Advanced: true},
			schemaField{Name: "seedCriteria.seedRatio", Label: "Seed ratio", Value: nil, Type: "number", Advanced: true},
			schemaField{Name: "seedCriteria.seedTime", Label: "Seed time (minutes)", Value: nil, Type: "number", Advanced: true})
		if sonarr {
			fs = append(fs, schemaField{Name: "seedCriteria.seasonPackSeedTime", Label: "Season pack seed time (minutes)", Value: nil, Type: "number", Advanced: true})
		}
		fs = append(fs, schemaField{Name: "rejectBlocklistedTorrentHashesWhileGrabbing", Label: "Refuse torrents turned down before", Value: false, Type: "switch", Advanced: true})
	}
	for i := range fs {
		fs[i].Order = i
	}
	return fs
}

// implOf names a format the way the old apps do: implementation, settings
// contract and protocol.
func implOf(format string) (impl, contract, protocol string) {
	if format == "torznab" {
		return "Torznab", "TorznabSettings", "torrent"
	}
	return "Newznab", "NewznabSettings", "usenet"
}

// template builds the blank entry of one format.
func (h *handler) template(format string) map[string]any {
	impl, contract, proto := implOf(format)
	m := map[string]any{"name": "", "implementation": impl, "implementationName": impl, "configContract": contract, "protocol": proto,
		"priority": 25, "enableRss": true, "enableAutomaticSearch": true, "enableInteractiveSearch": true, "supportsRss": true, "supportsSearch": true,
		"downloadClientId": 0, "tags": []any{}, "presets": []any{}, "fields": h.schemaFields(format == "torznab")}
	if h.side == "sonarr" {
		m["seasonSearchMaximumSingleEpisodeAge"] = 0
	}
	return m
}

// indexerSchema answers the templates. Only Newznab and Torznab are
// offered, since those are the two a Source can be.
func (h *handler) indexerSchema(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, []map[string]any{h.template("newznab"), h.template("torznab")})
}

// push is one indexer as Prowlarr writes it, read into Source values.
type push struct {
	id       int64 // the id in the body, 0 when none
	name     string
	format   string
	protocol string
	url      string // the address with the feed path joined on
	key      string
	keepKey  bool // the key came back masked, so the stored one stays
	cats     []int
	seeders  *int
	priority int
	use      domain.SourceUse
	keep     string // what is kept of it, without the key
}

// fieldRows reads the fields list of an entry. Names match in any case.
func fieldRows(m map[string]any) []map[string]any {
	l, _ := m["fields"].([]any)
	var out []map[string]any
	for _, x := range l {
		if f, ok := x.(map[string]any); ok {
			out = append(out, f)
		}
	}
	return out
}

func fieldName(f map[string]any) string { s, _ := f["name"].(string); return s }

func fieldValue(m map[string]any, name string) (any, bool) {
	for _, f := range fieldRows(m) {
		if strings.EqualFold(fieldName(f), name) {
			return f["value"], true
		}
	}
	return nil, false
}

func valueString(m map[string]any, key string) string {
	for k, v := range m {
		if strings.EqualFold(k, key) {
			s, _ := v.(string)
			return s
		}
	}
	return ""
}

func valueOf(m map[string]any, key string) (any, bool) {
	for k, v := range m {
		if strings.EqualFold(k, key) {
			return v, true
		}
	}
	return nil, false
}

// toInt reads a whole number sent as a number or as text, since clients
// send both.
func toInt(v any) (int, bool) {
	switch x := v.(type) {
	case float64:
		if x != float64(int(x)) {
			return 0, false
		}
		return int(x), true
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(x))
		return n, err == nil
	}
	return 0, false
}

func toInts(v any) ([]int, bool) {
	if v == nil {
		return nil, true
	}
	l, ok := v.([]any)
	if !ok {
		return nil, false
	}
	var out []int
	for _, x := range l {
		n, ok := toInt(x)
		if !ok || n <= 0 {
			return nil, false
		}
		out = append(out, n)
	}
	return out, true
}

func toBool(v any, def bool) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return def
}

// sortedInts returns a sorted copy without repeats, so lists compare
// equal.
func sortedInts(ls ...[]int) []int {
	out := []int{}
	for _, l := range ls {
		out = append(out, l...)
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// joinFeed returns the base address with the feed path after it.
func joinFeed(base, path string) string {
	base = strings.TrimSpace(base)
	path = strings.Trim(strings.TrimSpace(path), "/")
	if path == "" {
		return strings.TrimRight(base, "/")
	}
	return strings.TrimRight(base, "/") + "/" + path
}

// readPush reads an indexer body. It returns false after sending a 400.
func (h *handler) readPush(w http.ResponseWriter, r *http.Request) (push, bool) {
	var p push
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, bodyMax))
	var m map[string]any
	if err == nil {
		err = json.Unmarshal(b, &m)
	}
	if err != nil || m == nil {
		message(w, http.StatusBadRequest, "the body is not valid JSON for this route")
		return p, false
	}
	switch strings.ToLower(valueString(m, "implementation")) {
	case "newznab":
		p.format, p.protocol = "newznab", "usenet"
	case "torznab":
		p.format, p.protocol = "torznab", "torrent"
	default:
		refuse(w, failure{PropertyName: "Implementation", ErrorMessage: "HolyArrs takes Newznab and Torznab indexers only", AttemptedValue: valueString(m, "implementation")})
		return p, false
	}
	p.name = strings.TrimSpace(valueString(m, "name"))
	if p.name == "" {
		refuse(w, failure{PropertyName: "Name", ErrorMessage: "give the indexer a name", AttemptedValue: p.name})
		return p, false
	}
	base, _ := fieldValue(m, "baseUrl")
	path, hasPath := fieldValue(m, "apiPath")
	bs, _ := base.(string)
	ps, _ := path.(string)
	if !hasPath {
		ps = "/api"
	}
	p.url = joinFeed(bs, ps)
	if u, err := url.Parse(p.url); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery {
		refuse(w, failure{PropertyName: "BaseUrl", ErrorMessage: "the address must start with http:// or https:// and name a host, with no query", AttemptedValue: bs})
		return p, false
	}
	key, _ := fieldValue(m, "apiKey")
	p.key, _ = key.(string)
	p.key = strings.TrimSpace(p.key)
	if p.key == maskedKey || p.key == secrets.Masked {
		p.key, p.keepKey = "", true
	}
	var cats []int
	for _, name := range []string{"categories", "animeCategories"} {
		v, _ := fieldValue(m, name)
		l, ok := toInts(v)
		if !ok {
			refuse(w, failure{PropertyName: name, ErrorMessage: "categories are a list of category numbers", AttemptedValue: v})
			return p, false
		}
		if name == "animeCategories" && h.side != "sonarr" {
			continue
		}
		cats = append(cats, l...)
	}
	p.cats = sortedInts(cats)
	if p.format == "torznab" {
		if v, ok := fieldValue(m, "minimumSeeders"); ok && v != nil {
			n, ok := toInt(v)
			if !ok || n < 0 {
				refuse(w, failure{PropertyName: "MinimumSeeders", ErrorMessage: "the least seeders is 0 or more", AttemptedValue: v})
				return p, false
			}
			p.seeders = &n
		}
	}
	p.priority = 25
	if v, ok := valueOf(m, "priority"); ok && v != nil {
		n, ok := toInt(v)
		if !ok || n < 1 || n > 50 {
			refuse(w, failure{PropertyName: "Priority", ErrorMessage: "priority must be 1 to 50", AttemptedValue: v})
			return p, false
		}
		p.priority = n
	}
	use := func(k string) bool { v, _ := valueOf(m, k); return toBool(v, true) }
	p.use = domain.SourceUse{Feed: use("enableRss"), AutoSearch: use("enableAutomaticSearch"), HandSearch: use("enableInteractiveSearch")}
	// Keep the whole entry as sent, without its id and key.
	for k, v := range m {
		if strings.EqualFold(k, "id") {
			n, _ := toInt(v)
			p.id = int64(n)
			delete(m, k)
		}
	}
	for _, f := range fieldRows(m) {
		if strings.EqualFold(fieldName(f), "apiKey") {
			f["value"] = ""
		}
	}
	kept, _ := json.Marshal(m)
	if len(kept) > bodyKeep {
		message(w, http.StatusBadRequest, "the indexer is too big to keep")
		return p, false
	}
	p.keep = string(kept)
	return p, true
}

// mayWriteSources reports whether the caller may write Sources here. Only
// this side's key, made by someone who may change Sources, qualifies. It
// returns false after sending the answer.
func (h *handler) mayWriteSources(w http.ResponseWriter, r *http.Request) bool {
	who, ok := webserver.From(r.Context())
	if !ok || who.Via != "api_key" || !slices.Contains(who.Scopes, webserver.ScopeCompat) {
		message(w, http.StatusUnauthorized, "indexers come in only with this side's key")
		return false
	}
	if !who.Has(webserver.PermSettings) {
		message(w, http.StatusForbidden, "the key acts as the person who made it, who may not change Sources")
		return false
	}
	return true
}

// ownSource reads a Source this side made. Any other gives
// database.ErrNotFound.
func (h *handler) ownSource(ctx context.Context, id int64) (domain.Source, error) {
	var src domain.Source
	if err := database.Get(ctx, h.Store.Read(), "sources", id, &src); err != nil {
		return src, err
	}
	if src.Extra[extraSide] != h.side {
		return src, database.ErrNotFound
	}
	return src, nil
}

// indexerOf answers a Source as an indexer: what Prowlarr last wrote,
// with the values HolyArrs holds put back in. A value changed in HolyArrs
// shows as changed, so Prowlarr writes its own again, as with the old
// apps.
func (h *handler) indexerOf(src domain.Source) map[string]any {
	var m map[string]any
	if b := src.Extra[extraBody]; b != "" {
		_ = json.Unmarshal([]byte(b), &m)
	}
	if m == nil {
		b, _ := json.Marshal(h.template(src.Format))
		_ = json.Unmarshal(b, &m)
	}
	base, _ := fieldValue(m, "baseUrl")
	path, hasPath := fieldValue(m, "apiPath")
	bs, _ := base.(string)
	ps, _ := path.(string)
	if !hasPath {
		ps = "/api"
	}
	sameURL := joinFeed(bs, ps) == src.URL
	var sent [][]int
	for _, name := range []string{"categories", "animeCategories"} {
		if name == "animeCategories" && h.side != "sonarr" {
			continue
		}
		v, _ := fieldValue(m, name)
		l, _ := toInts(v)
		sent = append(sent, l)
	}
	ours := src.Categories[h.sourceKind()]
	sameCats := slices.Equal(sortedInts(sent...), sortedInts(ours))
	for _, f := range fieldRows(m) {
		switch strings.ToLower(fieldName(f)) {
		case "baseurl":
			if !sameURL {
				f["value"] = src.URL
			}
		case "apipath":
			if !sameURL {
				f["value"] = ""
			}
		case "apikey":
			f["value"] = ""
			if src.SecretRef != nil {
				f["value"] = maskedKey
			}
		case "categories":
			if !sameCats {
				f["value"] = sortedInts(ours)
			}
		case "animecategories":
			if !sameCats {
				f["value"] = []int{}
			}
		case "minimumseeders":
			if src.MinSeeders != nil {
				f["value"] = *src.MinSeeders
			}
		}
	}
	for k := range m {
		switch strings.ToLower(k) {
		case "name", "implementation", "implementationname", "configcontract", "protocol", "priority", "enablerss", "enableautomaticsearch",
			"enableinteractivesearch", "supportsrss", "supportssearch":
			delete(m, k)
		}
	}
	name := src.Name
	if sentName := valueStringOf(src.Extra[extraBody], "name"); sentName != "" && src.Name == src.Extra[extraName] {
		name = sentName
	}
	impl, contract, proto := implOf(src.Format)
	m["id"], m["name"], m["implementation"], m["implementationName"], m["configContract"], m["protocol"] = src.ID, name, impl, impl, contract, proto
	m["priority"], m["enableRss"], m["enableAutomaticSearch"], m["enableInteractiveSearch"] = src.Priority, src.UseFor.Feed, src.UseFor.AutoSearch, src.UseFor.HandSearch
	m["supportsRss"], m["supportsSearch"] = true, true
	if _, ok := valueOf(m, "tags"); !ok {
		m["tags"] = []any{}
	}
	return m
}

// valueStringOf reads one text value of a kept entry.
func valueStringOf(kept, key string) string {
	var m map[string]any
	if json.Unmarshal([]byte(kept), &m) != nil {
		return ""
	}
	return valueString(m, key)
}

func (h *handler) listIndexers(w http.ResponseWriter, r *http.Request) {
	srcs, err := database.Select[domain.Source](r.Context(), h.Store.Read(), "sources", "json_extract(extra, '$."+extraSide+"') = ? ORDER BY id", h.side)
	if err != nil {
		internalError(w, err)
		return
	}
	out := []map[string]any{}
	for _, s := range srcs {
		out = append(out, h.indexerOf(s))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handler) getIndexer(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	src, err := h.ownSource(r.Context(), id)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, h.indexerOf(src))
}

// cutName cuts a name to n bytes, since Source names are counted in
// bytes. It drops whole characters only.
func cutName(s string, n int) string {
	for len(s) > n {
		_, size := utf8.DecodeLastRuneInString(s)
		s = s[:len(s)-size]
	}
	return strings.TrimSpace(s)
}

// pickName returns the Source name for an indexer. It is the name sent,
// unless another Source has it, such as the same indexer on the other
// side or one made by hand. Then this side's word goes after it.
func (h *handler) pickName(ctx context.Context, want string, own int64) (string, bool) {
	word := " (shows)"
	if h.side == "radarr" {
		word = " (movies)"
	}
	for _, n := range []string{cutName(want, 80), cutName(want, 80-len(word)) + word} {
		if c, err := sources.CountNamed(ctx, h.Store.Read(), n, own); err == nil && c == 0 {
			return n, true
		}
	}
	return "", false
}

// hostOnly returns scheme://host of an address, which is how Sources
// compare them.
func hostOnly(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return u.Scheme + "://" + u.Host
}

// tryPush checks that an indexer answers its caps, as the old apps test
// before they save. When given, stored lends its key to a masked one, but
// only on the same host. A stored key never goes to a new host on its own.
func (h *handler) tryPush(w http.ResponseWriter, r *http.Request, p push, stored *domain.Source) bool {
	key := p.key
	if p.keepKey && stored != nil && stored.SecretRef != nil && h.Secrets != nil {
		if hostOnly(p.url) != hostOnly(stored.URL) {
			refuse(w, failure{PropertyName: "ApiKey", ErrorMessage: sources.ErrKeyAgain.Error()})
			return false
		}
		k, err := h.Secrets.Get(r.Context(), *stored.SecretRef)
		if err != nil {
			internalError(w, err)
			return false
		}
		key = k
	}
	cfg := sources.Config{URL: p.url, APIKey: key, Protocol: p.protocol, Format: p.format, AllowLocal: true, Timeout: 20 * time.Second}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if _, err := sources.New(cfg).Test(ctx); err != nil {
		refuse(w, failure{PropertyName: "BaseUrl", ErrorMessage: sources.Explain(err), AttemptedValue: p.url})
		return false
	}
	return true
}

// testIndexer tests an unsaved indexer. Prowlarr's own test sends one
// pointing at itself, so it may reach the local network.
func (h *handler) testIndexer(w http.ResponseWriter, r *http.Request) {
	if !h.mayWriteSources(w, r) {
		return
	}
	p, ok := h.readPush(w, r)
	if !ok {
		return
	}
	// A saved indexer sent back with a masked key is tested with its
	// stored key.
	var stored *domain.Source
	if p.keepKey && p.id > 0 {
		if src, err := h.ownSource(r.Context(), p.id); err == nil {
			stored = &src
		}
	}
	if !h.tryPush(w, r, p, stored) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{})
}

// sourceFailure answers a Source that was not saved.
func sourceFailure(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, database.ErrNotFound):
		notFound(w)
	case errors.Is(err, sources.ErrNoVault):
		message(w, http.StatusServiceUnavailable, err.Error())
	case errors.Is(err, sources.ErrNameUsed):
		refuse(w, failure{PropertyName: "Name", ErrorMessage: err.Error()})
	case errors.Is(err, sources.ErrKeyAgain), errors.Is(err, sources.ErrKeyAgainProxy):
		refuse(w, failure{PropertyName: "ApiKey", ErrorMessage: err.Error()})
	case errors.Is(err, sources.ErrURL), errors.Is(err, sources.ErrPriority), errors.Is(err, sources.ErrSeeders), errors.Is(err, sources.ErrName),
		errors.Is(err, sources.ErrFormat), errors.Is(err, sources.ErrProtocol), errors.Is(err, sources.ErrKinds), errors.Is(err, sources.ErrUse):
		refuse(w, failure{ErrorMessage: err.Error()})
	default:
		internalError(w, err)
	}
}

// extras builds the Source extras that mark it as made here.
func (h *handler) extras(old map[string]string, p push, name string) map[string]string {
	out := map[string]string{}
	for k, v := range old {
		out[k] = v
	}
	out[extraSide], out[extraBody], out[extraName] = h.side, p.keep, name
	return out
}

func (p push) input(kind, name string) sources.Input {
	in := sources.Input{Name: name, Format: p.format, Protocol: p.protocol, URL: p.url, APIKey: p.key, UseFor: &p.use, Kinds: []string{kind},
		Categories: map[string][]int{kind: p.cats}, MinSeeders: p.seeders, Priority: ptr(p.priority), AllowLocal: ptr(true)}
	return in
}

func (h *handler) saved(src domain.Source) {
	if h.SourceSaved != nil {
		h.SourceSaved(src)
	}
}

// addIndexer makes a paused Source from an indexer. Without forceSave it
// is tested first, and a failed test answers 400. Prowlarr then sends it
// again with forceSave.
func (h *handler) addIndexer(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.mayWriteSources(w, r) {
		return
	}
	p, ok := h.readPush(w, r)
	if !ok {
		return
	}
	if !queryBool(r, "forceSave") && !h.tryPush(w, r, p, nil) {
		return
	}
	name, ok := h.pickName(ctx, p.name, 0)
	if !ok {
		refuse(w, failure{PropertyName: "Name", ErrorMessage: sources.ErrNameUsed.Error(), AttemptedValue: p.name})
		return
	}
	extra, _ := json.Marshal(h.extras(nil, p, name))
	src, err := sources.Add(ctx, h.Store, h.Secrets, p.input(h.sourceKind(), name), actor(r), func(tx *sql.Tx, id int64) error {
		return sources.DisableTx(tx, id, string(extra))
	})
	if err != nil {
		sourceFailure(w, err)
		return
	}
	if src, err = h.ownSource(ctx, src.ID); err != nil {
		internalError(w, err)
		return
	}
	h.saved(src)
	writeJSON(w, http.StatusCreated, h.indexerOf(src))
}

// keySame reports whether a sent key leaves the stored one as it is.
func (h *handler) keySame(ctx context.Context, src domain.Source, p push) bool {
	switch {
	case p.keepKey:
		return true
	case p.key == "":
		return src.SecretRef == nil
	case src.SecretRef == nil || h.Secrets == nil:
		return false
	}
	k, err := h.Secrets.Get(ctx, *src.SecretRef)
	return err == nil && k == p.key
}

// putIndexer changes a Source made here. An unchanged indexer writes
// nothing. A moved address pauses the Source again, so it reaches the
// local network only after a person resumes it.
func (h *handler) putIndexer(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.mayWriteSources(w, r) {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	src, err := h.ownSource(ctx, id)
	if err != nil {
		internalError(w, err)
		return
	}
	p, ok := h.readPush(w, r)
	if !ok {
		return
	}
	kind := h.sourceKind()
	name := src.Name
	if p.name != valueStringOf(src.Extra[extraBody], "name") || src.Name != src.Extra[extraName] {
		if name, ok = h.pickName(ctx, p.name, src.ID); !ok {
			refuse(w, failure{PropertyName: "Name", ErrorMessage: sources.ErrNameUsed.Error(), AttemptedValue: p.name})
			return
		}
	}
	keySame := h.keySame(ctx, src, p)
	if name == src.Name && p.format == src.Format && p.url == src.URL && slices.Equal(p.cats, sortedInts(src.Categories[kind])) &&
		(p.seeders == nil || src.MinSeeders != nil && *p.seeders == *src.MinSeeders) && p.priority == src.Priority && p.use == src.UseFor &&
		src.AllowLocal && slices.Equal(src.Kinds, []string{kind}) && p.keep == src.Extra[extraBody] && name == src.Extra[extraName] && keySame {
		writeJSON(w, http.StatusAccepted, h.indexerOf(src))
		return
	}
	if !queryBool(r, "forceSave") && (p.url != src.URL || !keySame) && !h.tryPush(w, r, p, &src) {
		return
	}
	in := p.input(kind, name)
	in.ClearKey = p.key == "" && !p.keepKey
	// Turning the local network back on after a person turned it off
	// counts as a move too.
	moved := p.url != src.URL || !src.AllowLocal
	out, err := sources.Change(ctx, h.Store, h.Secrets, src.ID, in, actor(r), func(_ *sql.Tx, s *domain.Source) error {
		s.Extra = h.extras(s.Extra, p, name)
		if moved {
			s.State, s.PauseReason, s.PausedUntil = "disabled", nil, nil
		}
		return nil
	})
	if err != nil {
		sourceFailure(w, err)
		return
	}
	h.saved(out)
	writeJSON(w, http.StatusAccepted, h.indexerOf(out))
}

// deleteIndexer removes a Source made here. Any other id answers 404.
func (h *handler) deleteIndexer(w http.ResponseWriter, r *http.Request) {
	if !h.mayWriteSources(w, r) {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if _, err := h.ownSource(r.Context(), id); err != nil {
		internalError(w, err)
		return
	}
	if err := sources.Remove(r.Context(), h.Store, id); err != nil {
		internalError(w, err)
		return
	}
	if h.SourceRemoved != nil {
		h.SourceRemoved(id)
	}
	writeJSON(w, http.StatusOK, map[string]any{})
}
