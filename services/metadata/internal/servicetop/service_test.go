package servicetop

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeTMDB struct{ popular, posters int }

func (f *fakeTMDB) ProviderID(context.Context, string, string) (int, error) { return 73, nil }

func (f *fakeTMDB) Popular(_ context.Context, provider int, region string, _ bool) ([]Item, error) {
	f.popular++
	return []Item{{Rank: 1, TMDBID: int64(provider), Kind: "movie", Title: "Popular " + region, PosterPath: "/p.jpg"}}, nil
}

func (f *fakeTMDB) Poster(context.Context, string, int64) (string, error) {
	f.posters++
	return "/poster.jpg", nil
}

// answer is one fake API reply.
type answer struct {
	status int
	body   string
	err    error
	used   int
}

// fakeAPI answers per service; a service with no queued answer gets the
// default list. byKey counts calls per key.
type fakeAPI struct {
	mu      sync.Mutex
	queue   map[string][]answer
	keyFail map[string]int
	byKey   map[string]int
	used    map[string]int
	granted int
	reset   string
}

func newAPI() *fakeAPI {
	return &fakeAPI{queue: map[string][]answer{}, keyFail: map[string]int{}, byKey: map[string]int{}, used: map[string]int{}, granted: 1000, reset: "2026-11-01T00:00:00Z"}
}

func fakeList(svc string, n int) string {
	var b strings.Builder
	b.WriteString("[")
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		kind := "movie"
		if i%2 == 0 {
			kind = "tv"
		}
		b.WriteString(`{"tmdbId":"` + kind + `/` + strconv.Itoa(100+i) + `","title":"` + svc + ` ` + strconv.Itoa(i) + `"}`)
	}
	b.WriteString("]")
	return b.String()
}

func (f *fakeAPI) RoundTrip(r *http.Request) (*http.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key, svc := r.Header.Get("X-API-Key"), r.URL.Query().Get("service")
	f.byKey[key]++
	f.used[key]++
	a := answer{status: 200, body: fakeList(svc, 6)}
	if st := f.keyFail[key]; st != 0 {
		a = answer{status: st}
	} else if q := f.queue[svc]; len(q) > 0 {
		a, f.queue[svc] = q[0], q[1:]
	}
	if a.err != nil {
		return nil, a.err
	}
	if a.used != 0 {
		f.used[key] = a.used
	}
	h := http.Header{}
	h.Set("X-Quota-Used", strconv.Itoa(f.used[key]))
	h.Set("X-Quota-Granted", strconv.Itoa(f.granted))
	h.Set("X-Quota-Reset", f.reset)
	return &http.Response{StatusCode: a.status, Status: strconv.Itoa(a.status), Header: h, Body: io.NopCloser(strings.NewReader(a.body))}, nil
}

type rig struct {
	t    *testing.T
	path string
	api  *fakeAPI
	tm   *fakeTMDB
	now  time.Time
	s    *Service
	keys []string
}

func newRig(t *testing.T, keys ...string) *rig {
	if len(keys) == 0 {
		keys = []string{"k1"}
	}
	r := &rig{t: t, path: filepath.Join(t.TempDir(), "s.sqlite"), api: newAPI(), tm: &fakeTMDB{}, now: time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC), keys: keys}
	r.open()
	return r
}

func (r *rig) open() {
	if r.s != nil {
		r.s.Close()
	}
	s, err := Open(r.path, Options{BaseURL: "https://sa.test", Keys: r.keys, TMDB: r.tm, HTTP: &http.Client{Transport: r.api}, Now: func() time.Time { return r.now }})
	if err != nil {
		r.t.Fatal(err)
	}
	r.s = s
	r.t.Cleanup(func() { s.Close() })
}

func (r *rig) refresh(after time.Duration) {
	r.now = r.now.Add(after)
	r.s.Refresh(context.Background())
}

type outBody struct {
	Attribution string `json:"attribution"`
	Services    []struct {
		Service, Source string
		FetchedAt       string `json:"fetched_at"`
		AgeSeconds      int64  `json:"age_seconds"`
		Stale           bool
		Items           []Item
	} `json:"services"`
}

func (r *rig) body() outBody {
	var out outBody
	if b, _, ok := r.s.List("US"); ok {
		if err := json.Unmarshal(b, &out); err != nil {
			r.t.Fatal(err)
		}
	}
	return out
}

func (r *rig) items(svc string) []Item {
	for _, s := range r.body().Services {
		if s.Service == svc {
			return s.Items
		}
	}
	return nil
}

