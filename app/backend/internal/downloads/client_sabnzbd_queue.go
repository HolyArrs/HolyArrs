package downloads

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	sabPage    = 200
	sabMaxRows = 5000
)

var tagRe = regexp.MustCompile(`<[^>]*>`)

var passRe = regexp.MustCompile(`(?i)(password\W{0,3})("[^"]*"|'[^']*')`)

var (
	_ QueueReader     = (*Sabnzbd)(nil)
	_ UsenetInspector = (*Sabnzbd)(nil)
	_ Retrier         = (*Sabnzbd)(nil)
)

// QueueReader reads the live download queue.
type QueueReader interface {
	Queue(ctx context.Context) (QueuePage, error)
}

// QueueHead holds the queue summary values.
type QueueHead struct {
	Paused    bool
	Speed     int64
	TimeLeft  int64
	FreeSpace int64
}

// QueuePage is one snapshot of the queue.
type QueuePage struct {
	Head QueueHead
	Jobs []Job
}

// UsenetInspector gives per-job and per-server detail.
type UsenetInspector interface {
	JobStages(ctx context.Context, id string) ([]JobStage, error)
	ServerStats(ctx context.Context) (ServerStats, error)
}

// JobStage is one processing stage of a job.
type JobStage struct {
	Stage string   `json:"stage"`
	Lines []string `json:"lines"`
}

// ServerStats holds usage numbers for all news servers.
type ServerStats struct {
	Total   int64        `json:"total"`
	Month   int64        `json:"month"`
	Week    int64        `json:"week"`
	Day     int64        `json:"day"`
	Servers []ServerStat `json:"servers"`
}

// ServerStat holds usage numbers for one news server.
type ServerStat struct {
	Name          string `json:"name"`
	Total         int64  `json:"total"`
	Month         int64  `json:"month"`
	Week          int64  `json:"week"`
	Day           int64  `json:"day"`
	ArticlesTried int64  `json:"articles_tried"`
	ArticlesFound int64  `json:"articles_found"`
}

type sabQueueFull struct {
	Queue struct {
		Paused     bool   `json:"paused"`
		KBPerSec   string `json:"kbpersec"`
		TimeLeft   string `json:"timeleft"`
		DiskSpace1 string `json:"diskspace1"`
		DiskSpace2 string `json:"diskspace2"`
		Total      int    `json:"noofslots_total"`
		Slots      []struct {
			Index      int    `json:"index"`
			ID         string `json:"nzo_id"`
			Filename   string `json:"filename"`
			Cat        string `json:"cat"`
			MB         string `json:"mb"`
			MBLeft     string `json:"mbleft"`
			Percentage string `json:"percentage"`
			Status     string `json:"status"`
			TimeLeft   string `json:"timeleft"`
			TimeAdded  int64  `json:"time_added"`
			AvgAge     string `json:"avg_age"`
		} `json:"slots"`
	} `json:"queue"`
}

// Queue returns the current queue snapshot.
func (s *Sabnzbd) Queue(ctx context.Context) (QueuePage, error) {
	var p QueuePage
	var raw []Job
	speed := int64(0)
	for start := 0; start < sabMaxRows; start += sabPage {
		b, err := s.call(ctx, "queue", url.Values{"start": {strconv.Itoa(start)}, "limit": {strconv.Itoa(sabPage)}}, true)
		if err != nil {
			return QueuePage{}, err
		}
		var qu sabQueueFull
		if err := json.Unmarshal(b, &qu); err != nil {
			return QueuePage{}, fmt.Errorf("%w: sabnzbd queue: %v", ErrSyncParse, err)
		}
		if start == 0 {
			speed = int64(num(qu.Queue.KBPerSec) * 1024)
			p.Head = QueueHead{Paused: qu.Queue.Paused, Speed: speed, TimeLeft: clock(qu.Queue.TimeLeft), FreeSpace: freeOf(qu.Queue.DiskSpace1, qu.Queue.DiskSpace2)}
		}
		for i, x := range qu.Queue.Slots {
			mb, left := num(x.MB), num(x.MBLeft)
			pr := num(x.Percentage) / 100
			if mb > 0 {
				pr = (mb - left) / mb
			}
			pos := x.Index + 1
			if x.Index == 0 && i > 0 {
				pos = start + i + 1
			}
			raw = append(raw, Job{ID: x.ID, Name: x.Filename, Category: x.Cat, Size: int64(mb * 1024 * 1024), Progress: min(max(pr, 0), 1),
				ETA: clock(x.TimeLeft), State: sabQueueState(x.Status), Detail: x.Status, AddedAt: x.TimeAdded, QueuePos: pos, PostedAt: sabPostedAt(x.AvgAge, time.Now())})
		}
		if len(qu.Queue.Slots) < sabPage || start+len(qu.Queue.Slots) >= qu.Queue.Total {
			break
		}
	}
	moving := 0
	for _, j := range raw {
		if j.State == StateDownloading {
			moving++
		}
	}
	for i := range raw {
		if raw[i].State == StateDownloading && moving > 0 {
			raw[i].Speed = speed / int64(moving)
		}
	}
	p.Jobs = raw
	return p, nil
}

