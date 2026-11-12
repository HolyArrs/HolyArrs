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
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ErrNoCategory means the target category is missing in SABnzbd.
var ErrNoCategory = errors.New("the category does not exist in SABnzbd; add it there first (Config > Categories)")

var errSabNo = errors.New("sabnzbd said no")

var trailLink = regexp.MustCompile(`\s+-\s+https?://\S+$`)

// Sabnzbd is a Client for the SABnzbd HTTP API.
type Sabnzbd struct {
	base string
	key  string
	http *http.Client
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
			AvgAge     string `json:"avg_age"`
		} `json:"slots"`
	} `json:"queue"`
}

type sabHistory struct {
	History struct {
		Slots      []sabHistorySlot `json:"slots"`
		LastUpdate json.RawMessage  `json:"last_history_update"`
	} `json:"history"`
}

type sabHistorySlot struct {
	ID          string `json:"nzo_id"`
	Name        string `json:"name"`
	Category    string `json:"category"`
	Bytes       int64  `json:"bytes"`
	Status      string `json:"status"`
	FailMessage string `json:"fail_message"`
	Storage     string `json:"storage"`
	Completed   int64  `json:"completed"`
	TimeAdded   int64  `json:"time_added"`
	StageLog    []struct {
		Name    string   `json:"name"`
		Actions []string `json:"actions"`
	} `json:"stage_log"`
}

// NewSabnzbd returns a client for the SABnzbd at base, using key and h.
func NewSabnzbd(base, key string, h *http.Client) *Sabnzbd {
	if h == nil {
		h = &http.Client{Timeout: clientTimeout}
	}
	return &Sabnzbd{base: strings.TrimRight(base, "/"), key: key, http: h}
}

// Health reports whether SABnzbd answers and accepts the key.
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

// HasCategory reports whether cat exists in SABnzbd.
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
	return hasCategory(c.Categories, cat), nil
}

// finishedFolders tells where SABnzbd puts finished jobs, as it sees them: its
// completed folder (a full path in fullstatus, while its settings may hold a
// relative one), then each of cats' folder that is a full path of its own.
// Other category folders sit inside the completed one.
func (s *Sabnzbd) finishedFolders(ctx context.Context, cats []string) ([]string, error) {
	b, err := s.call(ctx, "fullstatus", url.Values{"skip_dashboard": {"1"}}, true)
	if err != nil {
		return nil, err
	}
	var st struct {
		Status map[string]json.RawMessage `json:"status"`
	}
	if err := json.Unmarshal(b, &st); err != nil {
		return nil, fmt.Errorf("sabnzbd fullstatus: %w", err)
	}
	var out []string
	var done string
	if json.Unmarshal(st.Status["completedir"], &done) == nil && strings.TrimSpace(done) != "" {
		out = append(out, strings.TrimSpace(done))
	}
	b, err = s.call(ctx, "get_config", url.Values{"section": {"categories"}}, true)
	if err != nil {
		return out, nil // the completed folder alone still helps
	}
	var c struct {
		Config struct {
			Categories []struct {
				Name string `json:"name"`
				Dir  string `json:"dir"`
			} `json:"categories"`
		} `json:"config"`
	}
	if json.Unmarshal(b, &c) != nil {
		return out, nil
	}
	for _, x := range c.Config.Categories {
		// A trailing * only says SABnzbd adds no job folder.
		if dir := strings.TrimSuffix(strings.TrimSpace(x.Dir), "*"); hasCategory(cats, x.Name) && absPath(dir) {
			out = append(out, dir)
		}
	}
	return out, nil
}

// Add queues an NZB and returns its job id.
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
	q := url.Values{}
	if r.Category != "" {
		q.Set("cat", r.Category)
	}
	if r.Name != "" {
		q.Set("nzbname", r.Name)
	}
	var b []byte
	var err error
	if len(r.File) > 0 {
		name := "release.nzb"
		if r.Name != "" {
			name = safeFileName(r.Name) + ".nzb"
		}
		b, err = s.callFile(ctx, "addfile", q, name, r.File)
	} else {
		q.Set("name", r.URL)
		b, err = s.call(ctx, "addurl", q, true)
	}
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

