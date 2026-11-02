package upstream

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestParseEgress(t *testing.T) {
	if e, err := ParseEgress("  "); e != nil || err != nil {
		t.Fatalf("empty: %v %v", e, err)
	}
	e, err := ParseEgress("a=default, b=:21000-21999, 10.0.0.58, c=10.0.0.91:22000-22099")
	if err != nil {
		t.Fatal(err)
	}
	if e.N() != 4 {
		t.Fatalf("N = %d", e.N())
	}
	p := e.Paths
	if !p[0].Def || p[0].Label != "a" || p[0].pace != Shared {
		t.Fatalf("default path: %+v", p[0])
	}
	if p[1].IP.IsValid() || p[1].Lo != 21000 || p[1].Hi != 21999 || p[1].pace == Shared {
		t.Fatalf("port path: %+v", p[1])
	}
	if p[2].IP.String() != "10.0.0.58" || p[2].Label != "10.0.0.58" || p[2].Lo != 0 {
		t.Fatalf("ip path: %+v", p[2])
	}
	if p[3].IP.String() != "10.0.0.91" || p[3].Lo != 22000 {
		t.Fatalf("ip+port path: %+v", p[3])
	}
	for _, bad := range []string{"x=nonsense", ":80-90", ":2000", "default,default", "1.2.3.4:5000-4000"} {
		if _, err := ParseEgress(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	var nilPool *Egress
	if nilPool.N() != 1 || nilPool.Stats() != nil {
		t.Fatal("nil pool")
	}
}

type countRT struct {
	mu     sync.Mutex
	status int
	n      int
}

func (c *countRT) RoundTrip(r *http.Request) (*http.Response, error) {
	c.mu.Lock()
	c.n++
	st := c.status
	c.mu.Unlock()
	if st == 0 {
		st = 200
	}
	return &http.Response{StatusCode: st, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
}

func testPool(t *testing.T, n int, gap time.Duration) (*Egress, []*countRT) {
	t.Helper()
	e := &Egress{start: time.Now()}
	var rts []*countRT
	for i := range n {
		rt := &countRT{}
		rts = append(rts, rt)
		e.Paths = append(e.Paths, &Path{Label: string(rune('a' + i)), pace: NewPace(gap), rt: rt})
	}
	return e, rts
}

func TestEgressSpreadsAndPaces(t *testing.T) {
	e, rts := testPool(t, 3, 100*time.Millisecond)
	start := time.Now()
	for range 9 {
		r, _ := http.NewRequest("GET", "https://musicbrainz.org/ws/2/artist/x", nil)
		resp, err := e.roundTrip(r)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	// 9 calls over 3 paths at 10 a second each: about 200 ms, not 800.
	if d := time.Since(start); d > 600*time.Millisecond || d < 150*time.Millisecond {
		t.Fatalf("took %s", d)
	}
	for i, rt := range rts {
		if rt.n != 3 {
			t.Fatalf("path %d got %d calls", i, rt.n)
		}
	}
	st := e.Stats()
	if st["calls"].(int64) != 9 {
		t.Fatalf("stats %v", st)
	}
}

func TestEgressBacksOffOn503(t *testing.T) {
	e, rts := testPool(t, 2, 10*time.Millisecond)
	rts[0].status = 503
	for range 6 {
		r, _ := http.NewRequest("GET", "https://musicbrainz.org/ws/2/x", nil)
		resp, err := e.roundTrip(r)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	if rts[0].n != 1 || rts[1].n != 5 {
		t.Fatalf("calls %d %d: the 503 path was not held back", rts[0].n, rts[1].n)
	}
	p := e.Paths[0]
	if p.n503 != 1 || p.back != egressBackoffMin || !p.until.After(time.Now()) {
		t.Fatalf("backoff %+v", p)
	}
	rts[0].status = 200
	p.until = time.Time{}
	p.observe(&http.Response{StatusCode: 200}, nil, time.Now())
	if p.back != 0 {
		t.Fatal("good answer kept the backoff")
	}
}

func TestEgressOnlyMusicBrainz(t *testing.T) {
	e, rts := testPool(t, 1, time.Millisecond)
	old := Egresses
	Egresses = e
	defer func() { Egresses = old }()
	other := &countRT{}
	tr := Transport{Next: other, Pace: NewPace(time.Millisecond), Health: NewHealth(nil)}
	for _, u := range []string{"https://musicbrainz.org/ws/2/a", "https://coverartarchive.org/x", "https://api.listenbrainz.org/x"} {
		r, _ := http.NewRequest("GET", u, nil)
		resp, err := tr.RoundTrip(r)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	if rts[0].n != 1 || other.n != 2 {
		t.Fatalf("pool %d, other %d", rts[0].n, other.n)
	}
}

func TestEgressDialPortRange(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, port, _ := net.SplitHostPort(r.RemoteAddr)
		io.WriteString(w, port)
	}))
	defer srv.Close()
	e, err := ParseEgress("p=127.0.0.1:41000-41050")
	if err != nil {
		t.Fatal(err)
	}
	p := e.Paths[0]
	conn, err := p.dial(context.Background(), "tcp", srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	la := conn.LocalAddr().(*net.TCPAddr)
	if la.Port < 41000 || la.Port > 41050 || !la.IP.Equal(net.IPv4(127, 0, 0, 1)) {
		t.Fatalf("local %s", la)
	}
}

func TestEgressRecordsPathInContext(t *testing.T) {
	e, _ := testPool(t, 2, time.Millisecond)
	ctx, via := WithVia(context.Background())
	r, _ := http.NewRequestWithContext(ctx, "GET", "https://musicbrainz.org/ws/2/artist/x", nil)
	resp, err := e.roundTrip(r)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if got := via(); got == "" {
		t.Fatal("no path recorded")
	}
	if _, none := WithVia(context.Background()); none() != "" {
		t.Fatal("fresh recorder not empty")
	}
}
