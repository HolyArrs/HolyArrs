package musictrend

import (
	"context"
	"testing"
)

// The prefetch must not make big sizes inside each fetch: that held one of
// the two picture CPU slots for a second download and a 1200 WebP encode per
// picture and capped the pass near 100 pictures a minute.
func TestPrefetchDefersBigSize(t *testing.T) {
	pics := &fakePictures{have: map[string]string{}, missed: map[string]bool{}, answer: map[string]string{}, busy: map[string]int{}}
	pics.answer["artist|"+id(1)] = "applemusic"
	pics.answer["album|"+id(101)] = "applemusic"
	p := newPrefetch(pics, []string{id(1)}, nil, map[string][]string{id(1): {id(101)}})
	p.Pass(context.Background())
	if pics.fetches != 2 || pics.bigNow != 0 {
		t.Fatalf("fetches %d, made big inline %d; want 2 and 0", pics.fetches, pics.bigNow)
	}
}
