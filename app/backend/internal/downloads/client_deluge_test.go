package downloads

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

const delugeHash = "0123456789abcdef0123456789abcdef01234567"

type delugeRPCRequest struct {
	Method string `json:"method"`
	Params []any  `json:"params"`
	ID     int64  `json:"id"`
}

type fakeDeluge struct {
	mu sync.Mutex

	password        string
	connected       bool
	labels          map[string]bool
	torrents        map[string]map[string]any
	added           int
	removed         []string
	calls           []delugeRPCRequest
	session         string
	logins          int
	loginWait       <-chan struct{}
	addHash         string
	ignoreLabel     bool
	malformedAdd    bool
	omitAddMutation bool
	addLabel        string
	emptyAddResult  bool
}

func TestDelugeConformance(t *testing.T) {
	runConformance(t, delugeHarness(t))
}

func TestDelugeRefusesDetachedWebDaemon(t *testing.T) {
	f := &fakeDeluge{connected: false}
	d := NewDeluge(startDeluge(t, f), f.password, nil)
	if _, err := d.Health(context.Background()); err == nil || !strings.Contains(strings.ToLower(err.Error()), "daemon") {
		t.Fatalf("detached web server health error = %v, want an explicit daemon error", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, call := range f.calls {
		if call.Method == "web.get_hosts" || call.Method == "web.connect" {
			t.Fatalf("adapter silently selected a daemon with %s", call.Method)
		}
	}
}

func TestDelugeRequiresLabelPlugin(t *testing.T) {
	f := &fakeDeluge{connected: true, labels: nil}
	base := startDeluge(t, f)
	f.mu.Lock()
	f.labels = nil
	f.mu.Unlock()
	if _, err := NewDeluge(base, f.password, nil).Health(context.Background()); err == nil || !strings.Contains(strings.ToLower(err.Error()), "label") {
		t.Fatalf("health without Label plugin = %v, want a label error", err)
	}
}

func TestDelugeTorrentURLIsFetchedByHolyArrs(t *testing.T) {
	payload := []byte("d4:infod6:lengthi1e4:name7:lab.mkvee")
	hash, err := torrentFileHash(payload)
	if err != nil {
		t.Fatal(err)
	}
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(payload) }))
	t.Cleanup(source.Close)
	f := &fakeDeluge{connected: true, addHash: hash}
	d := NewDeluge(startDeluge(t, f), f.password, nil)
	if _, err := d.Add(context.Background(), AddRequest{URL: source.URL + "/lab.torrent", Name: "Lab Torrent", Category: "movies", InfoHash: hash, AllowLocal: true}); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var fileCall *delugeRPCRequest
	for i := range f.calls {
		if f.calls[i].Method == "core.add_torrent_url" {
			t.Fatal("Deluge was asked to fetch the torrent URL itself")
		}
		if f.calls[i].Method == "core.add_torrent_file" {
			fileCall = &f.calls[i]
		}
	}
	if fileCall == nil || len(fileCall.Params) < 2 {
		t.Fatalf("calls = %#v, want core.add_torrent_file", f.calls)
	}
	encoded, _ := fileCall.Params[1].(string)
	got, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || string(got) != string(payload) {
		t.Fatalf("uploaded torrent = %q, %v", got, err)
	}
}

func TestDelugeTorrentURLUsesOutgoingPolicy(t *testing.T) {
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("not reached")) }))
	t.Cleanup(source.Close)
	f := &fakeDeluge{connected: true}
	d := NewDeluge(startDeluge(t, f), f.password, nil)
	if _, err := d.Add(context.Background(), AddRequest{URL: source.URL + "/private.torrent", Category: "movies", InfoHash: delugeHash}); err == nil {
		t.Fatal("private torrent URL was fetched without AllowLocal")
	}
}

