package downloads

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

type requestRecorder struct {
	Engine
	request AddRequest
}

func TestConcurrentEngineCacheUsesOneSession(t *testing.T) {
	const count = 8
	entered, release := make(chan struct{}, count), make(chan struct{})
	m := &Manager{Engine: func(domain.Downloader) (Engine, error) {
		entered <- struct{}{}
		<-release
		return &Transmission{}, nil
	}}
	var wg sync.WaitGroup
	got := make([]Engine, count)
	for i := range got {
		wg.Add(1)
		go func() { defer wg.Done(); got[i], _ = m.engine(context.Background(), domain.Downloader{ID: 1}) }()
	}
	for range count {
		<-entered
	}
	close(release)
	wg.Wait()
	for _, e := range got {
		if e != got[0] {
			t.Fatal("concurrent callers received separate session engines")
		}
	}
}

func TestEditedDownloaderRejectsOldConstructor(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	oldEngine, freshEngine := &Transmission{}, &Transmission{}
	m := &Manager{Engine: func(d domain.Downloader) (Engine, error) {
		if d.URL == "http://old-address" {
			close(entered)
			<-release
			return oldEngine, nil
		}
		return freshEngine, nil
	}}
	type result struct {
		engine Engine
		err    error
	}
	old := make(chan result, 1)
	go func() {
		e, err := m.engine(context.Background(), domain.Downloader{ID: 1, URL: "http://old-address"})
		old <- result{e, err}
	}()
	<-entered
	m.Forget(1)
	fresh := domain.Downloader{ID: 1, URL: "http://new-address"}
	if e, err := m.engine(context.Background(), fresh); err != nil || e != freshEngine {
		t.Fatal("new settings did not create the fresh engine")
	}
	close(release)
	if r := <-old; r.err == nil || r.engine != nil {
		t.Fatal("constructor from before the edit was allowed to publish or return an engine")
	}
	if e, err := m.engine(context.Background(), fresh); err != nil || e != freshEngine {
		t.Fatal("stale constructor replaced edited settings")
	}
}

func (e *requestRecorder) Get(context.Context, string) (Job, error) { return Job{}, ErrNotFound }

func (e *requestRecorder) Add(_ context.Context, r AddRequest) (string, error) {
	e.request = r
	return "owned", nil
}

func TestOptionalGrabUsesSourcePolicyAndStableOperation(t *testing.T) {
	w, m := setup(t)
	ctx := context.Background()
	var d domain.Downloader
	var rel domain.Release
	if err := database.Get(ctx, w.s.Read(), "downloaders", int64(1), &d); err != nil {
		t.Fatal(err)
	}
	if err := database.Get(ctx, w.s.Read(), "releases", w.relID, &rel); err != nil {
		t.Fatal(err)
	}
	d.Adapter = "deluge"
	e := &requestRecorder{}
	m.Engine = func(domain.Downloader) (Engine, error) { return e, nil }
	for _, allowed := range []bool{false, true} {
		if err := w.s.Write(ctx, func(tx *sql.Tx) error {
			_, err := tx.Exec("UPDATE sources SET allow_local_network = ? WHERE id = ?", allowed, rel.SourceID)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if err := w.s.Write(ctx, func(tx *sql.Tx) error {
			if _, err := tx.Exec("UPDATE downloaders SET adapter = ? WHERE id = ?", d.Adapter, d.ID); err != nil {
				return err
			}
			_, err := tx.Exec("UPDATE downloader_attempts SET state = 'resolved'")
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := m.send(ctx, d, rel, w.slotID, ""); err != nil {
			t.Fatal(err)
		}
		if e.request.AllowLocal != allowed || e.request.OperationKey == "" {
			t.Fatalf("source permission or operation lost: %+v", e.request)
		}
	}
	first := e.request.OperationKey
	if _, err := m.send(ctx, d, rel, w.slotID, ""); err != nil {
		t.Fatal(err)
	}
	if first == "" || !e.request.ReconcileOnly || e.request.OperationKey != "" || e.request.AttemptMarker == "" {
		t.Fatal("retry did not become a lookup of the durable identity")
	}
}

func TestOptionalFactoryUsesGuardedClient(t *testing.T) {
	m := &Manager{}
	for _, kind := range []string{"transmission", "nzbget", "deluge", "rtorrent"} {
		e, err := m.EngineFor(context.Background(), domain.Downloader{Adapter: kind, URL: "http://127.0.0.1:1", Category: "holyarrs"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = e.Health(context.Background()); err == nil || errors.Is(err, ErrAuth) {
			t.Fatalf("%s bypassed transport policy: %v", kind, err)
		}
	}
}

func TestOptionalDownloaderKinds(t *testing.T) {
	for _, c := range []struct{ kind, protocol string }{{"transmission", "torrent"}, {"nzbget", "usenet"}, {"deluge", "torrent"}, {"rtorrent", "torrent"}} {
		t.Run(c.kind, func(t *testing.T) {
			var d domain.Downloader
			if err := fill(Input{Name: "Optional", Adapter: c.kind, URL: "http://downloader:9091"}, &d, true); err != nil {
				t.Fatal(err)
			}
			if d.Adapter != c.kind || d.Protocol != c.protocol || d.Category != "holyarrs" {
				t.Fatalf("saved kind: %+v", d)
			}
			if err := fill(Input{Name: d.Name, Adapter: "qbittorrent"}, &d, false); err == nil {
				t.Fatal("changed the kind of an existing Downloader")
			}
		})
	}
}
