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
	verMu      sync.Mutex
	ver        [3]int    // API version, read once and kept for qbitVersionTTL
	verAt      time.Time // when ver was read; zero when not kept
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
	if h.Jar == nil {
		// Keep qBittorrent's session cookie so it reuses one session and
		// the change feed's deltas work.
		c := *h
		c.Jar, _ = cookiejar.New(nil)
		h = &c
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
		q.user, q.pass = user, pass
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
	h := Health{Version: strings.TrimSpace(string(b)), At: time.Now().UTC()}
	if q.key != "" && qbitBefore52(h.Version) {
		h.Note = "This qBittorrent is older than 5.2, so it ignores API keys; it answers only because it trusts this address."
	}
	return h, nil
}

// qbitBefore52 reads a version such as "v5.1.2"; an unreadable one is not older.
func qbitBefore52(v string) bool {
	var major, minor int
	if _, err := fmt.Sscanf(strings.TrimPrefix(strings.TrimSpace(v), "v"), "%d.%d", &major, &minor); err != nil {
		return false
	}
	return major < 5 || (major == 5 && minor < 2)
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
	Hash        string   `json:"hash"`
	Name        string   `json:"name"`
	Category    string   `json:"category"`
	SavePath    string   `json:"save_path"`
	ContentPath string   `json:"content_path"`
	Size        int64    `json:"size"`
	Progress    float64  `json:"progress"`
	DLSpeed     int64    `json:"dlspeed"`
	UPSpeed     int64    `json:"upspeed"`
	NumSeeds    int      `json:"num_seeds"`
	NumLeechs   int      `json:"num_leechs"`
	NumComplete int      `json:"num_complete"`
	NumIncompl  int      `json:"num_incomplete"`
	ETA         int64    `json:"eta"`
	State       string   `json:"state"`
	AddedOn     int64    `json:"added_on"`
	Ratio       float64  `json:"ratio"`
	RatioLimit  *float64 `json:"ratio_limit"`
	SeedLimit   *int64   `json:"seeding_time_limit"`
	IdleLimit   *int64   `json:"inactive_seeding_time_limit"`
	ShareAction string   `json:"share_limit_action"`
	DoneOn      int64    `json:"completion_on"`
	QueuePos    int      `json:"priority"`
	Tracker     string   `json:"tracker"`
	Trackers    int      `json:"trackers_count"`
	TrackerErr  bool     `json:"has_tracker_error"`
	TrackerWarn bool     `json:"has_tracker_warning"`
	OtherErr    bool     `json:"has_other_announce_error"`
	DLLimit     int64    `json:"dl_limit"`
	UPLimit     int64    `json:"up_limit"`
	LastAct     int64    `json:"last_activity"`
	SeedTime    int64    `json:"seeding_time"`
	SeenDone    int64    `json:"seen_complete"`
	Uploaded    int64    `json:"uploaded"`
	TimeActive  int64    `json:"time_active"`
}

func (t qbitTorrent) job() Job { return jobFromTorrent(t) }

// jobFromTorrent is the one conversion from a qBittorrent torrent to a Job,
// shared by torrents/info (List, ListAll, Get) and sync/maindata, so both
// paths give the same ID (lower-case hash) and the same fields.
func jobFromTorrent(t qbitTorrent) Job {
	eta := t.ETA
	if eta >= 8640000 || eta < 0 { // qBittorrent's "infinite"
		eta = 0
	}
	return Job{ID: strings.ToLower(t.Hash), Name: t.Name, Category: t.Category, SavePath: t.SavePath, ContentPath: t.ContentPath,
		Size: t.Size, Progress: t.Progress, Speed: t.DLSpeed, UpSpeed: max(t.UPSpeed, 0), Seeds: max(t.NumSeeds, 0), Peers: max(t.NumLeechs, 0), SeedsTotal: max(t.NumComplete, 0), PeersTotal: max(t.NumIncompl, 0), ETA: eta, State: qbitState(t.State), Detail: t.State,
		AddedAt: max(t.AddedOn, 0), Ratio: max(t.Ratio, 0), Limits: t.limits(),
		DoneAt: max(t.DoneOn, 0), QueuePos: max(t.QueuePos, 0), Tracker: HostOnly(t.Tracker), TrackerProblem: t.problem(),
		LimitDown: max(t.DLLimit, 0), LimitUp: max(t.UPLimit, 0),
		LastActivity: max(t.LastAct, 0), SeedingTime: max(t.SeedTime, 0), SeenComplete: max(t.SeenDone, 0), Uploaded: max(t.Uploaded, 0), TimeActive: max(t.TimeActive, 0)}
}

// problem reads the tracker flags (sent by newer clients) or, without
// them, a running torrent that has trackers but none working.
func (t qbitTorrent) problem() string {
	switch {
	case t.TrackerErr:
		return "error"
	case t.TrackerWarn:
		return "warning"
	case t.OtherErr:
		return "announce"
	}
	switch t.State {
	case "downloading", "stalledDL", "metaDL", "forcedDL", "forcedMetaDL", "uploading", "stalledUP", "forcedUP":
		if t.Trackers > 0 && t.Tracker == "" {
			return "announce"
		}
	}
	return ""
}

// HostOnly keeps a tracker address's host name: the path and query can
// hold a passkey.
func HostOnly(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

// limits reads the torrent's own seeding rules; nil when the client sent none.
func (t qbitTorrent) limits() *ShareLimits {
	if t.RatioLimit == nil && t.SeedLimit == nil && t.IdleLimit == nil {
		return nil
	}
	l := ShareLimits{Ratio: -2, SeedingMinutes: -2, InactiveMinutes: -2}
	if t.RatioLimit != nil {
		l.Ratio = *t.RatioLimit
	}
	if t.SeedLimit != nil {
		l.SeedingMinutes = *t.SeedLimit
	}
	if t.IdleLimit != nil {
		l.InactiveMinutes = *t.IdleLimit
	}
	switch t.ShareAction {
	case "Stop":
		l.Action = "stop"
	case "Remove":
		l.Action = "remove"
	case "RemoveWithContent":
		l.Action = "remove_files"
	case "EnableSuperSeeding":
		l.Action = "super_seed"
	}
	return &l
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
		out[i] = jobFromTorrent(t)
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
