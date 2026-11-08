package bridge

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/source"
)

// KindCheck is the job kind that keeps every Jackett bridge's Sources in
// step with its sites.
const KindCheck = "check_bridges"

// Keys in a Source's extra data that tie it to the bridge it came from.
const (
	extraBridge = "bridge"
	extraSite   = "bridge_site"
)

// Keys in a Jackett bridge's extra data. known_sites lists the sites it has
// brought in, so a site with no Source was removed by the person and stays
// away. local is "1" when the address is on the local network.
const (
	extraKnown = "known_sites"
	extraLocal = "local"
)

// ErrTaken says that this Jackett or Prowlarr is already connected.
var ErrTaken = errors.New("already connected")

// Bridge is one connected Jackett or Prowlarr.
type Bridge struct {
	domain.Meta
	Kind        string            `db:"kind"`
	URL         string            `db:"url"`
	SecretRef   *int64            `db:"secret_ref"`
	CallbackURL string            `db:"callback_url"`
	LastCheck   *time.Time        `db:"last_check"`
	LastError   string            `db:"last_error"`
	SiteCount   int               `db:"site_count"`
	Extra       map[string]string `db:"extra,json"`
}

// Result is what one check of a Jackett did.
type Result struct {
	Sites   int `json:"sites"`
	Added   int `json:"added"`
	Updated int `json:"updated"`
	Removed int `json:"removed"`
	Skipped int `json:"skipped"` // sites whose Source the person removed
}

// Service keeps bridges and their Sources.
type Service struct {
	Store *database.Store
	Vault *secrets.Vault

	mu sync.Mutex
	// checked holds the last quiet check in memory, so an unchanged check
	// writes nothing.
	checked map[int64]time.Time
	// work lets one check or removal change a bridge's Sources at a time.
	// Without it, Check now beside the scheduled check could add a site twice.
	work sync.Mutex
}

// List returns every bridge, oldest first.
func (s *Service) List(ctx context.Context) ([]Bridge, error) {
	return database.Select[Bridge](ctx, s.Store.Read(), "bridges", "1=1 ORDER BY id")
}

// Get returns one bridge.
func (s *Service) Get(ctx context.Context, id int64) (Bridge, error) {
	var b Bridge
	err := database.Get(ctx, s.Store.Read(), "bridges", id, &b)
	return b, err
}

// LastChecked returns the newest check time, a quiet one from memory or the
// stored one.
func (s *Service) LastChecked(b Bridge) *time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t, ok := s.checked[b.ID]; ok && (b.LastCheck == nil || t.After(*b.LastCheck)) {
		return &t
	}
	return b.LastCheck
}

// Key returns a bridge's key. It is never shown to the person.
func (s *Service) Key(ctx context.Context, b Bridge) (string, error) {
	if b.SecretRef == nil || s.Vault == nil {
		return "", nil
	}
	return s.Vault.Get(ctx, *b.SecretRef)
}

// Add stores a new bridge with its key, or returns ErrTaken when that kind
// and address are already connected.
func (s *Service) Add(ctx context.Context, kind, url, key, callback string, by domain.Actor) (Bridge, error) {
	b := Bridge{Kind: kind, URL: url, CallbackURL: callback, Extra: map[string]string{}}
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM bridges WHERE kind = ? AND lower(url) = lower(?)", kind, url).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return ErrTaken
		}
		if key != "" {
			id, err := s.Vault.PutTx(tx, "bridge_api_key", key, by)
			if err != nil {
				return err
			}
			b.SecretRef = &id
		}
		_, err := database.Insert(tx, "bridges", &b, by)
		return err
	})
	return b, err
}

// Find returns the bridge of that kind at that address, if one is connected.
func (s *Service) Find(ctx context.Context, kind, url string) (Bridge, bool) {
	bs, err := s.List(ctx)
	if err != nil {
		return Bridge{}, false
	}
	for _, b := range bs {
		if b.Kind == kind && strings.EqualFold(strings.TrimRight(b.URL, "/"), strings.TrimRight(url, "/")) {
			return b, true
		}
	}
	return Bridge{}, false
}

