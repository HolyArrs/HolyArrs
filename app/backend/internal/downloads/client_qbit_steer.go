package downloads

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// qbitVersionTTL is how long a read API version is trusted.
// qbitVersionRecheck is how soon an old answer is read again when it said a feature is missing, so an upgraded client is noticed.
const qbitVersionTTL = time.Hour

const qbitVersionRecheck = 2 * time.Minute

// ErrShareAction means the client is too old for a per-torrent limit action.
var ErrShareAction = errors.New("this Downloader cannot set what happens at the limit per torrent (it needs qBittorrent 5.2 or newer); leave it on the default")

// shareActionWord maps our action names to the words qBittorrent uses.
var shareActionWord = map[string]string{"": "Default", "stop": "Stop", "remove": "Remove", "remove_files": "RemoveWithContent", "super_seed": "EnableSuperSeeding"}

// ErrTrackerTier means the client is too old to change a tracker tier.
var ErrTrackerTier = errors.New("this Downloader cannot change a tracker's tier (it needs qBittorrent 5.2 or newer)")

// ShareLimits are a torrent's own seeding rules. Action says what to do when a limit is hit; empty means the client default.
type ShareLimits struct {
	Ratio           float64 `json:"ratio"`
	SeedingMinutes  int64   `json:"seeding_minutes"`
	InactiveMinutes int64   `json:"inactive_minutes"`
	Action          string  `json:"action"`
}

// ShareLimiter is the optional part of an engine that sets seeding rules per torrent.
type ShareLimiter interface {
	SetShareLimits(ctx context.Context, hashes []string, l ShareLimits) error
}

// SlowModer is the optional part of an engine that switches the client's alternative speed limits.
type SlowModer interface {
	SetSlowMode(ctx context.Context, on bool) error
}

// TrackerEditor is the optional part of an engine that changes a torrent's trackers. Trackers are named by a ref, never by address, so the caller cannot inject one.
type TrackerEditor interface {
	AddTrackers(ctx context.Context, id string, urls []string) error
	EditTracker(ctx context.Context, id, ref, newURL string, tier *int) error
	RemoveTrackers(ctx context.Context, id string, refs []string) error
}

// Mover is the optional part of an engine that moves a torrent's data and sets its speed caps.
type Mover interface {
	SetLocation(ctx context.Context, hashes []string, folder string) error
	SetTorrentLimits(ctx context.Context, hashes []string, down, up int64) error
	// SetCategory moves one torrent to a category, made first when missing.
	SetCategory(ctx context.Context, id, category string) error
}

// SetShareLimits sets seeding rules. The action field needs a newer web API; with an older client any non-default action is refused instead of silently ignored.
func (q *Qbittorrent) SetShareLimits(ctx context.Context, hashes []string, l ShareLimits) error {
	hs, err := hashList(hashes)
	if err != nil {
		return err
	}
	word, ok := shareActionWord[l.Action]
	if !ok {
		return ErrShareAction
	}
	f := url.Values{"hashes": {hs}, "ratioLimit": {strconv.FormatFloat(l.Ratio, 'f', -1, 64)},
		"seedingTimeLimit": {strconv.FormatInt(l.SeedingMinutes, 10)}, "inactiveSeedingTimeLimit": {strconv.FormatInt(l.InactiveMinutes, 10)}}
	newer, err := q.qbitAPIAtLeast(ctx, 2, 12, 0)
	if err != nil {
		return err
	}
	if newer {
		f.Set("shareLimitAction", word)
	} else if l.Action != "" {
		return ErrShareAction
	}
	_, err = q.call(ctx, http.MethodPost, "torrents/setShareLimits", f)
	return err
}

// SetSlowMode turns the alternative speed limits on or off. The client only offers a toggle, so read it first and skip when it already matches.
func (q *Qbittorrent) SetSlowMode(ctx context.Context, on bool) error {
	b, err := q.call(ctx, http.MethodGet, "transfer/speedLimitsMode", nil)
	if err != nil {
		return err
	}
	if (strings.TrimSpace(string(b)) == "1") == on {
		return nil
	}
	_, err = q.call(ctx, http.MethodPost, "transfer/toggleSpeedLimitsMode", url.Values{})
	return err
}

