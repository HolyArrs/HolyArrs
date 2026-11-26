package downloads

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
)

// soulseekPlugin brings the download type "soulseek" for plugin id "slskd".
func soulseekPlugin(t *testing.T) {
	t.Helper()
	sources.PluginDownloadTypes = func(id string) []string {
		if id == "slskd" {
			return []string{"soulseek"}
		}
		return nil
	}
	t.Cleanup(func() { sources.PluginDownloadTypes = nil })
}

// soulseekClient is a plugin's own download client: it takes the file the
// plugin's Source handed out and reports the job done once told to.
type soulseekClient struct {
	mu   sync.Mutex
	adds []AddRequest
	done bool
}

func (c *soulseekClient) Health(context.Context) (Health, error) {
	return Health{Version: "slskd"}, nil
}
func (c *soulseekClient) Add(_ context.Context, r AddRequest) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.adds = append(c.adds, r)
	return "slsk-1", nil
}
func (c *soulseekClient) job() Job {
	c.mu.Lock()
	defer c.mu.Unlock()
	j := Job{ID: "slsk-1", Name: "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS", State: StateDownloading, Progress: 0.5, SavePath: "/data/downloads/complete/slsk-1"}
	if c.done {
		j.State, j.Progress = StateFinished, 1
		j.Files = []File{{Name: "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS.mkv", Size: 29588180, Progress: 1}}
	}
	return j
}
func (c *soulseekClient) List(context.Context, string) ([]Job, error) { return []Job{c.job()}, nil }
func (c *soulseekClient) Get(context.Context, string) (Job, error)    { return c.job(), nil }
func (c *soulseekClient) Remove(context.Context, string, bool) error  { return nil }

func TestPluginDownloaderTypeSettings(t *testing.T) {
	soulseekPlugin(t)
	for _, c := range []struct {
		name, url, protocol string
		ok                  bool
	}{
		{"declared type", "plugin://slskd", "soulseek", true},
		{"type the plugin does not declare", "plugin://slskd", "ed2k", false},
		{"another plugin", "plugin://other", "soulseek", false},
		{"torrent through the plugin kind", "plugin://slskd", "torrent", false},
		{"no type", "plugin://slskd", "", false},
		{"not a plugin address", "http://10.0.0.2:5030", "soulseek", false},
	} {
		var d domain.Downloader
		err := fill(Input{Name: "Soulseek", Adapter: "plugin", URL: c.url, Protocol: c.protocol}, &d, true)
		if (err == nil) != c.ok || c.ok && (d.Protocol != c.protocol || d.Adapter != "plugin") {
			t.Errorf("%s: %v %+v", c.name, err, d)
		}
	}
	d := domain.Downloader{Name: "Soulseek", Adapter: "plugin", Protocol: "soulseek", URL: "plugin://slskd"}
	if err := fill(Input{Name: "Soulseek", Adapter: "plugin", Protocol: "other"}, &d, false); err == nil {
		t.Error("an edit changed the download type")
	}
	if err := fill(Input{Name: "Renamed", Adapter: "plugin"}, &d, false); err != nil || d.Protocol != "soulseek" {
		t.Errorf("edit: %v %+v", err, d)
	}
	sources.PluginDownloadTypes = nil
	if err := fill(Input{Name: "Soulseek", Adapter: "plugin", URL: "plugin://slskd", Protocol: "soulseek"}, &domain.Downloader{}, true); err == nil {
		t.Error("a plugin type was saved without plugin support")
	}
}