func TestDelugeTorrentFileRejectsHashMismatch(t *testing.T) {
	payload := []byte("d4:infod6:lengthi1e4:name7:lab.mkvee")
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(payload) }))
	t.Cleanup(source.Close)
	f := &fakeDeluge{connected: true}
	d := NewDeluge(startDeluge(t, f), f.password, nil)
	if _, err := d.Add(context.Background(), AddRequest{URL: source.URL + "/lab.torrent", Category: "movies", InfoHash: delugeHash, AllowLocal: true}); err == nil {
		t.Fatal("torrent file with a different info hash was added")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.added != 0 {
		t.Fatalf("adds = %d, want 0", f.added)
	}
}

func TestDelugeSafeRemoveID(t *testing.T) {
	f := &fakeDeluge{connected: true, torrents: map[string]map[string]any{delugeHash: fakeDelugeTorrent("Downloading", 25)}}
	d := NewDeluge(startDeluge(t, f), f.password, nil)
	for _, id := range []string{"", "all", delugeHash + "|" + delugeHash, "../" + delugeHash} {
		if err := d.Remove(context.Background(), id, true); err == nil {
			t.Errorf("Remove(%q) gave no error", id)
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.removed) != 0 {
		t.Fatalf("unsafe ids removed %v", f.removed)
	}
}

func TestDelugeRemoveStaysInsideConfiguredCategory(t *testing.T) {
	torrent := fakeDelugeTorrent("Downloading", 25)
	torrent["label"] = "another-app"
	f := &fakeDeluge{connected: true, labels: map[string]bool{"movies": true, "another-app": true}, torrents: map[string]map[string]any{delugeHash: torrent}}
	d := NewDeluge(startDeluge(t, f), f.password, nil)
	d.Category = "movies"
	if err := d.Remove(context.Background(), delugeHash, true); err == nil {
		t.Fatal("Remove deleted a torrent outside the configured category")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.removed) != 0 || f.torrents[delugeHash] == nil {
		t.Fatalf("foreign torrent was removed: removed=%v present=%t", f.removed, f.torrents[delugeHash] != nil)
	}
}

func TestDelugeTransportErrorsAreClassifiableAndRedacted(t *testing.T) {
	t.Run("http", func(t *testing.T) {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "provider-secret-detail", http.StatusServiceUnavailable)
		}))
		defer s.Close()
		_, err := NewDeluge(s.URL, "password", nil).Health(context.Background())
		var status adapterHTTPError
		if !errors.As(err, &status) || status.status != http.StatusServiceUnavailable {
			t.Fatalf("Health error = %v, want typed HTTP 503", err)
		}
		if strings.Contains(err.Error(), "provider-secret-detail") {
			t.Fatalf("Health error exposed provider response: %v", err)
		}
	})

	t.Run("network", func(t *testing.T) {
		s := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		base := s.URL
		s.Close()
		_, err := NewDeluge(base, "password", &http.Client{Timeout: time.Second}).Health(context.Background())
		if !errors.Is(err, errDownloaderUnavailable) {
			t.Fatalf("Health error = %v, want downloader unavailable", err)
		}
	})

	t.Run("rpc message", func(t *testing.T) {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writeDelugeRPC(w, 1, nil, map[string]any{"code": 42, "message": "provider-secret-detail"})
		}))
		defer s.Close()
		_, err := NewDeluge(s.URL, "password", nil).Health(context.Background())
		if err == nil || strings.Contains(err.Error(), "provider-secret-detail") {
			t.Fatalf("Health error was not safely redacted: %v", err)
		}
	})
}

func TestDelugeRejectsInvalidResponseFraming(t *testing.T) {
	for _, tc := range []struct {
		name string
		body func(int64) map[string]any
	}{
		{"missing id", func(int64) map[string]any { return map[string]any{"result": true, "error": nil} }},
		{"wrong id", func(id int64) map[string]any { return map[string]any{"id": id + 1, "result": true, "error": nil} }},
		{"missing result", func(id int64) map[string]any { return map[string]any{"id": id, "error": nil} }},
		{"missing error", func(id int64) map[string]any { return map[string]any{"id": id, "result": true} }},
		{"result and error", func(id int64) map[string]any {
			return map[string]any{"id": id, "result": true, "error": map[string]any{"code": 3, "message": "bad"}}
		}},
		{"malformed error", func(id int64) map[string]any {
			return map[string]any{"id": id, "result": nil, "error": map[string]any{}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var q delugeRPCRequest
				_ = json.NewDecoder(r.Body).Decode(&q)
				_ = json.NewEncoder(w).Encode(tc.body(q.ID))
			}))
			defer s.Close()
			var out bool
			if err := NewDeluge(s.URL, "password", nil).rpcOnce(context.Background(), "web.connected", []any{}, &out); err == nil {
				t.Fatal("accepted invalid response framing")
			}
		})
	}
}

