package downloads

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
)

// Bulk actions the Downloads screen can apply to many torrents at once.
const (
	BulkStart      = "start"
	BulkPause      = "pause"
	BulkRecheck    = "recheck"
	BulkReannounce = "reannounce"
	BulkTop        = "top"
	BulkBottom     = "bottom"
	BulkUp         = "up"
	BulkDown       = "down"
	BulkForce      = "force"
	BulkUnforce    = "unforce"
)

// BulkActions lists every action Bulk accepts.
var BulkActions = []string{BulkStart, BulkPause, BulkRecheck, BulkReannounce, BulkTop, BulkBottom, BulkUp, BulkDown, BulkForce, BulkUnforce}

// ErrQueueOff means the client does not queue, so moving in the queue has no meaning.
var ErrQueueOff = errors.New("the Downloader has its queue turned off, so there is no order to change")

// ErrBulkAction means the action name is not one of the Bulk constants.
var ErrBulkAction = errors.New("unknown action")

// TorrentBulk is the optional part of an engine that acts on many torrents in one call.
type TorrentBulk interface {
	Bulk(ctx context.Context, action string, hashes []string) error
	RemoveMany(ctx context.Context, hashes []string, deleteFiles bool) error
}

// Bulk applies one action to the listed torrents. qBittorrent answers 409 to a queue move when queueing is off.
func (q *Qbittorrent) Bulk(ctx context.Context, action string, hashes []string) error {
	hs, err := hashList(hashes)
	if err != nil {
		return err
	}
	f := url.Values{"hashes": {hs}}
	var path string
	switch action {
	case BulkStart, BulkPause:
		return q.qbitStartStop(ctx, hs, action == BulkStart)
	case BulkRecheck:
		path = "torrents/recheck"
	case BulkReannounce:
		path = "torrents/reannounce"
	case BulkTop, BulkBottom, BulkUp, BulkDown:
		path = map[string]string{BulkTop: "torrents/topPrio", BulkBottom: "torrents/bottomPrio", BulkUp: "torrents/increasePrio", BulkDown: "torrents/decreasePrio"}[action]
		code, err := q.callCode(ctx, path, f, http.StatusConflict)
		if err == nil && code == http.StatusConflict {
			return ErrQueueOff
		}
		return err
	case BulkForce, BulkUnforce:
		f.Set("value", map[string]string{BulkForce: "true", BulkUnforce: "false"}[action])
		path = "torrents/setForceStart"
	default:
		return ErrBulkAction
	}
	_, err = q.call(ctx, http.MethodPost, path, f)
	return err
}

// RemoveMany removes torrents from the client in one call.
func (q *Qbittorrent) RemoveMany(ctx context.Context, hashes []string, deleteFiles bool) error {
	hs, err := hashList(hashes)
	if err != nil {
		return err
	}
	_, err = q.call(ctx, http.MethodPost, "torrents/delete", url.Values{"hashes": {hs}, "deleteFiles": {boolWord(deleteFiles)}})
	return err
}

// hashList checks and de-duplicates hashes and joins them the way qBittorrent wants. An empty list is not found.
func hashList(hashes []string) (string, error) {
	if len(hashes) == 0 {
		return "", ErrNotFound
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(hashes))
	for _, h := range hashes {
		h, err := qbitHash(h)
		if err != nil {
			return "", err
		}
		if !seen[h] {
			seen[h] = true
			out = append(out, h)
		}
	}
	return strings.Join(out, "|"), nil
}

// callCode posts a form and returns the status code. A sign-in that expired is retried once; a refused API key is not, since signing in again cannot help.
func (q *Qbittorrent) callCode(ctx context.Context, path string, f url.Values, okCodes ...int) (int, error) {
	code, _, err := q.doRaw(ctx, http.MethodPost, path, f, "", nil, okCodes...)
	if errors.Is(err, ErrAuth) && q.user != "" {
		if err := q.signIn(ctx); err != nil {
			return 0, err
		}
		code, _, err = q.doRaw(ctx, http.MethodPost, path, f, "", nil, okCodes...)
	} else if errors.Is(err, ErrAuth) && q.key != "" {
		return 0, ErrQbitKeyRefused
	}
	return code, err
}

// boolWord writes a bool the way qBittorrent forms expect.
func boolWord(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
