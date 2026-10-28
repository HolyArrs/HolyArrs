package downloader

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Sabnzbd talks to the SABnzbd API: one address, every call names its
// action. The key travels in the query, so no error ever carries the
// address (it would carry the key).
type Sabnzbd struct {
	base string
	key  string
	http *http.Client
}

// NewSabnzbd makes an engine for the SABnzbd at base (such as http://sabnzbd:8080).
func NewSabnzbd(base, key string, h *http.Client) *Sabnzbd {
	if h == nil {
		h = &http.Client{Timeout: clientTimeout}
	}
	return &Sabnzbd{base: strings.TrimRight(base, "/"), key: key, http: h}
}

// ErrNoCategory means the category is missing in SABnzbd, which then files
// the job under its default and HolyArrs would lose it.
var ErrNoCategory = errors.New("the category does not exist in SABnzbd; add it there first (Config > Categories)")

// errSabNo is a status-false answer with no message: a delete of a job
// that is not in that list.
var errSabNo = errors.New("sabnzbd said no")

func (s *Sabnzbd) call(ctx context.Context, mode string, q url.Values, keyed bool) ([]byte, error) {
	if q == nil {
		q = url.Values{}
	}
	q.Set("mode", mode)
	q.Set("output", "json")
	if keyed {
		q.Set("apikey", s.key)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.base+"/api?"+q.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("sabnzbd %s: bad address", mode)
	}
	resp, err := s.http.Do(req)
	if err != nil {
		// url.Error holds the address, and the address holds the key.
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return nil, fmt.Errorf("sabnzbd %s: %w", mode, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		line := strings.TrimSpace(string(b))
		if line == "" || strings.Contains(line, "API Key") {
			return nil, ErrAuth
		}
		// Outside-access or host-name refusals: said in our words, never
		// with the body the other side sent.
		switch {
		case strings.Contains(line, "Hostname verification"):
			return nil, errors.New("sabnzbd refused the call: Hostname verification failed (add this address to its host_whitelist)")
		case strings.Contains(line, "External internet access denied"):
			return nil, errors.New("sabnzbd refused the call: it does not allow outside access from here")
		}
		return nil, fmt.Errorf("sabnzbd refused the call (%s)", resp.Status)
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("sabnzbd %s: no API at this address", mode)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("sabnzbd %s: %s", mode, resp.Status)
	}
	var st struct {
		Status *bool  `json:"status"`
		Error  string `json:"error"`
	}
	if json.Unmarshal(b, &st) == nil && st.Status != nil && !*st.Status {
		if st.Error == "" {
			return nil, errSabNo
		}
		if strings.Contains(st.Error, "API Key") {
			return nil, ErrAuth
		}
		return nil, fmt.Errorf("sabnzbd %s: %s", mode, firstLine(st.Error))
	}
	return b, nil
}

func firstLine(s string) string {
	s, _, _ = strings.Cut(s, "\n")
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

// Health asks for the version, then reads the queue with the key: the
// version call takes no key, so it alone would not catch a wrong one.
func (s *Sabnzbd) Health(ctx context.Context) (Health, error) {
	b, err := s.call(ctx, "version", nil, false)
	if err != nil {
		return Health{}, err
	}
	var v struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(b, &v); err != nil || v.Version == "" {
		return Health{}, errors.New("the address answered, but not like SABnzbd")
	}
	if _, err := s.call(ctx, "queue", url.Values{"limit": {"1"}}, true); err != nil {
		return Health{}, err
	}
	return Health{Version: v.Version, At: time.Now().UTC()}, nil
}

// HasCategory says whether SABnzbd knows the category.
func (s *Sabnzbd) HasCategory(ctx context.Context, cat string) (bool, error) {
	b, err := s.call(ctx, "get_cats", nil, true)
	if err != nil {
		return false, err
	}
	var c struct {
		Categories []string `json:"categories"`
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return false, fmt.Errorf("sabnzbd get_cats: %w", err)
	}
	return slices.Contains(c.Categories, cat), nil
}

// Add sends an NZB by link. A job with the same name already in the
// category (queue or finished list) is taken as this one, so a retry never
// adds it twice.
func (s *Sabnzbd) Add(ctx context.Context, r AddRequest) (string, error) {
	if r.Category != "" && r.Category != "*" {
		ok, err := s.HasCategory(ctx, r.Category)
		if err != nil {
			return "", err
		}
		if !ok {
			return "", fmt.Errorf("%w (%s)", ErrNoCategory, r.Category)
		}
	}
	if r.Name != "" {
		jobs, err := s.List(ctx, r.Category)
		if err != nil {
			return "", err
		}
		for _, j := range jobs {
			if strings.EqualFold(j.Name, r.Name) && j.State != StateFailed && !r.Skip[j.ID] {
				return j.ID, nil
			}
		}
	}
	q := url.Values{"name": {r.URL}}
	if r.Category != "" {
		q.Set("cat", r.Category)
	}
	if r.Name != "" {
		q.Set("nzbname", r.Name)
	}
	b, err := s.call(ctx, "addurl", q, true)
	if err != nil {
		return "", err
	}
	var out struct {
		IDs []string `json:"nzo_ids"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return "", fmt.Errorf("sabnzbd addurl: %w", err)
	}
	if len(out.IDs) == 0 || out.IDs[0] == "" {
		return "", errors.New("sabnzbd refused the NZB")
	}
	return out.IDs[0], nil
}

type sabQueue struct {
	Queue struct {
		Status   string `json:"status"`
		KBPerSec string `json:"kbpersec"`
		Slots    []struct {
			ID         string `json:"nzo_id"`
			Filename   string `json:"filename"`
			Cat        string `json:"cat"`
			MB         string `json:"mb"`
			MBLeft     string `json:"mbleft"`
			Percentage string `json:"percentage"`
			Status     string `json:"status"`
			TimeLeft   string `json:"timeleft"`
		} `json:"slots"`
	} `json:"queue"`
}

type sabHistory struct {
	History struct {
		Slots []struct {
			ID          string `json:"nzo_id"`
			Name        string `json:"name"`
			Category    string `json:"category"`
			Bytes       int64  `json:"bytes"`
			Status      string `json:"status"`
			FailMessage string `json:"fail_message"`
			Storage     string `json:"storage"`
		} `json:"slots"`
	} `json:"history"`
}

func num(s string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return f
}

// clock reads SABnzbd's time left: H:MM:SS, or D:HH:MM:SS past one day.
func clock(s string) int64 {
	parts := strings.Split(strings.TrimSpace(s), ":")
	if len(parts) < 3 || len(parts) > 4 {
		return 0
	}
	mult := []int64{1, 60, 3600, 86400}
	var total int64
	for i := range parts {
		n, err := strconv.ParseInt(parts[len(parts)-1-i], 10, 64)
		if err != nil || n < 0 {
			return 0
		}
		total += n * mult[i]
	}
	return total
}

// sabQueueState maps a queue word onto ours. Unknown words are downloading.
func sabQueueState(s string) string {
	switch s {
	case "Paused":
		return StatePaused
	case "Queued", "Grabbing", "Fetching", "Propagating", "Checking":
		return StateQueued
	}
	return StateDownloading
}

// parentDir is the folder holding p, for either kind of slash.
func parentDir(p string) string {
	p = strings.TrimRight(p, `/\`)
	if i := strings.LastIndexAny(p, `/\`); i > 0 {
		return p[:i]
	}
	return p
}

func (s *Sabnzbd) jobs(ctx context.Context, q url.Values) ([]Job, error) {
	b, err := s.call(ctx, "queue", q, true)
	if err != nil {
		return nil, err
	}
	var qu sabQueue
	if err := json.Unmarshal(b, &qu); err != nil {
		return nil, fmt.Errorf("sabnzbd queue: %w", err)
	}
	var out []Job
	moving := 0
	for _, x := range qu.Queue.Slots {
		if sabQueueState(x.Status) == StateDownloading {
			moving++
		}
	}
	for _, x := range qu.Queue.Slots {
		mb, left := num(x.MB), num(x.MBLeft)
		p := num(x.Percentage) / 100
		if mb > 0 {
			p = (mb - left) / mb
		}
		j := Job{ID: x.ID, Name: x.Filename, Category: x.Cat, Size: int64(mb * 1024 * 1024), Progress: min(max(p, 0), 1),
			ETA: clock(x.TimeLeft), State: sabQueueState(x.Status), Detail: x.Status}
		// SABnzbd gives one speed for the whole queue; the jobs moving
		// share it evenly so the total stays the real one.
		if j.State == StateDownloading && moving > 0 {
			j.Speed = int64(num(qu.Queue.KBPerSec)*1024) / int64(moving)
		}
		out = append(out, j)
	}
	hq := url.Values{"start": {"0"}, "limit": {"200"}}
	for _, k := range []string{"category", "nzo_ids"} {
		if v := q.Get(k); v != "" {
			hq.Set(k, v)
		}
	}
	b, err = s.call(ctx, "history", hq, true)
	if err != nil {
		return nil, err
	}
	var hi sabHistory
	if err := json.Unmarshal(b, &hi); err != nil {
		return nil, fmt.Errorf("sabnzbd history: %w", err)
	}
	for _, x := range hi.History.Slots {
		j := Job{ID: x.ID, Name: x.Name, Category: x.Category, Size: x.Bytes, Detail: x.Status}
		switch x.Status {
		case "Completed":
			j.State, j.Progress = StateFinished, 1
			j.ContentPath = x.Storage
			j.SavePath = parentDir(x.Storage)
		case "Failed":
			j.State, j.Progress = StateFailed, 0
			if x.FailMessage != "" {
				j.Detail = x.FailMessage
			}
		default:
			// Still being repaired or unpacked: downloading at full progress.
			j.State, j.Progress = StateDownloading, 1
		}
		out = append(out, j)
	}
	return out, nil
}

// List returns the jobs in a category, from the queue and the finished list.
func (s *Sabnzbd) List(ctx context.Context, category string) ([]Job, error) {
	q := url.Values{"start": {"0"}, "limit": {"0"}}
	if category != "" {
		q.Set("category", category)
	}
	return s.jobs(ctx, q)
}

// Get returns one job. SABnzbd lists no files for finished jobs; the
// manager reads the finished folder itself.
func (s *Sabnzbd) Get(ctx context.Context, id string) (Job, error) {
	js, err := s.jobs(ctx, url.Values{"start": {"0"}, "limit": {"0"}, "nzo_ids": {id}})
	if err != nil {
		return Job{}, err
	}
	for _, j := range js {
		if j.ID == id {
			return j, nil
		}
	}
	return Job{}, ErrNotFound
}

// Remove deletes a job from the queue and from the finished list (really
// deleted, not archived), with or without its files.
func (s *Sabnzbd) Remove(ctx context.Context, id string, deleteFiles bool) error {
	del := "0"
	if deleteFiles {
		del = "1"
	}
	_, qerr := s.call(ctx, "queue", url.Values{"name": {"delete"}, "value": {id}, "del_files": {del}}, true)
	if qerr != nil && !errors.Is(qerr, errSabNo) {
		return qerr
	}
	_, herr := s.call(ctx, "history", url.Values{"name": {"delete"}, "value": {id}, "del_files": {del}, "archive": {"0"}}, true)
	if herr != nil && !errors.Is(herr, errSabNo) {
		return herr
	}
	if qerr != nil && herr != nil {
		return ErrNotFound
	}
	return nil
}
