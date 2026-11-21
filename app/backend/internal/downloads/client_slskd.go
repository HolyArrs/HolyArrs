package downloads

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/doctor"
	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
)

// Slskd is a Client for slskd, which downloads from Soulseek users. A slskd
// Release is one user's folder of files; slskd keeps one transfer per file,
// so a job is every transfer from that user in that folder, and its id comes
// from the user and the folder. slskd has no categories: every folder it
// holds belongs to the Downloader's own category.
type Slskd struct {
	Category string
	Others   []string
	// Folder is slskd's download folder as slskd sees it, from the Downloader's
	// first path pair. It stands in when slskd keeps its settings from the key:
	// some versions show them to administrator keys only.
	Folder string

	base, key string
	http      *http.Client

	mu        sync.Mutex
	downloads string // slskd's download folder, as slskd sees it
}

// NewSlskd returns a client for the slskd at base, using an API key with the readwrite role.
func NewSlskd(base, key string, h *http.Client) *Slskd {
	if h == nil {
		h = &http.Client{Timeout: clientTimeout}
	}
	return &Slskd{base: strings.TrimRight(strings.TrimSpace(base), "/"), key: key, http: h}
}

type slskdTransfer struct {
	ID               string   `json:"id"`
	Username         string   `json:"username"`
	Filename         string   `json:"filename"`
	Size             int64    `json:"size"`
	State            string   `json:"state"`
	BytesTransferred int64    `json:"bytesTransferred"`
	AverageSpeed     float64  `json:"averageSpeed"`
	RequestedAt      string   `json:"requestedAt"`
	EndedAt          string   `json:"endedAt"`
	Exception        string   `json:"exception"`
	NextAttemptAt    *string  `json:"nextAttemptAt"`
	PercentComplete  *float64 `json:"percentComplete"`
}

type slskdUser struct {
	Username    string `json:"username"`
	Directories []struct {
		Directory string          `json:"directory"`
		Files     []slskdTransfer `json:"files"`
	} `json:"directories"`
}

// SlskdID is the job id of a user's folder.
func SlskdID(user, dir string) string {
	sum := sha256.Sum256([]byte(user + "\x00" + slskdDir(dir)))
	return "slskd-" + hex.EncodeToString(sum[:10])
}

