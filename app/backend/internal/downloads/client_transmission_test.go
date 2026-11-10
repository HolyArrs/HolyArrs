package downloads

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
)

type fakeTransmission struct {
	mu               sync.Mutex
	token            string
	modern           bool
	adds, challenges int
	items            map[string]map[string]any
	alwaysChallenge  bool
	deleteFiles      bool
	requestedFields  []string
	bothAddResults   bool
	invalidAdded     bool
	invalidDuplicate bool
	malformedAdd     bool
	omitAddMutation  bool
	addLabel         string
	lastAdd          map[string]any
}

func (f *fakeTransmission) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if u, p, ok := r.BasicAuth(); !ok || u != "task" || p != "<transmission-test-password>" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if r.URL.Path != "/transmission/rpc" {
		http.NotFound(w, r)
		return
	}
	if f.alwaysChallenge || r.Header.Get("X-Transmission-Session-Id") != f.token {
		f.challenges++
		w.Header().Set("X-Transmission-Session-Id", f.token)
		if f.modern {
			w.Header().Set("X-Transmission-Rpc-Version", "6.0.0")
		}
		w.WriteHeader(http.StatusConflict)
		return
	}
	var call struct {
		JSONRPC   string         `json:"jsonrpc"`
		Method    string         `json:"method"`
		Arguments map[string]any `json:"arguments"`
		Params    map[string]any `json:"params"`
		ID        int            `json:"id"`
		Tag       int            `json:"tag"`
	}
	if json.NewDecoder(r.Body).Decode(&call) != nil {
		w.WriteHeader(400)
		return
	}
	a := call.Arguments
	method := call.Method
	if f.modern {
		if call.JSONRPC != "2.0" || call.Params == nil {
			w.WriteHeader(400)
			return
		}
		a = call.Params
	} else {
		method = strings.ReplaceAll(method, "-", "_")
	}
	key := func(modern, old string) string {
		if f.modern {
			return modern
		}
		return old
	}
	answer := func(v any) {
		if f.modern {
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": call.ID, "result": v})
		} else {
			_ = json.NewEncoder(w).Encode(map[string]any{"tag": call.Tag, "result": "success", "arguments": v})
		}
	}
	switch method {
	case "session_get":
		answer(map[string]any{"version": "4.1.3", key("rpc_version", "rpc-version"): 18})
	case "torrent_get":
		f.requestedFields = nil
		if fields, ok := a["fields"].([]any); ok {
			for _, field := range fields {
				if value, ok := field.(string); ok {
					f.requestedFields = append(f.requestedFields, value)
				}
			}
		}
		out := []any{}
		for id, item := range f.items {
			if ids, ok := a["ids"].([]any); ok && len(ids) > 0 && ids[0] != id {
				continue
			}
			out = append(out, item)
		}
		answer(map[string]any{"torrents": out})
	case "torrent_add":
		f.adds++
		f.lastAdd = a
		kind := key("torrent_added", "torrent-added")
		if f.items[hash] != nil {
			kind = key("torrent_duplicate", "torrent-duplicate")
		} else if !f.omitAddMutation {
			f.items[hash] = map[string]any{key("hash_string", "hashString"): hash, "name": "Owned fixture", "labels": a["labels"], key("download_dir", "downloadDir"): a[key("download_dir", "download-dir")], "status": 4, key("percent_done", "percentDone"): 0.5, key("total_size", "totalSize"): 100, key("left_until_done", "leftUntilDone"): 50}
			if f.addLabel != "" {
				f.items[hash]["labels"] = []string{f.addLabel}
			}
		}
		if f.malformedAdd {
			_, _ = w.Write([]byte("{"))
			return
		}
		result := map[string]any{kind: map[string]any{key("hash_string", "hashString"): hash, "id": 1, "name": "Owned fixture"}}
		if f.bothAddResults {
			addedHash, duplicateHash := hash, hash
			if f.invalidAdded {
				addedHash = ""
			}
			if f.invalidDuplicate {
				duplicateHash = ""
			}
			result[key("torrent_added", "torrent-added")] = map[string]any{key("hash_string", "hashString"): addedHash, "id": 1, "name": "Owned fixture"}
			result[key("torrent_duplicate", "torrent-duplicate")] = map[string]any{key("hash_string", "hashString"): duplicateHash, "id": 1, "name": "Owned fixture"}
		}
		answer(result)
	case "torrent_remove":
		ids, _ := a["ids"].([]any)
		for _, id := range ids {
			delete(f.items, id.(string))
		}
		f.deleteFiles, _ = a[key("delete_local_data", "delete-local-data")].(bool)
		answer(map[string]any{})
	default:
		w.WriteHeader(400)
	}
}

