package downloader

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
)

// Bulk actions: one call for many torrents of one client, as the client's
// own screen does with a selection.
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

// BulkActions lists every action Bulk takes.
var BulkActions = []string{BulkStart, BulkPause, BulkRecheck, BulkReannounce, BulkTop, BulkBottom, BulkUp, BulkDown, BulkForce, BulkUnforce}

// TorrentBulk is the optional part of a torrent engine that acts on many of
// its torrents in one call.
type TorrentBulk interface {
	Bulk(ctx context.Context, action string, hashes []string) error
	RemoveMany(ctx context.Context, hashes []string, deleteFiles bool) error
}

// ErrQueueOff is a queue move while the client's queue is turned off.
var ErrQueueOff = errors.New("the Downloader has its queue turned off, so there is no order to change")

// ErrBulkAction is an action Bulk does not know.
var ErrBulkAction = errors.New("unknown action")

// hashList checks and joins info hashes into qBittorrent's pipe list; "all"
// and other selectors never pass.
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

// Bulk runs one action on many torrents in one call.
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

// callCode is a form POST that hands back the status too, for answers that
// differ only by it (409 on a queue move means the queue is off).
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

// RemoveMany removes many torrents, with or without their files.
func (q *Qbittorrent) RemoveMany(ctx context.Context, hashes []string, deleteFiles bool) error {
	hs, err := hashList(hashes)
	if err != nil {
		return err
	}
	_, err = q.call(ctx, http.MethodPost, "torrents/delete", url.Values{"hashes": {hs}, "deleteFiles": {boolWord(deleteFiles)}})
	return err
}

func boolWord(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
