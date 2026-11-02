package upstream

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func ok(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}}, nil
}

func TestMetaBrainzHosts(t *testing.T) {
	for h, want := range map[string]bool{"musicbrainz.org": true, "beta.musicbrainz.org": true, "coverartarchive.org": true, "api.listenbrainz.org": true,
		"labs.api.listenbrainz.org": true, "data.metabrainz.org": true, "musicbrainz.org:443": true, "archive.org": false, "api.deezer.com": false, "notmusicbrainz.org": false} {
		if MetaBrainz(h) != want {
			t.Errorf("%s: %v", h, !want)
		}
	}
}

// MusicBrainz, the Cover Art Archive and ListenBrainz share one pace,
// whichever transport makes the call; other hosts are not held.
func TestOnePaceForAllMetaBrainzHosts(t *testing.T) {
	gap := 40 * time.Millisecond
	tr := Transport{Next: rtFunc(ok), Pace: NewPace(gap), Health: NewHealth(nil)}
	other := Transport{Next: rtFunc(ok), Pace: tr.Pace, Health: tr.Health}
	var mu sync.Mutex
	var at []time.Time
	var wg sync.WaitGroup
	hosts := []string{"musicbrainz.org", "coverartarchive.org", "api.listenbrainz.org", "labs.api.listenbrainz.org"}
	start := time.Now()
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			t2 := tr
			if i%2 == 1 {
				t2 = other
			}
			r, _ := http.NewRequest("GET", "https://"+hosts[i%len(hosts)]+"/x", nil)
			if _, err := t2.RoundTrip(r); err != nil {
				t.Error(err)
			}
			mu.Lock()
			at = append(at, time.Now())
			mu.Unlock()
		}()
	}
	wg.Wait()
	if el := time.Since(start); el < 7*gap-5*time.Millisecond {
		t.Fatalf("8 MetaBrainz calls took %v, want at least %v", el, 7*gap)
	}
	t0 := time.Now()
	for range 5 {
		r, _ := http.NewRequest("GET", "https://api.deezer.com/x", nil)
		tr.RoundTrip(r)
	}
	if time.Since(t0) > gap {
		t.Fatal("a non-MetaBrainz host waited on the pace")
	}
}

func TestConnError(t *testing.T) {
	yes := []error{
		&net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")},
		fmt.Errorf("Get x: %w", io.EOF),
		errors.New("remote error: tls: handshake failure"),
		errors.New("read tcp: connection reset by peer"),
		errors.New("net/http: TLS handshake timeout"),
	}
	for _, e := range yes {
		if !ConnError(e) {
			t.Errorf("%v: want connection failure", e)
		}
	}
	for _, e := range []error{nil, errors.New("status 500"), errors.New("decode: invalid character")} {
		if ConnError(e) {
			t.Errorf("%v: not a connection failure", e)
		}
	}
}

// Three connection failures in a row to one host raise the alarm (a log
// line and the host listed as failing); an answer ends it.
func TestHealthAlarm(t *testing.T) {
	var logs []string
	h := NewHealth(func(f string, a ...any) { logs = append(logs, fmt.Sprintf(f, a...)) })
	reset := errors.New("read tcp: connection reset by peer")
	h.Observe("musicbrainz.org", reset)
	h.Observe("musicbrainz.org", reset)
	if h.Failing("musicbrainz.org") || len(logs) != 0 {
		t.Fatal("alarm before 3")
	}
	h.Observe("coverartarchive.org", nil)
	h.Observe("musicbrainz.org", errors.New("status 404")) // an answer, but not a connection outcome: ignored
	h.Observe("musicbrainz.org", reset)
	if !h.Failing("musicbrainz.org") || len(logs) != 1 || !strings.Contains(logs[0], "ALARM") {
		t.Fatalf("no alarm: %v", logs)
	}
	rep := h.Report()
	if len(rep.Failing) != 1 || rep.Failing[0] != "musicbrainz.org" || len(rep.Hosts) != 1 || rep.Hosts[0].Run != 3 || rep.Hosts[0].Alarms != 1 {
		t.Fatalf("report %+v", rep)
	}
	h.Observe("musicbrainz.org", nil)
	if h.Failing("musicbrainz.org") || len(logs) != 2 || !strings.Contains(logs[1], "answers again") {
		t.Fatalf("recovery: %v", logs)
	}
	if rep := h.Report(); rep.Hosts[0].LongestRun != 3 || len(rep.Failing) != 0 {
		t.Fatalf("after recovery %+v", rep)
	}
}

