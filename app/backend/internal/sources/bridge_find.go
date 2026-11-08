package sources

import (
	"bytes"
	"context"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
)

// findHosts are the usual places a bridge runs beside HolyArrs: this
// machine, the Docker host and the Docker addresses. Only these are tried,
// the network is never scanned.
var findHosts = []string{"localhost", "127.0.0.1", "host.docker.internal", "172.17.0.1"}

// findPorts are the default ports of each kind.
var findPorts = map[string]string{BridgeKindJackett: "9117", BridgeKindProwlarr: "9696"}

// BridgeFindHosts returns the hosts to try; tests replace it to point at fakes.
var BridgeFindHosts = func() []string { return findHosts }

// BridgeFound is a Jackett or Prowlarr that answered at an address.
type BridgeFound struct {
	Kind string `json:"kind"`
	URL  string `json:"url"`
}

// FindBridges tries the usual addresses at once with a short wait and returns the
// ones where a bridge answered, Jackett first. It only fetches one public
// page per address and sends no key.
func FindBridges(ctx context.Context) []BridgeFound {
	var cands []BridgeFound
	for _, kind := range []string{BridgeKindJackett, BridgeKindProwlarr} {
		// The kind itself is also a host: the usual Docker service name.
		hosts := append(slices.Clone(BridgeFindHosts()), kind)
		for _, h := range hosts {
			if !strings.Contains(h, ":") {
				h += ":" + findPorts[kind]
			}
			cands = append(cands, BridgeFound{Kind: kind, URL: "http://" + h})
		}
	}
	hc := client(2 * time.Second)
	var mu sync.Mutex
	var out []BridgeFound
	var wg sync.WaitGroup
	for _, c := range cands {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if BridgeLooks(ctx, hc, c.Kind, c.URL) {
				mu.Lock()
				out = append(out, c)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	// localhost and 127.0.0.1 are the same bridge, so keep one answer for them.
	slices.SortFunc(out, func(a, b BridgeFound) int {
		if a.Kind != b.Kind {
			return strings.Compare(a.Kind, b.Kind)
		}
		return strings.Compare(rank(a.URL), rank(b.URL))
	})
	seen := map[string]bool{}
	var uniq []BridgeFound
	for _, f := range out {
		port := f.URL[strings.LastIndex(f.URL, ":"):]
		local := strings.Contains(f.URL, "localhost") || strings.Contains(f.URL, "127.0.0.1")
		key := f.Kind
		if !local {
			key += f.URL
		} else {
			key += "local" + port
		}
		if !seen[key] {
			seen[key] = true
			uniq = append(uniq, f)
		}
	}
	return uniq
}

// BridgeLooks says whether the address answers like that kind, without a key.
// Prowlarr answers its ping with an OK status. Jackett answers a call
// without a key with its own invalid-key error (code 100, status 200),
// while its sign-in page only redirects. A wrong guess costs nothing since
// the Test with the key decides.
func BridgeLooks(ctx context.Context, hc *http.Client, kind, base string) bool {
	path := "/api/v2.0/indexers/all/results/torznab/api?t=caps"
	if kind == BridgeKindProwlarr {
		path = "/ping"
	}
	status, b, err := call(ctx, hc, http.MethodGet, strings.TrimRight(base, "/")+path, nil, nil, 64<<10)
	if err != nil {
		return false
	}
	low := bytes.ToLower(b)
	if kind == BridgeKindProwlarr {
		return status == http.StatusOK && bytes.Contains(low, []byte(`"status"`)) && bytes.Contains(low, []byte("ok"))
	}
	return status == http.StatusOK && bytes.Contains(low, []byte("<error")) && bytes.Contains(low, []byte(`code="100"`))
}

// rank orders addresses so the Docker name sorts before localhost and
// localhost before the rest, which decides which duplicate is kept.
func rank(u string) string {
	switch {
	case strings.Contains(u, "://jackett:"), strings.Contains(u, "://prowlarr:"):
		return "0" + u
	case strings.Contains(u, "localhost"):
		return "1" + u
	}
	return "2" + u
}
