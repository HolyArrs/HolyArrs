package downloads

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

var errTransmissionForeign = errors.New("the torrent belongs to another category")

var transmissionNames = map[string]string{
	"rpc-version": "rpc_version", "hashString": "hash_string", "downloadDir": "download_dir", "download-dir": "download_dir",
	"totalSize": "total_size", "sizeWhenDone": "size_when_done", "percentDone": "percent_done", "leftUntilDone": "left_until_done",
	"metadataPercentComplete": "metadata_percent_complete", "rateDownload": "rate_download", "rateUpload": "rate_upload", "peersSendingToUs": "peers_sending_to_us", "peersGettingFromUs": "peers_getting_from_us", "bytesCompleted": "bytes_completed",
	"isFinished": "is_finished", "errorString": "error_string", "recheckProgress": "recheck_progress",
	"torrent-added": "torrent_added", "torrent-duplicate": "torrent_duplicate", "delete-local-data": "delete_local_data",
}

// Transmission is a Client for the Transmission RPC API.
type Transmission struct {
	base, user, pass string
	http             *http.Client
	mu               sync.Mutex
	session          string
	modern           bool
	category         string
	others           []string // more categories that are also ours
}

type transmissionRefusedError struct{ error }

type transmissionTorrent struct {
	Hash        string   `json:"hash_string"`
	Name        string   `json:"name"`
	Labels      []string `json:"labels"`
	Dir         string   `json:"download_dir"`
	Size        int64    `json:"total_size"`
	Progress    float64  `json:"percent_done"`
	Left        int64    `json:"left_until_done"`
	Metadata    float64  `json:"metadata_percent_complete"`
	Recheck     float64  `json:"recheck_progress"`
	Rate        int64    `json:"rate_download"`
	UpRate      int64    `json:"rate_upload"`
	SeedsNow    int      `json:"peers_sending_to_us"`
	PeersNow    int      `json:"peers_getting_from_us"`
	ETA         int64    `json:"eta"`
	Status      int      `json:"status"`
	Error       int      `json:"error"`
	ErrorString string   `json:"error_string"`
	Files       []struct {
		Name   string `json:"name"`
		Length int64  `json:"length"`
		Done   int64  `json:"bytes_completed"`
	} `json:"files"`
}

// NewTransmission returns a client for the Transmission at base, using the given login and h.
func NewTransmission(base, user, pass string, h *http.Client) *Transmission {
	if h == nil {
		h = &http.Client{Timeout: clientTimeout}
	}
	base = strings.TrimRight(base, "/")
	u, err := url.Parse(base)
	if err == nil && (u.Path == "" || u.Path == "/") {
		base += "/transmission/rpc"
	} else if !strings.HasSuffix(base, "/rpc") {
		base = strings.TrimSuffix(base, "/web") + "/rpc"
	}
	return &Transmission{base: base, user: user, pass: pass, http: h}
}

// Health reports whether Transmission answers and accepts the login.
func (tr *Transmission) Health(ctx context.Context) (Health, error) {
	var v struct {
		Version string `json:"version"`
	}
	if err := tr.call(ctx, "session-get", nil, &v); err != nil {
		return Health{}, err
	}
	major, _, _ := strings.Cut(v.Version, ".")
	n, _ := strconv.Atoi(major)
	if n < 4 {
		return Health{}, errors.New("Transmission 4 or newer is needed for category labels")
	}
	return Health{Version: v.Version, At: time.Now().UTC()}, nil
}

// List returns the torrents in category.
func (tr *Transmission) List(ctx context.Context, category string) ([]Job, error) {
	ts, err := tr.torrents(ctx, "", false)
	if err != nil {
		return nil, err
	}
	out := []Job{}
	for _, t := range ts {
		if slices.Contains(t.Labels, category) {
			out = append(out, t.job(category))
		}
	}
	return out, nil
}

// Get returns the torrent with the given id.
func (tr *Transmission) Get(ctx context.Context, id string) (Job, error) {
	j, err := tr.get(ctx, id, tr.category)
	for _, c := range tr.others {
		if !errors.Is(err, errTransmissionForeign) {
			break
		}
		j, err = tr.get(ctx, id, c)
	}
	return j, err
}