// AddTrackers adds tracker addresses to one torrent.
func (q *Qbittorrent) AddTrackers(ctx context.Context, id string, urls []string) error {
	h, err := qbitHash(id)
	if err != nil {
		return err
	}
	if len(urls) == 0 {
		return nil
	}
	_, err = q.call(ctx, http.MethodPost, "torrents/addTrackers", url.Values{"hash": {h}, "urls": {strings.Join(urls, "\n")}})
	return err
}

// EditTracker changes a tracker address or tier. Newer clients renamed the old address field and added tier.
// An empty newURL keeps the address.
func (q *Qbittorrent) EditTracker(ctx context.Context, id, ref, newURL string, tier *int) error {
	h, err := qbitHash(id)
	if err != nil {
		return err
	}
	all, err := q.trackerURLs(ctx, h)
	if err != nil {
		return err
	}
	orig, ok := all[ref]
	if !ok {
		return ErrNotFound
	}
	if newURL == "" {
		newURL = orig
	}
	f := url.Values{"hash": {h}, "newUrl": {newURL}}
	newer, err := q.qbitAPIAtLeast(ctx, 2, 13, 0)
	if err != nil {
		return err
	}
	if newer {
		f.Set("url", orig)
		if tier != nil {
			f.Set("tier", strconv.Itoa(*tier))
		}
	} else {
		if tier != nil {
			return ErrTrackerTier
		}
		f.Set("origUrl", orig)
	}
	_, err = q.call(ctx, http.MethodPost, "torrents/editTracker", f)
	return err
}

// RemoveTrackers removes trackers by ref. Addresses are joined with a bar, so TrackerAllowed refuses any that hold one.
func (q *Qbittorrent) RemoveTrackers(ctx context.Context, id string, refs []string) error {
	h, err := qbitHash(id)
	if err != nil {
		return err
	}
	all, err := q.trackerURLs(ctx, h)
	if err != nil {
		return err
	}
	urls := make([]string, 0, len(refs))
	for _, r := range refs {
		u, ok := all[r]
		if !ok {
			return ErrNotFound
		}
		urls = append(urls, u)
	}
	if len(urls) == 0 {
		return nil
	}
	_, err = q.call(ctx, http.MethodPost, "torrents/removeTrackers", url.Values{"hash": {h}, "urls": {strings.Join(urls, "|")}})
	return err
}

// TrackerAllowed reports whether a user-given tracker address is safe to send to the client. Bars and line breaks are refused because they split the list.
func TrackerAllowed(v string, allowLocal bool) bool {
	return !strings.ContainsAny(v, "|\r\n") && trackerAllowed(v, allowLocal)
}

// SetLocation moves the data of torrents to a folder.
func (q *Qbittorrent) SetLocation(ctx context.Context, hashes []string, folder string) error {
	hs, err := hashList(hashes)
	if err != nil {
		return err
	}
	if folder == "" {
		return ErrNotFound
	}
	_, err = q.call(ctx, http.MethodPost, "torrents/setLocation", url.Values{"hashes": {hs}, "location": {folder}})
	return err
}

// SetTorrentLimits sets per-torrent speed caps in bytes per second. Zero or less means no cap.
func (q *Qbittorrent) SetTorrentLimits(ctx context.Context, hashes []string, down, up int64) error {
	hs, err := hashList(hashes)
	if err != nil {
		return err
	}
	if _, err := q.call(ctx, http.MethodPost, "torrents/setDownloadLimit", url.Values{"hashes": {hs}, "limit": {strconv.FormatInt(max(down, 0), 10)}}); err != nil {
		return err
	}
	_, err = q.call(ctx, http.MethodPost, "torrents/setUploadLimit", url.Values{"hashes": {hs}, "limit": {strconv.FormatInt(max(up, 0), 10)}})
	return err
}