// Update gives a connected bridge a new key and callback address. It is used
// when the key or HolyArrs's address changed and the person connects again.
func (s *Service) Update(ctx context.Context, b Bridge, key, callback string, by domain.Actor) (Bridge, error) {
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		if key != "" {
			if b.SecretRef != nil {
				if _, err := tx.ExecContext(ctx, "DELETE FROM secrets WHERE id = ?", *b.SecretRef); err != nil {
					return err
				}
			}
			id, err := s.Vault.PutTx(tx, "bridge_api_key", key, by)
			if err != nil {
				return err
			}
			b.SecretRef = &id
		}
		b.CallbackURL = callback
		// Connecting again brings back the sites the person removed.
		delete(b.Extra, extraKnown)
		return database.Update(tx, "bridges", &b, by)
	})
	if err != nil {
		return b, err
	}
	return s.Get(ctx, b.ID)
}

// Remove takes a bridge away and returns how many Sources it had brought in.
// With keepSources they stay as ordinary Sources that are no longer kept in
// step; otherwise they are removed too.
func (s *Service) Remove(ctx context.Context, b Bridge, keepSources bool) (int, error) {
	s.work.Lock()
	defer s.work.Unlock()
	srcs, err := s.sourcesOf(ctx, b)
	if err != nil {
		return 0, err
	}
	if !keepSources {
		for _, src := range srcs {
			if err := source.Remove(ctx, s.Store, src.ID); err != nil {
				return 0, err
			}
		}
	}
	err = s.Store.Write(ctx, func(tx *sql.Tx) error {
		if keepSources {
			if _, err := tx.ExecContext(ctx, `UPDATE sources SET extra = json_remove(extra, '$.bridge', '$.bridge_site') WHERE json_extract(extra, '$.bridge') = ?`, strconv.FormatInt(b.ID, 10)); err != nil {
				return err
			}
		}
		if b.SecretRef != nil {
			if _, err := tx.ExecContext(ctx, "DELETE FROM secrets WHERE id = ?", *b.SecretRef); err != nil {
				return err
			}
		}
		_, err := tx.ExecContext(ctx, "DELETE FROM bridges WHERE id = ?", b.ID)
		return err
	})
	return len(srcs), err
}

// SourceCount returns how many Sources a bridge brought in.
func (s *Service) SourceCount(ctx context.Context, b Bridge) int {
	srcs, _ := s.sourcesOf(ctx, b)
	return len(srcs)
}

