package hooks

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"
)

// newTestBroker grants each plugin the listed scopes; parents cover
// children as in accounts.PluginScopesCover (exact match is enough here).
func newTestBroker(grants map[string][]string) *Broker {
	b := NewBroker()
	b.Granted = func(p string) []string { return grants[p] }
	b.Covers = func(held []string, need string) bool { return slices.Contains(held, need) }
	return b
}

// stub answers every job on hook with fn, like a plugin's poll loop.
func stub(t *testing.T, b *Broker, plugin, hook string, fn func(Job) Answer) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ready := make(chan struct{})
	go func() {
		first := true
		for ctx.Err() == nil {
			if first {
				b.mu.Lock()
				b.laneLocked(plugin, hook).seen = b.now()
				b.mu.Unlock()
				close(ready)
				first = false
			}
			j, ok, err := b.Next(ctx, plugin, hook, 50*time.Millisecond)
			if err != nil {
				return
			}
			if ok {
				_ = b.Answer(plugin, hook, j.ID, fn(j))
			}
		}
	}()
	<-ready
}

func TestBrokerCallTable(t *testing.T) {
	echo := func(j Job) Answer { return Answer{Data: j.Payload} }
	for _, c := range []struct {
		name    string
		grants  map[string][]string
		serve   string // plugin that runs a stub loop
		call    string // plugin called
		answer  func(Job) Answer
		want    string
		wantErr error
	}{
		{"granted and running", map[string][]string{"dl": {"sources.downloader"}}, "dl", "dl", echo, `{"x":1}`, nil},
		{"not granted", map[string][]string{"dl": {"library.read"}}, "", "dl", echo, "", ErrNoProvider},
		{"granted but never polled", map[string][]string{"dl": {"sources.downloader"}}, "", "dl", echo, "", ErrNoProvider},
		{"other plugin serves", map[string][]string{"dl": {"sources.downloader"}, "x": {"sources.downloader"}}, "x", "dl", echo, "", ErrNoProvider},
		{"plugin error", map[string][]string{"dl": {"sources.downloader"}}, "dl", "dl", func(Job) Answer { return Answer{Error: "disk full"} }, "", errors.New("disk full")},
	} {
		t.Run(c.name, func(t *testing.T) {
			b := newTestBroker(c.grants)
			if c.serve != "" {
				stub(t, b, c.serve, Downloader, c.answer)
			}
			got, err := b.Call(context.Background(), c.call, Downloader, "add", map[string]int{"x": 1}, time.Second)
			if c.wantErr != nil {
				if err == nil || err.Error() != c.wantErr.Error() {
					t.Fatalf("err = %v, want %v", err, c.wantErr)
				}
				return
			}
			if err != nil || string(got) != c.want {
				t.Fatalf("got %s, %v", got, err)
			}
		})
	}
}

func TestBrokerAnswerOnlyByOwner(t *testing.T) {
	b := newTestBroker(map[string][]string{"a": {"sources.releases"}, "b": {"sources.releases"}})
	b.mu.Lock()
	b.laneLocked("a", Releases).seen = b.now()
	b.mu.Unlock()
	done := make(chan error, 1)
	go func() {
		_, err := b.Call(context.Background(), "a", Releases, "search", nil, 2*time.Second)
		done <- err
	}()
	j, ok, err := b.Next(context.Background(), "a", Releases, time.Second)
	if !ok || err != nil {
		t.Fatal("no job", err)
	}
	if b.Answer("b", Releases, j.ID, Answer{Data: json.RawMessage(`1`)}) == nil {
		t.Error("another plugin answered a's job")
	}
	if b.Answer("a", Downloader, j.ID, Answer{}) == nil {
		t.Error("an answer on another hook was taken")
	}
	if err := b.Answer("a", Releases, j.ID, Answer{Data: json.RawMessage(`1`)}); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestBrokerDropEndsWorkAtOnce(t *testing.T) {
	b := newTestBroker(map[string][]string{"a": {"assistant.tools"}})
	b.mu.Lock()
	b.laneLocked("a", Tools).seen = b.now()
	b.mu.Unlock()
	done := make(chan error, 1)
	go func() {
		_, err := b.Call(context.Background(), "a", Tools, "call", nil, 5*time.Second)
		done <- err
	}()
	time.Sleep(20 * time.Millisecond)
	b.Drop("a")
	select {
	case err := <-done:
		if !errors.Is(err, ErrDropped) {
			t.Fatalf("err = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("call kept waiting after drop")
	}
	if len(b.Providers(Tools)) != 0 {
		t.Error("dropped plugin still a provider")
	}
}

func TestBrokerNextRefusedWithoutScope(t *testing.T) {
	b := newTestBroker(map[string][]string{"a": {"library.read"}})
	if _, _, err := b.Next(context.Background(), "a", Downloader, time.Millisecond); !errors.Is(err, ErrNoProvider) {
		t.Fatalf("err = %v", err)
	}
	if _, _, err := b.Next(context.Background(), "a", "nope", time.Millisecond); !errors.Is(err, ErrNoProvider) {
		t.Fatalf("unknown hook: %v", err)
	}
}
