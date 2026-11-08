package downloads

import (
	"errors"
	"strings"
	"testing"
)

// UsenetID lets real job ids through and stops anything that could change
// the Usenet app's query string.
func TestUsenetID(t *testing.T) {
	for _, ok := range []string{"SABnzbd_nzo_ab12", "3f2a-11ee.b", "nzbid-21", strings.Repeat("a", 100)} {
		if got, err := UsenetID(ok); err != nil || got != ok {
			t.Errorf("%q: %v", ok, err)
		}
	}
	for _, bad := range []string{"", strings.Repeat("a", 101), "a&mode=delete", "a=b", "a b", "../x", "a/b", "id?x=1", "a#b", "a%26", "é", "a\nb"} {
		if _, err := UsenetID(bad); !errors.Is(err, ErrNotFound) {
			t.Errorf("%q passed", bad)
		}
	}
}
