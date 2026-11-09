package sources

import (
	"context"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

// uncachedJackettTransport keeps the real adapter and its dial policy, but makes the disposable bridge
// serve the current definition and fixture instead of a cached earlier result.
type uncachedJackettTransport struct {
	http.RoundTripper
}

// TestBridge_OptionalProwlarr runs the Source client against a disposable Prowlarr when one is set up, and skips otherwise.
func TestBridge_OptionalProwlarr(t *testing.T) {
	base := os.Getenv("HOLYARRS_PROWLARR_URL")
	key := os.Getenv("HOLYARRS_PROWLARR_API_KEY")
	keyFile := os.Getenv("HOLYARRS_PROWLARR_API_KEY_FILE")
	if base == "" && key == "" && keyFile == "" {
		t.Skip("disposable Prowlarr bridge is not running")
	}
	if base == "" || (key == "") == (keyFile == "") {
		t.Fatal("the disposable Prowlarr URL and exactly one key input are required")
	}
	if keyFile != "" {
		value, err := os.ReadFile(keyFile)
		if err != nil {
			t.Fatal("read disposable Prowlarr key file failed")
		}
		if len(value) > 4096 {
			t.Fatal("disposable Prowlarr key file is too large")
		}
		key = strings.TrimSpace(string(value))
		if key == "" {
			t.Fatal("disposable Prowlarr key file is empty")
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	c := New(Config{URL: base, APIKey: key, Protocol: "torrent", AllowLocal: true})
	caps, err := c.Caps(ctx)
	if err != nil {
		t.Fatal("read outward Prowlarr capabilities failed")
	}
	if !caps.Search || !caps.MovieSearch || !caps.TVSearch {
		t.Fatal("outward Prowlarr capabilities must offer search, movie, and TV")
	}
	items, err := c.Search(ctx, Query{Title: "HolyArrs Bridge Acceptance", Year: 2026, IMDb: "tt0123456", Categories: []int{2000}})
	if err != nil {
		t.Fatal("search outward Prowlarr feed failed")
	}
	if len(items) != 1 {
		t.Fatalf("search returned %d items, want 1", len(items))
	}
	got := items[0]
	if got.Title != "HolyArrs.Bridge.Acceptance.2026.1080p" || got.Protocol != "torrent" || got.Size != 4096 {
		t.Fatal("parsed disposable Prowlarr item does not match the synthetic fixture")
	}
	if got.InfoHash != "0123456789abcdef0123456789abcdef01234567" {
		t.Fatal("parsed disposable Prowlarr info hash does not match the synthetic fixture")
	}
	if got.Seeders == nil || *got.Seeders != 7 || got.Peers == nil || *got.Peers != 9 {
		t.Fatal("parsed disposable Prowlarr swarm counts do not match the synthetic fixture")
	}
}

// TestBridge_OptionalJackett runs the Source client against a disposable Jackett when one is set up, and skips otherwise.
func TestBridge_OptionalJackett(t *testing.T) {
	base := os.Getenv("HOLYARRS_JACKETT_URL")
	keyFile := os.Getenv("HOLYARRS_JACKETT_API_KEY_FILE")
	if base == "" && keyFile == "" {
		t.Skip("disposable Jackett bridge is not running")
	}
	if base == "" || keyFile == "" {
		t.Fatal("both the disposable Jackett URL and private key file are required")
	}
	key, err := os.ReadFile(keyFile)
	if err != nil {
		t.Fatal("read disposable Jackett key file failed")
	}
	if len(key) > 4096 || strings.TrimSpace(string(key)) == "" {
		t.Fatal("disposable Jackett key file is invalid")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	c := New(Config{URL: base, APIKey: strings.TrimSpace(string(key)), Protocol: "torrent", AllowLocal: true})
	c.http.Transport = uncachedJackettTransport{c.http.Transport}
	caps, err := c.Caps(ctx)
	if err != nil {
		t.Fatal("read outward Jackett capabilities failed")
	}
	if !caps.Search || !caps.Supports("search", "q") || caps.MovieSearch || caps.TVSearch || caps.MusicSearch {
		t.Fatal("disposable Jackett capabilities must offer plain text search only")
	}
	if !slices.Contains(caps.Categories, 2000) {
		t.Fatal("disposable Jackett movie category is missing")
	}
	items, err := c.Search(ctx, Query{Title: "HolyArrs Synthetic", Year: 2026, Categories: []int{2000}})
	if err != nil {
		t.Fatal("search outward Jackett feed failed")
	}
	if len(items) != 1 {
		t.Fatalf("search returned %d items, want 1", len(items))
	}
	got := items[0]
	if got.Title != "HolyArrs.Synthetic.2026.1080p" || got.Protocol != "torrent" || got.Size != 1048576 {
		t.Fatal("parsed disposable Jackett item does not match the synthetic fixture")
	}
	if got.Seeders == nil || *got.Seeders != 3 || got.GUID == "" || got.Link == "" {
		t.Fatal("parsed disposable Jackett item lacks its expected seeders or identity")
	}
}

func (t uncachedJackettTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Query().Get("t") != "search" {
		return t.RoundTripper.RoundTrip(req)
	}
	request := req.Clone(req.Context())
	query := request.URL.Query()
	query.Set("cache", "false")
	request.URL.RawQuery = query.Encode()
	return t.RoundTripper.RoundTrip(request)
}