// JobStages returns the processing stages of a job.
func (s *Sabnzbd) JobStages(ctx context.Context, id string) ([]JobStage, error) {
	if id == "" || strings.Contains(id, ",") {
		return nil, ErrNotFound
	}
	slots, _, _, _, err := s.historyPage(ctx, 0, 0, id)
	if err != nil {
		return nil, err
	}
	out := []JobStage{}
	for _, x := range slots {
		if x.ID != id {
			continue
		}
		for _, st := range x.StageLog {
			js := JobStage{Stage: stageKey(st.Name), Lines: []string{}}
			for _, l := range st.Actions {
				if l = cleanLine(l); l != "" {
					js.Lines = append(js.Lines, l)
				}
			}
			out = append(out, js)
		}
	}
	return out, nil
}

// ServerStats returns usage numbers per news server.
func (s *Sabnzbd) ServerStats(ctx context.Context) (ServerStats, error) {
	b, err := s.call(ctx, "server_stats", nil, true)
	if err != nil {
		return ServerStats{}, err
	}
	var raw struct {
		Total   int64 `json:"total"`
		Month   int64 `json:"month"`
		Week    int64 `json:"week"`
		Day     int64 `json:"day"`
		Servers map[string]struct {
			Total   int64           `json:"total"`
			Month   int64           `json:"month"`
			Week    int64           `json:"week"`
			Day     int64           `json:"day"`
			Tried   json.RawMessage `json:"articles_tried"`
			Success json.RawMessage `json:"articles_success"`
		} `json:"servers"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return ServerStats{}, fmt.Errorf("sabnzbd server_stats: %w", err)
	}
	out := ServerStats{Total: raw.Total, Month: raw.Month, Week: raw.Week, Day: raw.Day, Servers: []ServerStat{}}
	for name, v := range raw.Servers {
		out.Servers = append(out.Servers, ServerStat{Name: name, Total: v.Total, Month: v.Month, Week: v.Week, Day: v.Day, ArticlesTried: countOf(v.Tried), ArticlesFound: countOf(v.Success)})
	}
	sortServers(out.Servers)
	return out, nil
}

func freeOf(d1, d2 string) int64 {
	gib := func(s string) int64 { return int64(num(s) * 1024 * 1024 * 1024) }
	free := gib(d1)
	if f2 := gib(d2); f2 > 0 && (free == 0 || f2 < free) {
		free = f2
	}
	return free
}

func (s *Sabnzbd) historyPage(ctx context.Context, start int, since int64, ids string) (slots []sabHistorySlot, last int64, total int, unchanged bool, err error) {
	q := url.Values{"start": {strconv.Itoa(start)}, "limit": {strconv.Itoa(sabPage)}}
	if since > 0 {
		q.Set("last_history_update", strconv.FormatInt(since, 10))
	}
	if ids != "" {
		q.Set("nzo_ids", ids)
	}
	b, err := s.call(ctx, "history", q, true)
	if err != nil {
		return nil, 0, 0, false, err
	}
	var wrap struct {
		History json.RawMessage `json:"history"`
	}
	if err := json.Unmarshal(b, &wrap); err != nil {
		return nil, 0, 0, false, fmt.Errorf("%w: sabnzbd history: %v", ErrSyncParse, err)
	}
	if t := bytes.TrimSpace(wrap.History); bytes.Equal(t, []byte("false")) || len(t) == 0 || bytes.Equal(t, []byte("null")) {
		return nil, since, 0, true, nil
	}
	var h struct {
		Slots      []sabHistorySlot `json:"slots"`
		LastUpdate json.RawMessage  `json:"last_history_update"`
		Total      int              `json:"noofslots"`
	}
	if err := json.Unmarshal(wrap.History, &h); err != nil {
		return nil, 0, 0, false, fmt.Errorf("%w: sabnzbd history: %v", ErrSyncParse, err)
	}
	last, err = counter(h.LastUpdate)
	if err != nil {
		return nil, 0, 0, false, fmt.Errorf("%w: sabnzbd history counter: %v", ErrSyncParse, err)
	}
	return h.Slots, last, h.Total, false, nil
}

func stageKey(name string) string {
	switch strings.ToLower(name) {
	case "rss":
		return "feed"
	case "source":
		return "source"
	case "download":
		return "download"
	case "servers":
		return "servers"
	case "repair":
		return "repair"
	case "filejoin":
		return "join"
	case "unpack":
		return "unpack"
	case "deobfuscate":
		return "rename"
	case "script":
		return "script"
	}
	return "other"
}

func cleanLine(l string) string {
	l = strings.ReplaceAll(l, "<br/>", " ")
	l = strings.ReplaceAll(l, "<br>", " ")
	l = tagRe.ReplaceAllString(l, "")
	l = passRe.ReplaceAllString(l, `$1"…"`)
	return strings.TrimSpace(redactURLs(l))
}

func countOf(raw json.RawMessage) int64 {
	var n int64
	if json.Unmarshal(raw, &n) == nil {
		return n
	}
	var days map[string]int64
	if json.Unmarshal(raw, &days) == nil {
		for _, v := range days {
			n += v
		}
	}
	return n
}

func sortServers(s []ServerStat) {
	sort.Slice(s, func(i, j int) bool {
		if s[i].Total != s[j].Total {
			return s[i].Total > s[j].Total
		}
		return s[i].Name < s[j].Name
	})
}
