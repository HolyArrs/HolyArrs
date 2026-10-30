package downloader

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs/internal/source"
)

// Qbittorrent talks to the qBittorrent Web API v2. It signs in one of two
// ways: with the client's API key as a bearer header (5.2 or newer), or with
// a user name and password through the sign-in action, keeping the session
// cookie and signing in again once when a call answers 403. A wrong
// password counts towards qBittorrent's ban on the caller's address, so
// after a refused sign-in the engine does not try again for a while.
type Qbittorrent struct {
	base       string
	http       *http.Client
	key        string
	user, pass string
	signInMu   sync.Mutex
	refusedAt  time.Time // last refused sign-in; no new try until qbitSignInPause has passed
	refusedErr error
}

// qbitSignInPause is how long a refused sign-in is not tried again.
var qbitSignInPause = 5 * time.Minute

// ErrQbitKeyRefused is a refused API key: wrong, or a qBittorrent older than
// 5.2, which has no API key and ignores the bearer header.
var ErrQbitKeyRefused = fmt.Errorf("%w: qBittorrent turned down the API key (it needs qBittorrent 5.2 or newer)", ErrAuth)

// ErrQbitBanned is qBittorrent's ban on this address after failed sign-ins.
var ErrQbitBanned = fmt.Errorf("%w: qBittorrent has banned this address after failed sign-ins; wait for the ban to end (an hour by default)", ErrAuth)

// NewQbittorrent makes an engine for the Web UI at base (such as
// http://qbittorrent:8090) with no sign-in; only a client that trusts this
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

// NewQbittorrentLogin is NewQbittorrent with a user name and password.
func NewQbittorrentLogin(base, user, pass string, h *http.Client) *Qbittorrent {
	q := NewQbittorrent(base, h)
	if user != "" {
		c := *q.http
		c.Jar, _ = cookiejar.New(nil)
		q.http, q.user, q.pass = &c, user, pass
	}
	return q
}

// signIn asks for a session cookie; the jar keeps it. 204 or 200 "Ok." is
// a session, 401 or 200 "Fails." wrong details, 403 a ban.
func (q *Qbittorrent) signIn(ctx context.Context) error {
	q.signInMu.Lock()
	defer q.signInMu.Unlock()
	if !q.refusedAt.IsZero() && time.Since(q.refusedAt) < qbitSignInPause {
		return q.refusedErr
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, q.base+"/api/v2/auth/login",
		strings.NewReader(url.Values{"username": {q.user}, "password": {q.pass}}.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", q.base)
	resp, err := q.http.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return fmt.Errorf("qbittorrent auth/login: %w", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	text := strings.TrimSpace(string(b))
	switch {
	case resp.StatusCode == http.StatusNoContent, resp.StatusCode == http.StatusOK && text != "Fails.":
		q.refusedAt, q.refusedErr = time.Time{}, nil
		return nil
	case resp.StatusCode == http.StatusForbidden:
		q.refusedAt, q.refusedErr = time.Now(), ErrQbitBanned
	case resp.StatusCode == http.StatusUnauthorized, resp.StatusCode == http.StatusOK:
		q.refusedAt, q.refusedErr = time.Now(), ErrAuth
	default:
		return fmt.Errorf("qbittorrent auth/login: %s", resp.Status)
	}
	return q.refusedErr
}

func (q *Qbittorrent) call(ctx context.Context, method, path string, form url.Values, okCodes ...int) ([]byte, error) {
	b, err := q.do(ctx, method, path, form, okCodes...)
	if errors.Is(err, ErrAuth) && q.user != "" {
		if err := q.signIn(ctx); err != nil {
			return nil, err
		}
		return q.do(ctx, method, path, form, okCodes...)
	}
	if errors.Is(err, ErrAuth) && q.key != "" {
		return nil, ErrQbitKeyRefused
	}
	return b, err
}

func (q *Qbittorrent) do(ctx context.Context, method, path string, form url.Values, okCodes ...int) ([]byte, error) {
	_, b, err := q.doRaw(ctx, method, path, form, "", nil, okCodes...)
	return b, err
}

// callRaw is call for a body that is not a plain form (a multipart upload);
// it also hands back the status, since some answers differ only by it.
func (q *Qbittorrent) callRaw(ctx context.Context, path, ctype string, payload []byte, okCodes ...int) (int, []byte, error) {
	code, b, err := q.doRaw(ctx, http.MethodPost, path, nil, ctype, payload, okCodes...)
	if errors.Is(err, ErrAuth) && q.user != "" {
		if err := q.signIn(ctx); err != nil {
			return 0, nil, err
		}
		return q.doRaw(ctx, http.MethodPost, path, nil, ctype, payload, okCodes...)
	}
	if errors.Is(err, ErrAuth) && q.key != "" {
		return 0, nil, ErrQbitKeyRefused
	}
	return code, b, err
}

func (q *Qbittorrent) doRaw(ctx context.Context, method, path string, form url.Values, ctype string, payload []byte, okCodes ...int) (int, []byte, error) {
	var body io.Reader
	u := q.base + "/api/v2/" + path
	if payload != nil {
		body = strings.NewReader(string(payload))
	} else if method == http.MethodGet && len(form) > 0 {
		u += "?" + form.Encode()
	} else if form != nil {
		body = strings.NewReader(form.Encode())
		ctype = "application/x-www-form-urlencoded"
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return 0, nil, err
	}
	if body != nil && ctype != "" {
		req.Header.Set("Content-Type", ctype)
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
		return 0, nil, fmt.Errorf("qbittorrent %s: %w", path, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	code := resp.StatusCode
	switch {
	case code >= 200 && code < 300: // 5.x answers 202 to add
		return code, b, nil
	case code == http.StatusForbidden || code == http.StatusUnauthorized:
		return code, nil, ErrAuth
	}
	for _, c := range okCodes {
		if code == c {
			return code, b, nil
		}
	}
	if code == http.StatusNotFound {
		return code, nil, ErrNotFound
	}
	return code, nil, fmt.Errorf("qbittorrent %s: %s", path, resp.Status)
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
	AddedOn     int64   `json:"added_on"`
	Ratio       float64 `json:"ratio"`
}

func (t qbitTorrent) job() Job {
	eta := t.ETA
	if eta >= 8640000 || eta < 0 { // qBittorrent's "infinite"
		eta = 0
	}
	return Job{ID: strings.ToLower(t.Hash), Name: t.Name, Category: t.Category, SavePath: t.SavePath, ContentPath: t.ContentPath,
		Size: t.Size, Progress: t.Progress, Speed: t.DLSpeed, UpSpeed: max(t.UPSpeed, 0), Seeds: max(t.NumSeeds, 0), Peers: max(t.NumLeechs, 0), ETA: eta, State: qbitState(t.State), Detail: t.State,
		AddedAt: max(t.AddedOn, 0), Ratio: max(t.Ratio, 0)}
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

// ListAll returns every torrent in the client, in one torrents/info call.
func (q *Qbittorrent) ListAll(ctx context.Context) ([]Job, error) {
	return q.info(ctx, url.Values{})
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
