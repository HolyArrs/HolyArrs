package providers

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/model"
)

const mbid = "12345678-1234-1234-1234-123456789abc"

func query(provider, kind, operation string) model.Query {
	q := model.Query{Provider: provider, Kind: kind, Operation: operation, Page: 1, Limit: 20}
	if operation == "search" {
		q.Q = "Synthetic & + title"
	} else if provider == "musicbrainz" {
		q.ID = mbid
	} else {
		q.ID = "42"
	}
	return q
}

func fixtureClient(t *testing.T, provider string, handler http.HandlerFunc, change func(*Options)) *Client {
	t.Helper()
	s := httptest.NewServer(handler)
	t.Cleanup(s.Close)
	opts := Options{Providers: map[string]Config{provider: {BaseURL: s.URL, Interval: time.Nanosecond}}, AllowPrivate: true, Timeout: 3 * time.Second, Attempts: 1}
	if change != nil {
		change(&opts)
	}
	c, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func dataObject(t *testing.T, r model.Result) map[string]any {
	t.Helper()
	if !r.Complete {
		t.Fatal("result is incomplete")
	}
	var data map[string]any
	if err := json.Unmarshal(r.Data, &data); err != nil {
		t.Fatal(err)
	}
	return data
}

func fetchCode(t *testing.T, err error, want string) *model.FetchError {
	t.Helper()
	var fe *model.FetchError
	if !errors.As(err, &fe) || fe.Code != want {
		t.Fatalf("error = %v; want code %s", err, want)
	}
	return fe
}

func TestProviderRoutesAndNormalization(t *testing.T) {
	tests := []struct{ provider, kind, op, path, body, title string }{
		{"tmdb", "movie", "detail", "/movie/42", `{"id":42,"title":"Synthetic movie","overview":null,"genres":[],"unknown_secret":"drop"}`, "Synthetic movie"},
		{"tmdb", "show", "detail", "/tv/42", `{"id":42,"name":"Synthetic show","overview":"Summary","seasons":[]}`, "Synthetic show"},
		{"tmdb", "show", "season", "/tv/42/season/0", `{"id":99,"name":"Specials","season_number":0,"episodes":[]}`, "Specials"},
		{"tvmaze", "show", "detail", "/shows/42", `{"id":42,"name":"Synthetic maze","summary":null,"genres":[]}`, "Synthetic maze"},
	}
	for _, kind := range []string{"artist", "release-group", "release", "recording"} {
		field := "title"
		if kind == "artist" {
			field = "name"
		}
		tests = append(tests, struct{ provider, kind, op, path, body, title string }{"musicbrainz", kind, "detail", "/" + kind + "/" + mbid, fmt.Sprintf(`{"id":%q,%q:"Synthetic music","disambiguation":"Edition","aliases":[]}`, mbid, field), "Synthetic music"})
	}
	for _, tt := range tests {
		t.Run(tt.provider+"/"+tt.kind+"/"+tt.op, func(t *testing.T) {
			c := fixtureClient(t, tt.provider, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/release-group" && r.URL.Query().Has("artist") {
					fmt.Fprint(w, `{"release-groups":[]}`)
					return
				}
				if r.URL.Path != tt.path {
					t.Errorf("path=%s want %s", r.URL.Path, tt.path)
				}
				if tt.provider == "musicbrainz" && r.URL.Query().Get("fmt") != "json" {
					t.Error("missing JSON format")
				}
				fmt.Fprint(w, tt.body)
			}, nil)
			r, err := c.Fetch(context.Background(), query(tt.provider, tt.kind, tt.op))
			if err != nil {
				t.Fatal(err)
			}
			d := dataObject(t, r)
			if d["title"] != tt.title || d["source"] != tt.provider {
				t.Fatalf("normalization=%s", r.Data)
			}
			source := d["source_data"].(map[string]any)
			if _, ok := source["unknown_secret"]; ok {
				t.Fatal("unapproved field retained")
			}
		})
	}
}

