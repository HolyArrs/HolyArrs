package downloads

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// pluginClient is a plugin download client as the Manager sees it.
type pluginClient struct {
	mu   sync.Mutex
	adds []AddRequest
}

func (p *pluginClient) Health(context.Context) (Health, error) { return Health{Version: "stub"}, nil }
func (p *pluginClient) Add(_ context.Context, r AddRequest) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.adds = append(p.adds, r)
	return hash, nil
}
func (p *pluginClient) List(context.Context, string) ([]Job, error) { return nil, nil }
func (p *pluginClient) Get(context.Context, string) (Job, error) {
	return Job{ID: hash, State: StateDownloading}, nil
}
func (p *pluginClient) Remove(context.Context, string, bool) error { return nil }

type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// TestGrabToPluginClientAndFromPluginSource: a grab from a plugin search
// source reaches a plugin download client as the file, fetched only through
// that source plugin; the client never sees a link.
func TestGrabToPluginClientAndFromPluginSource(t *testing.T) {
	for _, c := range []struct {
		name, srcURL, link string
	}{
		{"plugin source", "plugin://idx", "plugin://idx/get?id=1"},
	} {
		t.Run(c.name, func(t *testing.T) {
			w, m := setup(t)
			w.exec(t, "UPDATE downloaders SET adapter = 'plugin-torrent', url = 'plugin://dl'")
			w.exec(t, "UPDATE sources SET url = ?, allow_local_network = 0", c.srcURL)
			w.exec(t, "UPDATE releases SET link_ref = ?, info_hash = NULL WHERE id = ?", c.link, w.relID)
			client := &pluginClient{}
			m.PluginEngine = func(id string) Engine {
				if id != "dl" {
					t.Errorf("engine for %q", id)
				}
				return client
			}
			var fetched []string
			m.SourceTransport = func(_ context.Context, src domain.Source) (http.RoundTripper, error) {
				return rtFunc(func(r *http.Request) (*http.Response, error) {
					fetched = append(fetched, r.URL.String())
					if r.URL.Scheme != "plugin" || r.URL.Host != "idx" {
						t.Errorf("fetched %s outside the plugin", r.URL)
					}
					return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/x-bittorrent"}},
						Body: io.NopCloser(bytes.NewReader(grabPayload)), Request: r}, nil
				}), nil
			}
			if _, err := m.Grab(context.Background(), w.slotID, w.relID, domain.System); err != nil {
				t.Fatal(err)
			}
			if len(client.adds) != 1 || !bytes.Equal(client.adds[0].File, grabPayload) {
				t.Fatalf("plugin client got %+v", client.adds)
			}
			if len(fetched) != 1 || !strings.HasPrefix(fetched[0], "plugin://idx/") {
				t.Errorf("fetched %v", fetched)
			}
		})
	}
}
