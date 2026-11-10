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
	"net/http/cookiejar"
	"net/url"
	"path"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

var (
	errDelugeDetached = errors.New("the Deluge Web UI is not attached to its daemon")
	errDelugeLabel    = errors.New("the Deluge Label plugin is required")
	errDelugeBadHash  = errors.New("not a torrent info hash")
)

var delugeStatusKeys = []string{
	"name", "label", "save_path", "total_wanted", "progress",
	"download_payload_rate", "upload_payload_rate", "num_seeds", "num_peers", "eta", "state", "is_finished",
}

// Deluge is a Client for the Deluge web JSON-RPC API.
type Deluge struct {
	Category string
	Others   []string // more categories that are also ours

	base string
	pass string
	http *http.Client

	requestID atomic.Int64
	authGate  chan struct{}
	authMu    sync.Mutex
	authGen   uint64
}

type delugeRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type delugeNullResultError struct{}

type delugeRPCResponse struct {
	ID     json.RawMessage `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  json.RawMessage `json:"error"`
}

type delugeTorrent struct {
	Name                string  `json:"name"`
	Label               string  `json:"label"`
	SavePath            string  `json:"save_path"`
	TotalWanted         int64   `json:"total_wanted"`
	Progress            float64 `json:"progress"`
	DownloadPayloadRate int64   `json:"download_payload_rate"`
	UploadPayloadRate   int64   `json:"upload_payload_rate"`
	NumSeeds            int     `json:"num_seeds"`
	NumPeers            int     `json:"num_peers"`
	ETA                 int64   `json:"eta"`
	State               string  `json:"state"`
	Finished            bool    `json:"is_finished"`
}

// NewDeluge returns a client for the Deluge at base, using pass and h.
func NewDeluge(base, pass string, h *http.Client) *Deluge {
	if h == nil {
		h = &http.Client{Timeout: clientTimeout}
	}
	c := *h
	if c.Jar == nil {
		c.Jar, _ = cookiejar.New(nil)
	}
	return &Deluge{base: strings.TrimRight(base, "/"), pass: pass, http: &c, authGate: make(chan struct{}, 1)}
}

// Error implements error.
func (delugeNullResultError) Error() string { return "Deluge returned no add result" }

// Error implements error.
func (e *delugeRPCError) Error() string {
	return fmt.Sprintf("Deluge RPC error %d", e.Code)
}

// Is matches errors by target kind.
func (e *delugeRPCError) Is(target error) bool {
	return target == ErrAuth && e.Code == 1
}

// Health reports whether Deluge answers and accepts the password.
func (d *Deluge) Health(ctx context.Context) (Health, error) {
	if _, err := d.ready(ctx); err != nil {
		return Health{}, err
	}
	var version string
	if err := d.read(ctx, "daemon.get_version", []any{}, &version); err != nil {
		return Health{}, err
	}
	return Health{Version: version, At: time.Now().UTC()}, nil
}

// Add queues a torrent and returns its hash.
func (d *Deluge) Add(ctx context.Context, r AddRequest) (string, error) {
	if r.Category == "" {
		return "", ErrCategory
	}
	expected := strings.ToLower(r.InfoHash)
	if expected != "" && !releasename.ValidInfoHash(expected) {
		return "", errDelugeBadHash
	}
	if r.ReconcileOnly {
		if expected == "" || r.Skip[expected] {
			return "", ErrUncertain
		}
		existing, err := d.Get(ctx, expected)
		if err != nil || existing.Category != r.Category || r.Skip[existing.ID] {
			return "", errors.Join(ErrUncertain, err)
		}
		return existing.ID, nil
	}

	var id string
	var add func() error
	if magnetHash, magnet := delugeMagnetHash(r.URL); magnet {
		if expected != "" && expected != magnetHash {
			return "", errors.New("the torrent address and expected info hash do not match")
		}
		expected = magnetHash
		add = func() error { return d.write(ctx, "core.add_torrent_magnet", []any{r.URL, delugeOptions(r)}, &id) }
	} else {
		payload, fetchErr := r.File, error(nil)
		if len(payload) == 0 {
			payload, fetchErr = fetchRelease(ctx, r.URL, r.AllowLocal, torrentFetchMax)
		}
		if fetchErr != nil {
			return "", fetchErr
		}
		if !r.AllowLocal {
			if payload, fetchErr = safeTorrent(payload); fetchErr != nil {
				return "", fetchErr
			}
		}
		fileHash, hashErr := torrentFileHash(payload)
		if hashErr != nil {
			return "", hashErr
		}
		if expected != "" && expected != fileHash {
			return "", errors.New("the torrent file and expected info hash do not match")
		}
		expected = fileHash
		name := "release.torrent"
		if u, parseErr := url.Parse(r.URL); parseErr == nil {
			if base := path.Base(u.Path); base != "." && base != "/" && base != "" {
				name = base
			}
		}
		add = func() error {
			return d.write(ctx, "core.add_torrent_file", []any{name, base64.StdEncoding.EncodeToString(payload), delugeOptions(r)}, &id)
		}
	}
	if r.Skip[expected] {
		return "", errors.New("the torrent belongs to another download")
	}
	labels, err := d.ready(ctx)
	if err != nil {
		return "", err
	}
	if existing, err := d.Get(ctx, expected); err == nil {
		if r.Skip[existing.ID] {
			return "", errors.New("the torrent belongs to another download")
		}
		if existing.Category != r.Category {
			return "", fmt.Errorf("deluge torrent %s already belongs to another category", expected)
		}
		return existing.ID, nil
	} else if !errors.Is(err, ErrNotFound) {
		return "", err
	}
	if err := d.ensureLabel(ctx, labels, r.Category); err != nil {
		return "", err
	}
	err = add()
	if err != nil {
		if delugeDefinitelyRefused(err) {
			return "", err
		}
		existing, getErr := d.Get(ctx, expected)
		if getErr == nil && existing.Category == r.Category && !r.Skip[existing.ID] {
			return existing.ID, nil
		}
		return "", errors.Join(ErrUncertain, err, getErr)
	}

	id = strings.ToLower(id)
	if !releasename.ValidInfoHash(id) {
		existing, getErr := d.Get(ctx, expected)
		if getErr == nil && existing.Category == r.Category && !r.Skip[existing.ID] {
			return existing.ID, nil
		}
		return "", errors.Join(ErrUncertain, errors.New("Deluge did not return a torrent info hash"), getErr)
	}
	if expected != "" && id != expected {
		return "", errors.Join(ErrUncertain, errors.New("Deluge returned a different torrent info hash"))
	}
	if r.Skip[id] {
		return "", errors.Join(ErrUncertain, errors.New("the torrent belongs to another download"))
	}
	if err := d.setLabel(ctx, id, r.Category); err != nil {
		return "", errors.Join(ErrUncertain, err)
	}
	return id, nil
}

// List returns the torrents in category.
func (d *Deluge) List(ctx context.Context, category string) ([]Job, error) {
	if _, err := d.ready(ctx); err != nil {
		return nil, err
	}
	var torrents map[string]delugeTorrent
	if err := d.read(ctx, "core.get_torrents_status", []any{map[string]any{}, delugeStatusKeys, false}, &torrents); err != nil {
		return nil, err
	}
	jobs := make([]Job, 0, len(torrents))
	for id, torrent := range torrents {
		if category == "" || torrent.Label == category {
			jobs = append(jobs, torrent.job(id))
		}
	}
	return jobs, nil
}

// Get returns the torrent with the given id.
func (d *Deluge) Get(ctx context.Context, id string) (Job, error) {
	id = strings.ToLower(id)
	if !releasename.ValidInfoHash(id) {
		return Job{}, errDelugeBadHash
	}
	if _, err := d.ready(ctx); err != nil {
		return Job{}, err
	}
	torrent, err := d.status(ctx, id)
	if err != nil {
		return Job{}, err
	}
	job := torrent.job(id)
	job.Files, err = d.files(ctx, id)
	if err != nil {
		return Job{}, err
	}
	return job, nil
}

// Remove deletes the torrent, and its data when deleteFiles is set.
func (d *Deluge) Remove(ctx context.Context, id string, deleteFiles bool) error {
	id = strings.ToLower(id)
	if !releasename.ValidInfoHash(id) {
		return errDelugeBadHash
	}
	if _, err := d.ready(ctx); err != nil {
		return err
	}
	if d.Category != "" {
		job, err := d.Get(ctx, id)
		if err != nil {
			return err
		}
		if job.Category != d.Category && !slices.Contains(d.Others, job.Category) {
			return errors.New("refusing to remove a torrent outside the configured category")
		}
	}
	var removed bool
	err := d.write(ctx, "core.remove_torrent", []any{id, deleteFiles}, &removed)
	if err == nil {
		if !removed {
			return ErrNotFound
		}
		return nil
	}
	_, getErr := d.Get(ctx, id)
	if errors.Is(getErr, ErrNotFound) {
		return nil
	}
	if errors.Is(err, ErrAuth) && getErr == nil {
		removed = false
		if retryErr := d.write(ctx, "core.remove_torrent", []any{id, deleteFiles}, &removed); retryErr != nil {
			return retryErr
		}
		if !removed {
			return ErrNotFound
		}
		return nil
	}
	return err
}

func delugeDefinitelyRefused(err error) bool {
	var rpcErr *delugeRPCError
	var nullResult delugeNullResultError
	return errors.Is(err, ErrAuth) || errors.As(err, &rpcErr) || errors.As(err, &nullResult)
}

func (d *Deluge) generation() uint64 {
	d.authMu.Lock()
	defer d.authMu.Unlock()
	return d.authGen
}

func (d *Deluge) rpcOnce(ctx context.Context, method string, params any, out any) error {
	id := d.requestID.Add(1)
	body, err := json.Marshal(map[string]any{
		"method": method,
		"params": params,
		"id":     id,
	})
	if err != nil {
		return fmt.Errorf("deluge %s request: %w", method, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.base+"/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("deluge %s: invalid Web address", method)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, outbound.ErrAddressRefused) {
			return outbound.ErrAddressRefused
		}
		return fmt.Errorf("deluge %s: %w", method, errDownloaderUnavailable)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return ErrAuth
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return adapterHTTPError{status: resp.StatusCode}
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("deluge %s response: %w", method, errDownloaderUnavailable)
	}
	var answer delugeRPCResponse
	if err := json.Unmarshal(raw, &answer); err != nil {
		return errors.New("Deluge returned an invalid JSON-RPC response")
	}
	var responseID int64
	if len(answer.ID) == 0 || json.Unmarshal(answer.ID, &responseID) != nil || responseID != id || len(answer.Result) == 0 || len(answer.Error) == 0 {
		return errors.New("Deluge returned an invalid JSON-RPC response")
	}
	resultNull := bytes.Equal(bytes.TrimSpace(answer.Result), []byte("null"))
	errorNull := bytes.Equal(bytes.TrimSpace(answer.Error), []byte("null"))
	if !errorNull {
		if !resultNull {
			return errors.New("Deluge returned an invalid JSON-RPC response")
		}
		var rpcErr delugeRPCError
		if json.Unmarshal(answer.Error, &rpcErr) != nil || rpcErr.Code == 0 || rpcErr.Message == "" {
			return errors.New("Deluge returned an invalid JSON-RPC error")
		}
		return &rpcErr
	}
	if out == nil {
		return nil
	}
	if resultNull {
		return delugeNullResultError{}
	}
	if err := json.Unmarshal(answer.Result, out); err != nil {
		return errors.New("Deluge returned an unexpected JSON-RPC result")
	}
	return nil
}

func (d *Deluge) login(ctx context.Context, stale uint64) error {
	select {
	case d.authGate <- struct{}{}:
		defer func() { <-d.authGate }()
	case <-ctx.Done():
		return ctx.Err()
	}
	if d.generation() != stale {
		return nil
	}
	var ok bool
	if err := d.rpcOnce(ctx, "auth.login", []any{d.pass}, &ok); err != nil {
		return err
	}
	if !ok {
		return ErrAuth
	}
	d.authMu.Lock()
	d.authGen++
	d.authMu.Unlock()
	return nil
}

func (d *Deluge) read(ctx context.Context, method string, params any, out any) error {
	stale := d.generation()
	err := d.rpcOnce(ctx, method, params, out)
	if !errors.Is(err, ErrAuth) {
		return err
	}
	if err := d.login(ctx, stale); err != nil {
		return err
	}
	return d.rpcOnce(ctx, method, params, out)
}

func (d *Deluge) write(ctx context.Context, method string, params any, out any) error {
	return d.rpcOnce(ctx, method, params, out)
}

func (d *Deluge) ready(ctx context.Context) ([]string, error) {
	var connected bool
	if err := d.read(ctx, "web.connected", []any{}, &connected); err != nil {
		return nil, err
	}
	if !connected {
		return nil, errDelugeDetached
	}
	var labels []string
	if err := d.read(ctx, "label.get_labels", []any{}, &labels); err != nil {
		return nil, errors.Join(errDelugeLabel, err)
	}
	return labels, nil
}

func hasString(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}

func (d *Deluge) ensureLabel(ctx context.Context, labels []string, label string) error {
	if label == "" || hasString(labels, label) {
		return nil
	}
	if err := d.write(ctx, "label.add", []any{label}, nil); err == nil {
		return nil
	} else {
		var refreshed []string
		if readErr := d.read(ctx, "label.get_labels", []any{}, &refreshed); readErr == nil && hasString(refreshed, label) {
			return nil
		}
		if errors.Is(err, ErrAuth) {
			if retryErr := d.write(ctx, "label.add", []any{label}, nil); retryErr == nil {
				return nil
			} else {
				err = retryErr
			}
		}
		return fmt.Errorf("deluge add label: %w", err)
	}
}

func (d *Deluge) setLabel(ctx context.Context, id, label string) error {
	if label == "" {
		return nil
	}
	set := func() error { return d.write(ctx, "label.set_torrent", []any{id, label}, nil) }
	err := set()
	existing, getErr := d.Get(ctx, id)
	if getErr == nil && existing.Category == label {
		return nil
	}
	if getErr == nil && existing.Category != "" {
		return fmt.Errorf("deluge torrent %s already belongs to another category", id)
	}
	if err == nil {
		if getErr != nil {
			return fmt.Errorf("deluge verify label: %w", getErr)
		}
		return errors.New("Deluge did not keep the requested label")
	}
	return fmt.Errorf("deluge set label: %w", err)
}

func delugeMagnetHash(raw string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(u.Scheme, "magnet") {
		return "", false
	}
	for _, xt := range u.Query()["xt"] {
		const prefix = "urn:btih:"
		if len(xt) > len(prefix) && strings.EqualFold(xt[:len(prefix)], prefix) {
			hash := strings.ToLower(xt[len(prefix):])
			if releasename.ValidInfoHash(hash) {
				return hash, true
			}
		}
	}
	return "", false
}

func delugeOptions(r AddRequest) map[string]any {
	options := make(map[string]any)
	if r.SavePath != "" {
		options["download_location"] = r.SavePath
	}
	return options
}

func (t delugeTorrent) job(id string) Job {
	progress := t.Progress / 100
	if progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}
	eta := t.ETA
	if eta < 0 || eta >= 8640000 {
		eta = 0
	}
	content := ""
	if t.SavePath != "" && t.Name != "" {
		content = path.Join(t.SavePath, t.Name)
	}
	finished := t.Finished && progress >= 1 && content != ""
	return Job{ID: strings.ToLower(id), Name: t.Name, Category: t.Label, SavePath: t.SavePath, ContentPath: content,
		Size: t.TotalWanted, Progress: progress, Speed: max(t.DownloadPayloadRate, 0), UpSpeed: max(t.UploadPayloadRate, 0), Seeds: max(t.NumSeeds, 0), Peers: max(t.NumPeers, 0), ETA: eta,
		State: delugeState(t.State, finished), Detail: t.State}
}

func delugeState(state string, finished bool) string {
	switch strings.ToLower(state) {
	case "error":
		return StatePaused
	case "checking", "moving":
		return StateQueued
	}
	if finished {
		return StateFinished
	}
	switch strings.ToLower(state) {
	case "paused":
		return StatePaused
	case "queued", "checking", "allocating", "moving":
		return StateQueued
	}
	if finished {
		return StateFinished
	}
	return StateDownloading
}

func (d *Deluge) status(ctx context.Context, id string) (delugeTorrent, error) {
	var torrent delugeTorrent
	var raw map[string]json.RawMessage
	if err := d.read(ctx, "core.get_torrent_status", []any{id, delugeStatusKeys}, &raw); err != nil {
		return torrent, err
	}
	if len(raw) == 0 {
		return torrent, ErrNotFound
	}
	b, _ := json.Marshal(raw)
	if err := json.Unmarshal(b, &torrent); err != nil {
		return torrent, fmt.Errorf("deluge torrent status: %w", err)
	}
	return torrent, nil
}

func safeDelugeFileName(name string) string {
	name = strings.ReplaceAll(name, `\`, "/")
	name = strings.TrimLeft(name, "/")
	name = path.Clean(name)
	if name == "." || name == ".." || strings.HasPrefix(name, "../") {
		return path.Base(name)
	}
	return name
}

func appendDelugeFiles(value any, out *[]File) {
	switch v := value.(type) {
	case []any:
		for _, item := range v {
			appendDelugeFiles(item, out)
		}
	case map[string]any:
		if contents, ok := v["contents"]; ok {
			appendDelugeFiles(contents, out)
			return
		}
		if kind, _ := v["type"].(string); kind == "file" || v["path"] != nil || v["name"] != nil {
			name, _ := v["path"].(string)
			if name == "" {
				name, _ = v["name"].(string)
			}
			size, _ := v["size"].(float64)
			progress, _ := v["progress"].(float64)
			if progress > 1 {
				progress /= 100
			}
			if progress < 0 {
				progress = 0
			}
			if progress > 1 {
				progress = 1
			}
			if name = safeDelugeFileName(name); name != "" {
				*out = append(*out, File{Name: name, Size: int64(size), Progress: progress})
			}
			return
		}
		for _, child := range v {
			appendDelugeFiles(child, out)
		}
	}
}

func (d *Deluge) files(ctx context.Context, id string) ([]File, error) {
	var raw any
	if err := d.read(ctx, "web.get_torrent_files", []any{id}, &raw); err != nil {
		return nil, err
	}
	var files []File
	appendDelugeFiles(raw, &files)
	return files, nil
}
