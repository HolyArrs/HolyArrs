package downloader

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"
)

// conformance is one adapter wired to its fake app for the shared suite
// (design/system/downloader-contract.md). Every adapter, and the made-up
// third engine, must pass TestDownloaderConformance.
type conformance struct {
	name    string
	engine  Engine                   // signed in to a working fake app
	badKey  Engine                   // the same app with a wrong key or password
	at      func(base string) Engine // the adapter at another address
	secret  string                   // a key or password that must never be in an error
	release AddRequest               // a Release the app accepts
	unknown string                   // a well-formed id the app does not have
	appear  func()                   // the app now holds release, part fetched, with files
	adds    func() int               // how many adds reached the app
	states  func(word string)        // sets the app's own state word on release (nil: recorded words only)
	words   map[string]string        // the app's state words and the state each must map to
}

var ourStates = []string{StateQueued, StateDownloading, StatePaused, StateFinished, StateFailed}

func TestDownloaderConformance(t *testing.T) {
	for _, h := range []func(*testing.T) conformance{qbitHarness, sabHarness, relayHarness, transmissionHarness, nzbgetHarness, delugeHarness} {
		c := h(t)
		t.Run(c.name, func(t *testing.T) { runConformance(t, c) })
	}
}

func runConformance(t *testing.T, c conformance) {
	ctx := context.Background()
	var errs []error
	keep := func(err error) error {
		if err != nil {
			errs = append(errs, err)
		}
		return err
	}

	// 1. Health: a version, a refused key is ErrAuth, anything else is not.
	h, err := c.engine.Health(ctx)
	if err != nil || h.Version == "" {
		t.Fatalf("health: %+v %v", h, err)
	}
	if err := keep(func() error { _, err := c.badKey.Health(ctx); return err }()); !errors.Is(err, ErrAuth) {
		t.Errorf("wrong key: want ErrAuth, got %v", err)
	}
	closed := httptest.NewServer(http.NotFoundHandler())
	gone := closed.URL
	closed.Close()
	if err := keep(func() error { _, err := c.at(gone).Health(ctx); return err }()); err == nil || errors.Is(err, ErrAuth) {
		t.Errorf("unreachable app: want a plain error, got %v", err)
	}
	// An app that never answers: the adapter gives up with the caller's
	// deadline (the contract's 20 s is the caller's, so the adapter must
	// honour the context).
	release := make(chan struct{})
	hung := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body) // so the server sees the caller hang up
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(hung.Close)
	t.Cleanup(func() { close(release) })
	tctx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	start := time.Now()
	err = keep(func() error { _, err := c.at(hung.URL).Health(tctx); return err }())
	cancel()
	if err == nil || time.Since(start) > 3*time.Second {
		t.Errorf("hung app: %v after %v", err, time.Since(start))
	}

	// 2. Add gives an id at once; doing it twice is safe. A torrent with a
	// known info hash is looked up by it first (contract section 4, as the
	// manager does); otherwise Add itself must find the Release again.
	send := func() (string, error) {
		if c.release.InfoHash != "" {
			if j, err := c.engine.Get(ctx, c.release.InfoHash); err == nil {
				return j.ID, nil
			}
		}
		return c.engine.Add(ctx, c.release)
	}
	id, err := send()
	if err != nil || id == "" {
		t.Fatalf("add: %q %v", id, err)
	}
	c.appear()
	again, err := send()
	if err != nil || again != id || c.adds() != 1 {
		t.Errorf("second add: %q %v, %d adds reached the app (want the same id and 1)", again, err, c.adds())
	}

	// 3. List: one call, only the category, states and numbers in range.
	jobs, err := c.engine.List(ctx, c.release.Category)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	found := false
	for _, j := range jobs {
		found = found || j.ID == id
		if j.Category != "" && j.Category != c.release.Category {
			t.Errorf("list %s gave a Download of %s", c.release.Category, j.Category)
		}
		checkJob(t, j)
	}
	if !found {
		t.Errorf("list does not hold the added Download %s", id)
	}
	other, err := c.engine.List(ctx, "not-holyarrs")
	if err != nil {
		t.Fatalf("list another category: %v", err)
	}
	for _, j := range other {
		if j.ID == id {
			t.Errorf("a Download of %s came back for another category", c.release.Category)
		}
	}

	// 4. Get: the Download, or ErrNotFound.
	j, err := c.engine.Get(ctx, id)
	if err != nil || j.ID != id {
		t.Fatalf("get: %+v %v", j, err)
	}
	checkJob(t, j)
	for _, f := range j.Files {
		if f.Name == "" || strings.HasPrefix(f.Name, "/") || f.Progress < 0 || f.Progress > 1 {
			t.Errorf("file %+v: names are relative to the save folder, progress 0 to 1", f)
		}
	}
	if _, err := c.engine.Get(ctx, c.unknown); !errors.Is(err, ErrNotFound) {
		t.Errorf("get unknown: want ErrNotFound, got %v", err)
	}

	// 5. States: each of the app's words maps as the contract says; an
	// unknown word is downloading.
	if c.states != nil {
		for word, want := range c.words {
			c.states(word)
			j, err := c.engine.Get(ctx, id)
			if err != nil || j.State != want {
				t.Errorf("state word %q: got %q %v, want %q", word, j.State, err, want)
			}
			if j.Detail == "" {
				t.Errorf("state word %q: the app's word is not kept in Detail", word)
			}
		}
	}

	// 6. Remove: done, and removing what is gone is not worth stopping for.
	if err := c.engine.Remove(ctx, id, true); err != nil {
		t.Errorf("remove: %v", err)
	}
	if err := c.engine.Remove(ctx, c.unknown, false); err != nil && !errors.Is(err, ErrNotFound) {
		t.Errorf("remove unknown: %v", err)
	}

	// 7. No error ever carries the key or password.
	for _, err := range errs {
		if c.secret != "" && strings.Contains(err.Error(), c.secret) {
			t.Errorf("an error carries the secret: %v", err)
		}
	}
}

