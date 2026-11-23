package plugins

import (
	"fmt"
	"strings"
	"testing"
)

// TestCapsAtTheEdge: hosts, owner hosts, folders and settings take exactly
// 100 distinct entries and refuse 101.
func TestCapsAtTheEdge(t *testing.T) {
	list := func(n int, f func(i int) string) string {
		var out []string
		for i := range n {
			out = append(out, f(i))
		}
		return strings.Join(out, ",")
	}
	man := func(hosts, folders string) []byte {
		return []byte(`{"id":"x","name":"X","version":"1.0.0","author":"a","min_app_version":"1.3.0","sha256":"` + strings.Repeat("a", 64) +
			`","access":{"hosts":[` + hosts + `],"folders":[` + folders + `],"actions":[]}}`)
	}
	host := func(i int) string { return fmt.Sprintf(`"h%d.example.com"`, i) }
	folder := func(i int) string { return fmt.Sprintf(`{"name":"f%d","mode":"read"}`, i) }
	for _, n := range []int{100, 101} {
		_, errH := ParseManifest(man(list(n, host), ""))
		_, errF := ParseManifest(man("", list(n, folder)))
		var owners []string
		for i := range n {
			owners = append(owners, fmt.Sprintf("o%d.example.com", i))
		}
		_, errO := cleanOwnerHosts(owners)
		var sets []Setting
		for i := range n {
			sets = append(sets, Setting{Key: fmt.Sprintf("k%d", i), Label: fmt.Sprintf("Field %d", i), Type: "text"})
		}
		errS := checkSettings(sets)
		ok := n == 100
		for name, err := range map[string]error{"hosts": errH, "folders": errF, "owner hosts": errO, "settings": errS} {
			if (err == nil) != ok {
				t.Errorf("%d %s: %v", n, name, err)
			}
		}
	}
}
