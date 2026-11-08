package metadata

import (
	"testing"
	"time"
)

// A person may have proxyStreams streams going at once; an idle one frees
// its place.
func TestTrailerPassThroughCap(t *testing.T) {
	var p trailerRelay
	exp := time.Now().Add(time.Hour)
	ids := []string{}
	for _, m := range []string{"a", "b", "c", "d", "e"} {
		ids = append(ids, p.register(7, "https://manifest.googlevideo.com/api/manifest/hls_variant/"+m+"/index.m3u8", exp))
	}
	now := time.Now()
	for _, id := range ids[:proxyStreams] {
		s, err := p.begin(7, id, now)
		if err != nil {
			t.Fatal(err)
		}
		p.done(7, s, 10)
	}
	// The stream past proxyStreams waits.
	if _, err := p.begin(7, ids[len(ids)-1], now); err != errProxyBusy {
		t.Fatalf("stream past the cap: %v", err)
	}
	if s, err := p.begin(8, p.register(8, "https://manifest.googlevideo.com/api/manifest/hls_variant/a/index.m3u8", exp), now); err != nil {
		t.Fatalf("another person: %v", err)
	} else {
		p.done(8, s, 0)
	}
	// The others go quiet: the last may start.
	if s, err := p.begin(7, ids[len(ids)-1], now.Add(proxyIdle+time.Second)); err != nil {
		t.Fatalf("after idle: %v", err)
	} else {
		p.done(7, s, 0)
	}
	// In-flight requests on one stream are capped too.
	var held []*proxyStream
	for range proxyRequests {
		s, err := p.begin(9, p.register(9, "https://manifest.googlevideo.com/api/manifest/hls_variant/z/index.m3u8", exp), now)
		if err != nil {
			t.Fatal(err)
		}
		held = append(held, s)
	}
	if _, err := p.begin(9, p.register(9, "https://manifest.googlevideo.com/api/manifest/hls_variant/z/index.m3u8", exp), now); err != errProxyBusy {
		t.Fatalf("in flight: %v", err)
	}
	for _, s := range held {
		p.done(9, s, 0)
	}
}

// The whole install has at most proxyMaxActive streams going, and one
// stream relays for at most proxyLife.
func TestTrailerPassThroughInstallCap(t *testing.T) {
	var p trailerRelay
	exp := time.Now().Add(time.Hour)
	now := time.Now()
	manifest := func(m string) string {
		return "https://manifest.googlevideo.com/api/manifest/hls_variant/" + m + "/index.m3u8"
	}
	for u := int64(1); u <= proxyMaxActive; u++ {
		s, err := p.begin(u, p.register(u, manifest("a"), exp), now)
		if err != nil {
			t.Fatalf("person %d: %v", u, err)
		}
		p.done(u, s, 0)
	}
	if _, err := p.begin(99, p.register(99, manifest("a"), exp), now); err != errProxyBusy {
		t.Fatalf("over the install cap: %v", err)
	}
	// Relaying without a break past proxyLife is refused...
	var q trailerRelay
	id := q.register(1, manifest("a"), exp)
	at := now
	for ; at.Sub(now) <= proxyLife; at = at.Add(proxyIdle / 2) {
		s, err := q.begin(1, id, at)
		if err != nil {
			t.Fatalf("at %v: %v", at.Sub(now), err)
		}
		q.done(1, s, 0)
		s.last = at // done stamps the wall clock
	}
	if _, err := q.begin(1, id, at); err != errProxyBusy {
		t.Fatalf("past the stream life: %v", err)
	}
	// ...but the same trailer played again later (same id) starts afresh.
	if s, err := q.begin(1, id, at.Add(proxyIdle)); err != nil {
		t.Fatalf("a new play of an old stream: %v", err)
	} else {
		q.done(1, s, 0)
	}
}

// An idle restart clears a stream's own count, never the person's hourly bytes.
func TestTrailerProxyPersonCapSurvivesIdle(t *testing.T) {
	var p trailerRelay
	now := time.Now()
	id := p.register(1, "https://rr1---sn.googlevideo.com/m.m3u8", now.Add(time.Hour))
	for i := 0; i < 5; i++ {
		s, err := p.begin(1, id, now)
		if err != nil {
			if i >= 4 {
				return
			}
			t.Fatalf("play %d refused early: %v", i, err)
		}
		p.done(1, s, proxyMaxStream)
		now = now.Add(proxyIdle + time.Second)
	}
	t.Fatal("hourly cap never stopped the replays")
}

// A stream with a request still running is not idle, however long it has been quiet.
func TestTrailerProxyBusyStreamNotFresh(t *testing.T) {
	var p trailerRelay
	now := time.Now()
	id := p.register(1, "https://rr1---sn.googlevideo.com/m.m3u8", now.Add(time.Hour))
	s, err := p.begin(1, id, now)
	if err != nil {
		t.Fatal(err)
	}
	started := s.started
	if _, err := p.begin(1, id, now.Add(proxyIdle+time.Second)); err != nil {
		t.Fatal(err)
	}
	if !s.started.Equal(started) {
		t.Fatal("a busy stream restarted its life")
	}
}