// TestSoulseekSearchGrabDownloadImport: a plugin Source of type soulseek
// finds a Release, which goes only to the plugin's soulseek Downloader, is
// followed to finished and handed to the Import.
func TestSoulseekSearchGrabDownloadImport(t *testing.T) {
	soulseekPlugin(t)
	ctx := context.Background()
	w, m := setup(t)

	// Search: the plugin Source answers through its plugin; its results carry its type.
	sources.PluginTransport = func(id string) http.RoundTripper {
		return rtFunc(func(r *http.Request) (*http.Response, error) {
			if id != "slskd" {
				t.Errorf("search went to plugin %q", id)
			}
			body := `<?xml version="1.0"?><rss><channel><item><title>Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS</title><guid>1</guid><link>plugin://slskd/get?id=1</link><size>29588180</size></item></channel></rss>`
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/rss+xml"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
		})
	}
	t.Cleanup(func() { sources.PluginTransport = nil })
	items, err := sources.New(sources.Config{URL: "plugin://slskd", Protocol: "soulseek", Format: "torznab"}).Feed(ctx, nil)
	if err != nil || len(items) != 1 || items[0].Protocol != "soulseek" {
		t.Fatalf("search: %+v %v", items, err)
	}
	w.exec(t, "UPDATE sources SET protocol = 'soulseek', url = 'plugin://slskd', allow_local_network = 0")
	w.exec(t, "UPDATE releases SET protocol = ?, link_ref = ?, info_hash = NULL WHERE id = ?", items[0].Protocol, items[0].Link, w.relID)

	// Grab: no soulseek Downloader yet, so the torrent client never gets it.
	if _, err := m.Grab(ctx, w.slotID, w.relID, domain.System); !errors.As(err, new(*NoDownloaderError)) {
		t.Fatalf("soulseek Release without a soulseek Downloader: %v", err)
	}
	if len(w.fake.added) != 0 {
		t.Fatal("a soulseek Release went to the torrent client")
	}
	slsk := w.addDownloader(t, &domain.Downloader{Name: "Soulseek", Adapter: "plugin", Protocol: "soulseek", URL: "plugin://slskd", Category: "holyarrs",
		Priority: 1, PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok"})
	client := &soulseekClient{}
	m.PluginEngine = func(id string) Engine {
		if id != "slskd" {
			t.Errorf("engine for %q", id)
		}
		return client
	}
	m.SourceTransport = func(_ context.Context, src domain.Source) (http.RoundTripper, error) {
		return rtFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Scheme != "plugin" || r.URL.Host != "slskd" {
				t.Errorf("fetched %s outside the plugin", r.URL)
			}
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/octet-stream"}},
				Body: io.NopCloser(bytes.NewReader([]byte("slsk-transfer"))), Request: r}, nil
		}), nil
	}
	id, err := m.Grab(ctx, w.slotID, w.relID, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if d := w.download(t, id); d.DownloaderID != slsk || d.ExternalID != "slsk-1" {
		t.Fatalf("download %+v", d)
	}
	if len(client.adds) != 1 || string(client.adds[0].File) != "slsk-transfer" || len(w.fake.added) != 0 {
		t.Fatalf("plugin client got %+v, torrent client %d", client.adds, len(w.fake.added))
	}

	// Download, then Import.
	if _, err := m.SyncOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if d := w.download(t, id); d.State != "downloading" {
		t.Fatalf("want downloading, got %s", d.State)
	}
	client.mu.Lock()
	client.done = true
	client.mu.Unlock()
	if _, err := m.SyncOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if d := w.download(t, id); d.State != "finished" {
		t.Fatalf("want finished, got %s", d.State)
	}
	if len(w.finshed) != 1 || w.finshed[0] != id {
		t.Fatalf("Import not started: %v", w.finshed)
	}
}

// TestPluginTypeStaysWithItsPlugin: two plugins both bring soulseek; a
// Release from one plugin's Source goes only to that plugin's Downloader.
func TestPluginTypeStaysWithItsPlugin(t *testing.T) {
	sources.PluginDownloadTypes = func(string) []string { return []string{"soulseek"} }
	t.Cleanup(func() { sources.PluginDownloadTypes = nil })
	w, m := setup(t)
	w.exec(t, "UPDATE sources SET protocol = 'soulseek', url = 'plugin://slskd', allow_local_network = 0")
	w.exec(t, "UPDATE releases SET protocol = 'soulseek', link_ref = 'plugin://slskd/get?id=1', info_hash = NULL WHERE id = ?", w.relID)
	w.exec(t, "UPDATE downloaders SET adapter = 'plugin', protocol = 'soulseek', url = 'plugin://other'")
	asked := []string{}
	m.PluginEngine = func(id string) Engine { asked = append(asked, id); return &soulseekClient{} }
	if _, err := m.Grab(context.Background(), w.slotID, w.relID, domain.System); !errors.As(err, new(*NoDownloaderError)) {
		t.Fatalf("soulseek Release with only another plugin's Downloader: %v", err)
	}
	if len(asked) != 0 {
		t.Fatalf("sent across plugins: %v", asked)
	}
}

// TestTorrentReleaseNeverGoesToPluginType: a torrent Release with only a
// soulseek Downloader set up is refused, not sent there.
func TestTorrentReleaseNeverGoesToPluginType(t *testing.T) {
	soulseekPlugin(t)
	w, m := setup(t)
	w.exec(t, "UPDATE downloaders SET adapter = 'plugin', protocol = 'soulseek', url = 'plugin://slskd'")
	client := &soulseekClient{}
	m.PluginEngine = func(string) Engine { return client }
	if _, err := m.Grab(context.Background(), w.slotID, w.relID, domain.System); !errors.As(err, new(*NoDownloaderError)) {
		t.Fatalf("torrent Release with only a soulseek Downloader: %v", err)
	}
	if len(client.adds) != 0 {
		t.Fatal("a torrent Release went to the soulseek Downloader")
	}
}
