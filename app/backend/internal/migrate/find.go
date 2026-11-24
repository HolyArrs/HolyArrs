package migrate

import (
	"bufio"
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
)

// Found is an old app that answered a network search: its kind, the address
// to read it at, and the version when the app tells it without a key.
type Found struct {
	Kind    Kind   `json:"kind"`
	Address string `json:"address"`
	Version string `json:"version,omitempty"`
}

// findTarget is one host and port to try, with the apps that use the port.
type findTarget struct {
	Host  string
	Port  int
	Kinds []Kind
}

// Search bounds. A /24 times the known ports is about 2,000 tries; 256 at a
// time with a short wait each fits three /24s in the budget.
const (
	findTry      = 500 * time.Millisecond
	findBudget   = 15 * time.Second
	findParallel = 256
	findMaxHits  = 50
	// maxNets is how many /24s a search covers; three keep the worst case
	// inside findBudget.
	maxNets = 3
)

// defaultPorts are the ports the apps listen on out of the box. SABnzbd and
// qBittorrent share 8080.
var defaultPorts = []struct {
	port  int
	kinds []Kind
}{
	{8989, []Kind{Sonarr}}, {7878, []Kind{Radarr}}, {8686, []Kind{Lidarr}}, {9696, []Kind{Prowlarr}},
	{6767, []Kind{Bazarr}}, {5055, []Kind{Overseerr}}, {8080, []Kind{SABnzbd, QBittorrent}}, {9117, []Kind{Jackett}},
}

// dockerNames are the container names the apps' own guides use.
var dockerNames = map[Kind][]string{
	Sonarr: {"sonarr"}, Radarr: {"radarr"}, Lidarr: {"lidarr"}, Prowlarr: {"prowlarr"}, Bazarr: {"bazarr"},
	Overseerr: {"overseerr", "jellyseerr", "seerr"}, SABnzbd: {"sabnzbd"}, QBittorrent: {"qbittorrent"}, Jackett: {"jackett"},
}

// findTargets lists what a search tries: the Docker names on their app's
// port, then this machine, the Docker host and every address of each /24 in
// nets ("a.b.c") on every known port. Tests replace it.
var findTargets = func(nets []string) []findTarget {
	var out []findTarget
	for _, p := range defaultPorts {
		for _, k := range p.kinds {
			for _, n := range dockerNames[k] {
				out = append(out, findTarget{Host: n, Port: p.port, Kinds: []Kind{k}})
			}
		}
	}
	hosts := []string{"127.0.0.1", "host.docker.internal"}
	if gw := defaultGateway(); gw != "" {
		hosts = append(hosts, gw)
	}
	for _, n := range nets {
		for i := 1; i < 255; i++ {
			hosts = append(hosts, n+"."+strconv.Itoa(i))
		}
	}
	seen := map[string]bool{}
	for _, h := range hosts {
		if seen[h] {
			continue
		}
		seen[h] = true
		for _, p := range defaultPorts {
			out = append(out, findTarget{Host: h, Port: p.port, Kinds: p.kinds})
		}
	}
	return out
}

// privateNet gives the /24 ("a.b.c") of a private IPv4 address written as
// a bare IP, IP:port or URL; names, public and loopback addresses give "".
func privateNet(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	if h, _, err := net.SplitHostPort(s); err == nil {
		s = h
	}
	ip := net.ParseIP(s).To4()
	if ip == nil || !ip.IsPrivate() || ip.IsLoopback() {
		return ""
	}
	return strconv.Itoa(int(ip[0])) + "." + strconv.Itoa(int(ip[1])) + "." + strconv.Itoa(int(ip[2]))
}

// findNets picks the /24s a search covers, at most maxNets: those of the
// hints first (the address the browser used, then the apps already known),
// then this server's own private addresses (in Docker, the bridge).
func findNets(hints []string) []string {
	var out []string
	add := func(n string) {
		if n == "" || len(out) == maxNets {
			return
		}
		for _, x := range out {
			if x == n {
				return
			}
		}
		out = append(out, n)
	}
	for _, h := range hints {
		add(privateNet(h))
	}
	for _, ip := range ownAddresses() {
		add(privateNet(ip.String()))
	}
	return out
}

// ownAddresses are this server's private IPv4 addresses.
func ownAddresses() []net.IP {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	var out []net.IP
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok {
			if ip := n.IP.To4(); ip != nil && ip.IsPrivate() && !ip.IsLoopback() {
				out = append(out, ip)
			}
		}
	}
	return out
}

// defaultGateway reads the IPv4 default route on Linux (in Docker, the
// host); elsewhere it gives "".
func defaultGateway() string {
	f, err := os.Open("/proc/net/route")
	if err != nil {
		return ""
	}
	defer f.Close()
	s := bufio.NewScanner(io.LimitReader(f, 64<<10))
	for s.Scan() {
		fs := strings.Fields(s.Text())
		if len(fs) < 3 || fs[1] != "00000000" {
			continue
		}
		b, err := hex.DecodeString(fs[2])
		if err != nil || len(b) != 4 {
			continue
		}
		// The kernel writes the address in host order (little-endian).
		return net.IPv4(b[3], b[2], b[1], b[0]).String()
	}
	return ""
}

