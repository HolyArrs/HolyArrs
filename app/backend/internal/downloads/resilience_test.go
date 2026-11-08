package downloads

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type kickSync struct {
	SyncSource
	kicks []int64
}

func (k *kickSync) Kick(id int64) { k.kicks = append(k.kicks, id) }

func (k *kickSync) View(int64) SyncView { return SyncView{Degraded: true} }

func (k *kickSync) Fallback(int64, bool) {}

func TestForgetKicksReader(t *testing.T) {
	k := &kickSync{}
	m := &Manager{Sync: k}
	m.Forget(7)
	if len(k.kicks) != 1 || k.kicks[0] != 7 {
		t.Fatalf("kicks %v", k.kicks)
	}
}

func TestQbitAnswerSizeCap(t *testing.T) {
	big := "[" + strings.Repeat(`{"hash":"x"},`, (9<<20)/13) + `{"hash":"y"}]`
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(big))
	}))
	defer s.Close()
	q := NewQbittorrentKey(s.URL, "k", nil)
	ctx := context.Background()
	if _, err := q.call(ctx, http.MethodGet, "torrents/info", nil); err != nil {
		t.Fatalf("9 MiB torrents/info: %v", err)
	}
	if _, err := q.call(ctx, http.MethodGet, "app/version", nil); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("9 MiB app/version: %v", err)
	}
}
