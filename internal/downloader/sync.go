package downloader

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

// ErrSyncParse is a change-feed answer (qBittorrent sync/maindata, SABnzbd
// history) that could not be read or merged. The guard counts it as a
// passing failure, never as a refusal, so a garbled answer only slows the
// next try.
var ErrSyncParse = errors.New("the Downloader's change feed could not be read")

// MainSyncer is the optional part of a torrent engine that answers
// qBittorrent's change feed: everything once (rid 0), then only what changed.
type MainSyncer interface {
	SyncMain(ctx context.Context, rid int64) (MainData, error)
}

// HistoryReader is the optional part of a Usenet engine that reads its
// finished list only when the change counter moved.
type HistoryReader interface {
	History(ctx context.Context, since int64) (HistoryPage, error)
}

// Syncer is what the background reader calls. Only guardedEngine implements
// it, so every call goes through the outside guard; an engine that lacks one
// of the parts answers ErrNotSupported without contacting the client.
type Syncer interface {
	MainSyncer
	HistoryReader
	AllLister
}

// MainData is one sync/maindata answer. Torrents holds, per hash as sent,
// only the fields that changed (all fields when FullUpdate).
type MainData struct {
	Rid         int64
	FullUpdate  bool
	Torrents    map[string]map[string]json.RawMessage
	Removed     []string
	ServerState map[string]json.RawMessage
	Bytes       int
}

// SyncMain asks for the changes since rid (0 for everything).
func (q *Qbittorrent) SyncMain(ctx context.Context, rid int64) (MainData, error) {
	b, err := q.call(ctx, http.MethodGet, "sync/maindata", url.Values{"rid": {strconv.FormatInt(rid, 10)}})
	if err != nil {
		return MainData{}, err
	}
	return parseMainData(b)
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

// MainState merges sync/maindata answers into the client's whole torrent
// list. Keys are lower-case hashes, the same as Job.ID from List.
type MainState struct {
	Rid    int64
	fields map[string]map[string]json.RawMessage
	jobs   map[string]Job
	Server map[string]json.RawMessage
}

// Apply merges one answer. A full answer replaces everything; a delta
// changes only the fields it carries. Changed reports the keys whose Job
// was rebuilt or removed.
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

// mainJob decodes one merged torrent through the shared conversion.
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
		t.Hash = hash // maindata keys torrents by hash and leaves the field out
	}
	return jobFromTorrent(t), nil
}

// Jobs returns the merged torrents, in one category ("" for all).
func (s *MainState) Jobs(category string) []Job {
	out := make([]Job, 0, len(s.jobs))
	for _, j := range s.jobs {
		if category == "" || j.Category == category {
			out = append(out, j)
		}
	}
	return out
}

// HistoryPage is SABnzbd's finished list, read with the change counter.
// Changed is false when the counter did not move (Jobs is then empty).
type HistoryPage struct {
	Last    int64
	Changed bool
	Jobs    []Job
}

// History reads the first page of the finished list when it changed since
// the counter since (0 always reads).
func (s *Sabnzbd) History(ctx context.Context, since int64) (HistoryPage, error) {
	q := url.Values{"start": {"0"}, "limit": {"200"}}
	if since > 0 {
		q.Set("last_history_update", strconv.FormatInt(since, 10))
	}
	b, err := s.call(ctx, "history", q, true)
	if err != nil {
		return HistoryPage{}, err
	}
	var hi sabHistory
	if err := json.Unmarshal(b, &hi); err != nil {
		return HistoryPage{}, fmt.Errorf("%w: sabnzbd history: %v", ErrSyncParse, err)
	}
	last, err := counter(hi.History.LastUpdate)
	if err != nil {
		return HistoryPage{}, fmt.Errorf("%w: sabnzbd history counter: %v", ErrSyncParse, err)
	}
	p := HistoryPage{Last: last, Changed: since == 0 || last != since || len(hi.History.Slots) > 0}
	for _, x := range hi.History.Slots {
		p.Jobs = append(p.Jobs, x.job())
	}
	return p, nil
}

// counter reads a number SABnzbd may send as a number or a string.
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