func checkJob(t *testing.T, j Job) {
	t.Helper()
	if !slices.Contains(ourStates, j.State) {
		t.Errorf("%s: state %q is not one of ours", j.ID, j.State)
	}
	if j.Progress < 0 || j.Progress > 1 || j.Speed < 0 || j.ETA < 0 || j.Size < 0 {
		t.Errorf("%s: progress %v speed %d time left %d size %d out of range", j.ID, j.Progress, j.Speed, j.ETA, j.Size)
	}
	if j.State == StateFinished && j.Progress < 1 && j.Progress != 0 {
		// Finished means final; an app that keeps a part number for a
		// finished one is read as finished anyway, but say so.
		t.Logf("%s: finished at progress %v", j.ID, j.Progress)
	}
}

func qbitHarness(t *testing.T) conformance {
	const pw = "lab-test-key" // test-only
	f, u := startFake(t)
	f.key = pw
	return conformance{
		name:    "qbittorrent",
		engine:  NewQbittorrentKey(u, pw, nil),
		badKey:  NewQbittorrentKey(u, "wrong", nil),
		at:      func(base string) Engine { return NewQbittorrentKey(base, pw, nil) },
		secret:  pw,
		release: AddRequest{URL: "magnet:?xt=urn:btih:" + hash, Name: "Lab.Torrent.Movie.2024.1080p.WEB.H264-HOLYARRS", Category: "holyarrs", SavePath: "/downloads", InfoHash: hash},
		unknown: "0000000000000000000000000000000000000000",
		appear: func() {
			f.set(hash, map[string]any{"name": "Lab.Torrent.Movie.2024.1080p.WEB.H264-HOLYARRS", "category": "holyarrs", "state": "downloading", "progress": 0.5, "dlspeed": 1 << 20, "eta": 600, "size": int64(2 << 30)})
			f.mu.Lock()
			f.files[hash] = []map[string]any{{"name": "Lab.Torrent.Movie.2024.1080p.WEB.H264-HOLYARRS.mkv", "size": int64(2 << 30), "progress": 0.5}}
			f.mu.Unlock()
		},
		adds:   func() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.added) },
		states: func(w string) { f.set(hash, map[string]any{"state": w}) },
		words: map[string]string{"queuedDL": StateQueued, "downloading": StateDownloading, "pausedDL": StatePaused, "stoppedDL": StatePaused,
			"uploading": StateFinished, "stalledUP": StateFinished, "error": StatePaused, "missingFiles": StatePaused, "brand-new-word": StateDownloading},
	}
}

func sabHarness(t *testing.T) conformance {
	f := &fakeSab{}
	u := startSab(t, f)
	return conformance{
		name:    "sabnzbd",
		engine:  NewSabnzbd(u, sabKey, nil),
		badKey:  NewSabnzbd(u, "ffffffffffffffffffffffffffffffff", nil),
		at:      func(base string) Engine { return NewSabnzbd(base, sabKey, nil) },
		secret:  sabKey,
		release: AddRequest{URL: "http://lab/good.nzb", Name: "Lab.Usenet.Movie.2024.1080p.WEB.H264-HOLYARRS", Category: "movies"},
		unknown: "00000000-0000-0000-0000-000000000000",
		appear:  func() { f.mu.Lock(); f.queue = "queue-downloading.json"; f.mu.Unlock() },
		adds:    func() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.added) },
		// SABnzbd's words come from the recorded replies; the queue and
		// finished-list words are checked in sabnzbd_test.go.
	}
}
