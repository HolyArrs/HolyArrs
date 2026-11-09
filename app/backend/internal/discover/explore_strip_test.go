package discover

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Varying the rows never grows the strip cache past one entry per member,
// tab and limit, and too many or too long rows are refused.
func TestExploreStripCacheBounded(t *testing.T) {
	t.Parallel()
	h := &Handlers{}
	for i := 0; i < 1000; i++ {
		rows := []string{"Row " + strconv.Itoa(i)}
		keepStrip(h.stripMemberKey(7)+"shows|", stripEntry{at: time.Now(), sig: "d|" + StripRowsHash(rows)})
	}
	n := 0
	stripAnswers.Range(func(k, _ any) bool {
		if strings.HasPrefix(k.(string), h.stripMemberKey(7)) {
			n++
		}
		return true
	})
	if n != 1 {
		t.Fatalf("%d entries for one member and tab", n)
	}
	if StripRowsHash([]string{"B", "a"}) != StripRowsHash([]string{"A", "b"}) {
		t.Fatal("hash not normalised")
	}
	for q, ok := range map[string]bool{"row=" + strings.Repeat("x", 81): false, "row=" + strings.Repeat("&row=x", 41): false, "row=a&row=b": true} {
		r := httptest.NewRequest(http.MethodGet, "/x?"+q, nil)
		if _, got := StripRows(r); got != ok {
			t.Errorf("%.20s... = %v", q, got)
		}
	}
}