func TestDelugeLabelFailurePreservesBothCauses(t *testing.T) {
	t.Run("auth", func(t *testing.T) {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var q delugeRPCRequest
			_ = json.NewDecoder(r.Body).Decode(&q)
			switch q.Method {
			case "web.connected":
				writeDelugeRPC(w, q.ID, true, nil)
			case "label.get_labels":
				writeDelugeRPC(w, q.ID, nil, map[string]any{"code": 1, "message": "sign in"})
			case "auth.login":
				writeDelugeRPC(w, q.ID, false, nil)
			}
		}))
		defer s.Close()
		_, err := NewDeluge(s.URL, "wrong", nil).ready(context.Background())
		if !errors.Is(err, errDelugeLabel) || !errors.Is(err, ErrAuth) {
			t.Fatalf("ready error = %v", err)
		}
	})

	t.Run("transient", func(t *testing.T) {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var q delugeRPCRequest
			_ = json.NewDecoder(r.Body).Decode(&q)
			if q.Method == "web.connected" {
				writeDelugeRPC(w, q.ID, true, nil)
				return
			}
			h, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("server does not support hijacking")
			}
			conn, _, err := h.Hijack()
			if err == nil {
				_ = conn.Close()
			}
		}))
		defer s.Close()
		_, err := NewDeluge(s.URL, "password", nil).ready(context.Background())
		if !errors.Is(err, errDelugeLabel) || !errors.Is(err, errDownloaderUnavailable) {
			t.Fatalf("ready error = %v", err)
		}
	})
}

func TestDelugeAddIsIdempotentWithoutRelabelingForeignTorrent(t *testing.T) {
	for _, tc := range []struct {
		name     string
		label    string
		wantErr  bool
		wantAdds int
	}{
		{name: "same category", label: "movies"},
		{name: "foreign category", label: "another-app", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			torrent := fakeDelugeTorrent("Downloading", 25)
			torrent["label"] = tc.label
			f := &fakeDeluge{connected: true, labels: map[string]bool{"movies": true, "another-app": true}, torrents: map[string]map[string]any{delugeHash: torrent}}
			d := NewDeluge(startDeluge(t, f), f.password, nil)
			id, err := d.Add(context.Background(), AddRequest{
				URL: "magnet:?xt=urn:btih:" + delugeHash, InfoHash: delugeHash, Category: "movies",
			})
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Add returned %q without an error", id)
				}
			} else if err != nil || id != delugeHash {
				t.Fatalf("Add = %q, %v", id, err)
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.added != tc.wantAdds {
				t.Fatalf("adds = %d, want %d", f.added, tc.wantAdds)
			}
			if got := f.torrents[delugeHash]["label"]; got != tc.label {
				t.Fatalf("label = %q, want original %q", got, tc.label)
			}
		})
	}
}