func TestSearchSchemasAndEscaping(t *testing.T) {
	tests := []struct{ provider, kind, path, field, body string }{
		{"tmdb", "movie", "/search/movie", "query", `{"page":1,"total_pages":1,"total_results":1,"results":[{"id":42,"title":"Movie"}]}`},
		{"tmdb", "show", "/search/tv", "query", `{"page":1,"total_pages":1,"total_results":1,"results":[{"id":42,"name":"Show"}]}`},
		{"tvmaze", "show", "/search/shows", "q", `[{"score":0.9,"show":{"id":42,"name":"Show"}}]`},
		{"musicbrainz", "artist", "/artist", "query", `{"count":1,"offset":0,"artists":[{"id":"` + mbid + `","name":"Artist"}]}`},
		{"musicbrainz", "release-group", "/release-group", "query", `{"count":1,"offset":0,"release-groups":[{"id":"` + mbid + `","title":"Group"}]}`},
		{"musicbrainz", "release", "/release", "query", `{"count":1,"offset":0,"releases":[{"id":"` + mbid + `","title":"Release"}]}`},
		{"musicbrainz", "recording", "/recording", "query", `{"count":1,"offset":0,"recordings":[{"id":"` + mbid + `","title":"Recording"}]}`},
	}
	for _, tt := range tests {
		t.Run(tt.provider+"/"+tt.kind, func(t *testing.T) {
			q := query(tt.provider, tt.kind, "search")
			q.Language = "en-US"
			q.Region = "US"
			c := fixtureClient(t, tt.provider, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tt.path || r.URL.Query().Get(tt.field) != q.Q {
					t.Errorf("unsafe/wrong URL %s", r.URL)
				}
				fmt.Fprint(w, tt.body)
			}, nil)
			r, err := c.Fetch(context.Background(), q)
			if err != nil {
				t.Fatal(err)
			}
			d := dataObject(t, r)
			items, ok := d["items"].([]any)
			if !ok || len(items) != 1 {
				t.Fatalf("items=%s", r.Data)
			}
			if items[0].(map[string]any)["source_id"] == nil {
				t.Fatal("missing normalized identity")
			}
		})
	}
}