// List returns the jobs in category.
func (s *Sabnzbd) List(ctx context.Context, category string) ([]Job, error) {
	q := url.Values{"start": {"0"}, "limit": {"0"}}
	if category != "" {
		q.Set("category", category)
	}
	return s.jobs(ctx, q)
}

// Get returns the job with the given id.
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

// Remove deletes the job, and its files when deleteFiles is set.
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
	return s.do(req, mode)
}

func (s *Sabnzbd) callFile(ctx context.Context, mode string, q url.Values, file string, data []byte) ([]byte, error) {
	q.Set("mode", mode)
	q.Set("output", "json")
	q.Set("apikey", s.key)
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	w, err := mw.CreateFormFile("name", file)
	if err != nil {
		return nil, err
	}
	_, _ = w.Write(data)
	_ = mw.Close()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.base+"/api?"+q.Encode(), &body)
	if err != nil {
		return nil, fmt.Errorf("sabnzbd %s: bad address", mode)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return s.do(req, mode)
}

func (s *Sabnzbd) do(req *http.Request, mode string) ([]byte, error) {
	resp, err := s.http.Do(req)
	if err != nil {
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

func (x sabHistorySlot) job() Job {
	j := Job{ID: x.ID, Name: x.Name, Category: x.Category, Size: x.Bytes, Detail: x.Status, AddedAt: x.TimeAdded}
	switch x.Status {
	case "Completed":
		j.State, j.Progress = StateFinished, 1
		j.DoneAt = x.Completed
		j.ContentPath = x.Storage
		j.SavePath = parentDir(x.Storage)
	case "Failed":
		j.State, j.Progress = StateFailed, 0
		if m := x.failReason(); m != "" {
			j.Detail = m
		}
	default:
		j.State, j.Progress = StateDownloading, 1
	}
	return j
}

func (x sabHistorySlot) failReason() string {
	m := x.FailMessage
	if m == "" || strings.HasPrefix(strings.TrimSpace(m), "Aborted, cannot be completed") {
		for i := len(x.StageLog) - 1; i >= 0; i-- {
			st := x.StageLog[i]
			if st.Name != "Download" && st.Name != "Verify" && st.Name != "Repair" {
				continue
			}
			for k := len(st.Actions) - 1; k >= 0; k-- {
				if a := strings.TrimSpace(st.Actions[k]); a != "" && !strings.HasPrefix(a, "Aborted") {
					m = a
					i = -1
					break
				}
			}
		}
	}
	return trailLink.ReplaceAllString(cleanLine(m), "")
}

func num(s string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return f
}

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

func sabQueueState(s string) string {
	switch s {
	case "Paused":
		return StatePaused
	case "Queued", "Grabbing", "Fetching", "Propagating", "Checking":
		return StateQueued
	}
	return StateDownloading
}

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
			ETA: clock(x.TimeLeft), State: sabQueueState(x.Status), Detail: x.Status, PostedAt: sabPostedAt(x.AvgAge, time.Now())}
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
		out = append(out, x.job())
	}
	return out, nil
}

func sabPostedAt(age string, now time.Time) int64 {
	age = strings.TrimSpace(age)
	if len(age) < 2 {
		return 0
	}
	n, err := strconv.Atoi(age[:len(age)-1])
	if err != nil || n < 0 {
		return 0
	}
	var unit time.Duration
	switch age[len(age)-1] {
	case 'd':
		unit = 24 * time.Hour
	case 'h':
		unit = time.Hour
	case 'm':
		unit = time.Minute
	default:
		return 0
	}
	return now.Add(-time.Duration(n) * unit).Truncate(unit).Unix()
}
