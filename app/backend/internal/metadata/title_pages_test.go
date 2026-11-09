package metadata

import (
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"strconv"
	"testing"
	"time"
)

// Finding 15: the extras cache drops its oldest entries at the cap.
func TestExtrasCacheEvictsOldest(t *testing.T) {
	m := map[string]extrasEntry{}
	base := time.Now()
	for i := 0; i < 5001; i++ {
		webserver.KeepUnder(m, 5000, func(e extrasEntry) time.Time { return e.at })
		m[strconv.Itoa(i)] = extrasEntry{at: base.Add(time.Duration(i) * time.Second)}
	}
	if len(m) < 4000 || len(m) > 5000 {
		t.Fatalf("size %d", len(m))
	}
	if _, ok := m["5"]; ok {
		t.Fatal("oldest kept")
	}
	if _, ok := m["5000"]; !ok {
		t.Fatal("newest dropped")
	}
}