// Add queues a torrent and returns its hash.
func (tr *Transmission) Add(ctx context.Context, r AddRequest) (string, error) {
	if r.Category == "" {
		return "", ErrCategory
	}
	id := strings.ToLower(r.InfoHash)
	if id != "" && !releasename.ValidInfoHash(id) {
		return "", errBadHash
	}
	if r.ReconcileOnly {
		if id == "" || r.Skip[id] {
			return "", ErrUncertain
		}
		job, err := tr.get(ctx, id, r.Category)
		if err != nil || job.Category != r.Category || r.Skip[job.ID] {
			return "", errors.Join(ErrUncertain, err)
		}
		return job.ID, nil
	}
	if r.Skip[id] {
		return "", errors.New("the torrent belongs to another download")
	}
	if id != "" {
		j, err := tr.get(ctx, id, r.Category)
		if err == nil {
			if j.Category != r.Category {
				return "", errors.New("the torrent already belongs to another category")
			}
			return id, nil
		}
		if !errors.Is(err, ErrNotFound) {
			return "", err
		}
	}
	a := map[string]any{"labels": []string{r.Category}, "paused": false}
	if r.SavePath != "" {
		a["download-dir"] = r.SavePath
	}
	u, err := url.Parse(r.URL)
	if err != nil {
		return "", errors.New("invalid torrent address")
	}
	if u.Scheme == "magnet" {
		if id == "" || !strings.Contains(strings.ToLower(u.Query().Get("xt")), id) {
			return "", errors.New("the magnet needs its matching info hash")
		}
		a["filename"] = r.URL
	} else {
		b, err := r.File, error(nil)
		if len(b) == 0 {
			b, err = fetchRelease(ctx, r.URL, r.AllowLocal, torrentFetchMax)
		}
		if err != nil {
			return "", err
		}
		if !r.AllowLocal {
			if b, err = safeTorrent(b); err != nil {
				return "", err
			}
		}
		h, err := torrentFileHash(b)
		if err != nil {
			return "", err
		}
		if id != "" && id != h {
			return "", errors.New("the torrent file does not match its info hash")
		}
		id = h
		if r.Skip[id] {
			return "", errors.New("the torrent belongs to another download")
		}
		a["metainfo"] = base64.StdEncoding.EncodeToString(b)
	}
	var out struct {
		Added *struct {
			Hash string `json:"hash_string"`
		} `json:"torrent_added"`
		Duplicate *struct {
			Hash string `json:"hash_string"`
		} `json:"torrent_duplicate"`
	}
	if err := tr.call(ctx, "torrent-add", a, &out); err != nil {
		if transmissionDefinitelyRefused(err) {
			return "", err
		}
		var getErr error
		if ctx.Err() == nil && id != "" {
			if j, e := tr.get(ctx, id, r.Category); e == nil && !r.Skip[j.ID] {
				return j.ID, nil
			} else {
				getErr = e
			}
		}
		return "", errors.Join(ErrUncertain, err, getErr)
	}
	if (out.Added == nil) == (out.Duplicate == nil) {
		return "", errors.Join(ErrUncertain, errors.New("transmission gave no unambiguous torrent identity"))
	}
	got := ""
	duplicate := false
	if out.Added != nil {
		got = out.Added.Hash
	} else {
		got = out.Duplicate.Hash
		duplicate = true
	}
	if !releasename.ValidInfoHash(got) {
		return "", errors.Join(ErrUncertain, errors.New("transmission returned an invalid torrent identity"))
	}
	if duplicate {
		if _, err := tr.get(ctx, got, r.Category); err != nil {
			if errors.Is(err, errTransmissionForeign) {
				return "", err
			}
			return "", errors.Join(ErrUncertain, err)
		}
	}
	if id != "" && !strings.EqualFold(got, id) {
		return "", errors.Join(ErrUncertain, errors.New("transmission returned another torrent identity"))
	}
	got = strings.ToLower(got)
	if r.Skip[got] {
		return "", errors.Join(ErrUncertain, errors.New("the torrent belongs to another download"))
	}
	return got, nil
}

// Remove deletes the torrent, and its data when deleteFiles is set.
func (tr *Transmission) Remove(ctx context.Context, id string, deleteFiles bool) error {
	id = strings.ToLower(id)
	if !releasename.ValidInfoHash(id) {
		return errBadHash
	}
	if tr.category != "" {
		if _, err := tr.Get(ctx, id); err != nil {
			return err
		}
	}
	return tr.call(ctx, "torrent-remove", map[string]any{"ids": []string{id}, "delete-local-data": deleteFiles}, nil)
}