// qbitAPI returns the web API version, cached for an hour. A failed read is not cached.
func (q *Qbittorrent) qbitAPI(ctx context.Context) ([3]int, error) {
	q.verMu.Lock()
	if !q.verAt.IsZero() && time.Since(q.verAt) < qbitVersionTTL {
		v := q.ver
		q.verMu.Unlock()
		return v, nil
	}
	q.verMu.Unlock()
	var v [3]int
	b, err := q.call(ctx, http.MethodGet, "app/webapiVersion", nil)
	if err == nil {
		parts := strings.SplitN(strings.TrimSpace(string(b)), ".", 3)
		for i, p := range parts {
			n, perr := strconv.Atoi(p)
			if perr != nil {
				err = fmt.Errorf("qBittorrent sent an API version that does not read: %q", strings.TrimSpace(string(b)))
				break
			}
			v[i] = n
		}
	}
	q.verMu.Lock()
	defer q.verMu.Unlock()
	if err != nil {
		q.verAt = time.Time{}
		return [3]int{}, err
	}
	q.ver, q.verAt = v, time.Now()
	return v, nil
}

// qbitAPIAtLeast reports whether the client has at least this API version. A cached "too old" is checked again after a short wait, since the user may have upgraded.
func (q *Qbittorrent) qbitAPIAtLeast(ctx context.Context, a, b, c int) (bool, error) {
	v, err := q.qbitAPI(ctx)
	if err != nil || atLeast(v, a, b, c) {
		return err == nil, err
	}
	q.verMu.Lock()
	if time.Since(q.verAt) < qbitVersionRecheck {
		q.verMu.Unlock()
		return false, nil
	}
	q.verAt = time.Time{}
	q.verMu.Unlock()
	v, err = q.qbitAPI(ctx)
	return err == nil && atLeast(v, a, b, c), err
}

// atLeast compares API versions part by part.
func atLeast(v [3]int, a, b, c int) bool {
	if v[0] != a {
		return v[0] > a
	}
	if v[1] != b {
		return v[1] > b
	}
	return v[2] >= c
}

// fixedSource names the pseudo trackers qBittorrent lists for DHT, PeX and LSD. They are not real trackers.
func fixedSource(u string) string {
	switch {
	case strings.Contains(u, "DHT"):
		return "DHT"
	case strings.Contains(u, "PeX"):
		return "PeX"
	case strings.Contains(u, "LSD"):
		return "LSD"
	}
	return ""
}

// trackerRef is a short stable id for a tracker address, so the screen can point at one without sending the address back.
func trackerRef(u string) string {
	h := sha256.Sum256([]byte(u))
	return hex.EncodeToString(h[:8])
}

// trackerURLs maps refs to addresses for real trackers. A negative tier marks the pseudo entries, which are skipped.
func (q *Qbittorrent) trackerURLs(ctx context.Context, h string) (map[string]string, error) {
	b, err := q.call(ctx, http.MethodGet, "torrents/trackers", url.Values{"hash": {h}})
	if err != nil {
		return nil, err
	}
	var rows []struct {
		URL  string `json:"url"`
		Tier int    `json:"tier"`
	}
	if err := json.Unmarshal(b, &rows); err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, r := range rows {
		if r.Tier >= 0 {
			out[trackerRef(r.URL)] = r.URL
		}
	}
	return out, nil
}

// SetCategory moves one torrent to category, making the category first when
// qBittorrent does not have it yet. A torrent the client no longer holds is
// ErrNotFound.
func (q *Qbittorrent) SetCategory(ctx context.Context, id, category string) error {
	h, err := qbitHash(id)
	if err != nil {
		return err
	}
	if category == "" || len(category) > 64 || strings.ContainsAny(category, `/\`) {
		return ErrCategory
	}
	js, err := q.info(ctx, url.Values{"hashes": {h}})
	if err != nil {
		return err
	}
	if len(js) == 0 {
		return ErrNotFound
	}
	if _, err := q.call(ctx, http.MethodPost, "torrents/createCategory", url.Values{"category": {category}}, http.StatusConflict); err != nil {
		return err
	}
	_, err = q.call(ctx, http.MethodPost, "torrents/setCategory", url.Values{"hashes": {h}, "category": {category}})
	return err
}