func TestEmptySearchAndUnknowns(t *testing.T) {
	for _, tc := range []struct{ provider, kind, body string }{
		{"tmdb", "movie", `{"page":1,"total_pages":0,"total_results":0,"results":[]}`},
		{"tvmaze", "show", `[]`},
		{"musicbrainz", "artist", `{"count":0,"offset":0,"artists":[]}`},
	} {
		t.Run(tc.provider, func(t *testing.T) {
			c := fixtureClient(t, tc.provider, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, tc.body) }, nil)
			r, err := c.Fetch(context.Background(), query(tc.provider, tc.kind, "search"))
			if err != nil {
				t.Fatal(err)
			}
			if len(dataObject(t, r)["items"].([]any)) != 0 {
				t.Fatal("empty search not empty")
			}
		})
	}
	c := fixtureClient(t, "tmdb", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"id":42}`) }, nil)
	r, err := c.Fetch(context.Background(), query("tmdb", "movie", "detail"))
	if err != nil {
		t.Fatal(err)
	}
	d := dataObject(t, r)
	if d["title"] != nil || d["summary"] != nil {
		t.Fatal("invented unknown fields")
	}
}

func TestRejectInvalidOrPartialJSON(t *testing.T) {
	tests := []struct{ provider, kind, op, body string }{
		{"tmdb", "movie", "search", `{"results":null,"page":1,"total_pages":0,"total_results":0}`},
		{"tmdb", "movie", "search", `{"results":[]}`},
		{"tmdb", "movie", "search", `{"page":1,"total_pages":1,"total_results":1,"results":[{}]}`},
		{"tvmaze", "show", "search", `{"shows":[]}`},
		{"tvmaze", "show", "search", `[{"show":null}]`},
		{"musicbrainz", "artist", "search", `{"count":0,"offset":0,"artists":null}`},
		{"musicbrainz", "artist", "search", `{"count":1,"offset":0,"artists":[{"id":"broken"}]}`},
		{"tmdb", "movie", "detail", `{"title":"No id"}`},
		{"tmdb", "movie", "detail", `{"id":43}`},
		{"tmdb", "movie", "detail", `{"id":42.2}`},
		{"tmdb", "movie", "detail", `{"id":42} {"extra":true}`},
		{"tmdb", "movie", "detail", `{"id":42`},
		{"tmdb", "show", "season", `{"id":99,"season_number":0}`},
		{"tmdb", "show", "season", `{"id":99,"season_number":1,"episodes":[]}`},
	}
	for i, tt := range tests {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			c := fixtureClient(t, tt.provider, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, tt.body) }, nil)
			r, err := c.Fetch(context.Background(), query(tt.provider, tt.kind, tt.op))
			fetchCode(t, err, "upstream_invalid")
			if r.Complete {
				t.Fatal("partial reply marked complete")
			}
		})
	}
}

func TestBodyLimitAfterGzipDecode(t *testing.T) {
	c := fixtureClient(t, "tmdb", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Encoding", "gzip")
		z := gzip.NewWriter(w)
		fmt.Fprintf(z, `{"id":42,"title":%q}`, strings.Repeat("x", 4096))
		z.Close()
	}, func(o *Options) { o.MaxBodyBytes = 512 })
	_, err := c.Fetch(context.Background(), query("tmdb", "movie", "detail"))
	fetchCode(t, err, "upstream_too_large")
}

func TestSafeConfigurationAndRoutes(t *testing.T) {
	for _, base := range []string{"http://api.themoviedb.org/3", "https://example.com", "https://user:pass@api.themoviedb.org/3", "https://api.themoviedb.org/3?api_key=secret", "https://api.themoviedb.org/3#fragment", "https://api.themoviedb.org:8443/3", "https://api.themoviedb.org/private"} {
		if _, err := New(Options{Providers: map[string]Config{"tmdb": {BaseURL: base}}}); err == nil {
			t.Errorf("accepted unsafe base %s", base)
		}
	}
	c := fixtureClient(t, "tmdb", func(w http.ResponseWriter, r *http.Request) { t.Error("invalid query reached upstream") }, nil)
	for _, id := range []string{"../42", "42?x=y", "https://bad", "42%2f", "00042", "4e2"} {
		q := query("tmdb", "movie", "detail")
		q.ID = id
		_, err := c.Fetch(context.Background(), q)
		fetchCode(t, err, "invalid_query")
	}
	_, err := c.Fetch(context.Background(), query("tvmaze", "show", "detail"))
	fetchCode(t, err, "provider_unavailable")
}

func TestRejectRedirectAndDoNotForwardAuth(t *testing.T) {
	var targetCalls atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { targetCalls.Add(1) }))
	defer target.Close()
	c := fixtureClient(t, "tvmaze", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("unexpected authorization")
		}
		http.Redirect(w, r, target.URL, http.StatusFound)
	}, nil)
	_, err := c.Fetch(context.Background(), query("tvmaze", "show", "detail"))
	fetchCode(t, err, "upstream_status")
	if targetCalls.Load() != 0 {
		t.Fatal("redirect followed")
	}
}

func TestTMDBCredentialFileAndPause(t *testing.T) {
	key := filepath.Join(t.TempDir(), "synthetic-token")
	if err := os.WriteFile(key, []byte("fixture-token\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	c := fixtureClient(t, "tmdb", func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer fixture-token" {
			t.Error("incorrect bearer header")
		}
		if r.URL.Query().Get("api_key") != "" {
			t.Error("key in URL")
		}
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, "private provider error")
	}, func(o *Options) { p := o.Providers["tmdb"]; p.KeyFile = key; o.Providers["tmdb"] = p; o.Attempts = 3 })
	for i := 0; i < 2; i++ {
		_, err := c.Fetch(context.Background(), query("tmdb", "movie", "detail"))
		fetchCode(t, err, "provider_credentials")
		if strings.Contains(err.Error(), "private") {
			t.Fatal("raw provider error leaked")
		}
	}
	if calls.Load() != 1 {
		t.Fatal("paused provider made more requests")
	}
}

func TestGlobalPacingAcrossDifferentQueries(t *testing.T) {
	var mu sync.Mutex
	var times []time.Time
	c := fixtureClient(t, "tmdb", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		times = append(times, time.Now())
		mu.Unlock()
		id := strings.TrimPrefix(r.URL.Path, "/movie/")
		fmt.Fprintf(w, `{"id":%s}`, id)
	}, func(o *Options) {
		p := o.Providers["tmdb"]
		p.Interval = 45 * time.Millisecond
		o.Providers["tmdb"] = p
	})
	var wg sync.WaitGroup
	for i := 1; i <= 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			q := query("tmdb", "movie", "detail")
			q.ID = fmt.Sprint(i)
			if _, err := c.Fetch(context.Background(), q); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	mu.Lock()
	defer mu.Unlock()
	if len(times) != 4 {
		t.Fatalf("calls=%d", len(times))
	}
	for i := 1; i < len(times); i++ {
		if times[i].Sub(times[i-1]) < 35*time.Millisecond {
			t.Fatal("provider pacing violated")
		}
	}
}

func TestRetryAfterAppliesAcrossQueries(t *testing.T) {
	var calls atomic.Int32
	first := make(chan struct{})
	var mu sync.Mutex
	var times []time.Time
	c := fixtureClient(t, "tmdb", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		times = append(times, time.Now())
		mu.Unlock()
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			close(first)
			return
		}
		fmt.Fprint(w, `{"id":42}`)
	}, func(o *Options) { o.Attempts = 1 })
	_, err := c.Fetch(context.Background(), query("tmdb", "movie", "detail"))
	fe := fetchCode(t, err, "upstream_status")
	if fe.RetryAfter < 900*time.Millisecond {
		t.Fatal("Retry-After not exposed")
	}
	<-first
	q := query("tmdb", "movie", "detail")
	q.Language = "fr"
	if _, err := c.Fetch(context.Background(), q); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if times[1].Sub(times[0]) < 950*time.Millisecond {
		t.Fatal("different query ignored Retry-After")
	}
}

func TestBoundedRetryAndDeadline(t *testing.T) {
	var calls atomic.Int32
	c := fixtureClient(t, "tmdb", func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}, func(o *Options) { o.Attempts = 3; o.Timeout = time.Second })
	_, err := c.Fetch(context.Background(), query("tmdb", "movie", "detail"))
	fetchCode(t, err, "upstream_status")
	if calls.Load() != 3 {
		t.Fatalf("attempts=%d", calls.Load())
	}
	c = fixtureClient(t, "tmdb", func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }, func(o *Options) { o.Attempts = 2; o.Timeout = 120 * time.Millisecond })
	start := time.Now()
	_, err = c.Fetch(context.Background(), query("tmdb", "movie", "detail"))
	fetchCode(t, err, "upstream_timeout")
	if time.Since(start) > time.Second {
		t.Fatal("unbounded timeout")
	}
}

func TestDedupWaiterCancellationAndBoundedQueue(t *testing.T) {
	var calls atomic.Int32
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	c := fixtureClient(t, "tmdb", func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		select {
		case entered <- struct{}{}:
		default:
		}
		select {
		case <-release:
			fmt.Fprint(w, `{"id":42}`)
		case <-r.Context().Done():
		}
	}, func(o *Options) { o.MaxPending = 1 })
	q := query("tmdb", "movie", "detail")
	ctx, cancel := context.WithCancel(context.Background())
	first := make(chan error, 1)
	go func() { _, err := c.Fetch(ctx, q); first <- err }()
	<-entered
	second := make(chan error, 1)
	go func() { _, err := c.Fetch(context.Background(), q); second <- err }()
	// Wait until the second caller has joined the existing flight before cancelling its first waiter.
	deadline := time.Now().Add(time.Second)
	for {
		c.mu.Lock()
		f := c.flights[q.Key()]
		joined := f != nil && f.waiters == 2
		c.mu.Unlock()
		if joined {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("second waiter did not join")
		}
		time.Sleep(time.Millisecond)
	}
	other := q
	other.Language = "fr"
	_, err := c.Fetch(context.Background(), other)
	fetchCode(t, err, "queue_full")
	cancel()
	fetchCode(t, <-first, "cancelled")
	close(release)
	if err := <-second; err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatal("deduplication failed")
	}
}

func TestAllWaitersCancelAllowsFreshFlight(t *testing.T) {
	var calls atomic.Int32
	entered := make(chan struct{}, 2)
	c := fixtureClient(t, "tmdb", func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		entered <- struct{}{}
		if n == 1 {
			<-r.Context().Done()
			return
		}
		fmt.Fprint(w, `{"id":42}`)
	}, nil)
	q := query("tmdb", "movie", "detail")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := c.Fetch(ctx, q); done <- err }()
	<-entered
	cancel()
	fetchCode(t, <-done, "cancelled")
	if _, err := c.Fetch(context.Background(), q); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatal("cancelled flight stranded next caller")
	}
}

func TestConcurrentDedupAndResultIsolation(t *testing.T) {
	var calls atomic.Int32
	c := fixtureClient(t, "tmdb", func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		time.Sleep(80 * time.Millisecond)
		fmt.Fprint(w, `{"id":42,"title":"Synthetic"}`)
	}, nil)
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make(chan model.Result, 24)
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			r, err := c.Fetch(context.Background(), query("tmdb", "movie", "detail"))
			if err != nil {
				t.Error(err)
				return
			}
			results <- r
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	if calls.Load() != 1 {
		t.Fatalf("calls=%d", calls.Load())
	}
	var wgMut sync.WaitGroup
	for r := range results {
		wgMut.Add(1)
		go func(r model.Result) {
			defer wgMut.Done()
			for i := range r.Data {
				r.Data[i] = 'x'
			}
		}(r)
	}
	wgMut.Wait()
}

func TestPrivateAndReservedAddressesBlocked(t *testing.T) {
	for _, address := range []string{"127.0.0.1", "10.1.2.3", "172.16.0.1", "192.168.0.1", "169.254.169.254", "100.87.70.86", "0.1.2.3", "198.18.0.1", "192.0.2.1", "224.0.0.1", "255.255.255.255", "::", "::1", "::ffff:127.0.0.1", "fc00::1", "fe80::1", "fec0::1", "64:ff9b::a00:1", "2001:db8::1", "2002:a00:1::"} {
		if publicAddress(netip.MustParseAddr(address)) {
			t.Errorf("accepted reserved address %s", address)
		}
	}
	for _, address := range []string{"1.1.1.1", "8.8.8.8", "2606:4700:4700::1111"} {
		if !publicAddress(netip.MustParseAddr(address)) {
			t.Errorf("blocked public address %s", address)
		}
	}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("private target reached") }))
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	conn, err := safeDial(false)(ctx, "tcp", strings.TrimPrefix(s.URL, "http://"))
	if conn != nil {
		conn.Close()
	}
	if err == nil {
		t.Fatal("dialed private fixture without explicit permission")
	}
}

func TestRetryAfterDateAndLongDelay(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		header string
		want   time.Duration
	}{
		{"2", 2 * time.Second}, {now.Add(3 * time.Second).Format(http.TimeFormat), 3 * time.Second}, {"-1", 0}, {"invalid", 0}, {now.Add(-time.Hour).Format(http.TimeFormat), 0}, {"172800", 48 * time.Hour},
	} {
		if got := retryAfter(tc.header, now); got != tc.want {
			t.Errorf("header %q got %s want %s", tc.header, got, tc.want)
		}
	}
}

func TestRetrySuccessAndTimeoutAttempts(t *testing.T) {
	var calls atomic.Int32
	c := fixtureClient(t, "tmdb", func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		fmt.Fprint(w, `{"id":42}`)
	}, func(o *Options) { o.Attempts = 2 })
	if _, err := c.Fetch(context.Background(), query("tmdb", "movie", "detail")); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatal("did not retry 503")
	}
	calls.Store(0)
	c = fixtureClient(t, "tmdb", func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			<-r.Context().Done()
			return
		}
		fmt.Fprint(w, `{"id":42}`)
	}, func(o *Options) { o.Attempts = 2; o.Timeout = 400 * time.Millisecond })
	if _, err := c.Fetch(context.Background(), query("tmdb", "movie", "detail")); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatal("did not retry timeout")
	}
}

func TestIndependentProvidersDoNotBlockEachOther(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	a := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		select {
		case <-release:
			fmt.Fprint(w, `{"id":42}`)
		case <-r.Context().Done():
		}
	}))
	defer a.Close()
	b := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"id":42}`) }))
	defer b.Close()
	c, err := New(Options{AllowPrivate: true, Providers: map[string]Config{"tmdb": {BaseURL: a.URL, Interval: time.Nanosecond}, "tvmaze": {BaseURL: b.URL, Interval: time.Nanosecond}}})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := c.Fetch(context.Background(), query("tmdb", "movie", "detail")); done <- err }()
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := c.Fetch(ctx, query("tvmaze", "show", "detail")); err != nil {
		t.Error(err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestInvalidOptionsAndCredentialFiles(t *testing.T) {
	for _, opts := range []Options{{MaxPending: -1}, {MaxBodyBytes: -1}, {Timeout: -time.Second}, {Attempts: 6}, {UserAgent: "bad\r\nheader"}, {Providers: map[string]Config{"unknown": {}}}, {Providers: map[string]Config{"tmdb": {Interval: -time.Second}}}, {Providers: map[string]Config{"tmdb": {KeyFile: filepath.Join(t.TempDir(), "missing")}}}} {
		if _, err := New(opts); err == nil {
			t.Errorf("accepted invalid options %+v", opts)
		}
	}
	key := filepath.Join(t.TempDir(), "bad-key")
	if err := os.WriteFile(key, []byte("bad\nheader"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(Options{Providers: map[string]Config{"tmdb": {KeyFile: key}}}); err == nil {
		t.Fatal("accepted newline in credential")
	}
}

func TestSearchRejectsEmptyPageWhenResultsRemain(t *testing.T) {
	for _, tc := range []struct{ provider, kind, body string }{
		{"tmdb", "movie", `{"page":1,"total_pages":1,"total_results":1,"results":[]}`},
		{"musicbrainz", "artist", `{"count":1,"offset":0,"artists":[]}`},
	} {
		t.Run(tc.provider, func(t *testing.T) {
			c := fixtureClient(t, tc.provider, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, tc.body) }, nil)
			_, err := c.Fetch(context.Background(), query(tc.provider, tc.kind, "search"))
			fetchCode(t, err, "upstream_invalid")
		})
	}
}

func TestDefaultProviderSafetyIntervals(t *testing.T) {
	c, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]time.Duration{"tmdb": 50 * time.Millisecond, "tvmaze": time.Second, "musicbrainz": 1100 * time.Millisecond} {
		if got := c.providers[name].interval; got != want {
			t.Errorf("%s interval=%s want %s", name, got, want)
		}
	}
	// TMDB runs 6 calls at once; MusicBrainz and TVmaze one.
	for name, want := range map[string]int{"tmdb": 6, "tvmaze": 1, "musicbrainz": 1} {
		if got := cap(c.providers[name].gate); got != want {
			t.Errorf("%s concurrency=%d want %d", name, got, want)
		}
	}
}

func TestDetailEnrichmentRequestsAndPreservation(t *testing.T) {
	tests := []struct {
		provider, kind, op, param, value, body string
		fields                                 []string
	}{
		{"tmdb", "movie", "detail", "append_to_response", "credits,translations,release_dates", `{"id":42,"credits":{"cast":[{"id":7,"name":"Synthetic Person","character":"Synthetic Character"}],"crew":[]},"translations":{"translations":[{"iso_639_1":"ja","data":{"title":"合成","overview":null}}]},"release_dates":{"results":[{"iso_3166_1":"US","release_dates":[{"type":4,"release_date":"2020-01-02T00:00:00Z"}]}]}}`, []string{"credits", "translations", "release_dates"}},
		{"tmdb", "show", "detail", "append_to_response", "credits,translations,content_ratings", `{"id":42,"credits":{"cast":[],"crew":[]},"translations":{"translations":[]},"content_ratings":{"results":[]}}`, []string{"credits", "translations", "content_ratings"}},
		{"tmdb", "show", "season", "append_to_response", "credits,translations", `{"id":99,"season_number":0,"episodes":[],"credits":{"cast":[],"crew":[]},"translations":{"translations":[]}}`, []string{"credits", "translations"}},
		{"tvmaze", "show", "detail", "embed", "episodes", `{"id":42,"_embedded":{"episodes":[{"id":99,"name":"Synthetic episode"}]}}`, []string{"_embedded"}},
		{"musicbrainz", "artist", "detail", "inc", "aliases+artist-rels+url-rels+genres+tags+ratings", `{"id":"` + mbid + `","aliases":[],"release-groups":[],"relations":[],"genres":[],"tags":[],"rating":{"value":null}}`, []string{"aliases", "release-groups", "relations", "genres", "tags", "rating"}},
		{"musicbrainz", "release-group", "detail", "inc", "artist-credits+releases+url-rels+genres+tags+ratings", `{"id":"` + mbid + `","artist-credit":[],"releases":[],"genres":[],"tags":[]}`, []string{"artist-credit", "releases", "genres", "tags"}},
		{"musicbrainz", "release", "detail", "inc", "artist-credits+labels+recordings+release-groups+isrcs+url-rels+genres+tags", `{"id":"` + mbid + `","release-group":{"id":"` + mbid + `","title":"Synthetic group"},"media":[{"position":1,"tracks":[{"id":"` + mbid + `","recording":{"id":"` + mbid + `"}}]}]}`, []string{"release-group", "media"}},
		{"musicbrainz", "recording", "detail", "inc", "artist-credits+releases+release-groups+isrcs+work-rels+url-rels+genres+tags+ratings", `{"id":"` + mbid + `","artist-credit":[],"releases":[],"isrcs":[],"relations":[]}`, []string{"artist-credit", "releases", "isrcs", "relations"}},
	}
	for _, tt := range tests {
		t.Run(tt.provider+"/"+tt.kind+"/"+tt.op, func(t *testing.T) {
			c := fixtureClient(t, tt.provider, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Has("artist") {
					// The artist's second call (release-group browse).
					fmt.Fprint(w, `{"release-groups":[],"release-group-count":0}`)
					return
				}
				if got := r.URL.Query().Get(tt.param); got != tt.value {
					t.Errorf("parameter %s=%q want %q", tt.param, got, tt.value)
				}
				fmt.Fprint(w, tt.body)
			}, nil)
			r, err := c.Fetch(context.Background(), query(tt.provider, tt.kind, tt.op))
			if err != nil {
				t.Fatal(err)
			}
			source := dataObject(t, r)["source_data"].(map[string]any)
			var original map[string]any
			if err := json.Unmarshal([]byte(tt.body), &original); err != nil {
				t.Fatal(err)
			}
			for _, field := range tt.fields {
				if got, ok := source[field]; !ok || !reflect.DeepEqual(got, original[field]) {
					t.Errorf("changed or lost source field %s in %s", field, r.Data)
				}
			}
		})
	}
}

