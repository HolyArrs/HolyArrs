package accounts

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// scanPorts are the usual Jellyfin and Emby web ports.
var scanPorts = []string{"8096", "8097", "8920"}

const (
	scanWorkers = 256
	scanTimeout = 400 * time.Millisecond
)

// scanNear looks for Jellyfin and Emby servers by asking each address of
// near's /24 network directly, for when the broadcast search gets no answer
// (a container on a bridge network cannot reach the home network's
// broadcast). Only a private IPv4 address is scanned.
func scanNear(ctx context.Context, near string) []foundServer {
	ip := net.ParseIP(strings.TrimSpace(near)).To4()
	if ip == nil || !ip.IsPrivate() {
		return nil
	}
	client := &http.Client{
		Timeout:       scanTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	type job struct{ host, port string }
	jobs := make(chan job)
	var mu sync.Mutex
	var out []foundServer
	var wg sync.WaitGroup
	for range scanWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				if f, ok := probe(ctx, client, j.host, j.port); ok {
					mu.Lock()
					if len(out) < discoverMax {
						out = append(out, f)
					}
					mu.Unlock()
				}
			}
		}()
	}
	for last := 1; last < 255; last++ {
		host := fmt.Sprintf("%d.%d.%d.%d", ip[0], ip[1], ip[2], last)
		for _, p := range scanPorts {
			select {
			case jobs <- job{host, p}:
			case <-ctx.Done():
			}
		}
	}
	close(jobs)
	wg.Wait()
	return out
}

func probe(ctx context.Context, c *http.Client, host, port string) (foundServer, bool) {
	addr := "http://" + net.JoinHostPort(host, port)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, addr+"/System/Info/Public", nil)
	if err != nil {
		return foundServer{}, false
	}
	resp, err := c.Do(req)
	if err != nil {
		return foundServer{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return foundServer{}, false
	}
	var v struct {
		ServerName  string `json:"ServerName"`
		ProductName string `json:"ProductName"`
		Version     string `json:"Version"`
		ID          string `json:"Id"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 16<<10)).Decode(&v) != nil || v.ID == "" || v.Version == "" {
		return foundServer{}, false
	}
	kind := ProviderEmby
	if strings.Contains(strings.ToLower(v.ProductName), "jellyfin") {
		kind = ProviderJellyfin
	}
	name := strings.TrimSpace(v.ServerName)
	if name == "" {
		name = kindWord[kind]
	}
	return foundServer{Kind: kind, Name: truncate(name, 100), Address: addr}, true
}

// RealCase is realCase for a name and address from elsewhere (the media
// server test answer).
func RealCase(ctx context.Context, name, addr string) string {
	return realCase(ctx, foundServer{Name: name, Address: addr}).Name
}

// realCase gives a server name the computer's own spelling. Jellyfin and Emby
// on Windows report the name in capitals (KINGSTREAMINGPC); the home network's
// DNS (the system resolver, then the router at .1) usually knows it as typed
// (KingStreamingPC). Only a name equal apart from case is replaced.
func realCase(ctx context.Context, f foundServer) foundServer {
	u, err := url.Parse(f.Address)
	if err != nil {
		return f
	}
	ip := net.ParseIP(u.Hostname()).To4()
	if ip == nil {
		return f
	}
	ctx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
	defer cancel()
	router := &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, network, fmt.Sprintf("%d.%d.%d.1:53", ip[0], ip[1], ip[2]))
	}}
	for _, r := range []*net.Resolver{net.DefaultResolver, router} {
		names, err := r.LookupAddr(ctx, ip.String())
		if err != nil {
			continue
		}
		for _, n := range names {
			label := strings.SplitN(strings.TrimSuffix(n, "."), ".", 2)[0]
			if label != "" && strings.EqualFold(label, f.Name) {
				f.Name = label
				return f
			}
		}
	}
	return f
}
