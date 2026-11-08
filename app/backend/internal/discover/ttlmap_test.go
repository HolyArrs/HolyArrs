package discover

import (
	"testing"
	"time"
)

func TestTTLMapExpiresAndCaps(t *testing.T) {
	t.Parallel()
	m := NewTTLMap(time.Hour, 3)
	m.Store("a", 1)
	if v, ok := m.Load("a"); !ok || v.(int) != 1 {
		t.Fatalf("load a = %v %v", v, ok)
	}
	m.m["a"] = ttlEntry{v: 1, at: time.Now().Add(-2 * time.Hour)}
	if _, ok := m.Load("a"); ok {
		t.Fatal("expired entry still loads")
	}
	for i := range 10 {
		m.Store(i, i)
	}
	if n := m.Len(); n > 3 {
		t.Fatalf("len = %d, want at most 3", n)
	}
}
