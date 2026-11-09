package downloads

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// ControlTorrentClient refuses a removed or Usenet Downloader without running
// the call, and queues a refresh after a write.
func TestControlTorrentClientGuards(t *testing.T) {
	ctx := context.Background()
	w, m := setup(t)
	m.Engine = func(domain.Downloader) (Engine, error) { return &Qbittorrent{}, nil }
	add := func(name, adapter, protocol string, removed bool) int64 {
		d := &domain.Downloader{Name: name, Adapter: adapter, Protocol: protocol, URL: "http://" + name + ":1", PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok", Removed: removed}
		if err := w.s.Write(ctx, func(tx *sql.Tx) error { _, err := database.Insert(tx, "downloaders", d, domain.System); return err }); err != nil {
			t.Fatal(err)
		}
		return d.ID
	}
	woke := func() bool {
		m.mu.Lock()
		defer m.mu.Unlock()
		return m.wake != nil && len(m.wake) > 0
	}
	ran := false
	call := func(Engine) error { ran = true; return nil }
	if err := m.ControlTorrentClient(ctx, add("sab", "sabnzbd", "usenet", false), true, call); !errors.Is(err, ErrNotSupported) || ran {
		t.Fatalf("usenet: %v ran=%v", err, ran)
	}
	if err := m.ControlTorrentClient(ctx, add("gone", "qbittorrent", "torrent", true), true, call); !errors.Is(err, ErrNoDownloader) || ran {
		t.Fatalf("removed: %v ran=%v", err, ran)
	}
	if err := m.ControlTorrentClient(ctx, 9999, true, call); !errors.Is(err, ErrNoDownloader) || ran {
		t.Fatalf("unknown: %v ran=%v", err, ran)
	}
	if woke() {
		t.Fatal("a refused call queued a refresh")
	}
	q := add("q", "qbittorrent", "torrent", false)
	if err := m.ControlTorrentClient(ctx, q, false, call); err != nil || !ran {
		t.Fatalf("read: %v ran=%v", err, ran)
	}
	if woke() {
		t.Fatal("a read queued a refresh")
	}
	ran = false
	if err := m.ControlTorrentClient(ctx, q, true, call); err != nil || !ran {
		t.Fatalf("write: %v ran=%v", err, ran)
	}
	if !woke() {
		t.Fatal("no poke after a write")
	}
	<-m.wake
	boom := errors.New("boom")
	if err := m.ControlTorrentClient(ctx, q, true, func(Engine) error { return boom }); !errors.Is(err, boom) || woke() {
		t.Fatalf("failing call: %v", err)
	}
}

// EngineFor needs the vault for a keyed Downloader, gives a plain error when
// the read fails, and builds the qBittorrent login or key client with the
// secret.
func TestEngineForVault(t *testing.T) {
	ctx := context.Background()
	ref := int64(7)
	d := domain.Downloader{Adapter: "qbittorrent", URL: "http://10.0.0.5:8080", SecretRef: &ref, Username: "u", AllowLocal: true}
	m := &Manager{}
	if _, err := m.EngineFor(ctx, d); err == nil || !strings.Contains(err.Error(), "vault is not open") {
		t.Fatal(err)
	}
	m.FromVault = func(context.Context, int64) (string, error) { return "", errors.New("x") }
	if _, err := m.EngineFor(ctx, d); err == nil {
		t.Fatal("want vault error")
	}
	m.FromVault = func(_ context.Context, id int64) (string, error) {
		if id != 7 {
			t.Errorf("secret id %d", id)
		}
		return "pw", nil
	}
	e, err := m.EngineFor(ctx, d)
	q, ok := e.(*Qbittorrent)
	if err != nil || !ok || q.pass != "pw" || q.user != "u" || q.key != "" {
		t.Fatalf("%v %#v", err, e)
	}
	d.Username = ""
	e, err = m.EngineFor(ctx, d)
	if q, ok = e.(*Qbittorrent); err != nil || !ok || q.key != "pw" || q.user != "" {
		t.Fatalf("key variant: %v %#v", err, e)
	}
	d.SecretRef, m.FromVault = nil, nil
	if _, err := m.EngineFor(ctx, d); err != nil {
		t.Fatalf("no secret: %v", err)
	}
}
