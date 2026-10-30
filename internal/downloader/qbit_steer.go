package downloader

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
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

// qbitAPI reads the client's API version as three numbers; an answer that
// does not read is 0.0.0 (the oldest rules).
func (q *Qbittorrent) qbitAPI(ctx context.Context) [3]int {
	var v [3]int
	b, err := q.call(ctx, http.MethodGet, "app/webapiVersion", nil)
	if err != nil {
		return v
	}
	for i, p := range strings.SplitN(strings.TrimSpace(string(b)), ".", 3) {
		n, err := strconv.Atoi(p)
		if err != nil {
			return [3]int{}
		}
		v[i] = n
	}
	return v
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
	if atLeast(q.qbitAPI(ctx), 2, 12, 0) {
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
