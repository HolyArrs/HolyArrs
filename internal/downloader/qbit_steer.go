package downloader

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

// Hand steering of single torrents from the Downloads screen, as the
// client's own screen offers it: seeding rules, folder, speed limits,
// trackers and the whole client's slow mode.

// ShareLimits are a torrent's own seeding rules. Ratio and the minutes use
// the client's values: -2 follows the client's global rule, -1 is no limit.
// Action is what happens at a limit: "" (the client's default), "stop",
// "remove", "remove_files" (with its files) or "super_seed".
type ShareLimits struct {
	Ratio           float64 `json:"ratio"`
	SeedingMinutes  int64   `json:"seeding_minutes"`
	InactiveMinutes int64   `json:"inactive_minutes"`
	Action          string  `json:"action"`
}

// ShareLimiter is the optional part of a torrent engine that sets seeding
// rules on many torrents at once.
type ShareLimiter interface {
	SetShareLimits(ctx context.Context, hashes []string, l ShareLimits) error
}

// ErrShareAction is a per-torrent action the client cannot take: qBittorrent
// before 5.2 has no action per torrent.
var ErrShareAction = errors.New("this Downloader cannot set what happens at the limit per torrent (it needs qBittorrent 5.2 or newer); leave it on the default")

// qbitVersionTTL is how long a read API version is trusted.
const qbitVersionTTL = time.Hour

// qbitAPI reads the client's API version as three numbers. A good read is
// kept for qbitVersionTTL; a failed or unreadable one is an error, never
// taken as the oldest rules, and clears what was kept.
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

// qbitAPIAtLeast says the client's API is a.b.c or newer. A kept version
// that is too old is read again once, so an upgraded client is seen at once.
func (q *Qbittorrent) qbitAPIAtLeast(ctx context.Context, a, b, c int) (bool, error) {
	v, err := q.qbitAPI(ctx)
	if err != nil || atLeast(v, a, b, c) {
		return err == nil, err
	}
	q.verMu.Lock()
	q.verAt = time.Time{}
	q.verMu.Unlock()
	v, err = q.qbitAPI(ctx)
	return err == nil && atLeast(v, a, b, c), err
}

// atLeast says version v is a or newer.
func atLeast(v [3]int, a, b, c int) bool {
	if v[0] != a {
		return v[0] > a
	}
	if v[1] != b {
		return v[1] > b
	}
	return v[2] >= c
}

var shareActionWord = map[string]string{"": "Default", "stop": "Stop", "remove": "Remove", "remove_files": "RemoveWithContent", "super_seed": "EnableSuperSeeding"}

// SetShareLimits sets seeding rules. API 2.12 (qBittorrent 5.2) takes the
// action per torrent and wants it sent; older ones take the three limits
// only, so any action but the default is refused there.
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

// SlowModer is the optional part of a torrent engine that turns the
// client's second, slower set of speed limits on or off.
type SlowModer interface {
	SetSlowMode(ctx context.Context, on bool) error
}

// SetSlowMode reads the client's mode and flips it only when it differs,
// which every 5.x version takes.
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

// fixedSource names the client's own peer source behind a fixed tracker
// row ("" when it is none of them).
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

// trackerRef names a tracker by a short hash of its full address.
func trackerRef(u string) string {
	h := sha256.Sum256([]byte(u))
	return hex.EncodeToString(h[:8])
}

// TrackerEditor is the optional part of a torrent engine that adds, edits
// and removes one torrent's trackers. Edit and remove name trackers by Ref.
type TrackerEditor interface {
	AddTrackers(ctx context.Context, id string, urls []string) error
	EditTracker(ctx context.Context, id, ref, newURL string, tier *int) error
	RemoveTrackers(ctx context.Context, id string, refs []string) error
}

// ErrTrackerTier is a tier change on a client that cannot set one.
var ErrTrackerTier = errors.New("this Downloader cannot change a tracker's tier (it needs qBittorrent 5.2 or newer)")

// trackerURLs reads a torrent's real tracker addresses by Ref.
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

// EditTracker changes one tracker's address or tier. API 2.13 (5.2) names
// the old address url and takes a tier; older ones name it origUrl and
// take no tier.
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

// RemoveTrackers removes trackers of one torrent by Ref.
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

// TrackerAllowed checks a tracker address someone typed, with the same
// rules as magnet trackers: http, https or udp, no user info, and without
// local network access no local, metadata or number-style hosts.
func TrackerAllowed(v string, allowLocal bool) bool {
	return !strings.ContainsAny(v, "|\r\n") && trackerAllowed(v, allowLocal)
}

// Mover is the optional part of a torrent engine that moves torrents' data
// to another folder and caps single torrents' speeds.
type Mover interface {
	SetLocation(ctx context.Context, hashes []string, folder string) error
	SetTorrentLimits(ctx context.Context, hashes []string, down, up int64) error
}

// SetLocation moves torrents' data to a folder (the client makes it when
// missing); qBittorrent turns automatic folder handling off for them.
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

// SetTorrentLimits caps torrents' download and upload speeds in bytes per
// second; 0 removes a cap.
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
