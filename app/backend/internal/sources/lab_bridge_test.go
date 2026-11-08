//go:build lab

package sources

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// TestLab_ProwlarrBridge talks to a real Prowlarr and is skipped unless its key is set.
func TestLab_ProwlarrBridge(t *testing.T) {
	base := os.Getenv("HOLYARRS_LAB_PROWLARR")
	if base == "" {
		base = "http://127.0.0.1:18704/1/api"
	}
	key := os.Getenv("HOLYARRS_LAB_PROWLARR_KEY")
	if key == "" {
		t.Skip("HOLYARRS_LAB_PROWLARR_KEY not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c := New(Config{URL: base, APIKey: key, Protocol: "torrent", AllowLocal: true})
	caps, err := c.Caps(ctx)
	if err != nil {
		t.Fatalf("caps through Prowlarr: %v", err)
	}
	if !caps.Search || !caps.MovieSearch {
		t.Fatalf("Prowlarr's feed should offer search and movie search: %+v", caps)
	}
	items, err := c.Search(ctx, Query{Title: "Dune Part Two", Year: 2024, Categories: []int{2000}})
	if err != nil {
		t.Fatalf("search through Prowlarr: %v", err)
	}
	var dune *Item
	for i := range items {

		if items[i].Title == "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS" {
			dune = &items[i]
			break
		}
	}
	if dune == nil {
		t.Fatalf("no Dune test torrent in %d items through Prowlarr", len(items))
	}

	if dune.Protocol != "torrent" || !strings.Contains(dune.Link, "/download") {
		t.Fatalf("Dune through Prowlarr: protocol %q, link %q", dune.Protocol, dune.Link)
	}
	if dune.InfoHash != "" && !strings.EqualFold(dune.InfoHash, "29be7bfa8759643f2d5dac9f94d516c3378696ed") {
		t.Fatalf("Dune through Prowlarr: wrong hash %q", dune.InfoHash)
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, dune.Link, nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("download link: %v", err)
	}
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK || !strings.HasPrefix(string(body), "d") || !strings.Contains(string(body), "4:info") {
		t.Fatalf("download link answered %d with %d bytes that are not a torrent", res.StatusCode, len(body))
	}
	seeds := -1
	if dune.Seeders != nil {
		seeds = *dune.Seeders
	}
	t.Logf("through Prowlarr: %d items, Dune %s (%d bytes, %d seeders)", len(items), dune.Title, dune.Size, seeds)
}
