package downloads

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// qbitSignInPause is how long a refused sign-in is remembered. Retrying sooner only adds failed attempts, and qBittorrent bans an address after a few.
var qbitSignInPause = 5 * time.Minute

// ErrQbitKeyRefused means the client rejected the API key. Only 5.2 and newer know keys.
var ErrQbitKeyRefused = fmt.Errorf("%w: qBittorrent turned down the API key (it needs qBittorrent 5.2 or newer)", ErrAuth)

// ErrQbitBanned means the client banned this address for too many failed sign-ins.
var ErrQbitBanned = fmt.Errorf("%w: qBittorrent has banned this address after failed sign-ins; wait for the ban to end (an hour by default)", ErrAuth)

// errBadHash stops an id that is not an info hash from reaching the client.
var errBadHash = errors.New("not a torrent info hash")

// Qbittorrent is the engine for qBittorrent's web API. It signs in with an API key, with a user and password, or with neither when the client trusts our address.
// refusedAt and refusedErr hold the last refused sign-in; ver and verAt cache the API version.
type Qbittorrent struct {
	base       string
	http       *http.Client
	key        string
	user, pass string
	signInMu   sync.Mutex
	refusedAt  time.Time
	refusedErr error
	verMu      sync.Mutex
	ver        [3]int
	verAt      time.Time
	// cats are the Downloader's own categories; Pause all and Resume all
	// touch only torrents in them.
	cats []string
}

// qbitTorrent is one row of torrents/info. Pointer fields are the ones older clients leave out.
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
	Avail       *float64 `json:"availability"`
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

	// DownloadPath is the temporary folder for unfinished data, when the client uses one.
	DownloadPath string `json:"download_path"`
}

// NewQbittorrent returns an engine with no credentials. The client gets its own cookie jar, and a caller's client is copied so its settings stay untouched.
func NewQbittorrent(base string, h *http.Client) *Qbittorrent {
	if h == nil {
		h = &http.Client{Timeout: clientTimeout}
	}
	if h.Jar == nil {
		c := *h
		c.Jar, _ = cookiejar.New(nil)
		h = &c
	}
	return &Qbittorrent{base: strings.TrimRight(base, "/"), http: h}
}

// NewQbittorrentKey returns an engine that signs in with an API key.
func NewQbittorrentKey(base, key string, h *http.Client) *Qbittorrent {
	q := NewQbittorrent(base, h)
	q.key = strings.TrimSpace(key)
	return q
}

// NewQbittorrentLogin returns an engine that signs in with a user and password. An empty user means no sign-in.
func NewQbittorrentLogin(base, user, pass string, h *http.Client) *Qbittorrent {
	q := NewQbittorrent(base, h)
	if user != "" {
		q.user, q.pass = user, pass
	}
	return q
}

// Health reads the client version. A key with a client older than 5.2 passes only because the client trusts the address, so a note says so.
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

// Add sends a torrent file or link and returns its info hash when known, else empty. The category is created first; a 409 means it exists.
// The reply is checked for the plain text failure word since older clients answer 200 for it.
func (q *Qbittorrent) Add(ctx context.Context, r AddRequest) (string, error) {
	if r.Category != "" {
		if _, err := q.call(ctx, http.MethodPost, "torrents/createCategory", url.Values{"category": {r.Category}}, http.StatusConflict); err != nil {
			return "", err
		}
	}
	var b []byte
	var err error
	if len(r.File) > 0 {
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		if r.Category != "" {
			_ = mw.WriteField("category", r.Category)
		}
		if r.SavePath != "" {
			_ = mw.WriteField("savepath", r.SavePath)
		}
		if r.Stopped {
			// stopped since WebUI API 2.11, paused before it.
			_ = mw.WriteField("stopped", "true")
			_ = mw.WriteField("paused", "true")
		}
		w, werr := mw.CreateFormFile("torrents", "release.torrent")
		if werr != nil {
			return "", werr
		}
		_, _ = w.Write(r.File)
		_ = mw.Close()
		_, b, err = q.callRaw(ctx, "torrents/add", mw.FormDataContentType(), body.Bytes())
	} else {
		f := url.Values{"urls": {r.URL}}
		if r.Category != "" {
			f.Set("category", r.Category)
		}
		if r.SavePath != "" {
			f.Set("savepath", r.SavePath)
		}
		if r.Stopped {
			f.Set("stopped", "true")
			f.Set("paused", "true")
		}
		b, err = q.call(ctx, http.MethodPost, "torrents/add", f)
	}
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(string(b)) == "Fails." {
		return "", errors.New("qbittorrent refused the torrent")
	}
	if h := strings.ToLower(r.InfoHash); releasename.ValidInfoHash(h) {
		return h, nil
	}
	return "", nil
}