// CheckJackett reads a Jackett's sites and keeps its Sources in step. A new
// site becomes a Source, a site that is gone loses its Source, and a change
// of address, key or category follows. Everything else on a Source (name,
// priority, use, limits, tags) belongs to the person and stays.
func (s *Service) CheckJackett(ctx context.Context, b Bridge, by domain.Actor) (Result, error) {
	s.work.Lock()
	defer s.work.Unlock()
	if fresh, err := s.Get(ctx, b.ID); err == nil {
		// Another check may have changed the bridge while this one waited.
		b = fresh
	}
	key, err := s.Key(ctx, b)
	if err != nil {
		return Result{}, err
	}
	j := NewJackett(b.URL, key)
	sites, err := j.Sites(ctx)
	if err != nil {
		s.record(ctx, b, 0, false, err)
		return Result{}, err
	}
	have, err := s.sourcesOf(ctx, b)
	if err != nil {
		return Result{}, err
	}
	res := Result{Sites: len(sites)}
	bySite := map[string]domain.Source{}
	for _, src := range have {
		bySite[src.Extra[extraSite]] = src
	}
	keyChanged := b.Extra["key_hint"] != hint(key)
	// Only a Jackett on this machine or the local network gives its Sources
	// the local-network allowance. A name that can't be looked up keeps the
	// last answer.
	local, ok := IsLocal(ctx, b.URL)
	if !ok {
		local = b.Extra[extraLocal] == "1"
	}
	known := map[string]bool{}
	var knownList []string
	_ = json.Unmarshal([]byte(b.Extra[extraKnown]), &knownList)
	for _, id := range knownList {
		known[id] = true
	}
	var failed []string
	// notIn holds the sites that could not be added, so they are tried again
	// next time.
	notIn := map[string]bool{}
	adopted := map[string]bool{}
	for _, site := range sites {
		feed := j.FeedURL(site.ID)
		kinds := kindList(site.Categories)
		if _, ok := bySite[site.ID]; !ok {
			// A Source the person added by hand for this site joins the bridge
			// instead of getting a twin.
			if own, err := database.Select[domain.Source](ctx, s.Store.Read(), "sources",
				"lower(rtrim(url, '/')) = lower(?) AND json_extract(extra, '$.bridge') IS NULL AND json_extract(extra, '$.old_app') IS NULL ORDER BY id LIMIT 1",
				strings.TrimRight(feed, "/")); err == nil && len(own) == 1 {
				src := own[0]
				if err := s.Store.Write(ctx, func(tx *sql.Tx) error {
					_, e := tx.ExecContext(ctx, `UPDATE sources SET extra = json_set(extra, '$.bridge', ?, '$.bridge_site', ?) WHERE id = ?`,
						strconv.FormatInt(b.ID, 10), site.ID, src.ID)
					return e
				}); err == nil {
					bySite[site.ID] = src
					adopted[site.ID] = true
					res.Updated++
				}
			}
		}
		if src, ok := bySite[site.ID]; ok {
			delete(bySite, site.ID)
			if src.URL == feed && !keyChanged && sameCategories(src.Categories, site.Categories) && src.AllowLocal == local {
				continue
			}
			in := source.Input{Name: src.Name, URL: feed, APIKey: key, Categories: site.Categories, AllowLocal: &local}
			if _, err := source.Change(ctx, s.Store, s.Vault, src.ID, in, by); err != nil {
				failed = append(failed, site.Title)
				continue
			}
			if !adopted[site.ID] {
				res.Updated++
			}
			continue
		}
		if known[site.ID] {
			// It was brought in before and the person removed it.
			res.Skipped++
			continue
		}
		in := source.Input{Name: s.freeName(ctx, site.Title), Format: "torznab", Protocol: "torrent", URL: feed, APIKey: key,
			Kinds: kinds, Categories: site.Categories, AllowLocal: &local}
		siteID := site.ID
		_, err := source.Add(ctx, s.Store, s.Vault, in, by, func(tx *sql.Tx, id int64) error {
			_, err := tx.ExecContext(ctx, `UPDATE sources SET extra = json_set(extra, '$.bridge', ?, '$.bridge_site', ?) WHERE id = ?`,
				strconv.FormatInt(b.ID, 10), siteID, id)
			return err
		})
		if err != nil {
			// One site that can't be added does not stop the others.
			failed = append(failed, site.Title)
			notIn[site.ID] = true
			continue
		}
		res.Added++
	}
	for _, src := range bySite {
		if err := source.Remove(ctx, s.Store, src.ID); err != nil {
			failed = append(failed, src.Name)
			continue
		}
		res.Removed++
	}
	changed := res.Added+res.Updated+res.Removed > 0 || keyChanged
	var now []string
	for _, site := range sites {
		if !notIn[site.ID] {
			now = append(now, site.ID)
		}
	}
	slices.Sort(now)
	nowJSON, _ := json.Marshal(now)
	localWord := ""
	if local {
		localWord = "1"
	}
	if keyChanged || string(nowJSON) != b.Extra[extraKnown] || localWord != b.Extra[extraLocal] {
		_ = s.Store.Write(ctx, func(tx *sql.Tx) error {
			_, e := tx.ExecContext(ctx, `UPDATE bridges SET extra = json_set(extra, '$.key_hint', ?, '$.known_sites', ?, '$.local', ?) WHERE id = ?`,
				hint(key), string(nowJSON), localWord, b.ID)
			return e
		})
	}
	if len(failed) > 0 {
		err := fail("HolyArrs could not bring in %s.", clip(strings.Join(failed, ", "), 200))
		s.record(ctx, b, len(sites), true, err)
		return res, nil
	}
	s.record(ctx, b, len(sites), changed, nil)
	return res, nil
}

// HandleCheck is the scheduled check of every Jackett bridge.
func (s *Service) HandleCheck(ctx context.Context, r *jobs.Run) error {
	bs, err := s.List(ctx)
	if err != nil {
		return err
	}
	var total Result
	var failed []string
	for _, b := range bs {
		if b.Kind != KindJackett {
			continue
		}
		res, err := s.CheckJackett(ctx, b, domain.System)
		if err != nil {
			failed = append(failed, b.URL+": "+Words(err))
			continue
		}
		total.Added += res.Added
		total.Updated += res.Updated
		total.Removed += res.Removed
		total.Sites += res.Sites
	}
	r.SetResult(map[string]any{"sites": total.Sites, "added": total.Added, "updated": total.Updated, "removed": total.Removed, "failed": strings.Join(failed, "; ")})
	return nil
}

// Probe says whether there is a Jackett bridge to check. When there is none
// the scheduled check does not run at all.
func (s *Service) Probe(ctx context.Context, _ time.Time) (bool, any, error) {
	var n int
	err := s.Store.Read().QueryRowContext(ctx, "SELECT count(*) FROM bridges WHERE kind = ?", KindJackett).Scan(&n)
	return n > 0, nil, err
}