// The transport reports what it saw; a call the caller gave up on says
// nothing about the host.
func TestTransportFeedsHealth(t *testing.T) {
	h := NewHealth(nil)
	fail := Transport{Next: rtFunc(func(*http.Request) (*http.Response, error) { return nil, io.ErrUnexpectedEOF }), Pace: NewPace(time.Millisecond), Health: h}
	for range 3 {
		r, _ := http.NewRequest("GET", "https://musicbrainz.org/ws/2/artist/x", nil)
		fail.RoundTrip(r)
	}
	if !h.Failing("musicbrainz.org") {
		t.Fatal("not failing")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	h2 := NewHealth(nil)
	gone := Transport{Next: rtFunc(func(r *http.Request) (*http.Response, error) { return nil, r.Context().Err() }), Pace: NewPace(time.Millisecond), Health: h2}
	r, _ := http.NewRequestWithContext(ctx, "GET", "https://api.deezer.com/x", nil)
	gone.RoundTrip(r)
	if len(h2.Report().Hosts) != 0 {
		t.Fatal("a cancelled call was counted")
	}
}

// A burst of callers that give up while waiting never pushes the next slot
// past the cap: the tail gives its slot back, the rest are refused at once
// with ErrBusy, and a caller after the burst still gets a slot within the
// cap.
func TestCancelledWaitersStayWithinTheCap(t *testing.T) {
	gap := 50 * time.Millisecond
	p := NewPace(gap)
	p.max = 8
	var wg sync.WaitGroup
	busy := 0
	var mu sync.Mutex
	for range 500 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
			defer cancel()
			if errors.Is(p.Wait(ctx), ErrBusy) {
				mu.Lock()
				busy++
				mu.Unlock()
			}
		}()
		p.mu.Lock()
		ahead := time.Until(p.next)
		p.mu.Unlock()
		if ahead > time.Duration(p.max+1)*gap {
			t.Fatalf("next slot %v ahead, cap %v", ahead, time.Duration(p.max)*gap)
		}
	}
	wg.Wait()
	if busy == 0 {
		t.Fatal("no caller was refused past the cap")
	}
	p.mu.Lock()
	ahead := time.Until(p.next)
	p.mu.Unlock()
	if ahead > time.Duration(p.max)*gap {
		t.Fatalf("after the burst the next slot is %v ahead", ahead)
	}
	start := time.Now()
	if err := p.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if w := time.Since(start); w > time.Duration(p.max+1)*gap {
		t.Fatalf("a later caller waited %v", w)
	}
	if p.Stats().Busy != int64(busy) {
		t.Fatalf("busy stat %d, refused %d", p.Stats().Busy, busy)
	}
}

// The waiter holding the last slot gives it back when it gives up.
func TestTailGivesSlotBack(t *testing.T) {
	p := NewPace(time.Second)
	_ = p.Wait(context.Background()) // takes now, next = now+1s
	p.mu.Lock()
	before := p.next
	p.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := p.Wait(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.next.Equal(before) {
		t.Fatalf("next %v, want %v given back", p.next, before)
	}
}

func TestBusyIsNotAConnectionFailure(t *testing.T) {
	if ConnError(ErrBusy) || ConnError(fmt.Errorf("Get x: %w", ErrBusy)) {
		t.Fatal("ErrBusy taken for a connection failure")
	}
}