// Find tries every target for the /24s of hints (see findNets) and lists
// the old apps that answered, with the /24s it searched. Every connection
// goes through the local-network policy (never a cloud metadata address);
// no key is sent anywhere.
// With only set, it tries just those apps' ports (setup asks for one app,
// so the search answers in a second or two instead of the full budget).
func Find(ctx context.Context, hints []string, only ...Kind) ([]Found, []string) {
	ctx, cancel := context.WithTimeout(ctx, findBudget)
	defer cancel()
	tr := downloads.Client("http://x", true).Transport
	h := &http.Client{Transport: tr, Timeout: findTry,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer h.CloseIdleConnections()
	nets := findNets(hints)
	targets := onlyKinds(findTargets(nets), only)
	var (
		mu   sync.Mutex
		hits []Found
		wg   sync.WaitGroup
	)
	sem := make(chan struct{}, findParallel)
	for _, t := range targets {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
		}
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		go func(t findTarget) {
			defer wg.Done()
			defer func() { <-sem }()
			if f, ok := identify(ctx, h, t); ok {
				mu.Lock()
				if len(hits) < findMaxHits {
					hits = append(hits, f)
				}
				mu.Unlock()
			}
		}(t)
	}
	wg.Wait()
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Kind != hits[j].Kind {
			return hits[i].Kind < hits[j].Kind
		}
		return hits[i].Address < hits[j].Address
	})
	searched := make([]string, len(nets))
	for i, n := range nets {
		searched[i] = n + ".0/24"
	}
	return hits, searched
}

// identify asks one target which app it is, with the calls each app answers
// without a key.
func identify(ctx context.Context, h *http.Client, t findTarget) (Found, bool) {
	base := "http://" + net.JoinHostPort(t.Host, strconv.Itoa(t.Port))
	get := func(path string) (int, []byte) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
		if err != nil {
			return 0, nil
		}
		req.Header.Set("Accept", "application/json, text/html")
		resp, err := h.Do(req)
		if err != nil {
			return 0, nil
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		return resp.StatusCode, b
	}
	for _, k := range t.Kinds {
		switch k {
		case Sonarr, Radarr, Lidarr, Prowlarr:
			// Each arr app answers /ping without a key; the port says which.
			st, b := get("/ping")
			if st == 0 {
				return Found{}, false
			}
			var p struct {
				Status string `json:"status"`
			}
			if st == http.StatusOK && json.Unmarshal(b, &p) == nil && strings.EqualFold(p.Status, "ok") {
				return Found{Kind: k, Address: base}, true
			}
		case Overseerr, Seerr:
			st, b := get("/api/v1/status")
			if st == 0 {
				return Found{}, false
			}
			var s struct {
				Version string `json:"version"`
			}
			if st == http.StatusOK && json.Unmarshal(b, &s) == nil && s.Version != "" {
				kind := Overseerr
				if t.Host == "seerr" || t.Host == "jellyseerr" {
					kind = Seerr
				}
				return Found{Kind: kind, Address: base, Version: s.Version}, true
			}
		case SABnzbd:
			st, b := get("/api?mode=version&output=json")
			if st == 0 {
				return Found{}, false
			}
			var s struct {
				Version string `json:"version"`
			}
			if st == http.StatusOK && json.Unmarshal(b, &s) == nil && s.Version != "" {
				return Found{Kind: k, Address: base, Version: s.Version}, true
			}
		case Jackett:
			// Jackett's sign-in page answers without a key and names the app.
			st, b := get("/UI/Login")
			if st == 0 {
				return Found{}, false
			}
			if st == http.StatusOK && strings.Contains(strings.ToLower(string(b)), "jackett") {
				return Found{Kind: k, Address: base}, true
			}
		case QBittorrent, Bazarr:
			// Only the page says which app this is.
			st, b := get("/")
			if st == 0 {
				return Found{}, false
			}
			if strings.Contains(strings.ToLower(string(b)), strings.ToLower(kindName(k))) {
				return Found{Kind: k, Address: base}, true
			}
		}
	}
	return Found{}, false
}

// onlyKinds keeps the targets that can answer as one of kinds (all when empty).
func onlyKinds(targets []findTarget, kinds []Kind) []findTarget {
	if len(kinds) == 0 {
		return targets
	}
	out := targets[:0:0]
	for _, t := range targets {
		var keep []Kind
		for _, k := range t.Kinds {
			if slices.Contains(kinds, k) {
				keep = append(keep, k)
			}
		}
		if len(keep) > 0 {
			t.Kinds = keep
			out = append(out, t)
		}
	}
	return out
}