// HostOnly returns just the lowercase host of an address, so tracker passkeys in the path or query never reach the screen.
func HostOnly(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

// List returns the torrents in a category, or all when category is empty.
func (q *Qbittorrent) List(ctx context.Context, category string) ([]Job, error) {
	f := url.Values{}
	if category != "" {
		f.Set("category", category)
	}
	return q.info(ctx, f)
}

// ListAll returns every torrent in the client.
func (q *Qbittorrent) ListAll(ctx context.Context) ([]Job, error) {
	return q.info(ctx, url.Values{})
}

// Get returns one torrent with its files. A client that has no file list for it still returns the job.
func (q *Qbittorrent) Get(ctx context.Context, id string) (Job, error) {
	id = strings.ToLower(id)
	if !releasename.ValidInfoHash(id) {
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
			return j, nil
		}
		return Job{}, err
	}
	if err := json.Unmarshal(b, &j.Files); err != nil {
		return Job{}, fmt.Errorf("qbittorrent torrents/files: %w", err)
	}
	return j, nil
}

// Remove deletes a torrent, and its data when deleteFiles is set.
func (q *Qbittorrent) Remove(ctx context.Context, id string, deleteFiles bool) error {
	id = strings.ToLower(id)
	if !releasename.ValidInfoHash(id) {
		return errBadHash
	}
	_, err := q.call(ctx, http.MethodPost, "torrents/delete", url.Values{"hashes": {id}, "deleteFiles": {fmt.Sprint(deleteFiles)}})
	return err
}

// signIn logs in with the user and password. A refused sign-in is remembered for a while and returned at once, to protect the address from a ban.
// A 403 means banned; a 200 with the word Fails. or a 401 means wrong credentials.
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

// call runs an API request and signs in again once if the session ended. A key that is refused is final.
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

// do runs a request and returns only the body.
func (q *Qbittorrent) do(ctx context.Context, method, path string, form url.Values, okCodes ...int) ([]byte, error) {
	_, b, err := q.doRaw(ctx, method, path, form, "", nil, okCodes...)
	return b, err
}

// callRaw is call for a ready-made body such as a file upload.
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