// slskdDir is a remote folder with slashes, whichever way its owner writes them.
func slskdDir(p string) string { return strings.TrimRight(strings.ReplaceAll(p, `\`, "/"), "/") }

// slskdLinkID is the job id a slskd Release link will have.
func slskdLinkID(link string) (string, bool) {
	r, err := sources.ParseSlskdLink(link)
	if err != nil {
		return "", false
	}
	return SlskdID(r.User, path.Dir(slskdDir(r.Files[0].Name))), true
}

// Health reports whether slskd answers, takes the key for downloads and where it saves.
func (s *Slskd) Health(ctx context.Context) (Health, error) {
	raw, err := s.call(ctx, http.MethodGet, "/api/v0/application/version", nil)
	if err != nil {
		return Health{}, err
	}
	// The version comes as JSON text or as plain text, depending on what slskd's framework picks.
	var version string
	if json.Unmarshal(raw, &version) != nil {
		version = strings.TrimSpace(string(raw))
	}
	if version == "" || len(version) > 40 || !strings.ContainsAny(version[:1], "0123456789") {
		return Health{}, doctor.Mark(doctor.WrongApp, errors.New("the address answered, but not like slskd"))
	}
	// An empty download list is refused for a read-only key and turned down as
	// empty for a readwrite one, so this asks without downloading anything.
	if _, err := s.call(ctx, http.MethodPost, "/api/v0/transfers/downloads/holyarrs-check", []any{}); err != nil {
		var st adapterHTTPError
		switch {
		case errors.Is(err, ErrAuth):
			return Health{}, errors.New("slskd's API key may only read; give HolyArrs a key with the readwrite role")
		case !errors.As(err, &st) || st.status != http.StatusBadRequest:
			return Health{}, err
		}
	}
	dir, sub, err := s.options(ctx)
	if err != nil {
		return Health{}, err
	}
	if dir == "" {
		return Health{}, errors.New("slskd does not show this key where it saves downloads; add a path pair from slskd's download folder to the same folder as HolyArrs sees it")
	}
	var notes []string
	if sub != "" && sub != "${SOURCE_DIRECTORY}" {
		notes = append(notes, "slskd saves into "+sub+"; HolyArrs looks for each folder by its own name in "+dir)
	}
	raw, err = s.call(ctx, http.MethodGet, "/api/v0/application", nil)
	if err == nil {
		var app struct {
			Server struct {
				State string `json:"state"`
			} `json:"server"`
		}
		if json.Unmarshal(raw, &app) == nil && app.Server.State != "" && !strings.Contains(app.Server.State, "LoggedIn") {
			notes = append(notes, "slskd is not signed in to the Soulseek network ("+app.Server.State+")")
		}
	}
	return Health{Version: version, At: time.Now().UTC(), Note: strings.Join(notes, "; ")}, nil
}

// options reads slskd's download folder and the folder pattern inside it.
// When slskd keeps its settings from the key, the folder is the path pair's,
// or empty without one, and the pattern is unknown.
func (s *Slskd) options(ctx context.Context) (string, string, error) {
	raw, err := s.call(ctx, http.MethodGet, "/api/v0/options", nil)
	if errors.Is(err, ErrAuth) {
		dir := strings.TrimRight(s.Folder, "/")
		if absPath(dir) {
			s.mu.Lock()
			s.downloads = dir
			s.mu.Unlock()
			return dir, "", nil
		}
		return "", "", nil
	}
	if err != nil {
		return "", "", err
	}
	var o struct {
		Directories struct {
			Downloads string `json:"downloads"`
		} `json:"directories"`
		Transfers struct {
			Download struct {
				Destination struct {
					Subdirectory string `json:"subdirectory"`
				} `json:"destination"`
			} `json:"download"`
		} `json:"transfers"`
	}
	if json.Unmarshal(raw, &o) != nil || !absPath(o.Directories.Downloads) {
		return "", "", errors.New("slskd did not say where it saves downloads")
	}
	s.mu.Lock()
	s.downloads = strings.TrimRight(o.Directories.Downloads, "/")
	s.mu.Unlock()
	return o.Directories.Downloads, o.Transfers.Download.Destination.Subdirectory, nil
}

func (s *Slskd) saveDir(ctx context.Context) (string, error) {
	s.mu.Lock()
	dir := s.downloads
	s.mu.Unlock()
	if dir != "" {
		return dir, nil
	}
	dir, _, err := s.options(ctx)
	if err == nil && dir == "" {
		err = errors.New("slskd does not show this key where it saves downloads; add a path pair from slskd's download folder")
	}
	return strings.TrimRight(dir, "/"), err
}

// Add enqueues the files of a slskd Release link. A folder slskd already
// holds is found again by its id and never sent twice.
func (s *Slskd) Add(ctx context.Context, r AddRequest) (string, error) {
	if r.ReconcileOnly {
		if r.InfoHash == "" {
			return "", errors.Join(ErrUncertain, errors.New("the send to slskd has no id to look for"))
		}
		if _, err := s.Get(ctx, r.InfoHash); err != nil {
			if errors.Is(err, ErrNotFound) {
				return "", errors.Join(ErrUncertain, ErrNotArrived)
			}
			return "", err
		}
		return r.InfoHash, nil
	}
	rel, err := sources.ParseSlskdLink(r.URL)
	if err != nil {
		return "", releaseRefused{err.Error()}
	}
	dir := path.Dir(slskdDir(rel.Files[0].Name))
	id := SlskdID(rel.User, dir)
	if r.Skip[id] {
		return "", errors.New("slskd already holds this folder for another download")
	}
	if _, err := s.Get(ctx, id); err == nil {
		return id, nil
	} else if !errors.Is(err, ErrNotFound) {
		return "", err
	}
	files := make([]map[string]any, 0, len(rel.Files))
	for _, f := range rel.Files {
		files = append(files, map[string]any{"filename": f.Name, "size": f.Size})
	}
	raw, err := s.call(ctx, http.MethodPost, "/api/v0/transfers/downloads/"+url.PathEscape(rel.User), files)
	if err != nil {
		var st adapterHTTPError
		if errors.As(err, &st) && st.status < 500 || errors.Is(err, ErrAuth) {
			return "", err
		}
		// slskd waits for the other user to answer before it replies, so a slow
		// or lost reply may still have queued the files.
		return "", errors.Join(ErrUncertain, err)
	}
	var out struct {
		Enqueued []json.RawMessage `json:"enqueued"`
		Failed   []struct {
			Filename string `json:"filename"`
			Message  string `json:"message"`
			Item1    string `json:"item1"`
			Item2    string `json:"item2"`
		} `json:"failed"`
	}
	if json.Unmarshal(raw, &out) == nil && len(out.Enqueued) == 0 && len(out.Failed) > 0 {
		why := out.Failed[0].Message
		if why == "" {
			why = out.Failed[0].Item2
		}
		return "", fmt.Errorf("slskd could not queue the files: %s", firstLine(why))
	}
	return id, nil
}

// List gives every folder slskd holds when category is the Downloader's own, and nothing for any other.
func (s *Slskd) List(ctx context.Context, category string) ([]Job, error) {
	if !s.ours(category) {
		return nil, nil
	}
	return s.jobs(ctx)
}

// Get gives one folder by its id.
func (s *Slskd) Get(ctx context.Context, id string) (Job, error) {
	jobs, err := s.jobs(ctx)
	if err != nil {
		return Job{}, err
	}
	for _, j := range jobs {
		if j.ID == id {
			return j, nil
		}
	}
	return Job{}, ErrNotFound
}

// Remove cancels the folder's transfers and drops them from slskd's list.
// slskd keeps the files it already saved; HolyArrs never deletes them here.
func (s *Slskd) Remove(ctx context.Context, id string, deleteFiles bool) error {
	users, err := s.transfers(ctx)
	if err != nil {
		return err
	}
	found := false
	for _, u := range users {
		for _, d := range u.Directories {
			if SlskdID(u.Username, d.Directory) != id {
				continue
			}
			found = true
			for _, t := range d.Files {
				_, err := s.call(ctx, http.MethodDelete, "/api/v0/transfers/downloads/"+url.PathEscape(u.Username)+"/"+url.PathEscape(t.ID)+"?remove=true", nil)
				if err != nil && !errors.Is(err, ErrNotFound) {
					return err
				}
			}
		}
	}
	if !found {
		return ErrNotFound
	}
	return nil
}

func (s *Slskd) ours(category string) bool {
	if category == s.Category {
		return true
	}
	for _, o := range s.Others {
		if o == category {
			return true
		}
	}
	return false
}

func (s *Slskd) transfers(ctx context.Context) ([]slskdUser, error) {
	raw, err := s.call(ctx, http.MethodGet, "/api/v0/transfers/downloads", nil)
	if err != nil {
		return nil, err
	}
	var users []slskdUser
	if json.Unmarshal(raw, &users) != nil {
		return nil, doctor.Mark(doctor.WrongApp, errors.New("slskd answered with something that is not its download list"))
	}
	return users, nil
}

func (s *Slskd) jobs(ctx context.Context) ([]Job, error) {
	users, err := s.transfers(ctx)
	if err != nil {
		return nil, err
	}
	save, err := s.saveDir(ctx)
	if err != nil {
		return nil, err
	}
	var out []Job
	for _, u := range users {
		for _, d := range u.Directories {
			if len(d.Files) > 0 {
				out = append(out, slskdJob(u.Username, d.Directory, d.Files, save, s.Category))
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// slskdJob folds a folder's transfers into one job. slskd saves a folder
// under its own last name in the download folder, so that is where it is.
func slskdJob(user, dir string, ts []slskdTransfer, save, category string) Job {
	name := path.Base(slskdDir(dir))
	j := Job{ID: SlskdID(user, dir), Name: name, Category: category, SavePath: save, ContentPath: save + "/" + name}
	var done int64
	running, waiting, failed, succeeded := 0, 0, 0, 0
	worst := ""
	for _, t := range ts {
		j.Size += t.Size
		got := min(max(t.BytesTransferred, 0), t.Size)
		done += got
		p := 0.0
		if t.Size > 0 {
			p = float64(got) / float64(t.Size)
		}
		j.Files = append(j.Files, File{Name: name + "/" + path.Base(slskdDir(t.Filename)), Size: t.Size, Progress: p})
		state := slskdState(t)
		switch state {
		case StateFinished:
			succeeded++
		case StateFailed:
			failed++
			worst = t.State
			if t.Exception != "" {
				worst += ": " + firstLine(t.Exception)
			}
		case StateDownloading:
			running++
			j.Speed += int64(max(t.AverageSpeed, 0))
		default:
			waiting++
		}
		if at, err := time.Parse(time.RFC3339Nano, t.RequestedAt); err == nil && (j.AddedAt == 0 || at.Unix() < j.AddedAt) {
			j.AddedAt = at.Unix()
		}
		if at, err := time.Parse(time.RFC3339Nano, t.EndedAt); err == nil && at.Unix() > j.DoneAt {
			j.DoneAt = at.Unix()
		}
	}
	if j.Size > 0 {
		j.Progress = float64(done) / float64(j.Size)
	}
	switch {
	case failed > 0 && running == 0 && waiting == 0:
		j.State, j.Detail = StateFailed, worst
	case running > 0:
		j.State, j.Detail = StateDownloading, "InProgress"
	case succeeded == len(ts):
		j.State, j.Detail = StateFinished, "Completed, Succeeded"
	default:
		j.State, j.Detail = StateQueued, ts[0].State
	}
	if j.State != StateFinished {
		j.DoneAt = 0
	}
	if j.Speed > 0 {
		j.ETA = (j.Size - done) / j.Speed
	}
	return j
}

// slskdState maps a transfer's state flags ("Queued, Remotely",
// "Completed, Succeeded", "Completed, Errored") onto ours. A failed file
// slskd will try again is still waiting.
func slskdState(t slskdTransfer) string {
	has := func(w string) bool {
		for _, f := range strings.Split(t.State, ",") {
			if strings.TrimSpace(f) == w {
				return true
			}
		}
		return false
	}
	switch {
	case has("Succeeded"):
		return StateFinished
	case has("Completed") && t.NextAttemptAt != nil && *t.NextAttemptAt != "":
		return StateQueued
	case has("Completed"):
		return StateFailed
	case has("InProgress"), has("Initializing"):
		return StateDownloading
	}
	return StateQueued
}

func (s *Slskd) call(ctx context.Context, method, p string, body any) ([]byte, error) {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, s.base+p, rd)
	if err != nil {
		return nil, errors.New("slskd: bad address")
	}
	req.Header.Set("X-API-Key", s.key)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := s.http.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return nil, fmt.Errorf("slskd: %w", err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20+1))
	if err != nil {
		return nil, fmt.Errorf("slskd: %w", err)
	}
	if len(b) > 32<<20 {
		return nil, ErrTooLarge
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		if e := doctor.Other(resp.Header, b, "slskd"); e != nil {
			return nil, fmt.Errorf("slskd: %w", e)
		}
		return nil, ErrAuth
	case resp.StatusCode == http.StatusNotFound && method == http.MethodDelete:
		return nil, ErrNotFound
	case resp.StatusCode < 200 || resp.StatusCode > 299:
		if e := doctor.Other(resp.Header, b, "slskd"); e != nil {
			return nil, fmt.Errorf("slskd: %w", e)
		}
		line := firstLine(strings.TrimSpace(string(b)))
		if len(line) > 200 {
			line = line[:200]
		}
		if line != "" {
			return nil, fmt.Errorf("slskd answered %s: %s: %w", resp.Status, line, adapterHTTPError{resp.StatusCode})
		}
		return nil, adapterHTTPError{resp.StatusCode}
	}
	return b, nil
}