func TestDelugeAddDoesNotSucceedWhileTorrentIsUnlabelled(t *testing.T) {
	f := &fakeDeluge{connected: true, ignoreLabel: true}
	d := NewDeluge(startDeluge(t, f), f.password, nil)
	if id, err := d.Add(context.Background(), AddRequest{
		URL: "magnet:?xt=urn:btih:" + delugeHash, InfoHash: delugeHash, Category: "movies",
	}); !errors.Is(err, ErrUncertain) {
		t.Fatalf("Add returned %q while Deluge ignored the label", id)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if got := f.torrents[delugeHash]["label"]; got != "" {
		t.Fatalf("label = %q, want empty", got)
	}
	sets := 0
	for _, call := range f.calls {
		if call.Method == "label.set_torrent" {
			sets++
		}
	}
	if sets != 1 {
		t.Fatalf("label mutations = %d, want 1", sets)
	}
}

func TestDelugeReconcileOnlyNeverMutatesOrFetches(t *testing.T) {
	for _, tc := range []struct {
		name  string
		label string
		skip  bool
		want  string
	}{
		{name: "owned", label: "movies", want: delugeHash},
		{name: "missing"},
		{name: "foreign", label: "other"},
		{name: "skip", label: "movies", skip: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeDeluge{connected: true}
			if tc.label != "" {
				torrent := fakeDelugeTorrent("Downloading", 10)
				torrent["label"] = tc.label
				f.torrents = map[string]map[string]any{delugeHash: torrent}
			}
			d := NewDeluge(startDeluge(t, f), f.password, nil)
			got, err := d.Add(context.Background(), AddRequest{
				URL: "http://127.0.0.1:1/must-not-fetch.torrent", InfoHash: delugeHash,
				Category: "movies", ReconcileOnly: true, Skip: map[string]bool{delugeHash: tc.skip},
			})
			if tc.want != "" {
				if err != nil || got != tc.want {
					t.Fatalf("Add = %q, %v", got, err)
				}
			} else if !errors.Is(err, ErrUncertain) {
				t.Fatalf("Add error = %v, want ErrUncertain", err)
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			for _, call := range f.calls {
				if strings.HasPrefix(call.Method, "core.add_torrent") || call.Method == "label.add" || call.Method == "label.set_torrent" {
					t.Fatalf("reconciliation mutated through %s", call.Method)
				}
			}
		})
	}

	f := &fakeDeluge{connected: true}
	d := NewDeluge(startDeluge(t, f), f.password, nil)
	if _, err := d.Add(context.Background(), AddRequest{Category: "movies", ReconcileOnly: true}); !errors.Is(err, ErrUncertain) {
		t.Fatalf("blank hash error = %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) != 0 {
		t.Fatalf("blank reconciliation calls = %#v", f.calls)
	}
}

func TestDelugeSkipPreventsKnownAndFileDerivedOwnership(t *testing.T) {
	t.Run("known", func(t *testing.T) {
		torrent := fakeDelugeTorrent("Downloading", 10)
		f := &fakeDeluge{connected: true, torrents: map[string]map[string]any{delugeHash: torrent}}
		d := NewDeluge(startDeluge(t, f), f.password, nil)
		_, err := d.Add(context.Background(), AddRequest{URL: "magnet:?xt=urn:btih:" + delugeHash, InfoHash: delugeHash, Category: "movies", Skip: map[string]bool{delugeHash: true}})
		if err == nil || errors.Is(err, ErrUncertain) {
			t.Fatalf("Add error = %v, want definite ownership refusal", err)
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.added != 0 {
			t.Fatalf("adds = %d, want 0", f.added)
		}
	})

	t.Run("file derived", func(t *testing.T) {
		payload := []byte("d4:infod6:lengthi1e4:name7:lab.mkvee")
		derived, err := torrentFileHash(payload)
		if err != nil {
			t.Fatal(err)
		}
		source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(payload) }))
		defer source.Close()
		f := &fakeDeluge{connected: true, addHash: derived}
		d := NewDeluge(startDeluge(t, f), f.password, nil)
		_, err = d.Add(context.Background(), AddRequest{URL: source.URL + "/release.torrent", Category: "movies", AllowLocal: true, Skip: map[string]bool{derived: true}})
		if err == nil || errors.Is(err, ErrUncertain) {
			t.Fatalf("Add error = %v, want definite ownership refusal", err)
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.added != 0 {
			t.Fatalf("adds = %d, want 0", f.added)
		}
	})
}

func TestDelugeUnknownAddOutcomeDoesNotPermitFailover(t *testing.T) {
	for _, tc := range []struct {
		name  string
		omit  bool
		label string
	}{
		{name: "missing", omit: true},
		{name: "foreign", label: "other"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeDeluge{connected: true, malformedAdd: true, omitAddMutation: tc.omit, addLabel: tc.label}
			d := NewDeluge(startDeluge(t, f), f.password, nil)
			_, err := d.Add(context.Background(), AddRequest{URL: "magnet:?xt=urn:btih:" + delugeHash, InfoHash: delugeHash, Category: "movies"})
			if !errors.Is(err, ErrUncertain) {
				t.Fatalf("Add error = %v, want ErrUncertain", err)
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.added != 1 {
				t.Fatalf("adds = %d, want 1", f.added)
			}
			for _, call := range f.calls {
				if call.Method == "label.set_torrent" {
					t.Fatal("unknown add response caused label mutation")
				}
			}
		})
	}
}

func TestDelugeEmptyAddIdentityOnlyReconciles(t *testing.T) {
	for _, tc := range []struct {
		name string
		omit bool
	}{
		{name: "missing", omit: true},
		{name: "unlabelled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeDeluge{connected: true, emptyAddResult: true, omitAddMutation: tc.omit}
			d := NewDeluge(startDeluge(t, f), f.password, nil)
			_, err := d.Add(context.Background(), AddRequest{URL: "magnet:?xt=urn:btih:" + delugeHash, InfoHash: delugeHash, Category: "movies"})
			if !errors.Is(err, ErrUncertain) {
				t.Fatalf("Add error = %v, want ErrUncertain", err)
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.added != 1 {
				t.Fatalf("adds = %d, want 1", f.added)
			}
			for _, call := range f.calls {
				if call.Method == "label.set_torrent" {
					t.Fatal("empty add identity caused label mutation")
				}
			}
		})
	}
}

func TestDelugeFinishedNeedsCompleteProgressAndFinalPath(t *testing.T) {
	for _, tc := range []struct {
		name     string
		progress float64
		savePath string
		want     string
	}{
		{name: "complete", progress: 100, savePath: "/downloads", want: StateFinished},
		{name: "still processing", progress: 99, savePath: "/downloads", want: StateDownloading},
		{name: "missing final path", progress: 100, want: StateDownloading},
	} {
		t.Run(tc.name, func(t *testing.T) {
			torrent := fakeDelugeTorrent("Seeding", tc.progress)
			torrent["is_finished"] = true
			torrent["save_path"] = tc.savePath
			f := &fakeDeluge{connected: true, torrents: map[string]map[string]any{delugeHash: torrent}}
			job, err := NewDeluge(startDeluge(t, f), f.password, nil).Get(context.Background(), delugeHash)
			if err != nil || job.State != tc.want {
				t.Fatalf("Get state = %q, %v, want %q", job.State, err, tc.want)
			}
		})
	}
}

func TestDelugeSessionRefreshLockHonorsContext(t *testing.T) {
	f := &fakeDeluge{connected: true}
	d := NewDeluge(startDeluge(t, f), f.password, nil)
	if _, err := d.Health(context.Background()); err != nil {
		t.Fatal(err)
	}

	block := make(chan struct{})
	f.mu.Lock()
	f.session = "expired"
	f.loginWait = block
	f.mu.Unlock()
	firstDone := make(chan error, 1)
	go func() {
		_, err := d.Health(context.Background())
		firstDone <- err
	}()
	deadline := time.Now().Add(2 * time.Second)
	for {
		f.mu.Lock()
		logins := f.logins
		f.mu.Unlock()
		if logins >= 2 || time.Now().After(deadline) {
			break
		}
		time.Sleep(time.Millisecond)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := d.Health(ctx)
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(started) > time.Second {
		t.Fatalf("waiting refresh = %v after %v, want caller deadline", err, time.Since(started))
	}
	close(block)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
}

func TestDelugeFinishedQueuedOrPausedIsFinished(t *testing.T) {
	for _, w := range []string{"Queued", "Paused", "Seeding"} {
		if got := delugeState(w, true); got != StateFinished {
			t.Fatalf("%s+finished = %s", w, got)
		}
	}
	if got := delugeState("Checking", true); got != StateQueued {
		t.Fatalf("checking+finished = %s", got)
	}
	if got := delugeState("Error", true); got != StatePaused {
		t.Fatalf("error+finished = %s", got)
	}
}

func startDeluge(t *testing.T, f *fakeDeluge) string {
	t.Helper()
	if f.password == "" {
		f.password = "<deluge-test-password>"
	}
	if f.labels == nil {
		f.labels = map[string]bool{"movies": true}
	}
	if f.torrents == nil {
		f.torrents = make(map[string]map[string]any)
	}
	s := httptest.NewServer(http.HandlerFunc(f.serveHTTP))
	t.Cleanup(s.Close)
	return s.URL
}

func (f *fakeDeluge) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Path != "/json" {
		http.NotFound(w, r)
		return
	}
	var q delugeRPCRequest
	if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	f.calls = append(f.calls, q)
	f.mu.Unlock()

	if q.Method == "auth.login" {
		f.mu.Lock()
		f.logins++
		wait := f.loginWait
		f.mu.Unlock()
		if wait != nil {
			select {
			case <-r.Context().Done():
				return
			case <-wait:
			}
		}
		pass, _ := q.Params[0].(string)
		ok := pass == f.password
		if ok {
			f.mu.Lock()
			f.session = "session-" + time.Now().UTC().Format("150405.000000000")
			session := f.session
			f.mu.Unlock()
			http.SetCookie(w, &http.Cookie{Name: "_session_id", Value: session, Path: "/", HttpOnly: true})
		}
		writeDelugeRPC(w, q.ID, ok, nil)
		return
	}

	cookie, _ := r.Cookie("_session_id")
	f.mu.Lock()
	authed := cookie != nil && cookie.Value != "" && cookie.Value == f.session
	f.mu.Unlock()
	if !authed {
		writeDelugeRPC(w, q.ID, nil, map[string]any{"code": 1, "message": "Not authenticated", "traceback": ""})
		return
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	switch q.Method {
	case "web.connected":
		writeDelugeRPC(w, q.ID, f.connected, nil)
	case "daemon.get_version":
		writeDelugeRPC(w, q.ID, "2.2.0", nil)
	case "label.get_labels":
		if f.labels == nil {
			writeDelugeRPC(w, q.ID, nil, map[string]any{"code": 3, "message": "Unknown method", "traceback": ""})
			return
		}
		labels := make([]string, 0, len(f.labels))
		for label := range f.labels {
			labels = append(labels, label)
		}
		writeDelugeRPC(w, q.ID, labels, nil)
	case "label.add":
		label, _ := q.Params[0].(string)
		f.labels[label] = true
		writeDelugeRPC(w, q.ID, nil, nil)
	case "label.set_torrent":
		hash, _ := q.Params[0].(string)
		label, _ := q.Params[1].(string)
		if torrent := f.torrents[hash]; torrent != nil && !f.ignoreLabel {
			torrent["label"] = label
		}
		writeDelugeRPC(w, q.ID, nil, nil)
	case "core.add_torrent_magnet":
		f.added++
		hash := f.addHash
		if hash == "" {
			hash = delugeHash
		}
		if f.torrents[hash] == nil && !f.omitAddMutation {
			f.torrents[hash] = fakeDelugeTorrent("Downloading", 0)
			f.torrents[hash]["label"] = f.addLabel
		}
		if f.malformedAdd {
			_, _ = w.Write([]byte("{"))
			return
		}
		if f.emptyAddResult {
			hash = ""
		}
		writeDelugeRPC(w, q.ID, hash, nil)
	case "core.add_torrent_file":
		f.added++
		hash := f.addHash
		if hash == "" {
			hash = delugeHash
		}
		if f.torrents[hash] == nil && !f.omitAddMutation {
			f.torrents[hash] = fakeDelugeTorrent("Downloading", 0)
			f.torrents[hash]["label"] = f.addLabel
		}
		if f.malformedAdd {
			_, _ = w.Write([]byte("{"))
			return
		}
		if f.emptyAddResult {
			hash = ""
		}
		writeDelugeRPC(w, q.ID, hash, nil)
	case "core.get_torrents_status":
		result := make(map[string]map[string]any)
		for hash, torrent := range f.torrents {
			result[hash] = cloneAnyMap(torrent)
		}
		writeDelugeRPC(w, q.ID, result, nil)
	case "core.get_torrent_status":
		hash, _ := q.Params[0].(string)
		if torrent := f.torrents[hash]; torrent != nil {
			writeDelugeRPC(w, q.ID, cloneAnyMap(torrent), nil)
		} else {
			writeDelugeRPC(w, q.ID, map[string]any{}, nil)
		}
	case "web.get_torrent_files":
		hash, _ := q.Params[0].(string)
		if torrent := f.torrents[hash]; torrent != nil {
			writeDelugeRPC(w, q.ID, torrent["files"], nil)
		} else {
			writeDelugeRPC(w, q.ID, []any{}, nil)
		}
	case "core.remove_torrent":
		hash, _ := q.Params[0].(string)
		if _, ok := f.torrents[hash]; !ok {
			writeDelugeRPC(w, q.ID, false, nil)
			return
		}
		delete(f.torrents, hash)
		f.removed = append(f.removed, hash)
		writeDelugeRPC(w, q.ID, true, nil)
	default:
		writeDelugeRPC(w, q.ID, nil, map[string]any{"code": 2, "message": "Unknown method", "traceback": ""})
	}
}

func writeDelugeRPC(w http.ResponseWriter, id int64, result, rpcErr any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "result": result, "error": rpcErr})
}

func cloneAnyMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func fakeDelugeTorrent(state string, progress float64) map[string]any {
	return map[string]any{
		"name":                  "Lab.Torrent.Movie.2024.1080p.WEB.H264-HOLYARRS",
		"label":                 "movies",
		"save_path":             "/downloads",
		"total_wanted":          float64(2 << 30),
		"progress":              progress,
		"download_payload_rate": float64(1 << 20),
		"eta":                   float64(600),
		"state":                 state,
		"is_finished":           state == "Seeding",
		"files": []any{map[string]any{
			"path": "Lab.Torrent.Movie.2024.1080p.WEB.H264-HOLYARRS.mkv", "size": float64(2 << 30), "progress": progress,
		}},
	}
}

func delugeHarness(t *testing.T) conformance {
	t.Helper()
	f := &fakeDeluge{connected: true}
	base := startDeluge(t, f)
	const password = "<deluge-test-password>"
	return conformance{
		name:    "deluge",
		engine:  NewDeluge(base, password, nil),
		badKey:  NewDeluge(base, "wrong-password", nil),
		at:      func(base string) Engine { return NewDeluge(base, password, nil) },
		secret:  password,
		release: AddRequest{URL: "magnet:?xt=urn:btih:" + delugeHash, Name: "Lab.Torrent.Movie.2024.1080p.WEB.H264-HOLYARRS", Category: "movies", SavePath: "/downloads", InfoHash: delugeHash},
		unknown: "0000000000000000000000000000000000000000",
		appear: func() {
			f.mu.Lock()
			f.torrents[delugeHash] = fakeDelugeTorrent("Downloading", 50)
			f.mu.Unlock()
		},
		adds: func() int { f.mu.Lock(); defer f.mu.Unlock(); return f.added },
		states: func(word string) {
			f.mu.Lock()
			f.torrents[delugeHash]["state"] = word
			f.torrents[delugeHash]["is_finished"] = word == "Seeding"
			if word == "Seeding" {
				f.torrents[delugeHash]["progress"] = float64(100)
			} else {
				f.torrents[delugeHash]["progress"] = float64(50)
			}
			f.mu.Unlock()
		},
		words: map[string]string{
			"Queued": StateQueued, "Checking": StateQueued, "Downloading": StateDownloading,
			"Paused": StatePaused, "Seeding": StateFinished, "Error": StatePaused,
			"brand-new-word": StateDownloading,
		},
	}
}