// doRaw sends one request. okCodes are statuses the caller wants to read as answers. The Referer header is what qBittorrent's CSRF check wants when no key is used.
// Answers are capped so a broken or hostile server cannot fill memory. Torrent lists can be large, so they get a bigger cap.
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
	// 8 MiB is plenty for small answers. The two list calls can reach 64 MiB on a big client.
	limit := int64(8 << 20)
	if path == "sync/maindata" || path == "torrents/info" {
		limit = 64 << 20
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if int64(len(b)) > limit {
		return 0, nil, fmt.Errorf("qbittorrent %s: %w", path, ErrTooLarge)
	}
	code := resp.StatusCode
	switch {
	case code >= 200 && code < 300:
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

// qbitBefore52 reports whether a version string is older than 5.2. An unreadable version counts as new.
func qbitBefore52(v string) bool {
	var major, minor int
	if _, err := fmt.Sscanf(strings.TrimPrefix(strings.TrimSpace(v), "v"), "%d.%d", &major, &minor); err != nil {
		return false
	}
	return major < 5 || (major == 5 && minor < 2)
}

// job converts the row to a Job.
func (t qbitTorrent) job() Job { return jobFromTorrent(t) }

// jobFromTorrent converts a row to a Job. Negative numbers some clients send are clamped to 0. An ETA of 100 days or more is the client's "infinite" and becomes unknown.
func jobFromTorrent(t qbitTorrent) Job {
	eta := t.ETA
	if eta >= 8640000 || eta < 0 {
		eta = 0
	}
	state, save := qbitState(t.State), t.SavePath
	if state == StateFinished && t.stillIncomplete() {
		save = t.DownloadPath
	}
	return Job{ID: strings.ToLower(t.Hash), Name: t.Name, Category: t.Category, SavePath: save, ContentPath: t.ContentPath,
		Size: t.Size, Progress: t.Progress, Speed: t.DLSpeed, UpSpeed: max(t.UPSpeed, 0), Seeds: max(t.NumSeeds, 0), Peers: max(t.NumLeechs, 0), SeedsTotal: max(t.NumComplete, 0), PeersTotal: max(t.NumIncompl, 0), ETA: eta, State: state, Detail: t.State,
		AddedAt: max(t.AddedOn, 0), Ratio: max(t.Ratio, 0), Limits: t.limits(),
		DoneAt: max(t.DoneOn, 0), QueuePos: max(t.QueuePos, 0), Tracker: HostOnly(t.Tracker), TrackerProblem: t.problem(),
		LimitDown: max(t.DLLimit, 0), LimitUp: max(t.UPLimit, 0),
		LastActivity: max(t.LastAct, 0), SeedingTime: max(t.SeedTime, 0), SeenComplete: max(t.SeenDone, 0), Uploaded: max(t.Uploaded, 0), TimeActive: max(t.TimeActive, 0), Availability: t.avail()}
}

// stillIncomplete spots a finished torrent whose data still sits in the temporary folder, because the client has not moved it yet. Then the save path is the temporary one.
func (t qbitTorrent) stillIncomplete() bool {
	if t.DownloadPath == "" || t.ContentPath == "" || t.Progress < 1 {
		return false
	}
	clean := func(p string) string { return path.Clean(strings.ReplaceAll(p, `\`, "/")) }
	in := func(p, dir string) bool {
		p, dir = clean(p), clean(dir)
		return dir != "." && (p == dir || strings.HasPrefix(p, strings.TrimSuffix(dir, "/")+"/"))
	}
	if !in(t.ContentPath, t.DownloadPath) {
		return false
	}
	return !in(t.ContentPath, t.SavePath) || (in(t.DownloadPath, t.SavePath) && clean(t.DownloadPath) != clean(t.SavePath))
}

// avail returns the availability, nil when the client reports a negative value for none.
func (t qbitTorrent) avail() *float64 {
	if t.Avail == nil || *t.Avail < 0 {
		return nil
	}
	a := *t.Avail
	return &a
}

// problem names the tracker trouble, if any. An active torrent that has trackers but none answering counts as an announce problem.
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

// limits returns the torrent's own seeding rules, nil when the client reports none. -2 is the client's mark for "use the default".
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

// qbitState maps qBittorrent's state words to ours. Errors and missing files count as paused so the user sees them, not as a failure that triggers a replacement. Any unknown word counts as downloading.
func qbitState(s string) string {
	switch s {
	case "error", "missingFiles", "unknown":
		return StatePaused
	case "uploading", "stalledUP", "pausedUP", "stoppedUP", "queuedUP", "checkingUP", "forcedUP":
		return StateFinished
	case "pausedDL", "stoppedDL":
		return StatePaused
	case "queuedDL", "checkingDL", "allocating", "checkingResumeData", "moving":
		return StateQueued
	}
	return StateDownloading
}

// info reads torrents/info with a filter.
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