func TestMusicBrainzRejectsItemsBeyondTotal(t *testing.T) {
	c := fixtureClient(t, "musicbrainz", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"count":1,"offset":20,"artists":[{"id":%q,"name":"Synthetic"}]}`, mbid)
	}, nil)
	q := query("musicbrainz", "artist", "search")
	q.Page = 2
	_, err := c.Fetch(context.Background(), q)
	fetchCode(t, err, "upstream_invalid")
}

func TestSearchNeverRequestsDetailExpansion(t *testing.T) {
	for _, tc := range []struct{ provider, kind, body string }{
		{"tmdb", "movie", `{"page":1,"total_pages":0,"total_results":0,"results":[]}`},
		{"tvmaze", "show", `[]`},
		{"musicbrainz", "artist", `{"count":0,"offset":0,"artists":[]}`},
	} {
		t.Run(tc.provider, func(t *testing.T) {
			c := fixtureClient(t, tc.provider, func(w http.ResponseWriter, r *http.Request) {
				for _, param := range []string{"inc", "append_to_response", "embed"} {
					if r.URL.Query().Has(param) {
						t.Errorf("search includes detail-only parameter %s", param)
					}
				}
				fmt.Fprint(w, tc.body)
			}, nil)
			if _, err := c.Fetch(context.Background(), query(tc.provider, tc.kind, "search")); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestInlineCredentialValidation(t *testing.T) {
	if _, err := New(Options{Providers: map[string]Config{"tmdb": {Key: "synthetic-inline"}}}); err != nil {
		t.Fatalf("inline credential rejected: %v", err)
	}
	for _, cfg := range []map[string]Config{{"tmdb": {Key: "has space"}}, {"tmdb": {Key: "a", KeyFile: "b"}}, {"musicbrainz": {Key: "x"}}} {
		if _, err := New(Options{Providers: cfg}); err == nil {
			t.Fatalf("accepted %+v", cfg)
		}
	}
}
