package bridge

import (
	"bytes"
	"context"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
)

// Found is a Jackett or Prowlarr that answered at an address.
type Found struct {
	Kind string `json:"kind"`
	URL  string `json:"url"`
}

// findHosts are the usual places a Jackett or Prowlarr runs beside
// HolyArrs: this machine, the Docker host, and the usual Docker names. Only
// these are tried, never a scan of the network.
var findHosts = []string{"localhost", "127.0.0.1", "host.docker.internal", "172.17.0.1"}

var findPorts = map[string]string{KindJackett: "9117", KindProwlarr: "9696"}

// FindHosts lets tests point the finder at their fake servers.
var FindHosts = func() []string { return findHosts }

// Find tries the usual addresses at once, each with a short wait, and
// answers the ones where Jackett or Prowlarr answered, Jackett first.
// Nothing is sent but a plain look at a page that needs no key.
func Find(ctx context.Context) []Found {
	var cands []Found
	for _, kind := range []string{KindJackett, KindProwlarr} {
		hosts := append(slices.Clone(FindHosts()), kind) // the Docker name
		for _, h := range hosts {
			if !strings.Contains(h, ":") {
				h += ":" + findPorts[kind]
			}
			cands = append(cands, Found{Kind: kind, URL: "http://" + h})
		}
	}
	hc := client(2 * time.Second)
	var mu sync.Mutex
	var out []Found
	var wg sync.WaitGroup
	for _, c := range cands {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if Looks(ctx, hc, c.Kind, c.URL) {
				mu.Lock()
				out = append(out, c)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	// One answer per bridge: localhost and 127.0.0.1 are the same one.
	slices.SortFunc(out, func(a, b Found) int {
		if a.Kind != b.Kind {
			return strings.Compare(a.Kind, b.Kind)
		}
		return strings.Compare(rank(a.URL), rank(b.URL))
	})
	seen := map[string]bool{}
	var uniq []Found
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

func rank(u string) string {
	switch {
	case strings.Contains(u, "://jackett:"), strings.Contains(u, "://prowlarr:"):
		return "0" + u
	case strings.Contains(u, "localhost"):
		return "1" + u
	}
	return "2" + u
}

// Looks says whether the address answers like that kind, without a key:
// Prowlarr's health probe answers an OK status, and Jackett's feed answers a
// call without a key with its own "invalid key" error (code 100, status 200;
// its sign-in page only redirects). A wrong guess costs nothing; the Test
// with the key decides.
func Looks(ctx context.Context, hc *http.Client, kind, base string) bool {
	path := "/api/v2.0/indexers/all/results/torznab/api?t=caps"
	if kind == KindProwlarr {
		path = "/ping"
	}
	status, b, err := call(ctx, hc, http.MethodGet, strings.TrimRight(base, "/")+path, nil, nil, 64<<10)
	if err != nil {
		return false
	}
	low := bytes.ToLower(b)
	if kind == KindProwlarr {
		return status == http.StatusOK && bytes.Contains(low, []byte(`"status"`)) && bytes.Contains(low, []byte("ok"))
	}
	return status == http.StatusOK && bytes.Contains(low, []byte("<error")) && bytes.Contains(low, []byte(`code="100"`))
}
