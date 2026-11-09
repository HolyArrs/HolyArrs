package downloads

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// ErrSyncParse is returned when a change feed cannot be parsed.
var ErrSyncParse = errors.New("the Downloader's change feed could not be read")

// MainSyncer reads incremental torrent changes from an engine.
type MainSyncer interface {
	SyncMain(ctx context.Context, rid int64) (MainData, error)
}

// HistoryReader reads finished jobs from an engine.
type HistoryReader interface {
	History(ctx context.Context, since int64) (HistoryPage, error)
}

// Syncer is an engine that can feed a sync loop.
type Syncer interface {
	MainSyncer
	HistoryReader
	QueueReader
	AllLister
}

// MainData is one change feed reply.
type MainData struct {
	Rid         int64
	FullUpdate  bool
	Torrents    map[string]map[string]json.RawMessage
	Removed     []string
	ServerState map[string]json.RawMessage
	Bytes       int
}

// MainState is the job set built up from change feed replies.
type MainState struct {
	Rid    int64
	fields map[string]map[string]json.RawMessage
	jobs   map[string]Job
	Server map[string]json.RawMessage
}

// HistoryPage is one page of finished jobs.
type HistoryPage struct {
	Last    int64
	Changed bool
	Jobs    []Job
}

// SyncMain fetches changes since rid.
func (q *Qbittorrent) SyncMain(ctx context.Context, rid int64) (MainData, error) {
	b, err := q.call(ctx, http.MethodGet, "sync/maindata", url.Values{"rid": {strconv.FormatInt(rid, 10)}})
	if err != nil {
		return MainData{}, err
	}
	return parseMainData(b)
}

// Apply merges d into the state and returns the changed job ids.
func (s *MainState) Apply(d MainData) (changed []string, err error) {
	if d.FullUpdate || s.fields == nil {
		for k := range s.jobs {
			changed = append(changed, k)
		}
		s.fields, s.jobs, s.Server = map[string]map[string]json.RawMessage{}, map[string]Job{}, map[string]json.RawMessage{}
	}
	for h, f := range d.Torrents {
		k := strings.ToLower(h)
		cur := s.fields[k]
		if cur == nil {
			cur = make(map[string]json.RawMessage, len(f))
			s.fields[k] = cur
		}
		for name, v := range f {
			cur[name] = v
		}
		j, err := mainJob(k, cur)
		if err != nil {
			return changed, err
		}
		s.jobs[k] = j
		changed = append(changed, k)
	}
	for _, h := range d.Removed {
		k := strings.ToLower(h)
		delete(s.fields, k)
		delete(s.jobs, k)
		changed = append(changed, k)
	}
	for name, v := range d.ServerState {
		s.Server[name] = v
	}
	s.Rid = d.Rid
	return changed, nil
}

// Jobs returns the jobs in category.
func (s *MainState) Jobs(category string) []Job {
	out := make([]Job, 0, len(s.jobs))
	for _, j := range s.jobs {
		if category == "" || j.Category == category {
			out = append(out, j)
		}
	}
	return out
}

// History returns finished jobs since the given time.
func (s *Sabnzbd) History(ctx context.Context, since int64) (HistoryPage, error) {
	slots, last, total, unchanged, err := s.historyPage(ctx, 0, since, "")
	if err != nil {
		return HistoryPage{}, err
	}
	if unchanged {
		return HistoryPage{Last: since}, nil
	}
	p := HistoryPage{Last: last, Changed: since == 0 || last != since || len(slots) > 0}
	if !p.Changed {
		return p, nil
	}
	for start := len(slots); len(slots) == start && start < total && start < sabMaxRows; start = len(slots) {
		more, _, _, _, err := s.historyPage(ctx, start, 0, "")
		if err != nil {
			return HistoryPage{}, err
		}
		if len(more) == 0 {
			break
		}
		slots = append(slots, more...)
	}
	for _, x := range slots {
		p.Jobs = append(p.Jobs, x.job())
	}
	return p, nil
}

func parseMainData(b []byte) (MainData, error) {
	var raw struct {
		Rid      int64                                 `json:"rid"`
		Full     bool                                  `json:"full_update"`
		Torrents map[string]map[string]json.RawMessage `json:"torrents"`
		Removed  []string                              `json:"torrents_removed"`
		Server   map[string]json.RawMessage            `json:"server_state"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return MainData{}, fmt.Errorf("%w: qbittorrent sync/maindata: %v", ErrSyncParse, err)
	}
	return MainData{Rid: raw.Rid, FullUpdate: raw.Full, Torrents: raw.Torrents, Removed: raw.Removed, ServerState: raw.Server, Bytes: len(b)}, nil
}

func mainJob(hash string, f map[string]json.RawMessage) (Job, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	first := true
	for name, v := range f {
		if !first {
			buf.WriteByte(',')
		}
		first = false
		n, _ := json.Marshal(name)
		buf.Write(n)
		buf.WriteByte(':')
		buf.Write(v)
	}
	buf.WriteByte('}')
	var t qbitTorrent
	if err := json.Unmarshal(buf.Bytes(), &t); err != nil {
		return Job{}, fmt.Errorf("%w: qbittorrent sync/maindata torrent: %v", ErrSyncParse, err)
	}
	if t.Hash == "" {
		t.Hash = hash
	}
	return jobFromTorrent(t), nil
}

func counter(raw json.RawMessage) (int64, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, nil
	}
	var n json.Number
	if err := json.Unmarshal(raw, &n); err == nil {
		return n.Int64()
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return 0, err
	}
	if s == "" {
		return 0, nil
	}
	return strconv.ParseInt(s, 10, 64)
}
