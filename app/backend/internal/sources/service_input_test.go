package sources

import (
	"errors"
	"strings"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// TestKeyMarker_PlantedIsNotFilled checks that a marker typed into a Source address is not filled with a stored key.
func TestKeyMarker_PlantedIsNotFilled(t *testing.T) {
	keys := []string{"realkey123"}
	link := "magnet:?xt=urn:btih:abc&tr=http://evil.example/announce?k=" + hidden + "0r"
	back, err := revealLink(cleanLink(link, keys), keys)
	if err != nil || strings.Contains(back, "realkey123") {
		t.Fatalf("key filled into a planted marker: %q %v", back, err)
	}
}

// TestRSSKey_SplitOnSemicolon checks that an rss key with several parts is split on the semicolon.
func TestRSSKey_SplitOnSemicolon(t *testing.T) {
	cfg := Config{Format: "rss"}
	cfg.APIKey = "u=123;t" + "p=Zq7wXy99;download"
	keys := secretValues(cfg)
	got := cleanLink("https://t.example/download.php/1/x.torrent?t=Zq7wXy99", keys)
	if strings.Contains(got, "Zq7wXy99") {
		t.Fatalf("passkey kept: %q", got)
	}
}

// TestIndexerAddress_WithQueryRefused checks that a Torznab or Newznab address with a query is refused.
func TestIndexerAddress_WithQueryRefused(t *testing.T) {
	var src domain.Source
	_, _, err := fill(Input{Name: "n", Format: "newznab", URL: "https://api.example/api?apikey=XXXX"}, &src, true)
	if !errors.Is(err, ErrQuery) || !errors.Is(err, ErrURL) {
		t.Fatalf("err %v", err)
	}
	var rss domain.Source
	if _, _, err := fill(Input{Name: "r", Format: "rss", URL: "https://t.example/rss?passkey=abcd"}, &rss, true); err != nil {
		t.Fatalf("rss with query: %v", err)
	}
}
