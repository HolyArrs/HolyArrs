//go:build lab

package downloads

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"testing"
)

const labCategory = "holyarrs-591test"

// HolyArrs makes a missing category in the real SABnzbd and qBittorrent, and
// the test removes it after.
func TestLabMakeCategory(t *testing.T) {
	ctx := context.Background()
	t.Run("sabnzbd", func(t *testing.T) {
		key := os.Getenv("HOLYARRS_LAB_SAB_KEY")
		if key == "" {
			t.Skip("HOLYARRS_LAB_SAB_KEY not set")
		}
		s := NewSabnzbd("http://127.0.0.1:18707", key, nil)
		if has, err := s.HasCategory(ctx, labCategory); err != nil || has {
			t.Fatalf("before: has %v, %v", has, err)
		}
		if err := s.MakeCategory(ctx, labCategory); err != nil {
			t.Fatal(err)
		}
		defer func() {
			if _, err := s.call(ctx, "del_config", url.Values{"section": {"categories"}, "keyword": {labCategory}}, true); err != nil {
				t.Errorf("remove: %v", err)
			}
			if has, _ := s.HasCategory(ctx, labCategory); has {
				t.Error("category still there after remove")
			}
		}()
		has, err := s.HasCategory(ctx, labCategory)
		t.Logf("sabnzbd: %s there after make: %v", labCategory, has)
		if err != nil || !has {
			t.Fatalf("after: %v %v", has, err)
		}
		// Making it again is fine.
		if err := s.MakeCategory(ctx, labCategory); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("qbittorrent", func(t *testing.T) {
		key := os.Getenv("HOLYARRS_LAB_QBIT_KEY")
		if key == "" {
			t.Skip("HOLYARRS_LAB_QBIT_KEY not set (run through labctl qbt-apikey)")
		}
		q := NewQbittorrentKey("http://127.0.0.1:18708", key, nil)
		cats := func() map[string]struct {
			SavePath string `json:"savePath"`
		} {
			b, err := q.call(ctx, http.MethodGet, "torrents/categories", nil)
			if err != nil {
				t.Fatal(err)
			}
			var out map[string]struct {
				SavePath string `json:"savePath"`
			}
			_ = json.Unmarshal(b, &out)
			return out
		}
		if _, ok := cats()[labCategory]; ok {
			t.Fatal("already there")
		}
		if err := q.MakeCategory(ctx, labCategory); err != nil {
			t.Fatal(err)
		}
		defer func() {
			if _, err := q.call(ctx, http.MethodPost, "torrents/removeCategories", url.Values{"categories": {labCategory}}); err != nil {
				t.Errorf("remove: %v", err)
			}
			if _, ok := cats()[labCategory]; ok {
				t.Error("category still there after remove")
			}
		}()
		c, ok := cats()[labCategory]
		t.Logf("qbittorrent: %s there after make: %v, own save path %q", labCategory, ok, c.SavePath)
		if !ok || c.SavePath != "" {
			t.Fatalf("after: %v %+v", ok, c)
		}
		if err := q.MakeCategory(ctx, labCategory); err != nil {
			t.Fatalf("again: %v", err)
		}
	})
}
