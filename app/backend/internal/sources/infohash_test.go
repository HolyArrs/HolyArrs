package sources

import "testing"

// TestInfoHash_FromASourceIsChecked checks that a Source's info hash is kept only when it is 40 or 64 hex digits.
// Any other value, such as "all" or "a|b", would select other torrents.
func TestInfoHash_FromASourceIsChecked(t *testing.T) {
	for v, want := range map[string]string{
		"ABCDEF0123456789ABCDEF0123456789ABCDEF01": "abcdef0123456789abcdef0123456789abcdef01",
		"all": "", "a|b": "", "abcdef0123456789abcdef0123456789abcdef01|29be7bfa8759643f2d5dac9f94d516c3378696ed": "",
		"abcdef0123456789abcdef0123456789abcdef0g": "",
	} {
		body := []byte(`<?xml version="1.0"?><rss xmlns:torznab="http://torznab.com/schemas/2015/feed"><channel><item><title>A.2021.1080p.WEB-DL-X</title>
<enclosure url="http://src/a.torrent" length="1" type="application/x-bittorrent"/><torznab:attr name="infohash" value="` + v + `"/></item></channel></rss>`)
		items, err := parseItems(body, "torrent")
		if err != nil || len(items) != 1 {
			t.Fatalf("%q: %v %d", v, err, len(items))
		}
		if items[0].InfoHash != want {
			t.Fatalf("%q kept as %q, want %q", v, items[0].InfoHash, want)
		}
	}
}