func transmissionRefused(message string) error {
	return transmissionRefusedError{errors.New(message)}
}

func transmissionDefinitelyRefused(err error) bool {
	var refused transmissionRefusedError
	return errors.Is(err, ErrAuth) || errors.As(err, &refused)
}

func transmissionModern(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			if n := transmissionNames[k]; n != "" {
				k = n
			}
			out[k] = transmissionModern(val)
		}
		return out
	case []any:
		for i, val := range x {
			x[i] = transmissionModern(val)
		}
		return x
	case []string:
		out := slices.Clone(x)
		for i, k := range out {
			if n := transmissionNames[k]; n != "" {
				out[i] = n
			}
		}
		return out
	}
	return v
}

func (tr *Transmission) call(ctx context.Context, method string, args map[string]any, out any) error {
	if args == nil {
		args = map[string]any{}
	}
	for attempt := 0; attempt < 2; attempt++ {
		tr.mu.Lock()
		token, modern := tr.session, tr.modern
		tr.mu.Unlock()
		body := map[string]any{"method": method, "arguments": args, "tag": 1}
		if modern {
			body = map[string]any{"jsonrpc": "2.0", "method": strings.ReplaceAll(method, "-", "_"), "params": transmissionModern(args), "id": 1}
		}
		b, _ := json.Marshal(body)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, tr.base, bytes.NewReader(b))
		if err != nil {
			return errors.New("transmission: invalid address")
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Transmission-Session-Id", token)
		if tr.user != "" {
			req.SetBasicAuth(tr.user, tr.pass)
		}
		resp, err := tr.http.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if errors.Is(err, outbound.ErrAddressRefused) {
				return outbound.ErrAddressRefused
			}
			return errDownloaderUnavailable
		}
		b, readErr := io.ReadAll(io.LimitReader(resp.Body, (8<<20)+1))
		resp.Body.Close()
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return ErrAuth
		}
		if resp.StatusCode == http.StatusConflict {
			fresh := resp.Header.Get("X-Transmission-Session-Id")
			if fresh == "" || attempt > 0 {
				return transmissionRefused("transmission could not refresh its session")
			}
			tr.mu.Lock()
			if tr.session == token {
				tr.session = fresh
				if v := resp.Header.Get("X-Transmission-Rpc-Version"); v != "" {
					major, _, _ := strings.Cut(v, ".")
					n, _ := strconv.Atoi(major)
					tr.modern = n >= 6
				}
			}
			tr.mu.Unlock()
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			return adapterHTTPError{resp.StatusCode}
		}
		if readErr != nil || len(b) > 8<<20 {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return errors.New("transmission sent an incomplete or oversized answer")
		}
		var env struct {
			JSONRPC   json.RawMessage `json:"jsonrpc"`
			ID        json.RawMessage `json:"id"`
			Tag       json.RawMessage `json:"tag"`
			Result    json.RawMessage `json:"result"`
			Arguments json.RawMessage `json:"arguments"`
			Error     json.RawMessage `json:"error"`
		}
		if json.Unmarshal(b, &env) != nil {
			return errors.New("the address answered, but not like Transmission")
		}
		result := env.Result
		if modern {
			var version string
			var id int
			if json.Unmarshal(env.JSONRPC, &version) != nil || version != "2.0" || json.Unmarshal(env.ID, &id) != nil || id != 1 || len(env.Tag) != 0 || len(env.Arguments) != 0 {
				return errors.New("transmission sent an invalid JSON-RPC envelope")
			}
			hasResult := len(env.Result) > 0 && !bytes.Equal(bytes.TrimSpace(env.Result), []byte("null"))
			hasError := len(env.Error) > 0 && !bytes.Equal(bytes.TrimSpace(env.Error), []byte("null"))
			if hasResult == hasError {
				return errors.New("transmission sent an invalid JSON-RPC outcome")
			}
			if hasError {
				var rpcErr struct {
					Code    json.RawMessage `json:"code"`
					Message *string         `json:"message"`
				}
				var code int
				if json.Unmarshal(env.Error, &rpcErr) != nil || json.Unmarshal(rpcErr.Code, &code) != nil || rpcErr.Message == nil {
					return errors.New("transmission sent an invalid JSON-RPC error")
				}
				return transmissionRefused("transmission refused the call")
			}
			var object map[string]json.RawMessage
			if json.Unmarshal(env.Result, &object) != nil || object == nil {
				return errors.New("transmission sent an invalid JSON-RPC result")
			}
		} else {
			var tag int
			if len(env.JSONRPC) != 0 || len(env.ID) != 0 || len(env.Error) != 0 || json.Unmarshal(env.Tag, &tag) != nil || tag != 1 {
				return errors.New("transmission sent an invalid legacy envelope")
			}
			var word string
			if json.Unmarshal(env.Result, &word) != nil || word != "success" {
				return transmissionRefused("transmission refused the call")
			}
			if out != nil {
				if len(env.Arguments) == 0 || bytes.Equal(bytes.TrimSpace(env.Arguments), []byte("null")) {
					return errors.New("transmission sent an invalid answer")
				}
				var legacy any
				if json.Unmarshal(env.Arguments, &legacy) != nil {
					return errors.New("transmission sent an invalid answer")
				}
				result, _ = json.Marshal(transmissionModern(legacy))
			}
		}
		if out != nil && (len(result) == 0 || json.Unmarshal(result, out) != nil) {
			return errors.New("transmission sent an invalid answer")
		}
		return nil
	}
	return errors.New("transmission could not refresh its session")
}

