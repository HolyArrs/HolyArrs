package downloads

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

const relaySessionHeader = "X-Relay-Session"

var errRelayNotFound = errors.New("no such item")

var _ Engine = (*relayEngine)(nil)

type relayItem struct {
	ID       string      `json:"id"`
	Name     string      `json:"name"`
	Label    string      `json:"label"`
	Dir      string      `json:"dir"`
	Size     int64       `json:"size"`
	Done     float64     `json:"done"`
	Rate     int64       `json:"rate"`
	Left     int64       `json:"left"`
	Status   string      `json:"status"`
	Error    string      `json:"error"`
	Contents []relayFile `json:"contents"`
}

type relayFile struct {
	Path  string  `json:"path"`
	Bytes int64   `json:"bytes"`
	Done  float64 `json:"done"`
}

type fakeRelay struct {
	mu      sync.Mutex
	user    string
	pass    string
	session string
	items   map[string]*relayItem
	adds    int
	hang    bool
}

type relayEngine struct {
	base, user, pass string
	http             *http.Client
	mu               sync.Mutex
	session          string
}

func (f *fakeRelay) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if f.hang {
		<-r.Context().Done()
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if u, p, ok := r.BasicAuth(); !ok || u != f.user || p != f.pass {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if r.Header.Get(relaySessionHeader) != f.session {
		w.Header().Set(relaySessionHeader, f.session)
		w.WriteHeader(http.StatusConflict)
		return
	}
	var call struct {
		Method string          `json:"method"`
		Args   json.RawMessage `json:"args"`
	}
	if r.URL.Path != "/rpc" || json.NewDecoder(r.Body).Decode(&call) != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	var a struct {
		ID, Link, Name, Label, Dir, Hash string
		Files                            bool
	}
	_ = json.Unmarshal(call.Args, &a)
	answer := func(v any) { _ = json.NewEncoder(w).Encode(map[string]any{"result": "ok", "value": v}) }
	switch call.Method {
	case "version":
		answer("relay 0.9")
	case "add":
		id := a.Hash
		if id == "" {
			s := sha1.Sum([]byte(a.Link))
			id = hex.EncodeToString(s[:])
		}
		if _, ok := f.items[id]; !ok {
			f.adds++
			f.items[id] = &relayItem{ID: id, Name: a.Name, Label: a.Label, Dir: a.Dir, Status: "waiting", Left: -1}
		}
		answer(map[string]string{"id": id})
	case "list":
		out := []*relayItem{}
		for _, it := range f.items {
			if a.Label == "" || it.Label == a.Label {
				c := *it
				c.Contents = nil
				out = append(out, &c)
			}
		}
		answer(out)
	case "get":
		it, ok := f.items[a.ID]
		if !ok {
			_ = json.NewEncoder(w).Encode(map[string]any{"result": "no such item"})
			return
		}
		answer(it)
	case "remove":
		if _, ok := f.items[a.ID]; !ok {
			_ = json.NewEncoder(w).Encode(map[string]any{"result": "no such item"})
			return
		}
		delete(f.items, a.ID)
		answer(nil)
	default:
		_ = json.NewEncoder(w).Encode(map[string]any{"result": "unknown method"})
	}
}

func (e *relayEngine) Health(ctx context.Context) (Health, error) {
	var v string
	if err := e.rpc(ctx, "version", nil, &v); err != nil {
		return Health{}, err
	}
	return Health{Version: v}, nil
}

func (e *relayEngine) Add(ctx context.Context, r AddRequest) (string, error) {
	var out struct {
		ID string `json:"id"`
	}
	err := e.rpc(ctx, "add", map[string]any{"link": r.URL, "name": r.Name, "label": r.Category, "dir": r.SavePath, "hash": r.InfoHash}, &out)
	if err != nil {
		return "", err
	}
	if out.ID == "" {
		return "", errors.New("relay refused the item")
	}
	return out.ID, nil
}

func (e *relayEngine) List(ctx context.Context, category string) ([]Job, error) {
	var items []relayItem
	if err := e.rpc(ctx, "list", map[string]any{"label": category}, &items); err != nil {
		return nil, err
	}
	out := make([]Job, 0, len(items))
	for _, it := range items {
		out = append(out, it.job())
	}
	return out, nil
}

func (e *relayEngine) Get(ctx context.Context, id string) (Job, error) {
	var it relayItem
	if err := e.rpc(ctx, "get", map[string]any{"id": id}, &it); err != nil {
		if errors.Is(err, errRelayNotFound) {
			return Job{}, ErrNotFound
		}
		return Job{}, err
	}
	return it.job(), nil
}

func (e *relayEngine) Remove(ctx context.Context, id string, deleteFiles bool) error {
	err := e.rpc(ctx, "remove", map[string]any{"id": id, "files": deleteFiles}, nil)
	if errors.Is(err, errRelayNotFound) {
		return ErrNotFound
	}
	return err
}

func newFakeRelay(user, pass string) *fakeRelay {
	return &fakeRelay{user: user, pass: pass, session: "s-1", items: map[string]*relayItem{}}
}

func (e *relayEngine) rpc(ctx context.Context, method string, args map[string]any, out any) error {
	body, _ := json.Marshal(map[string]any{"method": method, "args": args})
	for try := 0; try < 2; try++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.base+"/rpc", bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.SetBasicAuth(e.user, e.pass)
		e.mu.Lock()
		req.Header.Set(relaySessionHeader, e.session)
		e.mu.Unlock()
		resp, err := e.http.Do(req)
		if err != nil {
			return fmt.Errorf("relay %s: %w", method, err)
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()
		switch {
		case resp.StatusCode == http.StatusConflict && try == 0:
			e.mu.Lock()
			e.session = resp.Header.Get(relaySessionHeader)
			e.mu.Unlock()
			continue
		case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
			return ErrAuth
		case resp.StatusCode != http.StatusOK:
			return fmt.Errorf("relay %s: %s", method, resp.Status)
		}
		var env struct {
			Result string          `json:"result"`
			Value  json.RawMessage `json:"value"`
		}
		if err := json.Unmarshal(b, &env); err != nil {
			return fmt.Errorf("relay %s: %w", method, err)
		}
		if env.Result == "no such item" {
			return errRelayNotFound
		}
		if env.Result != "ok" {
			return fmt.Errorf("relay %s: %s", method, env.Result)
		}
		if out != nil {
			return json.Unmarshal(env.Value, out)
		}
		return nil
	}
	return fmt.Errorf("relay %s: the session handshake did not settle", method)
}

func relayState(it relayItem) string {
	if it.Error != "" {
		return StateFailed
	}
	switch it.Status {
	case "waiting", "checking":
		return StateQueued
	case "held":
		return StatePaused
	case "done", "sharing":
		return StateFinished
	}
	return StateDownloading
}

func (it relayItem) job() Job {
	j := Job{ID: it.ID, Name: it.Name, Category: it.Label, SavePath: it.Dir, Size: it.Size,
		Progress: min(max(it.Done, 0), 1), Speed: max(it.Rate, 0), State: relayState(it), Detail: it.Status}
	if it.Left > 0 {
		j.ETA = it.Left
	}
	if it.Error != "" {
		j.Detail = it.Error
	}
	for _, f := range it.Contents {
		j.Files = append(j.Files, File{Name: f.Path, Size: f.Bytes, Progress: f.Done})
	}
	return j
}

func relayHarness(t *testing.T) conformance {
	const pass = "relay-test-pw"
	f := newFakeRelay("holyarrs", pass)
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	at := func(base, pw string) Engine {
		return &relayEngine{base: base, user: "holyarrs", pass: pw, http: &http.Client{}}
	}
	rel := AddRequest{URL: "http://lab/relay.torrent", Name: "Lab.Relay.Movie.2024.1080p.WEB.H264-HOLYARRS", Category: "holyarrs", SavePath: "/downloads", InfoHash: hash}
	return conformance{
		name:    "relay (third engine)",
		engine:  at(srv.URL, pass),
		badKey:  at(srv.URL, "wrong-pw"),
		at:      func(base string) Engine { return at(base, pass) },
		secret:  pass,
		release: rel,
		unknown: "0000000000000000000000000000000000000000",
		appear: func() {
			f.mu.Lock()
			defer f.mu.Unlock()
			it := f.items[hash]
			it.Status, it.Done, it.Rate, it.Size, it.Left = "fetching", 0.5, 1<<20, 2<<30, 600
			it.Contents = []relayFile{{Path: "Lab.Relay.Movie.2024.1080p.WEB.H264-HOLYARRS.mkv", Bytes: 2 << 30, Done: 0.5}}
		},
		adds:   func() int { f.mu.Lock(); defer f.mu.Unlock(); return f.adds },
		states: func(st string) { f.mu.Lock(); defer f.mu.Unlock(); f.items[hash].Status = st },
		words:  map[string]string{"waiting": StateQueued, "fetching": StateDownloading, "held": StatePaused, "done": StateFinished, "brand-new-word": StateDownloading},
	}
}