func TestTransmissionConformance(t *testing.T) {
	for _, modern := range []bool{false, true} {
		t.Run(strconv.FormatBool(modern), func(t *testing.T) { c, _ := newTransmissionHarness(t, modern); runConformance(t, c) })
	}
}

func TestTransmissionRefreshAndBoundedHandshake(t *testing.T) {
	c, f := newTransmissionHarness(t, true)
	if _, err := c.engine.Health(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.token = "<new-session>"
	f.mu.Unlock()
	if _, err := c.engine.Health(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	n := f.challenges
	f.alwaysChallenge = true
	f.mu.Unlock()
	if _, err := c.engine.Health(context.Background()); err == nil || errors.Is(err, ErrAuth) {
		t.Fatalf("repeated 409: %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.challenges-n != 2 {
		t.Fatalf("unbounded challenge retry: %d", f.challenges-n)
	}
}

func TestTransmissionDoesNotAdoptForeignCategoryOrBroadSelectors(t *testing.T) {
	c, f := newTransmissionHarness(t, true)
	if _, err := c.engine.Add(context.Background(), c.release); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.items[hash]["labels"] = []string{"personal"}
	f.mu.Unlock()
	if _, err := c.engine.Add(context.Background(), c.release); err == nil {
		t.Fatal("adopted another category's torrent")
	}
	for _, id := range []string{"", "all", hash + "|" + hash, "1"} {
		if err := c.engine.Remove(context.Background(), id, true); err == nil {
			t.Fatalf("accepted selector %q", id)
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.items) != 1 {
		t.Fatal("removed another torrent")
	}
}

func TestTransmissionRemovalKeepsPayloadUnlessRequested(t *testing.T) {
	c, f := newTransmissionHarness(t, true)
	id, err := c.engine.Add(context.Background(), c.release)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.engine.Remove(context.Background(), id, false); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.deleteFiles {
		t.Fatal("requested payload deletion")
	}
}

func TestTransmissionCompletionAndLabels(t *testing.T) {
	for _, modern := range []bool{false, true} {
		c, f := newTransmissionHarness(t, modern)
		id, err := c.engine.Add(context.Background(), c.release)
		if err != nil {
			t.Fatal(err)
		}
		e := c.engine.(*Transmission)
		e.category = "holyarrs"
		for _, tc := range []struct {
			status   int
			metadata float64
			left     int64
			recheck  float64
			want     string
		}{{6, 1, 0, 0, StateFinished}, {0, 1, 0, 1, StateFinished}, {5, 1, 0, 1, StateFinished}, {2, 1, 0, 0, StateQueued}, {1, 1, 0, 0, StateQueued}, {4, 1, 0, 0, StateDownloading}, {6, 0, 0, 0, StateDownloading}, {6, 1, 1, 0, StateDownloading}, {5, 1, 0, .5, StateDownloading}, {6, 1, 0, .5, StateDownloading}} {
			f.mu.Lock()
			item := f.items[id]
			key := func(a, b string) string {
				if modern {
					return a
				}
				return b
			}
			item["labels"] = []string{"other", "holyarrs"}
			item["status"] = tc.status
			item[key("percent_done", "percentDone")] = 1
			item[key("metadata_percent_complete", "metadataPercentComplete")] = tc.metadata
			item[key("left_until_done", "leftUntilDone")] = tc.left
			item[key("recheck_progress", "recheckProgress")] = tc.recheck
			f.mu.Unlock()
			j, err := e.Get(context.Background(), id)
			if err != nil || j.State != tc.want || j.Category != "holyarrs" {
				t.Fatalf("%+v: got %+v %v", tc, j, err)
			}
		}
		f.mu.Lock()
		key := "error_string"
		if !modern {
			key = "errorString"
		}
		f.items[id]["error"] = 3
		f.items[id][key] = "disk full\n" + strings.Repeat("x", 300)
		f.mu.Unlock()
		j, err := e.Get(context.Background(), id)
		if err != nil || j.State != StatePaused || !strings.Contains(j.Detail, "error 3: disk full") || strings.Contains(j.Detail, "\n") || len(j.Detail) > 200 {
			t.Fatalf("error detail = %+v, %v", j, err)
		}
		f.mu.Lock()
		if !slices.Contains(f.requestedFields, key) {
			f.mu.Unlock()
			t.Fatalf("torrent_get fields = %v, want %q", f.requestedFields, key)
		}
		f.items[id]["labels"] = []string{"personal"}
		f.mu.Unlock()
		if err := e.Remove(context.Background(), id, true); err == nil {
			t.Fatal("removed foreign category")
		}
	}
}

func TestTransmissionRejectsWrongResponseEnvelope(t *testing.T) {
	for _, tc := range []struct {
		name   string
		modern bool
		body   string
	}{
		{"modern missing id", true, `{"jsonrpc":"2.0","result":{}}`},
		{"modern wrong id", true, `{"jsonrpc":"2.0","id":2,"result":{}}`},
		{"modern missing jsonrpc", true, `{"id":1,"result":{}}`},
		{"modern wrong jsonrpc", true, `{"jsonrpc":"1.0","id":1,"result":{}}`},
		{"modern receives legacy", true, `{"tag":1,"result":"success","arguments":{}}`},
		{"modern mixed legacy fields", true, `{"jsonrpc":"2.0","id":1,"tag":1,"result":{}}`},
		{"modern non-object result", true, `{"jsonrpc":"2.0","id":1,"result":"success"}`},
		{"modern result and error", true, `{"jsonrpc":"2.0","id":1,"result":{},"error":{"code":1,"message":"no"}}`},
		{"modern malformed error", true, `{"jsonrpc":"2.0","id":1,"error":{"message":"no code"}}`},
		{"legacy missing tag", false, `{"result":"success","arguments":{}}`},
		{"legacy wrong tag", false, `{"tag":2,"result":"success","arguments":{}}`},
		{"legacy missing arguments", false, `{"tag":1,"result":"success"}`},
		{"legacy receives modern", false, `{"jsonrpc":"2.0","id":1,"result":{}}`},
		{"legacy mixed modern fields", false, `{"id":1,"tag":1,"result":"success","arguments":{}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(tc.body))
			}))
			defer s.Close()
			tr := NewTransmission(s.URL, "", "", nil)
			tr.session, tr.modern = "owned", tc.modern
			var out map[string]any
			if err := tr.call(context.Background(), "session-get", nil, &out); err == nil {
				t.Fatal("accepted invalid response envelope")
			}
		})
	}
}

func TestTransmissionRejectsAmbiguousAddIdentity(t *testing.T) {
	for _, modern := range []bool{false, true} {
		for _, tc := range []struct {
			name             string
			invalidAdded     bool
			invalidDuplicate bool
		}{
			{name: "both valid"},
			{name: "invalid added", invalidAdded: true},
			{name: "invalid duplicate", invalidDuplicate: true},
		} {
			t.Run(strconv.FormatBool(modern)+"/"+tc.name, func(t *testing.T) {
				c, f := newTransmissionHarness(t, modern)
				f.bothAddResults = true
				f.invalidAdded = tc.invalidAdded
				f.invalidDuplicate = tc.invalidDuplicate
				if _, err := c.engine.Add(context.Background(), c.release); !errors.Is(err, ErrUncertain) {
					t.Fatalf("ambiguous add error = %v, want ErrUncertain", err)
				}
			})
		}
	}
}

func TestTransmissionReconcileOnlyNeverAddsOrFetches(t *testing.T) {
	for _, tc := range []struct {
		name string
		item map[string]any
		skip bool
		want string
	}{
		{name: "owned", item: map[string]any{"hash_string": hash, "name": "owned", "labels": []string{"holyarrs"}}, want: hash},
		{name: "missing"},
		{name: "foreign", item: map[string]any{"hash_string": hash, "name": "foreign", "labels": []string{"other"}}},
		{name: "skip", item: map[string]any{"hash_string": hash, "name": "owned", "labels": []string{"holyarrs"}}, skip: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, f := newTransmissionHarness(t, true)
			if tc.item != nil {
				f.items[hash] = tc.item
			}
			r := c.release
			r.URL = "http://127.0.0.1:1/must-not-fetch.torrent"
			r.ReconcileOnly = true
			r.Skip = map[string]bool{hash: tc.skip}
			got, err := c.engine.Add(context.Background(), r)
			if tc.want != "" {
				if err != nil || got != tc.want {
					t.Fatalf("Add = %q, %v", got, err)
				}
			} else if !errors.Is(err, ErrUncertain) {
				t.Fatalf("Add error = %v, want ErrUncertain", err)
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.adds != 0 {
				t.Fatalf("adds = %d, want 0", f.adds)
			}
		})
	}

	c, f := newTransmissionHarness(t, true)
	r := c.release
	r.InfoHash = ""
	r.ReconcileOnly = true
	if _, err := c.engine.Add(context.Background(), r); !errors.Is(err, ErrUncertain) {
		t.Fatalf("blank hash error = %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.items) != 0 || f.adds != 0 {
		t.Fatal("blank reconciliation contacted an add path")
	}
}

func TestTransmissionUnknownAddOutcomeDoesNotPermitFailover(t *testing.T) {
	for _, tc := range []struct {
		name  string
		omit  bool
		label string
	}{
		{name: "missing", omit: true},
		{name: "foreign", label: "other"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, f := newTransmissionHarness(t, true)
			f.malformedAdd, f.omitAddMutation, f.addLabel = true, tc.omit, tc.label
			if _, err := c.engine.Add(context.Background(), c.release); !errors.Is(err, ErrUncertain) {
				t.Fatalf("Add error = %v, want ErrUncertain", err)
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.adds != 1 {
				t.Fatalf("adds = %d, want 1", f.adds)
			}
		})
	}
}

func TestTransmissionConcurrentRefresh(t *testing.T) {
	c, f := newTransmissionHarness(t, true)
	if _, err := c.engine.Health(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.token = "<rotated-session>"
	f.mu.Unlock()
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.engine.Health(context.Background()); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
}

func newTransmissionHarness(t *testing.T, modern bool) (conformance, *fakeTransmission) {
	t.Helper()
	f := &fakeTransmission{token: "<owned-session>", modern: modern, items: map[string]map[string]any{}}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	makeEngine := func(base, pass string) *Transmission { return NewTransmission(base, "task", pass, nil) }
	c := conformance{name: "transmission", engine: makeEngine(srv.URL, "<transmission-test-password>"), badKey: makeEngine(srv.URL, "wrong"), at: func(base string) Engine { return makeEngine(base, "<transmission-test-password>") }, secret: "<transmission-test-password>",
		release: AddRequest{URL: "magnet:?xt=urn:btih:" + hash, Name: "Owned fixture", Category: "holyarrs", SavePath: "/downloads", InfoHash: hash}, unknown: strings.Repeat("0", 40),
		appear: func() {
			f.mu.Lock()
			defer f.mu.Unlock()
			k := func(a, b string) string {
				if modern {
					return a
				}
				return b
			}
			f.items[hash]["files"] = []any{map[string]any{"name": "owned.bin", "length": 100, k("bytes_completed", "bytesCompleted"): 50}}
		},
		adds: func() int { f.mu.Lock(); defer f.mu.Unlock(); return f.adds },
		states: func(word string) {
			f.mu.Lock()
			defer f.mu.Unlock()
			s, _ := strconv.Atoi(word)
			f.items[hash]["status"] = s
			f.items[hash]["error"] = 0
			if word == "error" {
				f.items[hash]["error"] = 3
			}
		},
		words: map[string]string{"0": StatePaused, "1": StateQueued, "2": StateQueued, "3": StateQueued, "4": StateDownloading, "99": StateDownloading, "error": StatePaused},
	}
	return c, f
}

func transmissionHarness(t *testing.T) conformance { c, _ := newTransmissionHarness(t, true); return c }