func (tr *Transmission) torrents(ctx context.Context, id string, files bool) ([]transmissionTorrent, error) {
	fields := []string{"hashString", "name", "labels", "downloadDir", "totalSize", "percentDone", "leftUntilDone", "metadataPercentComplete", "recheckProgress", "rateDownload", "rateUpload", "peersSendingToUs", "peersGettingFromUs", "eta", "status", "error", "errorString"}
	if files {
		fields = append(fields, "files")
	}
	a := map[string]any{"fields": fields}
	if id != "" {
		a["ids"] = []string{id}
	}
	var out struct {
		Torrents []transmissionTorrent `json:"torrents"`
	}
	err := tr.call(ctx, "torrent-get", a, &out)
	return out.Torrents, err
}

func (t transmissionTorrent) job(category string) Job {
	state := StateDownloading
	switch t.Status {
	case 0:
		state = StatePaused
	case 1, 2, 3:
		state = StateQueued
	}
	if t.Error == 3 {
		state = StatePaused
	} else if (t.Status == 0 || t.Status == 5 || t.Status == 6) && t.Progress >= 1 && t.Left == 0 && t.Metadata >= 1 && !(t.Recheck > 0 && t.Recheck < 1) && t.Dir != "" {
		state = StateFinished
	}
	progress := max(0, min(1, t.Progress))
	eta := max(0, t.ETA)
	j := Job{ID: strings.ToLower(t.Hash), Name: t.Name, Category: category, SavePath: t.Dir, Size: max(0, t.Size), Progress: progress, Speed: max(0, t.Rate), UpSpeed: max(0, t.UpRate), Seeds: max(0, t.SeedsNow), Peers: max(0, t.PeersNow), ETA: eta, State: state, Detail: strconv.Itoa(t.Status)}
	if t.Error != 0 {
		j.Detail = fmt.Sprintf("%d; error %d", t.Status, t.Error)
		if detail := strings.Join(strings.Fields(t.ErrorString), " "); detail != "" {
			runes := []rune(detail)
			if len(runes) > 128 {
				detail = string(runes[:128])
			}
			j.Detail += ": " + detail
		}
	}
	if t.Recheck > 0 && t.Recheck < 1 {
		j.Detail += fmt.Sprintf("; checking %.0f%%", t.Recheck*100)
	}
	for _, f := range t.Files {
		p := float64(0)
		if f.Length > 0 {
			p = max(0, min(1, float64(f.Done)/float64(f.Length)))
		}
		j.Files = append(j.Files, File{Name: f.Name, Size: max(0, f.Length), Progress: p})
	}
	return j
}

func (tr *Transmission) get(ctx context.Context, id, category string) (Job, error) {
	id = strings.ToLower(id)
	if !releasename.ValidInfoHash(id) {
		return Job{}, errBadHash
	}
	ts, err := tr.torrents(ctx, id, true)
	if err != nil {
		return Job{}, err
	}
	for _, t := range ts {
		if strings.EqualFold(t.Hash, id) {
			cat := category
			if cat != "" && !slices.Contains(t.Labels, cat) {
				return Job{}, errTransmissionForeign
			}
			if cat == "" && len(t.Labels) > 0 {
				cat = t.Labels[0]
			}
			return t.job(cat), nil
		}
	}
	return Job{}, ErrNotFound
}
