package downloader

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs/internal/source"
)

// Qbittorrent talks to the qBittorrent Web API v2 (5.2 or newer). It signs
// in only with the client's API key, sent as a bearer header; there is no
// name, password or cookie. A 401 or 403 is ErrAuth.
type Qbittorrent struct {
	base string
	http *http.Client
	key  string
}

// NewQbittorrent makes an engine for the Web UI at base (such as
// http://qbittorrent:8090) with no key; only a client that trusts this
// machine's address answers it.
func NewQbittorrent(base string, h *http.Client) *Qbittorrent {
	if h == nil {
		h = &http.Client{Timeout: clientTimeout}
	}
	return &Qbittorrent{base: strings.TrimRight(base, "/"), http: h}
}

// NewQbittorrentKey is NewQbittorrent with the client's API key.
func NewQbittorrentKey(base, key string, h *http.Client) *Qbittorrent {
	q := NewQbittorrent(base, h)
	q.key = strings.TrimSpace(key)
	return q
}

func (q *Qbittorrent) call(ctx context.Context, method, path string, form url.Values, okCodes ...int) ([]byte, error) {
	return q.do(ctx, method, path, form, okCodes...)
}

func (q *Qbittorrent) do(ctx context.Context, method, path string, form url.Values, okCodes ...int) ([]byte, error) {
	var body io.Reader
	u := q.base + "/api/v2/" + path
	if method == http.MethodGet && len(form) > 0 {
		u += "?" + form.Encode()
	} else if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if q.key != "" {
		req.Header.Set("Authorization", "Bearer "+q.key)
	} else {
		// Without a key, qBittorrent checks Referer/Origin against its own address for writes.
		req.Header.Set("Referer", q.base)
	}
	resp, err := q.http.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return nil, fmt.Errorf("qbittorrent %s: %w", path, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300: // 5.x answers 202 to add
		return b, nil
	case resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized:
		return nil, ErrAuth
	}
	for _, c := range okCodes {
		if resp.StatusCode == c {
			return b, nil
		}
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	return nil, fmt.Errorf("qbittorrent %s: %s", path, resp.Status)
}

// Health asks for the app version.
func (q *Qbittorrent) Health(ctx context.Context) (Health, error) {
	b, err := q.call(ctx, http.MethodGet, "app/version", nil)
	if err != nil {
		return Health{}, err
	}
	return Health{Version: strings.TrimSpace(string(b)), At: time.Now().UTC()}, nil
}

// Add sends a torrent by URL into a category and save path.
func (q *Qbittorrent) Add(ctx context.Context, r AddRequest) (string, error) {
	if r.Category != "" {
		// 409 means it already exists.
		if _, err := q.call(ctx, http.MethodPost, "torrents/createCategory", url.Values{"category": {r.Category}}, http.StatusConflict); err != nil {
			return "", err
		}
	}
	f := url.Values{"urls": {r.URL}}
	if r.Category != "" {
		f.Set("category", r.Category)
	}
	if r.SavePath != "" {
		f.Set("savepath", r.SavePath)
	}
	b, err := q.call(ctx, http.MethodPost, "torrents/add", f)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(string(b)) == "Fails." {
		return "", errors.New("qbittorrent refused the torrent")
	}
	if h := strings.ToLower(r.InfoHash); source.ValidInfoHash(h) {
		return h, nil
	}
	return "", nil
}

// errBadHash refuses an id that is not one info hash: qBittorrent reads
// "all" and "a|b" in its hashes field as selectors for other torrents.
var errBadHash = errors.New("not a torrent info hash")

type qbitTorrent struct {
	Hash        string  `json:"hash"`
	Name        string  `json:"name"`
	Category    string  `json:"category"`
	SavePath    string  `json:"save_path"`
	ContentPath string  `json:"content_path"`
	Size        int64   `json:"size"`
	Progress    float64 `json:"progress"`
	DLSpeed     int64   `json:"dlspeed"`
	UPSpeed     int64   `json:"upspeed"`
	NumSeeds    int     `json:"num_seeds"`
	NumLeechs   int     `json:"num_leechs"`
	ETA         int64   `json:"eta"`
	State       string  `json:"state"`
}

func (t qbitTorrent) job() Job {
	eta := t.ETA
	if eta >= 8640000 || eta < 0 { // qBittorrent's "infinite"
		eta = 0
	}
	return Job{ID: strings.ToLower(t.Hash), Name: t.Name, Category: t.Category, SavePath: t.SavePath, ContentPath: t.ContentPath,
		Size: t.Size, Progress: t.Progress, Speed: t.DLSpeed, UpSpeed: max(t.UPSpeed, 0), Seeds: max(t.NumSeeds, 0), Peers: max(t.NumLeechs, 0), ETA: eta, State: qbitState(t.State), Detail: t.State}
}

// qbitState maps qBittorrent's state words onto ours.
func qbitState(s string) string {
	switch s {
	case "error", "missingFiles", "unknown":
		return StateFailed
	case "uploading", "stalledUP", "pausedUP", "stoppedUP", "queuedUP", "checkingUP", "forcedUP":
		return StateFinished
	case "pausedDL", "stoppedDL":
		return StatePaused
	case "queuedDL", "checkingDL", "allocating", "checkingResumeData", "moving":
		return StateQueued
	}
	return StateDownloading
}

func (q *Qbittorrent) info(ctx context.Context, f url.Values) ([]Job, error) {
	b, err := q.call(ctx, http.MethodGet, "torrents/info", f)
	if err != nil {
		return nil, err
	}
	var ts []qbitTorrent
	if err := json.Unmarshal(b, &ts); err != nil {
		return nil, fmt.Errorf("qbittorrent torrents/info: %w", err)
	}
	out := make([]Job, len(ts))
	for i, t := range ts {
		out[i] = t.job()
	}
	return out, nil
}

// List returns the torrents in a category.
func (q *Qbittorrent) List(ctx context.Context, category string) ([]Job, error) {
	f := url.Values{}
	if category != "" {
		f.Set("category", category)
	}
	return q.info(ctx, f)
}

// Get returns one torrent with its files.
func (q *Qbittorrent) Get(ctx context.Context, id string) (Job, error) {
	id = strings.ToLower(id)
	if !source.ValidInfoHash(id) {
		return Job{}, errBadHash
	}
	js, err := q.info(ctx, url.Values{"hashes": {id}})
	if err != nil {
		return Job{}, err
	}
	if len(js) != 1 || js[0].ID != id {
		return Job{}, ErrNotFound
	}
	j := js[0]
	b, err := q.call(ctx, http.MethodGet, "torrents/files", url.Values{"hash": {j.ID}})
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return j, nil // metadata not there yet
		}
		return Job{}, err
	}
	if err := json.Unmarshal(b, &j.Files); err != nil {
		return Job{}, fmt.Errorf("qbittorrent torrents/files: %w", err)
	}
	return j, nil
}

// Remove deletes a torrent, with or without its files.
func (q *Qbittorrent) Remove(ctx context.Context, id string, deleteFiles bool) error {
	id = strings.ToLower(id)
	if !source.ValidInfoHash(id) {
		return errBadHash
	}
	_, err := q.call(ctx, http.MethodPost, "torrents/delete", url.Values{"hashes": {id}, "deleteFiles": {fmt.Sprint(deleteFiles)}})
	return err
}