// RecordProwlarr keeps the outcome of a Prowlarr connect or sync. Prowlarr
// writes its Sources through the old-app API, so they are counted there.
func (s *Service) RecordProwlarr(ctx context.Context, b Bridge, err error) {
	s.record(ctx, b, s.SourceCount(ctx, b), false, err)
}

// quiet remembers a check in memory without writing it.
func (s *Service) quiet(id int64, at time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.checked == nil {
		s.checked = map[int64]time.Time{}
	}
	s.checked[id] = at
}

// sourcesOf returns the Sources a bridge brought in. For Jackett these are
// the ones it made. For Prowlarr they are the ones its app sync wrote, which
// are old-app Sources whose feed is on this Prowlarr.
func (s *Service) sourcesOf(ctx context.Context, b Bridge) ([]domain.Source, error) {
	if b.Kind == KindProwlarr {
		return database.Select[domain.Source](ctx, s.Store.Read(), "sources",
			"json_extract(extra, '$.old_app') IS NOT NULL AND lower(url) LIKE lower(?) ESCAPE '\\' ORDER BY id", likePrefix(b.URL))
	}
	return database.Select[domain.Source](ctx, s.Store.Read(), "sources", "json_extract(extra, '$.bridge') = ? ORDER BY id", strconv.FormatInt(b.ID, 10))
}

// likePrefix returns a LIKE pattern for addresses that start with u, with
// the pattern characters in u escaped.
func likePrefix(u string) string {
	r := strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_")
	return r.Replace(strings.TrimRight(u, "/")) + "/%"
}

// record keeps the outcome of a check. A quiet check (same count, same
// error, nothing changed) is kept in memory only, so an idle app writes
// nothing.
func (s *Service) record(ctx context.Context, b Bridge, sites int, changed bool, err error) {
	now := time.Now().UTC()
	words := ""
	if err != nil {
		words = Words(err)
	}
	if err != nil {
		// A failed check keeps the last known count.
		sites = b.SiteCount
	}
	if !changed && sites == b.SiteCount && words == b.LastError && b.LastCheck != nil {
		s.quiet(b.ID, now)
		return
	}
	_ = s.Store.Write(ctx, func(tx *sql.Tx) error {
		_, e := tx.ExecContext(ctx, `UPDATE bridges SET last_check = ?, last_error = ?, site_count = ?, updated_at = ? WHERE id = ?`,
			database.FormatTime(now), words, sites, database.FormatTime(now), b.ID)
		return e
	})
	s.quiet(b.ID, now)
}

// hint tells keys apart without keeping one: its length and last two
// characters.
func hint(key string) string {
	if len(key) < 4 {
		return strconv.Itoa(len(key))
	}
	return strconv.Itoa(len(key)) + key[len(key)-2:]
}

// kindList returns the HolyArrs kinds a site has categories for.
func kindList(cats map[string][]int) []string {
	var out []string
	for _, k := range []string{"movie", "tv", "music"} {
		if len(cats[k]) > 0 {
			out = append(out, k)
		}
	}
	if len(out) == 0 {
		// A site with no standard category gets the usual two kinds.
		return []string{"movie", "tv"}
	}
	return out
}

// sameCategories compares two category sets, ignoring kinds with none.
func sameCategories(a, b map[string][]int) bool {
	return maps.EqualFunc(nonEmpty(a), nonEmpty(b), func(x, y []int) bool { return slices.Equal(x, y) })
}

// nonEmpty returns m without the kinds that have no categories.
func nonEmpty(m map[string][]int) map[string][]int {
	out := map[string][]int{}
	for k, v := range m {
		if len(v) > 0 {
			out[k] = v
		}
	}
	return out
}

// freeName returns a site's title, or "<title> (Jackett)" and then
// "<title> (Jackett 2)" and so on, until no Source has that name.
func (s *Service) freeName(ctx context.Context, title string) string {
	for i := 1; ; i++ {
		name := title
		switch {
		case i == 2:
			name = title + " (Jackett)"
		case i > 2:
			name = title + " (Jackett " + strconv.Itoa(i-1) + ")"
		}
		var n int
		_ = s.Store.Read().QueryRowContext(ctx, "SELECT count(*) FROM sources WHERE lower(name) = lower(?)", name).Scan(&n)
		if n == 0 {
			return name
		}
	}
}
