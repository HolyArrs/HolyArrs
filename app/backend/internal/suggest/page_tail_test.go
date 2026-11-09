package suggest

import "testing"

func TestTailSlice_Bounds(t *testing.T) {
	pg := Page{Tail: make([]Row, TailPage*2+1)}
	if r, more := pg.TailSlice(0); len(r) != TailPage || !more {
		t.Errorf("page 0: %d %v", len(r), more)
	}
	if r, more := pg.TailSlice(1); len(r) != TailPage || !more {
		t.Errorf("page 1: %d %v", len(r), more)
	}
	if r, more := pg.TailSlice(2); len(r) != 1 || more {
		t.Errorf("page 2: %d %v", len(r), more)
	}
	for _, n := range []int{3, -1} {
		if r, more := pg.TailSlice(n); r == nil || len(r) != 0 || more {
			t.Errorf("page %d: %v %v", n, r, more)
		}
	}
}