func (r *rig) state(svc string) ServiceStats {
	for _, st := range r.s.Stats().([]ServiceStats) {
		if st.Service == svc {
			return st
		}
	}
	return ServiceStats{}
}

func (r *rig) queue(svc string, a ...answer) { r.api.queue[svc] = append(r.api.queue[svc], a...) }

func TestScenarios(t *testing.T) {
	errNet := errors.New("dial tcp: i/o timeout")
	cases := []struct {
		name string
		run  func(t *testing.T, r *rig)
	}{
		{"1 network error retries with backoff then keeps the last good list", func(t *testing.T, r *rig) {
			r.refresh(0)
			r.queue("netflix", answer{err: errNet}, answer{err: errNet}, answer{err: errNet})
			r.refresh(12 * time.Hour)
			before := r.api.byKey["k1"]
			r.refresh(5 * time.Minute) // not due yet (first retry after 10 min)
			r.refresh(5 * time.Minute)
			r.refresh(20 * time.Minute)
			if r.api.byKey["k1"]-before != 2 || len(r.items("netflix")) != 6 {
				t.Fatalf("retries %d, items %d", r.api.byKey["k1"]-before, len(r.items("netflix")))
			}
			if st := r.state("netflix"); st.LastError == "" {
				t.Fatalf("no last_error %+v", st)
			}
			r.refresh(30 * time.Minute) // third failure: waits for the next round
			n := r.api.byKey["k1"]
			r.refresh(time.Hour)
			if r.api.byKey["k1"] != n {
				t.Fatal("kept calling after three tries")
			}
		}},
		{"2 rejected key stops calling and flags key_invalid", func(t *testing.T, r *rig) {
			r.refresh(0)
			r.api.keyFail["k1"] = 401
			r.refresh(12 * time.Hour)
			calls := r.api.byKey["k1"]
			r.refresh(12 * time.Hour)
			if r.api.byKey["k1"] != calls || r.state("hulu").State != StateKeyInvalid || len(r.items("hulu")) != 6 {
				t.Fatalf("calls %d→%d %+v", calls, r.api.byKey["k1"], r.state("hulu"))
			}
		}},
		{"3 429 and 90% quota pause until the reset, then resume (13)", func(t *testing.T, r *rig) {
			r.api.reset = "2026-10-21T00:00:00Z"
			r.queue("netflix", answer{status: 429})
			r.refresh(0)
			if r.state("netflix").State != StateQuotaPaused || r.api.byKey["k1"] != 1 {
				t.Fatalf("429: %+v calls %d", r.state("netflix"), r.api.byKey["k1"])
			}
			r.refresh(13 * time.Hour) // past the reset: resumes
			if r.state("netflix").State != StateOK || len(r.items("netflix")) != 6 {
				t.Fatalf("after reset %+v", r.state("netflix"))
			}
			r.api.reset = "2026-11-01T00:00:00Z"
			r.queue("netflix", answer{status: 200, body: fakeList("netflix", 6), used: 900})
			r.refresh(12 * time.Hour)
			if r.state("prime").State != StateQuotaPaused || len(r.items("prime")) != 6 {
				t.Fatalf("90%%: %+v", r.state("prime"))
			}
		}},
		{"4 5xx and malformed JSON keep the last good list", func(t *testing.T, r *rig) {
			r.refresh(0)
			r.queue("netflix", answer{status: 503})
			r.queue("prime", answer{status: 200, body: `{"not":"a list"`})
			r.refresh(12 * time.Hour)
			if len(r.items("netflix")) != 6 || len(r.items("prime")) != 6 || r.state("prime").LastError != "malformed answer" {
				t.Fatalf("%+v", r.state("prime"))
			}
		}},
		{"5 an empty list is believed only after three in a row", func(t *testing.T, r *rig) {
			r.refresh(0)
			r.queue("netflix", answer{status: 200, body: `[]`}, answer{status: 200, body: `[]`}, answer{status: 200, body: `[]`})
			r.refresh(12 * time.Hour)
			r.refresh(12 * time.Hour)
			if len(r.items("netflix")) != 6 {
				t.Fatal("empty list replaced the good one")
			}
			r.refresh(12 * time.Hour)
			if r.items("netflix") != nil {
				t.Fatal("three empty answers not believed")
			}
		}},
		{"6 a partial list is accepted", func(t *testing.T, r *rig) {
			r.refresh(0)
			r.queue("netflix", answer{status: 200, body: fakeList("netflix", 2)})
			r.refresh(12 * time.Hour)
			if len(r.items("netflix")) != 2 {
				t.Fatal("partial list refused")
			}
		}},
		{"7 one service failing leaves the others fresh", func(t *testing.T, r *rig) {
			r.refresh(0)
			r.queue("disney", answer{status: 500})
			r.queue("prime", answer{status: 200, body: fakeList("prime", 9)})
			r.refresh(12 * time.Hour)
			if len(r.items("disney")) != 6 || len(r.items("prime")) != 9 {
				t.Fatal("not independent")
			}
		}},
		{"8 and 9 titles without a TMDB id are skipped, repeats keep the best rank", func(t *testing.T, r *rig) {
			r.queue("netflix", answer{status: 200, body: `[{"tmdbId":"tv/1","title":"A"},{"title":"No id"},{"tmdbId":"movie/2","title":"B"},{"tmdbId":"tv/1","title":"A again"},{"tmdbId":"movie/3","title":"C"}]`})
			r.refresh(0)
			got := r.items("netflix")
			if len(got) != 3 || got[0].TMDBID != 1 || got[0].Rank != 1 || got[1].TMDBID != 2 || got[1].Rank != 2 || got[2].TMDBID != 3 || got[2].Kind != "movie" {
				t.Fatalf("%+v", got)
			}
		}},
		{"10 a restart serves the stored lists at once and spends no calls", func(t *testing.T, r *rig) {
			r.refresh(0)
			calls := r.api.byKey["k1"]
			r.open()
			if len(r.items("netflix")) != 6 {
				t.Fatal("not served on open")
			}
			r.refresh(time.Hour)
			if r.api.byKey["k1"] != calls {
				t.Fatal("restart spent calls")
			}
		}},
		{"11 cold start with no list hides the service; only Peacock and Tubi use TMDB", func(t *testing.T, r *rig) {
			for _, p := range Providers {
				r.queue(p.SA, answer{status: 500})
			}
			r.refresh(0)
			b := r.body()
			if len(b.Services) != 2 || b.Services[0].Service != "peacock" || b.Services[1].Service != "tubi" || b.Services[1].Items[0].TMDBID != 73 || b.Services[0].Source != SourcePopular || r.tm.popular != 2 {
				t.Fatalf("%+v", b)
			}
		}},
		{"12 age and stale after 14 days", func(t *testing.T, r *rig) {
			r.refresh(0)
			r.api.keyFail["k1"] = 500
			r.refresh(15 * 24 * time.Hour)
			b := r.body()
			if !b.Services[0].Stale || b.Services[0].AgeSeconds < 14*24*3600 || b.Services[0].FetchedAt != "2026-10-20T12:00:00Z" || r.state("netflix").State != StateStale {
				t.Fatalf("%+v %+v", b.Services[0], r.state("netflix"))
			}
		}},
		{"14 a second refresh while one runs returns at once", func(t *testing.T, r *rig) {
			r.s.run.Lock()
			r.refresh(0)
			r.s.run.Unlock()
			if r.api.byKey["k1"] != 0 {
				t.Fatal("ran twice at once")
			}
		}},
		{"15 snapshots are kept 30 days, the newest good one always", func(t *testing.T, r *rig) {
			r.refresh(0)
			r.refresh(12 * time.Hour)
			r.api.keyFail["k1"] = 500
			r.refresh(40 * 24 * time.Hour)
			var n int
			_ = r.s.db.QueryRow("SELECT COUNT(*) FROM snapshots WHERE service = 'netflix'").Scan(&n)
			if n != 1 || len(r.items("netflix")) != 6 {
				t.Fatalf("snapshots %d", n)
			}
		}},
		{"backup key takes over when the primary is rejected or at its quota", func(t *testing.T, r *rig) {
			r.keys = []string{"primary", "backup"}
			r.open()
			r.api.keyFail["primary"] = 403
			r.refresh(0)
			if r.api.byKey["primary"] != 1 || r.api.byKey["backup"] != 9 {
				t.Fatalf("%v", r.api.byKey)
			}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { c.run(t, newRig(t)) })
	}
}

func TestFirstRefreshCallsEachServiceOnce(t *testing.T) {
	r := newRig(t)
	r.refresh(0)
	b := r.body()
	if r.api.byKey["k1"] != 9 || len(b.Services) != len(Providers) || b.Attribution != Attribution || b.Services[0].Items[0].PosterPath != "/poster.jpg" {
		t.Fatalf("calls %d %+v", r.api.byKey["k1"], b)
	}
	r.refresh(11*time.Hour + 59*time.Minute)
	if r.api.byKey["k1"] != 9 {
		t.Fatal("fetched before 12 hours")
	}
	r.refresh(time.Minute)
	if r.api.byKey["k1"] != 18 {
		t.Fatalf("calls after 12 h %d", r.api.byKey["k1"])
	}
}
