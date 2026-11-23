//go:build lab

package downloads

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"slices"
	"testing"
)

const labMain = "holyarrs-591lab"

// Adding a lab SABnzbd and qBittorrent through the API picks their own
// categories and makes only the missing main one; the test removes what it made.
func TestLabSettleCategories(t *testing.T) {
	ctx := context.Background()
	t.Run("sabnzbd", func(t *testing.T) {
		key := os.Getenv("HOLYARRS_LAB_SAB_KEY")
		if key == "" {
			t.Skip("HOLYARRS_LAB_SAB_KEY not set")
		}
		s := NewSabnzbd("http://127.0.0.1:18707", key, nil)
		before, err := s.ListCategories(ctx)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("sabnzbd before: %v", before)
		defer func() {
			after, _ := s.ListCategories(ctx)
			for _, c := range after {
				if !slices.Contains(before, c) {
					_, _ = s.call(ctx, "del_config", url.Values{"section": {"categories"}, "keyword": {c}}, true)
				}
			}
			if left, _ := s.ListCategories(ctx); !slices.Equal(left, before) {
				t.Errorf("left behind: %v", left)
			}
		}()
		srv, _ := dlServer(t)
		var d DownloaderView
		_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/downloaders", body(map[string]any{"name": "Lab SAB", "kind": "sabnzbd", "url": "http://127.0.0.1:18707", "secret": key,
			"category": labMain, "allow_local_network": true}), 201), &d)
		after, _ := s.ListCategories(ctx)
		t.Logf("sabnzbd saved: shows=%q movies=%q music=%q; after: %v", d.CategoryShows, d.CategoryMovies, d.CategoryMusic, after)
		if d.CategoryShows != PickCategory("show", before) || d.CategoryMovies != PickCategory("movie", before) {
			t.Fatalf("picks %+v", d)
		}
		if !slices.Contains(after, labMain) {
			t.Fatal("main category not made")
		}
		var out struct {
			Listed bool              `json:"listed"`
			Picks  map[string]string `json:"picks"`
		}
		_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/downloaders/"+itoa(d.ID)+"/categories", "", 200), &out)
		t.Logf("sabnzbd list route: %+v", out)
	})
	t.Run("qbittorrent", func(t *testing.T) {
		key := os.Getenv("HOLYARRS_LAB_QBIT_KEY")
		if key == "" {
			t.Skip("HOLYARRS_LAB_QBIT_KEY not set")
		}
		q := NewQbittorrentKey("http://127.0.0.1:18708", key, nil)
		before, err := q.ListCategories(ctx)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("qbittorrent before: %v", before)
		// A throwaway Sonarr-style category, as a user who ran Sonarr has.
		made := []string{}
		if !slices.Contains(before, "tv-sonarr") {
			if err := q.MakeCategory(ctx, "tv-sonarr"); err != nil {
				t.Fatal(err)
			}
			made = append(made, "tv-sonarr")
		}
		defer func() {
			after, _ := q.ListCategories(ctx)
			for _, c := range after {
				if !slices.Contains(before, c) {
					_, _ = q.call(ctx, http.MethodPost, "torrents/removeCategories", url.Values{"categories": {c}})
				}
			}
			if left, _ := q.ListCategories(ctx); !slices.Equal(left, before) {
				t.Errorf("left behind: %v", left)
			}
		}()
		srv, _ := dlServer(t)
		var d DownloaderView
		_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/downloaders", body(map[string]any{"name": "Lab qBit", "kind": "qbittorrent", "url": "http://127.0.0.1:18708", "secret": key,
			"category": labMain, "allow_local_network": true}), 201), &d)
		after, _ := q.ListCategories(ctx)
		t.Logf("qbittorrent saved: shows=%q movies=%q music=%q; after: %v (made for the test: %v)", d.CategoryShows, d.CategoryMovies, d.CategoryMusic, after, made)
		if d.CategoryShows != "tv-sonarr" || !slices.Contains(after, labMain) {
			t.Fatalf("picks %+v after %v", d, after)
		}
	})
}
