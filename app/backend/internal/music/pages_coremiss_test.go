package music

import (
	"testing"
	"time"
)

// An album core that answered 404 answers so again for a minute without a
// lookup (a genre page asks the core of every album it lists).
func TestAlbumCoreMissRemembered(t *testing.T) {
	t.Parallel()
	a := newTestAPI(Server{Version: "test"})
	const al, ar = "66666666-6666-4666-8666-666666666666", "77777777-7777-4777-8777-777777777777"
	if a.coreMissed(al, ar) {
		t.Fatal("missed before any ask")
	}
	a.rememberCoreMiss(al, ar)
	if !a.coreMissed(al, ar) || a.coreMissed(al, "other") {
		t.Fatal("miss not remembered for that album and artist only")
	}
	a.music.mu.Lock()
	a.music.coreMiss[al+"|"+ar] = time.Now().Add(-albumCoreMiss - time.Second)
	a.music.mu.Unlock()
	if a.coreMissed(al, ar) {
		t.Fatal("miss kept past its minute")
	}
}
